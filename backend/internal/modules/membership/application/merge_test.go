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
	"strings"
	"testing"
	"time"
)

func employeeMergeDecision(t *testing.T, write bool) (context.Context, kernel.OrgScope, app.EmployeeActor, access.Decision, func()) {
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
	prepared, e := auth.Prepare(ctx, scope, access.Actor{AccountID: a.AccountID, Channel: "web"}, app.EmployeeMergePolicy(write), invitationRoot{})
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

type employeeMergeRepo struct {
	result     app.EmployeeMergeResult
	view       app.EmployeeMergeView
	resolution app.EmployeeResolution
	err        error
	calls      int
}

func (r *employeeMergeRepo) PreviewEmployeeMerge(context.Context, kernel.OrgScope, app.EmployeeActor, string, string, string) (app.EmployeeMergeView, error) {
	r.calls++
	return r.view, r.err
}
func (r *employeeMergeRepo) MergeEmployees(context.Context, kernel.OrgScope, app.EmployeeActor, string, app.EmployeeMergeCommand) (app.EmployeeMergeResult, error) {
	r.calls++
	return r.result, r.err
}
func (r *employeeMergeRepo) ResolveEmployee(context.Context, kernel.OrgScope, app.EmployeeActor, string, string) (app.EmployeeResolution, error) {
	r.calls++
	return r.resolution, r.err
}
func TestEmployeeMergeRequiresCurrentReviewAndRejectsProvisionalMetadata(t *testing.T) {
	ctx, scope, a, decision, finish := employeeMergeDecision(t, true)
	c := app.EmployeeMergeCommand{OperationID: invID(1501), SourceID: invID(1502), SurvivorID: invID(1503), SourceRevision: 1, SurvivorRevision: 2, ReviewDigest: strings.Repeat("a", 64), Reviewed: true, Reason: "Synthetic merge"}
	repo := &employeeMergeRepo{result: app.EmployeeMergeResult{OperationID: c.OperationID, SourceID: c.SourceID, SurvivorID: c.SurvivorID, SourceRevision: 2, SurvivorRevision: 2, SourceState: d.EmployeeTerminated, SurvivorState: d.EmployeeActive, Reason: c.Reason, Teams: []app.MergeTeamEffect{}, OccurredAt: time.Now().UTC()}}
	service := app.EmployeeMergeService{Enabled: true, Repository: repo}
	if _, e := service.Merge(ctx, scope, a, decision, c); e != nil {
		t.Fatal(e)
	}
	if _, e := service.Merge(context.Background(), scope, a, decision, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("detached request", e)
	}
	if _, e := service.Merge(ctx, scope, a, access.Decision{}, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("forged decision", e)
	}
	changed := c
	changed.SurvivorRevision++
	if changed.Digest(scope) == c.Digest(scope) {
		t.Fatal("unbound survivor revision")
	}
	changed = c
	changed.Reviewed = false
	if changed.Validate() == nil {
		t.Fatal("unreviewed merge")
	}
	repo.result.SurvivorID = invID(1599)
	if out, e := service.Merge(ctx, scope, a, decision, c); !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
		t.Fatal("foreign acknowledgement", e)
	}
	repo.err = errors.New("synthetic failure")
	if out, e := service.Merge(ctx, scope, a, decision, c); e == nil || out.OperationID != "" {
		t.Fatal("provisional result")
	}
	finish()
	if _, e := service.Merge(ctx, scope, a, decision, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("finished request", e)
	}
}
