//go:build integration

package geocoder

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
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
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

func freshStation(t *testing.T) (*parent.Repository, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("geocoder_test_%d", time.Now().UnixNano())
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
	repo := parent.NewRepository(pool)
	station, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto GEO", map[string]string{"municipio": "SAO PAULO"})
	if err != nil {
		t.Fatalf("station: %v", err)
	}
	return repo, station.ID
}

func TestResolvePersistsAttributionWithoutProjecting(t *testing.T) {
	repo, stationID := freshStation(t)
	provider := &FixtureProvider{ByStation: map[string]Candidate{
		stationID: {PointWKT: "POINT(-46.633 -23.550)", MatchInfo: "rooftop"},
	}}
	svc := &Service{
		Provider: provider,
		Store:    repo,
		Limiter:  &Limiter{},
		Cache:    &Cache{TTL: time.Hour},
	}
	ctx := context.Background()
	rev, err := svc.Resolve(ctx, Query{StationID: stationID, Address: "AV PAULISTA 1000"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rev.Quality != domain.QualityCityCentroid {
		t.Errorf("provider quality = %q, want city-centroid", rev.Quality)
	}
	if rev.Provider != "fixture" || rev.SourceReference != "rooftop" {
		t.Errorf("attribution missing: %+v", rev)
	}
	// The provider answer must not project: manual review does that.
	st, err := repo.Station(ctx, rev.StationID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if st.CurrentPointWKT != "" {
		t.Errorf("provider answer projected: %q", st.CurrentPointWKT)
	}
	// Cached second resolve writes no new revision.
	if _, err := svc.Resolve(ctx, Query{StationID: stationID, Address: "AV PAULISTA 1000"}); err != nil {
		t.Fatalf("cached: %v", err)
	}
	revs, err := repo.Revisions(ctx, rev.StationID)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions = %d, %v; want exactly 1", len(revs), err)
	}
	if provider.CallCount() != 1 {
		t.Errorf("provider calls = %d, want 1", provider.CallCount())
	}
	// Manual review promotes the recorded revision to the projection.
	reviewed, err := repo.RecordLocation(ctx, domain.LocationRevision{
		StationID: rev.StationID, PointWKT: "POINT(-46.633 -23.550)",
		Quality: domain.QualityReviewed, Provider: "manual-review",
		SupersedesID: rev.ID,
	})
	if err != nil {
		t.Fatalf("review record: %v", err)
	}
	projected, err := repo.ProjectLocation(ctx, rev.StationID, reviewed.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if projected.CurrentQuality != domain.QualityReviewed || projected.CurrentPointWKT == "" {
		t.Errorf("projection = %+v", projected)
	}
}
