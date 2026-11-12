//go:build integration

package adapters_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestCapabilityMigrationUpgradeFailureRetryAndPreviousCompatibility(t *testing.T) {
	const name = "000166_org_capabilities.sql"
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
	if _, err := f.Admin.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic previous catalog')`, operation(1)); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_capabilities') IS NULL
		AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000166')`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed migration left state", err)
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != "000166" {
		t.Fatal(applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous chain refused new schema", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatal("migration was not idempotent", applied, err)
	}
	var count int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_capabilities`).Scan(&count); err != nil || count != 0 {
		t.Fatal("migration activated an organization", err)
	}
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT,INSERT ON org_capabilities TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	scope := f.Organizations[0].Scope(t)
	if err := database.WithOrgTx(ctx, f.Runtime, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_capabilities(org_id,capability) VALUES($1,'fuelings')`, scope.OrgID().String())
		if err != nil {
			return err
		}
		var enabled bool
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT enabled,revision FROM org_capabilities`).Scan(&enabled, &revision); err != nil {
			return err
		}
		if enabled || revision != 1 {
			t.Fatal("new state did not default off")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := f.Admin.QueryRow(ctx, `SELECT display_name FROM directory_stations WHERE id=$1`, operation(1)).Scan(&label); err != nil || label != "Synthetic previous catalog" {
		t.Fatal("upgrade changed public data", err)
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("compatibility floor changed", err)
	}
}
