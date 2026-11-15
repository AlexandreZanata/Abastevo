//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// eventState is the trip fixture plus the real trip.event.record owner
// (EMP-TRIP-003).
type eventState struct {
	tripState
	events *trippg.EventStore
	event  syncapp.Handler
}

func eventGrants(t *testing.T, x tripState) {
	t.Helper()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	// EMP-TRIP-015: the owner and the logbook guard read the review history.
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events TO "+role+";GRANT SELECT ON org_trip_review_entries TO "+role); err != nil {
		t.Fatal(err)
	}
}

func eventIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := tripIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: tripapp.EventOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup,
		protocol.Rejected: syncapp.NoAsyncFollowup, protocol.AcceptedForReview: syncapp.AtomicReview}
	return config
}

func eventFixtureOn(t *testing.T, x tripState) eventState {
	t.Helper()
	eventGrants(t, x)
	e := eventState{tripState: x, events: &trippg.EventStore{NewID: func() (string, error) { return onboardingID(t), nil }}}
	var err error
	if e.event, err = trippg.NewEventHandler(x.authority, x.meters, x.store, e.events); err != nil {
		t.Fatal(err)
	}
	e.trips = syncpg.NewStore(eventIntakeConfig(x.syncFixtureState))
	return e
}

func eventFixture(t *testing.T) eventState { return eventFixtureOn(t, tripFixture(t)) }

// eventOp is member d's trip.event.record of a new event on trip.
func (x eventState) eventOp(t *testing.T, d syncFixtureState, trip, kind string, text any, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = tripapp.EventOperationType
		raw["entity"] = map[string]any{"type": tripapp.EventEntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		payload := map[string]any{"trip_id": trip, "type": kind, "text": text}
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

func eventOf(op protocol.Operation) string { return op.Snapshot().Entity.ID }

func (x eventState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	handler := x.event
	switch op.Snapshot().Type {
	case tripapp.StartOperationType:
		handler = x.start
	case tripapp.FinishOperationType:
		handler = x.finish
	}
	return x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), handler)
}

type eventCounts struct{ events, held, items, readings, effects, claims, receipts, audits int }

func (x eventState) count(t *testing.T, op protocol.Operation) eventCounts {
	t.Helper()
	var c eventCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_events WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_held_events WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`,
		x.scope.OrgID().String(), op.ID()).Scan(&c.events, &c.held, &c.items, &c.readings, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x eventState) accepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || len(ack.Warnings()) != 0 {
		t.Fatal("legitimate logbook event was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (eventCounts{events: 1, effects: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("accepted logbook event not stored exactly once: %+v", c)
	}
}

func (x eventState) held(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != tripapp.TripAlreadyClosed || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("expected a held TRIP_ALREADY_CLOSED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (eventCounts{held: 1, items: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("held logbook event not kept exactly once: %+v", c)
	}
}

func (x eventState) nothing(t *testing.T, op protocol.Operation) {
	t.Helper()
	if c := x.count(t, op); c != (eventCounts{}) {
		t.Fatalf("refused logbook event left state: %+v", c)
	}
}

func (x eventState) malformed(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (eventCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("malformed logbook event stored a fact: %+v", c)
	}
}

func (x eventState) pending(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
		t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
	}
	x.nothing(t, op)
}

func (x eventState) unavailable(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("expected RETRY/UNAVAILABLE with nothing stored", err, ack.Result())
	}
	x.nothing(t, op)
}

func (x eventState) refused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	x.nothing(t, op)
}

func (x eventState) replays(t *testing.T, d syncFixtureState, op protocol.Operation, ack protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
}

// startTrip accepts a new trip started now by member d on the vehicle.
func (x eventState) startTrip(t *testing.T, d syncFixtureState) (string, time.Time) {
	t.Helper()
	started := x.declaredNow(t)
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.tripState.accepted(t, op, ack, err, 1, 1)
	return tripOf(op), started
}

func (x eventState) finishTrip(t *testing.T, d syncFixtureState, trip string, at time.Time) {
	t.Helper()
	op := x.finishOp(t, d, trip, at, nil)
	ack, err := x.process(t, d, op)
	x.tripState.accepted(t, op, ack, err, 2, 1)
}

func (x eventState) stateOf(t *testing.T, trip string) string {
	t.Helper()
	var state string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_trips WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), trip).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (x eventState) timeline(t *testing.T, trip string) []tripapp.TimelineEntry {
	t.Helper()
	var entries []tripapp.TimelineEntry
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var err error
		entries, err = x.events.TimelineTx(t.Context(), syncapp.Borrow(tx), x.scope, trip)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

// BUC-EMP-161 / B-BR-EMP-321/324: typed facts appended to the VAL-01 trip,
// each with one ACK in one transaction, exact replay and digest reuse; a late
// event inside the window of the completed trip; the timeline in order of
// occurrence with fact type and receipt; nothing changes the trip.
func TestTripEventIntakeRecordsTheTimelineAndReplays(t *testing.T) {
	x := eventFixture(t)
	d := x.syncFixtureState
	trip, started := x.startTrip(t, d)
	arrival := x.eventOp(t, d, trip, "ARRIVAL", "Cliente Norte, doca 3", started.Add(time.Hour), nil)
	note := x.eventOp(t, d, trip, "NOTE", "Cliente ausente", started.Add(90*time.Minute), nil)
	departure := x.eventOp(t, d, trip, "DEPARTURE", nil, started.Add(2*time.Hour), nil)
	acks := map[string]protocol.Acknowledgement{}
	for _, op := range []protocol.Operation{departure, arrival, note} {
		ack, err := x.process(t, d, op)
		x.accepted(t, op, ack, err)
		acks[op.ID()] = ack
	}
	org := x.scope.OrgID().String()
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT e.trip_id=$3 AND e.event_type='ARRIVAL' AND e.event_text='Cliente Norte, doca 3' AND e.author_membership_id=$4
 AND e.author_employee_id=$5 AND e.enrollment_id=$6 AND e.payload_digest=$7 AND e.occurred_submicro=123 AND e.entry_zone='America/Cuiaba' AND e.device_offset_seconds=-14400
 AND e.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=e.org_id AND a.operation_id=e.operation_id)
 FROM org_trip_events e WHERE e.org_id=$1 AND e.id=$2`, org, eventOf(arrival), trip, d.source.MembershipID, d.person.employee, d.source.EnrollmentID, arrival.PayloadDigest()).Scan(&exact); err != nil || !exact {
		t.Fatal("stored event is not the exact logbook fact", err)
	}
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT event_text IS NULL AND event_type='DEPARTURE' FROM org_trip_events WHERE org_id=$1 AND id=$2`, org, eventOf(departure)).Scan(&exact); err != nil || !exact {
		t.Fatal("an optional text was invented", err)
	}
	for _, op := range []protocol.Operation{arrival, note, departure} {
		x.replays(t, d, op, acks[op.ID()])
		x.accepted(t, op, acks[op.ID()], nil)
	}
	reused := x.eventOp(t, d, trip, "NOTE", "Outro texto", started.Add(time.Hour), func(raw, _ map[string]any) {
		raw["operation_id"], raw["entity"] = note.ID(), map[string]any{"type": tripapp.EventEntityType, "id": eventOf(note), "expected_revision": nil}
	})
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.accepted(t, note, acks[note.ID()], nil)
	// No logbook event changes the trip, its readings, open work or active use.
	if x.stateOf(t, trip) != "IN_PROGRESS" {
		t.Fatal("a logbook event changed the trip state")
	}
	if ids := x.activeAt(t, started.Add(3*time.Hour)); len(ids) != 1 || ids[0] != trip {
		t.Fatal("active use changed", ids)
	}
	finished := started.Add(3 * time.Hour)
	x.finishTrip(t, d, trip, finished)
	// A fact recorded during the trip and delivered after its finish is accepted
	// as a late delivery inside the window; the trip stays COMPLETED.
	late := x.eventOp(t, d, trip, "STOP", nil, started.Add(150*time.Minute), nil)
	lateAck, err := x.process(t, d, late)
	x.accepted(t, late, lateAck, err)
	atFinish := x.eventOp(t, d, trip, "ARRIVAL", "Base", finished, nil)
	atFinishAck, err := x.process(t, d, atFinish)
	x.accepted(t, atFinish, atFinishAck, err)
	if x.stateOf(t, trip) != "COMPLETED" {
		t.Fatal("a late event changed the completed trip")
	}
	entries := x.timeline(t, trip)
	want := []struct{ kind, id string }{{"START", trip}, {"ARRIVAL", eventOf(arrival)}, {"NOTE", eventOf(note)}, {"DEPARTURE", eventOf(departure)},
		{"STOP", eventOf(late)}, {"ARRIVAL", eventOf(atFinish)}, {"FINISH", trip}}
	if len(entries) != len(want) {
		t.Fatal("timeline length", len(entries))
	}
	for i, w := range want {
		e := entries[i]
		if e.FactType != w.kind || e.Item.ID != w.id || e.AuthorMembershipID != d.source.MembershipID || e.ReceivedAt.IsZero() || e.Declared.EntryZone != "America/Cuiaba" {
			t.Fatal("timeline item", i, e.FactType, e.Item.ID == w.id)
		}
	}
	// The receipt is distinguishable from the original instant: the late STOP
	// was received after the finish although it happened before it.
	if !entries[4].ReceivedAt.After(entries[6].ReceivedAt) || !entries[4].Declared.OccurredAt.Before(entries[6].Declared.OccurredAt) {
		t.Fatal("timeline lost the receipt of a late event")
	}
	if entries[1].Text == nil || *entries[1].Text != "Cliente Norte, doca 3" || entries[3].Text != nil {
		t.Fatal("timeline texts")
	}
	x.replays(t, d, late, lateAck)
}

// BUC-EMP-162 / B-BR-EMP-322/323 negatives: nothing is stored and nothing is
// dropped; an event after the finish is held TRIP_ALREADY_CLOSED.
func TestTripEventIntakeRefusalsHoldsAndReservedTypes(t *testing.T) {
	x := eventFixture(t)
	d := x.syncFixtureState
	trip, started := x.startTrip(t, d)
	accepted := x.eventOp(t, d, trip, "NOTE", "Primeira nota", started.Add(time.Minute), nil)
	acceptedAck, err := x.process(t, d, accepted)
	x.accepted(t, accepted, acceptedAck, err)
	t.Run("before-start-malformed", func(t *testing.T) {
		op := x.eventOp(t, d, trip, "STOP", nil, started.Add(-time.Nanosecond), nil)
		ack, err := x.process(t, d, op)
		x.malformed(t, op, ack, err)
	})
	t.Run("unknown-type-malformed", func(t *testing.T) {
		for _, kind := range []string{"note", "START", "FINISH", " NOTE", ""} {
			op := x.eventOp(t, d, trip, kind, "Texto", started.Add(time.Minute), nil)
			ack, err := x.process(t, d, op)
			x.malformed(t, op, ack, err)
		}
	})
	t.Run("note-without-text-malformed", func(t *testing.T) {
		for _, text := range []any{nil, "   ", "linha\nnova"} {
			op := x.eventOp(t, d, trip, "NOTE", text, started.Add(time.Minute), nil)
			ack, err := x.process(t, d, op)
			x.malformed(t, op, ack, err)
		}
	})
	t.Run("payload-shape-malformed", func(t *testing.T) {
		for name, edit := range map[string]func(raw, p map[string]any){
			"extra-key":        func(_, p map[string]any) { p["asset_id"] = x.vehicle.AssetID },
			"missing-text-key": func(_, p map[string]any) { delete(p, "text") },
			"trip-not-uuid":    func(_, p map[string]any) { p["trip_id"] = "trip" },
			"trip-entity-type": func(raw, _ map[string]any) {
				raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
			},
		} {
			op := x.eventOp(t, d, trip, "NOTE", "Texto", started.Add(time.Minute), edit)
			ack, err := x.process(t, d, op)
			if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
				t.Fatal("malformed payload", name, err, ack.Code())
			}
			x.malformed(t, op, ack, err)
		}
	})
	t.Run("reserved-type-unavailable", func(t *testing.T) {
		for _, kind := range []string{"PAUSE", "RESUME", "CANCEL", "FUELING", "TOLL", "PARKING", "INCIDENT", "DAMAGE", "HANDOVER"} {
			op := x.eventOp(t, d, trip, kind, nil, started.Add(time.Minute), nil)
			ack, err := x.process(t, d, op)
			x.unavailable(t, op, ack, err)
		}
		if x.stateOf(t, trip) != "IN_PROGRESS" {
			t.Fatal("a reserved type changed the trip state")
		}
	})
	t.Run("identity-reused-malformed", func(t *testing.T) {
		op := x.eventOp(t, d, trip, "STOP", nil, started.Add(2*time.Minute), func(raw, _ map[string]any) {
			raw["entity"] = map[string]any{"type": tripapp.EventEntityType, "id": eventOf(accepted), "expected_revision": nil}
		})
		ack, err := x.process(t, d, op)
		x.malformed(t, op, ack, err)
	})
	t.Run("unknown-trip-pending", func(t *testing.T) {
		op := x.eventOp(t, d, onboardingID(t), "NOTE", "Texto", started.Add(time.Minute), nil)
		ack, err := x.process(t, d, op)
		x.pending(t, op, ack, err)
	})
	t.Run("held-start-trip-pending", func(t *testing.T) {
		busy := x.startOp(t, d, started.Add(time.Minute), nil)
		ack, err := x.tripState.process(t, d, busy)
		x.tripState.held(t, busy, ack, err, tripapp.AssetBusy)
		op := x.eventOp(t, d, tripOf(busy), "NOTE", "Texto", started.Add(2*time.Minute), nil)
		ack, err = x.process(t, d, op)
		x.pending(t, op, ack, err)
	})
	t.Run("foreign-trip-not-found-pending", func(t *testing.T) {
		foreign := x.foreignTrip(t)
		op := x.eventOp(t, d, foreign, "NOTE", "Texto", started.Add(time.Minute), nil)
		ack, err := x.process(t, d, op)
		x.pending(t, op, ack, err)
		var events int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_events WHERE trip_id=$1`, foreign).Scan(&events); err != nil || events != 0 {
			t.Fatal("an event reached another organization's trip", events, err)
		}
	})
	t.Run("another-member-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		x.assign(t, driver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		op := x.eventOp(t, driver, trip, "NOTE", "Texto", x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("never-assigned-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		op := x.eventOp(t, driver, trip, "NOTE", "Texto", x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("after-finish-held", func(t *testing.T) {
		finished := started.Add(time.Hour)
		x.finishTrip(t, d, trip, finished)
		op := x.eventOp(t, d, trip, "NOTE", "Depois do fim", finished.Add(time.Nanosecond), nil)
		ack, err := x.process(t, d, op)
		x.held(t, op, ack, err)
		x.replays(t, d, op, ack)
		x.held(t, op, ack, nil)
		var kept bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT h.trip_id=$3 AND h.event_type='NOTE' AND h.event_text='Depois do fim' AND h.occurred_submicro=124 AND i.code='TRIP_ALREADY_CLOSED'
 AND i.operation_type='trip.event.record' AND a.review_item_id=i.id AND a.review_proof_version=1
 FROM org_trip_held_events h JOIN org_sync_review_items i ON(i.org_id,i.id)=(h.org_id,h.review_item_id) JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(i.org_id,i.operation_id)
 WHERE h.org_id=$1 AND h.operation_id=$2`, x.scope.OrgID().String(), op.ID(), trip).Scan(&kept); err != nil || !kept {
			t.Fatal("held event not kept with its review item", err)
		}
		for _, e := range x.timeline(t, trip) {
			if e.Item.ID == eventOf(op) {
				t.Fatal("a held event became a timeline fact")
			}
		}
		// The held identity is reserved: the same event id is not reusable.
		again := x.eventOp(t, d, trip, "NOTE", "Antes do fim", started.Add(2*time.Minute), func(raw, _ map[string]any) {
			raw["entity"] = map[string]any{"type": tripapp.EventEntityType, "id": eventOf(op), "expected_revision": nil}
		})
		againAck, err := x.process(t, d, again)
		x.malformed(t, again, againAck, err)
		if x.stateOf(t, trip) != "COMPLETED" {
			t.Fatal("a held event changed the trip")
		}
	})
	x.replays(t, d, accepted, acceptedAck)
}

// foreignTrip plants one trip row of another organization in this database
// (triggers and keys off for the planted row only) so that an event can name
// an identity that exists elsewhere; forced RLS must hide it.
func (x eventState) foreignTrip(t *testing.T) string {
	t.Helper()
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	id := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_trips(org_id,id,operation_id,payload_digest,asset_id,driver_employee_id,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,
 entry_zone,device_offset_seconds,missing_reading_reason,reporting_day,reporting_zone,reporting_source,state)
VALUES($1,$2,$3,'sha256:'||repeat('0',64),$4,$5,$6,$5,$7,clock_timestamp()-interval '1 hour',0,'America/Cuiaba',-14400,'Sintetico',current_date,'America/Cuiaba','organization','IN_PROGRESS')`,
			other.OrgID.String(), id, onboardingID(t), onboardingID(t), onboardingID(t), other.MembershipID, onboardingID(t))
		return err
	}); err != nil {
		t.Fatal("foreign trip fixture", err)
	}
	return id
}

// The event and its fact commit together or not at all.
type eventFailingStore struct{ *trippg.EventStore }

func (s eventFailingStore) InsertEventTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, tripapp.EventCommand, string) error {
	return errors.New("injected logbook failure")
}

func TestTripEventIntakeRollsBackAndConcurrentDuplicatesLeaveOneEvent(t *testing.T) {
	x := eventFixture(t)
	d := x.syncFixtureState
	trip, started := x.startTrip(t, d)
	op := x.eventOp(t, d, trip, "ARRIVAL", nil, started.Add(time.Minute), nil)
	failing, err := trippg.NewEventHandler(x.authority, x.meters, x.store, eventFailingStore{x.events})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), failing)
	if !errors.Is(err, syncapp.ErrUnavailable) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("injected failure was not a retry", err)
	}
	x.nothing(t, op)
	config := eventIntakeConfig(x.syncFixtureState)
	config.Pool = x.f.RuntimePool(t, 12)
	store := syncpg.NewStore(config)
	var wg sync.WaitGroup
	acks := make([]protocol.Acknowledgement, 12)
	errs := make([]error, 12)
	for i := range 12 {
		wg.Go(func() {
			acks[i], errs[i] = store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), x.event)
		})
	}
	wg.Wait()
	var original []byte
	for i := range 12 {
		if errs[i] != nil {
			if !errors.Is(errs[i], syncapp.ErrUnavailable) || errors.Is(errs[i], syncapp.ErrSourceDenied) {
				t.Fatal("concurrent duplicate failed outside RETRY", errs[i])
			}
			continue
		}
		if acks[i].Result() != protocol.Accepted || original != nil && !bytes.Equal(original, acks[i].CanonicalBytes()) {
			t.Fatal("concurrent duplicates produced different results")
		}
		original = acks[i].CanonicalBytes()
	}
	if original == nil {
		t.Fatal("no concurrent duplicate was accepted")
	}
	ack, err = x.process(t, d, op)
	if err != nil || !bytes.Equal(ack.CanonicalBytes(), original) {
		t.Fatal("replay after the race", err)
	}
	x.accepted(t, op, ack, nil)
}

// opBarrier holds the winning operation inside its intake transaction (after
// admission) until the other one is observed waiting on a lock.
type opBarrier struct {
	syncapp.Handler
	winner          string
	inside, proceed chan struct{}
	once            *sync.Once
}

func (h opBarrier) ReviewProof() syncapp.ReviewProof {
	return h.Handler.(syncapp.ReviewProofProvider).ReviewProof()
}

func (h opBarrier) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	if op.ID() == h.winner && mode == syncapp.Write {
		h.once.Do(func() {
			close(h.inside)
			<-h.proceed
		})
	}
	return h.Handler.Admit(ctx, db, source, op, mode)
}

// B-BR-EMP-324: an event and the finish of its trip race with barriers, in
// both orders. An event inside the window is accepted either way and the
// finish completes the trip either way; an event declared after the finish is
// accepted when it wins and held TRIP_ALREADY_CLOSED when the finish wins.
func TestTripEventRacesTheFinishInBothOrders(t *testing.T) {
	for _, c := range []struct {
		name        string
		eventWins   bool
		afterFinish bool
	}{{"inside-event-first", true, false}, {"inside-finish-first", false, false}, {"after-event-first", true, true}, {"after-finish-first", false, true}} {
		t.Run(c.name, func(t *testing.T) {
			x := eventFixture(t)
			d := x.syncFixtureState
			trip, started := x.startTrip(t, d)
			finishAt := started.Add(time.Hour)
			eventAt := started.Add(30 * time.Minute)
			if c.afterFinish {
				eventAt = finishAt.Add(time.Minute)
			}
			event := x.eventOp(t, d, trip, "STOP", nil, eventAt, nil)
			finish := x.finishOp(t, d, trip, finishAt, nil)
			config := eventIntakeConfig(x.syncFixtureState)
			config.Pool = x.f.RuntimePool(t, 4)
			store := syncpg.NewStore(config)
			winner, loser := event, finish
			if !c.eventWins {
				winner, loser = finish, event
			}
			barrier := &sync.Once{}
			inside, proceed := make(chan struct{}), make(chan struct{})
			handlerOf := func(op protocol.Operation) syncapp.Handler {
				h := x.event
				if op.Snapshot().Type == tripapp.FinishOperationType {
					h = x.finish
				}
				return opBarrier{Handler: h, winner: winner.ID(), inside: inside, proceed: proceed, once: barrier}
			}
			acks := map[string]protocol.Acknowledgement{}
			errs := map[string]error{}
			var mu sync.Mutex
			var done sync.WaitGroup
			run := func(op protocol.Operation) {
				done.Go(func() {
					ack, err := store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), handlerOf(op))
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
				t.Fatal("the winner never reached its admission", errs)
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
				t.Fatal("the losing operation never waited on the winner's lock")
			}
			x.tripState.accepted(t, finish, acks[finish.ID()], errs[finish.ID()], 2, 1)
			if c.afterFinish && !c.eventWins {
				x.held(t, event, acks[event.ID()], errs[event.ID()])
			} else {
				x.accepted(t, event, acks[event.ID()], errs[event.ID()])
			}
			if x.stateOf(t, trip) != "COMPLETED" {
				t.Fatal("the race left the trip open")
			}
			x.replays(t, d, event, acks[event.ID()])
			x.replays(t, d, finish, acks[finish.ID()])
		})
	}
}

// Forced RLS, insert-only facts and the database guards of the window, the
// author and publication (EDR-208).
func TestTripEventSchemaIsForcedRLSInsertOnlyAndGuarded(t *testing.T) {
	x := eventFixture(t)
	d := x.syncFixtureState
	trip, started := x.startTrip(t, d)
	op := x.eventOp(t, d, trip, "NOTE", "Guardado", started.Add(time.Minute), nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_events,org_trip_held_events TO "+role); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_events`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("events visible without an organization setting", visible, err)
	}
	for _, statement := range []string{
		`UPDATE org_trip_events SET event_text='Reescrito' WHERE id=$1`,
		`DELETE FROM org_trip_events WHERE id=$1`,
		`UPDATE org_trip_held_events SET event_text='Reescrito' WHERE id=$1`,
		`DELETE FROM org_trip_held_events WHERE id=$1`,
	} {
		if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), statement, eventOf(op))
			return err
		}); err == nil {
			t.Fatal("logbook history was rewritten", statement)
		}
	}
	// Database guards with the handler bypassed, on the claimed operation of a
	// refused (MALFORMED) event of the same author: a row before the trip start
	// is refused by the source trigger; a row inside the window is refused at
	// commit because its operation has no ACCEPTED receipt and effect.
	refused := x.eventOp(t, d, trip, "note", "Recusado", started.Add(time.Minute), nil)
	refusedAck, err := x.process(t, d, refused)
	x.malformed(t, refused, refusedAck, err)
	insert := func(at time.Time) error {
		return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			stamp := at.Truncate(time.Microsecond)
			_, err := tx.Exec(t.Context(), `INSERT INTO org_trip_events(org_id,id,trip_id,operation_id,payload_digest,event_type,event_text,author_membership_id,author_employee_id,enrollment_id,
 occurred_at,occurred_submicro,entry_zone,device_offset_seconds) VALUES($1,$2,$3,$4,$5,'STOP',NULL,$6,$7,$8,$9,0,'America/Cuiaba',-14400)`,
				x.scope.OrgID().String(), eventOf(refused), trip, refused.ID(), refused.PayloadDigest(), d.source.MembershipID, d.person.employee, d.source.EnrollmentID, stamp)
			return err
		})
	}
	var pgErr *pgconn.PgError
	if err := insert(started.Add(-time.Hour)); !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.Message != "invalid trip event" {
		t.Fatal("an event before its trip passed the source trigger", err)
	}
	if err := insert(started.Add(time.Minute)); !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.Message != "trip event publication unavailable" {
		t.Fatal("an unpublished logbook fact committed", err)
	}
	x.malformed(t, refused, refusedAck, nil)
	var forced, indexed bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE relname IN('org_trip_events','org_trip_held_events')`).Scan(&forced); err != nil || !forced {
		t.Fatal("logbook tables are not forced RLS", err)
	}
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT indexdef LIKE '%(org_id, trip_id, occurred_at, occurred_submicro, id)%' FROM pg_indexes WHERE indexname='org_trip_events_trip'`).Scan(&indexed); err != nil || !indexed {
		t.Fatal("timeline index", err)
	}
	x.replays(t, d, op, ack)
}
