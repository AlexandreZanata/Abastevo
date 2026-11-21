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
	if e != nil || !slices.Contains(p, d.Permission("idn.employee.read")) || !slices.Contains(p, d.Permission("idn.invitation.manage")) || !slices.Contains(p, d.Permission("subs.plan.manage")) || !slices.Contains(p, d.Permission("fleet.asset.read")) || !slices.Contains(g, d.ContextGrant{Role: d.Owner, Scope: d.OrgScope}) {
		t.Fatal("current owner hints", p, g, e)
	}
	p, g, e = d.ContextHints(s, member, owner, now, false)
	if e != nil || !slices.Contains(p, d.Permission("idn.employee.read")) || !slices.Contains(p, d.Permission("subs.plan.read")) || !slices.Contains(p, d.Permission("fleet.asset.read")) || slices.Contains(p, d.Permission("subs.plan.manage")) || slices.Contains(p, d.Permission("idn.employee.manage")) || slices.Contains(p, d.Permission("idn.invitation.manage")) || len(g) != 0 {
		t.Fatal("stale proof granted sensitive controls", p, g, e)
	}
	for _, assignments := range [][]d.Assignment{{role(d.Owner, d.OrgScope, now)}, {role(d.Admin, d.SelfScope, time.Time{})}, {role(d.Driver, d.SelfScope, time.Time{})}} {
		p, g, e = d.ContextHints(s, member, assignments, now, true)
		if e != nil || len(g) != 0 || slices.Contains(p, d.Permission("subs.plan.read")) || slices.Contains(p, d.Permission("subs.plan.manage")) || slices.Contains(p, d.Permission("fleet.asset.read")) || slices.Contains(p, d.Permission("idn.employee.manage")) || slices.Contains(p, d.Permission("idn.invitation.manage")) {
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

// EMP-CHECK-004 (B-BR-EMP-507, EDR-329): the document-content hint appears
// only when a current ORG assignment covers doc.document.view_content on the
// organization, whatever the proof freshness; structural, SELF, expired and
// keyless roles give none. The hint is a read hint, never a write control.
func TestContextHintsOfferDocumentContentOnlyWithOrganizationCoverage(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000302")
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	member := "7b0e2a54-0000-4000-8000-000000000602"
	unit := "7b0e2a54-0000-4000-8000-000000000802"
	role := func(kind d.Role, scope d.ScopeType, until time.Time) d.Assignment {
		target := s.OrgID().String()
		switch scope {
		case d.SelfScope:
			target = member
		case d.UnitScope:
			target = unit
		}
		a, e := d.RehydrateAssignment(s, d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000702", MembershipID: member, AssignedBy: member, Role: kind, Scope: scope, TargetID: target, ValidFrom: now.Add(-time.Hour), ValidTo: until, Revision: 1, Reason: "synthetic content hint"})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	const content = d.Permission("doc.document.view_content")
	for name, a := range map[string]d.Assignment{"owner": role(d.Owner, d.OrgScope, time.Time{}), "fleet manager": role(d.FleetManager, d.OrgScope, time.Time{}),
		"finance": role(d.Finance, d.OrgScope, time.Time{}), "auditor": role(d.Auditor, d.OrgScope, time.Time{})} {
		for _, fresh := range []bool{true, false} {
			p, _, e := d.ContextHints(s, member, []d.Assignment{a}, now, fresh)
			if e != nil || !slices.Contains(p, content) {
				t.Fatal("ORG content holder without the hint", name, fresh, p, e)
			}
			if !fresh && (slices.Contains(p, d.Permission("idn.employee.manage")) || slices.Contains(p, d.Permission("idn.invitation.manage"))) {
				t.Fatal("the content hint opened a write control", name, p)
			}
		}
	}
	for name, a := range map[string]d.Assignment{"supervisor": role(d.Supervisor, d.OrgScope, time.Time{}), "unit fleet manager": role(d.FleetManager, d.UnitScope, time.Time{}),
		"self driver": role(d.Driver, d.SelfScope, time.Time{}), "expired owner": role(d.Owner, d.OrgScope, now)} {
		p, _, e := d.ContextHints(s, member, []d.Assignment{a}, now, true)
		if e != nil || slices.Contains(p, content) {
			t.Fatal("content hint without ORG coverage", name, p, e)
		}
	}
	if p, _, e := d.ContextHints(s, member, nil, now, true); e != nil || len(p) != 0 {
		t.Fatal("hints without any assignment", p, e)
	}
}
