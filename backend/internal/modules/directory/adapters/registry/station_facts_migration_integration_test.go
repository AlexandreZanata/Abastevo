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
	for _, tc := range []struct{ version, file, table, column string }{
		{"000146", "000146_station_source_facts.sql", "directory_stations", "legal_name"},
		{"000147", "000147_station_fact_projection_clock.sql", "directory_stations", "facts_source_clock"},
		{"000148", "000148_station_fact_reprojection_campaigns.sql", "directory_station_fact_reprojection_campaigns", "campaign_id"},
		{"000149", "000149_station_address_provenance.sql", "registry_assertions", "address_provenance"},
	} {
		t.Run(tc.version, func(t *testing.T) { testStationFactsUpgrade(t, tc.version, tc.file, tc.table, tc.column) })
	}
}

func testStationFactsUpgrade(t *testing.T, version, file, table, column string) {
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
	var legacyAssertion string
	if version == "000149" {
		runID := newUUID()
		legacyAssertion = newUUID()
		if _, err := pool.Exec(ctx, "INSERT INTO registry_source_runs(id,source,snapshot_identity,checksum,parser_version) VALUES($1::uuid,'registry-csv','[P25-TEST] legacy snapshot',$2,'synthetic')", runID, vectorChecksum("legacy-run")); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO registry_assertions(id,run_id,source,source_key,checksum,display_name,address,address_source) VALUES($1::uuid,$2::uuid,'registry-csv','91000000000066',$3,'[P25-TEST] Legacy assertion','{\"street\":\"[P25-TEST] Historical\"}','anp')", legacyAssertion, runID, vectorChecksum("legacy-assertion")); err != nil {
			t.Fatal(err)
		}
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
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_name=$1 AND column_name=$2", table, column).Scan(&columns); err != nil || columns != 0 {
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
	if legacyAssertion != "" {
		var oldStreet string
		var absent bool
		if err := pool.QueryRow(ctx, "SELECT address->>'street',address_provenance IS NULL FROM registry_assertions WHERE id=$1::uuid", legacyAssertion).Scan(&oldStreet, &absent); err != nil || oldStreet != "[P25-TEST] Historical" || !absent {
			t.Fatal("legacy assertion changed", oldStreet, absent, err)
		}
	}
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil || len(applied) != 0 {
		t.Fatal("upgrade replay", applied, err)
	}
}
