//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// tripHandoverState is the EMP-TRIP-011 lifecycle fixture (start, finish,
// logbook, pause/resume/cancel and submit owners on one intake) with the real
// trip review store and the real EMP-TRIP-012 handover store.
type tripHandoverState struct {
	tripLifecycleState
	reviews   *trippg.ReviewStore
	handovers *trippg.HandoverStore
}

// handoverGrants are the company runtime privileges of the handover owner:
// its insert-only facts and the rows its authority and guard read.
func handoverGrants(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_handovers TO "+role+
		";GRANT SELECT ON org_trips,org_memberships,org_employee_identities,org_audit_events,org_asset_assignments TO "+role); err != nil {
		t.Fatal(err)
	}
}

func tripHandoverStoreOf(t *testing.T, fx legalFixtureState, assets *fleetpg.AssetStore) *trippg.HandoverStore {
	t.Helper()
	catalog, err := audit.NewCatalog(trippg.HandoverAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	reviewAuthority := &orgpg.TripReviewAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags,
		Structure: &orgpg.ResourceStructure{Kind: orgpg.TripStructure, Pool: fx.f.Runtime}}
	return &trippg.HandoverStore{Pool: fx.f.Runtime, Authority: &orgpg.TripHandoverAuthority{Review: reviewAuthority, Assignments: assets}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}

func tripHandoverFixture(t *testing.T) tripHandoverState {
	t.Helper()
	x := tripHandoverState{tripLifecycleState: tripLifecycleFixture(t)}
	handoverGrants(t, x.f)
	x.reviews = tripReviewStoreOf(t, x.legalFixtureState)
	x.handovers = tripHandoverStoreOf(t, x.legalFixtureState, x.assets)
	return x
}

// staff is a new member with a real company session holding exactly one
// current grant; scopeType ORG targets the organization.
func (x tripHandoverState) staff(t *testing.T, role, scopeType, target string) tripReviewer {
	t.Helper()
	p := deviceNewPerson(t, x.deviceFixtureState)
	x.grant(t, p.member, role, scopeType, target)
	return tripReviewer{actor: tripapp.ReviewActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}, member: p.member, employee: p.employee, account: p.auth.Account.ID}
}

// grant replaces a member's current grants with exactly one.
func (x tripHandoverState) grant(t *testing.T, member, role, scopeType, target string) {
	t.Helper()
	org := x.scope.OrgID().String()
	switch scopeType {
	case "ORG":
		target = org
	case "SELF":
		target = member
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, member); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'Synthetic handover role')`,
		org, onboardingID(t), member, role, scopeType, target, time.Now().UTC(), x.member); err != nil {
		t.Fatal(role, scopeType, err)
	}
}

// assignedDriver is a new DRIVER/SELF member with its own device assigned to
// the vehicle.
func (x tripHandoverState) assignedDriver(t *testing.T) syncFixtureState {
	t.Helper()
	d := x.driver(t)
	x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	return d
}

// deviceReviewer is a device member's own company session as a reviewer.
func deviceReviewer(d syncFixtureState) tripReviewer {
	p := d.person
	return tripReviewer{actor: tripapp.ReviewActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}, member: p.member, employee: p.employee, account: p.auth.Account.ID}
}

// handOver never calls t.Fatal, so it is safe inside a goroutine.
func (x tripHandoverState) handOver(scope kernel.OrgScope, r tripReviewer, trip string, revision int64, receiver, reason, operation string) (tripapp.HandoverResult, bool, error) {
	command, err := tripapp.NewHandoverCommand(tripapp.HandoverInput{OperationID: operation, TripID: trip, ExpectedRevision: revision, ReceiverMembershipID: receiver, Reason: reason})
	if err != nil {
		return tripapp.HandoverResult{}, false, err
	}
	return x.handovers.HandOver(context.Background(), scope, r.actor, r.member, command)
}

func (x tripHandoverState) mine(t *testing.T, d syncFixtureState) tripapp.OwnTripPage {
	t.Helper()
	r := deviceReviewer(d)
	page, err := x.handovers.ListOwnTrips(context.Background(), x.scope, r.actor, r.member, tripapp.QueueFilter{Limit: 10})
	if err != nil {
		t.Fatal("own open trips", err)
	}
	return page
}

type handoverCounts struct{ handovers, audits int }

// handoverCount is everything handovers of a trip and one operation left.
func (x tripHandoverState) handoverCount(t *testing.T, trip, operation string) handoverCounts {
	t.Helper()
	var c handoverCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_handovers WHERE org_id=$1 AND trip_id=$2),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3)`, x.scope.OrgID().String(), trip, operation).Scan(&c.handovers, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x tripHandoverState) revisionOf(t *testing.T, trip string) int64 {
	t.Helper()
	var revision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT org_trip_revision($1,$2)`, x.scope.OrgID().String(), trip).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

// submitOp is member d's trip.submit of trip at the named review revision.
func (x tripHandoverState) submitOp(t *testing.T, d syncFixtureState, trip string, revision any, declared time.Time) protocol.Operation {
	t.Helper()
	return x.operation(t, d, tripapp.SubmitOperationType, trip, declared, map[string]any{}, func(raw, _ map[string]any) {
		raw["entity"] = map[string]any{"type": tripapp.ReviewEntityType, "id": trip, "expected_revision": revision}
	})
}

func (x tripHandoverState) decide(r tripReviewer, trip string, kind review.Kind, reason string) (tripapp.DecisionResult, bool, error) {
	command, err := tripapp.NewDecisionCommand(tripapp.DecisionInput{OperationID: handoverOpID(), TripID: trip, ExpectedRevision: 1, Decision: kind, Reason: reason})
	if err != nil {
		return tripapp.DecisionResult{}, false, err
	}
	return x.reviews.DecideTripReview(context.Background(), x.scope, r.actor, r.member, command)
}

// handoverOpID is a fresh operation identifier that is safe inside a goroutine.
func handoverOpID() string {
	id, err := jobs.NewUUIDv4()
	if err != nil {
		panic(err)
	}
	return id
}

// BUC-EMP-199/200 and B-BR-EMP-394/395 on real PostGIS: a supervisor hands an
// open trip over at its revision; every earlier fact keeps its actor, the
// giving driver's offline fact declared before the handover still arrives,
// anything it sends after is FORBIDDEN, and the receiver pauses, records,
// resumes, finishes and submits the same trip; replay and reuse follow the
// company command rules and the receiver lists the trip as its own.
func TestTripHandoverMovesFutureResponsibilityAndKeepsHistory(t *testing.T) {
	x := tripHandoverFixture(t)
	org := x.scope.OrgID().String()
	d1, d2 := x.assignedDriver(t), x.assignedDriver(t)
	at := x.declaredNow(t)
	start := x.startOp(t, d1, at, nil)
	ack, err := x.process(t, d1, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 1)
	trip := tripOf(start)
	early := x.eventOp(t, d1, trip, "STOP", nil, at.Add(time.Millisecond), nil)
	earlyAck, err := x.process(t, d1, early)
	x.eventState.accepted(t, early, earlyAck, err)
	if page := x.mine(t, d1); len(page.Items) != 1 || page.Items[0].TripID != trip || page.Items[0].Handover != nil || page.Items[0].Revision != 1 {
		t.Fatal("the starting driver's own open trip", page.Items)
	}
	sup := x.staff(t, "SUPERVISOR", "ORG", "")
	operation := onboardingID(t)
	result, replayed, err := x.handOver(x.scope, sup, trip, 1, d2.person.member, "Troca de turno", operation)
	h := result.Handover
	if err != nil || replayed || result.TripID != trip || result.Revision != 2 || result.Operational != "IN_PROGRESS" || h.Revision != 2 || h.OperationID != operation ||
		h.FromMembershipID != d1.person.member || h.FromEmployeeID != d1.person.employee || h.ToMembershipID != d2.person.member || h.ToEmployeeID != d2.person.employee ||
		h.RecorderMembershipID != sup.member || h.RecorderEmployeeID != sup.employee || h.Reason != "Troca de turno" || h.HandedOverAt.IsZero() {
		t.Fatalf("the handover was not recorded at the next revision: %v %+v", err, result)
	}
	handed := h.HandedOverAt
	if c := x.handoverCount(t, trip, operation); c != (handoverCounts{handovers: 1, audits: 1}) || x.revisionOf(t, trip) != 2 {
		t.Fatalf("one handover must leave one fact and one audit event: %+v", c)
	}
	var auditRow string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT to_jsonb(a)::text FROM org_audit_events a WHERE a.org_id=$1 AND a.operation_id=$2 AND a.action='trip.trip.handed_over' AND a.target_type='trip' AND a.target_id=$3 AND a.actor_id=$4`,
		org, operation, trip, sup.account).Scan(&auditRow); err != nil || strings.Contains(auditRow, "Troca de turno") {
		t.Fatal("the handover audit event must name the recorder and carry no reason text", err)
	}
	again, replayed, err := x.handOver(x.scope, sup, trip, 1, d2.person.member, "Troca de turno", operation)
	if err != nil || !replayed || again != result {
		t.Fatal("an identical handover did not replay the first answer", err, replayed)
	}
	if _, _, err := x.handOver(x.scope, sup, trip, 1, d2.person.member, "Outro motivo", operation); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("a reused handover operation was not OPERATION_ID_REUSED", err)
	}
	if c := x.handoverCount(t, trip, operation); c != (handoverCounts{handovers: 1, audits: 1}) {
		t.Fatalf("a replay or reuse wrote again: %+v", c)
	}
	// The giving driver's offline fact declared before the handover arrives.
	before := handed.Add(-time.Microsecond)
	if !before.After(at.Add(time.Millisecond)) {
		t.Fatal("fixture clock: the handover must follow the first event")
	}
	late := x.eventOp(t, d1, trip, "NOTE", "Parada antes da troca", before, nil)
	lateAck, err := x.process(t, d1, late)
	x.eventState.accepted(t, late, lateAck, err)
	// Anything the giving driver declares from the handover on is FORBIDDEN.
	after := x.eventOp(t, d1, trip, "STOP", nil, handed, nil)
	ack, err = x.process(t, d1, after)
	x.eventState.refused(t, after, ack, err, orgapi.Forbidden)
	pause := x.transitionOp(t, d1, tripapp.PauseOperationType, trip, 2, handed.Add(time.Second), nil, nil)
	ack, err = x.process(t, d1, pause)
	x.refusedCommand(t, pause, ack, err, orgapi.Forbidden)
	early2 := x.eventOp(t, d2, trip, "STOP", nil, before, nil)
	ack, err = x.process(t, d2, early2)
	x.eventState.refused(t, early2, ack, err, orgapi.Forbidden)
	// The receiver continues the same trip.
	stale := x.transitionOp(t, d2, tripapp.PauseOperationType, trip, 1, handed.Add(time.Second), nil, nil)
	ack, err = x.process(t, d2, stale)
	x.heldCommand(t, stale, ack, err, tripapp.StaleRevision)
	paused := x.transitionOp(t, d2, tripapp.PauseOperationType, trip, 2, handed.Add(2*time.Second), "Almoço", nil)
	ack, err = x.process(t, d2, paused)
	x.applied(t, paused, ack, err, 3)
	stop := x.eventOp(t, d2, trip, "STOP", nil, handed.Add(3*time.Second), nil)
	stopAck, err := x.process(t, d2, stop)
	x.eventState.accepted(t, stop, stopAck, err)
	resume := x.transitionOp(t, d2, tripapp.ResumeOperationType, trip, 3, handed.Add(4*time.Second), nil, nil)
	ack, err = x.process(t, d2, resume)
	x.applied(t, resume, ack, err, 4)
	if page := x.mine(t, d2); len(page.Items) != 1 || page.Items[0].TripID != trip || page.Items[0].Revision != 4 || page.Items[0].State != "IN_PROGRESS" ||
		page.Items[0].Handover == nil || page.Items[0].Handover.OperationID != operation || page.Items[0].AssetID != x.vehicle.AssetID {
		t.Fatal("the receiver's own open trips", page.Items)
	}
	if page := x.mine(t, d1); len(page.Items) != 0 {
		t.Fatal("the giving driver still lists the trip", page.Items)
	}
	wrongFinish := x.finishOp(t, d1, trip, handed.Add(5*time.Second), nil)
	ack, err = x.process(t, d1, wrongFinish)
	if code, _ := orgapi.Registered(err); code != orgapi.Forbidden || len(ack.CanonicalBytes()) != 0 || x.eventState.tripState.count(t, wrongFinish) != (tripCounts{}) {
		t.Fatal("the giving driver finished a handed-over trip", err)
	}
	finish := x.finishOp(t, d2, trip, handed.Add(5*time.Second), nil)
	ack, err = x.process(t, d2, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 5, 1)
	if page := x.mine(t, d2); len(page.Items) != 0 {
		t.Fatal("a finished trip is still listed as open", page.Items)
	}
	// Every fact keeps its original actor (B-BR-EMP-395).
	var actors string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT concat_ws('|',(SELECT author_membership_id::text FROM org_trips WHERE org_id=$1 AND id=$2),
 (SELECT string_agg(author_membership_id::text,',' ORDER BY received_at) FROM org_trip_events WHERE org_id=$1 AND trip_id=$2),
 (SELECT string_agg(author_membership_id::text,',' ORDER BY revision) FROM org_trip_transitions WHERE org_id=$1 AND trip_id=$2),
 (SELECT author_membership_id::text FROM org_trip_finishes WHERE org_id=$1 AND trip_id=$2))`, org, trip).Scan(&actors); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{d1.person.member, d1.person.member + "," + d1.person.member + "," + d2.person.member, d2.person.member + "," + d2.person.member, d2.person.member}, "|")
	if actors != want {
		t.Fatal("a fact lost its original actor", actors)
	}
	timeline := x.timeline(t, trip)
	authors := map[string]string{}
	for _, e := range timeline {
		authors[e.Item.ID] = e.AuthorMembershipID
	}
	if authors[eventOf(early)] != d1.person.member || authors[eventOf(late)] != d1.person.member || authors[eventOf(stop)] != d2.person.member || len(timeline) != 7 {
		t.Fatal("the timeline lost an actor", len(timeline))
	}
	// The giving driver still re-reads its own receipts.
	x.eventState.replays(t, d1, early, earlyAck)
	x.eventState.replays(t, d1, late, lateAck)
	x.eventState.replays(t, d2, stop, stopAck)
	// Only the current responsible member submits.
	wrong := x.submitOp(t, d1, trip, 1, handed.Add(6*time.Second))
	if _, err := x.process(t, d1, wrong); func() bool { code, _ := orgapi.Registered(err); return code != orgapi.Forbidden }() {
		t.Fatal("the giving driver submitted a handed-over trip", err)
	}
	submit := x.submitOp(t, d2, trip, 1, handed.Add(6*time.Second))
	ack, err = x.process(t, d2, submit)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 {
		t.Fatal("the receiver's submission was not accepted", err, ack.Result(), ack.Code())
	}
	// The recorder of the handover never decides the trip's review.
	if _, _, err := x.decide(sup, trip, review.Approve, ""); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("the recorder decided the handed-over trip", err)
	}
	independent := x.staff(t, "APPROVER", "ORG", "")
	if decided, _, err := x.decide(independent, trip, review.Approve, ""); err != nil || decided.State != review.Approved {
		t.Fatal("an independent reviewer could not decide", err)
	}
	if list, err := x.handovers.ListHandovers(context.Background(), x.scope, sup.actor, sup.member, tripapp.QueueFilter{Limit: 10}); err != nil ||
		len(list.Items) != 1 || list.Items[0].TripID != trip || list.Items[0].Handover != result.Handover || list.Items[0].AssetID != x.vehicle.AssetID {
		t.Fatal("the organization's handover list", err)
	}
}

// B-BR-EMP-394/396/397: an ineligible, foreign, unassigned, suspended or
// already responsible receiver is VALIDATION_FAILED; a closed trip or stale
// revision CONFLICT; a recorder who is a party SELF_APPROVAL; roles without
// trip.trip.close_for_other, uncovering structural grants FORBIDDEN; another
// organization's or an unknown trip NOT_FOUND; nothing is ever written.
func TestTripHandoverRefusesIneligibleClosedStaleSelfAndUncovered(t *testing.T) {
	x := tripHandoverFixture(t)
	org := x.scope.OrgID().String()
	d1, d2 := x.assignedDriver(t), x.assignedDriver(t)
	at := x.declaredNow(t)
	start := x.startOp(t, d1, at, absentReading)
	ack, err := x.process(t, d1, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	sup := x.staff(t, "SUPERVISOR", "ORG", "")
	nothing := func(name string) {
		t.Helper()
		var rows int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_handovers WHERE org_id=$1)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='trip.trip.handed_over')`, org).Scan(&rows); err != nil || rows != 0 {
			t.Fatal("a refused handover wrote state", name, rows, err)
		}
		if x.revisionOf(t, trip) != 1 {
			t.Fatal("a refused handover moved the revision", name)
		}
	}
	ineligible := func(name, receiver string) {
		t.Helper()
		if _, _, err := x.handOver(x.scope, sup, trip, 1, receiver, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrReceiverIneligible) {
			t.Fatal("receiver not VALIDATION_FAILED", name, err)
		}
		nothing(name)
	}
	other := legalFixtureFor(t, x.f, x.onboarding, x.legalFixtureState.actor, x.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	ineligible("foreign member", other.member)
	ineligible("unknown member", onboardingID(t))
	ineligible("unassigned driver", x.driver(t).person.member)
	ineligible("current responsible", d1.person.member)
	finance := x.staff(t, "FINANCE", "ORG", "")
	x.assign(t, finance.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	ineligible("no trip.trip.start", finance.member)
	suspended := x.assignedDriver(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, suspended.person.member); err != nil {
		t.Fatal(err)
	}
	ineligible("suspended member", suspended.person.member)
	// Separation: the ADMIN who is the receiver, or who drives the trip.
	admin := deviceReviewer(x.syncFixtureState)
	if _, _, err := x.handOver(x.scope, admin, trip, 1, admin.member, "Assumo a viagem", onboardingID(t)); !errors.Is(err, tripapp.ErrSelfHandover) {
		t.Fatal("the recorder handed the trip over to itself", err)
	}
	nothing("recorder is receiver")
	// Authority: roles without the permission and uncovering grants.
	for name, r := range map[string]tripReviewer{"auditor": x.staff(t, "AUDITOR", "ORG", ""), "driver": deviceReviewer(d2), "approver": x.staff(t, "APPROVER", "ORG", "")} {
		if _, _, err := x.handOver(x.scope, r, trip, 1, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatal("a role without trip.trip.close_for_other handed over", name, err)
		}
		nothing(name)
	}
	sul := onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic Sul','SUL','UTC','ACTIVE')`, org, sul); err != nil {
		t.Fatal(err)
	}
	if _, _, err := x.handOver(x.scope, x.staff(t, "SUPERVISOR", "UNIT", sul), trip, 1, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("a supervisor of another unit handed over", err)
	}
	nothing("uncovering unit")
	stranger := tripReviewer{actor: tripapp.ReviewActor{AccountID: other.actor.AccountID, SessionID: other.actor.SessionID}, member: other.member}
	if _, _, err := x.handOver(other.scope, stranger, trip, 1, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization's trip was not NOT_FOUND", err)
	}
	if _, _, err := x.handOver(x.scope, sup, onboardingID(t), 1, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("an unknown trip was not NOT_FOUND", err)
	}
	if _, _, err := x.handOver(x.scope, sup, trip, 1, d2.person.member, " ", onboardingID(t)); !errors.Is(err, tripapp.ErrInvalidHandover) {
		t.Fatal("a blank reason was admitted", err)
	}
	nothing("authority and shape")
	// Stale revisions.
	if _, _, err := x.handOver(x.scope, sup, trip, 2, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrHandoverConflict) {
		t.Fatal("a future revision was not CONFLICT", err)
	}
	pause := x.transitionOp(t, d1, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), nil, nil)
	ack, err = x.process(t, d1, pause)
	x.applied(t, pause, ack, err, 2)
	if _, _, err := x.handOver(x.scope, sup, trip, 1, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrHandoverConflict) {
		t.Fatal("a stale revision was not CONFLICT", err)
	}
	// A covering UNIT supervisor hands the paused trip over; the trip stays paused.
	norte := x.staff(t, "SUPERVISOR", "UNIT", x.unit)
	result, _, err := x.handOver(x.scope, norte, trip, 2, d2.person.member, "Troca de turno", onboardingID(t))
	if err != nil || result.Revision != 3 || result.Operational != "PAUSED" {
		t.Fatal("a covering UNIT supervisor could not hand a paused trip over", err)
	}
	// Separation: the ADMIN who now drives cannot hand its own trip over.
	if _, _, err := x.handOver(x.scope, x.staff(t, "SUPERVISOR", "ORG", ""), trip, 3, admin.member, "Troca de turno", onboardingID(t)); err != nil {
		t.Fatal("a handover to the assigned ADMIN", err)
	}
	if _, _, err := x.handOver(x.scope, admin, trip, 4, d2.person.member, "Devolvo a viagem", onboardingID(t)); !errors.Is(err, tripapp.ErrSelfHandover) {
		t.Fatal("the current responsible member handed its own trip over", err)
	}
	// Closed trips.
	finish := x.finishOp(t, x.syncFixtureState, trip, at.Add(time.Hour), absentReading)
	ack, err = x.process(t, x.syncFixtureState, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 5, 0)
	if _, _, err := x.handOver(x.scope, sup, trip, 5, d2.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrHandoverConflict) {
		t.Fatal("a completed trip was handed over", err)
	}
	cancelled := x.startOp(t, d2, at.Add(2*time.Hour), absentReading)
	ack, err = x.process(t, d2, cancelled)
	x.eventState.tripState.accepted(t, cancelled, ack, err, 1, 0)
	cancel := x.transitionOp(t, d2, tripapp.CancelOperationType, tripOf(cancelled), 1, at.Add(2*time.Hour+time.Second), "Cliente cancelou", nil)
	ack, err = x.process(t, d2, cancel)
	x.applied(t, cancel, ack, err, 2)
	if _, _, err := x.handOver(x.scope, sup, tripOf(cancelled), 2, d1.person.member, "Troca de turno", onboardingID(t)); !errors.Is(err, tripapp.ErrHandoverConflict) {
		t.Fatal("a cancelled trip was handed over", err)
	}
	var rows int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_handovers WHERE org_id=$1 AND trip_id=$2`, org, tripOf(cancelled)).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("a closed trip took a handover", rows, err)
	}
}

// B-BR-EMP-397 / BR-REV-003: every party of a handover — the giving and the
// receiving member and the recorder — is excluded from every review decision
// of the trip, by membership, account and canonical employee, while an
// independent reviewer decides; the database guard re-proves the exclusion.
func TestTripHandoverPartiesNeverReviewTheTrip(t *testing.T) {
	x := tripHandoverFixture(t)
	org := x.scope.OrgID().String()
	giver := x.syncFixtureState // ADMIN (ORG) with a device, assigned to the vehicle
	receiver := x.assignedDriver(t)
	x.grant(t, receiver.person.member, "SUPERVISOR", "ORG", "")
	at := x.declaredNow(t)
	start := x.startOp(t, giver, at, absentReading)
	ack, err := x.process(t, giver, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	recorder := x.staff(t, "FLEET_MANAGER", "ORG", "")
	result, _, err := x.handOver(x.scope, recorder, trip, 1, receiver.person.member, "Troca de turno", onboardingID(t))
	if err != nil {
		t.Fatal("handover", err)
	}
	finish := x.finishOp(t, receiver, trip, result.Handover.HandedOverAt.Add(time.Hour), absentReading)
	ack, err = x.process(t, receiver, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 3, 0)
	submit := x.submitOp(t, receiver, trip, 1, result.Handover.HandedOverAt.Add(2*time.Hour))
	if ack, err := x.process(t, receiver, submit); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("the receiver's submission", err)
	}
	for name, r := range map[string]tripReviewer{"giving member": deviceReviewer(giver), "receiving member": deviceReviewer(receiver), "recorder": recorder} {
		for _, kind := range []review.Kind{review.Approve, review.Return, review.Reject} {
			reason := ""
			if kind != review.Approve {
				reason = "Synthetic party decision"
			}
			if _, _, err := x.decide(r, trip, kind, reason); !errors.Is(err, review.ErrSelfApproval) {
				t.Fatal("a handover party decided the trip", name, kind, err)
			}
		}
	}
	var entries int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_review_entries WHERE org_id=$1 AND trip_id=$2`, org, trip).Scan(&entries); err != nil || entries != 1 {
		t.Fatal("a refused party decision wrote review history", entries, err)
	}
	// The guard refuses a direct OPEN entry by the recorder.
	err = pgx.BeginFunc(t.Context(), x.f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id)
VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`, org, trip, recorder.member, recorder.employee, onboardingID(t))
		return err
	})
	if !strings.Contains(errString(err), "trip self approval refused") {
		t.Fatal("the review-entry guard admitted a handover party", err)
	}
	independent := x.staff(t, "APPROVER", "ORG", "")
	if decided, _, err := x.decide(independent, trip, review.Return, "Informe a troca"); err != nil || decided.State != review.Returned {
		t.Fatal("an independent reviewer could not decide", err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handoverHoldRoot holds the organization root row lock in its own
// transaction until free is called (the barrier of the race tests).
func handoverHoldRoot(t *testing.T, x tripHandoverState) (int, func(), chan error) {
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

// B-BR-EMP-396 / 08-state-model §4.7: two handovers of one revision, and a
// handover against the giving driver's pause of that revision, race behind
// the organization root lock in both orders; exactly one command applies and
// one member is responsible.
func TestTripHandoverConcurrentCommandsResolveToOne(t *testing.T) {
	for _, race := range []string{"handover-first-of-two", "handover-second-of-two", "handover-then-pause", "pause-then-handover"} {
		t.Run(race, func(t *testing.T) {
			x := tripHandoverFixture(t)
			org := x.scope.OrgID().String()
			d1, d2, d3 := x.assignedDriver(t), x.assignedDriver(t), x.assignedDriver(t)
			at := x.declaredNow(t)
			start := x.startOp(t, d1, at, absentReading)
			ack, err := x.process(t, d1, start)
			x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
			trip := tripOf(start)
			sup, other := x.staff(t, "SUPERVISOR", "ORG", ""), x.staff(t, "FLEET_MANAGER", "ORG", "")
			x.handovers.Pool = x.f.RuntimePool(t, 4)
			config := tripLifecycleIntakeConfig(x.syncFixtureState)
			config.Pool = x.f.RuntimePool(t, 4)
			intake := syncpg.NewStore(config)
			pause := x.transitionOp(t, d1, tripapp.PauseOperationType, trip, 1, at.Add(time.Second), nil, nil)
			type outcome struct {
				name string
				err  error
				ack  protocol.Acknowledgement
			}
			results := make(chan outcome, 2)
			handover := func(name string, r tripReviewer, receiver syncFixtureState) func() {
				return func() {
					_, _, err := x.handOver(x.scope, r, trip, 1, receiver.person.member, "Troca de turno", handoverOpID())
					results <- outcome{name: name, err: err}
				}
			}
			pauseStep := func() {
				ack, err := intake.Process(context.Background(), d1.source, pause, protocol.DefaultLimits(), x.handler(pause))
				results <- outcome{name: "pause", err: err, ack: ack}
			}
			var steps []func()
			switch race {
			case "handover-first-of-two":
				steps = []func(){handover("to-d2", sup, d2), handover("to-d3", other, d3)}
			case "handover-second-of-two":
				steps = []func(){handover("to-d3", other, d3), handover("to-d2", sup, d2)}
			case "handover-then-pause":
				steps = []func(){handover("to-d2", sup, d2), pauseStep}
			case "pause-then-handover":
				steps = []func(){pauseStep, handover("to-d2", sup, d2)}
			}
			pid, free, done := handoverHoldRoot(t, x)
			var wg sync.WaitGroup
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
			var handovers int
			var responsible string
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_handovers WHERE org_id=$1 AND trip_id=$2),(SELECT r.membership_id::text FROM org_trip_responsible($1,$2,NULL,NULL) r)`,
				org, trip).Scan(&handovers, &responsible); err != nil {
				t.Fatal(err)
			}
			switch race {
			case "handover-first-of-two", "handover-second-of-two":
				winner, loser, member := "to-d2", "to-d3", d2.person.member
				if race == "handover-second-of-two" {
					winner, loser, member = "to-d3", "to-d2", d3.person.member
				}
				if got[winner].err != nil || !errors.Is(got[loser].err, tripapp.ErrHandoverConflict) || handovers != 1 || responsible != member || x.revisionOf(t, trip) != 2 {
					t.Fatal("two handovers of one revision", got, handovers, responsible)
				}
			case "handover-then-pause":
				// The giving driver no longer holds the trip: FORBIDDEN, nothing stored.
				if got["to-d2"].err != nil || handovers != 1 || responsible != d2.person.member || x.stateOf(t, trip) != "IN_PROGRESS" {
					t.Fatal("handover then pause", got)
				}
				x.refusedCommand(t, pause, got["pause"].ack, got["pause"].err, orgapi.Forbidden)
			case "pause-then-handover":
				if !errors.Is(got["to-d2"].err, tripapp.ErrHandoverConflict) || handovers != 0 || responsible != d1.person.member || x.stateOf(t, trip) != "PAUSED" {
					t.Fatal("pause then handover", got)
				}
				x.applied(t, pause, got["pause"].ack, got["pause"].err, 2)
			}
		})
	}
}

// B-BR-EMP-396: the handover facts are forced-RLS and insert-only; the
// database refuses a forged revision, a giving member who is not the current
// responsible one, an inactive receiver, a recorder who is a party, a closed
// trip and a handover published without its audit event, and moves no trip
// column.
func TestTripHandoverSchemaIsForcedRLSGuardedAndInsertOnly(t *testing.T) {
	x := tripHandoverFixture(t)
	org := x.scope.OrgID().String()
	d1, d2, d3 := x.assignedDriver(t), x.assignedDriver(t), x.assignedDriver(t)
	at := x.declaredNow(t)
	start := x.startOp(t, d1, at, absentReading)
	ack, err := x.process(t, d1, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	sup := x.staff(t, "SUPERVISOR", "ORG", "")
	if _, _, err := x.handOver(x.scope, sup, trip, 1, d2.person.member, "Troca de turno", onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT to_jsonb(t)::text FROM org_trips t WHERE org_id=$1 AND id=$2`, org, trip).Scan(&before); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_handovers TO "+role+";GRANT INSERT ON org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
	// insert is a direct handover row of the trip with the given parties; the
	// audit event is written first so only the guard under test can refuse.
	insert := func(revision int64, from, fromEmployee, to, toEmployee, recorder, recorderEmployee string) error {
		op := onboardingID(t)
		return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			hash := strings.Repeat("a", 64)
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash)
 SELECT $1,$2,m.account_id,'trip.trip.handed_over','trip',$3,$4 FROM org_memberships m WHERE m.org_id=$1 AND m.id=$5`, org, op, trip, hash, recorder); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `INSERT INTO org_trip_handovers(org_id,trip_id,revision,operation_id,request_hash,from_membership_id,from_employee_id,to_membership_id,to_employee_id,
 recorder_membership_id,recorder_employee_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'Direta')`, org, trip, revision, op, hash, from, fromEmployee, to, toEmployee, recorder, recorderEmployee)
			return err
		})
	}
	p1, p2, p3 := d1.person, d2.person, d3.person
	for name, err := range map[string]error{
		"forged revision":            insert(4, p2.member, p2.employee, p3.member, p3.employee, sup.member, sup.employee),
		"stale revision":             insert(2, p2.member, p2.employee, p3.member, p3.employee, sup.member, sup.employee),
		"giver not responsible":      insert(3, p1.member, p1.employee, p3.member, p3.employee, sup.member, sup.employee),
		"receiver employee mismatch": insert(3, p2.member, p2.employee, p3.member, p1.employee, sup.member, sup.employee),
		"recorder is the receiver":   insert(3, p2.member, p2.employee, sup.member, sup.employee, sup.member, sup.employee),
		"recorder is the giver":      insert(3, p2.member, p2.employee, p3.member, p3.employee, p2.member, p2.employee),
		"receiver is the giver":      insert(3, p2.member, p2.employee, p2.member, p2.employee, sup.member, sup.employee),
	} {
		if err == nil {
			t.Fatal("the handover guard admitted", name)
		}
	}
	// A suspended receiver is refused by the guard.
	if err := unitSchemaExec(t, x.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, p3.member); err != nil {
		t.Fatal(err)
	}
	if err := insert(3, p2.member, p2.employee, p3.member, p3.employee, sup.member, sup.employee); err == nil || !strings.Contains(err.Error(), "receiver unavailable") {
		t.Fatal("the guard admitted a suspended receiver", err)
	}
	// Without its audit event a handover is not published at commit.
	err = database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_trip_handovers(org_id,trip_id,revision,operation_id,request_hash,from_membership_id,from_employee_id,to_membership_id,to_employee_id,
 recorder_membership_id,recorder_employee_id,reason) VALUES($1,$2,3,$3,$4,$5,$6,$7,$8,$9,$10,'Direta')`, org, trip, onboardingID(t), strings.Repeat("b", 64), p2.member, p2.employee, p1.member, p1.employee, sup.member, sup.employee)
		return err
	})
	if err == nil {
		t.Fatal("a handover without its audit event was published")
	}
	for _, statement := range []string{
		`UPDATE org_trip_handovers SET reason='Reescrito' WHERE trip_id=$1`,
		`DELETE FROM org_trip_handovers WHERE trip_id=$1`,
		`TRUNCATE org_trip_handovers`,
	} {
		err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), strings.ReplaceAll(statement, "$1", "'"+trip+"'"))
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "insert-only") {
			t.Fatal("handover history accepted a direct change", statement, err)
		}
	}
	var after string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT to_jsonb(t)::text FROM org_trips t WHERE org_id=$1 AND id=$2`, org, trip).Scan(&after); err != nil || after != before {
		t.Fatal("a handover or a refused change moved a trip column", err)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_handovers`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("handovers visible without an organization setting", visible, err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE relname='org_trip_handovers'`).Scan(&forced); err != nil || !forced {
		t.Fatal("the handover table is not forced-RLS", err)
	}
	var rows int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_handovers WHERE org_id=$1 AND trip_id=$2`, org, trip).Scan(&rows); err != nil || rows != 1 || x.revisionOf(t, trip) != 2 {
		t.Fatal("refused direct handovers changed the trip", rows, err)
	}
	// A closed trip takes no handover.
	finish := x.finishOp(t, d2, trip, at.Add(time.Hour), absentReading)
	ack, err = x.process(t, d2, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 3, 0)
	if err := insert(4, p2.member, p2.employee, p1.member, p1.employee, sup.member, sup.employee); err == nil || !strings.Contains(err.Error(), "already closed") {
		t.Fatal("the guard admitted a handover of a closed trip", err)
	}
}

// EMP-TRIP-012 with EMP-TRIP-014/018: a handover receiver never closes the
// trip it is responsible for (SELF_APPROVAL in the application, the domain
// and the closure guard), another supervisor's closure ends it and a closed
// trip takes no handover; the open work an offboarding must flag follows the
// responsibility, not the start: the giving member's offboarding lists no
// flag for the handed-over trip, the receiver's needs it and keeps the trip
// open.
func TestTripHandoverRespectsClosureAndOffboarding(t *testing.T) {
	x := tripEndFixture(t)
	handoverGrants(t, x.f)
	handovers := tripHandoverStoreOf(t, x.legalFixtureState, x.assets)
	handOver := func(r tripReviewer, trip string, revision int64, receiver string) (tripapp.HandoverResult, error) {
		command, err := tripapp.NewHandoverCommand(tripapp.HandoverInput{OperationID: onboardingID(t), TripID: trip, ExpectedRevision: revision, ReceiverMembershipID: receiver, Reason: "Troca de turno"})
		if err != nil {
			return tripapp.HandoverResult{}, err
		}
		result, _, err := handovers.HandOver(context.Background(), x.scope, r.actor, r.member, command)
		return result, err
	}
	d1, at := x.assignedDriver(t)
	start := x.startOp(t, d1, at, absentReading)
	ack, err := x.process(t, d1, start)
	x.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	sup := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	receiver := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	x.assign(t, receiver.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	if _, err := handOver(sup, trip, 1, receiver.member); err != nil {
		t.Fatal("handover to a supervisor who drives", err)
	}
	if _, _, err := x.close(x.scope, receiver, trip, 2, "Encerramento", "Sem leitura", onboardingID(t)); !errors.Is(err, tripdomain.ErrSelfClosure) {
		t.Fatal("the current responsible member closed its own trip", err)
	}
	if closed, _, err := x.close(x.scope, sup, trip, 2, "Motorista não retornou", "Sem leitura", onboardingID(t)); err != nil || closed.Revision != 3 {
		t.Fatal("another supervisor's closure of a handed-over trip", err)
	}
	if _, err := handOver(sup, trip, 3, d1.source.MembershipID); !errors.Is(err, tripapp.ErrHandoverConflict) {
		t.Fatal("a closed trip was handed over", err)
	}
	// Offboarding follows the responsibility.
	d2, _ := x.assignedDriver(t)
	second := x.startOp(t, d1, at.Add(2*time.Hour), absentReading)
	ack, err = x.process(t, d1, second)
	x.accepted(t, second, ack, err, 1, 0)
	if _, err := handOver(sup, tripOf(second), 1, d2.source.MembershipID); err != nil {
		t.Fatal("handover of the second trip", err)
	}
	key := "TRIP:" + tripOf(second)
	giver := x.review(t, x.offboarding, d1.source.MembershipID)
	if slices.Contains(keysOf(giver.Items), key) {
		t.Fatal("the giving member's offboarding lists the handed-over trip")
	}
	taker := x.review(t, x.offboarding, d2.source.MembershipID)
	if !slices.Contains(keysOf(taker.Items), key) {
		t.Fatal("the receiver's offboarding does not list its open trip", keysOf(taker.Items))
	}
	without := slices.DeleteFunc(keysOf(taker.Items), func(k string) bool { return k == key })
	if _, err := x.offboard(t, x.offboarding, taker, without); err == nil {
		t.Fatal("the receiver's offboarding left its open trip unflagged")
	}
	if _, err := x.offboard(t, x.offboarding, giver, keysOf(giver.Items)); err != nil {
		t.Fatal("the giving member's offboarding without the handed-over trip", err)
	}
	if _, err := x.offboard(t, x.offboarding, taker, keysOf(taker.Items)); err != nil {
		t.Fatal("the receiver's flagged offboarding", err)
	}
	if x.membershipState(t, d2.source.MembershipID) != "ENDED" {
		t.Fatal("the receiver's membership did not end")
	}
	var state string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_trips WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), tripOf(second)).Scan(&state); err != nil || state != "IN_PROGRESS" {
		t.Fatal("offboarding closed the handed-over trip", state, err)
	}
}
