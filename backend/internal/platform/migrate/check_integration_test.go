//go:build integration

package migrate

import (
	"context"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func TestReadinessRejectsUnmigratedModifiedAndNewerSchema(t *testing.T) {
	ctx := context.Background()
	dsn := freshDB(t, testDSN(t))
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Check(ctx, pool, dbmigrations.Files); err == nil {
		t.Fatal("unmigrated database marked ready")
	}
	if _, err := Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := pool.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version='000001'").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum='modified' WHERE version='000001'"); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool, dbmigrations.Files); err == nil {
		t.Fatal("modified schema marked ready")
	}
	if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum=$1 WHERE version='000001'", original); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES ('999999','unknown')"); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool, dbmigrations.Files); err == nil {
		t.Fatal("newer schema marked ready")
	}
}
