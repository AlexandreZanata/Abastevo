//go:build integration

package migrate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
)

// testDSN points at the disposable Compose database. It fails the test when
// unreachable: integration must prove PostGIS, never skip it.
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

// freshDB creates an empty database and returns its DSN with Drop cleanup.
func freshDB(t *testing.T, adminDSN string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("migrate_test_%d", time.Now().UnixNano())
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
	// NOTE: pgx ConnConfig.ConnString() does not reflect a reassigned
	// Database field (verified empirically), so swap dbname at URL level.
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func postGISVersion(t *testing.T, dsn string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var version string
	if err := conn.QueryRow(ctx, "SELECT PostGIS_version()").Scan(&version); err != nil {
		t.Fatalf("PostGIS_version: %v", err)
	}
	return version
}

func TestApplyFromEmptyThenIdempotent(t *testing.T) {
	dsn := freshDB(t, testDSN(t))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	applied, err := Apply(ctx, dsn, dbmigrations.Files)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(applied) != 1 || applied[0] != "000001" {
		t.Fatalf("applied = %v, want [000001]", applied)
	}
	if v := postGISVersion(t, dsn); v == "" {
		t.Fatal("PostGIS_version() empty after apply")
	}

	again, err := Apply(ctx, dsn, dbmigrations.Files)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second Apply applied %v, want no-op", again)
	}
}

func TestChecksumMutationFails(t *testing.T) {
	dsn := freshDB(t, testDSN(t))
	dir := t.TempDir()
	file := filepath.Join(dir, "000001_enable_postgis.sql")
	if err := os.WriteFile(file, []byte("CREATE EXTENSION IF NOT EXISTS postgis;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := Apply(ctx, dsn, os.DirFS(dir)); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	// Mutate the applied file: same version, different bytes.
	if err := os.WriteFile(file, []byte("CREATE EXTENSION IF NOT EXISTS postgis;\n-- drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, dsn, os.DirFS(dir)); err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	} else if got := err.Error(); !strings.Contains(got, "000001") || !strings.Contains(got, "checksum") {
		t.Fatalf("error %q must name version and checksum", got)
	}
	// Unknown stray SQL files fail fast instead of silently skipping.
	if err := os.WriteFile(filepath.Join(dir, "oops.sql"), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, dsn, os.DirFS(dir)); err == nil {
		t.Fatal("expected error for stray SQL file, got nil")
	}
}

func TestConcurrentRunnersApplyOnce(t *testing.T) {
	dsn := freshDB(t, testDSN(t))
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = Apply(ctx, dsn, dbmigrations.Files)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("runner %d: %v", i, err)
		}
	}
	applied, err := appliedVersions(ctx, dsn)
	if err != nil {
		t.Fatalf("ledger read: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("ledger has %d rows, want exactly one apply per version", len(applied))
	}
}

func appliedVersions(ctx context.Context, dsn string) ([]string, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
