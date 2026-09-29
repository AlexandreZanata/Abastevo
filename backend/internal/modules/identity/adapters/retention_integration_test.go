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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func retentionDSN(t *testing.T) string {
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

func retentionPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := retentionDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("identity_retention_test_%d", time.Now().UnixNano())
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
	return pool
}

func TestPurgeExpiredChallengesBoundaryAndBatch(t *testing.T) {
	pool := retentionPool(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	// Boundary: expires_at exactly now stays (strictly-expired only);
	// consumed challenges stay until expiry as well.
	if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges
		(id, nonce_hash, fingerprint, purpose, expires_at, consumed_at)
		VALUES (gen_random_uuid(), 'h', 'fp-edge', 'SIGN', $1, $1)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges
		(id, nonce_hash, fingerprint, purpose, expires_at)
		SELECT gen_random_uuid(), 'h', 'fp-' || g, 'SIGN', $1
		FROM generate_series(1, 1200) g`, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	r := NewRegistrar(pool)
	purged, oldest, err := r.PurgeExpiredChallenges(ctx, now, 500)
	if err != nil {
		t.Fatalf("purge = %v", err)
	}
	if purged != 1200 {
		t.Errorf("purged = %d, want 1200 across batches", purged)
	}
	if oldest.IsZero() || oldest.After(now) || oldest.Before(now.Add(-70*time.Minute)) {
		t.Errorf("oldest = %v, want within the expired hour", oldest)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM identity_challenges`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Errorf("remaining = %d, want the boundary row", left)
	}
}

func TestPurgeExpiredAttempts(t *testing.T) {
	pool := retentionPool(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	if _, err := pool.Exec(ctx, `INSERT INTO identity_contributors (id) VALUES ('c1111111-0000-4000-8000-000000000001')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_idempotency
		(contributor_id, method, route, key, request_hash, expires_at)
		VALUES ('c1111111-0000-4000-8000-000000000001', 'POST', '/v1/x', 'old', 'h', $1)`, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_idempotency
		(contributor_id, method, route, key, request_hash, expires_at)
		VALUES ('c1111111-0000-4000-8000-000000000001', 'POST', '/v1/x', 'fresh', 'h', $1)`, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(pool)
	purged, oldest, err := runner.PurgeExpiredAttempts(ctx, now, 500)
	if err != nil {
		t.Fatalf("purge = %v", err)
	}
	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}
	if oldest.IsZero() {
		t.Error("oldest overdue not reported")
	}
	var left string
	if err := pool.QueryRow(ctx, `SELECT key FROM identity_idempotency`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != "fresh" {
		t.Errorf("remaining = %q", left)
	}
}
