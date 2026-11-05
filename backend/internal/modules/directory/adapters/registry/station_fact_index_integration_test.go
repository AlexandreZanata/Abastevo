//go:build integration

package registry

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStationFactLookupIndexFailureRecoveryAndSelectivePlan(t *testing.T) {
	const file = "000156_registry_station_fact_lookup.sql"
	files := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") && entry.Name() < file {
			raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name()] = &fstest.MapFile{Data: raw}
		}
	}
	_, dsn := freshStoreWithMigrations(t, files)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	station, run := newUUID(), newUUID()
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1::uuid,'[P25-TEST] Legacy')", station); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO registry_source_runs(id,source,snapshot_identity,checksum,parser_version) VALUES($1::uuid,'registry-csv','[P25-TEST] index','synthetic','synthetic')", run); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO registry_assertions(id,run_id,source,source_key,checksum,station_id,display_name)
 SELECT md5('synthetic-index-'||n)::uuid,$1::uuid,'registry-csv','synthetic-'||n,'synthetic',
 CASE WHEN n=1 THEN $2::uuid ELSE NULL END,'[P25-TEST] Historical' FROM generate_series(1,6000) n`, run, station); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, file)
	if err != nil {
		t.Fatal(err)
	}
	files[file] = &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;")...)}
	if _, err := migrate.Apply(ctx, dsn, files); err == nil {
		t.Fatal("failed migration committed")
	}
	var absent bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('registry_assertions_station_idx') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("partial index survived", err)
	}
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE registry_assertions"); err != nil {
		t.Fatal(err)
	}
	var plan string
	if err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) SELECT id FROM registry_assertions WHERE station_id=$1::uuid", station).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "registry_assertions_station_idx") {
		t.Fatal("selective ownership lookup scans unrelated assertions", plan)
	}
	var count int
	var name string
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM registry_assertions").Scan(&count); err != nil || count != 6000 {
		t.Fatal("assertions changed", count, err)
	}
	if err := pool.QueryRow(ctx, "SELECT display_name FROM directory_stations WHERE id=$1::uuid", station).Scan(&name); err != nil || name != "[P25-TEST] Legacy" {
		t.Fatal("legacy name changed", err)
	}
	if applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil || len(applied) != 0 {
		t.Fatal("index migration replay", applied, err)
	}
}
