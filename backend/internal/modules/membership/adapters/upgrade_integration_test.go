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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestMembershipUpgradeRecoveryAndOrdinaryDefinerErasure(t *testing.T) {
	const name = "000170_membership_periods.sql"
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
	runtimeRole := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+runtimeRole+"; GRANT SELECT,INSERT,UPDATE ON organizations TO "+runtimeRole); err != nil {
		t.Fatal(err)
	}
	a, b := scope(t, 301), scope(t, 302)
	for _, scope := range []kernel.OrgScope{a, b} {
		v, err := org.NewPending(scope, org.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orgpg.NewStore(f.Runtime).Create(ctx, scope, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-migration-member','active')`, key(401)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash) VALUES($1,$2,$3,'org.synthetic','org.root',$1,repeat('a',64))`, a.OrgID().String(), key(701), key(401)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic previous catalog')`, key(901)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO payment_billing_exits(id,account_id,operation_key,reason,state,requested_at,last_event_at) VALUES($1,$2,'previous','OWNER_REQUEST','PENDING',now(),now())`, key(801), key(401)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(a)) FROM accounts a),(SELECT jsonb_agg(to_jsonb(e)) FROM org_audit_events e),(SELECT jsonb_agg(to_jsonb(p)) FROM payment_billing_exits p),(SELECT jsonb_agg(to_jsonb(s)) FROM directory_stations s))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT ON schema_migrations TO "+role+"; GRANT SELECT,UPDATE,REFERENCES ON organizations TO "+role+"; ALTER TABLE accounts OWNER TO "+role+"; ALTER TABLE payment_billing_exits OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(clean, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(dbmigrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected upgrade failure", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_memberships') IS NULL AND to_regclass('org_employee_identities') IS NULL
 AND to_regprocedure('org_membership_check_account_exit(uuid)') IS NULL
 AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname IN ('org_membership_account_erasure','org_membership_billing_exit_guard'))
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000170')`).Scan(&clean); err != nil || !clean {
		t.Fatal("upgrade failed to roll back schema/triggers/ledger", err)
	}
	current := fstest.MapFS{}
	for file, value := range previous {
		current[file] = value
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000170" {
		t.Fatal("upgrade recovery", applied, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed previous private/public/billing rows")
	}
	var empty bool
	if err := f.Admin.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM org_memberships) AND NOT EXISTS(SELECT 1 FROM org_employee_identities)`).Scan(&empty); err != nil || !empty {
		t.Fatal("invented identity or access rows", err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous binary incompatible", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("upgrade not idempotent", applied, err)
	}
	var ordinary bool
	if err := f.Admin.QueryRow(ctx, `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin FROM pg_roles WHERE rolname=$1`, migrator).Scan(&ordinary); err != nil || !ordinary {
		t.Fatal("not an ordinary migrator", err)
	}
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT,INSERT,UPDATE ON org_employee_identities,org_memberships TO "+runtimeRole+"; GRANT SELECT,UPDATE ON accounts TO "+runtimeRole+"; GRANT SELECT ON payment_billing_exits TO "+runtimeRole); err != nil {
		t.Fatal(err)
	}
	store := adapters.NewStore(f.Runtime)
	bound(t, store, a, 501, 601, 401)
	bound(t, store, b, 501, 601, 401)
	// The definer owns forced-RLS tables without BYPASSRLS. Its owner-only
	// account policy must traverse BOTH organizations, never silently zero rows.
	if _, err := f.Admin.Exec(ctx, `UPDATE accounts SET status='deleted' WHERE id=$1`, key(401)); err != nil {
		t.Fatal("ordinary definer could not erase", err)
	}
	var erased int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_memberships WHERE state='ENDED' AND account_id IS NULL AND account_tombstone IS NOT NULL`).Scan(&erased); err != nil || erased != 2 {
		t.Fatal("FORCE RLS hid private links", erased, err)
	}
	noScope(t, f)
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("compatibility floor changed")
	}
}
