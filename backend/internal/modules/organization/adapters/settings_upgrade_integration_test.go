//go:build integration

package adapters_test

import (
	"context"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestSettingsPopulatedOrdinaryUpgradeRecoveryAndAuthenticBootstrap(t *testing.T) {
	const name = "000178_organization_settings.sql"
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
	snapshot := func() string {
		t.Helper()
		var value string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(o) ORDER BY org_id) FROM organizations o),(SELECT jsonb_agg(to_jsonb(u) ORDER BY org_id,id) FROM org_units u),(SELECT jsonb_agg(to_jsonb(c) ORDER BY org_id,operation_id) FROM org_onboarding_commands c),(SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,id) FROM org_memberships m),(SELECT jsonb_agg(to_jsonb(r) ORDER BY org_id,id) FROM org_role_assignments r),(SELECT jsonb_agg(to_jsonb(a) ORDER BY org_id,operation_id) FROM org_audit_events a))::text`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_settings_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT ON schema_migrations TO "+role+"; GRANT REFERENCES ON organizations,accounts,org_memberships,org_audit_events TO "+role+"; GRANT TRIGGER ON organizations,org_onboarding_commands,org_memberships TO "+role); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	location, e := url.Parse(f.AdminDSN)
	if e != nil {
		t.Fatal(e)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, e := fs.ReadFile(migrations.Files, name)
	if e != nil {
		t.Fatal(e)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, e := migrate.Apply(t.Context(), location.String(), broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected migration outage", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_settings_versions') IS NULL AND to_regprocedure('org_settings_bootstrap(uuid)') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000178')`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("failed upgrade left changes", e)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000178" {
		t.Fatal("upgrade recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("upgrade rewrote prior rows")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("migration repeated", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("prior image compatibility", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=5 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_settings_versions'::regclass,'org_settings_heads'::regclass,'org_settings_designations'::regclass,'org_settings_commands'::regclass,'org_settings_notices'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); e != nil || !clean {
		t.Fatal("ordinary settings ownership missing", e)
	}
	// Forced RLS prevents a global backfill. Initialization occurs now, scoped to
	// an authentic creator receipt, and never claims past authorship/time.
	var count int
	if e := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_settings_versions`).Scan(&count); e != nil || count != 0 {
		t.Fatal("unscoped backfill invented history", e)
	}
	f.GrantSettingsLifecycle(t)
	if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	s := settingsStore(t, fx)
	if _, e := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_bootstrap_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic bootstrap storage outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_bootstrap_outage AFTER INSERT ON org_settings_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_bootstrap_outage()`); e != nil {
		t.Fatal(e)
	}
	failed, e := s.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if !errors.Is(e, app.ErrUnavailable) || failed != (app.SettingsRead{}) {
		t.Fatal("bootstrap commit failure exposed a provisional read or misleading absence", e)
	}
	page, e := s.ListSettings(t.Context(), fx.scope, fx.actor, fx.member, app.SettingsFilter{Limit: 10})
	if !errors.Is(e, app.ErrUnavailable) || len(page.Versions) != 0 || page.NextRevision != 0 {
		t.Fatal("bootstrap list failure exposed provisional versions", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_settings_versions`).Scan(&count); e != nil || count != 0 {
		t.Fatal("failed bootstrap persisted", e)
	}
	if _, e := f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_bootstrap_outage ON org_settings_versions`); e != nil {
		t.Fatal(e)
	}
	started := time.Now().UTC()
	boot := settingsRead(t, fx, s, 0)
	if boot.Source != d.SettingsBootstrap || boot.Revision != 1 || boot.Author != (d.SettingsAuthor{}) || boot.OperationID != "" || boot.EffectiveAt.Before(started) || boot.Values.Responsibilities.Billing.PrimaryID != fx.member {
		t.Fatal("bootstrap invented author/time/default")
	}
	if settingsRead(t, fx, s, 0) != boot || snapshot() != before {
		t.Fatal("bootstrap repeat changed prior state")
	}
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, boot.Values, "")); e != nil {
		t.Fatal("ordinary upgraded settings write", e)
	}
	if _, _, e := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t))); e != nil {
		t.Fatal("ordinary upgraded onboarding atomic defaults", e)
	}
	// Legacy metadata anchors have no genuine creation receipt to bootstrap.
	err := database.WithOrgTx(t.Context(), f.Runtime, f.Organizations[0].Scope(t), func(tx pgx.Tx) error {
		_, e := tx.Exec(t.Context(), `SELECT org_settings_bootstrap($1::uuid)`, f.Organizations[0].ID)
		return e
	})
	teamSQLCode(t, err, "23503")
}
