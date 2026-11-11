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

func TestAuditMigrationUpgradeFailureRecoveryAndPreviousCompatibility(t *testing.T) {
	const name = "000164_org_audit_events.sql"
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
	id := auditEvent().TargetID
	if _, err := f.Admin.Exec(ctx, `INSERT INTO directory_stations (id,display_name)
		VALUES ($1,'Synthetic previous catalog row')`, id); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for filename, file := range previous {
		current[filename] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken audit migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_audit_events') IS NULL
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version='000164')`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed audit migration left schema or ledger state")
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != "000164" {
		t.Fatalf("audit migration retry failed: %v, %v", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatalf("previous chain became incompatible: %v", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatalf("audit migration repeated work: %v, %v", applied, err)
	}
	var label string
	if err := f.Admin.QueryRow(ctx, `SELECT display_name FROM directory_stations WHERE id=$1`, id).Scan(&label); err != nil || label != "Synthetic previous catalog row" {
		t.Fatal("upgrade changed previous data")
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("audit migration changed compatibility floor")
	}
}
