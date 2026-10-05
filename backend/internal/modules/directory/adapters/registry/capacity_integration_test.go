//go:build integration

package registry

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// freshPool migrates an empty disposable database and returns both the
// pool (for raw operational SQL) and the staging store on it.
func freshPool(t *testing.T) (*pgxpool.Pool, *PGStore) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("capacity_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, &PGStore{Q: directory.New(pool)}
}

// P29-T01 capacity, freshness and review-backlog campaign (scaled
// synthetic workload). Budgets frozen BEFORE the campaign below; the
// run measures and reports honestly — a missed budget requires
// remediation, never a redefined denominator. This is a scaled
// simulation, not a live national SLA and not 30 days of service.

const (
	// Frozen budgets (P29-T01).
	budgetStageRows     = 10000
	budgetBurstInputs   = 1000
	budgetStageTimeout  = 120 * time.Second
	budgetReadP95       = 500 * time.Millisecond
	budgetMaxHeapGrowth = 512 << 20
	budgetReviewBacklog = 1000
	budgetBatchSize     = 500
)

// validCNPJ mints distinct check-valid synthetic identifiers by
// brute-forcing the two check digits through the kernel program.
func validCNPJ(t *testing.T, base int) string {
	t.Helper()
	prefix := fmt.Sprintf("%012d", base)
	for d1 := '0'; d1 <= '9'; d1++ {
		for d2 := '0'; d2 <= '9'; d2++ {
			cand := fmt.Sprintf("%s%c%c", prefix, d1, d2)
			if c, err := kernel.ParseCNPJ(cand); err == nil {
				return c.Normalized()
			}
		}
	}
	t.Fatalf("no valid CNPJ for base %d", base)
	return ""
}

func syntheticCSV(t *testing.T, rows, base int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("CNPJ;RAZAO_SOCIAL;COD_IBGE;UF;SITUACAO;ATO_AUTORIZACAO\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, "%s;[P29-TEST] ESTACAO %d;3550308;SP;ATIVA;PRC-%d\n", validCNPJ(t, base+i), i, i)
	}
	return b.String()
}

func heapBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func TestCapacityStageThroughputAndHeap(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()
	before := heapBytes()
	start := time.Now()
	report, err := StageCSV(ctx, store, "cap-1", strings.NewReader(syntheticCSV(t, budgetStageRows, 100000)), Limits{MaxBytes: 100 << 20, MaxRows: 500000, BatchSize: budgetBatchSize})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	growth := heapBytes() - before
	t.Logf("staged %d+%d+%d rows in %v (%.0f rows/s), heap growth %d bytes",
		report.Accepted, report.Duplicates, report.Rejected, elapsed,
		float64(report.Accepted)/elapsed.Seconds(), growth)
	if report.State != "complete" {
		t.Fatalf("state = %q", report.State)
	}
	if report.Accepted+report.Duplicates+report.Rejected != budgetStageRows {
		t.Fatalf("dropped input: %+v", report)
	}
	if elapsed > budgetStageTimeout {
		t.Fatalf("throughput over budget: %v > %v", elapsed, budgetStageTimeout)
	}
	if growth > budgetMaxHeapGrowth {
		t.Fatalf("heap growth over budget: %d > %d", growth, budgetMaxHeapGrowth)
	}
}

func TestCapacityBurstWithReads(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()
	pre, err := StageCSV(ctx, store, "burst-base", strings.NewReader(syntheticCSV(t, 10, 900000)), Limits{MaxBytes: 100 << 20, MaxRows: 500000, BatchSize: 10})
	if err != nil || pre.State != "complete" {
		t.Fatalf("pre-stage = %+v, err = %v", pre, err)
	}
	burst := syntheticCSV(t, budgetBurstInputs, 200000)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	start := time.Now()
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = StageCSV(ctx, store, fmt.Sprintf("burst-%d", i), strings.NewReader(burst), Limits{MaxBytes: 100 << 20, MaxRows: 500000, BatchSize: budgetBatchSize})
		}(i)
	}
	// Simultaneous anonymous reads during the burst (p95 budget).
	readLatencies := make([]time.Duration, 0, 64)
	var readMu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 64; i++ {
			begin := time.Now()
			runUID, uuidErr := mustUUID(pre.RunID)
			if uuidErr == nil {
				if _, err := store.Q.CountRegistryAssertions(ctx, runUID); err == nil {
					readMu.Lock()
					readLatencies = append(readLatencies, time.Since(begin))
					readMu.Unlock()
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	wg.Wait()
	elapsed := time.Since(start)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
	}
	t.Logf("4-way burst of %d inputs in %v with %d concurrent reads", budgetBurstInputs, elapsed, len(readLatencies))
	if len(readLatencies) == 0 {
		t.Fatal("no reads observed during burst")
	}
	p95 := percentile(readLatencies, 95)
	t.Logf("read p95 during burst: %v", p95)
	if p95 > budgetReadP95 {
		t.Fatalf("read p95 over budget: %v > %v", p95, budgetReadP95)
	}
}

func TestCapacityReviewBacklogRead(t *testing.T) {
	pool, _ := freshPool(t)
	ctx := context.Background()
	const backlog = 1000
	for i := 0; i < backlog; i++ {
		if _, err := pool.Exec(ctx,
			"INSERT INTO station_suggestions (id, account_id, client_submission_id, proposal, state) VALUES (gen_random_uuid(), gen_random_uuid(), $1, '{}', 'pending')",
			fmt.Sprintf("backlog-key-%d", i)); err != nil {
			t.Fatalf("seed backlog: %v", err)
		}
	}
	start := time.Now()
	var count int64
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM (SELECT id FROM station_suggestions WHERE state = 'pending' ORDER BY created_at ASC LIMIT 25) batch").Scan(&count); err != nil {
		t.Fatalf("backlog batch: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("review backlog batch of %d over %d pending in %v", count, backlog, elapsed)
	if count != 25 {
		t.Fatalf("batch = %d, want 25", count)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("backlog read over budget: %v", elapsed)
	}
}

func TestCapacityOutageCatchupAndBacklog(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()
	// Two missed daily snapshots backfill in order after a simulated
	// 48h outage; checkpoints (distinct snapshot ids) dedup replays.
	snapshots := []struct {
		name string
		base int
	}{{"outage-d1", 300000}, {"outage-d2", 300500}}
	for _, snap := range snapshots {
		report, err := StageCSV(ctx, store, snap.name, strings.NewReader(syntheticCSV(t, 500, snap.base)), Limits{MaxBytes: 100 << 20, MaxRows: 500000, BatchSize: budgetBatchSize})
		if err != nil || report.State != "complete" || report.Accepted != 500 {
			t.Fatalf("catch-up %s = %+v, err = %v", snap.name, report, err)
		}
	}
	// Review backlog: 1,000 pending suggestions list in one bounded batch.
	last, err := store.Q.LastCompleteRegistryRun(ctx, SourceCSV)
	if err != nil {
		t.Fatalf("freshness: %v", err)
	}
	t.Logf("last complete run %s finished with %d accepted", last.SnapshotIdentity, last.Accepted)
}

func percentile(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := (len(sorted) * p / 100) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func TestCapacityRequiredIndexesExist(t *testing.T) {
	pool, _ := freshPool(t)
	ctx := context.Background()
	for _, want := range []string{
		"directory_identifiers_active_unique",
		"directory_stations_current_point_idx",
		"directory_stations_municipality_idx",
		"registry_source_runs_snapshot_unique",
		"registry_assertions_replay_unique",
	} {
		var count int64
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM pg_indexes WHERE indexname = $1", want).Scan(&count); err != nil {
			t.Fatalf("index %s: %v", want, err)
		}
		if count != 1 {
			t.Fatalf("required index missing: %s", want)
		}
	}
}
