//go:build integration

package enterpriseexport_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	enterpriseexport "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshExportPool(t *testing.T) *pgxpool.Pool {
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
	name := fmt.Sprintf("enterprise_export_test_%d", time.Now().UnixNano())
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
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedWeek(t *testing.T, pool *pgxpool.Pool, stationID, checksum string, start, end time.Time, amounts ...int64) string {
	t.Helper()
	ctx := context.Background()
	im := parentofficial.NewImporter(pool)
	run, err := im.BeginRun(ctx, domain.ImportKey{
		SourceURL:      "https://www.gov.br/anp/" + checksum + ".xlsx",
		SourceChecksum: checksum, ParserVersion: "esp03-it",
	}, start, end)
	if err != nil || run.NoOp {
		t.Fatalf("begin %s = %+v, %v", checksum, run, err)
	}
	products := []string{"GASOLINE_REGULAR", "ETHANOL"}
	rows := make([]domain.PriceRow, 0, len(amounts))
	for i, amount := range amounts {
		rows = append(rows, domain.PriceRow{
			StationID: stationID, Product: products[i%len(products)], Unit: "L",
			AmountMilli: amount, RawText: "5,999",
			CollectedOn: start.AddDate(0, 0, 2), SourceRow: i + 8,
		})
	}
	var tally domain.QuarantineTally
	if _, err := im.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
		t.Fatal(err)
	}
	fin, err := im.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
	if err != nil || !fin.Published {
		t.Fatalf("finish %s = %+v, %v", checksum, fin, err)
	}
	return run.RevisionID
}

func buildInTx(t *testing.T, tx pgx.Tx, output string) enterpriseexport.Manifest {
	t.Helper()
	var snapshot string
	if err := tx.QueryRow(context.Background(), "SELECT pg_current_snapshot()::text").Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	builder := enterpriseexport.Builder{
		Stations: directoryread.NewExportReader(tx),
		Prices:   officialread.NewExportReader(tx),
		PageSize: 2,
		Now:      func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) },
	}
	manifest, err := builder.Build(context.Background(), enterpriseexport.Options{
		FromSurvey: "2025-01-01", ToSurvey: "2025-12-31",
		OutputDir: output, DatabaseSnapshot: snapshot,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return manifest
}

func exportedAmounts(t *testing.T, dir string) map[int64]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "prices.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]bool{}
	for _, line := range splitLines(raw) {
		var row struct {
			AmountMilliBRL int64 `json:"amount_milli_brl"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		out[row.AmountMilliBRL] = true
	}
	return out
}

func splitLines(raw []byte) []string {
	lines := []string{}
	start := 0
	for i, b := range raw {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(raw[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(raw) {
		lines = append(lines, string(raw[start:]))
	}
	return lines
}

func TestBuildUsesOneConsistentSnapshotAcrossCorrection(t *testing.T) {
	pool := freshExportPool(t)
	ctx := context.Background()
	station, err := parentdirectory.NewRepository(pool).ResolveCNPJ(ctx, "04218406000104", "Posto A", nil)
	if err != nil {
		t.Fatal(err)
	}
	week1Start := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	week1End := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)
	week2Start := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
	week2End := time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC)
	week1 := seedWeek(t, pool, station.ID, "w1a", week1Start, week1End, 5999, 3799)
	seedWeek(t, pool, station.ID, "w2", week2Start, week2End, 6199, 3899)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Establish the snapshot before the correction publishes.
	var snapshot string
	if err := tx.QueryRow(ctx, "SELECT pg_current_snapshot()::text").Scan(&snapshot); err != nil {
		t.Fatal(err)
	}

	correction := seedWeek(t, pool, station.ID, "w1b", week1Start, week1End, 5799, 3699)
	if correction == week1 {
		t.Fatal("correction reused the original revision")
	}

	oldDir := filepath.Join(t.TempDir(), "old")
	oldManifest := buildInTx(t, tx, oldDir)
	if oldManifest.Sources.Official[0].RevisionID != week1 {
		t.Fatalf("pinned revision = %s, want pre-correction %s",
			oldManifest.Sources.Official[0].RevisionID, week1)
	}
	oldAmounts := exportedAmounts(t, oldDir)
	if !oldAmounts[5999] || !oldAmounts[3799] || oldAmounts[5799] || oldAmounts[3699] {
		t.Errorf("old snapshot amounts = %v", oldAmounts)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	tx2, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()
	newDir := filepath.Join(t.TempDir(), "new")
	newManifest := buildInTx(t, tx2, newDir)
	if newManifest.Sources.Official[0].RevisionID != correction {
		t.Fatalf("pinned revision = %s, want correction %s",
			newManifest.Sources.Official[0].RevisionID, correction)
	}
	newAmounts := exportedAmounts(t, newDir)
	if !newAmounts[5799] || !newAmounts[3699] || newAmounts[5999] || newAmounts[3799] {
		t.Errorf("new snapshot amounts = %v", newAmounts)
	}
}
