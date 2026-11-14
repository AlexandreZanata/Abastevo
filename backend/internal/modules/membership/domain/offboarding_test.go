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
