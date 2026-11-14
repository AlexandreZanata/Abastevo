package domain_test

import (
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"slices"
	"testing"
	"time"
)

func TestContextHintsRetainCurrentScopeFreshnessAndGrantContainment(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	member := "7b0e2a54-0000-4000-8000-000000000601"
	role := func(kind d.Role, scope d.ScopeType, until time.Time) d.Assignment {
		target := s.OrgID().String()
		if scope == d.SelfScope {
			target = member
		}
		a, e := d.RehydrateAssignment(s, d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000701", MembershipID: member, AssignedBy: member, Role: kind, Scope: scope, TargetID: target, ValidFrom: now.Add(-time.Hour), ValidTo: until, Revision: 1, Reason: "synthetic context"})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	owner := []d.Assignment{role(d.Owner, d.OrgScope, time.Time{})}
	p, g, e := d.ContextHints(s, member, owner, now, true)
	if e != nil || !slices.Contains(p, d.Permission("idn.employee.read")) || !slices.Contains(p, d.Permission("idn.invitation.manage")) || !slices.Contains(g, d.ContextGrant{Role: d.Owner, Scope: d.OrgScope}) {
		t.Fatal("current owner hints", p, g, e)
	}
	p, g, e = d.ContextHints(s, member, owner, now, false)
	if e != nil || !slices.Contains(p, d.Permission("idn.employee.read")) || slices.Contains(p, d.Permission("idn.employee.manage")) || slices.Contains(p, d.Permission("idn.invitation.manage")) || len(g) != 0 {
		t.Fatal("stale proof granted sensitive controls", p, g, e)
	}
	for _, assignments := range [][]d.Assignment{{role(d.Owner, d.OrgScope, now)}, {role(d.Admin, d.SelfScope, time.Time{})}, {role(d.Driver, d.SelfScope, time.Time{})}} {
		p, g, e = d.ContextHints(s, member, assignments, now, true)
		if e != nil || len(g) != 0 || slices.Contains(p, d.Permission("idn.employee.manage")) || slices.Contains(p, d.Permission("idn.invitation.manage")) {
			t.Fatal("invalid scope or period granted controls", p, g, e)
		}
	}
	_, g, e = d.ContextHints(s, member, []d.Assignment{role(d.Admin, d.OrgScope, time.Time{})}, now, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, grant := range g {
		if grant.Role == d.Owner || !d.CanAssignScopedRole(d.SystemCatalog(), []d.Role{d.Admin}, grant.Role, grant.Scope) {
			t.Fatal("grant escalation", grant)
		}
	}
}
