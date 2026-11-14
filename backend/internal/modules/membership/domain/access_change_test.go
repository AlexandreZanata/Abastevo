package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"strings"
	"testing"
)

func TestMembershipManagerChangeRequiresCurrentRecordedAuthority(t *testing.T) {
	scope, pending := pendingInvitation(t)
	employee, e := NewEmployee(scope, pending.EmployeeID)
	if e != nil {
		t.Fatal(e)
	}
	account, e := PersonalAccount(pending.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	member, e := RehydrateMembership(scope, MembershipSnapshot{OrgID: scope.OrgID(), ID: pending.MembershipID, EmployeeID: pending.EmployeeID, Account: account, Model: ExistingAccount, State: MembershipActive, Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	suspend, e := PlanManagerAccessChange(scope, member, pending.IssuerMembershipID, 1, ManagerSuspend, "Synthetic pause", employee, 1, true)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := suspend.Snapshot(scope)
	if v.State != MembershipSuspended || v.Suspension != ByManager || v.Revision != 2 {
		t.Fatal("suspension state")
	}
	if _, e := PlanManagerAccessChange(scope, member, pending.MembershipID, 1, ManagerSuspend, "Synthetic pause", employee, 1, true); e == nil {
		t.Fatal("self suspension")
	}
	if _, e := PlanManagerAccessChange(scope, member, pending.IssuerMembershipID, 1, ManagerSuspend, "Synthetic pause", employee, 1, false); e == nil {
		t.Fatal("unprotected owner")
	}
	if _, e := PlanManagerAccessChange(scope, suspend, pending.IssuerMembershipID, 2, ManagerReactivate, "Synthetic resume", employee, 0, true); e == nil {
		t.Fatal("no current original assignment")
	}
	active, e := PlanManagerAccessChange(scope, suspend, pending.IssuerMembershipID, 2, ManagerReactivate, "Synthetic resume", employee, 1, true)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := active.Snapshot(scope)
	if a.State != MembershipActive || a.Suspension != "" || a.Revision != 3 {
		t.Fatal("reactivation state")
	}
	inactive, e := employee.Transition(scope, 1, EmployeeInactive, EmployeeConditions{OwnershipProtected: true, ReasonRecorded: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := PlanManagerAccessChange(scope, suspend, pending.IssuerMembershipID, 2, ManagerReactivate, "Synthetic resume", inactive, 1, true); e == nil {
		t.Fatal("inactive employee")
	}
	for _, reason := range []string{"", "  ", "synthetic\nreason", "synthetic\u202ereason"} {
		if ValidateAccessChangeReason(reason) == nil {
			t.Fatal("unsafe reason")
		}
	}
}

func TestMembershipAccessMetadataIsScopedAndCannotCarryAccountAuthority(t *testing.T) {
	scope, pending := pendingInvitation(t)
	v := AccessState{OrgID: scope.OrgID(), ID: pending.MembershipID, EmployeeID: pending.EmployeeID, Model: ExistingAccount, State: MembershipSuspended, Revision: 2, Suspension: ByManager}
	if e := v.Validate(scope); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*AccessState){func(v *AccessState) { v.ID = "bad" }, func(v *AccessState) { v.EmployeeID = "bad" }, func(v *AccessState) { v.Model = "unknown" }, func(v *AccessState) { v.Revision = 0 }, func(v *AccessState) { v.State = "unknown" }, func(v *AccessState) { v.Suspension = "unknown" }, func(v *AccessState) { v.EndReason = Left }} {
		bad := v
		mutate(&bad)
		if bad.Validate(scope) == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
	v.State = MembershipActive
	v.Suspension = ""
	if e := v.Validate(scope); e != nil {
		t.Fatal(e)
	}
	v.EndReason = Left
	if v.Validate(scope) == nil {
		t.Fatal("active ended cause")
	}
	v.State = MembershipEnded
	v.EndReason = Left
	if e := v.Validate(scope); e != nil {
		t.Fatal(e)
	}
	v.EndReason = "unknown"
	if v.Validate(scope) == nil {
		t.Fatal("invalid ended cause")
	}
	v.EndReason = Left
	v.Suspension = "unknown"
	if v.Validate(scope) == nil {
		t.Fatal("invalid preserved origin")
	}
	if v.Validate(kernel.OrgScope{}) != ErrScope {
		t.Fatal("missing scope")
	}
	for _, reason := range []string{strings.Repeat("x", 281), string([]byte{0xff})} {
		if ValidateAccessChangeReason(reason) == nil {
			t.Fatal("invalid reason boundary")
		}
	}
	if ValidateAccessChangeReason(strings.Repeat("x", 280)) != nil {
		t.Fatal("valid reason boundary")
	}
}
