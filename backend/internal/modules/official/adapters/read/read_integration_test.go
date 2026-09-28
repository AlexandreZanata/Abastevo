//go:build integration

package read

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
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

func freshReader(t *testing.T) (*Reader, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("read_official_test_%d", time.Now().UnixNano())
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
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	dirRepo := parentdirectory.NewRepository(pool)
	st, err := dirRepo.ResolveCNPJ(ctx, "04218406000104", "Posto A", nil)
	if err != nil {
		t.Fatalf("station: %v", err)
	}
	im := parentofficial.NewImporter(pool)
	weekStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	weekEnd := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)
	mkrow := func(n int, milli int64, collected string) domain.PriceRow {
		day, err := time.Parse("2006-01-02", collected)
		if err != nil {
			t.Fatal(err)
		}
		return domain.PriceRow{StationID: st.ID, Product: "GASOLINE_REGULAR", Unit: "L",
			AmountMilli: milli, RawText: "5,999", CollectedOn: day, SourceRow: n}
	}
	publish := func(checksum string, rows []domain.PriceRow) {
		t.Helper()
		run, err := im.BeginRun(ctx, domain.ImportKey{
			SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: checksum, ParserVersion: "read-test",
		}, weekStart, weekEnd)
		if err != nil || run.NoOp {
			t.Fatalf("begin = %+v, %v", run, err)
		}
		var tally domain.QuarantineTally
		if _, err := im.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
			t.Fatalf("stage: %v", err)
		}
		fin, err := im.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
		if err != nil || !fin.Published {
			t.Fatalf("finish = %+v, %v", fin, err)
		}
	}
	publish("sha256:old", []domain.PriceRow{mkrow(8, 5999, "2025-01-08")})
	publish("sha256:new", []domain.PriceRow{mkrow(8, 6099, "2025-01-08")})
	return NewReader(pool), st.ID
}

func TestGroupsCurrentOnly(t *testing.T) {
	r, station := freshReader(t)
	ctx := context.Background()
	groups, err := r.Groups(ctx, station, "")
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	if groups[0].Official == nil || groups[0].Official.AmountMilli != 6099 {
		t.Errorf("group = %+v", groups[0])
	}
	if groups[0].Community != nil || groups[0].Condition.Kind != "STANDARD" {
		t.Errorf("group shape = %+v", groups[0])
	}
	filtered, err := r.Groups(ctx, station, "CNG")
	if err != nil || len(filtered) != 0 {
		t.Errorf("filtered = %+v, %v", filtered, err)
	}
}

func TestHistoryPortPagination(t *testing.T) {
	r, station := freshReader(t)
	ctx := context.Background()
	p1, next, err := r.History(ctx, officialapp.HistoryFilter{StationID: station, Limit: 1})
	if err != nil || len(p1) != 1 || next == "" {
		t.Fatalf("page one = %+v, next=%q, %v", p1, next, err)
	}
	date, id, ok := strings.Cut(next, ":")
	if !ok {
		t.Fatalf("opaque key malformed: %q", next)
	}
	p2, next2, err := r.History(ctx, officialapp.HistoryFilter{
		StationID: station, Limit: 1, AfterDate: date, AfterID: id, HasCursor: true,
	})
	if err != nil || len(p2) != 1 || next2 != "" {
		t.Fatalf("page two = %+v, next=%q, %v", p2, next2, err)
	}
	if p1[0].RevisionID == p2[0].RevisionID {
		t.Error("pagination did not cross revisions")
	}
	only, _, err := r.History(ctx, officialapp.HistoryFilter{StationID: station, Limit: 10, RevisionID: p2[0].RevisionID})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range only {
		if e.RevisionID != p2[0].RevisionID {
			t.Errorf("scope leaked: %+v", e)
		}
	}
}
