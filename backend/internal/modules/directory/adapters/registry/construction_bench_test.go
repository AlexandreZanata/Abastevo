//go:build integration

package registry

// RST-14 construction baseline: the owned Go loader over an isolated
// real PostGIS database with per-stage timing (create, migrate,
// validate, stage, ANALYZE, index build) and WAL/storage metrics.
//
// Inputs come from the RST-13 chain: the committed tiny emit under
// contracts/testdata/station-prep/datasets/emit-tiny always runs;
// larger emits (representative 20k) run when RST14_EMIT_DIR_20K points
// at a bench_stages out dir, otherwise they Skip. Regenerate inputs
// with tools/station-prep bench_datasets + bench_stages to scratch;
// only the tiny set is committed.
//
// Publication stages stay NOT_AVAILABLE: staging never publishes, and
// no tested station-prep-to-canonical publication exists, so
// build-to-query-ready here means build-to-staged-ready (load complete
// plus count agreement plus ANALYZE) with explicit denominators.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// constructionDB holds an isolated fresh database with per-stage timings.
type constructionDB struct {
	store     *PGStore
	dsn       string
	createMS  float64
	migrateMS float64
	poolMS    float64
	cleanup   func()
}

// timedFreshStore replicates freshStoreWithDSN with per-stage timing so
// database initialization is measured separately from staging.
func timedFreshStore(b *testing.B) *constructionDB {
	b.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		b.Skipf("integration database unreachable: %v", err)
	}
	defer admin.Close(ctx)
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		b.Fatalf("parse dsn: %v", err)
	}
	name := fmt.Sprintf("registry_rst14_%d", time.Now().UnixNano())
	stage := time.Now()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		b.Fatalf("create database: %v", err)
	}
	createMS := time.Since(stage).Seconds() * 1000
	parsed.Path = "/" + name
	stage = time.Now()
	if _, err := migrate.Apply(ctx, parsed.String(), dbmigrations.Files); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	migrateMS := time.Since(stage).Seconds() * 1000
	stage = time.Now()
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		b.Fatalf("pool: %v", err)
	}
	poolMS := time.Since(stage).Seconds() * 1000
	cleanup := func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			return
		}
		defer admin.Close(ctx)
		_, _ = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
	}
	b.Cleanup(cleanup)
	return &constructionDB{
		store:     &PGStore{Q: directory.New(pool)},
		dsn:       parsed.String(),
		createMS:  createMS,
		migrateMS: migrateMS,
		poolMS:    poolMS,
		cleanup:   cleanup,
	}
}

// constructionMetrics snapshots WAL position, relation sizes, tuple
// counters and transaction commits for before/after deltas.
type constructionMetrics struct {
	walLSN     string
	tableBytes int64
	indexBytes int64
	liveTup    int64
	deadTup    int64
	xactCommit int64
	hasStat    bool
}

func captureMetrics(ctx context.Context, conn *pgx.Conn) (constructionMetrics, error) {
	var metrics constructionMetrics
	if err := conn.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&metrics.walLSN); err != nil {
		return metrics, err
	}
	const tables = "'registry_source_runs','registry_assertions'"
	if err := conn.QueryRow(ctx, `SELECT COALESCE(SUM(pg_total_relation_size(c.oid)),0),
		COALESCE(SUM(pg_indexes_size(c.oid)),0),
		COALESCE(SUM(s.n_live_tup),0), COALESCE(SUM(s.n_dead_tup),0)
		FROM pg_class c LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		WHERE c.relname IN (`+tables+`)`).Scan(
		&metrics.tableBytes, &metrics.indexBytes, &metrics.liveTup, &metrics.deadTup); err != nil {
		return metrics, err
	}
	if err := conn.QueryRow(ctx, `SELECT COALESCE(SUM(xact_commit),0) FROM pg_stat_database
		WHERE datname = current_database()`).Scan(&metrics.xactCommit); err != nil {
		return metrics, err
	}
	var ext string
	_ = conn.QueryRow(ctx, `SELECT extname FROM pg_extension WHERE extname='pg_stat_statements'`).Scan(&ext)
	metrics.hasStat = ext != ""
	return metrics, nil
}

func walBytes(ctx context.Context, conn *pgx.Conn, before, after string) (int64, error) {
	var delta int64
	if err := conn.QueryRow(ctx, `SELECT pg_wal_lsn_diff($1::pg_lsn,$2::pg_lsn)`, after, before).Scan(&delta); err != nil {
		return 0, err
	}
	return delta, nil
}

// emitInput reads one bench_stages out dir (manifest + three streams).
func emitInput(b *testing.B, dir string) ([]byte, BatchStreams) {
	b.Helper()
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			b.Skipf("emit input missing (generate it first): %v", err)
		}
		return raw
	}
	manifest := read("manifest.json")
	return manifest, BatchStreams{
		Assertions: read("assertions.jsonl"),
		Candidates: read("candidates.jsonl"),
		Quarantine: read("quarantine.jsonl"),
	}
}

func committedEmitDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..")
	return filepath.Join(root, "contracts", "testdata", "station-prep", "datasets", "emit-tiny")
}

// expectedCounts decodes manifest accounting for oracle agreement checks.
func expectedCounts(b *testing.B, manifestJSON []byte) map[string]BatchCounts {
	b.Helper()
	var manifest BatchManifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		b.Fatalf("manifest decode: %v", err)
	}
	return manifest.Counts
}

// analyzeStaging runs ANALYZE over the staging tables, timed separately.
func analyzeStaging(ctx context.Context, conn *pgx.Conn) float64 {
	stage := time.Now()
	_, _ = conn.Exec(ctx, "ANALYZE registry_source_runs, registry_assertions")
	return time.Since(stage).Seconds() * 1000
}

func assertConstructionComplete(b *testing.B, reports map[string]Report, want map[string]BatchCounts, store *PGStore) {
	b.Helper()
	for key, counts := range want {
		report, ok := reports[key]
		if !ok {
			b.Fatalf("input %q has no report", key)
		}
		if report.State != "complete" {
			b.Fatalf("input %q state = %q (%s)", key, report.State, report.ErrorCode)
		}
		if report.Accepted != int64(counts.Accepted) || report.Duplicates != int64(counts.Duplicates) || report.Rejected != int64(counts.Quarantined) {
			b.Fatalf("input %q report = %+v, oracle = %+v", key, report, counts)
		}
		if got := countConstructionRows(store, report.RunID); got != int64(counts.Accepted) {
			b.Fatalf("input %q live rows = %d, want %d", key, got, counts.Accepted)
		}
	}
}

func countConstructionRows(store *PGStore, runID string) int64 {
	uid, err := mustUUID(runID)
	if err != nil {
		return -1
	}
	n, err := store.Q.CountRegistryAssertions(context.Background(), uid)
	if err != nil {
		return -1
	}
	return n
}

func openFreshConn(b *testing.B, dsn string) *pgx.Conn {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		b.Fatalf("fresh connect: %v", err)
	}
	b.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func logConstruction(b *testing.B, db *constructionDB, what string, validateMS, loadMS, analyzeMS float64, before, after constructionMetrics, conn *pgx.Conn, accepted int64) {
	b.Helper()
	wal, err := walBytes(context.Background(), conn, before.walLSN, after.walLSN)
	if err != nil {
		b.Fatalf("wal diff: %v", err)
	}
	perRow := "undefined"
	if accepted > 0 {
		perRow = fmt.Sprintf("%.1fms/%.0fWAL-bytes", loadMS/float64(accepted), float64(wal)/float64(accepted))
	}
	b.ReportMetric(validateMS, "validate_ms")
	b.ReportMetric(loadMS, "load_ms")
	b.ReportMetric(float64(wal), "wal_bytes")
	b.Logf("%s: create=%.0fms migrate=%.0fms validate=%.1fms load=%.1fms analyze=%.1fms wal=%dB tableΔ=%dB indexΔ=%dB live=%d dead=%d xactΔ=%d per_accepted=%s stat_statements=%v",
		what, db.createMS, db.migrateMS, validateMS, loadMS, analyzeMS, wal,
		after.tableBytes-before.tableBytes, after.indexBytes-before.indexBytes,
		after.liveTup, after.deadTup, after.xactCommit-before.xactCommit, perRow, after.hasStat)
}

func BenchmarkConstructionInit(b *testing.B) {
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		b.ReportMetric(db.createMS, "create_ms")
		b.ReportMetric(db.migrateMS, "migrate_ms")
		b.ReportMetric(db.poolMS, "pool_ms")
		b.Logf("init: create=%.0fms migrate=%.0fms pool=%.0fms", db.createMS, db.migrateMS, db.poolMS)
	}
}

func BenchmarkConstructionEmpty(b *testing.B) {
	empty := BatchStreams{}
	manifest := BatchManifest{
		FormatVersion: FormatBatch,
		RunID:         "33333333-3333-4333-8444-555555555555",
		ParserVersion: "station-prep-v0.2.0",
		PolicyVersion: "station-policy-v1",
		Inputs:        []BatchInput{{Key: "registry-13col", Reference: "empty.csv", Edition: "synthetic-empty", EditionSeq: 1, SHA256: vectorChecksum("raw-empty"), Rows: 0}},
		Counts:        map[string]BatchCounts{"registry-13col": {}},
		Outputs: []BatchOutput{
			streamOutput(PrepAssertionsFile, empty.Assertions),
			streamOutput(PrepCandidatesFile, empty.Candidates),
			streamOutput(PrepQuarantineFile, empty.Quarantine),
		},
	}
	manifest.MunicipalityReference.Reference = "ibge-localidades-v1"
	manifest.MunicipalityReference.ReferenceHash = vectorChecksum("aliases")
	manifest.Completeness.EOFValidated = true
	manifest.Completeness.ExpectedManifest = true
	manifestJSON, err := jsonMarshal(manifest)
	if err != nil {
		b.Fatal(err)
	}
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		ctx := context.Background()
		conn := openFreshConn(b, db.dsn)
		before, err := captureMetrics(ctx, conn)
		if err != nil {
			b.Fatalf("metrics: %v", err)
		}
		stage := time.Now()
		if _, err := ValidateBatch(manifestJSON, empty); err != nil {
			b.Fatalf("validate: %v", err)
		}
		validateMS := time.Since(stage).Seconds() * 1000
		stage = time.Now()
		reports, err := LoadBatch(ctx, db.store, manifestJSON, empty)
		if err != nil {
			b.Fatalf("load: %v", err)
		}
		loadMS := time.Since(stage).Seconds() * 1000
		analyzeMS := analyzeStaging(ctx, conn)
		after, err := captureMetrics(ctx, conn)
		if err != nil {
			b.Fatalf("metrics: %v", err)
		}
		assertConstructionComplete(b, reports, map[string]BatchCounts{"registry-13col": {}}, db.store)
		logConstruction(b, db, "empty", validateMS, loadMS, analyzeMS, before, after, conn, 0)
	}
}

func constructionLoad(b *testing.B, manifestJSON []byte, streams BatchStreams, what string) {
	b.Helper()
	want := expectedCounts(b, manifestJSON)
	var accepted int64
	for _, counts := range want {
		accepted += int64(counts.Accepted)
	}
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		ctx := context.Background()
		conn := openFreshConn(b, db.dsn)
		before, err := captureMetrics(ctx, conn)
		if err != nil {
			b.Fatalf("metrics: %v", err)
		}
		stage := time.Now()
		if _, err := ValidateBatch(manifestJSON, streams); err != nil {
			b.Fatalf("validate: %v", err)
		}
		validateMS := time.Since(stage).Seconds() * 1000
		stage = time.Now()
		reports, err := LoadBatch(ctx, db.store, manifestJSON, streams)
		if err != nil {
			b.Fatalf("load: %v", err)
		}
		loadMS := time.Since(stage).Seconds() * 1000
		analyzeMS := analyzeStaging(ctx, conn)
		after, err := captureMetrics(ctx, conn)
		if err != nil {
			b.Fatalf("metrics: %v", err)
		}
		assertConstructionComplete(b, reports, want, db.store)
		logConstruction(b, db, what, validateMS, loadMS, analyzeMS, before, after, conn, accepted)
	}
}

func BenchmarkConstructionTiny(b *testing.B) {
	manifestJSON, streams := emitInput(b, committedEmitDir())
	constructionLoad(b, manifestJSON, streams, "tiny")
}

func BenchmarkConstructionReplay(b *testing.B) {
	manifestJSON, streams := emitInput(b, committedEmitDir())
	want := expectedCounts(b, manifestJSON)
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		ctx := context.Background()
		first, err := LoadBatch(ctx, db.store, manifestJSON, streams)
		if err != nil {
			b.Fatalf("first load: %v", err)
		}
		stage := time.Now()
		second, err := LoadBatch(ctx, db.store, manifestJSON, streams)
		if err != nil {
			b.Fatalf("replay load: %v", err)
		}
		replayMS := time.Since(stage).Seconds() * 1000
		assertConstructionComplete(b, second, want, db.store)
		for key := range want {
			if first[key].RunID != second[key].RunID {
				b.Fatalf("input %q replay changed run id", key)
			}
		}
		b.ReportMetric(replayMS, "replay_ms")
		b.Logf("replay: second load=%.1fms converges on stable run ids", replayMS)
	}
}

func BenchmarkConstructionConcurrent(b *testing.B) {
	manifestJSON, streams := emitInput(b, committedEmitDir())
	want := expectedCounts(b, manifestJSON)
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		ctx := context.Background()
		// Cold-concurrent first loads can observe a running snapshot, so
		// the owned pattern loads once, then reloads concurrently: all
		// reloads must converge on the first run ids.
		first, err := LoadBatch(ctx, db.store, manifestJSON, streams)
		if err != nil {
			b.Fatalf("first load: %v", err)
		}
		stage := time.Now()
		results := make([]map[string]Report, 4)
		errs := make([]error, 4)
		var wg sync.WaitGroup
		for worker := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reports, err := LoadBatch(ctx, db.store, manifestJSON, streams)
				results[worker] = reports
				errs[worker] = err
			}()
		}
		wg.Wait()
		wallMS := time.Since(stage).Seconds() * 1000
		for worker := range 4 {
			if errs[worker] != nil {
				b.Fatalf("worker %d: %v", worker, errs[worker])
			}
			assertConstructionComplete(b, results[worker], want, db.store)
		}
		for key := range want {
			for worker := 1; worker < 4; worker++ {
				if results[worker][key].RunID != first[key].RunID {
					b.Fatalf("input %q diverged across loaders", key)
				}
			}
		}
		assertConstructionComplete(b, first, want, db.store)
		b.ReportMetric(wallMS, "concurrent_4x_ms")
		b.Logf("concurrent: 4 duplicate loaders converge in %.1fms", wallMS)
	}
}

func committedEmitDeltaDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..")
	return filepath.Join(root, "contracts", "testdata", "station-prep", "datasets", "emit-tiny-delta1")
}

// BenchmarkConstructionEditionChange measures changed and stale
// editions. Staging dedups globally by (source, source_key, checksum),
// so a changed edition whose rows were already staged cannot agree
// with its manifest: the run fails count_mismatch with a visible
// failed state and stays invisible to publishers. Incremental editions
// need a separate loader task; this bench pins the current behavior.
func BenchmarkConstructionEditionChange(b *testing.B) {
	baseJSON, baseStreams := emitInput(b, committedEmitDir())
	deltaJSON, deltaStreams := emitInput(b, committedEmitDeltaDir())
	deltaWant := expectedCounts(b, deltaJSON)
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		ctx := context.Background()
		// Newer edition first: stages its changed row, completes.
		stage := time.Now()
		delta, err := LoadBatch(ctx, db.store, deltaJSON, deltaStreams)
		if err != nil {
			b.Fatalf("delta load: %v", err)
		}
		deltaMS := time.Since(stage).Seconds() * 1000
		assertConstructionComplete(b, delta, deltaWant, db.store)
		// Older edition after: global dedup starves the restage, so the
		// run must fail loudly instead of publishing partial work. The
		// registry input fails first (manifest order), so the pmqc input
		// is never staged: fail-fast leaves no partial second input.
		stage = time.Now()
		_, err = LoadBatch(ctx, db.store, baseJSON, baseStreams)
		staleMS := time.Since(stage).Seconds() * 1000
		if err == nil {
			b.Fatal("stale edition loaded, want count_mismatch failure")
		}
		var baseManifest BatchManifest
		if uerr := json.Unmarshal(baseJSON, &baseManifest); uerr != nil {
			b.Fatalf("manifest decode: %v", uerr)
		}
		registrySnap := "station-prep:" + baseManifest.RunID + ":registry-13col"
		report, gerr := db.store.GetRun(ctx, SourcePrep, registrySnap)
		if gerr != nil {
			b.Fatalf("failed run invisible: %v", gerr)
		}
		if report.State != "failed" {
			b.Fatalf("registry state = %q, want failed", report.State)
		}
		pmqcSnap := "station-prep:" + baseManifest.RunID + ":pmqc"
		if _, gerr := db.store.GetRun(ctx, SourcePrep, pmqcSnap); gerr == nil {
			b.Fatal("pmqc staged after registry failure, want fail-fast")
		}
		complete, cerr := db.store.Q.CountCompleteRegistryRuns(ctx, SourcePrep)
		if cerr != nil {
			b.Fatalf("complete runs: %v", cerr)
		}
		if complete != int64(len(deltaWant)) {
			b.Fatalf("complete runs = %d, want %d (failed run leaked)", complete, len(deltaWant))
		}
		b.ReportMetric(deltaMS, "delta_ms")
		b.ReportMetric(staleMS, "stale_ms")
		b.Logf("edition-change: delta=%.1fms completes, stale older edition=%.1fms fails count_mismatch with visible failed state", deltaMS, staleMS)
	}
}

func BenchmarkConstructionLarge(b *testing.B) {
	dir := os.Getenv("RST14_EMIT_DIR_20K")
	if dir == "" {
		b.Skip("RST14_EMIT_DIR_20K unset (generate a representative emit to scratch first)")
	}
	manifestJSON, streams := emitInput(b, dir)
	constructionLoad(b, manifestJSON, streams, "large-20k")
}

func BenchmarkConstructionIndex(b *testing.B) {
	dir := os.Getenv("RST14_EMIT_DIR_20K")
	if dir == "" {
		b.Skip("RST14_EMIT_DIR_20K unset (generate a representative emit to scratch first)")
	}
	manifestJSON, streams := emitInput(b, dir)
	for range make([]struct{}, b.N) {
		db := timedFreshStore(b)
		ctx := context.Background()
		if _, err := LoadBatch(ctx, db.store, manifestJSON, streams); err != nil {
			b.Fatalf("seed load: %v", err)
		}
		conn := openFreshConn(b, db.dsn)
		stage := time.Now()
		if _, err := conn.Exec(ctx, `CREATE INDEX rst14_probe_source_key ON registry_assertions (source_key, checksum)`); err != nil {
			b.Fatalf("create index: %v", err)
		}
		buildMS := time.Since(stage).Seconds() * 1000
		var size int64
		if err := conn.QueryRow(ctx, `SELECT pg_relation_size('rst14_probe_source_key')`).Scan(&size); err != nil {
			b.Fatalf("index size: %v", err)
		}
		stage = time.Now()
		if _, err := conn.Exec(ctx, `DROP INDEX rst14_probe_source_key`); err != nil {
			b.Fatalf("drop index: %v", err)
		}
		dropMS := time.Since(stage).Seconds() * 1000
		b.ReportMetric(buildMS, "index_build_ms")
		b.ReportMetric(float64(size), "index_bytes")
		b.Logf("index: fresh-lab build=%.1fms size=%dB drop=%.1fms (concurrent live-index creation NOT_APPLICABLE: no shared traffic in lab)", buildMS, size, dropMS)
	}
}
