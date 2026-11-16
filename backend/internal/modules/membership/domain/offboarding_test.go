package domain

import (
	"errors"
	"testing"
)

func TestOffboardingRequiresExactReviewAndPreservesCause(t *testing.T) {
	scope, p := pendingInvitation(t)
	account, _ := PersonalAccount(p.OperationID)
	m, e := RehydrateMembership(scope, MembershipSnapshot{OrgID: scope.OrgID(), ID: p.MembershipID, EmployeeID: p.EmployeeID, Account: account, Model: ExistingAccount, State: MembershipSuspended, Suspension: ByManager, Revision: 2})
	if e != nil {
		t.Fatal(e)
	}
	employee, _ := NewEmployee(scope, p.EmployeeID)
	items := []HandoverItem{{Key: "SETTING:billing:primary", Revision: 3}}
	digest, e := HandoverDigest(scope, m, employee, items)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := PlanOffboarding(scope, m, employee, 2, false, true, digest, nil, items, true); !errors.Is(e, ErrOpenResponsibilities) {
		t.Fatal("missing review", e)
	}
	if _, e := PlanOffboarding(scope, m, employee, 2, false, true, digest, []string{items[0].Key}, items, false); e == nil {
		t.Fatal("owner bypass")
	}
	next, e := PlanOffboarding(scope, m, employee, 2, false, true, digest, []string{items[0].Key}, items, true)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := next.Snapshot(scope)
	if v.State != MembershipEnded || v.EndReason != Left || v.Suspension != ByManager || v.Revision != 3 {
		t.Fatal("lost cause")
	}
	if _, e := PlanOffboarding(scope, m, employee, 2, true, true, digest, []string{items[0].Key}, items, true); e == nil {
		t.Fatal("self termination")
	}
	if _, e := PlanOffboarding(scope, m, employee, 2, false, false, "changed", []string{items[0].Key}, items, true); e == nil {
		t.Fatal("stale review")
	}
}

// EMP-TRIP-018 (B-BR-EMP-404): a trip in progress is a handover item
// TRIP:<canonical trip id> at its operational revision 1, digested and
// flagged like any other item.
func TestOffboardingTripItemsAreExactAndMustBeFlagged(t *testing.T) {
	scope, p := pendingInvitation(t)
	account, _ := PersonalAccount(p.OperationID)
	m, e := RehydrateMembership(scope, MembershipSnapshot{OrgID: scope.OrgID(), ID: p.MembershipID, EmployeeID: p.EmployeeID, Account: account, Model: ExistingAccount, State: MembershipActive, Revision: 2})
	if e != nil {
		t.Fatal(e)
	}
	employee, _ := NewEmployee(scope, p.EmployeeID)
	trip := "TRIP:7b0e2a54-0000-4000-8000-000000008701"
	if (HandoverItem{Key: trip, Revision: 1}).Validate() != nil {
		t.Fatal("a trip in progress is not a handover item")
	}
	for _, bad := range []HandoverItem{{Key: trip, Revision: 2}, {Key: "TRIP:7B0E2A54-0000-4000-8000-000000008701", Revision: 1}, {Key: "TRIP:", Revision: 1},
		{Key: "TRIP:7b0e2a54-0000-4000-8000-000000008701:x", Revision: 1}, {Key: "TRIPS:7b0e2a54-0000-4000-8000-000000008701", Revision: 1}} {
		if bad.Validate() == nil {
			t.Fatal("invalid trip item admitted", bad.Key, bad.Revision)
		}
	}
	items := []HandoverItem{{Key: "SETTING:billing:primary", Revision: 3}, {Key: trip, Revision: 1}}
	digest, e := HandoverDigest(scope, m, employee, items)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := PlanOffboarding(scope, m, employee, 2, false, false, digest, []string{items[0].Key}, items, true); !errors.Is(e, ErrOpenResponsibilities) {
		t.Fatal("an unflagged trip was accepted", e)
	}
	next, e := PlanOffboarding(scope, m, employee, 2, false, false, digest, []string{items[0].Key, trip}, items, true)
	if e != nil {
		t.Fatal(e)
	}
	if v, _ := next.Snapshot(scope); v.State != MembershipEnded || v.EndReason != Offboarded {
		t.Fatal("flagged offboarding did not end the membership")
	}
}
