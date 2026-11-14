package domain

import (
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ManagerAccessAction string

const (
	ManagerSuspend    ManagerAccessAction = "SUSPEND"
	ManagerReactivate ManagerAccessAction = "REACTIVATE"
)

var ErrSelfSuspension = errors.New("membership: self suspension forbidden")

func ValidateAccessChangeReason(reason string) error {
	if !utf8.ValidString(reason) || strings.TrimSpace(reason) == "" || utf8.RuneCountInString(reason) > 280 {
		return ErrInvalid
	}
	for _, r := range reason {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ErrInvalid
		}
	}
	return nil
}
func PlanManagerAccessChange(scope kernel.OrgScope, m Membership, actorMember string, expected int64, action ManagerAccessAction, reason string, employee Employee, liveAssignments int64, ownershipAllowed bool) (Membership, error) {
	if e := ValidateAccessChangeReason(reason); e != nil {
		return Membership{}, e
	}
	v, e := m.Snapshot(scope)
	if e != nil {
		return Membership{}, e
	}
	actor, e := canonicalID(actorMember)
	if e != nil || actor != actorMember {
		return Membership{}, ErrInvalid
	}
	switch action {
	case ManagerSuspend:
		if v.ID == actorMember {
			return Membership{}, ErrSelfSuspension
		}
		return m.Transition(scope, expected, MembershipSuspended, MembershipConditions{Employee: employee, OwnershipProtected: ownershipAllowed, ReasonRecorded: true, Suspension: ByManager})
	case ManagerReactivate:
		if liveAssignments < 1 || !ownershipAllowed {
			return Membership{}, ErrConditions
		}
		return m.Transition(scope, expected, MembershipActive, MembershipConditions{Employee: employee, LiveRoleAssignments: liveAssignments})
	default:
		return Membership{}, ErrInvalid
	}
}

// AccessState is immutable acknowledgement metadata, never membership authority
// or a substitute for account binding. No account/tombstone identifier is exposed.
type AccessState struct {
	privateValue
	OrgID          kernel.OrgID
	ID, EmployeeID string
	Model          AccessModel
	State          MembershipState
	Revision       int64
	Suspension     SuspensionOrigin
	EndReason      EndReason
}

func AccessStateOf(scope kernel.OrgScope, m Membership) (AccessState, error) {
	v, e := m.Snapshot(scope)
	if e != nil {
		return AccessState{}, e
	}
	out := AccessState{OrgID: v.OrgID, ID: v.ID, EmployeeID: v.EmployeeID, Model: v.Model, State: v.State, Revision: v.Revision, Suspension: v.Suspension, EndReason: v.EndReason}
	return out, out.Validate(scope)
}
func (v AccessState) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return ErrScope
	}
	for _, id := range []string{v.ID, v.EmployeeID} {
		p, e := canonicalID(id)
		if e != nil || p != id {
			return ErrInvalid
		}
	}
	if !validModel(v.Model) || v.Revision < 1 {
		return ErrInvalid
	}
	switch v.State {
	case MembershipInvited, MembershipActive:
		if v.Suspension != "" || v.EndReason != "" {
			return ErrInvalid
		}
	case MembershipSuspended:
		if !validSuspension(v.Suspension) || v.EndReason != "" {
			return ErrInvalid
		}
	case MembershipEnded:
		if !validEnd(v.EndReason) || (v.Suspension != "" && !validSuspension(v.Suspension)) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
