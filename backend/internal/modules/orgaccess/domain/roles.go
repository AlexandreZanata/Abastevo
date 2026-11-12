package domain

import (
	"errors"
	"slices"
)

var (
	ErrEscalation                = ErrPermissionDenied
	ErrSelfAssignment            = ErrPermissionDenied
	ErrCannotAssignOwner         = errors.New("orgaccess: only owner can assign owner role")
	ErrSelfModificationForbidden  = errors.New("orgaccess: cannot modify own role assignments")
	ErrSelfAssignmentForbidden   = errors.New("orgaccess: cannot assign role to oneself")
)

// CanAssignRole evaluates BR-AUTH-007 (No escalation) and Footnote 1:
// 1. A member assigns only roles within effective scope.
// 2. Only an OWNER assigns OWNER (ADMIN cannot assign OWNER).
// 3. Station roles cannot be assigned in F1.
func CanAssignRole(catalog Catalog, actorRoles []Role, requestedRole Role) bool {
	if !knownRole(requestedRole) {
		return false
	}
	// Station roles are unavailable in F1.
	if requestedRole == StationAdmin || requestedRole == StationManager || requestedRole == StationAttendant {
		return false
	}
	// Actor must hold at least one role with auth.role.assign (Owner or Admin).
	hasAssignPerm := false
	for _, role := range actorRoles {
		rule, err := catalog.Lookup(role, "auth.role.assign")
		if err == nil && rule.Coverage() != Denied {
			hasAssignPerm = true
			break
		}
	}
	if !hasAssignPerm {
		return false
	}

	// BR-AUTH-007 & Footnote 1: Only an OWNER assigns OWNER.
	isOwner := slices.Contains(actorRoles, Owner)
	if requestedRole == Owner {
		return isOwner
	}
	return true
}

// CanModifyAssignment evaluates BR-AUTH-007 on self-modification:
// "Nobody changes their own assignments except an OWNER reducing their own, subject to BR-AUTH-006."
func CanModifyAssignment(actorMemberID, targetMemberID string, actorRoles []Role, isReduction bool) bool {
	if actorMemberID != targetMemberID {
		return true
	}
	// Self modification: only an OWNER reducing own assignment is allowed.
	return slices.Contains(actorRoles, Owner) && isReduction
}
