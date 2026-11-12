package domain

import "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

type EmployeeState string

const (
	EmployeeActive     EmployeeState = "ACTIVE"
	EmployeeInactive   EmployeeState = "INACTIVE"
	EmployeeTerminated EmployeeState = "TERMINATED"
)

type EmployeeSnapshot struct {
	privateValue
	OrgID      kernel.OrgID
	ID         string
	State      EmployeeState
	Revision   int64
	MergedInto string
}
type Employee struct {
	privateValue
	value EmployeeSnapshot
}

func NewEmployee(scope kernel.OrgScope, id string) (Employee, error) {
	return RehydrateEmployee(scope, EmployeeSnapshot{OrgID: scope.OrgID(), ID: id, State: EmployeeActive, Revision: 1})
}

// RehydrateEmployee is trusted-storage-only, never a client command decoder.
func RehydrateEmployee(scope kernel.OrgScope, snapshot EmployeeSnapshot) (Employee, error) {
	if scope.Validate() != nil || snapshot.OrgID != scope.OrgID() {
		return Employee{}, ErrScope
	}
	id, err := canonicalID(snapshot.ID)
	if err != nil || snapshot.Revision < 1 {
		return Employee{}, ErrInvalid
	}
	snapshot.ID = id
	switch snapshot.State {
	case EmployeeActive, EmployeeInactive, EmployeeTerminated:
	default:
		return Employee{}, ErrInvalid
	}
	if snapshot.MergedInto != "" {
		target, err := canonicalID(snapshot.MergedInto)
		if err != nil || target == id || snapshot.State != EmployeeTerminated {
			return Employee{}, ErrInvalid
		}
		snapshot.MergedInto = target
	}
	return Employee{value: snapshot}, nil
}
func (e Employee) Snapshot(scope kernel.OrgScope) (EmployeeSnapshot, error) {
	if err := scoped(scope, e.value.OrgID, e.value.ID, e.value.Revision); err != nil {
		return EmployeeSnapshot{}, err
	}
	return e.value, nil
}

// Conditions describe facts checked by the owning command in its transaction.
// They do not perform the access effects or prove the actor has permission.
type EmployeeConditions struct{ OwnershipProtected, ResponsibilitiesReviewed, ReasonRecorded bool }

func (e Employee) Transition(scope kernel.OrgScope, expected int64, to EmployeeState, facts EmployeeConditions) (Employee, error) {
	v := e.value
	if err := changing(scope, v.OrgID, v.ID, v.Revision, expected); err != nil {
		return Employee{}, err
	}
	switch [2]EmployeeState{v.State, to} {
	case [2]EmployeeState{EmployeeActive, EmployeeInactive}:
		if !facts.OwnershipProtected || !facts.ReasonRecorded {
			return Employee{}, ErrConditions
		}
	case [2]EmployeeState{EmployeeActive, EmployeeTerminated}, [2]EmployeeState{EmployeeInactive, EmployeeTerminated}:
		if !facts.OwnershipProtected || !facts.ResponsibilitiesReviewed {
			return Employee{}, ErrConditions
		}
	case [2]EmployeeState{EmployeeInactive, EmployeeActive}:
	case [2]EmployeeState{EmployeeTerminated, EmployeeActive}:
		if v.MergedInto != "" {
			return Employee{}, ErrMerged
		}
	default:
		return Employee{}, ErrTransition
	}
	v.State = to
	v.Revision++
	return Employee{value: v}, nil
}
