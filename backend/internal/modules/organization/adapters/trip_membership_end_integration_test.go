//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	memberapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// tripEndState is the EMP-TRIP-014 fixture with the real IDN-010
// offboarding store (its open-work source wired to trips) and a late intake
// store for the ended member's device (EMP-OFFL-009).
type tripEndState struct {
	tripClosureState
	offboarding *memberpg.OffboardingStore
	late        *syncpg.Store
}

func tripEndOffboardingStore(t *testing.T, x tripClosureState, work memberapp.OpenWork) *memberpg.OffboardingStore {
	t.Helper()
	x.f.GrantOffboardingCommands(t)
	grantTeamTables(t, x.legalFixtureState)
	catalog, err := audit.NewCatalog(memberpg.OffboardingAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &memberpg.OffboardingStore{Pool: x.f.Runtime, Memberships: x.legalFixtureState.store.Memberships, Audit: auditpg.NewStore(x.f.Runtime, catalog), Work: work,
		Authority: &orgpg.OffboardingAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles, Capabilities: x.flags}}}
}

func tripEndFixture(t *testing.T) tripEndState {
	t.Helper()
	x := tripEndState{tripClosureState: tripClosureFixture(t)}
	x.offboarding = tripEndOffboardingStore(t, x.tripClosureState, trippg.MembershipOpenWork{})
	x.f.GrantLateIntake(t)
	config := tripAmendIntakeConfig(x.syncFixtureState)
	config.ConfirmLate = lateIntakeConfig(x.syncFixtureState).ConfirmLate
	x.late = syncpg.NewStore(config)
	return x
}

func (x tripEndState) manager() memberapp.EmployeeActor {
	return memberapp.EmployeeActor{AccountID: x.actor.AccountID, SessionID: x.actor.SessionID}
}

func (x tripEndState) review(t *testing.T, s *memberpg.OffboardingStore, target string) memberapp.OffboardingView {
	t.Helper()
	v, err := s.ReadOffboarding(t.Context(), x.scope, x.manager(), x.member, target, false)
	if err != nil {
		t.Fatal("offboarding review", err)
	}
	return v
}

func (x tripEndState) offboard(t *testing.T, s *memberpg.OffboardingStore, v memberapp.OffboardingView, flags []string) (memberapp.OffboardingResult, error) {
	t.Helper()
	c := memberapp.OffboardingCommand{OperationID: onboardingID(t), MembershipID: v.State.ID, ExpectedRevision: v.State.Revision, EmployeeRevision: v.EmployeeRevision,
		Reason: "Synthetic access ending", HandoverDigest: v.Digest, Flags: flags}
	return s.CompleteOffboarding(context.Background(), x.scope, x.manager(), x.member, c)
}

func keysOf(items []memberdomain.HandoverItem) []string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, i.Key)
	}
	return out
}

func (x tripEndState) membershipState(t *testing.T, member string) string {
	t.Helper()
	var state string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_memberships WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), member).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

// BUC-EMP-204 / B-BR-EMP-404/405 on real PostGIS: the open trip is listed and
// flagged, an offboarding omitting it is refused (by the review and by the
// database), the trip survives the end unchanged and is closed by a scoped
// supervisor.
func TestTripOffboardingFlagsTheOpenTripAndNeverClosesIt(t *testing.T) {
	x := tripEndFixture(t)
	d, at := x.assignedDriver(t)
	trip := x.running(t, d, at, nil)
	other, _ := x.assignedDriver(t)
	row := x.tripRow(t, trip)
	key := "TRIP:" + trip
	v := x.review(t, x.offboarding, d.source.MembershipID)
	if !slices.Contains(keysOf(v.Items), key) {
		t.Fatal("the open trip is not an open responsibility", keysOf(v.Items))
	}
	for _, i := range v.Items {
		if i.Key == key && i.Revision != 1 {
			t.Fatal("trip item revision", i.Revision)
		}
	}
	if o := x.review(t, x.offboarding, other.source.MembershipID); slices.ContainsFunc(o.Items, func(i memberdomain.HandoverItem) bool { return i.Key == key }) {
		t.Fatal("another member's offboarding lists the trip")
	}
	// The review cannot skip the trip.
	without := slices.DeleteFunc(keysOf(v.Items), func(k string) bool { return k == key })
	if _, err := x.offboard(t, x.offboarding, v, without); !errors.Is(err, memberdomain.ErrOpenResponsibilities) {
		t.Fatal("an offboarding without the trip flag was not OPEN_RESPONSIBILITIES", err)
	}
	// A source that omits the trip is refused by the database guard.
	blind := tripEndOffboardingStore(t, x.tripClosureState, nil)
	b := x.review(t, blind, d.source.MembershipID)
	if slices.Contains(keysOf(b.Items), key) {
		t.Fatal("the blind source listed the trip")
	}
	if _, err := x.offboard(t, blind, b, keysOf(b.Items)); err == nil {
		t.Fatal("the database admitted an offboarding leaving the open trip unflagged")
	}
	if x.membershipState(t, d.source.MembershipID) != "ACTIVE" || x.tripRow(t, trip) != row {
		t.Fatal("a refused offboarding changed the membership or the trip")
	}
	// The flagged offboarding ends access and leaves the trip as it was.
	out, err := x.offboard(t, x.offboarding, v, keysOf(v.Items))
	if err != nil || out.View.State.State != memberdomain.MembershipEnded || !slices.Contains(keysOf(out.View.Items), key) {
		t.Fatal("flagged offboarding", err)
	}
	if x.tripRow(t, trip) != row {
		t.Fatal("offboarding changed, closed or deleted the trip")
	}
	if after := x.review(t, x.offboarding, d.source.MembershipID); !slices.Contains(keysOf(after.Items), key) {
		t.Fatal("the retained trip flag is not readable after the end")
	}
	supervisor := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	page := x.open(t, supervisor)
	if len(page.Items) != 1 || page.Items[0].TripID != trip || page.Items[0].AuthorState != "ENDED" {
		t.Fatal("the open-trip list does not show the ended member's trip", len(page.Items))
	}
	// Resolution under scoped authority: an uncovering grant cannot close it,
	// the supervisor can (EMP-TRIP-014).
	if _, _, err := x.close(x.scope, x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.installed.AssetID), trip, 1, closureReason, closureMissing, onboardingID(t)); err == nil {
		t.Fatal("an uncovering grant closed the ended member's trip")
	}
	if result, _, err := x.close(x.scope, supervisor, trip, 1, "Motorista desligado", closureMissing, onboardingID(t)); err != nil || result.Operational != tripdomain.Completed {
		t.Fatal("the supervisor could not close the ended member's trip", err)
	}
	var trips int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trips WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), trip).Scan(&trips); err != nil || trips != 1 {
		t.Fatal("the trip row is gone", trips, err)
	}
	// Another organization never sees the membership or the trip.
	foreign := legalFixtureFor(t, x.f, x.onboarding, x.legalFixtureState.actor, x.flags)
	if _, _, err := foreign.onboarding.Repository.ActivateOrganization(t.Context(), foreign.scope, foreign.actor, foreign.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := x.offboarding.ReadOffboarding(t.Context(), foreign.scope, memberapp.EmployeeActor{AccountID: foreign.actor.AccountID, SessionID: foreign.actor.SessionID}, foreign.member, d.source.MembershipID, false); err == nil {
		t.Fatal("another organization read the ended member's responsibilities")
	}
}

// B-BR-EMP-406 (EMP-OFFL-009): after the end, the ended member's finish
// declared before it is held ACCESS_ENDED_BEFORE_RECEIPT with its reading and
// never applied; work declared after it is MEMBERSHIP_ENDED; a finish of
// another member's trip is MALFORMED; nothing reaches the normal path.
func TestTripLateIntakeAfterTheMembershipEnds(t *testing.T) {
	x := tripEndFixture(t)
	d, at := x.assignedDriver(t)
	trip := x.running(t, d, at, nil)
	before := x.finishOp(t, d, trip, at.Add(time.Microsecond), nil)
	v := x.review(t, x.offboarding, d.source.MembershipID)
	if _, err := x.offboard(t, x.offboarding, v, keysOf(v.Items)); err != nil {
		t.Fatal(err)
	}
	var ended time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT ended_at FROM org_memberships WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), d.source.MembershipID).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	// Normal work is refused for the ended member.
	if _, err := x.process(t, d, x.finishOp(t, d, trip, ended.Add(time.Minute), absentReading)); err == nil {
		t.Fatal("the ended member's normal intake was admitted")
	}
	late := func(op protocol.Operation) (protocol.Acknowledgement, error) {
		handler := x.finish
		if op.Snapshot().Type == tripapp.StartOperationType {
			handler = x.start
		}
		return x.late.ProcessLate(context.Background(), d.source, op, protocol.DefaultLimits(), handler)
	}
	ack, err := late(before)
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != syncapp.LateHeldCode || ack.Snapshot().ReviewItemID == "" {
		t.Fatal("the pre-end finish was not held", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, before); c != (tripCounts{heldFinishes: 1, items: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("the held late finish applied or lost a fact: %+v", c)
	}
	var value int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT reading_value FROM org_trip_held_finishes WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), before.ID()).Scan(&value); err != nil || value != 184_610_000 {
		t.Fatal("the held late finish lost its reading", value, err)
	}
	if c := x.closureFacts(t, trip); c.state != "IN_PROGRESS" || c.finishes != 0 {
		t.Fatalf("a late finish completed the trip: %+v", c)
	}
	for range 2 {
		again, err := late(before)
		if err != nil || string(again.CanonicalBytes()) != string(ack.CanonicalBytes()) {
			t.Fatal("late replay", err)
		}
	}
	// Work declared after the end is refused and stores nothing.
	for name, op := range map[string]protocol.Operation{
		"finish": x.finishOp(t, d, trip, ended.Add(time.Minute), absentReading),
		"start":  x.startOp(t, d, ended.Add(time.Minute), absentReading),
	} {
		ack, err := late(op)
		if err != nil || ack.Result() != protocol.Rejected || ack.Code() != syncapp.LateMembershipEndedCode {
			t.Fatal("work after the end was not MEMBERSHIP_ENDED", name, err, ack.Result(), ack.Code())
		}
		if c := x.count(t, op); c.trips+c.finishes+c.heldStarts+c.heldFinishes+c.items+c.readings+c.effects != 0 {
			t.Fatalf("refused late %s stored a fact: %+v", name, c)
		}
	}
	// A finish naming an unknown trip is MALFORMED (EDR-198).
	unknown := x.finishOp(t, d, onboardingID(t), at.Add(2*time.Microsecond), absentReading)
	if ack, err := late(unknown); err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("a late finish of an unknown trip", err, ack.Result(), ack.Code())
	}
}

// B-BR-EMP-406 negative: a key revoked lost or compromised is refused as a
// whole, even for a finish declared before the end.
func TestTripLateIntakeRefusesALostDevice(t *testing.T) {
	x := tripEndFixture(t)
	d, at := x.assignedDriver(t)
	trip := x.running(t, d, at, nil)
	before := x.finishOp(t, d, trip, at.Add(time.Microsecond), nil)
	record, err := x.devices.CheckDevice(t.Context(), x.scope, d.source.Actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.devices.RevokeDevice(t.Context(), x.scope, employeeActor(x.legalFixtureState), x.member, d.person.member, false, deviceRevokeCommand(t, x.scope, record, memberdomain.LostOrCompromised)); err != nil {
		t.Fatal(err)
	}
	v := x.review(t, x.offboarding, d.source.MembershipID)
	if _, err := x.offboard(t, x.offboarding, v, keysOf(v.Items)); err != nil {
		t.Fatal(err)
	}
	ack, err := x.late.ProcessLate(context.Background(), d.source, before, protocol.DefaultLimits(), x.finish)
	lateSourceDenied(t, err, memberapp.ErrDeviceRevoked)
	if len(ack.CanonicalBytes()) != 0 {
		t.Fatal("a lost device received an acknowledgement")
	}
	if c := x.count(t, before); c != (tripCounts{}) {
		t.Fatalf("a lost device's finish stored something: %+v", c)
	}
	if c := x.closureFacts(t, trip); c.state != "IN_PROGRESS" {
		t.Fatalf("the trip changed: %+v", c)
	}
}
