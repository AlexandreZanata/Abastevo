//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// tripAmendState is the EMP-TRIP-016 fixture with the logbook owner on the
// same intake, the amendment authority and the fleet reading port, and a
// fleet manager who proposes amendments (VAL-26: Elisa).
type tripAmendState struct {
	tripPeriodState
	events  *trippg.EventStore
	event   syncapp.Handler
	manager tripReviewer
}

func tripAmendIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := tripReviewIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: tripapp.EventOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup,
		protocol.Rejected: syncapp.NoAsyncFollowup, protocol.AcceptedForReview: syncapp.AtomicReview}
	return config
}

// amendReviews composes the review store with its amendment ports.
func amendReviews(x tripReviewState) {
	x.reviews.Amenders = x.reviews.Authority.(trippg.AmendAuthority)
	x.reviews.Readings = x.meters
}

func tripAmendFixture(t *testing.T) tripAmendState {
	t.Helper()
	x := tripAmendState{tripPeriodState: tripPeriodFixture(t)}
	amendReviews(x.tripReviewState)
	// The METER-004 parent set reads the fueling review state too.
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,reading_id,review_state) ON org_fuelings TO "+pgx.Identifier{x.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	x.events = &trippg.EventStore{NewID: func() (string, error) { return onboardingID(t), nil }}
	var err error
	if x.event, err = trippg.NewEventHandler(x.authority, x.meters, x.store, x.events); err != nil {
		t.Fatal(err)
	}
	x.intake = syncpg.NewStore(tripAmendIntakeConfig(x.syncFixtureState))
	x.manager = x.reviewerFor(t, "FLEET_MANAGER", "ORG", "")
	return x
}

func (x tripAmendState) processAny(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	if op.Snapshot().Type == tripapp.EventOperationType {
		return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.event)
	}
	return x.process(t, d, op)
}

func (x tripAmendState) eventOp(t *testing.T, d syncFixtureState, trip, kind string, text any, declared time.Time) protocol.Operation {
	t.Helper()
	return eventState{tripState: x.tripState}.eventOp(t, d, trip, kind, text, declared, nil)
}

type tripLateCounts struct{ events, heldEvents, items int }

func (x tripAmendState) lateCounts(t *testing.T, op protocol.Operation) tripLateCounts {
	t.Helper()
	var c tripLateCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_events WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_held_events WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.events, &c.heldEvents, &c.items); err != nil {
		t.Fatal(err)
	}
	return c
}

// heldEvent proves a held logbook fact with its code, kept once and replayed.
func (x tripAmendState) heldEvent(t *testing.T, d syncFixtureState, op protocol.Operation, code string) {
	t.Helper()
	ack, err := x.processAny(t, d, op)
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != code || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("expected a held logbook event", code, err, ack.Result(), ack.Code())
	}
	if c := x.lateCounts(t, op); c != (tripLateCounts{heldEvents: 1, items: 1}) {
		t.Fatalf("held logbook event not kept exactly once: %+v", c)
	}
	for range 2 {
		replay, err := x.processAny(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("the held event resend did not return the stored ACK", err)
		}
	}
	if c := x.lateCounts(t, op); c != (tripLateCounts{heldEvents: 1, items: 1}) {
		t.Fatalf("a resend counted the late event twice: %+v", c)
	}
}

func (x tripAmendState) acceptedEvent(t *testing.T, d syncFixtureState, op protocol.Operation) {
	t.Helper()
	ack, err := x.processAny(t, d, op)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 {
		t.Fatal("a field correction event was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.lateCounts(t, op); c != (tripLateCounts{events: 1}) {
		t.Fatalf("accepted event not stored once: %+v", c)
	}
}

func (x tripAmendState) approve(t *testing.T, r tripReviewer, trip string, revision int64) tripapp.DecisionResult {
	t.Helper()
	result, _, err := x.decideBy(t, r, trip, revision, review.Approve, "")
	if err != nil || result.State != review.Approved {
		t.Fatal("independent approval", revision, err)
	}
	return result
}

// propose never calls t.Fatal, so it is safe inside a goroutine.
func (x tripAmendState) propose(store *trippg.ReviewStore, r tripReviewer, trip string, expected int64, start, end *tripapp.Correction, reason, op string) (tripapp.AmendmentResult, bool, error) {
	command, err := tripapp.NewAmendmentCommand(tripapp.AmendmentInput{OperationID: op, TripID: trip, ExpectedRevision: expected, Reason: reason, Start: start, End: end})
	if err != nil {
		return tripapp.AmendmentResult{}, false, err
	}
	first, err := jobs.NewUUIDv4()
	if err != nil {
		return tripapp.AmendmentResult{}, false, err
	}
	second, err := jobs.NewUUIDv4()
	if err != nil {
		return tripapp.AmendmentResult{}, false, err
	}
	return store.ProposeTripAmendment(context.Background(), x.scope, r.actor, r.member, command, [2]string{first, second})
}

func odometer(value int64) *tripapp.Correction {
	return &tripapp.Correction{Kind: meter.Odometer, Value: value}
}

// measuredTrip is driver d's VAL-01 trip with both readings (184 320 000 m
// and 184 610 000 m), submitted.
func (x tripAmendState) measuredTrip(t *testing.T, d syncFixtureState, at time.Time) string {
	t.Helper()
	start := x.startOp(t, d, at, nil)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 1)
	trip := tripOf(start)
	finish := x.finishOp(t, d, trip, at.Add(3*time.Hour), nil)
	ack, err = x.process(t, d, finish)
	x.accepted(t, finish, ack, err, 2, 1)
	x.submitted(t, d, trip, at.Add(3*time.Hour+5*time.Minute))
	return trip
}

func (x tripAmendState) finishReading(t *testing.T, trip string) (string, string) {
	t.Helper()
	var reading, start string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT f.reading_id::text,t.reading_id::text FROM org_trip_finishes f JOIN org_trips t ON(t.org_id,t.id)=(f.org_id,f.trip_id) WHERE f.org_id=$1 AND f.trip_id=$2`,
		x.scope.OrgID().String(), trip).Scan(&reading, &start); err != nil {
		t.Fatal(err)
	}
	return reading, start
}

// correctionStore is the real METER-004 decision store whose approved-parent
// port is the production set of fuelings and trips (B-BR-EMP-368).
func (x tripAmendState) correctionStore(t *testing.T) *fleetpg.MeterStore {
	t.Helper()
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store := *x.meters
	store.Audit = auditpg.NewStore(x.f.Runtime, catalog)
	store.Parents = fleetpg.MeterParentSet{&fuelpg.Store{Allocations: x.assets}, &trippg.Store{}}
	return &store
}

func (x tripAmendState) meterDecision(t *testing.T, reading string, value int64) error {
	t.Helper()
	store := x.correctionStore(t)
	v, err := meter.NewValue(meter.Odometer, value, nil)
	if err != nil {
		t.Fatal(err)
	}
	command, err := fa.NewMeterDecisionCommand(fa.MeterDecisionInput{OperationID: onboardingID(t), AssetID: x.vehicle.AssetID, MeterID: x.vehicle.MeterID, ReadingID: reading,
		Decision: fa.MeterSupersede, Reason: "Correção direta", Value: v})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.DecideMeterReading(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, onboardingID(t), command)
	return err
}

// VAL-26 (c), trips §10, BUC-EMP-184 / B-BR-EMP-365..368 on real PostGIS:
// the fleet manager's correction of the final reading is a reviewed
// revision; 290 000 m stay in effect until an independent approval, then
// 280 000 m with the original reading superseded and kept; the return,
// edit and rejection loop and the locked month keep the effective revision.
func TestTripAmendmentIsAReviewedRevisionVAL26(t *testing.T) {
	x := tripAmendFixture(t)
	d, at := x.assignedDriver(t)
	owner := x.owner()
	trip := x.measuredTrip(t, d, at)
	endReading, startReading := x.finishReading(t, trip)
	x.approve(t, owner, trip, 1)
	view, err := x.read(owner, trip)
	if err != nil || view.EffectiveRevision != 1 || view.EffectiveUsage == nil || view.EffectiveUsage.Delta != 290_000 || view.EffectiveUsage.Revision != 1 || len(view.Amendments) != 0 {
		t.Fatal("the approved trip's effective usage", err)
	}
	// METER-004 cannot change an approved trip's readings in place.
	for _, reading := range []string{endReading, startReading} {
		if err := x.meterDecision(t, reading, 184_600_000); !errors.Is(err, fa.ErrAmendmentRequired) {
			t.Fatal("a reading of an approved trip was corrected in place", err)
		}
	}
	op := onboardingID(t)
	reason := "Leitura final digitada errada"
	proposed, replayed, err := x.propose(x.reviews, x.manager, trip, 1, nil, odometer(184_600_000), reason, op)
	if err != nil || replayed || proposed.Revision != 2 || proposed.Base != 1 || proposed.Intent != amend.IntentAmend || proposed.State != review.Submitted {
		t.Fatal("the manager's amendment of the approved trip", err)
	}
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1,AMEND:SUBMITTED:2" || audits != 2 {
		t.Fatal("amendment history", entries, audits)
	}
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT a.action='trip.trip.amended' AND a.actor_id=$3 AND a.reason_code='' AND e.reason=$4 AND e.actor_membership_id=$5
 FROM org_audit_events a JOIN org_trip_review_entries e ON(e.org_id,e.operation_id)=(a.org_id,a.operation_id) WHERE a.org_id=$1 AND a.operation_id=$2`,
		x.scope.OrgID().String(), op, x.manager.account, reason, x.manager.member).Scan(&exact); err != nil || !exact {
		t.Fatal("the amendment audit event or entry is not exact", err)
	}
	again, replayed, err := x.propose(x.reviews, x.manager, trip, 1, nil, odometer(184_600_000), reason, op)
	if err != nil || !replayed || again != proposed {
		t.Fatal("the proposal replay did not return the first result", err)
	}
	if _, _, err := x.propose(x.reviews, x.manager, trip, 1, nil, odometer(184_600_001), reason, op); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("the same operation with another digest", err)
	}
	// BR-AMD-002: the previous approved revision stays in effect; nothing on
	// the meter changes before the approval.
	view, err = x.read(owner, trip)
	if err != nil || view.EffectiveRevision != 1 || view.EffectiveUsage.Delta != 290_000 || len(view.Amendments) != 1 || view.Amendments[0].State != review.Submitted ||
		view.Amendments[0].End == nil || view.Amendments[0].End.From != 184_610_000 || view.Amendments[0].End.To != 184_600_000 || view.Amendments[0].Usage.Delta != 280_000 ||
		view.Amendments[0].Usage.Revision != 2 || view.Review != tripdomain.ReviewSubmitted || view.Revision != 2 {
		t.Fatal("a pending amendment changed the effective revision", err)
	}
	var voided, created int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_reading_states WHERE org_id=$1 AND reading_id=$2 AND state='VOIDED'),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND value=184600000)`,
		x.scope.OrgID().String(), endReading).Scan(&voided, &created); err != nil || voided != 0 || created != 0 {
		t.Fatal("the meter series changed before the approval", voided, created, err)
	}
	// Refusals: a second pending amendment, a stale revision, the author's
	// own approval and a decision naming the approved revision.
	if _, _, err := x.propose(x.reviews, x.manager, trip, 2, odometer(184_310_000), nil, reason, onboardingID(t)); !errors.Is(err, amend.ErrAmendmentPending) {
		t.Fatal("a second amendment beside a pending one", err)
	}
	if _, _, err := x.propose(x.reviews, x.manager, trip, 1, odometer(184_310_000), nil, reason, onboardingID(t)); !errors.Is(err, amend.ErrConflict) {
		t.Fatal("a stale amendment", err)
	}
	if _, _, err := x.decideBy(t, x.manager, trip, 2, review.Approve, ""); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("the amendment's author approved it", err)
	}
	if _, _, err := x.decideBy(t, owner, trip, 1, review.Approve, ""); !errors.Is(err, review.ErrConflict) {
		t.Fatal("a decision naming the approved revision", err)
	}
	approveOp := onboardingID(t)
	approved, _, err := x.decide(x.scope, owner, trip, 2, review.Approve, "", approveOp)
	if err != nil || approved.State != review.Approved || approved.Revision != 2 {
		t.Fatal("the independent approval of the amendment", err)
	}
	view, err = x.read(owner, trip)
	if err != nil || view.EffectiveRevision != 2 || view.EffectiveUsage.Delta != 280_000 || view.EffectiveUsage.Revision != 2 || view.Amendments[0].State != review.Approved {
		t.Fatal("the approved amendment is not in effect with 280 000 m", err)
	}
	newReading := view.Amendments[0].End.NewReadingID
	var superseded bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT s.cause='SUPERSEDED' AND s.superseded_by=$3::uuid AND s.operation_id=$4::uuid AND s.reason=$5
 AND n.value=184600000 AND n.origin='TRIP_END' AND n.operation_id=$4::uuid AND n.author_membership_id=$6::uuid AND n.capture_kind='WEB_SESSION' AND o.value=184610000
 FROM org_meter_reading_states s JOIN org_meter_readings n ON n.org_id=s.org_id AND n.id=$3::uuid JOIN org_meter_readings o ON o.org_id=s.org_id AND o.id=s.reading_id
 WHERE s.org_id=$1 AND s.reading_id=$2::uuid AND s.state='VOIDED'`, x.scope.OrgID().String(), endReading, newReading, approveOp, reason, owner.member).Scan(&superseded); err != nil || !superseded {
		t.Fatal("the approval did not supersede the original reading, which must be kept", err)
	}
	if again, replayed, err := x.decide(x.scope, owner, trip, 2, review.Approve, "", approveOp); err != nil || !replayed || again != approved {
		t.Fatal("the approval replay", err)
	}
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND value=184600000`, x.scope.OrgID().String()).Scan(&created); err != nil || created != 1 {
		t.Fatal("the replay superseded twice", created, err)
	}
	if err := x.meterDecision(t, newReading, 184_500_000); !errors.Is(err, fa.ErrAmendmentRequired) {
		t.Fatal("the amendment's reading was corrected in place", err)
	}
	// Return, edit and reject: the approved revision 2 stays in effect.
	if r, _, err := x.propose(x.reviews, x.manager, trip, 2, odometer(184_310_000), nil, "Leitura inicial", onboardingID(t)); err != nil || r.Revision != 3 || r.Base != 2 {
		t.Fatal("an amendment of the amended trip", err)
	}
	if _, _, err := x.decideBy(t, owner, trip, 3, review.Return, "Confirme o valor"); err != nil {
		t.Fatal(err)
	}
	edit, _, err := x.propose(x.reviews, x.manager, trip, 3, odometer(184_300_000), nil, "Valor confirmado no painel", onboardingID(t))
	if err != nil || edit.Intent != amend.IntentEdit || edit.Revision != 4 || edit.Base != 3 {
		t.Fatal("the edit of a returned amendment", err)
	}
	if _, _, err := x.decideBy(t, owner, trip, 4, review.Reject, "Sem evidência"); err != nil {
		t.Fatal(err)
	}
	view, err = x.read(owner, trip)
	if err != nil || view.EffectiveRevision != 2 || view.EffectiveUsage.Delta != 280_000 || len(view.Amendments) != 3 || view.Amendments[1].State != review.Returned ||
		view.Amendments[2].State != review.Rejected || view.Amendments[2].Usage.Delta != 300_000 || view.Review != tripdomain.ReviewRejected {
		t.Fatal("a returned or rejected amendment changed the effective revision", err)
	}
	// BR-AMD-004 / EMP-TRIP-016: a locked month refuses the amendment.
	month := x.month(t, trip)
	locked := x.lock(t, month, 1)
	if _, _, err := x.propose(x.reviews, x.manager, trip, 4, odometer(184_330_000), nil, "Leitura inicial", onboardingID(t)); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("an amendment in a locked month", err)
	}
	x.reopen(t, month, locked.Revision, "Correção de viagens")
	if r, _, err := x.propose(x.reviews, x.manager, trip, 4, odometer(184_330_000), nil, "Leitura inicial", onboardingID(t)); err != nil || r.Revision != 5 || r.Base != 2 {
		t.Fatal("an amendment after the reopening", err)
	}
}

// VAL-26 (a)/(b), BUC-EMP-183 / B-BR-EMP-363/364: field facts are edits
// only while the field revision is a draft or returned; after submission or
// approval a late event or a stale or duplicate submission is one held fact;
// the resubmission loop keeps both revisions.
func TestTripLateAndDuplicateFactsAreHeldOnceAndTheReturnIsResubmitted(t *testing.T) {
	x := tripAmendFixture(t)
	d, at := x.assignedDriver(t)
	owner := x.owner()
	trip, _ := x.completed(t, d, at)
	x.submitted(t, d, trip, at.Add(90*time.Minute))
	// While SUBMITTED, an event inside the trip is held STALE_REVISION and
	// one after the finish keeps TRIP_ALREADY_CLOSED.
	x.heldEvent(t, d, x.eventOp(t, d, trip, "STOP", nil, at.Add(30*time.Minute)), tripapp.StaleRevision)
	x.heldEvent(t, d, x.eventOp(t, d, trip, "NOTE", "Depois do fim", at.Add(2*time.Hour)), tripapp.TripAlreadyClosed)
	if _, _, err := x.decideBy(t, owner, trip, 1, review.Return, "Informe o motivo da parada"); err != nil {
		t.Fatal(err)
	}
	// While RETURNED, the author's correction is an ordinary edit.
	note := x.eventOp(t, d, trip, "NOTE", "Parada para descanso obrigatório", at.Add(31*time.Minute))
	x.acceptedEvent(t, d, note)
	if ack := x.heldSubmission(t, d, x.submitOp(t, d, trip, 2, at.Add(2*time.Hour), nil)); ack.Code() != tripapp.StaleRevision {
		t.Fatal("a revision the returned case does not have")
	}
	resubmit := x.submitOp(t, d, trip, 1, at.Add(2*time.Hour), nil)
	ack, err := x.process(t, d, resubmit)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 2 {
		t.Fatal("the resubmission after the return", err)
	}
	x.replays(t, d, resubmit, ack)
	// A duplicate resubmission is held once, never a second RESUBMIT.
	x.heldSubmission(t, d, x.submitOp(t, d, trip, 1, at.Add(2*time.Hour+time.Minute), nil))
	if entries, _ := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,RETURN:RETURNED:1,RESUBMIT:SUBMITTED:2" {
		t.Fatal("resubmission history", entries)
	}
	if _, _, err := x.decideBy(t, owner, trip, 1, review.Approve, ""); !errors.Is(err, review.ErrConflict) {
		t.Fatal("a decision naming the returned revision", err)
	}
	x.approve(t, owner, trip, 2)
	// VAL-26 (a)/(b): after the approval a late event inside the trip is held
	// once, resent once, and changes nothing approved.
	before := x.tripRow(t, trip)
	x.heldEvent(t, d, x.eventOp(t, d, trip, "ARRIVAL", nil, at.Add(45*time.Minute)), tripapp.StaleRevision)
	x.heldSubmission(t, d, x.submitOp(t, d, trip, 2, at.Add(5*time.Hour), nil))
	if x.tripRow(t, trip) != before {
		t.Fatal("a late fact changed the approved trip")
	}
	if entries, _ := x.history(t, x.scope, trip); !strings.HasSuffix(entries, "OPEN:IN_REVIEW:2,APPROVE:APPROVED:2") {
		t.Fatal("a late fact changed the approved history", entries)
	}
	var accepted int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_events WHERE org_id=$1 AND trip_id=$2`, x.scope.OrgID().String(), trip).Scan(&accepted); err != nil || accepted != 1 {
		t.Fatal("held facts entered the logbook", accepted, err)
	}
	// The database refuses a direct fact on the approved revision, even with
	// a valid source operation (the guard runs before the identity checks).
	err = unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_events(org_id,id,trip_id,operation_id,payload_digest,event_type,event_text,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds)
SELECT org_id,id,trip_id,operation_id,payload_digest,event_type,event_text,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,entry_zone,device_offset_seconds FROM org_trip_events WHERE org_id=$1 AND trip_id=$2`,
		x.scope.OrgID().String(), trip)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "23514" || pe.Message != "trip revision closed" {
		t.Fatal("a direct fact on an approved revision was not refused", err)
	}
}

// B-BR-EMP-364 / BR-OFFL-005 race: two submissions of one draft revision
// from the author's queue, in both orders, leave one SUBMIT and one held fact.
func TestTripDuplicateSubmissionsRaceLeaveOneSubmit(t *testing.T) {
	for _, order := range []string{"first-wins", "second-wins"} {
		t.Run(order, func(t *testing.T) {
			x := tripAmendFixture(t)
			d, at := x.assignedDriver(t)
			trip, _ := x.completed(t, d, at)
			one, two := x.submitOp(t, d, trip, 1, at.Add(2*time.Hour), nil), x.submitOp(t, d, trip, 1, at.Add(2*time.Hour+time.Second), nil)
			if order == "second-wins" {
				one, two = two, one
			}
			config := tripAmendIntakeConfig(x.syncFixtureState)
			config.Pool = x.f.RuntimePool(t, 4)
			store := syncpg.NewStore(config)
			barrier := barrierHandler{Handler: x.submit, winner: d.source.MembershipID, inside: make(chan struct{}), proceed: make(chan struct{}), once: &sync.Once{}}
			acks := map[string]protocol.Acknowledgement{}
			errs := map[string]error{}
			var mu sync.Mutex
			var done sync.WaitGroup
			run := func(op protocol.Operation) {
				done.Go(func() {
					ack, err := store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), barrier)
					mu.Lock()
					acks[op.ID()], errs[op.ID()] = ack, err
					mu.Unlock()
				})
			}
			run(one)
			select {
			case <-barrier.inside:
			case <-time.After(30 * time.Second):
				close(barrier.proceed)
				done.Wait()
				t.Fatal("the first submission never reached its admission")
			}
			run(two)
			waited := false
			for deadline := time.Now().Add(10 * time.Second); !waited && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				var waiting int
				if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				waited = waiting > 0
			}
			close(barrier.proceed)
			done.Wait()
			if !waited {
				t.Fatal("the second submission never waited on the first's lock")
			}
			if a := acks[one.ID()]; errs[one.ID()] != nil || a.Result() != protocol.Accepted || *a.Snapshot().EntityRevision != 1 {
				t.Fatal("the first submission was not accepted", errs[one.ID()])
			}
			if a := acks[two.ID()]; errs[two.ID()] != nil || a.Result() != protocol.AcceptedForReview || a.Code() != tripapp.StaleRevision {
				t.Fatal("the duplicate submission was not held", errs[two.ID()], a.Result(), a.Code())
			}
			if entries, _ := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" {
				t.Fatal("a duplicate submission was counted", entries)
			}
		})
	}
}

// B-BR-EMP-365 race: two proposals naming the approved revision, in both
// orders with a lock-wait barrier, leave one amendment; the other is CONFLICT.
func TestTripAmendmentProposalsRaceInBothOrders(t *testing.T) {
	for _, order := range []string{"end-first", "start-first"} {
		t.Run(order, func(t *testing.T) {
			x := tripAmendFixture(t)
			d, at := x.assignedDriver(t)
			trip := x.measuredTrip(t, d, at)
			x.approve(t, x.owner(), trip, 1)
			first, second := []*tripapp.Correction{nil, odometer(184_600_000)}, []*tripapp.Correction{odometer(184_310_000), nil}
			if order == "start-first" {
				first, second = second, first
			}
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			gated, proceed := make(chan int, 1), make(chan struct{})
			held := *x.reviews
			held.Periods = heldPeriodGate{hold: func(pid int) { gated <- pid; <-proceed }}
			type outcome struct {
				result tripapp.AmendmentResult
				err    error
			}
			opOne, opTwo := onboardingID(t), onboardingID(t)
			winner := make(chan outcome, 1)
			go func() {
				r, _, err := x.propose(&held, x.manager, trip, 1, first[0], first[1], "Primeira correção", opOne)
				winner <- outcome{r, err}
			}()
			holder := <-gated
			loser, done := make(chan outcome, 1), make(chan struct{})
			go func() {
				r, _, err := x.propose(x.reviews, x.manager, trip, 1, second[0], second[1], "Segunda correção", opTwo)
				loser <- outcome{r, err}
				close(done)
			}()
			periodAwaitWaiter(t, ctx, x.legalFixtureState, holder, done)
			close(proceed)
			w, l := <-winner, <-loser
			if w.err != nil || w.result.Revision != 2 {
				t.Fatal("the proposal holding the trip was not recorded", w.err)
			}
			if !errors.Is(l.err, amend.ErrConflict) {
				t.Fatal("the racing proposal was not CONFLICT", l.err)
			}
			if entries, _ := x.history(t, x.scope, trip); !strings.HasSuffix(entries, "APPROVE:APPROVED:1,AMEND:SUBMITTED:2") {
				t.Fatal("two amendments were recorded", entries)
			}
		})
	}
}

// noSupersession is a defective reading port: it supersedes nothing.
type noSupersession struct{}

func (noSupersession) SupersedeApprovedTx(context.Context, pgx.Tx, kernel.OrgScope, fa.FleetActor, string, fa.AmendmentSupersession) error {
	return nil
}

// B-BR-EMP-368 / EDR-234..237: the database refuses an amendment without
// lineage, of a reading the trip does not use, with a forged value, an
// AMEND without its content, a review of an amendment by its author and an
// approval that did not supersede its readings; amendments and held
// submissions are insert-only and forced RLS.
func TestTripAmendmentSchemaGuards(t *testing.T) {
	x := tripAmendFixture(t)
	d, at := x.assignedDriver(t)
	org := x.scope.OrgID().String()
	draft := x.measuredTrip(t, d, at)
	endReading, startReading := x.finishReading(t, draft)
	code := func(name string, err error, want, message string) {
		t.Helper()
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != want || message != "" && pe.Message != message {
			t.Fatal("database guard did not refuse", name, err)
		}
	}
	insert := `INSERT INTO org_trip_amendments(org_id,trip_id,revision,base_revision,intent,end_meter_id,end_reading_id,end_from,end_to,end_new_reading_id,actor_membership_id,actor_employee_id,operation_id,request_hash)
VALUES($1,$2,$3,$4,'AMEND',$5,$6,$7,$8,$9,$10,$11,$12,repeat('a',64))`
	amendRow := func(trip, reading string, revision, base, from int64) error {
		return unitSchemaExec(t, x.legalFixtureState, insert, org, trip, revision, base, x.vehicle.MeterID, reading, from, from-10_000, onboardingID(t), x.manager.member, x.manager.employee, onboardingID(t))
	}
	code("amend a never-approved trip", amendRow(draft, endReading, 2, 1, 184_610_000), "23514", "invalid trip amendment lineage")
	x.approve(t, x.owner(), draft, 1)
	code("the start reading as the end", amendRow(draft, startReading, 2, 1, 184_320_000), "23514", "trip amendment reading unavailable")
	code("a forged original value", amendRow(draft, endReading, 2, 1, 184_610_001), "23514", "trip amendment reading unavailable")
	code("a skipped revision", amendRow(draft, endReading, 3, 1, 184_610_000), "23514", "invalid trip amendment lineage")
	// A well-formed content row without its AMEND entry fails at commit, and
	// an AMEND entry without content fails at once.
	code("content without entry", amendRow(draft, endReading, 2, 1, 184_610_000), "23514", "trip amendment publication unavailable")
	code("entry without content", unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,4,'AMEND',2,'SUBMITTED','Sem conteúdo',$3,$4,$5)`,
		org, draft, x.manager.member, x.manager.employee, onboardingID(t)), "23514", "invalid trip review transition")
	if _, _, err := x.propose(x.reviews, x.manager, draft, 1, nil, odometer(184_600_000), "Leitura final digitada errada", onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	code("the amendment's author opens it", unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,5,'OPEN',2,'IN_REVIEW','',$3,$4,$5)`,
		org, draft, x.manager.member, x.manager.employee, onboardingID(t)), "23514", "trip self approval refused")
	x.approve(t, x.owner(), draft, 2)
	// A defective reading port: the approval cannot commit without the
	// supersession of exactly its readings.
	defective := *x.reviews
	defective.Readings = noSupersession{}
	second := x.measuredTripAfter(t, d, at.Add(6*time.Hour))
	x.approve(t, x.owner(), second, 1)
	if _, _, err := x.propose(x.reviews, x.manager, second, 1, nil, odometer(184_900_000), "Leitura final digitada errada", onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	command, err := tripapp.NewDecisionCommand(tripapp.DecisionInput{OperationID: onboardingID(t), TripID: second, ExpectedRevision: 2, Decision: review.Approve})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := defective.DecideTripReview(t.Context(), x.scope, x.owner().actor, x.owner().member, command); !errors.Is(err, tripapp.ErrUnavailable) {
		t.Fatal("an approval without its supersession committed", err)
	}
	if entries, _ := x.history(t, x.scope, second); !strings.HasSuffix(entries, "AMEND:SUBMITTED:2") {
		t.Fatal("a refused approval left history", entries)
	}
	// Insert-only and forced RLS.
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_amendments,org_trip_held_submissions TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_trip_amendments SET end_to=0 WHERE org_id=$1`, `DELETE FROM org_trip_amendments WHERE org_id=$1`, `DELETE FROM org_trip_held_submissions WHERE org_id=$1`} {
		code(statement, unitSchemaExec(t, x.legalFixtureState, statement, org), "55000", "")
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_amendments)+(SELECT count(*) FROM org_trip_held_submissions)`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("amendments visible without the organization setting", visible, err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_amendments`).Scan(&visible)
	}); err != nil || visible != 2 {
		t.Fatal("amendments in scope", visible, err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN('org_trip_amendments'::regclass,'org_trip_held_submissions'::regclass)`).Scan(&forced); err != nil || !forced {
		t.Fatal("new histories are not forced RLS", err)
	}
}

// measuredTripAfter is a second measured trip whose readings continue the
// odometer series (184 620 000 m to 184 910 000 m).
func (x tripAmendState) measuredTripAfter(t *testing.T, d syncFixtureState, at time.Time) string {
	t.Helper()
	start := x.startOp(t, d, at, func(_, payload map[string]any) { payload["reading"] = x.reading(t, "184620000") })
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 1)
	trip := tripOf(start)
	finish := x.finishOp(t, d, trip, at.Add(3*time.Hour), func(_, payload map[string]any) { payload["reading"] = x.reading(t, "184910000") })
	ack, err = x.process(t, d, finish)
	x.accepted(t, finish, ack, err, 2, 1)
	x.submitted(t, d, trip, at.Add(3*time.Hour+5*time.Minute))
	return trip
}
