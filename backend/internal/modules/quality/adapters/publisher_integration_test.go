//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("quality_test_%d", time.Now().UnixNano())
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
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	applied, err := migrate.Apply(ctx, u.String(), dbmigrations.Files)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	found := false
	for _, v := range applied {
		if v == "000093" {
			found = true
		}
	}
	if !found {
		t.Fatalf("quality assay ledger migration missing from applied %d versions", len(applied))
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type idMaker struct {
	n atomic.Int64
}

func (m *idMaker) next() (string, error) {
	return fmt.Sprintf("aaaaaaaa-1111-4111-8111-%012d", m.n.Add(1)), nil
}

func sampleInput(hashChar string) application.Input {
	return application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pmqc_2026_09.zip",
			SHA256: strings.Repeat("a", 64), Bytes: 412001,
		}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat(hashChar, 64),
		Assays: []application.AssayInput{
			{SampleRef: "S-1", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "GASOLINE_REGULAR", ResultText: "Límpido", Unit: "-", Conformity: "CONFORMING", EvidenceRef: "f#S-1#ASPECTO", Fingerprint: strings.Repeat("c", 64)},
			{SampleRef: "S-1", AssayRef: "DESTILACAO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "GASOLINE_REGULAR", ResultText: "181,5", Unit: "°C", Conformity: "CONFORMING", EvidenceRef: "f#S-1#DESTILACAO", Fingerprint: strings.Repeat("d", 64)},
		},
		Links: []application.LinkInput{
			{RecordKey: "pmqc-assay-v1:S-1", Reason: "exact cnpj", Active: true},
		},
	}
}

func revisionCount(t *testing.T, pool *pgxpool.Pool, editionID string) int {
	t.Helper()
	var n int
	eid, err := mustUUID(editionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM quality_revisions WHERE edition_id = $1", eid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPublishAndLease(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	stationID := "bbbbbbbb-2222-4222-8222-000000000001"
	if _, err := pool.Exec(ctx,
		"INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Teste')",
		stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	in := sampleInput("b")
	in.Links = []application.LinkInput{{
		RecordKey: "pmqc-assay-v1:S-1", StationID: stationID,
		Automatic: true, Reason: "exact cnpj", Active: true,
	}}
	rev, err := pub.Publish(ctx, in, maker.next)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	q := quality.New(pool)
	current, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	})
	if err != nil {
		t.Fatalf("lease read: %v", err)
	}
	if got := uuidString(current); got != rev {
		t.Fatalf("current pointer = %s, want published %s", got, rev)
	}
	revID, err := mustUUID(rev)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.ListRevisionAssays(ctx, revID)
	if err != nil {
		t.Fatalf("list assays: %v", err)
	}
	if len(rows) != 2 || rows[0].SampleRef != "S-1" || rows[0].AssayRef != "ASPECTO" {
		t.Fatalf("published rows = %+v, want 2 ordered assays", rows)
	}
	if rows[1].ResultText != "181,5" || rows[1].Unit != "°C" {
		t.Errorf("native text not verbatim: %+v", rows[1])
	}
}

func TestReplayStable(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	first, err := pub.Publish(ctx, sampleInput("b"), maker.next)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	second, err := pub.Publish(ctx, sampleInput("b"), maker.next)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first != second {
		t.Fatalf("replay returned %s, want stable %s", second, first)
	}
	revID, err := mustUUID(first)
	if err != nil {
		t.Fatal(err)
	}
	var staged int64
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM quality_assays WHERE revision_id = $1", revID).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if staged != 2 {
		t.Fatalf("replay duplicated rows: %d", staged)
	}
}

func TestConcurrentReplayConverges(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	const racers = 8
	results := make([]string, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rev, err := pub.Publish(ctx, sampleInput("b"), maker.next)
			results[i], errs[i] = rev, err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("racer %d: %v", i, err)
		}
		if results[i] != results[0] {
			t.Fatalf("racer %d returned %s, want converged %s", i, results[i], results[0])
		}
	}
	edition, err := quality.New(pool).GetEditionByKey(ctx, quality.GetEditionByKeyParams{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := revisionCount(t, pool, uuidString(edition.ID)); n != 1 {
		t.Fatalf("concurrent publish created %d revisions, want 1", n)
	}
}

func TestPartialFailureRollback(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	// Preparation-shaped but storage-refused fuel: the CHECK must fail the
	// whole transaction, leaving no edition, manifest or revision behind.
	bad := sampleInput("b")
	bad.Assays[0].Fuel = "HYDROGEN"
	if _, err := pub.Publish(ctx, bad, maker.next); err == nil {
		t.Fatal("refused fuel published")
	}
	if _, err := quality.New(pool).GetEditionByKey(ctx, quality.GetEditionByKeyParams{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
	}); err == nil {
		t.Fatal("failed publication left a visible edition")
	}
	// Validation-level refusal persists nothing either.
	mismatched := sampleInput("b")
	mismatched.Counts.Input = 9
	if _, err := pub.Publish(ctx, mismatched, maker.next); err == nil {
		t.Fatal("mismatched counts published")
	}
}

func TestCorrectionSupersedes(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	rev1, err := pub.Publish(ctx, sampleInput("b"), maker.next)
	if err != nil {
		t.Fatalf("publish rev1: %v", err)
	}
	corrected := sampleInput("e")
	corrected.Assays[1].ResultText = "182,0"
	corrected.Assays[1].Fingerprint = strings.Repeat("f", 64)
	rev2, err := pub.Publish(ctx, corrected, maker.next)
	if err != nil {
		t.Fatalf("publish correction: %v", err)
	}
	if rev1 == rev2 {
		t.Fatal("correction reused the revision id")
	}
	q := quality.New(pool)
	rev1ID, _ := mustUUID(rev1)
	got1, err := q.GetRevision(ctx, rev1ID)
	if err != nil {
		t.Fatal(err)
	}
	if got1.Status != "superseded" {
		t.Errorf("rev1 status = %s, want superseded", got1.Status)
	}
	current, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if uuidString(current) != rev2 {
		t.Errorf("current pointer did not move to the correction")
	}
	// History is retained: the superseded revision still serves its rows.
	rows, err := q.ListRevisionAssays(ctx, rev1ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("superseded rows lost: %+v %v", rows, err)
	}
	rev2ID, _ := mustUUID(rev2)
	rows2, err := q.ListRevisionAssays(ctx, rev2ID)
	if err != nil || len(rows2) != 2 || rows2[1].ResultText != "182,0" {
		t.Fatalf("correction rows = %+v %v", rows2, err)
	}
}

func TestStagingInvisibleToLease(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	q := quality.New(pool)

	editionID, _ := mustUUID("aaaaaaaa-1111-4111-8111-000000000001")
	revisionID, _ := mustUUID("aaaaaaaa-1111-4111-8111-000000000002")
	if _, err := q.CreateEdition(ctx, quality.CreateEditionParams{
		ID: editionID, Source: "pmqc", Stream: "pmqc-assay-v1",
		Edition: "2026-08", Predecessor: pgText(""),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateRevision(ctx, quality.CreateRevisionParams{
		ID: revisionID, EditionID: editionID, SupersedesRevisionID: nullUUID(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	}); err == nil {
		t.Fatal("staging revision visible through the lease")
	}
}

func TestMigrationRecovery(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	applied, err := migrate.Apply(ctx, dsnOf(t, pool), dbmigrations.Files)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("re-apply migrated %v, want no-op", applied)
	}
	var exists bool
	if err := pool.QueryRow(ctx,
		"SELECT to_regclass('public.quality_editions') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatalf("quality_editions missing: %v", exists)
	}
}

func dsnOf(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	var name string
	if err := pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}
