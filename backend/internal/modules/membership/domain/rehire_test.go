package domain

import (
	"errors"
	"testing"
)

func TestRehireRequiresUnmergedTerminationAndNoLivePeriod(t *testing.T) {
	scope, p := pendingInvitation(t)
	e, _ := NewEmployee(scope, p.EmployeeID)
	terminated, err := e.Transition(scope, 1, EmployeeTerminated, EmployeeConditions{OwnershipProtected: true, ResponsibilitiesReviewed: true})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := PlanRehire(scope, terminated, 2, 0, "Synthetic rehire")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := reopened.Snapshot(scope)
	if v.ID != p.EmployeeID || v.State != EmployeeActive || v.Revision != 3 {
		t.Fatal("identity or revision")
	}
	for _, count := range []int64{-1, 1} {
		if _, err := PlanRehire(scope, terminated, 2, count, "Synthetic rehire"); err == nil {
			t.Fatal("invalid live period count")
		}
	}
	if _, err := PlanRehire(scope, e, 1, 0, "Synthetic rehire"); !errors.Is(err, ErrTransition) {
		t.Fatal("active rehire", err)
	}
	inactive, _ := e.Transition(scope, 1, EmployeeInactive, EmployeeConditions{OwnershipProtected: true, ReasonRecorded: true})
	if _, err := PlanRehire(scope, inactive, 2, 0, "Synthetic rehire"); !errors.Is(err, ErrTransition) {
		t.Fatal("inactive rehire", err)
	}
	merged, _ := RehydrateEmployee(scope, EmployeeSnapshot{OrgID: scope.OrgID(), ID: p.EmployeeID, State: EmployeeTerminated, Revision: 2, MergedInto: p.MembershipID})
	if _, err := PlanRehire(scope, merged, 2, 0, "Synthetic rehire"); !errors.Is(err, ErrMerged) {
		t.Fatal("merged rehire", err)
	}
	if _, err := PlanRehire(scope, terminated, 1, 0, "Synthetic rehire"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision", err)
	}
	if _, err := PlanRehire(scope, terminated, 2, 0, "\n"); !errors.Is(err, ErrInvalid) {
		t.Fatal("unrecordable reason", err)
	}
}
