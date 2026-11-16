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
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// The migration under test; the previous image is every file before it.
const runMigration = "000242_organization_checklist_runs.sql"

// A populated head before 000242 (a completed and an in-progress trip with
// their receipts and an ACTIVE departure template) upgrades through an
// ordinary non-superuser migrator: an injected failure changes no schema,
// function or data, recovery applies once, replay is a no-op, the previous
// embedded image stays compatible, the new tables are owned by the ordinary
// forced-RLS migrator, every usage-source line is kept with the run keys, and
// the pre-existing template then takes a BLOCKING run that blocks the
// vehicle (EMP-CHECK-002, EDR-285).
func TestChecklistRunPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= runMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000233_organization_trip_lifecycle.sql"]; !ok {
		t.Fatal("previous image does not reach the 000233 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	x := tripStateOn(t, f)
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	first := x.startOp(t, d, completedAt, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 1, 1)
	end := x.finishOp(t, d, tripOf(first), completedAt.Add(time.Microsecond), nil)
	ack, err = x.process(t, d, end)
	x.accepted(t, end, ack, err, 2, 1)
	ongoing := x.startOp(t, d, x.declaredNow(t), nil)
	ack, err = x.process(t, d, ongoing)
	x.accepted(t, ongoing, ack, err, 1, 1)
	grantChecklistTables(t, x.legalFixtureState)
	catalog, err := audit.NewCatalog(maintpg.ChecklistAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	templates := &maintpg.ChecklistStore{Pool: f.Runtime, Authority: &orgpg.ChecklistAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles, Capabilities: x.flags,
		Structure: &orgpg.ResourceStructure{Kind: orgpg.AssetStructure, Pool: f.Runtime, Allocations: x.assets}, Assignee: &orgpg.ResourceAssignee{Pool: f.Runtime, Assignments: x.assets}}, Audit: auditpg.NewStore(f.Runtime, catalog)}
	template, _, err := templateRun(t, x.legalFixtureState, templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandCreate, Context: checklist.Departure,
		Name: "Checklist anterior", Target: organizationTarget(), Items: runItems(), Policy: checklist.DefaultPolicy()}))
	if err != nil {
		t.Fatal("pre-upgrade template", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_checklist_templates x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,template_id,version)FROM org_checklist_template_versions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,template_id,revision)FROM org_checklist_policy_revisions x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	functions := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure)||pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure)
 ||pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure)`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before, beforeFunctions := snapshot(), functions()
	migrator := f.RuntimeRole + "_run_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// The production migration owner owns the three usage sources it
	// redefines and holds REFERENCES on the parents.
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_sync_operations,org_assets,org_units,org_cost_centres,org_checklist_template_versions,org_checklist_template_items,org_checklist_policy_revisions,org_memberships,org_employee_identities,org_device_enrollments,org_audit_events TO "+role+
		";ALTER FUNCTION org_asset_current_usage(uuid,uuid) OWNER TO "+role+";ALTER FUNCTION org_unit_current_usage(uuid,uuid) OWNER TO "+role+
		";ALTER FUNCTION org_cost_centre_current_usage(uuid,uuid) OWNER TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, runMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(runMigration, "_", 2)[0]
	broken := fstest.MapFS{runMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected run migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_checklist_runs') IS NULL AND to_regclass('org_checklist_run_answers') IS NULL AND to_regclass('org_checklist_run_releases') IS NULL
 AND to_regprocedure('org_checklist_run_source()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() || beforeFunctions != functions() {
		t.Fatal("failed run migration changed the schema, a function or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[runMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary run migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("run upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("run migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls))
 FROM pg_class WHERE oid IN('org_checklist_runs'::regclass,'org_checklist_run_answers'::regclass,'org_checklist_run_releases'::regclass)`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("run tables not owned by the ordinary forced-RLS migrator", err)
	}
	// Every previous allow-list line is kept, with the run keys added.
	var kept bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT a LIKE '%org_trips_asset_fkey%' AND a LIKE '%org_trip_held_starts_asset_fkey%' AND a LIKE '%org_asset_implement_attachments_powered_fkey%'
 AND a LIKE '%org_checklist_template_targets_asset_fkey%' AND a LIKE '%org_document_links_asset_fkey%' AND a LIKE '%org_fuelings_asset_fkey%' AND a LIKE '%org_incident_reports_asset_fkey%'
 AND a LIKE '%org_trip_missions_asset_fkey%' AND a LIKE '%org_trip_mission_vehicles_previous_fkey%' AND a LIKE '%org_trip_mission_vehicles_asset_fkey%' AND a LIKE '%org_checklist_runs_asset_fkey%'
 AND u LIKE '%org_trip_missions_unit_fkey%' AND u LIKE '%org_trips_unit_fkey%' AND u LIKE '%org_fuelings_unit_fkey%' AND u LIKE '%org_incident_reports_unit_fkey%'
 AND u LIKE '%org_trip_mission_plans_destination_unit_fkey%' AND u LIKE '%org_checklist_runs_unit_fkey%'
 AND c LIKE '%org_trips_cost_centre_fkey%' AND c LIKE '%org_fuelings_cost_centre_fkey%' AND c LIKE '%org_incident_reports_cost_centre_fkey%'
 AND c LIKE '%org_trip_mission_plans_cost_centre_fkey%' AND c LIKE '%org_checklist_runs_cost_centre_fkey%'
 FROM(SELECT pg_get_functiondef('org_asset_current_usage(uuid,uuid)'::regprocedure) a,pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) u,
 pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure) c) s`).Scan(&kept); err != nil || !kept {
		t.Fatal("a usage source lost a line or misses the run key", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 242 rehearsal", err)
	}
	// The pre-existing template takes a BLOCKING run on the upgraded schema;
	// the vehicle is then BLOCKED and the usage sources still answer.
	tripGrants(t, x.legalFixtureState)
	x.assets.ChecklistBlocks = checklistBlockSource(t, x.legalFixtureState)
	e := incidentFixtureOn(t, eventFixtureOn(t, x))
	tripReviewGrants(t, e.f)
	h := handlingState{incidentState: e}
	r := runFixtureOn(t, h)
	at := r.declaredNow(t)
	op := r.runOp(t, d, r.runPayload(template.TemplateID, 1, 1, at.Add(-time.Minute), "BLOCKING", answer("OK", nil), answer("NOT_OK", nil), answer("OK", nil)), at, nil)
	ack, err = r.process(t, d, op)
	r.runAccepted(t, op, ack, err, 3)
	// The pre-existing in-progress trip keeps the vehicle IN_USE; the run is
	// listed as a further BLOCKED cause for the next start.
	availability, causes := r.availability(t)
	blocked := false
	for _, c := range causes {
		blocked = blocked || c.Kind == fleet.ChecklistCause && c.ID == runOf(op) && c.Availability == fleet.Blocked
	}
	if availability != fleet.InUse || !blocked {
		t.Fatal("upgraded vehicle does not list the run's block", availability, causes)
	}
	var usage bool
	org := x.scope.OrgID().String()
	if err := pgx.BeginFunc(t.Context(), f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2) IS NOT NULL AND org_unit_current_usage($1,$3) IS NOT NULL`, org, x.vehicle.AssetID, x.unit).Scan(&usage)
	}); err != nil || !usage {
		t.Fatal("upgraded usage sources", err)
	}
}
