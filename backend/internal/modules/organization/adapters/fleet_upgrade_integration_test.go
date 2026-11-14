//go:build integration

package adapters_test

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	idn "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
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

func fleetUnitCounts(t *testing.T, fx legalFixtureState, unit string) (int64, int64) {
	t.Helper()
	var assets, employees int64
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT (u->>'assets')::bigint,(u->>'employees')::bigint FROM(SELECT org_unit_current_usage($1,$2) u) q`, fx.scope.OrgID().String(), unit).Scan(&assets, &employees)
	})
	if err != nil {
		t.Fatal("real unit source counts unavailable", err)
	}
	return assets, employees
}
func fleetPriorMemberAndTeam(t *testing.T, fx legalFixtureState, unit string) {
	t.Helper()
	grantEmployeeTables(t, fx)
	grantTeamTables(t, fx)
	catalog, err := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	employees := &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &orgpg.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	person, _, err := writeEmployee(t, fx, employees, employeeCommand(t, member.EmployeeWriteInput{Kind: member.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic home-and-team worker", HomeUnitID: unit}}))
	if err != nil {
		t.Fatal(err)
	}
	teams := teamStore(t, fx)
	team, _, err := teamWrite(t, fx, teams, teamCommand(t, org.TeamWriteInput{Kind: org.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic preserved crew", UnitID: unit}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := teamWrite(t, fx, teams, teamCommand(t, org.TeamWriteInput{Kind: org.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: person.EmployeeID, ExpectedRevision: 1})); err != nil {
		t.Fatal(err)
	}
}
func TestFleetPopulatedOrdinaryUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000197_organization_assets.sql"
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
	if assets, employees := fleetUnitCounts(t, fx, unit); assets != 0 || employees != 1 {
		t.Fatal("prior employee UNION fixture not established")
	}
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic preserved public station')`, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_employee_identities x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,employee_id)FROM org_employee_profiles x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_teams x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_team_links x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_onboarding_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM account_session_families x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM directory_stations x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_fleet_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON organizations,org_audit_events,org_units TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
		t.Fatal("injected migration failure was not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_assets') IS NULL AND to_regclass('org_asset_categories') IS NULL AND to_regclass('org_fuel_products_v1') IS NULL AND to_regprocedure('org_asset_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000197')`).Scan(&clean); err != nil || !clean || snapshot() != before {
		t.Fatal("asset migration failed atomic recovery", err)
	}
	if assets, employees := fleetUnitCounts(t, fx, unit); assets != 0 || employees != 1 {
		t.Fatal("failed migration changed prior usage")
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000197" {
		t.Fatal("ordinary migration recovery", err)
	}
	if snapshot() != before {
		t.Fatal("asset upgrade changed prior private/public rows")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("asset upgrade no-op repeat", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=8 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))FROM pg_class WHERE oid=ANY(ARRAY['org_asset_categories'::regclass,'org_assets'::regclass,'org_asset_tanks'::regclass,'org_asset_tank_products'::regclass,'org_asset_expected'::regclass,'org_asset_unit_allocations'::regclass,'org_asset_revisions'::regclass,'org_asset_commands'::regclass]))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("ordinary fleet migration ownership/RLS missing", err)
	}
	f.GrantFleetUsage(t)
	if assets, employees := fleetUnitCounts(t, fx, unit); assets != 0 || employees != 1 {
		t.Fatal("new fleet source changed prior distinct home/team count")
	}
}
