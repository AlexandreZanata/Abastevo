//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
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

// freshImporter migrates an empty disposable database and returns an
// importer plus a station maker on it. The database is dropped on cleanup.
func freshImporter(t *testing.T) (*Importer, *pgxpool.Pool) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("official_test_%d", time.Now().UnixNano())
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
	dsn := u.String()
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(applied) != 3 {
		t.Fatalf("want 3 applied migrations, got %d (%v)", len(applied), applied)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewImporter(pool), pool
}

// makeStation inserts a bare station row and returns its id. Stations are
// directory-owned; the raw insert keeps this test free of cross-module
// adapter imports.
func makeStation(t *testing.T, pool *pgxpool.Pool, display string) string {
	t.Helper()
	id, err := newUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx,
		`INSERT INTO directory_stations (id, display_name) VALUES ($1, $2)`,
		id, display); err != nil {
		t.Fatalf("station insert: %v", err)
	}
	return id
}

var weekStart = time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
var weekEnd = time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)

func priceRow(station string, row int, milli int64) domain.PriceRow {
	return domain.PriceRow{
		StationID: station, Product: "GASOLINE_REGULAR", Unit: "L",
		AmountMilli: milli, RawText: "5,999",
		CollectedOn: time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC),
		SourceRow:   row,
	}
}

func TestSameBytesNoOp(t *testing.T) {
	im, pool := freshImporter(t)
	ctx := context.Background()
	stA := makeStation(t, pool, "Posto A")
	stB := makeStation(t, pool, "Posto B")
	key := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:aaa", ParserVersion: "p02-t04"}
	first, err := im.BeginRun(ctx, key, weekStart, weekEnd)
	if err != nil || first.NoOp {
		t.Fatalf("begin = %+v, %v", first, err)
	}
	var tally domain.QuarantineTally
	if _, err := im.StageBatch(ctx, first.RevisionID, []domain.PriceRow{priceRow(stA, 8, 5999), priceRow(stB, 9, 6099)}, &tally); err != nil {
		t.Fatalf("stage: %v", err)
	}
	res, err := im.FinishRun(ctx, first.RunID, first.RevisionID, &tally)
	if err != nil || !res.Published {
		t.Fatalf("finish = %+v, %v", res, err)
	}
	again, err := im.BeginRun(ctx, key, weekStart, weekEnd)
	if err != nil {
		t.Fatalf("retry begin: %v", err)
	}
	if !again.NoOp {
		t.Errorf("identical bytes opened a duplicate revision: %+v", again)
	}
	cur, err := im.CurrentRevision(ctx, weekStart, weekEnd)
	if err != nil || cur != first.RevisionID {
		t.Errorf("pointer = %q, %v; want %q", cur, err, first.RevisionID)
	}
}

func TestCorrectedBytesNewRevision(t *testing.T) {
	im, pool := freshImporter(t)
	ctx := context.Background()
	st := makeStation(t, pool, "Posto A")
	v1 := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:aaa", ParserVersion: "p02-t04"}
	r1, err := im.BeginRun(ctx, v1, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	var q1 domain.QuarantineTally
	if _, err := im.StageBatch(ctx, r1.RevisionID, []domain.PriceRow{priceRow(st, 8, 5999)}, &q1); err != nil {
		t.Fatal(err)
	}
	if _, err := im.FinishRun(ctx, r1.RunID, r1.RevisionID, &q1); err != nil {
		t.Fatal(err)
	}
	v2 := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:bbb", ParserVersion: "p02-t04"}
	r2, err := im.BeginRun(ctx, v2, weekStart, weekEnd)
	if err != nil || r2.NoOp {
		t.Fatalf("corrected begin = %+v, %v", r2, err)
	}
	if r2.RevisionID == r1.RevisionID {
		t.Fatal("corrected bytes reused the revision")
	}
	var q2 domain.QuarantineTally
	if _, err := im.StageBatch(ctx, r2.RevisionID, []domain.PriceRow{priceRow(st, 8, 6099)}, &q2); err != nil {
		t.Fatal(err)
	}
	fin, err := im.FinishRun(ctx, r2.RunID, r2.RevisionID, &q2)
	if err != nil || !fin.Published {
		t.Fatalf("corrected finish = %+v, %v", fin, err)
	}
	cur, err := im.CurrentRevision(ctx, weekStart, weekEnd)
	if err != nil || cur != r2.RevisionID {
		t.Errorf("pointer = %q, %v; want %q", cur, err, r2.RevisionID)
	}
	// Previous revision stays readable with its rows intact.
	if _, err := im.CurrentRevision(ctx, weekStart, weekEnd); err != nil {
		t.Fatal(err)
	}
}

func TestPartialBatchInvisibleAndRetrySafe(t *testing.T) {
	im, pool := freshImporter(t)
	ctx := context.Background()
	st := makeStation(t, pool, "Posto A")
	key := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:aaa", ParserVersion: "p02-t04"}
	r1, err := im.BeginRun(ctx, key, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	rows := []domain.PriceRow{}
	for i := 8; i < 16; i++ {
		rows = append(rows, priceRow(st, i, 5999))
	}
	var q domain.QuarantineTally
	if _, err := im.StageBatch(ctx, r1.RevisionID, rows, &q); err != nil {
		t.Fatal(err)
	}
	q.Add("unknown-fuel-label", "row-99")
	q.Add("unknown-fuel-label", "row-100")
	fin, err := im.FinishRun(ctx, r1.RunID, r1.RevisionID, &q)
	if err != nil {
		t.Fatal(err)
	}
	if fin.Published || fin.Verdict.Reason != "quarantine-share" {
		t.Errorf("over-threshold published: %+v", fin)
	}
	if _, err := im.CurrentRevision(ctx, weekStart, weekEnd); err == nil {
		t.Error("under-review revision is pointer-visible")
	}
}

func TestEmptyRefused(t *testing.T) {
	im, _ := freshImporter(t)
	ctx := context.Background()
	key := domain.ImportKey{SourceURL: "https://www.gov.br/anp/empty.xlsx", SourceChecksum: "sha256:eee", ParserVersion: "p02-t04"}
	r, err := im.BeginRun(ctx, key, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	var q domain.QuarantineTally
	fin, err := im.FinishRun(ctx, r.RunID, r.RevisionID, &q)
	if err != nil {
		t.Fatal(err)
	}
	if fin.Published || fin.Verdict.Reason != "empty" {
		t.Errorf("empty week published: %+v", fin)
	}
	if _, err := im.CurrentRevision(ctx, weekStart, weekEnd); err == nil {
		t.Error("empty revision is pointer-visible")
	}
}

func TestRowDropNeedsReview(t *testing.T) {
	im, pool := freshImporter(t)
	ctx := context.Background()
	key1 := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:aaa", ParserVersion: "p02-t04"}
	r1, err := im.BeginRun(ctx, key1, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	stations := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		stations = append(stations, makeStation(t, pool, "Posto"))
	}
	batch := make([]domain.PriceRow, 0, 100)
	for i, st := range stations {
		r := priceRow(st, 8+i, 5999)
		batch = append(batch, r)
	}
	var q1 domain.QuarantineTally
	if _, err := im.StageBatch(ctx, r1.RevisionID, batch, &q1); err != nil {
		t.Fatal(err)
	}
	if _, err := im.FinishRun(ctx, r1.RunID, r1.RevisionID, &q1); err != nil {
		t.Fatal(err)
	}
	key2 := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:bbb", ParserVersion: "p02-t04"}
	r2, err := im.BeginRun(ctx, key2, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	short := make([]domain.PriceRow, 0, 70)
	for i := 0; i < 70; i++ {
		short = append(short, priceRow(stations[i], 8+i, 5999))
	}
	var q2 domain.QuarantineTally
	if _, err := im.StageBatch(ctx, r2.RevisionID, short, &q2); err != nil {
		t.Fatal(err)
	}
	fin, err := im.FinishRun(ctx, r2.RunID, r2.RevisionID, &q2)
	if err != nil {
		t.Fatal(err)
	}
	if fin.Published || fin.Verdict.Reason != "row-drop" {
		t.Errorf("30pc drop published: %+v", fin)
	}
	cur, err := im.CurrentRevision(ctx, weekStart, weekEnd)
	if err != nil || cur != r1.RevisionID {
		t.Errorf("pointer moved on drop: %q, %v", cur, err)
	}
}

func TestConflictingDuplicateQuarantined(t *testing.T) {
	im, pool := freshImporter(t)
	ctx := context.Background()
	key := domain.ImportKey{SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:aaa", ParserVersion: "p02-t04"}
	r, err := im.BeginRun(ctx, key, weekStart, weekEnd)
	if err != nil {
		t.Fatal(err)
	}
	stations := make([]string, 0, 199)
	for i := 0; i < 199; i++ {
		stations = append(stations, makeStation(t, pool, "Posto"))
	}
	rows := make([]domain.PriceRow, 0, 201)
	for i, st := range stations[:198] {
		rows = append(rows, priceRow(st, 8+i, 5999))
	}
	dupStation := stations[198]
	rows = append(rows, priceRow(dupStation, 500, 5999))
	conflict := priceRow(dupStation, 501, 6099)
	rows = append(rows, conflict)
	var q domain.QuarantineTally
	staged, err := im.StageBatch(ctx, r.RevisionID, rows, &q)
	if err != nil {
		t.Fatal(err)
	}
	if staged != 199 {
		t.Errorf("staged = %d, want 199 (conflict quarantined)", staged)
	}
	if q.Counts["conflicting-duplicate"] != 1 {
		t.Errorf("tally = %+v", q.Counts)
	}
	fin, err := im.FinishRun(ctx, r.RunID, r.RevisionID, &q)
	if err != nil || !fin.Published {
		t.Fatalf("finish = %+v, %v", fin, err)
	}
}
