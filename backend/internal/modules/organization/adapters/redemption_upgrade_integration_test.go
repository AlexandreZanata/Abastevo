//go:build integration

package adapters_test

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
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

// beforeAdmissions runs a seed with the current invitation store on an image
// older than 000189. The current issue/list path first materializes expired
// admissions (B-BR-EMP-174) in a table that image does not have and that can
// hold no rows before 000189, so an empty stand-in keeps that step a no-op. It
// is dropped before any snapshot or migration assertion runs.
func beforeAdmissions(t *testing.T, f *orgtest.Fixture, seed func()) {
	t.Helper()
	if _, e := f.Admin.Exec(t.Context(), `CREATE TABLE org_invitation_acceptances(org_id uuid NOT NULL,state text NOT NULL,revision bigint NOT NULL,cause text,deadline timestamptz NOT NULL);GRANT SELECT,UPDATE ON org_invitation_acceptances TO `+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	seed()
	if _, e := f.Admin.Exec(t.Context(), `DROP TABLE org_invitation_acceptances`); e != nil {
		t.Fatal(e)
	}
}

func TestRedemptionOrdinaryPopulatedUpgradeRollbackRecovery(t *testing.T) {
	const name = "000189_invitation_redemption.sql"
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
	catalog, e := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	invfx := invitationFixtureState{legalFixtureState: fx, invites: &memberpg.InvitationStore{Pool: f.Runtime, Memberships: fx.store.Memberships, Authority: &orgpg.InvitationAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags, Settings: &orgpg.SettingsStore{Pool: f.Runtime}}, Audit: auditpg.NewStore(f.Runtime, catalog)}}
	var v d.InvitationSnapshot
	var code string
	beforeAdmissions(t, f, func() { v, code = redeemCode(t, invfx, "DRIVER") })
	snapshot := func() string {
		t.Helper()
		var out string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)-'failed_attempts' ORDER BY org_id,id) FROM org_invitations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_invitation_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_admission_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,accounts,org_memberships,org_settings_versions,org_audit_events TO "+role+";ALTER TABLE org_invitations OWNER TO "+role+";ALTER FUNCTION org_membership_guard() OWNER TO "+role+";ALTER FUNCTION org_invitation_guard() OWNER TO "+role+";ALTER FUNCTION org_invitation_project() OWNER TO "+role+";ALTER FUNCTION org_invitation_context_changed() OWNER TO "+role+";ALTER FUNCTION org_invitation_command_guard() OWNER TO "+role+";ALTER FUNCTION org_settings_values_valid(jsonb) OWNER TO "+role); e != nil {
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
		t.Fatal("failure injection not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_invitation_acceptances') IS NULL AND to_regclass('org_invitation_locators') IS NULL AND to_regprocedure('org_invitation_acceptance_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='org_invitations' AND column_name='failed_attempts') AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000189')`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("partial upgrade", e)
	}
	current := fstest.MapFS{}
	for key, value := range previous {
		current[key] = value
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000189" {
		t.Fatal("upgrade recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("previous history rewritten")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("repeat migration", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("previous image schema", e)
	}
	invitationCount(t, invfx, `SELECT count(*) FROM org_invitation_locators WHERE invitation_id=$1`, 1, v.ID)
	invitationCount(t, invfx, `SELECT count(*) FROM org_invitation_acceptances`, 0)
	f.GrantInvitationLifecycle(t)
	tx, e := f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e := tx.Exec(t.Context(), "SET LOCAL ROLE "+role); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_invitation_locators`).Scan(&count); e != nil || count != 0 {
		t.Fatal("owner RLS", e)
	}
	_ = tx.Rollback(t.Context())
	s := redemptionStore(t, invfx)
	a := redemptionActor(t, invfx)
	preview, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil {
		t.Fatal("historical locator", e)
	}
	if _, e = s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}); e != nil {
		t.Fatal("accept historical invitation", e)
	}
	for _, table := range []string{"org_invitation_locators", "org_invitation_acceptances", "org_invitation_person_commands", "org_invitation_acceptance_notices"} {
		if _, e := f.Admin.Exec(t.Context(), "GRANT DELETE,TRUNCATE ON "+table+" TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
			t.Fatal(e)
		}
		teamSQLCode(t, unitSchemaExec(t, fx, "DELETE FROM "+table), "55000")
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `TRUNCATE org_invitation_locators,org_invitation_acceptances,org_invitation_person_commands,org_invitation_acceptance_notices`), "55000")
}
