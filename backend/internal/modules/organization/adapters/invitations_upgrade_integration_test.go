//go:build integration

package adapters_test

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestInvitationOrdinaryPopulatedUpgradeRollbackRecovery(t *testing.T) {
	const name = "000188_employee_invitations.sql"
	previous := fstest.MapFS{}
	entries, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= name {
			continue
		}
		raw, e := fs.ReadFile(migrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	grantEmployeeTables(t, fx)
	employee := onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, fx.scope.OrgID().String(), employee); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,work_email)VALUES($1,$2,'Synthetic historical employee','historical@example.invalid')`, fx.scope.OrgID().String(), employee); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,employee_id)FROM org_employee_profiles x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_invitation_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,accounts,org_employee_identities,org_memberships,org_settings_versions,org_audit_events TO "+role+";GRANT TRIGGER ON organizations,org_memberships,org_employee_identities,org_employee_profiles TO "+role); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	location, e := url.Parse(f.AdminDSN)
	if e != nil {
		t.Fatal(e)
	}
	params := location.Query()
	params.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(params.Encode(), "+", "%20")
	raw, e := fs.ReadFile(migrations.Files, name)
	if e != nil {
		t.Fatal(e)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, e := migrate.Apply(t.Context(), location.String(), broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("migration failure injection not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_invitations') IS NULL AND to_regclass('org_invitation_commands') IS NULL AND to_regclass('org_invitation_notices') IS NULL AND to_regprocedure('org_invitation_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgname IN('org_invitation_contact','org_invitation_contact_created','org_invitation_issuer','org_invitation_employee','org_invitation_organization')) AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000188')`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("partial failed migration", e)
	}
	current := fstest.MapFS{}
	for k, v := range previous {
		current[k] = v
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000188" {
		t.Fatal("upgrade recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("historical identities or audit rewritten")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("upgrade not idempotent", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("prior image compatibility", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))FROM pg_class WHERE oid IN('org_invitations'::regclass,'org_invitation_commands'::regclass,'org_invitation_notices'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); e != nil || !clean {
		t.Fatal("ordinary forced RLS ownership", e)
	}
	for _, table := range []string{"org_invitations", "org_invitation_commands", "org_invitation_notices"} {
		var n int
		if e := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&n); e != nil || n != 0 {
			t.Fatal("fabricated invitation history", e)
		}
	}
	f.GrantInvitationLifecycle(t)
	catalog, e := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	invfx := invitationFixtureState{legalFixtureState: fx, invites: &memberpg.InvitationStore{Pool: f.Runtime, Memberships: fx.store.Memberships, Authority: &orgpg.InvitationAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags, Settings: &orgpg.SettingsStore{Pool: f.Runtime}}, Audit: auditpg.NewStore(f.Runtime, catalog)}}
	var v d.Invitation
	beforeAdmissions(t, f, func() { v = invitationIssue(t, invfx, employee) })
	value, _ := v.Snapshot(fx.scope)
	// Owner remains subject to FORCE RLS; no selected organization sees no rows.
	tx, e := f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e := tx.Exec(t.Context(), "SET LOCAL ROLE "+role); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_invitations`).Scan(&n); e != nil || n != 0 {
		t.Fatal("table owner bypassed RLS", e)
	}
	_ = tx.Rollback(t.Context())
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_invitations SET verifier=repeat('a',64) WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), value.ID), "P8803")
	// Give deletion privileges only to prove the retention trigger still refuses.
	if _, e := f.Admin.Exec(t.Context(), "GRANT DELETE,TRUNCATE ON org_invitations,org_invitation_commands,org_invitation_notices TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"org_invitations", "org_invitation_commands", "org_invitation_notices"} {
		teamSQLCode(t, unitSchemaExec(t, fx, `DELETE FROM `+pgx.Identifier{table}.Sanitize()), "55000")

	}
	teamSQLCode(t, unitSchemaExec(t, fx, `TRUNCATE org_invitations,org_invitation_commands,org_invitation_notices`), "55000")
}
