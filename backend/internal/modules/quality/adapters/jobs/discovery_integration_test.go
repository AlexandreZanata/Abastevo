//go:build integration

package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshJobsDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("quality_jobs_test_%d", time.Now().UnixNano())
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
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	applied, err := migrate.Apply(ctx, u.String(), dbmigrations.Files)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	found := false
	for _, v := range applied {
		if v == "000095" {
			found = true
		}
	}
	if !found {
		t.Fatalf("watermark migration missing from %d applied versions", len(applied))
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type jobsIDMaker struct{}

var jobsIDs atomic.Int64

func (m *jobsIDMaker) next() (string, error) {
	return fmt.Sprintf("cccccccc-3333-4333-8333-%012d", jobsIDs.Add(1)), nil
}

func seedAssayEdition(t *testing.T, pool *pgxpool.Pool, label, hashChar, resSHA string) string {
	t.Helper()
	pub := adapters.NewPublisher(pool)
	maker := &jobsIDMaker{}
	rev, err := pub.Publish(context.Background(), application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: label,
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pmqc_" + label + ".zip",
			SHA256: resSHA, Bytes: 10,
		}},
		Counts:       domain.Counts{Input: 1, Accepted: 1},
		ManifestHash: strings.Repeat(hashChar, 64),
		Assays: []application.AssayInput{
			{SampleRef: "S-1", AssayRef: "A", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "92,6", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#A", Fingerprint: strings.Repeat("c", 64)},
		},
	}, maker.next)
	if err != nil {
		t.Fatalf("seed %s: %v", label, err)
	}
	return rev
}

func inventoryFixture() []application.InventoryEntry {
	return []application.InventoryEntry{
		{Label: "2026-06", URL: "u/06.zip", SHA256: strings.Repeat("6", 64)},
		{Label: "2026-07", URL: "u/07.zip", SHA256: strings.Repeat("7", 64)},
		{Label: "2026-08", URL: "", SHA256: ""},
		{Label: "2026-09", URL: "u/09.zip", SHA256: strings.Repeat("9", 64)},
		{Label: "2026-10", URL: "u/10.zip", SHA256: strings.Repeat("a", 64)},
		{Label: "2026-11", URL: "u/11.zip", SHA256: strings.Repeat("b", 64)},
	}
}

func TestDiscoveryEnqueuesBoundedChunk(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	seedAssayEdition(t, pool, "2026-06", "e", strings.Repeat("6", 64))
	seedAssayEdition(t, pool, "2026-07", "e", strings.Repeat("7", 64))

	var mu sync.Mutex
	var got []ImportPayload
	d := Discovery{
		Source: "pmqc", Stream: "pmqc-assay-v1", Pool: pool,
		List: func(ctx context.Context) ([]application.InventoryEntry, error) {
			return inventoryFixture(), nil
		},
		Enqueue: func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, payload)
			return platformjobs.EnqueueResult{ID: fmt.Sprintf("job-%d", len(got)), Created: true}, nil
		},
		MaxEditions: 2,
	}
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if len(got) != 2 || got[0].Edition != "2026-09" || got[1].Edition != "2026-10" {
		t.Fatalf("chunk = %+v, want oldest-first [2026-09 2026-10]", got)
	}
	for _, p := range got {
		if p.Version != 1 || p.Source != "pmqc" || p.Stream != "pmqc-assay-v1" || p.URL == "" || p.SHA256 == "" {
			t.Errorf("malformed payload %+v", p)
		}
	}
}

func TestDiscoveryConcurrentConverges(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	d := Discovery{
		Source: "pmqc", Stream: "pmqc-assay-v1", Pool: pool,
		List: func(ctx context.Context) ([]application.InventoryEntry, error) {
			return inventoryFixture(), nil
		},
		Enqueue: func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			raw, err := json.Marshal(payload)
			if err != nil {
				return platformjobs.EnqueueResult{}, err
			}
			return platformjobs.Enqueue(ctx, pool, "quality-import", raw, dedupe, 3, time.Now())
		},
		MaxEditions: 10,
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
				t.Errorf("concurrent discovery: %v", err)
			}
		}()
	}
	wg.Wait()
	ids := map[string]bool{}
	rows, err := pool.Query(ctx,
		"SELECT DISTINCT payload->>'edition' FROM job_queue WHERE kind = 'quality-import'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var edition string
		if err := rows.Scan(&edition); err != nil {
			t.Fatal(err)
		}
		ids[edition] = true
	}
	// Fresh ledger: 06/07/09/10/11 import, 08 still missing. Four
	// overlapping runs converge on exactly those five jobs, once each.
	for _, want := range []string{"2026-06", "2026-07", "2026-09", "2026-10", "2026-11"} {
		if !ids[want] {
			t.Errorf("edition %s never enqueued", want)
		}
	}
	if len(ids) != 5 {
		t.Fatalf("import jobs cover %v, want exactly the 5 listed editions", ids)
	}
}

func TestDiscoveryLateResourceImports(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	seedAssayEdition(t, pool, "2026-07", "e", strings.Repeat("7", 64))
	q := quality.New(pool)
	if err := q.SetWatermark(ctx, quality.SetWatermarkParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
		LastEdition: "2026-09", LastHash: strings.Repeat("e", 64),
	}); err != nil {
		t.Fatal(err)
	}
	var got []ImportPayload
	d := Discovery{
		Source: "pmqc", Stream: "pmqc-assay-v1", Pool: pool,
		List: func(ctx context.Context) ([]application.InventoryEntry, error) {
			return []application.InventoryEntry{
				{Label: "2026-02", URL: "u/02.zip", SHA256: strings.Repeat("2", 64)},
				{Label: "2026-07", URL: "u/07.zip", SHA256: strings.Repeat("7", 64)},
			}, nil
		},
		Enqueue: func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			got = append(got, payload)
			return platformjobs.EnqueueResult{ID: "job-1", Created: true}, nil
		},
		MaxEditions: 4,
	}
	if err := d.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if len(got) != 1 || got[0].Edition != "2026-02" {
		t.Fatalf("late resource = %+v, want single 2026-02 import behind the watermark", got)
	}
}

func fakePrepare(hash string) func([]byte) (PreparedBatch, error) {
	return func(raw []byte) (PreparedBatch, error) {
		return PreparedBatch{
			Assays: []application.AssayInput{
				{SampleRef: "S-1", AssayRef: "A", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "92,6", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#A", Fingerprint: strings.Repeat("c", 64)},
			},
			Links: []application.LinkInput{
				{RecordKey: "pmqc-assay-v1:S-1", Reason: "exact cnpj", Active: true},
			},
			Counts:       domain.Counts{Input: 1, Accepted: 1},
			ManifestHash: hash,
		}, nil
	}
}

func importJob(t *testing.T, raw []byte) platformjobs.Job {
	t.Helper()
	sum := sha256.Sum256(raw)
	payload, err := json.Marshal(ImportPayload{
		Version: 1, Source: "pmqc", Stream: "pmqc-assay-v1",
		Edition: "2026-09", URL: "u/09.zip",
		SHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	return platformjobs.Job{Payload: payload}
}

func TestImportPublishesAndAdvancesWatermark(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	raw := []byte("S-1;11222333000181;2026-09-04;ETANOL;A;92,6;%;CONFORME\n")
	hash := strings.Repeat("e", 64)
	im := Import{
		Pool: pool, Publish: adapters.NewPublisher(pool),
		Fetch:   func(ctx context.Context, url string) ([]byte, error) { return raw, nil },
		Prepare: fakePrepare(hash),
	}
	if err := im.Handle(ctx, importJob(t, raw)); err != nil {
		t.Fatalf("import: %v", err)
	}
	q := quality.New(pool)
	current, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	})
	if err != nil {
		t.Fatalf("lease after import: %v", err)
	}
	if !current.Valid {
		t.Fatal("lease points nowhere after import")
	}
	mark, err := q.GetWatermark(ctx, quality.GetWatermarkParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	})
	if err != nil {
		t.Fatalf("watermark after import: %v", err)
	}
	if mark.LastEdition != "2026-09" || mark.LastHash != hash {
		t.Errorf("watermark = %s/%s, want 2026-09/%s", mark.LastEdition, mark.LastHash, hash)
	}
}

func TestImportSupersededRetryDoesNotRegressWatermark(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	raw := []byte("old source")
	manifest := strings.Repeat("e", 64)
	im := Import{Pool: pool, Publish: adapters.NewPublisher(pool), Fetch: func(context.Context, string) ([]byte, error) { return raw, nil }, Prepare: func(b []byte) (PreparedBatch, error) { return fakePrepare(manifest)(b) }}
	oldJob := importJob(t, raw)
	if e := im.Handle(ctx, oldJob); e != nil {
		t.Fatal(e)
	}
	raw = []byte("corrected source")
	manifest = strings.Repeat("f", 64)
	if e := im.Handle(ctx, importJob(t, raw)); e != nil {
		t.Fatal(e)
	}
	q := quality.New(pool)
	current, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if e != nil {
		t.Fatal(e)
	}
	raw = []byte("old source")
	manifest = strings.Repeat("e", 64)
	if e := im.Handle(ctx, oldJob); e != nil {
		t.Fatal("obsolete retry not acknowledged", e)
	}
	mark, e := q.GetWatermark(ctx, quality.GetWatermarkParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if e != nil || mark.LastHash != strings.Repeat("f", 64) {
		t.Fatal("watermark regressed", mark, e)
	}
	actual, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if e != nil || actual != current {
		t.Fatal("publication regressed", actual, e)
	}
}

func TestImportHashMismatchFailsClean(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	im := Import{
		Pool: pool, Publish: adapters.NewPublisher(pool),
		Fetch:   func(ctx context.Context, url string) ([]byte, error) { return []byte("tampered"), nil },
		Prepare: fakePrepare(strings.Repeat("e", 64)),
	}
	raw := []byte("original")
	if err := im.Handle(ctx, importJob(t, raw)); err == nil {
		t.Fatal("mismatched content imported")
	}
	if _, err := quality.New(pool).GetEditionByKey(ctx, quality.GetEditionByKeyParams{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
	}); err == nil {
		t.Fatal("mismatched run left a visible edition")
	}
	if _, err := quality.New(pool).GetWatermark(ctx, quality.GetWatermarkParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	}); err == nil {
		t.Fatal("mismatched run moved the watermark")
	}
}

func TestImportFailurePreservesPublication(t *testing.T) {
	pool := freshJobsDB(t)
	ctx := context.Background()
	rev1 := seedAssayEdition(t, pool, "2026-09", "e", strings.Repeat("a", 64))
	raw := []byte("S-1;...\n")
	im := Import{
		Pool: pool, Publish: adapters.NewPublisher(pool),
		Fetch: func(ctx context.Context, url string) ([]byte, error) { return raw, nil },
		Prepare: func(raw []byte) (PreparedBatch, error) {
			return PreparedBatch{}, fmt.Errorf("prepare: injection failure")
		},
	}
	if err := im.Handle(ctx, importJob(t, raw)); err == nil {
		t.Fatal("failed preparation published")
	}
	q := quality.New(pool)
	current, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := testUUIDString(t, current); got != rev1 {
		t.Errorf("current moved to %s, want preserved %s", got, rev1)
	}
	if _, err := q.GetWatermark(ctx, quality.GetWatermarkParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	}); err == nil {
		t.Fatal("failed run moved the watermark")
	}
}

func testUUIDString(t *testing.T, id pgtype.UUID) string {
	t.Helper()
	if !id.Valid {
		t.Fatal("invalid UUID")
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func TestDiscoveryAnnualMembersAreSeparateAndReplayStable(t *testing.T) {
	p := freshJobsDB(t)
	ctx := context.Background()
	hash := strings.Repeat("a", 64)
	maker := &jobsIDMaker{}
	in := application.Input{Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2025-01", Resources: []application.Resource{{URL: "https://example.invalid/pmqc_2025.zip", SHA256: hash, Bytes: 10, Member: "2025/PMQC_2025_01.csv", MemberSHA256: strings.Repeat("b", 64), MemberBytes: 20}}, Counts: domain.Counts{}, ManifestHash: strings.Repeat("c", 64)}
	if _, e := adapters.NewPublisher(p).Publish(ctx, in, maker.next); e != nil {
		t.Fatal(e)
	}
	var got []ImportPayload
	var keys []string
	d := Discovery{Pool: p, Source: "pmqc", Stream: "pmqc-assay-v1", List: func(context.Context) ([]application.InventoryEntry, error) {
		return []application.InventoryEntry{{Label: "2025-01", URL: in.Resources[0].URL, SHA256: hash, Member: "2025/PMQC_2025_01.csv"}, {Label: "2025-02", URL: in.Resources[0].URL, SHA256: hash, Member: "2025/PMQC_2025_02.csv"}}, nil
	}, Enqueue: func(ctx context.Context, payload ImportPayload, key string) (platformjobs.EnqueueResult, error) {
		got = append(got, payload)
		keys = append(keys, key)
		return platformjobs.EnqueueResult{ID: "job", Created: true}, nil
	}}
	if e := d.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if len(got) != 1 || got[0].Edition != "2025-02" || got[0].Member != "2025/PMQC_2025_02.csv" || !strings.Contains(keys[0], got[0].Member) {
		t.Fatal("monthly member aliased or lost", got, keys)
	}
}
