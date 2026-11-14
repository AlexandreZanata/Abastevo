package domain

import (
	"errors"
	"slices"
)

var (
	ErrEscalation                = ErrPermissionDenied
	ErrSelfAssignment            = ErrPermissionDenied
	ErrCannotAssignOwner         = errors.New("orgaccess: only owner can assign owner role")
	ErrSelfModificationForbidden = errors.New("orgaccess: cannot modify own role assignments")
	ErrSelfAssignmentForbidden   = errors.New("orgaccess: cannot assign role to oneself")
)

// CanAssignRole evaluates BR-AUTH-007 (No escalation) and Footnote 1:
// 1. A member assigns only roles within effective scope.
// 2. Only an OWNER assigns OWNER (ADMIN cannot assign OWNER).
// 3. Station roles cannot be assigned in F1.
func CanAssignRole(catalog Catalog, actorRoles []Role, requestedRole Role) bool {
	return CanAssignScopedRole(catalog, actorRoles, requestedRole, OrgScope)
}

// CanAssignScopedRole compares the granted role's effective permissions with
// the grantor's current organization-scoped role union. SELF narrows the new
// assignment to own records; it never widens a grantor's descriptor.
func CanAssignScopedRole(catalog Catalog, actorRoles []Role, requestedRole Role, scope ScopeType) bool {
	if scope != OrgScope && scope != SelfScope || !knownRole(requestedRole) || stationRole(requestedRole) || requestedRole == Owner && scope != OrgScope {
		return false
	}
	hasAssignPerm := false
	for _, role := range actorRoles {
		rule, err := catalog.Lookup(role, "auth.role.assign")
		if err == nil && rule.Coverage() != Denied {
			hasAssignPerm = true
			break
		}
	}
	if !hasAssignPerm || requestedRole == Owner && !slices.Contains(actorRoles, Owner) {
		return false
	}
	return PermissionsWithin(catalog, actorRoles, requestedRole, scope == SelfScope)
}

// PermissionsWithin compares fixed permission descriptors independently from
// permission to grant roles. It also serves OWNER self-reductions, where the
// assignment being reduced need not itself contain auth.role.assign.
func PermissionsWithin(catalog Catalog, actorRoles []Role, requestedRole Role, ownRecordsOnly bool) bool {
	requested, err := catalog.Rules(requestedRole)
	if err != nil {
		return false
	}
	for _, needed := range requested {
		if needed.Coverage() == Denied {
			continue
		}
		coverage := needed.Coverage()
		if ownRecordsOnly {
			coverage = OwnRecords
		}
		covered := false
		for _, role := range actorRoles {
			held, err := catalog.Lookup(role, needed.Permission())
			if err != nil || held.Coverage() == Denied || held.Coverage() == OwnRecords && coverage == AssignedScope {
				continue
			}
			restrictionsFit := true
			for _, r := range held.Requirements() {
				if !slices.Contains(needed.Requirements(), r) {
					restrictionsFit = false
					break
				}
			}
			if restrictionsFit {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
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
