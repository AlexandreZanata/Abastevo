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
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// The one file-name constant of EMP-AUTH-006; the coordinator may renumber it.
const structuralMigration = "000208_role_structural_scopes.sql"

// Populated previous head → 208 (EDR-160): a failed attempt changes nothing;
// recovery by an ordinary migrator keeps every existing assignment, receipt and
// structure byte for byte, leaves their structural references NULL, replays as
// a no-op, keeps the previous image compatible and installs the admission.
func TestStructuralScopePopulatedUpgradeKeepsAssignmentsAndAdmitsScopes(t *testing.T) {
	version := strings.SplitN(structuralMigration, "_", 2)[0]
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= structuralMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000206_organization_asset_allocation_history.sql"]; !ok {
		t.Fatal("previous image lacks the allocation schema")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	targets := structuralSchemaFixture(t, fx)
	member := structuralMemberIn(t, fx)
	if err := unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'AUDITOR','ORG',$1,clock_timestamp(),$4,'Synthetic pre-208 organization grant')`, org, onboardingID(t), member, fx.member); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "previous image structural scope", structuralGrantSQL(t, fx, onboardingID(t), member, "SUPERVISOR", "UNIT", targets.unit), "23514")
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(jsonb_build_array(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason,revision,created_at,ended_at) ORDER BY org_id,id) FROM org_role_assignments),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_role_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_units x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_cost_centres x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_teams x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_assets x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id) FROM organizations x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	catalog := func() string {
		t.Helper()
		var out string
		if err := f.Admin.QueryRow(t.Context(), `SELECT concat_ws(E'\n',
 (SELECT string_agg(pg_get_functiondef(oid),E'\n' ORDER BY proname) FROM pg_proc WHERE oid=ANY(ARRAY['org_asset_current_usage(uuid,uuid)'::regprocedure,'org_unit_current_usage(uuid,uuid)'::regprocedure,'org_cost_centre_current_usage(uuid,uuid)'::regprocedure,'org_team_current_usage(uuid,uuid)'::regprocedure])),
 (SELECT string_agg(conname||' '||pg_get_constraintdef(oid),E'\n' ORDER BY conname) FROM pg_constraint WHERE conrelid='org_role_assignments'::regclass),
 (SELECT string_agg(attname,',' ORDER BY attnum) FROM pg_attribute WHERE attrelid='org_role_assignments'::regclass AND attnum>0 AND NOT attisdropped),
 (SELECT string_agg(tgname,',' ORDER BY tgname) FROM pg_trigger WHERE tgrelid='org_role_assignments'::regclass AND NOT tgisinternal))`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, beforeCatalog := snapshot(), catalog()
	migrator := f.RuntimeRole + "_scope_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// An ordinary migrator owns exactly what 208 alters or replaces and only
	// references the four structure parents; neither superuser nor BYPASSRLS.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON org_units,org_cost_centres,org_teams,org_assets TO "+role+
		";ALTER TABLE org_role_assignments OWNER TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role+
		";ALTER FUNCTION org_cost_centre_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_team_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	reowned := false
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		statement := "DROP OWNED BY " + role + ";DROP ROLE " + role
		if !reowned {
			statement = "REASSIGN OWNED BY " + role + " TO CURRENT_USER;" + statement
		}
		if _, err := f.Admin.Exec(ctx, statement); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, structuralMigration)
	if err != nil {
		t.Fatal("structural scope migration missing", err)
	}
	broken := fstest.MapFS{structuralMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected structural migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1) AND to_regprocedure('org_role_assignment_structure_guard()') IS NULL
 AND(SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_role_assignments'::regclass)`, version).Scan(&clean); err != nil || !clean || before != snapshot() || beforeCatalog != catalog() {
		t.Fatal("failed migration changed the previous schema, admission, RLS or data", err)
	}
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[structuralMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary structural migration recovery", err)
	}
	if before != snapshot() {
		t.Fatal("structural migration rewrote existing assignments, receipts, memberships or structures")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("structural migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	var exact bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT(SELECT bool_and(num_nulls(scope_unit_id,scope_cost_centre_id,scope_team_id,scope_asset_id)=4) FROM org_role_assignments)
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE connamespace='public'::regnamespace AND NOT convalidated)
 AND(SELECT count(*) FROM pg_constraint WHERE conrelid='org_role_assignments'::regclass AND contype='f' AND conname LIKE 'org_role_assignments_scope_%_fkey')=4
 AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1) FROM pg_class WHERE oid='org_role_assignments'::regclass)`, migrator).Scan(&exact); err != nil || !exact {
		t.Fatal("upgrade invented references, left constraints unvalidated or weakened RLS", err)
	}
	if _, err := f.Admin.Exec(t.Context(), "REASSIGN OWNED BY "+role+" TO CURRENT_USER"); err != nil {
		t.Fatal(err)
	}
	reowned = true
	// Installed admission on the upgraded schema, through ordinary runtime SQL.
	for _, tc := range []struct{ kind, target string }{{"UNIT", targets.unit}, {"COST_CENTRE", targets.centre}, {"TEAM", targets.team}, {"ASSET", targets.active}} {
		if err := structuralGrantSQL(t, fx, onboardingID(t), member, "SUPERVISOR", tc.kind, tc.target); err != nil {
			t.Fatal("upgraded structural admission", tc.kind, err)
		}
	}
	allocationWant(t, "upgraded foreign unit", structuralGrantSQL(t, fx, onboardingID(t), member, "AUDITOR", "UNIT", onboardingID(t)), "23503")
	allocationWant(t, "upgraded draft asset", structuralGrantSQL(t, fx, onboardingID(t), member, "AUDITOR", "ASSET", targets.draft), "23514")
	if err := unitSchemaExec(t, fx, `UPDATE org_teams SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, targets.team); err != nil {
		t.Fatal("upgraded team usage source refused a grant", err)
	}
	// Persisted rows, old and new, rehydrate through the actual store.
	candidates, err := accesspg.NewStore(f.Runtime).Candidates(t.Context(), fx.scope, member)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[access.ScopeType]int{}
	for _, c := range candidates {
		v, err := c.Snapshot(fx.scope)
		if err != nil {
			t.Fatal(err)
		}
		kinds[v.Scope]++
	}
	if kinds[access.OrgScope] != 1 || kinds[access.SelfScope] != 1 || kinds[access.UnitScope] != 1 || kinds[access.CostCentreScope] != 1 || kinds[access.TeamScope] != 1 || kinds[access.AssetScope] != 1 {
		t.Fatal("upgraded candidates lost or invented scopes", kinds)
	}
	if err := pgx.BeginTxFunc(t.Context(), f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments`).Scan(&n); err != nil || n != 0 {
			t.Fatal("assignments visible without organization setting", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
