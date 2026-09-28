//go:build integration

package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
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

// freshRepo migrates an empty disposable database and returns a repository
// on it. The database is dropped during cleanup.
func freshRepo(t *testing.T) *Repository {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("directory_test_%d", time.Now().UnixNano())
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
	if len(applied) != 2 {
		t.Fatalf("want 2 applied migrations, got %d (%v)", len(applied), applied)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewRepository(pool)
}

func TestResolveStableUUID(t *testing.T) {
	repo := freshRepo(t)
	ctx := context.Background()
	addr := map[string]string{"municipio": "SAO PAULO", "uf": "SP"}
	first, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto A", addr)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if first.ID == "" || first.Status != "active" {
		t.Errorf("unexpected station: %+v", first)
	}
	second, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto Renamed", addr)
	if err != nil {
		t.Fatalf("re-resolve: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("unstable identity: %s != %s", second.ID, first.ID)
	}
	if second.DisplayName != "Posto A" {
		t.Errorf("re-resolve overwrote display name: %q", second.DisplayName)
	}
	got, err := repo.Station(ctx, first.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != first.ID {
		t.Errorf("load mismatch: %+v", got)
	}
}

func TestConcurrentSameCNPJProducesOneStation(t *testing.T) {
	repo := freshRepo(t)
	ctx := context.Background()
	const workers = 16
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st, err := repo.ResolveCNPJ(ctx, "11222333000181", "Posto Race", nil)
			if err == nil {
				ids[i] = st.ID
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if ids[i] != ids[0] || ids[0] == "" {
			t.Fatalf("divergent identity: %v", ids)
		}
	}
	q := directory.New(repo.pool)
	n, err := q.CountStations(ctx)
	if err != nil || n != 1 {
		t.Fatalf("stations = %d, %v; want exactly 1", n, err)
	}
	m, err := q.CountActiveIdentifiers(ctx)
	if err != nil || m != 1 {
		t.Fatalf("active identifiers = %d, %v; want exactly 1", m, err)
	}
}

func TestAliasRetireKeepsHistory(t *testing.T) {
	repo := freshRepo(t)
	ctx := context.Background()
	st, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto A", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := repo.RetireIdentifier(ctx, "CNPJ", "04218406000104"); err != nil {
		t.Fatalf("retire: %v", err)
	}
	// A retired CNPJ resolves to a NEW station; the old row stays historied.
	other, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto B", nil)
	if err != nil {
		t.Fatalf("re-resolve after retire: %v", err)
	}
	if other.ID == st.ID {
		t.Error("retired identifier still resolves to the old station")
	}
	if err := repo.RetireIdentifier(ctx, "CNPJ", "no-such-value"); err == nil {
		t.Error("retiring an unknown identifier succeeded")
	}
}

func TestMissingLocationStaysExplicit(t *testing.T) {
	repo := freshRepo(t)
	ctx := context.Background()
	st, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto A", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rev, err := repo.RecordLocation(ctx, domain.LocationRevision{
		StationID: st.ID,
		Quality:   domain.QualityUnknown,
		Provider:  "none",
	})
	if err != nil {
		t.Fatalf("record unknown: %v", err)
	}
	if rev.ID == "" {
		t.Error("revision needs an id")
	}
	loaded, err := repo.Station(ctx, st.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.CurrentPointWKT != "" || loaded.CurrentQuality != "" {
		t.Errorf("missing location projected: %+v", loaded)
	}
	if _, err := repo.ProjectLocation(ctx, st.ID, rev.ID); err == nil {
		t.Error("unknown location projected")
	}
	centroid, err := repo.RecordLocation(ctx, domain.LocationRevision{
		StationID: st.ID, PointWKT: "POINT(-46.6 -23.5)",
		Quality: domain.QualityCityCentroid, Provider: "fixture",
		SupersedesID: rev.ID,
	})
	if err != nil {
		t.Fatalf("record centroid: %v", err)
	}
	if _, err := repo.ProjectLocation(ctx, st.ID, centroid.ID); err == nil {
		t.Error("city centroid projected as precise position")
	}
	reviewed, err := repo.RecordLocation(ctx, domain.LocationRevision{
		StationID: st.ID, PointWKT: "POINT(-46.633 -23.550)",
		Quality: domain.QualityReviewed, Provider: "review",
		SupersedesID: centroid.ID,
	})
	if err != nil {
		t.Fatalf("record reviewed: %v", err)
	}
	projected, err := repo.ProjectLocation(ctx, st.ID, reviewed.ID)
	if err != nil {
		t.Fatalf("project reviewed: %v", err)
	}
	if projected.CurrentQuality != domain.QualityReviewed {
		t.Errorf("projection quality = %q", projected.CurrentQuality)
	}
	if projected.CurrentPointWKT == "" || len(projected.CurrentPointWKT) < 10 {
		t.Errorf("projection point missing: %q", projected.CurrentPointWKT)
	}
	revs, err := repo.Revisions(ctx, st.ID)
	if err != nil || len(revs) != 3 {
		t.Fatalf("revisions = %d, %v; want 3 append-only rows", len(revs), err)
	}
}

func TestGeographyRoundtrip(t *testing.T) {
	repo := freshRepo(t)
	ctx := context.Background()
	st, err := repo.ResolveCNPJ(ctx, "12ABC34501DE35", "Posto GEO", nil)
	if err != nil {
		t.Fatalf("resolve alphanumeric: %v", err)
	}
	if _, err := repo.RecordLocation(ctx, domain.LocationRevision{
		StationID: st.ID, PointWKT: "POINT(-46.633 -23.550)",
		Quality: domain.QualityReviewed, Provider: "review",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
}
