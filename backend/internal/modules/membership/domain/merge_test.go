package domain

import (
	"errors"
	"testing"
)

func TestEmployeeMergePreservesIdentityAndLiveSurvivor(t *testing.T) {
	scope, p := pendingInvitation(t)
	source, _ := NewEmployee(scope, p.EmployeeID)
	survivor, _ := NewEmployee(scope, p.MembershipID)
	next, e := PlanEmployeeMerge(scope, source, survivor, 1, 1, 0, 1, 0, "Synthetic duplicate review")
	if e != nil {
		t.Fatal(e)
	}
	v, _ := next.Snapshot(scope)
	if v.ID != p.EmployeeID || v.State != EmployeeTerminated || v.MergedInto != p.MembershipID || v.Revision != 2 {
		t.Fatal("historical identity")
	}
	if _, e := PlanEmployeeMerge(scope, source, survivor, 1, 1, 1, 1, 0, "Synthetic duplicate review"); !errors.Is(e, ErrMergeConflict) {
		t.Fatal("two live", e)
	}
	if _, e := PlanEmployeeMerge(scope, source, survivor, 1, 1, 1, 0, 0, "Synthetic duplicate review"); !errors.Is(e, ErrMergeConflict) {
		t.Fatal("wrong live orientation", e)
	}
	if _, e := PlanEmployeeMerge(scope, source, source, 1, 1, 0, 0, 0, "Synthetic duplicate review"); !errors.Is(e, ErrMergeConflict) {
		t.Fatal("self merge", e)
	}
	if _, e := PlanEmployeeMerge(scope, next, survivor, 2, 1, 0, 0, 0, "Synthetic duplicate review"); !errors.Is(e, ErrMergeConflict) {
		t.Fatal("merged source", e)
	}
	inactive, _ := survivor.Transition(scope, 1, EmployeeInactive, EmployeeConditions{OwnershipProtected: true, ReasonRecorded: true})
	if _, e := PlanEmployeeMerge(scope, source, inactive, 1, 2, 0, 0, 1, "Synthetic duplicate review"); !errors.Is(e, ErrMergeConflict) {
		t.Fatal("ineligible team survivor", e)
	}
	terminated, _ := source.Transition(scope, 1, EmployeeTerminated, EmployeeConditions{OwnershipProtected: true, ResponsibilitiesReviewed: true})
	if _, e := PlanEmployeeMerge(scope, terminated, survivor, 2, 1, 0, 0, 0, "Synthetic duplicate review"); e != nil {
		t.Fatal("terminated historical record", e)
	}
}
