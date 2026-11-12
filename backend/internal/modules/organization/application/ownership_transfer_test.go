package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	membership "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

const (
	transferOrg       = "7b0e2a54-0000-4000-8000-000000000301"
	transferMember    = "7b0e2a54-0000-4000-8000-000000000601"
	transferRecipient = "7b0e2a54-0000-4000-8000-000000000602"
	transferID        = "7b0e2a54-0000-4000-8000-000000000901"
	transferOperation = "7b0e2a54-0000-4000-8000-000000001001"
)

type transferCalls struct {
	calls            int
	actor            app.CompanyActor
	member, selected string
	proposed         d.OwnershipTransfer
	err              error
}

func (r *transferCalls) Propose(_ context.Context, _ kernel.OrgScope, actor app.CompanyActor, member string, proposal d.OwnershipTransfer) (d.OwnershipTransfer, bool, error) {
	r.calls++
	r.actor = actor
	r.member = member
	r.proposed = proposal
	return proposal, false, r.err
}
func (r *transferCalls) Accept(_ context.Context, _ kernel.OrgScope, actor app.CompanyActor, member, id, operation string) (d.OwnershipTransfer, bool, error) {
	r.calls++
	r.actor = actor
	r.member = member
	r.selected = id
	return d.OwnershipTransfer{TransferID: id, OperationID: operation, State: d.OwnershipTransferAccepted}, true, r.err
}
func (r *transferCalls) Cancel(_ context.Context, _ kernel.OrgScope, actor app.CompanyActor, member, id, operation string) (d.OwnershipTransfer, bool, error) {
	r.calls++
	r.actor = actor
	r.member = member
	r.selected = id
	return d.OwnershipTransfer{TransferID: id, OperationID: operation, State: d.OwnershipTransferDeclined}, true, r.err
}
func (r *transferCalls) GetCurrent(_ context.Context, _ kernel.OrgScope, actor app.CompanyActor, member, id string) (d.OwnershipTransfer, error) {
	r.calls++
	r.actor = actor
	r.member = member
	r.selected = id
	return d.OwnershipTransfer{TransferID: id}, r.err
}

type transferAuthority struct{ authority access.Authority }

func (s transferAuthority) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return s.authority, nil
}

type transferGates struct{}

func (transferGates) Confirm(context.Context, access.Subject, access.Policy) error { return nil }
func (transferGates) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type transferRecordGate struct{}

func (transferRecordGate) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

type transferRoot struct{}

func (transferRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

// Build a real request-bound AUTH decision through its public gates. These
// application tests exercise decision binding; current database proof and
// participant eligibility are independently verified by the owning adapter.
func transferDecision(t *testing.T, policy access.Policy) (context.Context, kernel.OrgScope, app.CompanyActor, access.Decision, func()) {
	t.Helper()
	org, e := kernel.ParseOrgID(transferOrg)
	if e != nil {
		t.Fatal(e)
	}
	scope, e := kernel.NewOrgScope(org)
	if e != nil {
		t.Fatal(e)
	}
	actor := app.CompanyActor{AccountID: "7b0e2a54-0000-4000-8000-000000000701", SessionID: "7b0e2a54-0000-4000-8000-000000000702"}
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	assignment, e := permissions.RehydrateAssignment(scope, permissions.AssignmentSnapshot{OrgID: org, ID: "7b0e2a54-0000-4000-8000-000000000801", MembershipID: transferMember, AssignedBy: transferMember, Role: permissions.Owner, Scope: permissions.OrgScope, TargetID: transferOrg, ValidFrom: now.Add(-time.Hour), Reason: "Synthetic owner", Revision: 1})
	if e != nil {
		t.Fatal(e)
	}
	auth := access.Authorization{Source: transferAuthority{access.Authority{OrgID: org, AccountID: actor.AccountID, MembershipID: transferMember, EmployeeID: "7b0e2a54-0000-4000-8000-000000000501", MembershipState: membership.MembershipActive, EmployeeState: membership.EmployeeActive, Model: membership.ExistingAccount, OrganizationState: d.Active, Assignments: []permissions.Assignment{assignment}}}, Capabilities: transferGates{}, Records: transferRecordGate{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	prepared, e := auth.Prepare(ctx, scope, access.Actor{AccountID: actor.AccountID, Channel: "web"}, policy, transferRoot{})
	if e != nil {
		finish()
		t.Fatal(e)
	}
	decision, e := auth.Complete(ctx, prepared, transferGates{})
	if e != nil {
		finish()
		t.Fatal(e)
	}
	t.Cleanup(finish)
	return ctx, scope, actor, decision, finish
}

func TestOwnershipTransferServiceBindsAllEntriesToLiveProof(t *testing.T) {
	for _, action := range []string{"propose", "accept", "cancel", "read"} {
		t.Run(action, func(t *testing.T) {
			policy := app.OwnershipTransferReadPolicy()
			switch action {
			case "propose":
				policy = app.OwnershipTransferProposePolicy()
			case "accept":
				policy = app.OwnershipTransferAcceptPolicy()
			case "cancel":
				policy = app.OwnershipTransferCancelPolicy()
			}
			ctx, scope, actor, decision, finish := transferDecision(t, policy)
			for _, invalid := range []string{"empty decision", "missing actor", "other account", "other scope", "other request", "cancelled request", "finished request", "disabled"} {
				t.Run(invalid, func(t *testing.T) {
					source := &transferCalls{}
					service := app.OwnershipTransferService{Enabled: true, Repository: source, NewID: func() (string, error) { t.Fatal("invalid authority generated an ID"); return "", nil }}
					callCtx, callScope, callActor, callDecision := ctx, scope, actor, decision
					expected := app.ErrUnauthenticated
					switch invalid {
					case "empty decision":
						callDecision = access.Decision{}
					case "missing actor":
						callActor = app.CompanyActor{}
					case "other account":
						callActor.AccountID = "7b0e2a54-0000-4000-8000-000000000703"
					case "other scope":
						oid, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000302")
						callScope, _ = kernel.NewOrgScope(oid)
					case "other request":
						var end func()
						callCtx, end = access.BeginRequest(ctx)
						defer end()
					case "cancelled request":
						var cancel context.CancelFunc
						callCtx, cancel = context.WithCancel(ctx)
						cancel()
					case "finished request":
						finish()
					case "disabled":
						service.Enabled = false
						expected = app.ErrSurfaceDisabled
					}
					var e error
					switch action {
					case "propose":
						_, e = service.Propose(callCtx, callScope, callActor, callDecision, app.ProposeTransferInput{TargetMemberID: transferRecipient, OperationID: transferOperation})
					case "accept":
						_, e = service.Accept(callCtx, callScope, callActor, callDecision, app.AcceptTransferInput{TransferID: transferID, OperationID: transferOperation})
					case "cancel":
						_, e = service.Cancel(callCtx, callScope, callActor, callDecision, app.CancelTransferInput{TransferID: transferID, OperationID: transferOperation})
					case "read":
						_, e = service.GetCurrent(callCtx, callScope, callActor, callDecision, transferID)
					}
					if !errors.Is(e, expected) || source.calls != 0 {
						t.Fatal("invalid authority reached transfer storage", invalid, e, source.calls)
					}
				})
			}
		})
	}
}

func TestOwnershipTransferCommandsBindMemberAndPreserveReceipts(t *testing.T) {
	ctx, scope, actor, decision, _ := transferDecision(t, app.OwnershipTransferProposePolicy())
	source := &transferCalls{}
	service := app.OwnershipTransferService{Enabled: true, Repository: source, NewID: func() (string, error) { return transferID, nil }}
	for _, in := range []app.ProposeTransferInput{
		{OrgID: transferRecipient, TargetMemberID: transferRecipient, OperationID: transferOperation},
		{InitiatorMemberID: transferRecipient, TargetMemberID: transferRecipient, OperationID: transferOperation},
		{TargetMemberID: "not-a-canonical-id", OperationID: transferOperation},
		{TargetMemberID: transferRecipient, OperationID: ""},
	} {
		if _, e := service.Propose(ctx, scope, actor, decision, in); !errors.Is(e, d.ErrInvalid) || source.calls != 0 {
			t.Fatal("spoofed proposal accepted", e)
		}
	}
	if _, e := service.Propose(ctx, scope, actor, decision, app.ProposeTransferInput{TargetMemberID: transferMember, OperationID: transferOperation}); !errors.Is(e, d.ErrSelfTransferForbidden) || source.calls != 0 {
		t.Fatal("self transfer reached storage", e)
	}
	result, e := service.Propose(ctx, scope, actor, decision, app.ProposeTransferInput{TargetMemberID: transferRecipient, OperationID: transferOperation, StepDownToAdmin: false})
	if e != nil || result.Replayed || source.calls != 1 || source.member != transferMember || source.actor != actor || source.proposed.InitiatorMembershipID != transferMember || source.proposed.StepDownToAdmin || !source.proposed.CreatedAt.IsZero() {
		t.Fatal("proposal lost authenticated member or invented a persistence clock", e)
	}
	if _, e := service.Accept(ctx, scope, actor, decision, app.AcceptTransferInput{TransferID: transferID, OperationID: transferOperation}); !errors.Is(e, app.ErrUnauthenticated) || source.calls != 1 {
		t.Fatal("live proposal authority was reused as recipient consent", e)
	}
	missing := service
	missing.Repository = nil
	if _, e := missing.Propose(ctx, scope, actor, decision, app.ProposeTransferInput{TargetMemberID: transferRecipient, OperationID: transferOperation}); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("missing transfer repository did not fail closed", e)
	}
	for _, factory := range []func() (string, error){nil, func() (string, error) { return "", errors.New("synthetic generator failure") }, func() (string, error) { return "invalid", nil }} {
		source.calls = 0
		service.NewID = factory
		if _, e := service.Propose(ctx, scope, actor, decision, app.ProposeTransferInput{TargetMemberID: transferRecipient, OperationID: transferOperation}); !errors.Is(e, app.ErrUnavailable) || source.calls != 0 {
			t.Fatal("unavailable generator reached transfer storage", e)
		}
	}
	for _, action := range []string{"accept", "cancel", "read"} {
		t.Run(action, func(t *testing.T) {
			policy := app.OwnershipTransferAcceptPolicy()
			if action == "cancel" {
				policy = app.OwnershipTransferCancelPolicy()
			}
			if action == "read" {
				policy = app.OwnershipTransferReadPolicy()
			}
			ctx, scope, actor, proof, _ := transferDecision(t, policy)
			source := &transferCalls{}
			service := app.OwnershipTransferService{Enabled: true, Repository: source}
			var result app.OwnershipTransferResult
			var e error
			switch action {
			case "accept":
				if _, e = service.Accept(ctx, scope, actor, proof, app.AcceptTransferInput{TargetMemberID: transferRecipient, TransferID: transferID, OperationID: transferOperation}); !errors.Is(e, d.ErrInvalid) {
					t.Fatal("caller-selected recipient accepted", e)
				}
				result, e = service.Accept(ctx, scope, actor, proof, app.AcceptTransferInput{TransferID: transferID, OperationID: transferOperation})
			case "cancel":
				if _, e = service.Cancel(ctx, scope, actor, proof, app.CancelTransferInput{ActorMemberID: transferRecipient, TransferID: transferID, OperationID: transferOperation}); !errors.Is(e, d.ErrInvalid) {
					t.Fatal("caller-selected participant accepted", e)
				}
				result, e = service.Cancel(ctx, scope, actor, proof, app.CancelTransferInput{TransferID: transferID, OperationID: transferOperation, IsInitiator: true})
			case "read":
				if _, e = service.GetCurrent(ctx, scope, actor, proof, "bad"); !errors.Is(e, d.ErrInvalid) {
					t.Fatal("invalid read selector accepted", e)
				}
				_, e = service.GetCurrent(ctx, scope, actor, proof, transferID)
			}
			if e != nil || source.calls != 1 || source.actor != actor || source.member != transferMember || source.selected != transferID {
				t.Fatal("transfer command lost authenticated binding", action, e, source.calls)
			}
			if action != "read" && (!result.Replayed || result.Transfer.TransferID != transferID || result.Transfer.OperationID != transferOperation) {
				t.Fatal("committed transfer receipt lost", action)
			}
			if action == "cancel" && result.Transfer.State != d.OwnershipTransferDeclined {
				t.Fatal("caller input selected cancellation outcome")
			}
		})
	}
}

func TestOwnershipTransferPoliciesKeepParticipantAndLifecycleBoundaries(t *testing.T) {
	for _, policy := range []access.Policy{app.OwnershipTransferProposePolicy(), app.OwnershipTransferReadPolicy(), app.OwnershipTransferAcceptPolicy(), app.OwnershipTransferCancelPolicy()} {
		if policy.Validate() != nil || !policy.Fresh || policy.Capability != "organizations" {
			t.Fatal("invalid ownership source policy", policy)
		}
	}
	if p := app.OwnershipTransferProposePolicy(); p.Permission != "org.ownership.transfer" || p.Effect != access.ConfigurationWrite {
		t.Fatal("proposal lost current owner configuration guard")
	}
	if p := app.OwnershipTransferAcceptPolicy(); p.Permission != "org.ownership.respond" || p.Effect != access.ConfigurationWrite {
		t.Fatal("recipient consent lost capability write gate")
	}
	if app.OwnershipTransferReadPolicy().Effect != access.Read || app.OwnershipTransferCancelPolicy().Effect != access.ReduceAccess {
		t.Fatal("participant reads/reductions lost root lifecycle continuity")
	}
}
