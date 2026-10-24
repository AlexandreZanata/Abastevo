//go:build integration

package read

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
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
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
	name := fmt.Sprintf("official_export_test_%d", time.Now().UnixNano())
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

func TestExportRevisionsAndPricesExcludeNonCurrent(t *testing.T) {
	pool := freshExportPool(t)
	ctx := context.Background()
	station, err := parentdirectory.NewRepository(pool).ResolveCNPJ(ctx, "04218406000104", "Posto A", nil)
	if err != nil {
		t.Fatal(err)
	}
	im := parentofficial.NewImporter(pool)
	week1Start := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	week1End := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)
	week2Start := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
	week2End := time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC)
	row := func(n int, day, product string, milli int64) domain.PriceRow {
		collected, err := time.Parse("2006-01-02", day)
		if err != nil {
			t.Fatal(err)
		}
		return domain.PriceRow{
			StationID: station.ID, Product: product, Unit: "L",
			AmountMilli: milli, RawText: "5,999", CollectedOn: collected, SourceRow: n,
		}
	}
	publish := func(checksum string, start, end time.Time, rows []domain.PriceRow) string {
		t.Helper()
		run, err := im.BeginRun(ctx, domain.ImportKey{
			SourceURL:      "https://www.gov.br/anp/" + checksum + ".xlsx",
			SourceChecksum: checksum, ParserVersion: "esp03-test",
		}, start, end)
		if err != nil || run.NoOp {
			t.Fatalf("begin %s = %+v, %v", checksum, run, err)
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
	week1 := publish("w1a", week1Start, week1End, []domain.PriceRow{
		row(8, "2025-01-08", "GASOLINE_REGULAR", 5999),
		row(9, "2025-01-08", "ETHANOL", 3799),
	})
	week2 := publish("w2", week2Start, week2End, []domain.PriceRow{
		row(8, "2025-01-15", "GASOLINE_REGULAR", 6199),
		row(9, "2025-01-15", "ETHANOL", 3899),
	})
	week1Corrected := publish("w1b", week1Start, week1End, []domain.PriceRow{
		row(8, "2025-01-08", "GASOLINE_REGULAR", 5799),
		row(9, "2025-01-08", "ETHANOL", 3699),
	})
	staging, err := im.BeginRun(ctx, domain.ImportKey{
		SourceURL:      "https://www.gov.br/anp/staging.xlsx",
		SourceChecksum: "sha256:staging", ParserVersion: "esp03-test",
	}, time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var stagedTally domain.QuarantineTally
	if _, err := im.StageBatch(ctx, staging.RevisionID, []domain.PriceRow{row(8, "2025-01-22", "GASOLINE_REGULAR", 6299)}, &stagedTally); err != nil {
		t.Fatal(err)
	}

	reader := NewExportReader(pool)
	revisions, next, err := reader.ExportRevisions(ctx, "2025-01-01", "2025-12-31", "", 10)
	if err != nil || len(revisions) != 2 || next != "" {
		t.Fatalf("revisions = %+v, next=%q, err=%v", revisions, next, err)
	}
	byWeek := map[string]struct {
		id        string
		supersede string
	}{}
	for _, revision := range revisions {
		byWeek[revision.SurveyStart] = struct {
			id        string
			supersede string
		}{revision.ID, revision.SupersedesRevisionID}
	}
	if got := byWeek["2025-01-06"].id; got != week1Corrected {
		t.Errorf("week1 current = %q, want correction %q", got, week1Corrected)
	}
	if got := byWeek["2025-01-06"].supersede; got != week1 {
		t.Errorf("week1 supersedes = %q, want original %q", got, week1)
	}
	if got := byWeek["2025-01-13"].id; got != week2 {
		t.Errorf("week2 current = %q, want %q", got, week2)
	}
	if got := byWeek["2025-01-13"].supersede; got != "" {
		t.Errorf("week2 supersedes = %q, want none", got)
	}

	first, next, err := reader.ExportPrices(ctx, week1Corrected, "", 1)
	if err != nil || len(first) != 1 || next == "" {
		t.Fatalf("correction page one = %+v, next=%q, err=%v", first, next, err)
	}
	second, next2, err := reader.ExportPrices(ctx, week1Corrected, next, 1)
	if err != nil || len(second) != 1 || next2 != "" {
		t.Fatalf("correction page two = %+v, next=%q, err=%v", second, next2, err)
	}
	for _, price := range append(first, second...) {
		if price.AmountMilliBRL != 5799 && price.AmountMilliBRL != 3699 {
			t.Errorf("correction price = %+v", price)
		}
	}
	if old, _, err := reader.ExportPrices(ctx, week1, "", 10); err != nil || len(old) != 0 {
		t.Errorf("superseded revision exported: %+v, %v", old, err)
	}
	if staged, _, err := reader.ExportPrices(ctx, staging.RevisionID, "", 10); err != nil || len(staged) != 0 {
		t.Errorf("staging revision exported: %+v, %v", staged, err)
	}
	if none, _, err := reader.ExportRevisions(ctx, "2024-01-01", "2024-12-31", "", 10); err != nil || len(none) != 0 {
		t.Errorf("unknown range = %+v, %v", none, err)
	}
}
