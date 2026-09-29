//go:build integration

package jobs

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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/geocoder"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
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

func TestGeocodeBatchRecordsRevisions(t *testing.T) {
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("geojobs_test_%d", time.Now().UnixNano())
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
	st, err := repo.ResolveCNPJ(ctx, "04218406000104", "Posto G", nil)
	if err != nil {
		t.Fatalf("station: %v", err)
	}
	provider := &geocoder.FixtureProvider{ByStation: map[string]geocoder.Candidate{
		st.ID: {PointWKT: "POINT(-46.6 -23.5)", MatchInfo: "city"},
	}}
	svc := &geocoder.Service{Provider: provider, Store: repo, Cache: &geocoder.Cache{}}
	_ = svc
	h := Geocode{Pool: pool, Service: &geocoder.Service{
		Provider: provider, Store: repo,
	}, Batch: 25}
	if err := h.Handle(ctx, jobs.Job{}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if provider.CallCount() != 1 {
		t.Errorf("provider calls = %d, want 1", provider.CallCount())
	}
	var quality, providerName string
	if err := pool.QueryRow(ctx, `SELECT quality, provider FROM directory_location_revisions`).Scan(&quality, &providerName); err != nil {
		t.Fatalf("revision: %v", err)
	}
	if quality != "city-centroid" || providerName != "fixture" {
		t.Errorf("revision = %q/%q", quality, providerName)
	}
	// Second run finds nothing missing and records nothing new.
	if err := h.Handle(ctx, jobs.Job{}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if provider.CallCount() != 1 {
		t.Errorf("provider calls = %d after no-op run", provider.CallCount())
	}
}
