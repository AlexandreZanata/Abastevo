//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const tripLifecycleMigration = "000233_organization_trip_lifecycle.sql"

// A populated head before 000233 (a completed trip, a trip in progress with a
// logbook event, a held busy start, all through the real signed intake)
// upgrades to 000233 by an ordinary non-superuser migrator: an injected
// failure leaves schema, functions, constraints, indexes and data untouched,
// recovery applies once, replay is a no-op, the previous embedded image stays
// compatible, every pre-upgrade receipt replays byte for byte, the completed
// trip keeps revision 2, and the trip in progress is paused, resumed and
// finished on the upgraded schema (EMP-TRIP-011, B-BR-EMP-393).
func TestTripLifecyclePopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= tripLifecycleMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous[missionVehicleMigration]; !ok {
		t.Fatal("previous image does not contain", missionVehicleMigration)
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := eventFixtureOn(t, tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal)))))
	if !x.store.PreLifecycleImage {
		t.Fatal("the rehearsal store must know the image predates the lifecycle rows")
	}
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	first := x.startOp(t, d, completedAt, nil)
	firstAck, err := x.process(t, d, first)
	x.tripState.accepted(t, first, firstAck, err, 1, 1)
	end := x.finishOp(t, d, tripOf(first), completedAt.Add(time.Microsecond), nil)
	endAck, err := x.process(t, d, end)
	x.tripState.accepted(t, end, endAck, err, 2, 1)
	ongoingAt := x.declaredNow(t)
	ongoing := x.startOp(t, d, ongoingAt, nil)
	ongoingAck, err := x.process(t, d, ongoing)
	x.tripState.accepted(t, ongoing, ongoingAck, err, 1, 1)
	trip := tripOf(ongoing)
	note := x.eventOp(t, d, trip, "NOTE", "Antes da pausa", ongoingAt.Add(time.Second), nil)
	noteAck, err := x.process(t, d, note)
	x.accepted(t, note, noteAck, err)
	busy := x.startOp(t, d, ongoingAt.Add(time.Second), nil)
	busyAck, err := x.process(t, d, busy)
	x.tripState.held(t, busy, busyAck, err, tripapp.AssetBusy)
	org := x.scope.OrgID().String()
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trip_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,review_item_id)FROM org_trip_held_starts x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meter_readings x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_sync_review_items x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
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
		var a string
		if err := tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text`, org, x.vehicle.AssetID).Scan(&a); err != nil {
			t.Fatal("asset usage refused the populated state", err)
		}
		return a
	}
	replaced := []string{"org_trip_completion_guard()", "org_trip_finish_source()", "org_trip_finish_apply()", "org_trip_complete()", "org_trip_held_source()",
		"org_trip_event_source()", "org_trip_held_event_source()", "org_trip_mission_vehicle_source()", "org_sync_review_source_valid(uuid,uuid,text,bigint)"}
	functions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(to_regprocedure(p)),E'\n' ORDER BY p) FROM unnest($1::text[]) p`, replaced).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	shape := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT(SELECT string_agg(conname||'='||pg_get_constraintdef(oid),E'\n' ORDER BY conname) FROM pg_constraint WHERE conrelid='org_trips'::regclass)
 ||(SELECT string_agg(indexname||'='||indexdef,E'\n' ORDER BY indexname) FROM pg_indexes WHERE tablename='org_trips')`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before, beforeUsage, beforeFunctions, beforeShape := snapshot(), usage(), functions(), shape()
	migrator := f.RuntimeRole + "_lifecycle_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	grants := "CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO " + role + ";GRANT SELECT,INSERT ON schema_migrations TO " + role +
		";GRANT REFERENCES ON org_sync_operations,org_memberships,org_employee_identities,org_device_enrollments,org_sync_review_items TO " + role +
		";ALTER TABLE org_trips OWNER TO " + role
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
	raw, err := fs.ReadFile(migrations.Files, tripLifecycleMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(tripLifecycleMigration, "_", 2)[0]
	broken := fstest.MapFS{tripLifecycleMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected lifecycle migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_transitions') IS NULL AND to_regclass('org_trip_held_transitions') IS NULL AND to_regprocedure('org_trip_revision(uuid,uuid)') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean ||
		before != snapshot() || beforeFunctions != functions() || beforeShape != shape() {
		t.Fatal("failed lifecycle migration changed the original schema, functions, constraints, indexes or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[tripLifecycleMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary lifecycle migration recovery", applied, err)
	}
	if before != snapshot() || beforeUsage != usage() {
		t.Fatal("lifecycle upgrade rewrote existing state or a usage answer")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("lifecycle migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1))
 FROM pg_class WHERE oid IN('org_trip_transitions'::regclass,'org_trip_held_transitions'::regclass))FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the lifecycle tables are not owned by the ordinary forced-RLS migrator", err)
	}
	var lines bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT r LIKE '%org_trip_held_submissions%' AND r LIKE '%org_trip_held_events%' AND r LIKE '%org_meter_held_readings%' AND r LIKE '%org_trip_held_transitions%'
 AND v LIKE '%mission vehicle in use%' AND v LIKE '%mission trip in progress%' AND v LIKE '%IMPLEMENT%' AND e LIKE '%org_trip_revision_closed%' AND e LIKE '%trip event identity unavailable%'
 AND h LIKE '%ASSET_UNAVAILABLE%' AND h LIKE '%held trip finish source unavailable%'
 FROM(SELECT pg_get_functiondef('org_sync_review_source_valid(uuid,uuid,text,bigint)'::regprocedure) AS r,pg_get_functiondef('org_trip_mission_vehicle_source()'::regprocedure) AS v,
 pg_get_functiondef('org_trip_event_source()'::regprocedure) AS e,pg_get_functiondef('org_trip_held_source()'::regprocedure) AS h) s`).Scan(&lines); err != nil || !lines {
		t.Fatal("a redefined function lost a line of its latest body", err)
	}
	// Every pre-upgrade receipt replays; the completed trip stays revision 2.
	for i, op := range []protocol.Operation{first, end, ongoing, note, busy} {
		ack := []protocol.Acknowledgement{firstAck, endAck, ongoingAck, noteAck, busyAck}[i]
		again, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("a pre-upgrade receipt does not replay byte for byte", op.Snapshot().Type, err)
		}
	}
	var revision int64
	if err := f.Admin.QueryRow(t.Context(), `SELECT org_trip_revision($1,$2)`, org, tripOf(first)).Scan(&revision); err != nil || revision != 2 {
		t.Fatal("a pre-upgrade completed trip changed its revision", revision, err)
	}
	// The trip in progress is paused, resumed and finished on the new schema.
	tripGrants(t, x.legalFixtureState)
	store := &trippg.Store{Allocations: x.assets, NewID: func() (string, error) { return onboardingID(t), nil }, PreHandoverImage: !handoverImage(t, x.f)}
	lifecycle := tripLifecycleState{eventState: x, lifecycle: map[string]syncapp.Handler{}}
	for _, kind := range tripapp.LifecycleOperationTypes {
		handler, err := trippg.NewTransitionHandler(kind, x.authority, x.meters, store, store)
		if err != nil {
			t.Fatal(err)
		}
		lifecycle.lifecycle[kind] = handler
	}
	lifecycle.start, lifecycle.finish = x.handlers(t, x.meters, store)
	lifecycle.intake = x.trips
	pause := lifecycle.transitionOp(t, d, tripapp.PauseOperationType, trip, 1, ongoingAt.Add(2*time.Second), "Almoço", nil)
	ack, err := lifecycle.process(t, d, pause)
	lifecycle.applied(t, pause, ack, err, 2)
	resume := lifecycle.transitionOp(t, d, tripapp.ResumeOperationType, trip, 2, ongoingAt.Add(3*time.Second), nil, nil)
	ack, err = lifecycle.process(t, d, resume)
	lifecycle.applied(t, resume, ack, err, 3)
	finish := x.finishOp(t, d, trip, ongoingAt.Add(4*time.Second), nil)
	ack, err = lifecycle.process(t, d, finish)
	if err != nil || ack.Result() != protocol.Accepted || *ack.Snapshot().EntityRevision != 4 {
		t.Fatal("finish of a pre-upgrade trip after a pause", err, ack.Code())
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 233 rehearsal", err)
	}
}
