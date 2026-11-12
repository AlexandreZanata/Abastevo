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
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	employee "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	idn "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestEmployeeProfilesPopulatedOrdinaryUpgradeFailureRecoveryAndHistory(t *testing.T) {
	const name = "000176_employee_profiles.sql"
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
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0)); err != nil {
		t.Fatal(err)
	}
	oldUnits := operatingUnitStore(t, fx)
	priorUnit := onboardingID(t)
	if _, _, err := oldUnits.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, priorUnit, operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic prior unit", Code: "PRE"}})); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO directory_stations(id,display_name) VALUES($1,'Prior synthetic public row')`, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY org_id) FROM organizations o),
 (SELECT jsonb_agg(to_jsonb(u) ORDER BY org_id,id) FROM org_units u),
 (SELECT jsonb_agg(to_jsonb(e) ORDER BY org_id,id) FROM org_establishments e),
 (SELECT jsonb_agg(to_jsonb(l) ORDER BY org_id,id) FROM org_legal_entities l),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY org_id,operation_id) FROM org_audit_events a),
 (SELECT jsonb_agg(to_jsonb(c) ORDER BY org_id,operation_id) FROM org_onboarding_commands c),
 (SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,id) FROM org_memberships m),
 (SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,id) FROM org_employee_identities m),
 (SELECT jsonb_agg(to_jsonb(m) ORDER BY org_id,operation_id) FROM org_unit_commands m),
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY org_id,id) FROM org_role_assignments r),
 (SELECT jsonb_agg(to_jsonb(d) ORDER BY id) FROM directory_stations d))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_employee_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE; GRANT USAGE,CREATE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT ON schema_migrations TO "+role+"; GRANT REFERENCES ON organizations,accounts,org_audit_events,org_employee_identities,org_units TO "+role+"; ALTER TABLE org_units OWNER TO "+role+"; ALTER TABLE org_unit_commands OWNER TO "+role+"; ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); err != nil {
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
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("did not reach injected employee-profile migration failure", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_employee_profiles') IS NULL AND to_regclass('org_employee_commands') IS NULL AND to_regclass('org_employee_unit_clearings') IS NULL AND to_regprocedure('org_employee_text_valid(text,integer,boolean)') IS NULL AND to_regprocedure('org_employee_profile_guard()') IS NULL AND to_regprocedure('org_employee_consume_unit_archive()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000176')`).Scan(&clean); err != nil || !clean {
		t.Fatal("incomplete employee-profile migration rollback", err)
	}
	if before != snapshot() {
		t.Fatal("failed migration changed prior rows")
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000176" {
		t.Fatal("employee-profile migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("upgrade changed prior rows")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("employee-profile migration repeat", applied, err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous schema compatibility failed", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)) FROM pg_class WHERE oid IN('org_employee_profiles'::regclass,'org_employee_commands'::regclass,'org_employee_unit_clearings'::regclass)) FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("ordinary employee-profile ownership/forced RLS missing", err)
	}
	grantEmployeeTables(t, fx)
	catalog, err := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store := &memberpg.EmployeeStore{Pool: f.Runtime, Authority: &adapters.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(f.Runtime, catalog)}
	var creator string
	if err := f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member).Scan(&creator); err != nil {
		t.Fatal(err)
	}
	previousIdentity, err := memberpg.NewStore(f.Runtime).GetEmployee(t.Context(), fx.scope, creator)
	if err != nil {
		t.Fatal("previous image employee read", err)
	}
	beforeIdentity, _ := previousIdentity.Snapshot(fx.scope)
	pending, err := store.ReadEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, creator)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := pending.Snapshot(fx.scope)
	if view.ProfileRevision != 0 {
		t.Fatal("upgrade invented profile")
	}
	command := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CompleteEmployee, EmployeeID: creator, ExpectedRevision: beforeIdentity.Revision, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Upgraded Creator", HomeUnitID: priorUnit}})
	if _, _, err := writeEmployee(t, fx, store, command); err != nil {
		t.Fatal("ordinary upgraded completion", err)
	}
	previousIdentity, err = memberpg.NewStore(f.Runtime).GetEmployee(t.Context(), fx.scope, creator)
	if err != nil {
		t.Fatal(err)
	}
	afterIdentity, _ := previousIdentity.Snapshot(fx.scope)
	if beforeIdentity != afterIdentity {
		t.Fatal("profile upgrade changed legacy lifecycle")
	}
	created, _, err := writeEmployee(t, fx, store, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Upgrade No Login"}}))
	if err != nil || created.IdentityRevision != 1 {
		t.Fatal("ordinary upgraded employee", err)
	}
}
