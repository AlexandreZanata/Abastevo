//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestRootMigrationInstallsForcedScopedOwnership(t *testing.T) {
	f := orgtest.New(t)
	var present bool
	if err := f.Admin.QueryRow(t.Context(), "SELECT to_regclass('public.organizations') IS NOT NULL").Scan(&present); err != nil || !present {
		t.Fatal("organization root missing", err)
	}
	var forced bool
	if err := f.Admin.QueryRow(t.Context(), "SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='public.organizations'::regclass").Scan(&forced); err != nil || !forced {
		t.Fatal("root RLS is not forced", err)
	}
}

func TestRootPopulatedUpgradeRecoveryAndPreviousImageCompatibility(t *testing.T) {
	const name = "000169_organization_roots.sql"
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
	a, b := f.Organizations[0], f.Organizations[1]
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash) VALUES($1,$2,$3,'org.synthetic','org.root',$1,repeat('a',64))`, a.ID, a.RecordID, a.ParentID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_capabilities(org_id,capability,enabled,revision) VALUES($1,'synthetic',true,7)`, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_event_intents(org_id,event_id,consumer,event_name,target_type,target_id,revision,command_hash,job_id) VALUES($1,$2,'synthetic.consumer','org.synthetic.changed','org.root',$1,4,repeat('b',64),$3)`, a.ID, a.RecordID, a.ParentID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_event_receipts(org_id,event_id,consumer) VALUES($1,$2,'synthetic.consumer')`, a.ID, a.RecordID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic previous catalog')`, a.RecordID); err != nil {
		t.Fatal(err)
	}
	payloads := func() string {
		t.Helper()
		var data string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(e)) FROM org_audit_events e),(SELECT jsonb_agg(to_jsonb(c)) FROM org_capabilities c),(SELECT jsonb_agg(to_jsonb(i)) FROM org_event_intents i),(SELECT jsonb_agg(to_jsonb(r)) FROM org_event_receipts r))::text`).Scan(&data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	before := payloads()
	// Rehearse with an ordinary table-owning migrator, not the fixture superuser.
	// FORCE RLS would hide the legacy IDs without the atomic owner-only backfill.
	migrator := f.RuntimeRole + "_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT ON schema_migrations TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(clean, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	for _, table := range []string{"org_audit_events", "org_capabilities", "org_event_intents", "org_event_receipts"} {
		if _, err := f.Admin.Exec(ctx, "ALTER TABLE "+pgx.Identifier{table}.Sanitize()+" OWNER TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	dsn := location.String()
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, dsn, broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("broken migration did not reach the injected transaction failure", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('organizations') IS NULL
  AND to_regprocedure('organization_kind_tags_valid(text[])') IS NULL
  AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000169')
  AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname LIKE '%root_fk')
  AND (SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN ('org_audit_events'::regclass,'org_capabilities'::regclass,'org_event_intents'::regclass,'org_event_receipts'::regclass))`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed upgrade left schema/ledger or relaxed RLS", err)
	}
	current := fstest.MapFS{}
	for filename, file := range previous {
		current[filename] = file
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, dsn, current); err != nil || len(applied) != 1 || applied[0] != "000169" {
		t.Fatal("migration recovery", applied, err)
	}
	if got := payloads(); got != before {
		t.Fatal("upgrade changed private payloads")
	}
	var preserved bool
	if err := f.Admin.QueryRow(ctx, `SELECT count(*)=2 AND bool_and(NOT has_snapshot AND revision=1 AND display_name IS NULL AND time_zone IS NULL AND kind_tags IS NULL AND state IS NULL AND suspension_reason IS NULL) FROM organizations`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("legacy IDs lost or metadata invented", err)
	}
	var label string
	if err := f.Admin.QueryRow(ctx, `SELECT display_name FROM directory_stations WHERE id=$1`, a.RecordID).Scan(&label); err != nil || label != "Synthetic previous catalog" {
		t.Fatal("public catalog changed", err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous image incompatible", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, dsn, current); err != nil || len(applied) != 0 {
		t.Fatal("migration replay", applied, err)
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("schema floor changed")
	}
}
