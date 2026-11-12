//go:build integration

package adapters_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestEventMigrationRecoveryAndPreviousCompatibility(t *testing.T) {
	const name = "000168_org_event_deliveries.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	id := f.Organizations[0].RecordID
	if _, err := f.Admin.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic previous catalog row');
	`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO job_queue(id,kind,payload,dedupe_key,max_attempts) VALUES($1,'synthetic-previous','{"version":1}','previous',5)`, id); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_event_intents') IS NULL
		AND to_regclass('org_event_receipts') IS NULL AND to_regprocedure('org_event_append_only()') IS NULL
		AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000168')`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed migration left tables, function or ledger", err)
	}
	current := fstest.MapFS{}
	for filename, file := range previous {
		current[filename] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 1 || applied[0] != "000168" {
		t.Fatal("retry failed", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous schema chain incompatible", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatal("migration not idempotent", applied, err)
	}
	var label, kind string
	var jobs, intents, receipts int
	if err := f.Admin.QueryRow(ctx, `SELECT display_name FROM directory_stations WHERE id=$1`, id).Scan(&label); err != nil || label != "Synthetic previous catalog row" {
		t.Fatal("previous catalog changed", err)
	}
	if err := f.Admin.QueryRow(ctx, `SELECT kind FROM job_queue WHERE id=$1`, id).Scan(&kind); err != nil || kind != "synthetic-previous" {
		t.Fatal("previous job changed", err)
	}
	if err := f.Admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM job_queue),(SELECT count(*) FROM org_event_intents),(SELECT count(*) FROM org_event_receipts)`).Scan(&jobs, &intents, &receipts); err != nil || jobs != 1 || intents != 0 || receipts != 0 {
		t.Fatal("migration seeded work", err)
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("compatibility floor changed")
	}
}
