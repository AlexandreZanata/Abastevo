//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestOnboardingPopulatedUpgradeRecoveryAndOrdinaryDefiners(t *testing.T) {
	const name = "000172_organization_onboarding.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	a := scopeFor(t, "7b0e2a54-0000-4000-8000-000000000301")
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON organizations TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	root, err := d.NewPending(a, d.Profile{Name: "Prior synthetic company", TimeZone: "UTC", Kinds: []d.Kind{d.Other}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapters.NewStore(f.Runtime).Create(t.Context(), a, root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES('7b0e2a54-0000-4000-8000-000000000701','prior-synthetic-account','active');
 INSERT INTO directory_stations(id,display_name) VALUES('7b0e2a54-0000-4000-8000-000000000901','Prior synthetic public station')`); err != nil {
		t.Fatal(err)
	}

	legacy, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, a.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES('7b0e2a54-0000-4000-8000-000000000301','7b0e2a54-0000-4000-8000-000000000501','ACTIVE');
 INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES('7b0e2a54-0000-4000-8000-000000000301','7b0e2a54-0000-4000-8000-000000000601','7b0e2a54-0000-4000-8000-000000000501','7b0e2a54-0000-4000-8000-000000000701','EXISTING_ACCOUNT','INVITED');
 INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES('7b0e2a54-0000-4000-8000-000000000301','7b0e2a54-0000-4000-8000-000000000801','7b0e2a54-0000-4000-8000-000000000601','OWNER','ORG','7b0e2a54-0000-4000-8000-000000000301',clock_timestamp(),'7b0e2a54-0000-4000-8000-000000000601','Prior synthetic owner');
 UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id='7b0e2a54-0000-4000-8000-000000000301' AND id='7b0e2a54-0000-4000-8000-000000000601'`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var got string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(o) ORDER BY org_id) FROM organizations o),(SELECT jsonb_agg(to_jsonb(a)-'enterprise_managed_org_id' ORDER BY id) FROM accounts a),(SELECT jsonb_agg(to_jsonb(d) ORDER BY id) FROM directory_stations d),(SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM org_memberships m),(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM org_employee_identities e),(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM org_role_assignments r))::text`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_onboard_migrator"
	r := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+r+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+r+"; GRANT SELECT,INSERT ON schema_migrations TO "+r+"; GRANT REFERENCES ON org_audit_events TO "+r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+r+" TO CURRENT_USER; DROP OWNED BY "+r+"; DROP ROLE "+r); err != nil {
			t.Error(err)
		}
	})
	for _, table := range []string{"accounts", "organizations", "org_employee_identities", "org_memberships", "org_role_assignments", "payment_billing_exits"} {
		if _, err := f.Admin.Exec(t.Context(), "ALTER TABLE "+pgx.Identifier{table}.Sanitize()+" OWNER TO "+r); err != nil {
			t.Fatal(err)
		}
	}
	for _, signature := range []string{"org_membership_check_account_exit(uuid)", "org_membership_account_erasure()", "org_membership_billing_exit_guard()", "org_account_access_reduction()"} {
		if _, err := f.Admin.Exec(t.Context(), "ALTER FUNCTION public."+signature+" OWNER TO "+r); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected migration failure", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_units') IS NULL AND to_regclass('org_onboarding_commands') IS NULL AND to_regprocedure('org_pending_owner_count(uuid,uuid)') IS NULL AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='accounts' AND column_name='enterprise_managed_org_id') AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000172')`).Scan(&clean); err != nil || !clean {
		t.Fatal("incomplete failed upgrade rollback", err)
	}
	if snapshot() != before {
		t.Fatal("failed upgrade changed existing rows")
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000172" {
		t.Fatal("upgrade recovery failed", applied, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed previous rows")
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous image incompatible", err)
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("upgrade replay failed", applied, err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND (SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN ('org_units'::regclass,'org_onboarding_commands'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("ordinary ownership/forced RLS missing", err)
	}
	_, _, service, actor, _ := onboardingOnFixture(t, f)
	for i := range 4 {
		got, replay, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
		if i < 3 {
			if err != nil || replay || got.Revision != 1 {
				t.Fatal("ordinary definer failed", i, err)
			}
		} else if !errors.Is(err, kernel.ErrPendingOrgLimit) {
			t.Fatal("ordinary definer missed limit", err)
		}
	}
	// Runtime cannot invoke trigger-only inventory or use its owner GUC to
	// inspect unselected roots/members/roles. It remains an ordinary login.
	if _, err := f.Runtime.Exec(t.Context(), `SELECT org_pending_owner_count($1,NULL)`, actor.AccountID); err == nil {
		t.Fatal("runtime invoked privileged inventory")
	}
	tx, err := f.Runtime.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.owner_account',$1,true)`, actor.AccountID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"organizations", "org_memberships", "org_role_assignments"} {
		var count int
		if err := tx.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Fatal("owner inventory GUC exposed private rows", table, count, err)
		}
	}
}
