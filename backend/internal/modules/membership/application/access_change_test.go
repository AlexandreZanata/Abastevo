package application_test

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"testing"
	"time"
)

func accessChangeDecision(t *testing.T, action app.MembershipAccessAction) (context.Context, kernel.OrgScope, app.EmployeeActor, access.Decision, func()) {
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
	prepared, e := auth.Prepare(ctx, scope, access.Actor{AccountID: a.AccountID, Channel: "web"}, app.MembershipAccessPolicy(action), invitationRoot{})
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

type accessChangeRepo struct {
	calls  int
	result app.MembershipAccessResult
	err    error
}

func (r *accessChangeRepo) ChangeMembershipAccess(context.Context, kernel.OrgScope, app.EmployeeActor, string, app.MembershipAccessWrite) (app.MembershipAccessResult, error) {
	r.calls++
	return r.result, r.err
}
func (r *accessChangeRepo) ReadMembershipAccess(context.Context, kernel.OrgScope, app.EmployeeActor, string, string) (app.MembershipAccessResult, error) {
	r.calls++
	return r.result, r.err
}
func TestMembershipAccessServiceBindsCurrentRequestAndRejectsProvisionalMetadata(t *testing.T) {
	ctx, scope, a, decision, finish := accessChangeDecision(t, app.MembershipSuspend)
	command := app.MembershipAccessWrite{Action: app.MembershipSuspend, OperationID: invID(1001), MembershipID: invID(602), ExpectedRevision: 2, Reason: "Synthetic pause"}
	repo := &accessChangeRepo{result: app.MembershipAccessResult{State: d.AccessState{OrgID: scope.OrgID(), ID: command.MembershipID, EmployeeID: invID(502), Model: d.ExistingAccount, State: d.MembershipSuspended, Revision: 3, Suspension: d.ByManager}, OperationID: command.OperationID, Reason: command.Reason, OccurredAt: time.Now().UTC()}}
	service := app.MembershipAccessService{Enabled: true, Repository: repo}
	if _, e := service.Change(ctx, scope, a, decision, command); e != nil || repo.calls != 1 {
		t.Fatal("current decision", e)
	}
	if _, e := service.Change(context.Background(), scope, a, decision, command); !errors.Is(e, app.ErrEmployeeUnauthenticated) || repo.calls != 1 {
		t.Fatal("detached request", e)
	}
	if _, e := service.Change(ctx, scope, a, access.Decision{}, command); !errors.Is(e, app.ErrEmployeeUnauthenticated) || repo.calls != 1 {
		t.Fatal("forged decision", e)
	}
	other := a
	other.AccountID = invID(999)
	if _, e := service.Change(ctx, scope, other, decision, command); !errors.Is(e, app.ErrEmployeeUnauthenticated) || repo.calls != 1 {
		t.Fatal("foreign account", e)
	}
	repo.result.State.ID = invID(998)
	if out, e := service.Change(ctx, scope, a, decision, command); !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
		t.Fatal("corrupt result", e)
	}
	repo.result.State.ID = command.MembershipID
	repo.err = errors.New("synthetic late storage failure")
	if out, e := service.Change(ctx, scope, a, decision, command); e == nil || out.OperationID != "" {
		t.Fatal("provisional result", e)
	}
	finish()
	if _, e := service.Change(ctx, scope, a, decision, command); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("completed request", e)
	}
}
func TestMembershipAccessPolicyRequiresFreshnessAndStablePrivateIntent(t *testing.T) {
	for _, action := range []app.MembershipAccessAction{app.MembershipAccessRead, app.MembershipSuspend, app.MembershipReactivate} {
		p := app.MembershipAccessPolicy(action)
		if p.Validate() != nil {
			t.Fatal("source policy")
		}
		p.Fresh = false
		if p.Validate() == nil {
			t.Fatal("missing mandatory fresh proof")
		}
	}
	c := app.MembershipAccessWrite{Action: app.MembershipSuspend, OperationID: invID(1001), MembershipID: invID(602), ExpectedRevision: 2, Reason: "Synthetic original reason"}
	_, scope, _, _, _ := accessChangeDecision(t, app.MembershipSuspend)
	original := c.Digest(scope)
	c.Reason = "Synthetic changed reason"
	if original == "" || c.Digest(scope) == original {
		t.Fatal("intent lost original reason")
	}
	c.ExpectedRevision = 0
	if c.Validate() == nil {
		t.Fatal("missing CAS")
	}
}
