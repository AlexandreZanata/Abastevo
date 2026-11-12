package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"strings"
	"testing"
	"time"
)

func invID(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

type invitationAuthority struct{ value access.Authority }

func (a invitationAuthority) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return a.value, nil
}

type invitationGates struct{}

func (invitationGates) Confirm(context.Context, access.Subject, access.Policy) error { return nil }
func (invitationGates) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type invitationRecordGate struct{}

func (invitationRecordGate) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

type invitationRoot struct{}

func (invitationRoot) Resolve(_ context.Context, s kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: s.OrgID().String()}, nil
}
func invitationDecision(t *testing.T, action app.InvitationAction) (context.Context, kernel.OrgScope, app.EmployeeActor, access.Decision, func()) {
	t.Helper()
	oid, e := kernel.ParseOrgID(invID(301))
	if e != nil {
		t.Fatal(e)
	}
	scope, e := kernel.NewOrgScope(oid)
	if e != nil {
		t.Fatal(e)
	}
	a := app.EmployeeActor{AccountID: invID(701), SessionID: invID(702)}
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	role, e := permissions.RehydrateAssignment(scope, permissions.AssignmentSnapshot{OrgID: oid, ID: invID(801), MembershipID: invID(601), AssignedBy: invID(601), Role: permissions.Owner, Scope: permissions.OrgScope, TargetID: oid.String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic owner", Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	auth := access.Authorization{Source: invitationAuthority{access.Authority{OrgID: oid, AccountID: a.AccountID, MembershipID: invID(601), EmployeeID: invID(501), MembershipState: d.MembershipActive, EmployeeState: d.EmployeeActive, Model: d.ExistingAccount, OrganizationState: org.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: invitationGates{}, Records: invitationRecordGate{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	prepared, e := auth.Prepare(ctx, scope, access.Actor{AccountID: a.AccountID, Channel: "web"}, app.InvitationPolicy(action), invitationRoot{})
	if e != nil {
		finish()
		t.Fatal(e)
	}
	decision, e := auth.Complete(ctx, prepared, invitationGates{})
	if e != nil {
		finish()
		t.Fatal(e)
	}
	t.Cleanup(finish)
	return ctx, scope, a, decision, finish
}

type invitationRepo struct {
	calls  int
	value  d.Invitation
	replay bool
	err    error
	plan   app.InvitationPlan
}

func (r *invitationRepo) WriteInvitation(_ context.Context, _ kernel.OrgScope, _ app.EmployeeActor, _ string, _ app.InvitationWrite, p app.InvitationPlan) (d.Invitation, bool, error) {
	r.calls++
	r.plan = p
	return r.value, r.replay, r.err
}
func (r *invitationRepo) ListInvitations(context.Context, kernel.OrgScope, app.EmployeeActor, string, app.InvitationFilter) (app.InvitationPage, error) {
	r.calls++
	return app.InvitationPage{Invitations: []d.Invitation{r.value}}, r.err
}
func invitationValue(t *testing.T, scope kernel.OrgScope) d.Invitation {
	t.Helper()
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	v, e := d.RehydrateInvitation(scope, d.InvitationSnapshot{OrgID: scope.OrgID(), ID: invID(901), EmployeeID: invID(501), MembershipID: invID(602), IssuerMembershipID: invID(601), OperationID: invID(1001), State: d.InvitationPending, Revision: 1, SettingsRevision: 1, ValidityDays: 7, CreatedAt: now, ChangedAt: now, ExpiresAt: now.Add(168 * time.Hour), Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestInvitationServiceRequiresLiveRequestDecision(t *testing.T) {
	for _, mode := range []string{"empty", "foreign account", "other request", "finished", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, scope, a, decision, finish := invitationDecision(t, app.InvitationIssue)
			repo := &invitationRepo{}
			s := app.InvitationService{Enabled: true, Repository: repo, NewID: func() (string, error) { t.Fatal("invalid authority generated credential metadata"); return "", nil }}
			want := app.ErrEmployeeUnauthenticated
			switch mode {
			case "empty":
				decision = access.Decision{}
			case "foreign account":
				a.AccountID = invID(703)
			case "other request":
				ctx, finish = access.BeginRequest(t.Context())
				defer finish()
			case "finished":
				finish()
			case "disabled":
				s.Enabled = false
				want = app.ErrRegisterDisabled
			}
			if result, e := s.Write(ctx, scope, a, decision, app.InvitationWrite{Action: app.InvitationIssue, OperationID: invID(1001), EmployeeID: invID(501), Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}); !errors.Is(e, want) || result.Code != "" || repo.calls != 0 {
				t.Fatal("unverified command", e)
			}
		})
	}
	ctx, scope, a, decision, finish := invitationDecision(t, app.InvitationRead)
	finish()
	r := &invitationRepo{}
	if _, e := (app.InvitationService{Enabled: true, Repository: r}).List(ctx, scope, a, decision, app.InvitationFilter{Limit: 1}); !errors.Is(e, app.ErrEmployeeUnauthenticated) || r.calls != 0 {
		t.Fatal("expired read lease")
	}
}
func TestInvitationServiceDeliveryOnlyAfterValidNewCommit(t *testing.T) {
	for _, mode := range []string{"new", "replay", "failure", "corrupt receipt"} {
		t.Run(mode, func(t *testing.T) {
			ctx, scope, a, decision, _ := invitationDecision(t, app.InvitationIssue)
			repo := &invitationRepo{value: invitationValue(t, scope), replay: mode == "replay"}
			if mode == "failure" {
				repo.err = app.ErrRegisterUnavailable
			}
			if mode == "corrupt receipt" {
				repo.value = d.Invitation{}
			}
			s := app.InvitationService{Enabled: true, Repository: repo, NewID: func() (string, error) { return invID(901), nil }, NewSecret: func(kernel.OrgScope, string) (app.InvitationSecret, error) {
				return app.InvitationSecret{Code: "synthetic-private-once", Verifier: strings.Repeat("a", 64)}, nil
			}}
			result, e := s.Write(ctx, scope, a, decision, app.InvitationWrite{Action: app.InvitationIssue, OperationID: invID(1001), EmployeeID: invID(501), Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}})
			if mode == "failure" || mode == "corrupt receipt" {
				if !errors.Is(e, app.ErrRegisterUnavailable) || result.Code != "" {
					t.Fatal("provisional secret", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if (result.Code != "") != (mode == "new") {
				t.Fatal("secret redisplayed")
			}
			if repo.plan.Verifier == "" {
				t.Fatal("no private verifier")
			}
			raw, e := json.Marshal(result)
			if e != nil {
				t.Fatal(e)
			}
			for _, render := range []string{string(raw), fmt.Sprintf("%+v", result), fmt.Sprintf("%#v", repo.plan)} {
				if strings.Contains(render, "synthetic-private-once") || strings.Contains(render, strings.Repeat("a", 64)) {
					t.Fatal("generic secret formatting")
				}
			}
		})
	}
}
func TestInvitationCanonicalCommandAndFilterBoundaries(t *testing.T) {
	_, scope, _, _, _ := invitationDecision(t, app.InvitationIssue)
	c := app.InvitationWrite{Action: app.InvitationIssue, OperationID: invID(1001), EmployeeID: invID(501), Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}, {Role: "ADMIN", Scope: "SELF"}}}
	a := c.Digest(scope)
	c.Grants[0], c.Grants[1] = c.Grants[1], c.Grants[0]
	if a == "" || a != c.Digest(scope) {
		t.Fatal("role-set ordering changed digest")
	}
	c.EmployeeID = invID(502)
	if a == c.Digest(scope) {
		t.Fatal("target omitted from digest")
	}
	for _, invalid := range []app.InvitationWrite{{Action: app.InvitationIssue}, {Action: app.InvitationRevoke, OperationID: invID(1001), InvitationID: invID(901)}, {Action: app.InvitationResend, OperationID: invID(1001), InvitationID: invID(901), ExpectedRevision: 1, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}} {
		if _, e := invalid.Canonical(); e == nil {
			t.Fatal("ambiguous command")
		}
	}
	for _, f := range []app.InvitationFilter{{Limit: 0}, {Limit: 101}, {Limit: 1, ID: "malformed"}, {Limit: 1, State: "UNKNOWN"}, {Limit: 1, ID: invID(901), AfterID: invID(902)}} {
		if f.Validate() == nil {
			t.Fatal("invalid selector")
		}
	}
	p := app.InvitationPolicy(app.InvitationIssue)
	p.Fresh = false
	if p.Validate() == nil {
		t.Fatal("invitation fresh metadata optional")
	}
}
