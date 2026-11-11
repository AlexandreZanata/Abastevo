package domain

import "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

type MembershipState string

const (
	MembershipInvited   MembershipState = "INVITED"
	MembershipActive    MembershipState = "ACTIVE"
	MembershipSuspended MembershipState = "SUSPENDED"
	MembershipEnded     MembershipState = "ENDED"
)

type SuspensionOrigin string

const (
	ByManager            SuspensionOrigin = "MANAGER"
	ByEmployeeInactivity SuspensionOrigin = "EMPLOYEE_INACTIVE"
)

func validSuspension(c SuspensionOrigin) bool { return c == ByManager || c == ByEmployeeInactivity }

type EndReason string

const (
	Offboarded         EndReason = "OFFBOARDED"
	Left               EndReason = "LEFT"
	AccountDeleted     EndReason = "ACCOUNT_DELETED"
	OrganizationClosed EndReason = "ORGANIZATION_CLOSED"
	AdmissionEnded     EndReason = "ADMISSION_ENDED"
)

func validEnd(c EndReason) bool {
	switch c {
	case Offboarded, Left, AccountDeleted, OrganizationClosed, AdmissionEnded:
		return true
	}
	return false
}

type MembershipSnapshot struct {
	privateValue
	OrgID          kernel.OrgID
	ID, EmployeeID string
	Account        AccountRef
	Model          AccessModel
	State          MembershipState
	Revision       int64
	Suspension     SuspensionOrigin
	EndReason      EndReason
}
type Membership struct {
	privateValue
	value MembershipSnapshot
}

// ValidateLiveMemberships checks BR-IDN-004 within a supplied, scoped set.
// Empty account references on INVITED periods do not collide. This does not
// discover other rows or replace the persistence task's concurrency constraints.
func ValidateLiveMemberships(scope kernel.OrgScope, values []Membership) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	employees, accounts := map[string]bool{}, map[string]bool{}
	for _, value := range values {
		snapshot, err := value.Snapshot(scope)
		if err != nil {
			return err
		}
		if snapshot.State == MembershipEnded {
			continue
		}
		if employees[snapshot.EmployeeID] {
			return ErrConflict
		}
		employees[snapshot.EmployeeID] = true
		if snapshot.Account.id != "" {
			if accounts[snapshot.Account.id] {
				return ErrConflict
			}
			accounts[snapshot.Account.id] = true
		}
	}
	return nil
}

func NewInvited(scope kernel.OrgScope, id string, employee Employee, model AccessModel) (Membership, error) {
	snap, err := employee.Snapshot(scope)
	if err != nil {
		return Membership{}, err
	}
	if snap.State != EmployeeActive {
		return Membership{}, ErrConditions
	}
	return RehydrateMembership(scope, MembershipSnapshot{OrgID: scope.OrgID(), ID: id, EmployeeID: snap.ID, Model: model, State: MembershipInvited, Revision: 1})
}

// RehydrateMembership accepts storage values only, never an untrusted wire DTO.
func RehydrateMembership(scope kernel.OrgScope, snapshot MembershipSnapshot) (Membership, error) {
	if scope.Validate() != nil || snapshot.OrgID != scope.OrgID() {
		return Membership{}, ErrScope
	}
	id, err := canonicalID(snapshot.ID)
	if err != nil {
		return Membership{}, err
	}
	snapshot.ID = id
	employee, err := canonicalID(snapshot.EmployeeID)
	if err != nil {
		return Membership{}, err
	}
	snapshot.EmployeeID = employee
	if snapshot.Revision < 1 || !validModel(snapshot.Model) {
		return Membership{}, ErrInvalid
	}
	if snapshot.Account != (AccountRef{}) {
		if err := snapshot.Account.check(scope); err != nil {
			return Membership{}, err
		}
		if snapshot.Account.model != snapshot.Model {
			return Membership{}, ErrInvalid
		}
	}
	switch snapshot.State {
	case MembershipInvited:
		if snapshot.Suspension != "" || snapshot.EndReason != "" {
			return Membership{}, ErrInvalid
		}
	case MembershipActive:
		if snapshot.Account == (AccountRef{}) || snapshot.Suspension != "" || snapshot.EndReason != "" {
			return Membership{}, ErrInvalid
		}
	case MembershipSuspended:
		if snapshot.Account == (AccountRef{}) || !validSuspension(snapshot.Suspension) || snapshot.EndReason != "" {
			return Membership{}, ErrInvalid
		}
	case MembershipEnded:
		if !validEnd(snapshot.EndReason) || (snapshot.Suspension != "" && !validSuspension(snapshot.Suspension)) {
			return Membership{}, ErrInvalid
		}
	default:
		return Membership{}, ErrInvalid
	}
	return Membership{value: snapshot}, nil
}
func (m Membership) Snapshot(scope kernel.OrgScope) (MembershipSnapshot, error) {
	if err := scoped(scope, m.value.OrgID, m.value.ID, m.value.Revision); err != nil {
		return MembershipSnapshot{}, err
	}
	return m.value, nil
}
func (m Membership) changing(scope kernel.OrgScope, expected int64) error {
	if err := changing(scope, m.value.OrgID, m.value.ID, m.value.Revision, expected); err != nil {
		return err
	}
	if m.value.State == MembershipEnded {
		return ErrEnded
	}
	return nil
}

// BindAccount records an association only; an awaiting-confirmation period
// stays INVITED. Account kind and organization must match the chosen model.
func (m Membership) BindAccount(scope kernel.OrgScope, expected int64, account AccountRef) (Membership, error) {
	if err := m.changing(scope, expected); err != nil {
		return Membership{}, err
	}
	if m.value.State != MembershipInvited {
		return Membership{}, ErrTransition
	}
	if m.value.Account != (AccountRef{}) {
		return Membership{}, ErrConflict
	}
	if err := account.check(scope); err != nil {
		return Membership{}, err
	}
	if account.model != m.value.Model {
		return Membership{}, ErrInvalid
	}
	result := m
	result.value.Account = account
	result.value.Revision++
	return result, nil
}

// AdmissionConfirmed covers invitation/code acceptance or initial-owner
// creation as resolved by its owning command. No token or actor is verified
// here. LiveRoleAssignments is a checked count, not role data or a grant.
type MembershipConditions struct {
	Employee                                                     Employee
	AdmissionConfirmed, ConfirmationSatisfied                    bool
	LiveRoleAssignments                                          int64
	OwnershipProtected, ResponsibilitiesReviewed, ReasonRecorded bool
	Suspension                                                   SuspensionOrigin
	EmployeeStateChange                                          bool
	EndReason                                                    EndReason
}

func (m Membership) Transition(scope kernel.OrgScope, expected int64, to MembershipState, facts MembershipConditions) (Membership, error) {
	if err := m.changing(scope, expected); err != nil {
		return Membership{}, err
	}
	v := m.value
	switch [2]MembershipState{v.State, to} {
	case [2]MembershipState{MembershipInvited, MembershipActive}, [2]MembershipState{MembershipSuspended, MembershipActive}:
		employee, err := facts.Employee.Snapshot(scope)
		if err != nil {
			return Membership{}, err
		}
		if employee.ID != v.EmployeeID {
			return Membership{}, ErrScope
		}
		if employee.State != EmployeeActive {
			return Membership{}, ErrConditions
		}
		if v.State == MembershipInvited {
			if v.Account == (AccountRef{}) || !facts.AdmissionConfirmed || !facts.ConfirmationSatisfied || facts.LiveRoleAssignments < 1 {
				return Membership{}, ErrConditions
			}
		} else if facts.EmployeeStateChange && v.Suspension != ByEmployeeInactivity {
			return Membership{}, ErrConditions
		}
		v.Suspension = ""
	case [2]MembershipState{MembershipActive, MembershipSuspended}:
		if !facts.OwnershipProtected || !facts.ReasonRecorded || !validSuspension(facts.Suspension) {
			return Membership{}, ErrConditions
		}
		if facts.Suspension == ByEmployeeInactivity {
			employee, err := facts.Employee.Snapshot(scope)
			if err != nil {
				return Membership{}, err
			}
			if employee.ID != v.EmployeeID {
				return Membership{}, ErrScope
			}
			if employee.State != EmployeeInactive {
				return Membership{}, ErrConditions
			}
		}
		v.Suspension = facts.Suspension
	case [2]MembershipState{MembershipInvited, MembershipEnded}:
		if !validEnd(facts.EndReason) {
			return Membership{}, ErrConditions
		}
		v.EndReason = facts.EndReason
	case [2]MembershipState{MembershipActive, MembershipEnded}, [2]MembershipState{MembershipSuspended, MembershipEnded}:
		if !facts.OwnershipProtected || !facts.ResponsibilitiesReviewed || !validEnd(facts.EndReason) || facts.EndReason == AdmissionEnded {
			return Membership{}, ErrConditions
		}
		v.EndReason = facts.EndReason
	default:
		return Membership{}, ErrTransition
	}
	v.State = to
	v.Revision++
	return Membership{value: v}, nil
}
