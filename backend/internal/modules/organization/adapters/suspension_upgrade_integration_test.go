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

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestSuspensionOrdinaryPopulatedUpgradeRollbackAndRecovery(t *testing.T) {
	const name = "000184_organization_suspensions.sql"
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
	base := legalFixtureFor(t, f, onboarding, actor, flags)
	snapshot := func() string {
		t.Helper()
		var value string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(o) ORDER BY org_id) FROM organizations o),(SELECT jsonb_agg(to_jsonb(c) ORDER BY org_id,operation_id) FROM org_onboarding_commands c),(SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,id) FROM org_memberships m),(SELECT jsonb_agg(to_jsonb(s) ORDER BY org_id,revision) FROM org_settings_versions s),(SELECT jsonb_agg(to_jsonb(a) ORDER BY org_id,operation_id) FROM org_audit_events a))::text`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_suspension_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,admin_sessions TO "+role); e != nil {
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
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, e := fs.ReadFile(migrations.Files, name)
	if e != nil {
		t.Fatal(e)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, e := migrate.Apply(t.Context(), location.String(), broken); e == nil || !strings.Contains(e.Error(), "SQLSTATE 22012") {
		t.Fatal("injected upgrade failure not reached", e)
	}
	var clean bool
	if e := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_suspension_periods') IS NULL AND to_regclass('org_suspension_notices') IS NULL AND to_regprocedure('org_suspension_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000184')`).Scan(&clean); e != nil || !clean || snapshot() != before {
		t.Fatal("migration partial state", e)
	}
	current := fstest.MapFS{}
	for k, v := range previous {
		current[k] = v
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 1 || applied[0] != "000184" {
		t.Fatal("recovery", applied, e)
	}
	if snapshot() != before {
		t.Fatal("upgrade rewrote prior rows")
	}
	if applied, e := migrate.Apply(t.Context(), location.String(), current); e != nil || len(applied) != 0 {
		t.Fatal("repeat migration", e)
	}
	if e := migrate.Check(t.Context(), f.Admin, previous); e != nil {
		t.Fatal("previous image schema compatibility", e)
	}
	if e := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=2 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_suspension_periods'::regclass,'org_suspension_notices'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); e != nil || !clean {
		t.Fatal("ordinary forced-RLS ownership", e)
	}
	fx := suspensionOnFixture(t, base)
	if _, e := fx.change(t, app.SuspendOrganization, 2, ""); e != nil {
		t.Fatal("upgraded ordinary operator write", e)
	}
	if _, e := fx.change(t, app.ReactivateOrganization, 3, ""); e != nil {
		t.Fatal("upgraded resolution", e)
	}
	var count int
	if e := f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_suspension_periods`).Scan(&count); e != nil || count != 0 {
		t.Fatal("unscoped history leaked", e)
	}
}
