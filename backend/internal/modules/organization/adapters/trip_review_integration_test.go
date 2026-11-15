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
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// tripReviewGrants are the runtime privileges of the EMP-TRIP-004 owners: the
// append-only history, the trip and finish reads, the structure lookup and
// the rows the database guards re-prove (README proposal in the evidence).
func tripReviewGrants(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_review_entries TO "+role+
		";GRANT SELECT ON org_trips,org_trip_finishes,org_team_links,org_memberships,org_employee_identities,org_audit_events,org_sync_operations,org_sync_acknowledgements,org_sync_effects,org_device_enrollments TO "+role+
		";GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
}

// tripReviewState is the EMP-TRIP-002 trip fixture with the real
// trip.submit owner on the same intake and the real company review store.
type tripReviewState struct {
	tripState
	submit  syncapp.Handler
	intake  *syncpg.Store
	reviews *trippg.ReviewStore
}

type tripReviewer struct {
	actor                     tripapp.ReviewActor
	member, employee, account string
}

func tripReviewIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := tripIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: tripapp.SubmitOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	return config
}

func tripReviewStoreOf(t *testing.T, fx legalFixtureState) *trippg.ReviewStore {
	t.Helper()
	catalog, err := audit.NewCatalog(trippg.ReviewAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &trippg.ReviewStore{Pool: fx.f.Runtime, Authority: &orgpg.TripReviewAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags,
		Structure: &orgpg.ResourceStructure{Kind: orgpg.TripStructure, Pool: fx.f.Runtime}}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}

func tripReviewFixture(t *testing.T) tripReviewState {
	t.Helper()
	x := tripReviewState{tripState: tripFixture(t)}
	tripReviewGrants(t, x.f)
	submit, err := trippg.NewSubmitHandler(x.authority, x.meters, x.store, x.store)
	if err != nil {
		t.Fatal(err)
	}
	x.submit, x.intake, x.reviews = submit, syncpg.NewStore(tripReviewIntakeConfig(x.syncFixtureState)), tripReviewStoreOf(t, x.legalFixtureState)
	return x
}

func (x tripReviewState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	handler := x.start
	switch op.Snapshot().Type {
	case tripapp.FinishOperationType:
		handler = x.finish
	case tripapp.SubmitOperationType:
		handler = x.submit
	}
	return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), handler)
}

// absentReading declares the stated-absence form, so sequential trips need no
// meter series (B-BR-EMP-314).
func absentReading(_, payload map[string]any) {
	payload["reading"], payload["missing_reading_reason"] = nil, "Painel apagado"
}

// completed is member d's started and finished trip at the given instant.
func (x tripReviewState) completed(t *testing.T, d syncFixtureState, at time.Time) (string, protocol.Operation) {
	t.Helper()
	start := x.startOp(t, d, at, absentReading)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	finish := x.finishOp(t, d, trip, at.Add(time.Hour), absentReading)
	ack, err = x.process(t, d, finish)
	x.accepted(t, finish, ack, err, 2, 0)
	return trip, finish
}

// submitOp is member d's trip.submit of trip at the named review revision.
func (x tripReviewState) submitOp(t *testing.T, d syncFixtureState, trip string, revision any, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return x.operation(t, d, tripapp.SubmitOperationType, trip, declared, map[string]any{}, func(raw, payload map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.ReviewEntityType, "id": trip, "expected_revision": revision}
		if edit != nil {
			edit(raw, payload)
		}
	})
}

func (x tripReviewState) submitted(t *testing.T, d syncFixtureState, trip string, at time.Time) protocol.Operation {
	t.Helper()
	op := x.submitOp(t, d, trip, 1, at, nil)
	ack, err := x.process(t, d, op)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || ack.Code() != "" {
		t.Fatal("the author's submission of a completed trip was not accepted", err, ack.Result(), ack.Code())
	}
	return op
}

type tripReviewCounts struct{ entries, effects, claims, receipts, audits int }

// countOp is everything one submission operation left.
func (x tripReviewState) countOp(t *testing.T, op protocol.Operation) tripReviewCounts {
	t.Helper()
	var c tripReviewCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_review_entries WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.entries, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

// history is a trip's review history and its decision audit events.
func (x tripReviewState) history(t *testing.T, scope kernel.OrgScope, trip string) (string, int) {
	t.Helper()
	var entries string
	var audits int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT COALESCE((SELECT string_agg(kind||':'||state||':'||revision,',' ORDER BY sequence) FROM org_trip_review_entries WHERE org_id=$1 AND trip_id=$2),''),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_type='trip' AND target_id=$2)`, scope.OrgID().String(), trip).Scan(&entries, &audits); err != nil {
		t.Fatal(err)
	}
	return entries, audits
}

func (x tripReviewState) tripRow(t *testing.T, trip string) string {
	t.Helper()
	var row string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT to_jsonb(r) FROM org_trips r WHERE r.org_id=$1 AND r.id=$2),(SELECT to_jsonb(f) FROM org_trip_finishes f WHERE f.org_id=$1 AND f.trip_id=$2))::text`, x.scope.OrgID().String(), trip).Scan(&row); err != nil {
		t.Fatal(err)
	}
	return row
}

// reviewerFor is a new member with a real company session holding exactly one
// current grant; scopeType ORG targets the organization.
func (x tripReviewState) reviewerFor(t *testing.T, role, scopeType, target string) tripReviewer {
	t.Helper()
	p := deviceNewPerson(t, x.deviceFixtureState)
	org := x.scope.OrgID().String()
	switch scopeType {
	case "ORG":
		target = org
	case "SELF":
		target = p.member
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, p.member); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Synthetic trip reviewer')`,
		org, onboardingID(t), p.member, role, scopeType, target, time.Now().UTC(), x.member); err != nil {
		t.Fatal(role, scopeType, err)
	}
	return tripReviewer{actor: tripapp.ReviewActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}, member: p.member, employee: p.employee, account: p.auth.Account.ID}
}

func (x tripReviewState) owner() tripReviewer {
	return tripReviewer{actor: tripapp.ReviewActor{AccountID: x.actor.AccountID, SessionID: x.actor.SessionID}, member: x.member, account: x.actor.AccountID}
}

// decide never calls t.Fatal, so it is safe inside a goroutine.
func (x tripReviewState) decide(scope kernel.OrgScope, r tripReviewer, trip string, revision int64, kind review.Kind, reason, operation string) (tripapp.DecisionResult, bool, error) {
	command, err := tripapp.NewDecisionCommand(tripapp.DecisionInput{OperationID: operation, TripID: trip, ExpectedRevision: revision, Decision: kind, Reason: reason})
	if err != nil {
		return tripapp.DecisionResult{}, false, err
	}
	return x.reviews.DecideTripReview(context.Background(), scope, r.actor, r.member, command)
}

func (x tripReviewState) read(r tripReviewer, trip string) (tripapp.ReviewView, error) {
	return x.reviews.ReadTripReview(context.Background(), x.scope, r.actor, r.member, trip)
}

// sequentialTrips is a new DRIVER/SELF member with its own device assigned
// to the vehicle, and the database instant after the assignment.
func (x tripReviewState) assignedDriver(t *testing.T) (syncFixtureState, time.Time) {
	t.Helper()
	d := x.driver(t)
	x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	return d, x.declaredNow(t)
}

// BUC-EMP-172 / B-BR-EMP-342/343 on real PostGIS through the real owner: only
// the author's COMPLETED trip at the draft revision is submitted, once, with
// its receipt; everything else stores nothing.
func TestTripSubmitAcceptsOnlyTheAuthorsCompletedDraftAndReplays(t *testing.T) {
	x := tripReviewFixture(t)
	admin := x.syncFixtureState
	d, at := x.assignedDriver(t)
	start := x.startOp(t, d, at, absentReading)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	nothing := func(op protocol.Operation) {
		t.Helper()
		if c := x.countOp(t, op); c != (tripReviewCounts{}) {
			t.Fatalf("a refused submission left state: %+v", c)
		}
		if entries, _ := x.history(t, x.scope, trip); entries != "" {
			t.Fatal("a refused submission wrote review history", entries)
		}
	}
	pending := func(op protocol.Operation) {
		t.Helper()
		ack, err := x.process(t, d, op)
		if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
			t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
		}
		nothing(op)
	}
	// An IN_PROGRESS trip is not submitted: the finish is its dependency.
	pending(x.submitOp(t, d, trip, 1, at.Add(30*time.Minute), nil))
	finish := x.finishOp(t, d, trip, at.Add(time.Hour), absentReading)
	waiting := x.submitOp(t, d, trip, 1, at.Add(time.Hour+time.Minute), func(raw, _ map[string]any) { raw["depends_on"] = []string{finish.ID()} })
	pending(waiting)
	// A trip not stored here (undelivered or another organization's).
	pending(x.submitOp(t, d, onboardingID(t), 1, at, nil))
	ack, err = x.process(t, d, finish)
	x.accepted(t, finish, ack, err, 2, 0)
	// Another member, even an ORG ADMIN holding trip.trip.start, is refused.
	other := x.submitOp(t, admin, trip, 1, at.Add(2*time.Hour), nil)
	if ack, err := x.process(t, admin, other); err == nil || !errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("another member's submission was not refused", err)
	} else if code, _ := orgapi.Registered(err); code != orgapi.Forbidden {
		t.Fatal("another member's submission code", code)
	}
	nothing(other)
	// Envelope and payload outside the shape are final MALFORMED receipts.
	for name, edit := range map[string]func(raw, payload map[string]any){
		"payload-key":      func(_, payload map[string]any) { payload["trip_id"] = trip },
		"no-revision":      func(raw, _ map[string]any) { raw["entity"].(map[string]any)["expected_revision"] = nil },
		"trip-entity":      func(raw, _ map[string]any) { raw["entity"].(map[string]any)["type"] = tripapp.EntityType },
		"two-dependencies": func(raw, _ map[string]any) { raw["depends_on"] = []string{start.ID(), finish.ID()} },
		"revision-above-cap": func(raw, _ map[string]any) {
			raw["entity"].(map[string]any)["expected_revision"] = tripapp.MaxReviewRevision + 1
		},
	} {
		op := x.submitOp(t, admin, trip, 1, at.Add(2*time.Hour), edit)
		ack, err := x.process(t, admin, op)
		if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
			t.Fatal("malformed submission not refused", name, err, ack.Result(), ack.Code())
		}
		if c := x.countOp(t, op); c != (tripReviewCounts{claims: 1, receipts: 1, audits: 1}) {
			t.Fatalf("malformed submission %s stored a fact: %+v", name, c)
		}
	}
	// A revision the draft does not have stores nothing and stays RETRY.
	stale := x.submitOp(t, d, trip, 2, at.Add(2*time.Hour), nil)
	if _, err := x.process(t, d, stale); !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("a stale submission was not kept RETRY/UNAVAILABLE", err)
	}
	nothing(stale)
	before := x.tripRow(t, trip)
	ack, err = x.process(t, d, waiting)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || ack.Code() != "" || len(ack.Warnings()) != 0 {
		t.Fatal("the author's submission was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.countOp(t, waiting); c != (tripReviewCounts{entries: 1, effects: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("accepted submission not stored exactly once: %+v", c)
	}
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT e.sequence=1 AND e.kind='SUBMIT' AND e.revision=1 AND e.state='SUBMITTED' AND e.reason='' AND e.actor_membership_id=$3 AND e.actor_employee_id=$4
 AND e.recorded_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=e.org_id AND a.operation_id=e.operation_id)
 FROM org_trip_review_entries e WHERE e.org_id=$1 AND e.operation_id=$2`, x.scope.OrgID().String(), waiting.ID(), d.source.MembershipID, d.person.employee).Scan(&exact); err != nil || !exact {
		t.Fatal("stored submission is not the author's exact SUBMIT entry", err)
	}
	if x.tripRow(t, trip) != before {
		t.Fatal("submission changed the operational trip facts")
	}
	x.replays(t, d, waiting, ack)
	reused := x.submitOp(t, d, trip, 1, at.Add(3*time.Hour), func(raw, _ map[string]any) { raw["operation_id"] = waiting.ID() })
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same submission was not refused", err, refusal.Code())
	}
	// A second submission of the submitted case stores nothing (EDR-220).
	again := x.submitOp(t, d, trip, 1, at.Add(3*time.Hour), nil)
	if _, err := x.process(t, d, again); !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("a second submission was not kept RETRY/UNAVAILABLE", err)
	}
	if c := x.countOp(t, again); c != (tripReviewCounts{}) {
		t.Fatalf("a second submission left state: %+v", c)
	}
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
		t.Fatal("submission history", entries, audits)
	}
	x.replays(t, d, waiting, ack)
	view, err := x.read(x.owner(), trip)
	if err != nil || view.Review != tripdomain.ReviewSubmitted || view.Operational != tripdomain.Completed || view.Revision != 1 || len(view.History) != 1 || view.FinishedAt == nil || !view.StartedAt.Equal(at) {
		t.Fatal("the submitted trip's decision context", err, view.Review, view.Operational)
	}
}

func (x tripReviewState) replays(t *testing.T, d syncFixtureState, op protocol.Operation, ack protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
}

// BUC-EMP-173 / B-BR-EMP-344/345/347: one audited decision per revision, the
// queue, replay, operation reuse, stale revisions, a second decision, a draft
// and another organization, with the operational trip untouched.
func TestTripReviewDecidesOneRevisionWithAuditReplayAndRefusals(t *testing.T) {
	x := tripReviewFixture(t)
	d, at := x.assignedDriver(t)
	var trips []string
	for i := range 4 {
		trip, _ := x.completed(t, d, at.Add(time.Duration(2*i)*time.Hour))
		if i < 3 {
			x.submitted(t, d, trip, at.Add(time.Duration(2*i)*time.Hour+90*time.Minute))
		}
		trips = append(trips, trip)
	}
	approved, returned, rejected, draft := trips[0], trips[1], trips[2], trips[3]
	owner := x.owner()
	ctx := context.Background()
	page, err := x.reviews.ListTripReviews(ctx, x.scope, owner.actor, owner.member, tripapp.QueueFilter{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.Items[0].TripID != approved || page.Items[1].TripID != returned || page.NextID != returned || page.Items[0].Review != tripdomain.ReviewSubmitted {
		t.Fatal("first queue page", err, page.NextID)
	}
	if page, err = x.reviews.ListTripReviews(ctx, x.scope, owner.actor, owner.member, tripapp.QueueFilter{Limit: 2, AfterID: page.NextID}); err != nil || len(page.Items) != 1 || page.Items[0].TripID != rejected || page.NextID != "" {
		t.Fatal("second queue page", err)
	}
	rows := map[string]string{}
	for _, trip := range trips {
		rows[trip] = x.tripRow(t, trip)
	}
	unchanged := func(trip, want string, audits int) {
		t.Helper()
		if entries, n := x.history(t, x.scope, trip); entries != want || n != audits {
			t.Fatal("review history changed unexpectedly", trip, entries, n)
		}
	}
	// A stale revision changes nothing (BR-REV-007).
	if _, _, err := x.decide(x.scope, owner, approved, 2, review.Approve, "", onboardingID(t)); !errors.Is(err, review.ErrConflict) {
		t.Fatal("stale revision not CONFLICT", err)
	}
	unchanged(approved, "SUBMIT:SUBMITTED:1", 0)
	// Approve the current revision: OPEN and APPROVE with one audit event.
	op := onboardingID(t)
	result, replayed, err := x.decide(x.scope, owner, approved, 1, review.Approve, "", op)
	if err != nil || replayed || result.State != review.Approved || result.Revision != 1 || result.Decision != review.Approve || result.TripID != approved {
		t.Fatal("approval of the current revision", err, result.State)
	}
	unchanged(approved, "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1", 1)
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT bool_and(e.actor_membership_id=$3 AND e.operation_id=$4) AND(SELECT a.action='trip.trip.approved' AND a.actor_id=$5 AND a.target_type='trip' AND a.target_id=$2 AND a.reason_code=''
 FROM org_audit_events a WHERE a.org_id=$1 AND a.operation_id=$4) FROM org_trip_review_entries e WHERE e.org_id=$1 AND e.trip_id=$2 AND e.sequence>1`,
		x.scope.OrgID().String(), approved, owner.member, op, owner.account).Scan(&exact); err != nil || !exact {
		t.Fatal("decision entries or audit event not exact", err)
	}
	// The same operation replays the stored answer with no second audit.
	again, replayed, err := x.decide(x.scope, owner, approved, 1, review.Approve, "", op)
	if err != nil || !replayed || again != result {
		t.Fatal("replay did not return the first result", err, replayed)
	}
	if _, _, err := x.decide(x.scope, owner, approved, 1, review.Approve, "Synthetic other digest", op); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("operation reuse with another digest", err)
	}
	// A second decision on the decided revision changes nothing.
	if _, _, err := x.decide(x.scope, owner, approved, 1, review.Return, "Synthetic late return", onboardingID(t)); !errors.Is(err, review.ErrInvalidTransition) {
		t.Fatal("a second decision was not refused", err)
	}
	unchanged(approved, "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1", 1)
	// Return with the reason; the author's resubmission stays RETRY (EDR-220).
	if result, _, err := x.decide(x.scope, owner, returned, 1, review.Return, "Informe o motivo da parada", onboardingID(t)); err != nil || result.State != review.Returned || result.Reason != "Informe o motivo da parada" {
		t.Fatal("return with reason", err, result.State)
	}
	resubmit := x.submitOp(t, d, returned, 1, at.Add(10*time.Hour), nil)
	if _, err := x.process(t, d, resubmit); !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("a submission of a returned case was not kept RETRY/UNAVAILABLE", err)
	}
	unchanged(returned, "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,RETURN:RETURNED:1", 1)
	if result, _, err := x.decide(x.scope, owner, rejected, 1, review.Reject, "Viagem duplicada", onboardingID(t)); err != nil || result.State != review.Rejected {
		t.Fatal("rejection with reason", err)
	}
	// A draft is never decided: operational completion is not submission.
	if _, _, err := x.decide(x.scope, owner, draft, 1, review.Approve, "", onboardingID(t)); !errors.Is(err, review.ErrInvalidTransition) {
		t.Fatal("a draft trip was decided", err)
	}
	unchanged(draft, "", 0)
	if page, err := x.reviews.ListTripReviews(ctx, x.scope, owner.actor, owner.member, tripapp.QueueFilter{Limit: 10}); err != nil || len(page.Items) != 0 {
		t.Fatal("decided trips stayed in the queue", err, len(page.Items))
	}
	for trip, row := range rows {
		if x.tripRow(t, trip) != row {
			t.Fatal("a review decision changed the operational trip", trip)
		}
	}
	view, err := x.read(owner, returned)
	if err != nil || view.Review != tripdomain.ReviewReturned || view.Operational != tripdomain.Completed || len(view.History) != 3 || view.History[2].Reason != "Informe o motivo da parada" {
		t.Fatal("returned decision context", err)
	}
	// Another organization's reviewer naming this trip finds nothing.
	other := legalFixtureFor(t, x.f, x.onboarding, x.legalFixtureState.actor, x.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	stranger := tripReviewer{actor: tripapp.ReviewActor{AccountID: other.actor.AccountID, SessionID: other.actor.SessionID}, member: other.member}
	if _, _, err := x.decide(other.scope, stranger, draft, 1, review.Approve, "", onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization's trip was not NOT_FOUND", err)
	}
	if _, err := x.reviews.ReadTripReview(ctx, other.scope, stranger.actor, stranger.member, approved); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization read a trip", err)
	}
	if _, _, err := x.decide(x.scope, owner, onboardingID(t), 1, review.Approve, "", onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("an unknown trip was not NOT_FOUND", err)
	}
	unchanged(draft, "", 0)
}

// B-BR-EMP-345 / BR-REV-003/004 and EMP-AUTH-006: the author cannot decide
// its own trip whatever its roles; roles without trip.trip.review and grants
// whose structure does not cover the trip are FORBIDDEN; covering UNIT and
// ASSET grants decide.
func TestTripReviewRefusesSelfApprovalUncoveredScopesAndAuditors(t *testing.T) {
	x := tripReviewFixture(t)
	org := x.scope.OrgID().String()
	// The ADMIN (ORG, holding trip.trip.review) drives and submits its own trip.
	at := x.declaredNow(t)
	own, _ := x.completed(t, x.syncFixtureState, at)
	x.submitted(t, x.syncFixtureState, own, at.Add(90*time.Minute))
	d, _ := x.assignedDriver(t)
	trip, _ := x.completed(t, d, at.Add(2*time.Hour))
	x.submitted(t, d, trip, at.Add(3*time.Hour+30*time.Minute))
	admin := tripReviewer{actor: tripapp.ReviewActor{AccountID: x.person.web.AccountID, SessionID: x.person.web.SessionID}, member: x.person.member, account: x.person.auth.Account.ID}
	if _, _, err := x.decide(x.scope, admin, own, 1, review.Approve, "", onboardingID(t)); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("self-approval was not refused", err)
	}
	if _, _, err := x.decide(x.scope, admin, own, 1, review.Reject, "Synthetic self rejection", onboardingID(t)); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("self-rejection was not refused", err)
	}
	if entries, audits := x.history(t, x.scope, own); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
		t.Fatal("a refused self decision changed the history", entries, audits)
	}
	if view, err := x.read(admin, own); err != nil || view.Review != tripdomain.ReviewSubmitted {
		t.Fatal("the author's own read of the decision context", err)
	}
	sul, centre := onboardingID(t), onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic Sul','SUL','UTC','ACTIVE')`, org, sul); err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic centre','SYN-1','ACTIVE')`, org, centre)
		return err
	}); err != nil {
		t.Fatal("cost-centre fixture", err)
	}
	// The role guard proves a COST_CENTRE target's state (EDR-160).
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_cost_centres TO "+pgx.Identifier{x.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	denied := func(name string, r tripReviewer) {
		t.Helper()
		if _, _, err := x.decide(x.scope, r, trip, 1, review.Approve, "", onboardingID(t)); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatal("decision not FORBIDDEN", name, err)
		}
		if _, err := x.read(r, trip); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatal("decision context not FORBIDDEN", name, err)
		}
		if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
			t.Fatal("a refused decision changed the history", name, entries, audits)
		}
	}
	denied("auditor", x.reviewerFor(t, "AUDITOR", "ORG", ""))
	denied("finance", x.reviewerFor(t, "FINANCE", "ORG", ""))
	denied("driver", x.reviewerFor(t, "DRIVER", "SELF", ""))
	denied("supervisor of another unit", x.reviewerFor(t, "SUPERVISOR", "UNIT", sul))
	denied("approver of an uncovering cost centre", x.reviewerFor(t, "APPROVER", "COST_CENTRE", centre))
	denied("fleet manager of another asset", x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.installed.AssetID))
	// The queue is organization-level: a structural grant does not list it.
	norte := x.reviewerFor(t, "SUPERVISOR", "UNIT", x.unit)
	if _, err := x.reviews.ListTripReviews(context.Background(), x.scope, norte.actor, norte.member, tripapp.QueueFilter{Limit: 5}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("a UNIT grant listed the organization queue", err)
	}
	// The trip's stored unit and its asset are covered (BR-AUTH-005).
	if view, err := x.read(norte, trip); err != nil || view.TripID != trip {
		t.Fatal("covering UNIT supervisor read", err)
	}
	vehicle := x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.vehicle.AssetID)
	if view, err := x.read(vehicle, trip); err != nil || view.TripID != trip {
		t.Fatal("covering ASSET fleet manager read", err)
	}
	if result, _, err := x.decide(x.scope, norte, trip, 1, review.Return, "Informe o motivo da parada", onboardingID(t)); err != nil || result.State != review.Returned {
		t.Fatal("covering UNIT supervisor decision", err)
	}
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,RETURN:RETURNED:1" || audits != 1 {
		t.Fatal("covered decision history", entries, audits)
	}
}

// B-BR-EMP-345 / BR-REV-007: two reviewers deciding one revision at once
// leave exactly one decision and one audit event, in both arrival orders.
func TestTripReviewConcurrentDecisionsLeaveOneDecision(t *testing.T) {
	for _, order := range []string{"approve-first", "reject-first"} {
		t.Run(order, func(t *testing.T) {
			x := tripReviewFixture(t)
			d, at := x.assignedDriver(t)
			trip, _ := x.completed(t, d, at)
			x.submitted(t, d, trip, at.Add(90*time.Minute))
			x.reviews.Pool = x.f.RuntimePool(t, 4)
			a, b := x.owner(), x.reviewerFor(t, "APPROVER", "ORG", "")
			type attempt struct {
				r      tripReviewer
				kind   review.Kind
				reason string
			}
			first, second := attempt{a, review.Approve, ""}, attempt{b, review.Reject, "Viagem duplicada"}
			if order == "reject-first" {
				first, second = second, first
			}
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
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
			var pid int
			select {
			case pid = <-holder:
			case err := <-done:
				t.Fatal("holder failed", err)
			}
			results := make(chan error, 2)
			var wg sync.WaitGroup
			run := func(v attempt) {
				wg.Go(func() {
					_, _, err := x.decide(x.scope, v.r, trip, 1, v.kind, v.reason, onboardingID(t))
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
				case errors.Is(err, review.ErrInvalidTransition), errors.Is(err, review.ErrConflict):
					lost++
				default:
					t.Fatal("unexpected race outcome", err)
				}
			}
			entries, audits := x.history(t, x.scope, trip)
			want := "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1"
			if first.kind == review.Reject {
				want = "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,REJECT:REJECTED:1"
			}
			if ok != 1 || lost != 1 || entries != want || audits != 1 {
				t.Fatal("two concurrent decisions did not leave exactly the first", ok, lost, entries, audits)
			}
		})
	}
}

// EDR-221: forced RLS, append-only history and the database guards refuse an
// impossible, self-approved, reasonless or unaudited entry even when the
// application is bypassed.
func TestTripReviewSchemaIsForcedRLSAppendOnlyAndGuarded(t *testing.T) {
	x := tripReviewFixture(t)
	d, at := x.assignedDriver(t)
	trip, _ := x.completed(t, d, at)
	org := x.scope.OrgID().String()
	inProgress := x.startOp(t, d, at.Add(2*time.Hour), absentReading)
	ack, err := x.process(t, d, inProgress)
	x.accepted(t, inProgress, ack, err, 1, 0)
	reviewer := x.reviewerFor(t, "APPROVER", "ORG", "")
	insert := func(trip string, sequence int64, kind, state, reason, member, employee, op string) error {
		return unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,$3,$4,1,$5,$6,$7,$8,$9)`,
			org, trip, sequence, kind, state, reason, member, employee, op)
	}
	refused := func(name string, err error, code string) {
		t.Helper()
		var pe interface{ SQLState() string }
		if !errors.As(err, &pe) || pe.SQLState() != code {
			t.Fatal("database guard did not refuse", name, err)
		}
	}
	// A submission without its intake operation, of an in-progress trip, or
	// by someone other than the author.
	refused("unsourced submit", insert(trip, 1, "SUBMIT", "SUBMITTED", "", d.source.MembershipID, d.person.employee, onboardingID(t)), "23514")
	refused("in-progress submit", insert(tripOf(inProgress), 1, "SUBMIT", "SUBMITTED", "", d.source.MembershipID, d.person.employee, onboardingID(t)), "23514")
	refused("draft opened", insert(trip, 1, "OPEN", "IN_REVIEW", "", reviewer.member, reviewer.employee, onboardingID(t)), "23514")
	x.submitted(t, d, trip, at.Add(90*time.Minute))
	op := onboardingID(t)
	refused("self review", insert(trip, 2, "OPEN", "IN_REVIEW", "", d.source.MembershipID, d.person.employee, op), "23514")
	refused("decision skipping the opening", insert(trip, 2, "APPROVE", "APPROVED", "", reviewer.member, reviewer.employee, op), "23514")
	refused("state not of the kind", insert(trip, 2, "OPEN", "APPROVED", "", reviewer.member, reviewer.employee, op), "23514")
	// A RETURN without a reason violates the reason policy.
	err = database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`, org, trip, reviewer.member, reviewer.employee, op); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,3,'RETURN',1,'RETURNED','  ',$3,$4,$5)`, org, trip, reviewer.member, reviewer.employee, op)
		return err
	})
	refused("reasonless return", err, "23514")
	// A well-formed opening and decision without its audit event fails at commit.
	err = database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`, org, trip, reviewer.member, reviewer.employee, op); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,3,'APPROVE',1,'APPROVED','',$3,$4,$5)`, org, trip, reviewer.member, reviewer.employee, op)
		return err
	})
	refused("unaudited decision", err, "23514")
	if entries, _ := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" {
		t.Fatal("a refused direct write left history", entries)
	}
	// Even with table privileges the history is append-only (55000).
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_review_entries TO "+pgx.Identifier{x.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_trip_review_entries SET reason='x' WHERE org_id=$1`, `DELETE FROM org_trip_review_entries WHERE org_id=$1`} {
		refused(statement, unitSchemaExec(t, x.legalFixtureState, statement, org), "55000")
	}
	refused("truncate", unitSchemaExec(t, x.legalFixtureState, `TRUNCATE org_trip_review_entries`), "55000")
	// Forced RLS: nothing is visible or insertable without the organization.
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_review_entries`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("review history visible without the organization setting", visible, err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_review_entries`).Scan(&visible)
	}); err != nil || visible != 1 {
		t.Fatal("review history in scope", visible, err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_trip_review_entries'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("review history is not forced RLS", err)
	}
	if _, err := x.f.Runtime.Exec(t.Context(), `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`, org, trip, reviewer.member, reviewer.employee, onboardingID(t)); err == nil {
		t.Fatal("an entry was inserted without the organization setting")
	}
}
