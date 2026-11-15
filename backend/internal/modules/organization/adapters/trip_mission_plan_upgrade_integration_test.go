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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated head before 000231 (000230 included; a completed and an in-progress trip through
// the real signed intake; DRAFT, ASSIGNED, ACCEPTED and CANCELLED missions
// with their receipts; a second unit and a cost centre) upgrades to 000231 by
// an ordinary non-superuser migrator: an injected failure leaves schema,
// functions and data untouched, recovery applies once, replay is a no-op, the
// previous embedded image stays compatible, the unit and cost-centre usage
// sources keep every line and their answers, the EMP-TRIP-008 receipts replay
// unchanged, and plans then work on the upgraded schema (EMP-TRIP-009).
func TestTripMissionPlanPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= missionPlanMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, head := range []string{missionMigration, "000229_organization_trip_amendments.sql", "000230_organization_incident_reports.sql"} {
		if _, ok := previous[head]; !ok {
			t.Fatal("previous image does not contain", head)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal))))
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
	m := missionFixtureOn(t, x)
	org := x.scope.OrgID().String()
	south := m.secondUnit(t)
	grantCostCentreTables(t, m.legalFixtureState)
	centre := onboardingID(t)
	if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic client contract','CTR-UPGRADE','ACTIVE')`, org, centre); err != nil {
		t.Fatal(err)
	}
	driver := m.driver(t)
	draft := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	accepted := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, accepted, 1, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	if ack, err := m.deliver(t, driver, m.respondOp(t, driver, accepted, 2, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("acceptance before the upgrade", err)
	}
	assigned := m.create(t, south, "").MissionID
	assignInput := tripapp.MissionCommandInput{Kind: tripapp.MissionAssignCommand, OperationID: onboardingID(t), MissionID: assigned, ExpectedRevision: 1, AssigneeEmployeeID: driver.person.employee}
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, assignInput)); err != nil {
		t.Fatal(err)
	}
	cancelled := m.create(t, m.unit, "").MissionID
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripappCancel(cancelled, 1))); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id)FROM organizations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_units x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_cost_centres x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trip_missions x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,mission_id,revision)FROM org_trip_mission_transitions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_trip_mission_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM accounts x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	usage := func() string {
		t.Helper()
		tx, err := f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		var unit, other, cost string
		if err := tx.QueryRow(t.Context(), `SELECT org_unit_current_usage($1,$2)::text,org_unit_current_usage($1,$3)::text,org_cost_centre_current_usage($1,$4)::text`, org, x.unit, south, centre).Scan(&unit, &other, &cost); err != nil {
			t.Fatal("usage sources refused the populated state", err)
		}
		return unit + other + cost
	}
	replaced := []string{"org_unit_current_usage(uuid,uuid)", "org_cost_centre_current_usage(uuid,uuid)", "org_trip_mission_next(text,text)", "org_trip_mission_transition_source()",
		"org_trip_mission_guard()", "org_trip_mission_complete()"}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(to_regprocedure(p)),E'\n' ORDER BY p) FROM unnest($1::text[]) p`, replaced).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	constraints := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(conname||'='||pg_get_constraintdef(oid),E'\n' ORDER BY conname) FROM pg_constraint WHERE conrelid IN('org_trip_mission_transitions'::regclass,'org_trip_mission_commands'::regclass)`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeUsage, beforeFunctions, beforeConstraints := snapshot(), usage(), functions(), constraints()
	migrator := f.RuntimeRole + "_plan_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	grants := "CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO " + role + ";GRANT SELECT,INSERT ON schema_migrations TO " + role +
		";GRANT REFERENCES ON accounts,org_units,org_cost_centres,org_memberships TO " + role +
		";ALTER TABLE org_trip_mission_transitions OWNER TO " + role + ";ALTER TABLE org_trip_mission_commands OWNER TO " + role
	for _, p := range replaced {
		grants += ";ALTER FUNCTION " + p + " OWNER TO " + role
	}
	if _, err := f.Admin.Exec(t.Context(), grants); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, missionPlanMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(missionPlanMigration, "_", 2)[0]
	broken := fstest.MapFS{missionPlanMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected plan migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_mission_plans') IS NULL AND to_regprocedure('org_trip_mission_plan_text_valid(text)') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() || beforeFunctions != functions() || beforeConstraints != constraints() {
		t.Fatal("failed plan migration changed the original schema, functions, constraints or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[missionPlanMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary plan migration recovery", applied, err)
	}
	if before != snapshot() || beforeUsage != usage() {
		t.Fatal("plan upgrade rewrote existing state or a usage answer")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("plan migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid='org_trip_mission_plans'::regclass)FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the plan table is not owned by the ordinary forced-RLS migrator", err)
	}
	var lines bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT u LIKE '%org_trip_mission_plans_destination_unit_fkey%' AND u LIKE '%org_trip_missions_unit_fkey%' AND u LIKE '%org_trips_unit_fkey%' AND u LIKE '%org_fuelings_unit_fkey%'
 AND u LIKE '%org_onboarding_commands_org_id_unit_id_fkey%' AND u LIKE '%org_incident_reports_unit_fkey%' AND c LIKE '%org_trip_mission_plans_cost_centre_fkey%' AND c LIKE '%org_trips_cost_centre_fkey%'
 AND c LIKE '%org_fuelings_cost_centre_fkey%' AND c LIKE '%org_cost_centre_codes_centre_fkey%' AND c LIKE '%org_incident_reports_cost_centre_fkey%'
 FROM(SELECT pg_get_functiondef('org_unit_current_usage(uuid,uuid)'::regprocedure) AS u,pg_get_functiondef('org_cost_centre_current_usage(uuid,uuid)'::regprocedure) AS c) s`).Scan(&lines); err != nil || !lines {
		t.Fatal("upgraded usage sources lost a line or miss the plan keys", err)
	}
	// The EMP-TRIP-008 receipts replay unchanged on the upgraded schema.
	if again, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, assignInput)); err != nil || !replayed || again.Revision != 2 || again.State != tripdomain.MissionAssigned {
		t.Fatal("a pre-upgrade assignment does not replay", err, again)
	}
	// Plans work on the upgraded schema with the pre-existing missions.
	grantMissionTables(t, m.legalFixtureState)
	plan := cargoPlan(centre)
	if ack, err := m.plan(t, draft, 1, plan); err != nil || ack.State != tripdomain.MissionPlanned {
		t.Fatal("plan of a pre-upgrade draft", err)
	}
	if ack, err := m.plan(t, assigned, 2, plan); err != nil || ack.Revision != 3 {
		t.Fatal("re-plan of a pre-upgrade assignment", err)
	}
	_, err = m.plan(t, accepted, 3, plan)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	_, err = m.plan(t, cancelled, 2, plan)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	if usage() != beforeUsage {
		t.Fatal("a plan changed a usage answer")
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 231 rehearsal", err)
	}
}
