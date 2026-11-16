package http

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
)

const (
	recordsOrg    = "a7410000-0000-4000-8000-000000000001"
	recordsMember = "a7410000-0000-4000-8000-000000000004"
	recordsOther  = "a7410000-0000-4000-8000-000000000009"
)

type recordsSource struct{ value access.Authority }

func (s recordsSource) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return s.value, nil
}

type recordsCapability struct{}

func (recordsCapability) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type recordsAllow struct{}

func (recordsAllow) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

type recordsProof struct{}

func (recordsProof) Confirm(context.Context, access.Subject, access.Policy) error { return nil }

// recordsSubject is the subject of a completed SELF-scoped DRIVER decision.
func recordsSubject(t *testing.T) (kernel.OrgScope, access.Subject) {
	t.Helper()
	org, err := kernel.ParseOrgID(recordsOrg)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	role, err := permissions.RehydrateAssignment(s, permissions.AssignmentSnapshot{OrgID: s.OrgID(), ID: "a7410000-0000-4000-8000-000000000007", MembershipID: recordsMember, AssignedBy: recordsMember, Role: permissions.Driver, Scope: permissions.SelfScope, TargetID: recordsMember, ValidFrom: now.Add(-time.Hour), Reason: "Synthetic driver fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: recordsSource{access.Authority{OrgID: s.OrgID(), AccountID: "a7410000-0000-4000-8000-000000000002", MembershipID: recordsMember, EmployeeID: "a7410000-0000-4000-8000-000000000005", MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: recordsCapability{}, Records: recordsAllow{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	t.Cleanup(finish)
	policy := access.Policy{Permission: "trip.trip.start", Capability: "organizations", Effect: access.Read}
	prepared, err := a.Prepare(ctx, s, access.Actor{AccountID: "a7410000-0000-4000-8000-000000000002", Channel: "web"}, policy, policyOwn{})
	if err != nil {
		t.Fatal("own decision", err)
	}
	d, err := a.Complete(ctx, prepared, recordsProof{})
	if err != nil {
		t.Fatal(err)
	}
	return s, d.Subject()
}

// EMP-GPS-002: the own read resolves the requesting membership itself and
// the records check admits the own policy only over that record, never over
// the organization or another member; the settings policies keep their
// organization target.
func TestPolicyRecordsAdmitTheOwnReadOnlyOverTheOwnRecord(t *testing.T) {
	s, subject := recordsSubject(t)
	facts, err := policyOwn{}.Resolve(t.Context(), s, subject)
	if err != nil || facts.OrgID != s.OrgID() || facts.ID != recordsMember || facts.AssignedTo != recordsMember || facts.AuthoredBy != "" || facts.TripDriver != "" {
		t.Fatal("own resolver", err, facts.ID)
	}
	own, read, manage := app.PolicySettingsPolicy(app.PolicyOwn), app.PolicySettingsPolicy(app.PolicyRead), app.PolicySettingsPolicy(app.PolicyManage)
	rules := []permissions.Rule{{}}
	root := permissions.ResourceFacts{OrgID: s.OrgID(), ID: recordsOrg}
	check := func(policy access.Policy, f permissions.ResourceFacts, r []permissions.Rule) error {
		return PolicyRecords{}.Check(t.Context(), s, subject, policy, f, r)
	}
	if check(own, facts, rules) != nil {
		t.Fatal("the own read over the own record was refused")
	}
	for name, c := range map[string]struct {
		policy access.Policy
		facts  permissions.ResourceFacts
		rules  []permissions.Rule
	}{
		"own over the organization":    {own, root, rules},
		"own over another member":      {own, permissions.ResourceFacts{OrgID: s.OrgID(), ID: recordsOther, AssignedTo: recordsOther}, rules},
		"own without the assignment":   {own, permissions.ResourceFacts{OrgID: s.OrgID(), ID: recordsMember}, rules},
		"own assigned to another":      {own, permissions.ResourceFacts{OrgID: s.OrgID(), ID: recordsMember, AssignedTo: recordsOther}, rules},
		"own without covering rules":   {own, facts, nil},
		"read over the own record":     {read, facts, rules},
		"manage over the own record":   {manage, facts, rules},
		"trip.trip.start as a write":   {access.Policy{Permission: "trip.trip.start", Capability: "organizations", Effect: access.OperationalWrite}, facts, rules},
		"another permission, own fact": {access.Policy{Permission: "trip.trip.read", Capability: "organizations", Effect: access.Read}, facts, rules},
	} {
		if err := check(c.policy, c.facts, c.rules); !errors.Is(err, access.ErrUnavailable) {
			t.Fatal("records admitted", name, err)
		}
	}
	if check(read, root, rules) != nil || check(manage, root, rules) != nil {
		t.Fatal("the settings policies lost their organization target")
	}
}
