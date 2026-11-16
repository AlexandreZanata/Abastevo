//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// A populated head before 000256 (the 000255 head: a mission planned without
// stops, accepted and executed by a linked trip with logbook events, a DRAFT
// mission and an unlinked trip, all through the real stores and signed
// intake) upgrades to 000256 by an ordinary non-superuser migrator: an
// injected failure leaves schema, functions, constraints and data untouched,
// recovery applies once, replay is a no-op, the previous embedded image stays
// compatible, no existing function changes (the usage sources keep their
// bodies), earlier receipts replay, the pre-upgrade plan reads without stops
// and the pre-upgrade trip reads its plan and observations; planned stops
// then work on the upgraded schema (EMP-TRIP-010).
func TestTripStopsPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= tripStopsMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, head := range []string{"000255_organization_trip_checklist_gate.sql", missionReservationLinkMigration, missionPlanMigration} {
		if _, ok := previous[head]; !ok {
			t.Fatal("previous image does not contain", head)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	e := eventFixtureOn(t, tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal)))))
	m := missionFixtureOn(t, e.tripState)
	handoverGrants(t, f)
	d := e.syncFixtureState
	org := e.scope.OrgID().String()
	// The executed mission: planned without stops, accepted, linked, observed.
	executed := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	planOp := onboardingID(t)
	planInput := tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: planOp, MissionID: executed, ExpectedRevision: 1, Plan: stopsPlan(nil)}
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, planInput)); err != nil {
		t.Fatal("plan before the upgrade", err)
	}
	if _, err := m.assignTo(t, executed, 2, d.person.employee); err != nil {
		t.Fatal(err)
	}
	if ack, err := m.deliver(t, d, m.respondOp(t, d, executed, 3, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("acceptance before the upgrade", err)
	}
	start := m.missionStartOp(t, d, executed, e.declaredNow(t))
	ack, err := e.process(t, d, start)
	e.tripState.accepted(t, start, ack, err, 1, 1)
	trip := tripOf(start)
	arrival := e.eventOp(t, d, trip, "ARRIVAL", nil, e.declaredNow(t), nil)
	ack, err = e.process(t, d, arrival)
	e.accepted(t, arrival, ack, err)
	draft := m.create(t, m.unit, "").MissionID
	snapshot := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_trip_missions x),
(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,mission_id,revision) FROM org_trip_mission_transitions x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_trip_mission_commands x),
(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,mission_id,revision) FROM org_trip_mission_plans x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,trip_id) FROM org_trip_mission_links x),
(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_trip_events x),
(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	kept := []string{"org_unit_current_usage(uuid,uuid)", "org_cost_centre_current_usage(uuid,uuid)", "org_asset_current_usage(uuid,uuid)", "org_trip_mission_plan_source()",
		"org_trip_mission_guard()", "org_trip_mission_complete()", "org_trip_mission_plan_text_valid(text)", "org_trip_keep_history()"}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(to_regprocedure(p)),E'\n' ORDER BY p) FROM unnest($1::text[]) p`, kept).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	constraints := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(conname||'='||pg_get_constraintdef(oid),E'\n' ORDER BY conname) FROM pg_constraint WHERE conrelid IN('org_trip_mission_plans'::regclass,'org_trip_mission_commands'::regclass,'org_trip_missions'::regclass)`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeFunctions, beforeConstraints := snapshot(), functions(), constraints()
	migrator := f.RuntimeRole + "_stops_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+
		";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON org_trip_mission_plans TO "+role); err != nil {
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
	raw, err := fs.ReadFile(migrations.Files, tripStopsMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(tripStopsMigration, "_", 2)[0]
	broken := fstest.MapFS{tripStopsMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected stops migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_mission_plan_stops') IS NULL AND to_regprocedure('org_trip_mission_plan_stop_source()') IS NULL
 AND to_regprocedure('org_trip_mission_plan_stop_published()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean ||
		before != snapshot() || beforeFunctions != functions() || beforeConstraints != constraints() {
		t.Fatal("failed stops migration changed the original schema, functions, constraints or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[tripStopsMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary stops migration recovery", applied, err)
	}
	if before != snapshot() || beforeFunctions != functions() || beforeConstraints != constraints() {
		t.Fatal("the stops upgrade rewrote existing state, a function or a constraint")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("stops migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid='org_trip_mission_plan_stops'::regclass)FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the stop table is not owned by the ordinary forced-RLS migrator", err)
	}
	grantMissionTables(t, m.legalFixtureState)
	// The pre-upgrade plan receipt replays and reads without stops.
	if again, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, planInput)); err != nil || !replayed || again.Revision != 2 {
		t.Fatal("a pre-upgrade plan does not replay", err, again)
	}
	view, err := m.missions.ReadMission(t.Context(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, executed)
	if err != nil || view.Plan == nil || view.Plan.Fields.Stops != nil {
		t.Fatal("a pre-upgrade plan reads with stops", err)
	}
	// The pre-upgrade trip reads its plan and observations.
	stops := &trippg.StopsStore{Pool: f.Runtime, Authority: tripHandoverStoreOf(t, m.legalFixtureState, m.assets).Authority}
	r := deviceReviewer(d)
	record, err := stops.ReadOwnTripStops(t.Context(), e.scope, r.actor, r.member, trip)
	if err != nil {
		t.Fatal("pre-upgrade trip read", err)
	}
	got, err := tripapp.BuildTripStops(record)
	if err != nil || got.Planned == nil || got.Planned.MissionID != executed || len(got.Planned.Stops) != 0 || len(got.Planned.Legs) != 1 ||
		got.Planned.Legs[0].To.Kind != tripdomain.PointDestination || len(got.Stops) != 1 || got.Stops[0].Arrival.FactID != eventOf(arrival) {
		t.Fatal("pre-upgrade trip planned and observed sides", err)
	}
	// Planned stops work on a pre-upgrade DRAFT mission.
	if ack, err := m.plan(t, draft, 1, stopsPlan(bucStops)); err != nil || ack.Revision != 2 {
		t.Fatal("planned stops on the upgraded schema", err)
	}
	var stored []string
	if err := f.Admin.QueryRow(t.Context(), `SELECT array_agg(place_text ORDER BY sequence) FROM org_trip_mission_plan_stops WHERE org_id=$1 AND mission_id=$2 AND revision=2`, org, draft).Scan(&stored); err != nil ||
		!reflect.DeepEqual(stored, bucStops) {
		t.Fatal("stops on the upgraded schema", stored, err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 256 rehearsal", err)
	}
}
