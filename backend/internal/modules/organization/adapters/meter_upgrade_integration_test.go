//go:build integration

package adapters_test

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
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

func TestMeterPopulatedOrdinaryUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000198_organization_meters.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	var unit string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	fleetPriorMemberAndTeam(t, fx, unit)
	assets := fleetStore(t, fx)
	input := fleetCreateInput()
	input.UnitID = unit
	command := fleetCommand(t, input)
	asset, _, err := fleetWrite(t, fx, assets, command)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,employee_id)FROM org_employee_profiles x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_teams x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_team_links x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_asset_categories x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_asset_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,asset_id,revision)FROM org_asset_revisions x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM account_session_families x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_meter_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_audit_events,org_assets,org_asset_revisions,org_memberships,org_device_enrollments TO "+role+";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_asset_guard() OWNER TO "+role+";ALTER FUNCTION org_asset_configuration_complete() OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected meter migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_meters') IS NULL AND to_regclass('org_meter_segments') IS NULL AND to_regclass('org_meter_readings') IS NULL AND to_regclass('org_meter_commands') IS NULL AND to_regprocedure('org_meter_observation_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000198')`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed migration changed original schema/data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000198" {
		t.Fatal("ordinary meter migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("meter upgrade rewrote existing enterprise/personal state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("meter migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=4 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))FROM pg_class WHERE oid=ANY(ARRAY['org_meters'::regclass,'org_meter_segments'::regclass,'org_meter_readings'::regclass,'org_meter_commands'::regclass]))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("new meter tables not owned by ordinary forced-RLS migrator", err)
	}
	f.GrantMeterUsage(t)
	if assets, employees := fleetUnitCounts(t, fx, unit); assets != 0 || employees != 1 {
		t.Fatal("meter source changed prior employee/home/team union")
	}
	store := meterStore(t, fx)
	installed, _, err := meterInstall(t, fx, store, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil || installed.AssetRevision != 2 {
		t.Fatal("ordinary upgraded meter source", err)
	}
	original, replay, err := fleetWrite(t, fx, assets, command)
	if err != nil || !replay || original != asset {
		t.Fatal("new meter source changed historical asset ACK", err)
	}
}
