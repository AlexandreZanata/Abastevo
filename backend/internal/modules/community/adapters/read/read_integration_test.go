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

func freshReader(t *testing.T) (*Reader, *pgxpool.Pool, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("read_test_%d", time.Now().UnixNano())
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
	stationID := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto T')`, stationID); err != nil {
		t.Fatalf("station: %v", err)
	}
	return NewReader(pool), pool, stationID
}

func seedProjection(t *testing.T, pool *pgxpool.Pool, stationID string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO community_observations
		(id, contributor_ref, client_submission_id, station_id, fuel_product,
		 unit, amount_milli_brl, condition_kind, qualifier_key, received_at,
		 policy_version)
		VALUES ('d6c74c23-63db-4c24-a2e5-408cb23bad27', 'tok-c1', 'sub-1', $1,
		 'GASOLINE_REGULAR', 'L', 5999, 'STANDARD', 'STANDARD', now(),
		 'community-pricing-v1')`, stationID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO community_current_prices
		(station_id, fuel_product, unit, condition_kind, qualifier_key,
		 amount_milli_brl, availability, confidence,
		 representative_observation_id, independent_supporters,
		 confirmation_count, anchor_received_at, expires_at,
		 next_recompute_at, computed_at, projection_version,
		 algorithm_version, policy_config_version)
		VALUES ($1, 'GASOLINE_REGULAR', 'L', 'STANDARD', 'STANDARD',
		 5999, 'AVAILABLE', 'LOW',
		 'd6c74c23-63db-4c24-a2e5-408cb23bad27', 1,
		 0, now() - interval '1 hour', now() + interval '47 hours',
		 now() + interval '5 hours', now(), 4,
		 'consensus-v1', 'consensus-v1')`, stationID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCurrentPriceServesIndexedRow(t *testing.T) {
	r, pool, stationID := freshReader(t)
	ctx := context.Background()
	seedProjection(t, pool, stationID)
	view, err := r.CurrentPrice(ctx, stationID, "GASOLINE_REGULAR", "L", "STANDARD", "STANDARD")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Found || view.Availability != "AVAILABLE" || !view.HasAmount || view.AmountMilliBrl != 5999 {
		t.Errorf("view = %+v", view)
	}
	if view.Freshness != "FRESH" || view.ProjectionVersion != 4 {
		t.Errorf("view = %+v", view)
	}
	// Missing keys report absence without error: callers render null.
	absent, err := r.CurrentPrice(ctx, stationID, "ETHANOL", "L", "STANDARD", "STANDARD")
	if err != nil || absent.Found {
		t.Errorf("absent = %+v, %v", absent, err)
	}
	if _, err := r.CurrentPrice(ctx, "not-a-uuid", "GASOLINE_REGULAR", "L", "STANDARD", "STANDARD"); err == nil {
		t.Error("malformed station accepted")
	}
}
