package http

import (
	"context"
	"errors"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"time"
)

const OwnershipTransferPrefix = "/v1/orgs/{org_id}/ownership-transfer"
const OwnershipTransferProposePath = OwnershipTransferPrefix + "/propose"
const OwnershipTransferAcceptPath = OwnershipTransferPrefix + "/accept"
const OwnershipTransferCancelPath = OwnershipTransferPrefix + "/cancel"
const OwnershipTransferReadPath = OwnershipTransferPrefix + "/read"

func ownershipID(raw string) bool {
	id, e := kernel.ParseOrgID(raw)
	return e == nil && id.String() == raw
}

type ownershipTransferProposeRequest struct {
	companyRequest
	SchemaVersion      int    `json:"schema_version"`
	TargetMembershipID string `json:"target_membership_id"`
	StepDownToAdmin    *bool  `json:"step_down_to_admin"`
	OperationID        string `json:"operation_id"`
}

func (v ownershipTransferProposeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *ownershipTransferProposeRequest) UnmarshalJSON(raw []byte) error {
	type plain ownershipTransferProposeRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	if input.StepDownToAdmin == nil {
		return orgapi.Malformed
	}
	*v = ownershipTransferProposeRequest(input)
	return nil
}

type ownershipTransferAcceptRequest struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	TransferID    string `json:"transfer_id"`
	OperationID   string `json:"operation_id"`
}

func (v ownershipTransferAcceptRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *ownershipTransferAcceptRequest) UnmarshalJSON(raw []byte) error {
	type plain ownershipTransferAcceptRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = ownershipTransferAcceptRequest(input)
	return nil
}

type ownershipTransferCancelRequest ownershipTransferAcceptRequest

func (v ownershipTransferCancelRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}
func (v *ownershipTransferCancelRequest) UnmarshalJSON(raw []byte) error {
	type plain ownershipTransferCancelRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = ownershipTransferCancelRequest(input)
	return nil
}

type ownershipTransferReadRequest struct {
	companyRequest
	TransferID *string `json:"transfer_id,omitempty"`
}

func (v *ownershipTransferReadRequest) UnmarshalJSON(raw []byte) error {
	type plain ownershipTransferReadRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	if input.TransferID != nil && (*input.TransferID == "" || !ownershipID(*input.TransferID)) {
		return orgapi.Malformed
	}
	*v = ownershipTransferReadRequest(input)
	return nil
}

type OwnershipTransferRecords struct{}

func (OwnershipTransferRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || subject.MembershipID() == "" || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	if policy != app.OwnershipTransferProposePolicy() && policy != app.OwnershipTransferReadPolicy() && policy != app.OwnershipTransferAcceptPolicy() && policy != app.OwnershipTransferCancelPolicy() {
		return access.ErrUnavailable
	}
	if policy.Permission == "org.ownership.respond" && facts.AssignedTo != subject.MembershipID() {
		return access.ErrUnavailable
	}
	return nil
}

// These facts permit a member to attempt consent using a SELF assignment.
// They never attest participation: the transaction rechecks the actual source
// participants before returning private state or applying a command.
type ownershipResponseFacts struct{ RootFacts }

func (f ownershipResponseFacts) Resolve(ctx context.Context, scope kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	v, e := f.RootFacts.Resolve(ctx, scope, subject)
	if e != nil {
		return permissions.ResourceFacts{}, e
	}
	v.AssignedTo = subject.MembershipID()
	return v, nil
}
func ownershipTransferRule(policy access.Policy, write bool) orgapi.Rule {
	return orgapi.Rule{Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, CompanyOnly: true, Success: 200}
}
func ownershipTransferMapped(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, d.ErrOwnerRequiresPersonalAccount) {
		return orgapi.OwnerRequiresPersonal
	}
	if errors.Is(err, d.ErrTransferPending) {
		return orgapi.TransferPending
	}
	if errors.Is(err, d.ErrSelfTransferForbidden) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrTargetAlreadyOwner) || errors.Is(err, permissions.ErrAssignmentConflict) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrTargetNotActive) || errors.Is(err, d.ErrTargetEmployeeNotActive) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrTransferNotFound) {
		return orgapi.NotFound
	}
	if errors.Is(err, d.ErrTransferNotPending) || errors.Is(err, d.ErrTransferExpired) {
		return orgapi.Conflict
	}
	if errors.Is(err, d.ErrNotTransferTarget) || errors.Is(err, d.ErrNotTransferParticipant) {
		return orgapi.NotFound
	}
	if errors.Is(err, d.ErrTransferOperationReused) {
		return orgapi.OperationReused
	}
	if errors.Is(err, d.ErrOrganizationNotActive) {
		return orgapi.OrgSuspended
	}
	return legalMapped(err, "")
}

func ownershipTransferView(t d.OwnershipTransfer) map[string]any {
	view := map[string]any{
		"transfer_id":             t.TransferID,
		"org_id":                  t.OrgID,
		"initiator_membership_id": t.InitiatorMembershipID,
		"target_membership_id":    t.TargetMembershipID,
		"step_down_to_admin":      t.StepDownToAdmin,
		"state":                   string(t.State),
		"expires_at":              t.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"created_at":              t.CreatedAt.UTC().Format(time.RFC3339Nano),
		"operation_id":            t.OperationID,
		"data_mode":               "SYNTHETIC",
	}
	if t.AcceptedAt != nil {
		view["accepted_at"] = t.AcceptedAt.UTC().Format(time.RFC3339Nano)
	}
	if t.CancelledAt != nil {
		view["cancelled_at"] = t.CancelledAt.UTC().Format(time.RFC3339Nano)
	}
	if t.DeclinedAt != nil {
		view["declined_at"] = t.DeclinedAt.UTC().Format(time.RFC3339Nano)
	}
	return view
}

func RegisterOwnershipTransfer(router chi.Router, service app.OwnershipTransferService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository, memberships *memberpg.Store) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || roots == nil || memberships == nil {
		return app.ErrUnavailable
	}
	owned, gateway, e := companyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	propose, read, accept, cancel := app.OwnershipTransferProposePolicy(), app.OwnershipTransferReadPolicy(), app.OwnershipTransferAcceptPolicy(), app.OwnershipTransferCancelPolicy()
	ack := func(result app.OwnershipTransferResult) any {
		return map[string]any{"transfer": ownershipTransferView(result.Transfer), "replayed": result.Replayed}
	}
	if e := guarded.Register(owned, OwnershipTransferProposePath, gateway, ownershipTransferRule(propose, true), authorization, propose, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input ownershipTransferProposeRequest) (any, error) {
		if !ownershipID(input.TargetMembershipID) || input.StepDownToAdmin == nil {
			return nil, orgapi.Malformed
		}
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Propose(ctx, p.Scope, actor, decision, app.ProposeTransferInput{OrgID: p.Scope.OrgID().String(), TargetMemberID: input.TargetMembershipID, StepDownToAdmin: *input.StepDownToAdmin, OperationID: input.OperationID})
		if e != nil {
			return nil, ownershipTransferMapped(e)
		}
		return ack(result), nil
	}); e != nil {
		return e
	}
	facts := ownershipResponseFacts{RootFacts{roots}}
	if e := guarded.Register(owned, OwnershipTransferReadPath, gateway, ownershipTransferRule(read, false), authorization, read, facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input ownershipTransferReadRequest) (any, error) {
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		id := ""
		if input.TransferID != nil {
			id = *input.TransferID
		}
		result, e := service.GetCurrent(ctx, p.Scope, actor, decision, id)
		if e != nil {
			return nil, ownershipTransferMapped(e)
		}
		return ownershipTransferView(result), nil
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, OwnershipTransferAcceptPath, gateway, ownershipTransferRule(accept, true), authorization, accept, facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input ownershipTransferAcceptRequest) (any, error) {
		if !ownershipID(input.TransferID) {
			return nil, orgapi.Malformed
		}
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Accept(ctx, p.Scope, actor, decision, app.AcceptTransferInput{TransferID: input.TransferID, OperationID: input.OperationID})
		if e != nil {
			return nil, ownershipTransferMapped(e)
		}
		return ack(result), nil
	}); e != nil {
		return e
	}
	return guarded.Register(owned, OwnershipTransferCancelPath, gateway, ownershipTransferRule(cancel, true), authorization, cancel, facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input ownershipTransferCancelRequest) (any, error) {
		if !ownershipID(input.TransferID) {
			return nil, orgapi.Malformed
		}
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		result, e := service.Cancel(ctx, p.Scope, actor, decision, app.CancelTransferInput{TransferID: input.TransferID, OperationID: input.OperationID})
		if e != nil {
			return nil, ownershipTransferMapped(e)
		}
		return ack(result), nil
	})
}
