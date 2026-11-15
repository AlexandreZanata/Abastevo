//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// tripLifecycleState is the EMP-TRIP-003 event fixture with the real trip.pause,
// trip.resume, trip.cancel and trip.submit owners on one intake (EMP-TRIP-011).
type tripLifecycleState struct {
	eventState
	lifecycle map[string]syncapp.Handler
	submit    syncapp.Handler
	intake    *syncpg.Store
}

func tripLifecycleIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := eventIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: tripapp.SubmitOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup,
		protocol.Rejected: syncapp.NoAsyncFollowup, protocol.AcceptedForReview: syncapp.AtomicReview}
	return config
}

func tripLifecycleFixture(t *testing.T) tripLifecycleState {
	t.Helper()
	e := eventFixture(t)
	tripReviewGrants(t, e.f)
	x := tripLifecycleState{eventState: e, lifecycle: map[string]syncapp.Handler{}}
	for _, kind := range tripapp.LifecycleOperationTypes {
		handler, err := trippg.NewTransitionHandler(kind, e.authority, e.meters, e.store, e.store)
		if err != nil {
			t.Fatal(err)
		}
		x.lifecycle[kind] = handler
	}
	var err error
	if x.submit, err = trippg.NewSubmitHandler(e.authority, e.meters, e.store, e.store); err != nil {
		t.Fatal(err)
	}
	x.intake = syncpg.NewStore(tripLifecycleIntakeConfig(e.syncFixtureState))
	return x
}

func (x tripLifecycleState) handler(op protocol.Operation) syncapp.Handler {
	switch kind := op.Snapshot().Type; kind {
	case tripapp.StartOperationType:
		return x.start
	case tripapp.FinishOperationType:
		return x.finish
	case tripapp.EventOperationType:
		return x.event
	case tripapp.SubmitOperationType:
		return x.submit
	default:
		return x.lifecycle[kind]
	}
}

func (x tripLifecycleState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.handler(op))
}

// transitionOp is member d's lifecycle command on trip at the expected revision.
func (x tripLifecycleState) transitionOp(t *testing.T, d syncFixtureState, kind, trip string, expected any, declared time.Time, reason any, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return x.operation(t, d, kind, trip, declared, map[string]any{"reason": reason}, func(raw, payload map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": trip, "expected_revision": expected}
		if edit != nil {
			edit(raw, payload)
		}
	})
}

type tripLifecycleCounts struct{ transitions, held, items, effects, claims, receipts, audits int }

func (x tripLifecycleState) lifecycleCount(t *testing.T, op protocol.Operation) tripLifecycleCounts {
	t.Helper()
	var c tripLifecycleCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_transitions WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_held_transitions WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.transitions, &c.held, &c.items, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x tripLifecycleState) applied(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, revision int64) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != revision || ack.Code() != "" || len(ack.Warnings()) != 0 {
		t.Fatal("legitimate lifecycle command was not accepted at its revision", revision, err, ack.Result(), ack.Code())
	}
	if c := x.lifecycleCount(t, op); c != (tripLifecycleCounts{transitions: 1, effects: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("accepted lifecycle command not stored exactly once: %+v", c)
	}
}

func (x tripLifecycleState) heldCommand(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, code string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != code || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("expected a held lifecycle receipt", code, err, ack.Result(), ack.Code())
	}
	if c := x.lifecycleCount(t, op); c != (tripLifecycleCounts{held: 1, items: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("held lifecycle command not kept exactly once: %+v", c)
	}
}

func (x tripLifecycleState) refusedCommand(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	if c := x.lifecycleCount(t, op); c != (tripLifecycleCounts{}) {
		t.Fatalf("refused lifecycle command left state: %+v", c)
	}
}

func (x tripLifecycleState) malformedCommand(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.lifecycleCount(t, op); c != (tripLifecycleCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("malformed lifecycle command stored a fact: %+v", c)
	}
}

func (x tripLifecycleState) pendingCommand(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
	}
	if c := x.lifecycleCount(t, op); c != (tripLifecycleCounts{}) {
		t.Fatalf("pending lifecycle command left state: %+v", c)
	}
}

func (x tripLifecycleState) replaysAck(t *testing.T, d syncFixtureState, op protocol.Operation, ack protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK byte for byte", err)
		}
	}
}

func (x tripLifecycleState) stateOf(t *testing.T, trip string) string {
	t.Helper()
	var state string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_trips WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), trip).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

// started is member d's accepted start on the vehicle at the given instant.
func (x tripLifecycleState) started(t *testing.T, d syncFixtureState, at time.Time) string {
	t.Helper()
	op := x.startOp(t, d, at, nil)
	ack, err := x.process(t, d, op)
	x.eventState.tripState.accepted(t, op, ack, err, 1, 1)
	return tripOf(op)
}

// BUC-EMP-197: one trip paused for lunch, resumed and finished keeps one
// identity, a revision per command, every fact with its actor and time, its
// asset while paused and its full meter evidence; replays and identifier
// reuse follow BR-OFFL-004.
func TestTripLifecyclePausesResumesFinishesAndReplays(t *testing.T) {
	x := tripLifecycleFixture(t)
	d := x.syncFixtureState
	at := x.declaredNow(t)
	trip := x.started(t, d, at)
	org := x.scope.OrgID().String()
	pause := x.transitionOp(t, d, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), "Almoço", nil)
	ack, err := x.process(t, d, pause)
	x.applied(t, pause, ack, err, 2)
	if state := x.stateOf(t, trip); state != "PAUSED" {
		t.Fatal("pause did not move the trip", state)
	}
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT x.revision=2 AND x.command='PAUSE' AND x.from_state='IN_PROGRESS' AND x.to_state='PAUSED' AND x.reason='Almoço'
 AND x.author_membership_id=$3 AND x.author_employee_id=$4 AND x.enrollment_id=$5 AND x.payload_digest=$6 AND x.entry_zone='America/Cuiaba' AND x.device_offset_seconds=-14400
 AND x.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=x.org_id AND a.operation_id=x.operation_id)
 AND EXISTS(SELECT 1 FROM org_audit_events a WHERE a.org_id=x.org_id AND a.operation_id=x.operation_id)
 FROM org_trip_transitions x WHERE x.org_id=$1 AND x.trip_id=$2`, org, trip, d.source.MembershipID, d.person.employee, d.source.EnrollmentID, pause.PayloadDigest()).Scan(&exact); err != nil || !exact {
		t.Fatal("stored lifecycle row is not the exact pause fact", err)
	}
	x.replaysAck(t, d, pause, ack)
	reused := x.transitionOp(t, d, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), "Almoço longo", func(raw, _ map[string]any) { raw["operation_id"] = pause.ID() })
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.applied(t, pause, ack, nil, 2)
	// B-BR-EMP-392: a paused trip still holds its asset and stays open work.
	if kinds := x.openWork(t); len(kinds) != 1 || kinds[0] != fa.OpenTrip {
		t.Fatal("a paused trip is not open work", kinds)
	}
	if ids := x.activeAt(t, at.Add(2*time.Second)); len(ids) != 1 || ids[0] != trip {
		t.Fatal("a paused trip released its asset", ids)
	}
	busy := x.startOp(t, d, at.Add(2*time.Second), nil)
	busyAck, err := x.process(t, d, busy)
	x.eventState.tripState.held(t, busy, busyAck, err, tripapp.AssetBusy)
	// A paused trip still takes logbook events.
	stop := x.eventOp(t, d, trip, "STOP", "Restaurante", at.Add(3*time.Second), nil)
	stopAck, err := x.process(t, d, stop)
	x.eventState.accepted(t, stop, stopAck, err)
	resume := x.transitionOp(t, d, tripapp.ResumeOperationType, trip, 2, at.Add(4*time.Second), nil, nil)
	ack, err = x.process(t, d, resume)
	x.applied(t, resume, ack, err, 3)
	if state := x.stateOf(t, trip); state != "IN_PROGRESS" {
		t.Fatal("resume did not move the trip", state)
	}
	end := x.finishOp(t, d, trip, at.Add(5*time.Second), nil)
	endAck, err := x.process(t, d, end)
	if err != nil || endAck.Result() != protocol.Accepted || *endAck.Snapshot().EntityRevision != 4 {
		t.Fatal("the finish after a pause is not acknowledged at the trip revision", err, endAck.Result(), endAck.Code())
	}
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT t.state='COMPLETED' AND r2.value-r1.value=290000
 AND(SELECT count(*) FROM org_trip_transitions x WHERE x.org_id=t.org_id AND x.trip_id=t.id)=2 AND(SELECT count(*) FROM org_trips o WHERE o.org_id=t.org_id AND o.id=t.id)=1
 FROM org_trips t JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id) JOIN org_meter_readings r1 ON(r1.org_id,r1.id)=(t.org_id,t.reading_id)
 JOIN org_meter_readings r2 ON(r2.org_id,r2.id)=(f.org_id,f.reading_id) WHERE t.org_id=$1 AND t.id=$2`, org, trip).Scan(&exact); err != nil || !exact {
		t.Fatal("the trip lost its identity or its meter evidence across pause and resume", err)
	}
	x.replaysAck(t, d, end, endAck)
	// The timeline keeps every fact with its actor and original time.
	var entries []tripapp.TimelineEntry
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		entries, err = x.events.TimelineTx(t.Context(), tx, x.scope, trip)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range entries {
		kinds = append(kinds, e.FactType)
		if e.AuthorMembershipID != d.source.MembershipID || e.ReceivedAt.IsZero() {
			t.Fatal("a timeline fact lost its actor or receipt", e.FactType)
		}
	}
	if want := []string{"START", "PAUSE", "STOP", "RESUME", "FINISH"}; len(kinds) != len(want) || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] || kinds[3] != want[3] || kinds[4] != want[4] {
		t.Fatal("timeline order", kinds)
	}
}

// BUC-EMP-197: a cancellation keeps every prior fact, fabricates no distance,
// is terminal and releases the asset; late facts follow the closed trip.
func TestTripLifecycleCancellationReleasesTheAssetAndKeepsFacts(t *testing.T) {
	x := tripLifecycleFixture(t)
	d := x.syncFixtureState
	at := x.declaredNow(t)
	trip := x.started(t, d, at)
	org := x.scope.OrgID().String()
	note := x.eventOp(t, d, trip, "NOTE", "Cliente ausente", at.Add(time.Second), nil)
	noteAck, err := x.process(t, d, note)
	x.eventState.accepted(t, note, noteAck, err)
	cancel := x.transitionOp(t, d, tripapp.CancelOperationType, trip, 1, at.Add(2*time.Second), "Cliente cancelou", nil)
	ack, err := x.process(t, d, cancel)
	x.applied(t, cancel, ack, err, 2)
	var kept bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT t.state='CANCELLED' AND t.reading_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM org_trip_finishes f WHERE f.org_id=t.org_id AND f.trip_id=t.id)
 AND NOT EXISTS(SELECT 1 FROM org_meter_readings r WHERE r.org_id=t.org_id AND r.origin='TRIP_END') AND EXISTS(SELECT 1 FROM org_trip_events e WHERE e.org_id=t.org_id AND e.trip_id=t.id)
 AND NOT EXISTS(SELECT 1 FROM org_trip_review_entries v WHERE v.org_id=t.org_id AND v.trip_id=t.id)
 AND(SELECT reason FROM org_trip_transitions x WHERE x.org_id=t.org_id AND x.trip_id=t.id)='Cliente cancelou'
 FROM org_trips t WHERE t.org_id=$1 AND t.id=$2`, org, trip).Scan(&kept); err != nil || !kept {
		t.Fatal("the cancellation lost a prior fact, fabricated evidence or wrote a review entry", err)
	}
	if kinds := x.openWork(t); len(kinds) != 0 {
		t.Fatal("a cancelled trip is still open work", kinds)
	}
	if ids := x.activeAt(t, at.Add(time.Second)); len(ids) != 1 || ids[0] != trip {
		t.Fatal("a cancelled trip did not hold its asset before the cancellation", ids)
	}
	if ids := x.activeAt(t, at.Add(2*time.Second)); len(ids) != 0 {
		t.Fatal("a cancelled trip still holds its asset at its cancellation", ids)
	}
	// The next start on the vehicle is admitted.
	next := x.started(t, d, at.Add(3*time.Second))
	if next == trip || x.stateOf(t, next) != "IN_PROGRESS" {
		t.Fatal("the vehicle was not free after the cancellation")
	}
	// A late event inside the cancelled trip is a late delivery; after it,
	// the event, a finish and a resume are held TRIP_ALREADY_CLOSED.
	late := x.eventOp(t, d, trip, "STOP", nil, at.Add(2*time.Second), nil)
	lateAck, err := x.process(t, d, late)
	x.eventState.accepted(t, late, lateAck, err)
	after := x.eventOp(t, d, trip, "STOP", nil, at.Add(2*time.Second+time.Nanosecond), nil)
	afterAck, err := x.process(t, d, after)
	x.eventState.held(t, after, afterAck, err)
	end := x.finishOp(t, d, trip, at.Add(4*time.Second), nil)
	endAck, err := x.process(t, d, end)
	x.eventState.tripState.held(t, end, endAck, err, tripapp.TripAlreadyClosed)
	resume := x.transitionOp(t, d, tripapp.ResumeOperationType, trip, 2, at.Add(4*time.Second), nil, nil)
	resumeAck, err := x.process(t, d, resume)
	x.heldCommand(t, resume, resumeAck, err, tripapp.TripAlreadyClosed)
	x.replaysAck(t, d, resume, resumeAck)
	// A cancelled trip is never submitted.
	submit := x.operation(t, d, tripapp.SubmitOperationType, trip, at.Add(5*time.Second), map[string]any{}, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.ReviewEntityType, "id": trip, "expected_revision": 1}
	})
	submitAck, err := x.process(t, d, submit)
	if err != nil || submitAck.Result() != protocol.Rejected || submitAck.Code() != "MALFORMED" {
		t.Fatal("a cancelled trip was submitted or left pending", err, submitAck.Result(), submitAck.Code())
	}
	if x.stateOf(t, trip) != "CANCELLED" {
		t.Fatal("a fact after the cancellation changed the trip")
	}
	x.replaysAck(t, d, cancel, ack)
}

// B-BR-EMP-391 negatives: terminal, stale and duplicated commands are held;
// shape, a missing reason and an instant before the start are MALFORMED;
// another member is FORBIDDEN; an undelivered or another organization's trip
// is a pending dependency that reveals and stores nothing.
func TestTripLifecycleRefusalsAndHoldsAtReceipt(t *testing.T) {
	x := tripLifecycleFixture(t)
	d := x.syncFixtureState
	at := x.declaredNow(t)
	trip := x.started(t, d, at)
	send := func(t *testing.T, kind string, expected any, offset time.Duration, reason any, edit func(envelope, payload map[string]any)) (protocol.Operation, protocol.Acknowledgement, error) {
		t.Helper()
		op := x.transitionOp(t, d, kind, trip, expected, at.Add(offset), reason, edit)
		ack, err := x.process(t, d, op)
		return op, ack, err
	}
	t.Run("missing-cancel-reason-malformed", func(t *testing.T) {
		op, ack, err := send(t, tripapp.CancelOperationType, 1, time.Second, nil, nil)
		x.malformedCommand(t, op, ack, err)
	})
	t.Run("blank-reason-malformed", func(t *testing.T) {
		op, ack, err := send(t, tripapp.CancelOperationType, 1, time.Second, "   ", nil)
		x.malformedCommand(t, op, ack, err)
	})
	t.Run("missing-expected-revision-malformed", func(t *testing.T) {
		op, ack, err := send(t, tripapp.PauseOperationType, nil, time.Second, nil, nil)
		x.malformedCommand(t, op, ack, err)
	})
	t.Run("extra-payload-key-malformed", func(t *testing.T) {
		op, ack, err := send(t, tripapp.PauseOperationType, 1, time.Second, nil, func(_, p map[string]any) { p["state"] = "PAUSED" })
		x.malformedCommand(t, op, ack, err)
	})
	t.Run("before-start-malformed", func(t *testing.T) {
		op, ack, err := send(t, tripapp.PauseOperationType, 1, -time.Nanosecond, nil, nil)
		x.malformedCommand(t, op, ack, err)
	})
	t.Run("resume-of-running-trip-stale", func(t *testing.T) {
		op, ack, err := send(t, tripapp.ResumeOperationType, 1, time.Second, nil, nil)
		x.heldCommand(t, op, ack, err, tripapp.StaleRevision)
	})
	t.Run("unknown-trip-pending", func(t *testing.T) {
		op := x.transitionOp(t, d, tripapp.PauseOperationType, onboardingID(t), 1, at.Add(time.Second), nil, nil)
		ack, err := x.process(t, d, op)
		x.pendingCommand(t, op, ack, err)
	})
	t.Run("another-organizations-trip-pending", func(t *testing.T) {
		foreign := x.foreignTrip(t)
		op := x.transitionOp(t, d, tripapp.CancelOperationType, foreign, 1, at.Add(time.Second), "Cliente cancelou", nil)
		ack, err := x.process(t, d, op)
		x.pendingCommand(t, op, ack, err)
		var touched int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_transitions WHERE trip_id=$1)+(SELECT count(*) FROM org_trip_held_transitions WHERE trip_id=$1)
 +(SELECT count(*) FROM org_trips WHERE id=$1 AND state<>'IN_PROGRESS')`, foreign).Scan(&touched); err != nil || touched != 0 {
			t.Fatal("a command reached another organization's trip", touched, err)
		}
	})
	t.Run("another-member-pause-forbidden", func(t *testing.T) {
		other := x.driver(t)
		x.assign(t, other.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		op := x.transitionOp(t, other, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), nil, nil)
		ack, err := x.process(t, other, op)
		x.refusedCommand(t, op, ack, err, orgapi.Forbidden)
	})
	if state := x.stateOf(t, trip); state != "IN_PROGRESS" {
		t.Fatal("a refused command moved the trip", state)
	}
	// Two pause intents on revision 1: the first applies, the duplicate and
	// a later stale one are held, never applied.
	first, ack, err := send(t, tripapp.PauseOperationType, 1, time.Second, nil, nil)
	x.applied(t, first, ack, err, 2)
	t.Run("duplicated-pause-stale", func(t *testing.T) {
		op, ack, err := send(t, tripapp.PauseOperationType, 1, time.Second, "Outra vez", nil)
		x.heldCommand(t, op, ack, err, tripapp.StaleRevision)
		x.replaysAck(t, d, op, ack)
	})
	resume, ack, err := send(t, tripapp.ResumeOperationType, 2, 2*time.Second, nil, nil)
	x.applied(t, resume, ack, err, 3)
	t.Run("stale-cancel-held", func(t *testing.T) {
		op, ack, err := send(t, tripapp.CancelOperationType, 2, 3*time.Second, "Cliente cancelou", nil)
		x.heldCommand(t, op, ack, err, tripapp.StaleRevision)
	})
	t.Run("stale-before-start-held", func(t *testing.T) {
		op, ack, err := send(t, tripapp.PauseOperationType, 1, -time.Second, nil, nil)
		x.heldCommand(t, op, ack, err, tripapp.StaleRevision)
	})
	end := x.finishOp(t, d, trip, at.Add(4*time.Second), nil)
	endAck, err := x.process(t, d, end)
	if err != nil || endAck.Result() != protocol.Accepted || *endAck.Snapshot().EntityRevision != 4 {
		t.Fatal("finish", err, endAck.Code())
	}
	t.Run("resume-after-terminal-held", func(t *testing.T) {
		op, ack, err := send(t, tripapp.ResumeOperationType, 4, 5*time.Second, nil, nil)
		x.heldCommand(t, op, ack, err, tripapp.TripAlreadyClosed)
	})
	t.Run("cancel-after-terminal-stale-revision-held-closed", func(t *testing.T) {
		op, ack, err := send(t, tripapp.CancelOperationType, 1, 5*time.Second, "Cliente cancelou", nil)
		x.heldCommand(t, op, ack, err, tripapp.TripAlreadyClosed)
	})
	if state := x.stateOf(t, trip); state != "COMPLETED" {
		t.Fatal("a held command moved the trip", state)
	}
}

// commandBarrier holds one operation's write admission inside its intake
// transaction until the other command is observed waiting on a lock.
type commandBarrier struct {
	syncapp.Handler
	winner          string
	inside, proceed chan struct{}
	once            *sync.Once
}

func (h commandBarrier) ReviewProof() syncapp.ReviewProof {
	return h.Handler.(syncapp.ReviewProofProvider).ReviewProof()
}

func (h commandBarrier) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	if op.ID() == h.winner && mode == syncapp.Write {
		h.once.Do(func() {
			close(h.inside)
			<-h.proceed
		})
	}
	return h.Handler.Admit(ctx, db, source, op, mode)
}

// 08-state-model §4.7: a pause and a cancellation decided on revision 1 race;
// whichever takes the locks first applies and the other is held (a stale
// revision, or a trip already closed by the cancellation), in both orders,
// with the loser observed waiting on the winner's lock.
func TestTripLifecycleConcurrentCommandsResolveToOne(t *testing.T) {
	for _, order := range []string{"pause-wins", "cancel-wins"} {
		t.Run(order, func(t *testing.T) {
			x := tripLifecycleFixture(t)
			d := x.syncFixtureState
			at := x.declaredNow(t)
			trip := x.started(t, d, at)
			pause := x.transitionOp(t, d, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), nil, nil)
			cancel := x.transitionOp(t, d, tripapp.CancelOperationType, trip, 1, at.Add(time.Second), "Cliente cancelou", nil)
			winner, loser := pause, cancel
			if order == "cancel-wins" {
				winner, loser = cancel, pause
			}
			config := tripLifecycleIntakeConfig(x.syncFixtureState)
			config.Pool = x.f.RuntimePool(t, 4)
			store := syncpg.NewStore(config)
			barrier := sync.Once{}
			inside, proceed := make(chan struct{}), make(chan struct{})
			wrap := func(op protocol.Operation) syncapp.Handler {
				return commandBarrier{Handler: x.handler(op), winner: winner.ID(), inside: inside, proceed: proceed, once: &barrier}
			}
			acks := map[string]protocol.Acknowledgement{}
			errs := map[string]error{}
			var mu sync.Mutex
			var done sync.WaitGroup
			run := func(op protocol.Operation) {
				done.Go(func() {
					ack, err := store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), wrap(op))
					mu.Lock()
					acks[op.ID()], errs[op.ID()] = ack, err
					mu.Unlock()
				})
			}
			run(winner)
			select {
			case <-inside:
			case <-time.After(30 * time.Second):
				close(proceed)
				done.Wait()
				t.Fatal("the winning command never reached its admission", errs)
			}
			run(loser)
			waited := false
			for deadline := time.Now().Add(10 * time.Second); !waited && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				var waiting int
				if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				waited = waiting > 0
			}
			close(proceed)
			done.Wait()
			if !waited {
				t.Fatal("the losing command never waited on the winner's lock")
			}
			// The losing pause meets a closed trip (TRIP_ALREADY_CLOSED comes
			// first); the losing cancellation a stale revision.
			want, code := "PAUSED", tripapp.StaleRevision
			if order == "cancel-wins" {
				want, code = "CANCELLED", tripapp.TripAlreadyClosed
			}
			x.applied(t, winner, acks[winner.ID()], errs[winner.ID()], 2)
			x.heldCommand(t, loser, acks[loser.ID()], errs[loser.ID()], code)
			if state := x.stateOf(t, trip); state != want {
				t.Fatal("the race left an inconsistent trip", state)
			}
			var rows int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_transitions WHERE org_id=$1 AND trip_id=$2`, x.scope.OrgID().String(), trip).Scan(&rows); err != nil || rows != 1 {
				t.Fatal("two commands applied on one revision", rows, err)
			}
			x.replaysAck(t, d, loser, acks[loser.ID()])
			x.replaysAck(t, d, winner, acks[winner.ID()])
		})
	}
}

// B-BR-EMP-393: forced RLS, insert-only rows and a trip state that moves only
// with the lifecycle row of its next revision.
func TestTripLifecycleSchemaIsForcedRLSGuardedAndInsertOnly(t *testing.T) {
	x := tripLifecycleFixture(t)
	d := x.syncFixtureState
	at := x.declaredNow(t)
	trip := x.started(t, d, at)
	pause := x.transitionOp(t, d, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), nil, nil)
	ack, err := x.process(t, d, pause)
	x.applied(t, pause, ack, err, 2)
	org := x.scope.OrgID().String()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_transitions,org_trip_held_transitions TO "+role+";GRANT UPDATE ON org_trips TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE org_trips SET state='IN_PROGRESS' WHERE id=$1`,
		`UPDATE org_trips SET state='CANCELLED' WHERE id=$1`,
		`UPDATE org_trips SET state='COMPLETED' WHERE id=$1`,
		`UPDATE org_trip_transitions SET reason='Reescrito' WHERE trip_id=$1`,
		`DELETE FROM org_trip_transitions WHERE trip_id=$1`,
		`DELETE FROM org_trip_held_transitions WHERE trip_id=$1`,
		// A direct row with a forged revision or without its operation.
		`INSERT INTO org_trip_transitions(org_id,trip_id,revision,command,from_state,to_state,operation_id,payload_digest,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
 SELECT org_id,trip_id,5,'RESUME','PAUSED','IN_PROGRESS',operation_id,payload_digest,author_membership_id,author_employee_id,enrollment_id,occurred_at,0,entry_zone,device_offset_seconds FROM org_trip_transitions WHERE trip_id=$1`,
		`INSERT INTO org_trip_transitions(org_id,trip_id,revision,command,from_state,to_state,operation_id,payload_digest,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
 SELECT org_id,trip_id,3,'RESUME','PAUSED','IN_PROGRESS',gen_random_uuid(),payload_digest,author_membership_id,author_employee_id,enrollment_id,occurred_at,0,entry_zone,device_offset_seconds FROM org_trip_transitions WHERE trip_id=$1`,
		`INSERT INTO org_trip_transitions(org_id,trip_id,revision,command,from_state,to_state,operation_id,payload_digest,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
 SELECT org_id,trip_id,3,'CANCEL','PAUSED','CANCELLED',operation_id,payload_digest,author_membership_id,author_employee_id,enrollment_id,occurred_at,0,entry_zone,device_offset_seconds FROM org_trip_transitions WHERE trip_id=$1`,
	} {
		err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), statement, trip)
			return err
		})
		if err == nil {
			t.Fatal("trip lifecycle history accepted a direct change", statement)
		}
	}
	if state := x.stateOf(t, trip); state != "PAUSED" {
		t.Fatal("a direct change moved the trip", state)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_transitions`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("lifecycle rows visible without an organization setting", visible, err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE relname IN('org_trip_transitions','org_trip_held_transitions')`).Scan(&forced); err != nil || !forced {
		t.Fatal("lifecycle tables are not forced-RLS", err)
	}
	var revision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT org_trip_revision($1,$2)`, org, trip).Scan(&revision); err != nil || revision != 2 {
		t.Fatal("the database revision of the paused trip", revision, err)
	}
}
