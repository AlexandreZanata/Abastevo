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
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const tripHandoverMigration = "000243_organization_trip_handovers.sql"

// A populated 000237 head (a completed trip, a paused trip with a logbook
// event, a held busy start, all through the real signed intake) upgrades to
// 000243 by an ordinary non-superuser migrator: an injected failure leaves
// schema, functions, constraints, indexes and data untouched, recovery
// applies once, replay is a no-op, the previous embedded image stays
// compatible, every redefined body keeps the lines of its latest version,
// every pre-upgrade receipt replays byte for byte and every revision is
// unchanged; the paused trip is then handed over and its receiver resumes
// and finishes it (EMP-TRIP-012, B-BR-EMP-396, EDR-255).
func TestTripHandoverPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= tripHandoverMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	for _, name := range []string{"000235_organization_trip_closures.sql", "000236_organization_trip_membership_end.sql", "000237_organization_trip_conflict_resolutions.sql"} {
		if _, ok := previous[name]; !ok {
			t.Fatal("previous image does not contain", name)
		}
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	e := eventFixtureOn(t, tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal)))))
	if !e.store.PreHandoverImage || e.store.PreLifecycleImage {
		t.Fatal("the rehearsal store must know the image has lifecycle rows and predates the handovers")
	}
	tripReviewGrants(t, f)
	x := tripLifecycleState{eventState: e, lifecycle: map[string]syncapp.Handler{}}
	for _, kind := range tripapp.LifecycleOperationTypes {
		handler, err := trippg.NewTransitionHandler(kind, e.authority, e.meters, e.store, e.store)
		if err != nil {
			t.Fatal(err)
		}
		x.lifecycle[kind] = handler
	}
	if x.submit, err = trippg.NewSubmitHandler(e.authority, e.meters, e.store, e.store); err != nil {
		t.Fatal(err)
	}
	x.intake = syncpg.NewStore(tripLifecycleIntakeConfig(e.syncFixtureState))
	d := x.syncFixtureState
	completedAt := x.declaredNow(t)
	first := x.startOp(t, d, completedAt, absentReading)
	firstAck, err := x.process(t, d, first)
	x.eventState.tripState.accepted(t, first, firstAck, err, 1, 0)
	end := x.finishOp(t, d, tripOf(first), completedAt.Add(time.Microsecond), absentReading)
	endAck, err := x.process(t, d, end)
	x.eventState.tripState.accepted(t, end, endAck, err, 2, 0)
	ongoingAt := x.declaredNow(t)
	ongoing := x.startOp(t, d, ongoingAt, absentReading)
	ongoingAck, err := x.process(t, d, ongoing)
	x.eventState.tripState.accepted(t, ongoing, ongoingAck, err, 1, 0)
	trip := tripOf(ongoing)
	note := x.eventOp(t, d, trip, "NOTE", "Antes da troca", ongoingAt.Add(time.Second), nil)
	noteAck, err := x.process(t, d, note)
	x.eventState.accepted(t, note, noteAck, err)
	pause := x.transitionOp(t, d, tripapp.PauseOperationType, trip, 1, ongoingAt.Add(2*time.Second), "Almoço", nil)
	pauseAck, err := x.process(t, d, pause)
	x.applied(t, pause, pauseAck, err, 2)
	busy := x.startOp(t, d, ongoingAt.Add(3*time.Second), absentReading)
	busyAck, err := x.process(t, d, busy)
	x.eventState.tripState.held(t, busy, busyAck, err, tripapp.AssetBusy)
	org := x.scope.OrgID().String()
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trips x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id)FROM org_trip_finishes x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_trip_events x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,trip_id,revision)FROM org_trip_transitions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,review_item_id)FROM org_trip_held_starts x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_sync_review_items x),
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
	revisions := func() string {
		t.Helper()
		var v string
		if err := f.Admin.QueryRow(t.Context(), `SELECT string_agg(id::text||'='||org_trip_revision(org_id,id),',' ORDER BY id) FROM org_trips WHERE org_id=$1`, org).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	replaced := []string{"org_trip_revision(uuid,uuid)", "org_trip_transition_source()", "org_trip_held_transition_source()", "org_trip_finish_source()", "org_trip_held_source()",
		"org_trip_event_source()", "org_trip_held_event_source()", "org_trip_held_submission_source()", "org_trip_review_entry_source()", "org_incident_report_source()",
		"org_offboarding_guard()", "org_trip_closure_source()"}
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
	before, beforeUsage, beforeFunctions, beforeShape, beforeRevisions := snapshot(), usage(), functions(), shape(), revisions()
	migrator := f.RuntimeRole + "_handover_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	grants := "CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO " + role + ";GRANT SELECT,INSERT ON schema_migrations TO " + role +
		";GRANT REFERENCES ON org_trips,org_memberships,org_employee_identities,org_audit_events TO " + role
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
	raw, err := fs.ReadFile(migrations.Files, tripHandoverMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(tripHandoverMigration, "_", 2)[0]
	broken := fstest.MapFS{tripHandoverMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected handover migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_handovers') IS NULL AND to_regprocedure('org_trip_responsible(uuid,uuid,timestamptz,smallint)') IS NULL AND to_regprocedure('org_trip_responsible_member(uuid,uuid)') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean ||
		before != snapshot() || beforeFunctions != functions() || beforeShape != shape() || beforeRevisions != revisions() {
		t.Fatal("failed handover migration changed the original schema, functions, constraints, indexes or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[tripHandoverMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary handover migration recovery", applied, err)
	}
	tripHandoverGrant(t, f)
	if before != snapshot() || beforeUsage != usage() || beforeRevisions != revisions() {
		t.Fatal("handover upgrade rewrote existing state, a usage answer or a revision")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("handover migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid='org_trip_handovers'::regclass)FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the handover table is not owned by the ordinary forced-RLS migrator", err)
	}
	var lines bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT tr LIKE '%trip transition stale%' AND tr LIKE '%org_trip_lifecycle_next%' AND ht LIKE '%STALE_REVISION%' AND ht LIKE '%TRIP_ALREADY_CLOSED%'
 AND fs LIKE '%trip finish reading unavailable%' AND fs LIKE '%end_day%' AND hs LIKE '%ASSET_UNAVAILABLE%' AND hs LIKE '%held trip finish source unavailable%'
 AND es LIKE '%org_trip_revision_closed%' AND es LIKE '%trip event identity unavailable%' AND he LIKE '%held trip event source unavailable%' AND he LIKE '%end_sub%'
 AND hm LIKE '%held trip submission condition unavailable%' AND re LIKE '%trip reporting period locked%' AND re LIKE '%amendment_person%' AND re LIKE '%org_trip_handovers%'
 AND rv LIKE '%org_trip_finishes%' AND rv LIKE '%org_trip_transitions%' AND rv LIKE '%org_trip_handovers%'
 AND ir LIKE '%incident report allocation snapshot unavailable%' AND ir LIKE '%org_trip_responsible%'
 AND rv LIKE '%org_trip_closures%' AND es LIKE '%org_trip_closures%' AND he LIKE '%closed BOOLEAN%' AND re LIKE '%closer_person%' AND hs LIKE '%ACCESS_ENDED_BEFORE_RECEIPT%'
 AND og LIKE '%TEAM_LINK%' AND og LIKE '%org_trip_responsible_member%' AND cs LIKE '%trip closer unavailable%' AND cs LIKE '%org_trip_responsible%'
 FROM(SELECT pg_get_functiondef('org_trip_transition_source()'::regprocedure) AS tr,pg_get_functiondef('org_trip_held_transition_source()'::regprocedure) AS ht,
 pg_get_functiondef('org_trip_finish_source()'::regprocedure) AS fs,pg_get_functiondef('org_trip_held_source()'::regprocedure) AS hs,
 pg_get_functiondef('org_trip_event_source()'::regprocedure) AS es,pg_get_functiondef('org_trip_held_event_source()'::regprocedure) AS he,
 pg_get_functiondef('org_trip_held_submission_source()'::regprocedure) AS hm,pg_get_functiondef('org_trip_review_entry_source()'::regprocedure) AS re,
 pg_get_functiondef('org_trip_revision(uuid,uuid)'::regprocedure) AS rv,pg_get_functiondef('org_incident_report_source()'::regprocedure) AS ir,
 pg_get_functiondef('org_offboarding_guard()'::regprocedure) AS og,pg_get_functiondef('org_trip_closure_source()'::regprocedure) AS cs) s`).Scan(&lines); err != nil || !lines {
		t.Fatal("a redefined function lost a line of its latest body", err)
	}
	// Every pre-upgrade receipt replays byte for byte.
	for i, op := range []protocol.Operation{first, end, ongoing, note, pause, busy} {
		ack := []protocol.Acknowledgement{firstAck, endAck, ongoingAck, noteAck, pauseAck, busyAck}[i]
		again, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("a pre-upgrade receipt does not replay byte for byte", op.Snapshot().Type, err)
		}
	}
	// The paused pre-upgrade trip is handed over on the new schema; the
	// receiver resumes and finishes it (revision 3 handover, 4 resume, 5 finish).
	h := tripHandoverState{tripLifecycleState: x}
	handoverGrants(t, f)
	h.handovers = tripHandoverStoreOf(t, x.legalFixtureState, x.assets)
	store := &trippg.Store{Allocations: x.assets, NewID: func() (string, error) { return onboardingID(t), nil }}
	for _, kind := range tripapp.LifecycleOperationTypes {
		handler, err := trippg.NewTransitionHandler(kind, x.authority, x.meters, store, store)
		if err != nil {
			t.Fatal(err)
		}
		h.lifecycle[kind] = handler
	}
	h.start, h.finish = x.handlers(t, x.meters, store)
	receiver := h.driver(t)
	h.assign(t, receiver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	sup := h.staff(t, "SUPERVISOR", "ORG", "")
	result, _, err := h.handOver(x.scope, sup, trip, 2, receiver.person.member, "Troca de turno", onboardingID(t))
	if err != nil || result.Revision != 3 || result.Operational != "PAUSED" {
		t.Fatal("handover of a pre-upgrade paused trip", err)
	}
	resume := h.transitionOp(t, receiver, tripapp.ResumeOperationType, trip, 3, result.Handover.HandedOverAt.Add(time.Second), nil, nil)
	ack, err := h.process(t, receiver, resume)
	h.applied(t, resume, ack, err, 4)
	finish := h.finishOp(t, receiver, trip, result.Handover.HandedOverAt.Add(2*time.Second), absentReading)
	ack, err = h.process(t, receiver, finish)
	if err != nil || ack.Result() != protocol.Accepted || *ack.Snapshot().EntityRevision != 5 {
		t.Fatal("the receiver's finish of a pre-upgrade trip", err, ack.Code())
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 243 rehearsal", err)
	}
}
