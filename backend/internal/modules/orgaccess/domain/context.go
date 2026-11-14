package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"time"
)

// ContextGrant and ContextHints describe current controls, not authorization.
// Every owning command independently repeats its proof and current source gates.
type ContextGrant struct {
	Role  Role      `json:"role"`
	Scope ScopeType `json:"scope"`
}

func ContextHints(scope kernel.OrgScope, member string, assignments []Assignment, now time.Time, fresh bool) ([]Permission, []ContextGrant, error) {
	permissions := make([]Permission, 0, 3)
	grants := make([]ContextGrant, 0)
	if scope.Validate() != nil || now.IsZero() {
		return nil, nil, ErrAssignmentInvalid
	}
	if _, e := kernel.ParseOrgID(member); e != nil {
		return nil, nil, ErrAssignmentInvalid
	}
	facts := ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}
	for _, permission := range []Permission{"idn.employee.read", "idn.employee.manage", "idn.invitation.manage"} {
		if permission != "idn.employee.read" && !fresh {
			continue
		}
		if _, e := CoveringRules(scope, member, assignments, permission, facts, now); e == nil {
			permissions = append(permissions, permission)
		}
	}
	if !fresh {
		return permissions, grants, nil
	}
	canInvite := false
	for _, p := range permissions {
		if p == "idn.invitation.manage" {
			canInvite = true
		}
	}
	if !canInvite {
		return permissions, grants, nil
	}
	roles := make([]Role, 0, len(assignments))
	for _, a := range assignments {
		v, e := a.Snapshot(scope)
		if e != nil {
			return nil, nil, e
		}
		valid, e := a.ValidAt(scope, now)
		if e != nil {
			return nil, nil, e
		}
		if valid && v.MembershipID == member && v.Scope == OrgScope {
			roles = append(roles, v.Role)
		}
	}
	for _, info := range SystemCatalog().Roles() {
		for _, kind := range []ScopeType{OrgScope, SelfScope} {
			if CanAssignScopedRole(SystemCatalog(), roles, info.Key, kind) {
				grants = append(grants, ContextGrant{Role: info.Key, Scope: kind})
			}
		}
	}
	return permissions, grants, nil
}
