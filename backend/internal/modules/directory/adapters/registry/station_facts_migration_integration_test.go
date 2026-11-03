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

func TestStationFactsMigrationFailureRecoveryPreservesLegacy(t *testing.T) {
	for _, tc := range []struct{ version, file, column string }{
		{"000146", "000146_station_source_facts.sql", "legal_name"},
		{"000147", "000147_station_fact_projection_clock.sql", "facts_source_clock"},
	} {
		t.Run(tc.version, func(t *testing.T) { testStationFactsUpgrade(t, tc.version, tc.file, tc.column) })
	}
}

func testStationFactsUpgrade(t *testing.T, version, file, column string) {
	files := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") && entry.Name() < version {
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
	id := newUUID()
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name,address) VALUES($1::uuid,'[P25-TEST] Legacy','{\"street\":\"SYNTHETIC\"}')", id); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, file)
	if err != nil {
		t.Fatal(err)
	}
	files[file] = &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;")...)}
	if _, err := migrate.Apply(ctx, dsn, files); err == nil {
		t.Fatal("failed upgrade committed")
	}
	var columns int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_name='directory_stations' AND column_name=$1", column).Scan(&columns); err != nil || columns != 0 {
		t.Fatal("partial migration", columns, err)
	}
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET facts_source_clock=now(),facts_source_checksum=NULL WHERE id=$1::uuid", id); err == nil {
		t.Fatal("partial projection clock accepted")
	}
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET facts_source_clock=NULL,facts_source_checksum=$2 WHERE id=$1::uuid", id, vectorChecksum("partial")); err == nil {
		t.Fatal("partial projection checksum accepted")
	}
	var name string
	var empty bool
	if err := pool.QueryRow(ctx, "SELECT display_name, legal_name IS NULL AND trade_name IS NULL AND address_source IS NULL AND facts_source_clock IS NULL FROM directory_stations WHERE id=$1::uuid", id).Scan(&name, &empty); err != nil || name != "[P25-TEST] Legacy" || !empty {
		t.Fatal(name, empty, err)
	}
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 0 {
		t.Fatal("upgrade replay", applied, err)
	}
}
