//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// conflictState is the trip review fixture with the real trip.event.record
// owner on the same intake and the real EMP-TRIP-007 conflict store.
type conflictState struct {
	tripReviewState
	event     syncapp.Handler
	lifecycle map[string]syncapp.Handler
	conflicts *trippg.ConflictStore
}

// conflictGrants are the runtime privileges of the conflict owner: the review
// items and resolutions, every held trip table, the structure lookup and the
// rows an accepted start writes (README proposal in the evidence).
func conflictGrants(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_review_resolutions,org_sync_effects,org_trips,org_meter_readings TO "+role+
		";GRANT SELECT ON org_sync_review_items,org_trip_held_starts,org_trip_held_finishes,org_trip_held_events,org_trip_held_submissions,org_trip_finishes,org_team_links,"+
		"org_asset_unit_periods,org_asset_cost_centre_periods,org_units,org_meters,org_meter_segments,org_device_enrollments,company_web_sessions,org_trip_transitions,org_trip_held_transitions TO "+role); err != nil {
		t.Fatal(err)
	}
}

func conflictStoreOf(t *testing.T, x tripReviewState) *trippg.ConflictStore {
	t.Helper()
	conflictGrants(t, x.f)
	catalog, err := audit.NewCatalog(trippg.ConflictAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &trippg.ConflictStore{Pool: x.f.Runtime, Authority: &orgpg.TripConflictAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles,
		Capabilities: x.flags, Structure: &orgpg.ResourceStructure{Kind: orgpg.HeldTripStructure, Pool: x.f.Runtime}}, Audit: auditpg.NewStore(x.f.Runtime, catalog),
		Allocations: x.assets, Readings: x.meters.Reviews}
}

func conflictFixture(t *testing.T) conflictState {
	t.Helper()
	r := tripReviewFixture(t)
	eventGrants(t, r.tripState)
	x := conflictState{tripReviewState: r}
	events := &trippg.EventStore{NewID: func() (string, error) { return onboardingID(t), nil }, Reports: &maintpg.IncidentStore{}}
	var err error
	if x.event, err = trippg.NewEventHandler(r.authority, r.meters, r.store, events); err != nil {
		t.Fatal(err)
	}
	// EMP-TRIP-011: the lifecycle owners on the same intake.
	x.lifecycle = map[string]syncapp.Handler{}
	for _, kind := range tripapp.LifecycleOperationTypes {
		handler, err := trippg.NewTransitionHandler(kind, r.authority, r.meters, r.store, r.store)
		if err != nil {
			t.Fatal(err)
		}
		x.lifecycle[kind] = handler
	}
	config := tripReviewIntakeConfig(r.syncFixtureState)
	config.NoAsyncPlans[syncapp.OperationKey{Type: tripapp.EventOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup,
		protocol.Rejected: syncapp.NoAsyncFollowup, protocol.AcceptedForReview: syncapp.AtomicReview}
	x.intake = syncpg.NewStore(config)
	x.conflicts = conflictStoreOf(t, r)
	return x
}

func (x conflictState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	if op.Snapshot().Type == tripapp.EventOperationType {
		return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.event)
	}
	if handler, ok := x.lifecycle[op.Snapshot().Type]; ok {
		return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), handler)
	}
	return x.tripReviewState.process(t, d, op)
}

// deliver processes op and requires the given result and code.
func (x conflictState) deliver(t *testing.T, d syncFixtureState, op protocol.Operation, result, code string) protocol.Acknowledgement {
	t.Helper()
	ack, err := x.process(t, d, op)
	if err != nil || ack.Result() != result || ack.Code() != code {
		t.Fatal("unexpected delivery", op.Snapshot().Type, err, ack.Result(), ack.Code())
	}
	return ack
}

func (x conflictState) eventOp(t *testing.T, d syncFixtureState, trip, kind string, text any, declared time.Time) protocol.Operation {
	t.Helper()
	return eventState{tripState: x.tripState}.eventOp(t, d, trip, kind, text, declared, nil)
}

// resolve never calls t.Fatal, so it is safe inside a goroutine.
func (x conflictState) resolve(r tripReviewer, item string, revision int64, kind tripapp.ConflictDecisionKind, reason, operation string) (protocol.ReviewResolution, bool, error) {
	command, err := tripapp.NewConflictDecision(tripapp.ConflictDecisionInput{OperationID: operation, ReviewItemID: item, ExpectedRevision: revision, Decision: kind, Reason: reason})
	if err != nil {
		return protocol.ReviewResolution{}, false, err
	}
	return x.conflicts.ResolveTripConflict(context.Background(), x.scope, r.actor, r.member, command)
}

func (x conflictState) readConflict(r tripReviewer, item string) (tripapp.ConflictView, error) {
	return x.conflicts.ReadTripConflict(context.Background(), x.scope, r.actor, r.member, item)
}

// evidence is everything one held operation keeps: its review item, held
// row, original acknowledgement bytes and its resolution.
func (x conflictState) evidence(t *testing.T, op protocol.Operation) string {
	t.Helper()
	var v string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT to_jsonb(i) FROM org_sync_review_items i WHERE i.org_id=$1 AND i.operation_id=$2),
 (SELECT to_jsonb(h) FROM org_trip_held_starts h WHERE h.org_id=$1 AND h.operation_id=$2),(SELECT to_jsonb(h) FROM org_trip_held_finishes h WHERE h.org_id=$1 AND h.operation_id=$2),
 (SELECT to_jsonb(h) FROM org_trip_held_events h WHERE h.org_id=$1 AND h.operation_id=$2),(SELECT to_jsonb(h) FROM org_trip_held_submissions h WHERE h.org_id=$1 AND h.operation_id=$2),
 (SELECT encode(canonical_ack,'hex') FROM org_sync_acknowledgements a WHERE a.org_id=$1 AND a.operation_id=$2))::text`, x.scope.OrgID().String(), op.ID()).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

type conflictCounts struct{ resolutions, audits, trips, effects, readings int }

func (x conflictState) conflictCount(t *testing.T, op protocol.Operation) conflictCounts {
	t.Helper()
	var c conflictCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_resolutions z JOIN org_sync_review_items i ON(i.org_id,i.id)=(z.org_id,z.review_item_id) WHERE i.org_id=$1 AND i.operation_id=$2),
 (SELECT count(*) FROM org_audit_events a JOIN org_sync_review_items i ON(i.org_id,i.id)=(a.org_id,a.target_id) WHERE i.org_id=$1 AND i.operation_id=$2 AND a.action='offl.review.resolved'),
 (SELECT count(*) FROM org_trips WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
 (SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.resolutions, &c.audits, &c.trips, &c.effects, &c.readings); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x conflictState) inProgress(t *testing.T) int {
	t.Helper()
	var n int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trips WHERE org_id=$1 AND asset_id=$2 AND state='IN_PROGRESS'`, x.scope.OrgID().String(), x.vehicle.AssetID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (x conflictState) admin() tripReviewer {
	return tripReviewer{actor: tripapp.ReviewActor{AccountID: x.person.web.AccountID, SessionID: x.person.web.SessionID}, member: x.person.member, account: x.person.auth.Account.ID}
}

// busyThenFree runs trip R of member a from at (the asset is busy), holds a
// start of member b declared at at+2h ASSET_BUSY, then finishes R at at+1h:
// b's held start overlaps no trip any more (B-BR-EMP-409).
func (x conflictState) busyThenFree(t *testing.T, a, b syncFixtureState, at time.Time) (protocol.Operation, protocol.Acknowledgement) {
	t.Helper()
	running := x.startOp(t, a, at, absentReading)
	x.deliver(t, a, running, protocol.Accepted, "")
	held := x.startOp(t, b, at.Add(2*time.Hour), nil)
	ack := x.deliver(t, b, held, protocol.AcceptedForReview, tripapp.AssetBusy)
	x.deliver(t, a, x.finishOp(t, a, tripOf(running), at.Add(time.Hour), absentReading), protocol.Accepted, "")
	return held, ack
}

// BUC-EMP-206/208 and B-BR-EMP-407: every held trip owner is listed in item
// order with its state, read with its exact fact and the trips it conflicts
// with; meter, unknown and foreign items stay invisible.
func TestTripConflictQueueListsAndReadsEveryHeldTripFact(t *testing.T) {
	x := conflictFixture(t)
	org := x.scope.OrgID().String()
	owner := x.owner()
	diego, bruno := x.syncFixtureState, func() syncFixtureState { d, _ := x.assignedDriver(t); return d }()
	at := x.declaredNow(t)
	// VAL-07: Diego's 08:05 start is received first; Bruno's 08:00 start is held.
	diegoStart := x.startOp(t, diego, at.Add(5*time.Minute), nil)
	x.deliver(t, diego, diegoStart, protocol.Accepted, "")
	brunoStart := x.startOp(t, bruno, at, func(_, payload map[string]any) {
		payload["run_clock"] = map[string]any{"boot_id": "boot-b1", "elapsed_millis": "3600000"}
	})
	startAck := x.deliver(t, bruno, brunoStart, protocol.AcceptedForReview, tripapp.AssetBusy)
	// A completed trip of Diego with a second finish, a late event and a stale submission.
	diegoTrip := tripOf(diegoStart)
	x.deliver(t, diego, x.finishOp(t, diego, diegoTrip, at.Add(3*time.Hour), nil), protocol.Accepted, "")
	secondFinish := x.finishOp(t, diego, diegoTrip, at.Add(4*time.Hour), absentReading)
	x.deliver(t, diego, secondFinish, protocol.AcceptedForReview, tripapp.TripAlreadyClosed)
	late := x.eventOp(t, diego, diegoTrip, "NOTE", "Abastecimento esquecido", at.Add(5*time.Hour))
	x.deliver(t, diego, late, protocol.AcceptedForReview, tripapp.TripAlreadyClosed)
	x.submitted(t, diego, diegoTrip, at.Add(6*time.Hour))
	stale := x.submitOp(t, diego, diegoTrip, 1, at.Add(7*time.Hour), nil)
	x.deliver(t, diego, stale, protocol.AcceptedForReview, tripapp.StaleRevision)
	// EMP-TRIP-011: a pause of the completed trip is held TRIP_ALREADY_CLOSED.
	pause := x.operation(t, diego, tripapp.PauseOperationType, diegoTrip, at.Add(8*time.Hour), map[string]any{"reason": nil}, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": diegoTrip, "expected_revision": 2}
	})
	x.deliver(t, diego, pause, protocol.AcceptedForReview, tripapp.TripAlreadyClosed)

	page, err := x.conflicts.ListTripConflicts(context.Background(), x.scope, owner.actor, owner.member, tripapp.ConflictFilter{Limit: 10})
	if err != nil || len(page.Items) != 5 || page.NextID != "" {
		t.Fatal("conflict queue", err, len(page.Items))
	}
	kinds := map[string]tripapp.ConflictView{}
	for i, v := range page.Items {
		if i > 0 && page.Items[i-1].ReviewItemID >= v.ReviewItemID || v.State != tripapp.ConflictOpen || v.Revision != 1 || v.Resolution != nil {
			t.Fatal("queue order or state", v.ReviewItemID, v.State)
		}
		kinds[v.OperationType] = v
	}
	start := kinds[tripapp.StartOperationType]
	if start.Code != tripapp.AssetBusy || start.ReviewItemID != startAck.Snapshot().ReviewItemID || start.AuthorEmployeeID != bruno.person.employee || start.AuthorMembershipID != bruno.person.member ||
		start.Fact.Start == nil || start.Fact.Start.AssetID != x.vehicle.AssetID || start.Fact.Start.DriverID != bruno.person.employee || start.Fact.Start.ReadingValue == nil ||
		*start.Fact.Start.ReadingValue != 184_320_000 || start.Fact.Start.ReadingKind != "ODOMETER" || start.Fact.Start.Clock == nil || start.Fact.Start.Clock.BootID() != "boot-b1" ||
		!start.Fact.Declared.OccurredAt.Equal(at) || len(start.Trips) != 1 || start.Trips[0].TripID != diegoTrip || start.Trips[0].State != tripdomain.Completed {
		t.Fatal("held start view lost its exact fact or its conflicting trip", start.Fact.Start, start.Trips)
	}
	finish, event, submission := kinds[tripapp.FinishOperationType], kinds[tripapp.EventOperationType], kinds[tripapp.SubmitOperationType]
	if finish.Code != tripapp.TripAlreadyClosed || finish.Fact.Finish == nil || finish.Fact.Finish.MissingReason == nil || *finish.Fact.Finish.MissingReason != "Painel apagado" || len(finish.Trips) != 1 || finish.Trips[0].TripID != diegoTrip ||
		event.Code != tripapp.TripAlreadyClosed || event.Fact.Event == nil || event.Fact.Event.EventType != "NOTE" || event.Fact.Event.Text == nil || *event.Fact.Event.Text != "Abastecimento esquecido" ||
		submission.Code != tripapp.StaleRevision || submission.Fact.SubmittedRevision == nil || *submission.Fact.SubmittedRevision != 1 || submission.Fact.TripID != diegoTrip {
		t.Fatal("held finish, event or submission view drift", finish.Fact, event.Fact, submission.Fact)
	}
	paused := kinds[tripapp.PauseOperationType]
	if paused.Code != tripapp.TripAlreadyClosed || paused.Fact.Transition == nil || paused.Fact.Transition.Command != "PAUSE" || paused.Fact.Transition.ExpectedRevision != 2 ||
		paused.Fact.Transition.Reason != nil || len(paused.Trips) != 1 || paused.Trips[0].TripID != diegoTrip || paused.Trips[0].State != tripdomain.Completed {
		t.Fatal("held lifecycle command view drift", paused.Fact.Transition, paused.Trips)
	}
	// Keyset pages and the state filter.
	first, err := x.conflicts.ListTripConflicts(context.Background(), x.scope, owner.actor, owner.member, tripapp.ConflictFilter{Limit: 3})
	if err != nil || len(first.Items) != 3 || first.NextID != first.Items[2].ReviewItemID {
		t.Fatal("first page", err)
	}
	rest, err := x.conflicts.ListTripConflicts(context.Background(), x.scope, owner.actor, owner.member, tripapp.ConflictFilter{Limit: 3, AfterID: first.NextID})
	if err != nil || len(rest.Items) != 2 || rest.Items[0].ReviewItemID != page.Items[3].ReviewItemID || rest.Items[1].ReviewItemID != page.Items[4].ReviewItemID || rest.NextID != "" {
		t.Fatal("second page", err)
	}
	for state, want := range map[string]int{tripapp.ConflictOpen: 5, tripapp.ConflictExpired: 0, tripapp.ConflictAccepted: 0, tripapp.ConflictRefused: 0} {
		if p, err := x.conflicts.ListTripConflicts(context.Background(), x.scope, owner.actor, owner.member, tripapp.ConflictFilter{Limit: 10, State: state}); err != nil || len(p.Items) != want {
			t.Fatal("state filter", state, err)
		}
	}
	// Reads equal the listed items; unknown, foreign and meter items are NOT_FOUND.
	for _, v := range page.Items {
		read, err := x.readConflict(owner, v.ReviewItemID)
		if err != nil || read.ReviewItemID != v.ReviewItemID || read.OperationType != v.OperationType || !read.ReceivedAt.Equal(v.ReceivedAt) {
			t.Fatal("conflict read", err)
		}
	}
	if _, err := x.readConflict(owner, onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("unknown item", err)
	}
	other := legalFixtureFor(t, x.f, x.onboarding, x.legalFixtureState.actor, x.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	stranger := tripReviewer{actor: tripapp.ReviewActor{AccountID: other.actor.AccountID, SessionID: other.actor.SessionID}, member: other.member}
	if _, err := x.conflicts.ReadTripConflict(context.Background(), other.scope, stranger.actor, stranger.member, start.ReviewItemID); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization read a held trip fact", err)
	}
	command, _ := tripapp.NewConflictDecision(tripapp.ConflictDecisionInput{OperationID: onboardingID(t), ReviewItemID: start.ReviewItemID, ExpectedRevision: 1, Decision: tripapp.ConflictRefuse, Reason: "Estranho"})
	if _, _, err := x.conflicts.ResolveTripConflict(context.Background(), other.scope, stranger.actor, stranger.member, command); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization resolved a held trip fact", err)
	}
	if p, err := x.conflicts.ListTripConflicts(context.Background(), other.scope, stranger.actor, stranger.member, tripapp.ConflictFilter{Limit: 10}); err != nil || len(p.Items) != 0 {
		t.Fatal("another organization listed held trip facts", err)
	}
	var meterItems int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_type NOT LIKE 'trip.%'`, org).Scan(&meterItems); err != nil || meterItems != 0 {
		t.Fatal("unexpected non-trip items", meterItems, err)
	}
	if c := x.conflictCount(t, brunoStart); c != (conflictCounts{}) {
		t.Fatalf("a read or list wrote state: %+v", c)
	}
	// A held lifecycle command is only refused (B-BR-EMP-408).
	if _, _, err := x.resolve(owner, paused.ReviewItemID, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
		t.Fatal("a held pause was accepted", err)
	}
	if r, _, err := x.resolve(owner, paused.ReviewItemID, 1, tripapp.ConflictRefuse, "Viagem já encerrada", onboardingID(t)); err != nil || r.Snapshot().Outcome != protocol.ReviewRefused {
		t.Fatal("refusal of a held pause", err)
	}
	if x.stateOf(t, diegoTrip) != "COMPLETED" {
		t.Fatal("a refused lifecycle command changed the trip")
	}
}

func (x conflictState) stateOf(t *testing.T, trip string) string {
	t.Helper()
	var state string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_trips WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), trip).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

// BUC-EMP-206 / VAL-07 and B-BR-EMP-408..411: an overlapping held start is
// refused, never accepted; the refusal is one audited resolution replayed
// once, reused never, decided again never; the evidence stays and the author
// reads the outcome; two reviewers racing leave one resolution.
func TestTripConflictRefusalKeepsTheEvidenceAndAnswersTheAuthor(t *testing.T) {
	for _, order := range []string{"elisa-first", "ivo-first"} {
		t.Run(order, func(t *testing.T) {
			x := conflictFixture(t)
			diego, bruno := x.syncFixtureState, func() syncFixtureState { d, _ := x.assignedDriver(t); return d }()
			at := x.declaredNow(t)
			diegoStart := x.startOp(t, diego, at.Add(5*time.Minute), nil)
			x.deliver(t, diego, diegoStart, protocol.Accepted, "")
			brunoStart := x.startOp(t, bruno, at, nil)
			ack := x.deliver(t, bruno, brunoStart, protocol.AcceptedForReview, tripapp.AssetBusy)
			item := ack.Snapshot().ReviewItemID
			before := x.evidence(t, brunoStart)
			owner := x.owner()
			// Diego is still driving and later finishes: Bruno's start overlaps both ways.
			if _, _, err := x.resolve(owner, item, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
				t.Fatal("an overlapping held start was accepted while the asset runs", err)
			}
			x.deliver(t, diego, x.finishOp(t, diego, tripOf(diegoStart), at.Add(4*time.Hour), nil), protocol.Accepted, "")
			if _, _, err := x.resolve(owner, item, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
				t.Fatal("a held start overlapping a finished trip was accepted", err)
			}
			if c := x.conflictCount(t, brunoStart); c != (conflictCounts{}) || x.evidence(t, brunoStart) != before {
				t.Fatalf("a refused acceptance wrote state: %+v", c)
			}
			if _, _, err := x.resolve(owner, item, 2, tripapp.ConflictRefuse, "Veículo em uso", onboardingID(t)); !errors.Is(err, review.ErrConflict) {
				t.Fatal("a stale revision was decided", err)
			}
			// Elisa and Ivo resolve at the same moment: exactly one resolution applies.
			x.conflicts.Pool = x.f.RuntimePool(t, 4)
			elisa, ivo := x.reviewerFor(t, "FLEET_MANAGER", "ORG", ""), x.reviewerFor(t, "SUPERVISOR", "ORG", "")
			type attempt struct {
				r      tripReviewer
				reason string
				op     string
			}
			first, second := attempt{elisa, "Veículo em uso por outra viagem", onboardingID(t)}, attempt{ivo, "Duplicado", onboardingID(t)}
			if order == "ivo-first" {
				first, second = second, first
			}
			pid, free, done := conflictHoldRoot(t, x)
			results := make(chan error, 2)
			var wg sync.WaitGroup
			run := func(v attempt) {
				wg.Go(func() {
					_, _, err := x.resolve(v.r, item, 1, tripapp.ConflictRefuse, v.reason, v.op)
					results <- err
				})
			}
			run(first)
			waitBlocked(t, x.syncFixtureState, pid, 1)
			run(second)
			waitBlocked(t, x.syncFixtureState, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			close(results)
			ok, lost := 0, 0
			for err := range results {
				switch {
				case err == nil:
					ok++
				case errors.Is(err, review.ErrConflict):
					lost++
				default:
					t.Fatal("unexpected race outcome", err)
				}
			}
			if ok != 1 || lost != 1 || x.conflictCount(t, brunoStart) != (conflictCounts{resolutions: 1, audits: 1}) {
				t.Fatal("two concurrent resolutions did not leave exactly one", ok, lost, x.conflictCount(t, brunoStart))
			}
			view, err := x.readConflict(owner, item)
			if err != nil || view.State != tripapp.ConflictRefused || view.Revision != 2 || view.Resolution == nil || view.Resolution.Snapshot().Reason != first.reason ||
				view.Resolution.Snapshot().ResolutionID != first.op || view.Resolution.Snapshot().EntityRevision != nil {
				t.Fatal("the first queued resolution is not the stored one", err, view.State)
			}
			// Replay, reuse and a second decision.
			replay, replayed, err := x.resolve(first.r, item, 1, tripapp.ConflictRefuse, first.reason, first.op)
			if err != nil || !replayed || !bytes.Equal(replay.CanonicalBytes(), view.Resolution.CanonicalBytes()) {
				t.Fatal("replay of the stored resolution", err, replayed)
			}
			if _, _, err := x.resolve(first.r, item, 1, tripapp.ConflictRefuse, "Outro motivo", first.op); !errors.Is(err, tripapp.ErrOperationReused) {
				t.Fatal("operation reuse", err)
			}
			if _, _, err := x.resolve(owner, item, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, review.ErrConflict) {
				t.Fatal("a second resolution was applied", err)
			}
			// B-BR-EMP-411: the evidence is unchanged and immutable; nothing became a trip.
			if x.evidence(t, brunoStart) != before || x.conflictCount(t, brunoStart) != (conflictCounts{resolutions: 1, audits: 1}) {
				t.Fatal("the resolution changed the held evidence")
			}
			for _, statement := range []string{`UPDATE org_trip_held_starts SET entry_zone='UTC' WHERE org_id=$1`, `DELETE FROM org_trip_held_starts WHERE org_id=$1`,
				`UPDATE org_sync_review_resolutions SET reason='x' WHERE org_id=$1`, `DELETE FROM org_sync_review_resolutions WHERE org_id=$1`, `DELETE FROM org_sync_review_items WHERE org_id=$1`} {
				err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
					_, err := tx.Exec(t.Context(), statement, x.scope.OrgID().String())
					return err
				})
				var pg *pgconn.PgError
				if !errors.As(err, &pg) || pg.Code != "55000" && pg.Code != "42501" {
					t.Fatal("held evidence was rewritable", statement, err)
				}
			}
			// The author's signed own-result read answers the refusal.
			results2, err := x.meters.Reviews.OwnMeterReviewResults(t.Context(), bruno.source, []protocol.ReviewSelector{{OperationID: brunoStart.ID(), PayloadDigest: brunoStart.PayloadDigest()}})
			if err != nil || len(results2) != 1 || results2[0].Resolution == nil || !bytes.Equal(results2[0].Resolution.CanonicalBytes(), view.Resolution.CanonicalBytes()) {
				t.Fatal("the author's own result read of the refusal", err)
			}
			if _, err := x.meters.Reviews.OwnMeterReviewResults(t.Context(), diego.source, []protocol.ReviewSelector{{OperationID: brunoStart.ID(), PayloadDigest: brunoStart.PayloadDigest()}}); err == nil {
				t.Fatal("another member read the author's outcome")
			}
			if x.inProgress(t) != 0 {
				t.Fatal("a refusal made a trip active")
			}
		})
	}
}

// conflictHoldRoot holds the organization root row in its own transaction.
func conflictHoldRoot(t *testing.T, x conflictState) (int, func(), chan error) {
	t.Helper()
	holder := make(chan int, 1)
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free)
	done := make(chan error, 1)
	go func() {
		done <- database.WithOrgTx(context.Background(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), `SELECT state FROM organizations WHERE org_id=$1 FOR UPDATE`, x.scope.OrgID().String()); err != nil {
				return err
			}
			var pid int
			if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			holder <- pid
			<-release
			return nil
		})
	}()
	select {
	case pid := <-holder:
		return pid, free, done
	case err := <-done:
		t.Fatal("holder failed", err)
	}
	return 0, free, done
}

// BUC-EMP-207 and B-BR-EMP-408/411: a held start that overlaps no trip any
// more is accepted as the trip it declared: in progress at revision 1 with
// its TRIP_START reading at the original receipt and readiness, so its
// pending finish completes it; the author reads ACCEPTED.
func TestTripConflictAcceptanceStoresTheDeclaredTripAndItsPendingFinish(t *testing.T) {
	x := conflictFixture(t)
	a := x.syncFixtureState
	b, _ := x.assignedDriver(t)
	at := x.declaredNow(t)
	held, ack := x.busyThenFree(t, a, b, at)
	item, trip := ack.Snapshot().ReviewItemID, tripOf(held)
	before := x.evidence(t, held)
	pending := x.finishOp(t, b, trip, at.Add(3*time.Hour), func(envelope, _ map[string]any) { envelope["depends_on"] = []any{held.ID()} })
	x.deliver(t, b, pending, protocol.Retry, "DEPENDENCY_PENDING")
	view, err := x.readConflict(x.owner(), item)
	if err != nil || len(view.Trips) != 0 || view.State != tripapp.ConflictOpen {
		t.Fatal("the freed held start still conflicts", err, view.Trips)
	}
	// The author and its device are the declared ones (self-review refused first).
	if _, _, err := x.resolve(tripReviewer{actor: tripapp.ReviewActor{AccountID: b.person.web.AccountID, SessionID: b.person.web.SessionID}, member: b.person.member}, item, 1, tripapp.ConflictAccept, "", onboardingID(t)); err == nil {
		t.Fatal("the driver accepted its own held start")
	}
	op := onboardingID(t)
	resolution, replayed, err := x.resolve(x.owner(), item, 1, tripapp.ConflictAccept, "Liberado pela gestão", op)
	if err != nil || replayed || resolution.Snapshot().Outcome != protocol.ReviewAccepted || resolution.Snapshot().EntityRevision == nil || *resolution.Snapshot().EntityRevision != 1 {
		t.Fatal("acceptance of the freed held start", err)
	}
	if c := x.conflictCount(t, held); c != (conflictCounts{resolutions: 1, audits: 1, trips: 1, effects: 1, readings: 1}) || x.evidence(t, held) != before || x.inProgress(t) != 1 {
		t.Fatalf("accepted start not stored exactly once: %+v", c)
	}
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT t.state='IN_PROGRESS' AND t.author_membership_id=$3 AND t.driver_employee_id=$4 AND t.enrollment_id=$5 AND t.payload_digest=$6
 AND t.received_at=k.received_at AND t.unit_id=$7 AND t.reporting_day=(t.occurred_at AT TIME ZONE t.reporting_zone)::date
 AND (t.occurred_at,t.occurred_submicro)=(h.occurred_at,h.occurred_submicro) AND t.reading_id=h.reading_id
 AND EXISTS(SELECT 1 FROM org_meter_readings r WHERE r.org_id=t.org_id AND r.id=t.reading_id AND r.origin='TRIP_START' AND r.value=184320000 AND r.received_at=k.received_at
  AND r.review_resolution_id=$8 AND r.operation_id=t.operation_id AND r.capture_kind='ENROLLED_DEVICE')
 FROM org_trips t JOIN org_trip_held_starts h ON(h.org_id,h.trip_id)=(t.org_id,t.id) JOIN org_sync_acknowledgements k ON(k.org_id,k.operation_id)=(t.org_id,t.operation_id)
 WHERE t.org_id=$1 AND t.id=$2`, x.scope.OrgID().String(), trip, b.person.member, b.person.employee, b.source.EnrollmentID, held.PayloadDigest(), x.unit, op).Scan(&exact); err != nil || !exact {
		t.Fatal("the accepted trip is not exactly its held start at its original receipt", err)
	}
	if replay, replayed, err := x.resolve(x.owner(), item, 1, tripapp.ConflictAccept, "Liberado pela gestão", op); err != nil || !replayed || !bytes.Equal(replay.CanonicalBytes(), resolution.CanonicalBytes()) {
		t.Fatal("acceptance replay", err)
	}
	// The original receipt is unchanged and resends return it byte for byte.
	x.replays(t, b, held, ack)
	// The pending finish now completes the accepted trip.
	x.deliver(t, b, pending, protocol.Accepted, "")
	if x.inProgress(t) != 0 {
		t.Fatal("the pending finish did not complete the accepted trip")
	}
	if view, err := x.readConflict(x.owner(), item); err != nil || view.State != tripapp.ConflictAccepted || view.Revision != 2 || len(view.Trips) != 0 {
		t.Fatal("accepted view", err)
	}
	results, err := x.meters.Reviews.OwnMeterReviewResults(t.Context(), b.source, []protocol.ReviewSelector{{OperationID: held.ID(), PayloadDigest: held.PayloadDigest()}})
	if err != nil || len(results) != 1 || results[0].Resolution == nil || !bytes.Equal(results[0].Resolution.CanonicalBytes(), resolution.CanonicalBytes()) {
		t.Fatal("the author's own result read of the acceptance", err)
	}
}

// B-BR-EMP-409 / VAL-07: a resolution never produces a second active trip,
// racing a new start or another acceptance on the asset, in both orders.
func TestTripConflictResolutionNeverMakesTwoTripsActive(t *testing.T) {
	for _, race := range []string{"accept-then-start", "start-then-accept", "accept-first-of-two", "accept-second-of-two"} {
		t.Run(race, func(t *testing.T) {
			x := conflictFixture(t)
			a := x.syncFixtureState
			b, _ := x.assignedDriver(t)
			c, _ := x.assignedDriver(t)
			at := x.declaredNow(t)
			held, ack := x.busyThenFree(t, a, b, at)
			item := ack.Snapshot().ReviewItemID
			x.conflicts.Pool = x.f.RuntimePool(t, 4)
			x.intake = syncpg.NewStore(func() syncpg.Config {
				config := tripReviewIntakeConfig(x.syncFixtureState)
				config.Pool = x.f.RuntimePool(t, 4)
				return config
			}())
			second, otherItem := protocol.Operation{}, ""
			if race == "accept-first-of-two" || race == "accept-second-of-two" {
				// A second held start of c, declared later, also overlaps no trip.
				blocker := x.startOp(t, a, at.Add(70*time.Minute), absentReading)
				x.deliver(t, a, blocker, protocol.Accepted, "")
				second = x.startOp(t, c, at.Add(170*time.Minute), absentReading)
				otherItem = x.deliver(t, c, second, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
				x.deliver(t, a, x.finishOp(t, a, tripOf(blocker), at.Add(80*time.Minute), absentReading), protocol.Accepted, "")
			}
			reviewer := x.reviewerFor(t, "FLEET_MANAGER", "ORG", "")
			newStart := x.startOp(t, c, at.Add(150*time.Minute), absentReading)
			pid, free, done := conflictHoldRoot(t, x)
			type outcome struct {
				name string
				err  error
				ack  protocol.Acknowledgement
			}
			results := make(chan outcome, 2)
			var wg sync.WaitGroup
			accept := func(r tripReviewer, id string) func() {
				return func() {
					_, _, err := x.resolve(r, id, 1, tripapp.ConflictAccept, "", onboardingID(t))
					results <- outcome{name: "accept:" + id, err: err}
				}
			}
			start := func() {
				ack, err := x.intake.Process(context.Background(), c.source, newStart, protocol.DefaultLimits(), x.start)
				results <- outcome{name: "start", err: err, ack: ack}
			}
			var steps []func()
			switch race {
			case "accept-then-start":
				steps = []func(){accept(x.owner(), item), start}
			case "start-then-accept":
				steps = []func(){start, accept(x.owner(), item)}
			case "accept-first-of-two":
				steps = []func(){accept(x.owner(), item), accept(reviewer, otherItem)}
			case "accept-second-of-two":
				steps = []func(){accept(reviewer, otherItem), accept(x.owner(), item)}
			}
			for i, step := range steps {
				wg.Go(step)
				waitBlocked(t, x.syncFixtureState, pid, i+1)
			}
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			close(results)
			got := map[string]outcome{}
			for r := range results {
				got[r.name] = r
			}
			firstAccepted := !errors.Is(got["accept:"+item].err, tripapp.ErrConflictUnacceptable)
			switch race {
			case "accept-then-start":
				if got["accept:"+item].err != nil || got["start"].err != nil || got["start"].ack.Result() != protocol.AcceptedForReview || got["start"].ack.Code() != tripapp.AssetBusy {
					t.Fatal("accepted start then new start", got)
				}
			case "start-then-accept":
				if !errors.Is(got["accept:"+item].err, tripapp.ErrConflictUnacceptable) || got["start"].err != nil || got["start"].ack.Result() != protocol.Accepted {
					t.Fatal("new start then acceptance", got)
				}
				firstAccepted = false
			case "accept-first-of-two", "accept-second-of-two":
				winner, loser := item, otherItem
				if race == "accept-second-of-two" {
					winner, loser = otherItem, item
				}
				if got["accept:"+winner].err != nil || !errors.Is(got["accept:"+loser].err, tripapp.ErrConflictUnacceptable) {
					t.Fatal("two acceptances", got)
				}
				firstAccepted = winner == item
			}
			if x.inProgress(t) != 1 {
				t.Fatal("the race left a number of active trips other than one", x.inProgress(t))
			}
			if c := x.conflictCount(t, held); firstAccepted && c.trips != 1 || !firstAccepted && c != (conflictCounts{}) {
				t.Fatalf("held start state after the race: %+v", c)
			}
		})
	}
}

// B-BR-EMP-408/410: the ADMIN author holds trip.trip.review and still cannot
// resolve its own held start; accepting a finish, event or submission is a
// CONFLICT; AUDITOR, roles without the permission and uncovering structural
// grants are FORBIDDEN while covering UNIT and ASSET grants read and decide;
// the queue is organization-level.
func TestTripConflictRefusesSelfReviewUncoveredScopesAndAuditors(t *testing.T) {
	x := conflictFixture(t)
	b, _ := x.assignedDriver(t)
	at := x.declaredNow(t)
	// b drives; the ADMIN's own start is held behind b's trip.
	running := x.startOp(t, b, at, absentReading)
	x.deliver(t, b, running, protocol.Accepted, "")
	own := x.startOp(t, x.syncFixtureState, at.Add(time.Hour), nil)
	ownItem := x.deliver(t, x.syncFixtureState, own, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
	if _, _, err := x.resolve(x.admin(), ownItem, 1, tripapp.ConflictRefuse, "Meu próprio registro", onboardingID(t)); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("self review was not refused", err)
	}
	if view, err := x.readConflict(x.admin(), ownItem); err != nil || view.State != tripapp.ConflictOpen {
		t.Fatal("the author's own read", err)
	}
	// b's finish is declared before the ADMIN's start: the held start no
	// longer overlaps any trip once it arrives.
	x.deliver(t, b, x.finishOp(t, b, tripOf(running), at.Add(30*time.Minute), absentReading), protocol.Accepted, "")
	second := x.finishOp(t, b, tripOf(running), at.Add(3*time.Hour), absentReading)
	finishItem := x.deliver(t, b, second, protocol.AcceptedForReview, tripapp.TripAlreadyClosed).Snapshot().ReviewItemID
	late := x.eventOp(t, b, tripOf(running), "STOP", nil, at.Add(4*time.Hour))
	eventItem := x.deliver(t, b, late, protocol.AcceptedForReview, tripapp.TripAlreadyClosed).Snapshot().ReviewItemID
	for _, id := range []string{finishItem, eventItem} {
		if _, _, err := x.resolve(x.owner(), id, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
			t.Fatal("a held finish or event was accepted", err)
		}
	}
	sul := onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic Sul','SUL','UTC','ACTIVE')`, x.scope.OrgID().String(), sul); err != nil {
		t.Fatal(err)
	}
	denied := func(name string, r tripReviewer, item string) {
		t.Helper()
		if _, err := x.readConflict(r, item); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatal("read not FORBIDDEN", name, err)
		}
		if _, _, err := x.resolve(r, item, 1, tripapp.ConflictRefuse, "Synthetic", onboardingID(t)); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatal("resolution not FORBIDDEN", name, err)
		}
	}
	for _, item := range []string{finishItem, ownItem} {
		denied("auditor", x.reviewerFor(t, "AUDITOR", "ORG", ""), item)
		denied("finance", x.reviewerFor(t, "FINANCE", "ORG", ""), item)
		denied("driver", x.reviewerFor(t, "DRIVER", "SELF", ""), item)
		denied("supervisor of another unit", x.reviewerFor(t, "SUPERVISOR", "UNIT", sul), item)
		denied("fleet manager of another asset", x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.installed.AssetID), item)
	}
	if c := x.conflictCount(t, own); c != (conflictCounts{}) {
		t.Fatalf("a refused resolution wrote state: %+v", c)
	}
	norte := x.reviewerFor(t, "SUPERVISOR", "UNIT", x.unit)
	if _, err := x.conflicts.ListTripConflicts(context.Background(), x.scope, norte.actor, norte.member, tripapp.ConflictFilter{Limit: 5}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("a UNIT grant listed the organization queue", err)
	}
	auditor := x.reviewerFor(t, "AUDITOR", "ORG", "")
	if _, err := x.conflicts.ListTripConflicts(context.Background(), x.scope, auditor.actor, auditor.member, tripapp.ConflictFilter{Limit: 5}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("AUDITOR listed the queue", err)
	}
	// The held start's declared asset and its unit at the declaration, and the
	// named trip's stored structure, are covered.
	vehicle := x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.vehicle.AssetID)
	if view, err := x.readConflict(norte, ownItem); err != nil || view.ReviewItemID != ownItem {
		t.Fatal("covering UNIT read of a held start", err)
	}
	if view, err := x.readConflict(vehicle, finishItem); err != nil || view.ReviewItemID != finishItem {
		t.Fatal("covering ASSET read of a held finish", err)
	}
	if r, _, err := x.resolve(norte, eventItem, 1, tripapp.ConflictRefuse, "Evento após o fim da viagem", onboardingID(t)); err != nil || r.Snapshot().Outcome != protocol.ReviewRefused {
		t.Fatal("covering UNIT refusal of a held event", err)
	}
	if r, _, err := x.resolve(vehicle, ownItem, 1, tripapp.ConflictAccept, "", onboardingID(t)); err != nil || r.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("covering ASSET acceptance of the ADMIN's freed held start", err)
	}
	if x.inProgress(t) != 1 {
		t.Fatal("the accepted start is not the one active trip")
	}
}

// EDR-264: with the application bypassed the database refuses an accepted
// finish, an unproven trip identity, an overlapping accepted trip, an
// unready effect and an accepted resolution without its trip.
func TestTripConflictSchemaGuardsReproveEveryAcceptance(t *testing.T) {
	x := conflictFixture(t)
	org := x.scope.OrgID().String()
	b, _ := x.assignedDriver(t)
	at := x.declaredNow(t)
	running := x.startOp(t, x.syncFixtureState, at, absentReading)
	x.deliver(t, x.syncFixtureState, running, protocol.Accepted, "")
	held := x.startOp(t, b, at.Add(30*time.Minute), absentReading)
	item := x.deliver(t, b, held, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
	x.deliver(t, x.syncFixtureState, x.finishOp(t, x.syncFixtureState, tripOf(running), at.Add(time.Hour), absentReading), protocol.Accepted, "")
	second := x.finishOp(t, x.syncFixtureState, tripOf(running), at.Add(2*time.Hour), absentReading)
	finishItem := x.deliver(t, x.syncFixtureState, second, protocol.AcceptedForReview, tripapp.TripAlreadyClosed).Snapshot().ReviewItemID
	owner := x.owner()
	var reviewerEmployee string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, org, owner.member).Scan(&reviewerEmployee); err != nil {
		t.Fatal(err)
	}
	expect := func(name, code string, run func(tx pgx.Tx) error) {
		t.Helper()
		err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
				return err
			}
			if err := run(tx); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`)
			return err
		})
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != code {
			t.Fatal(name, "expected", code, "got", err)
		}
	}
	resolution := func(tx pgx.Tx, id, review string, outcome string, entity any) error {
		op, digest := held.ID(), held.PayloadDigest()
		if review == finishItem {
			op, digest = second.ID(), second.PayloadDigest()
		}
		var revision *int64
		if entity != nil {
			v := entity.(int64)
			revision = &v
		}
		var resolved time.Time
		if err := tx.QueryRow(t.Context(), `SELECT date_trunc('milliseconds',clock_timestamp())`).Scan(&resolved); err != nil {
			return err
		}
		reason := "Synthetic"
		if outcome == protocol.ReviewAccepted {
			reason = ""
		}
		value, err := protocol.NewReviewResolution(protocol.ReviewResolutionSnapshot{OperationID: op, PayloadDigest: digest, ReviewItemID: review, ResolutionID: id, ReviewRevision: 2,
			Outcome: outcome, Reason: reason, ResolvedAt: orgtime.FormatInstant(resolved.UTC()), EntityRevision: revision})
		if err != nil {
			return err
		}
		_, err = tx.Exec(t.Context(), `INSERT INTO org_sync_review_resolutions(org_id,id,review_item_id,expected_revision,outcome,reason,reviewer_account_id,reviewer_membership_id,reviewer_employee_id,reviewer_session_id,request_hash,resolved_at,entity_revision,canonical_resolution)
 VALUES($1,$2,$3,1,$4,$5,$6,$7,$8,$9,repeat('a',64),$10,$11,$12)`, org, id, review, outcome, reason, owner.actor.AccountID, owner.member, reviewerEmployee, owner.actor.SessionID, resolved, revision, value.CanonicalBytes())
		return err
	}
	expect("accepted held finish", "23514", func(tx pgx.Tx) error {
		return resolution(tx, onboardingID(t), finishItem, protocol.ReviewAccepted, int64(1))
	})
	expect("accepted resolution without its trip, reading and readiness", "23514", func(tx pgx.Tx) error {
		id := onboardingID(t)
		// Its audit event exists: only the missing trip, reading and readiness refuse it.
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash) VALUES($1,$2,$3,'offl.review.resolved','offl.review',$4,repeat('a',64))`,
			org, id, owner.actor.AccountID, item); err != nil {
			return err
		}
		return resolution(tx, id, item, protocol.ReviewAccepted, int64(1))
	})
	expect("effect of a held start without a resolution", "23514", func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_sync_effects(org_id,operation_id,entity_revision) VALUES($1,$2,1)`, org, held.ID())
		return err
	})
	insertTrip := func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_trips(org_id,id,operation_id,payload_digest,asset_id,driver_employee_id,author_membership_id,author_employee_id,enrollment_id,occurred_at,occurred_submicro,
 entry_zone,device_offset_seconds,missing_reading_reason,unit_id,reporting_day,reporting_zone,reporting_source,state)
 SELECT h.org_id,h.trip_id,h.operation_id,o.payload_digest,h.asset_id,h.driver_employee_id,o.membership_id,i.author_employee_id,o.enrollment_id,h.occurred_at,h.occurred_submicro,h.entry_zone,
 h.device_offset_seconds,h.missing_reading_reason,$3,(h.occurred_at AT TIME ZONE u.time_zone)::date,u.time_zone,'unit','IN_PROGRESS'
 FROM org_trip_held_starts h JOIN org_sync_review_items i ON(i.org_id,i.id)=(h.org_id,h.review_item_id) JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(h.org_id,h.operation_id)
 JOIN org_units u ON(u.org_id,u.id)=(h.org_id,$3) WHERE h.org_id=$1 AND h.review_item_id=$2`, org, item, x.unit)
		return err
	}
	expect("held trip identity without its accepted resolution", "23505", insertTrip)
	// The held start lies inside the running trip's finished interval: even with
	// an accepted resolution the trip source refuses the overlap.
	expect("accepted trip overlapping a finished trip", "23514", func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		if err := resolution(tx, onboardingID(t), item, protocol.ReviewAccepted, int64(1)); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=origin`); err != nil {
			return err
		}
		return insertTrip(tx)
	})
	if c := x.conflictCount(t, held); c != (conflictCounts{}) {
		t.Fatalf("a refused database write left state: %+v", c)
	}
	if _, _, err := x.resolve(owner, item, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
		t.Fatal("the overlapping held start was accepted through the store", err)
	}
}

// B-BR-EMP-409 with EMP-TRIP-011: a cancelled trip holds the asset until its
// cancellation, so a held start after it is acceptable and one before it is
// not; a paused trip still holds the asset.
func TestTripConflictCancelledTripEndsAtItsCancellation(t *testing.T) {
	x := conflictFixture(t)
	a := x.syncFixtureState
	b, _ := x.assignedDriver(t)
	at := x.declaredNow(t)
	running := x.startOp(t, a, at, absentReading)
	x.deliver(t, a, running, protocol.Accepted, "")
	early := x.startOp(t, b, at.Add(30*time.Minute), absentReading)
	earlyItem := x.deliver(t, b, early, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
	pause := x.operation(t, a, tripapp.PauseOperationType, tripOf(running), at.Add(40*time.Minute), map[string]any{"reason": nil}, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": tripOf(running), "expected_revision": 1}
	})
	x.deliver(t, a, pause, protocol.Accepted, "")
	late := x.startOp(t, b, at.Add(2*time.Hour), absentReading)
	lateItem := x.deliver(t, b, late, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
	if _, _, err := x.resolve(x.owner(), lateItem, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
		t.Fatal("a held start was accepted while the asset's trip is paused", err)
	}
	cancel := x.operation(t, a, tripapp.CancelOperationType, tripOf(running), at.Add(time.Hour), map[string]any{"reason": "Viagem cancelada"}, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.EntityType, "id": tripOf(running), "expected_revision": 2}
	})
	x.deliver(t, a, cancel, protocol.Accepted, "")
	view, err := x.readConflict(x.owner(), earlyItem)
	if err != nil || len(view.Trips) != 1 || view.Trips[0].State != tripdomain.Cancelled || view.Trips[0].FinishedAt == nil || !view.Trips[0].FinishedAt.Equal(at.Add(time.Hour)) {
		t.Fatal("the cancelled trip is not shown ending at its cancellation", err, view.Trips)
	}
	if _, _, err := x.resolve(x.owner(), earlyItem, 1, tripapp.ConflictAccept, "", onboardingID(t)); !errors.Is(err, tripapp.ErrConflictUnacceptable) {
		t.Fatal("a held start inside the cancelled trip was accepted", err)
	}
	if r, _, err := x.resolve(x.owner(), lateItem, 1, tripapp.ConflictAccept, "", onboardingID(t)); err != nil || r.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("a held start after the cancellation was not accepted", err)
	}
	if x.inProgress(t) != 1 || x.stateOf(t, tripOf(late)) != "IN_PROGRESS" || x.stateOf(t, tripOf(running)) != "CANCELLED" {
		t.Fatal("acceptance after the cancellation")
	}
}
