package application

import (
	"context"
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/jackc/pgx/v5"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func OwnershipTransferProposePolicy() access.Policy {
	return access.Policy{Permission: "org.ownership.transfer", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}
func OwnershipTransferReadPolicy() access.Policy {
	return access.Policy{Permission: "org.ownership.respond", Capability: "organizations", Effect: access.Read, Fresh: true}
}
func OwnershipTransferAcceptPolicy() access.Policy {
	return access.Policy{Permission: "org.ownership.respond", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}
func OwnershipTransferCancelPolicy() access.Policy {
	return access.Policy{Permission: "org.ownership.respond", Capability: "organizations", Effect: access.ReduceAccess, Fresh: true}
}

type ProposeTransferInput struct {
	privateOnboarding
	OrgID, InitiatorMemberID, TargetMemberID string
	StepDownToAdmin                          bool
	OperationID                              string
}
type AcceptTransferInput struct {
	privateOnboarding
	OrgID, TransferID, TargetMemberID, OperationID string
}
type CancelTransferInput struct {
	privateOnboarding
	OrgID, TransferID, ActorMemberID, OperationID string
	IsInitiator                                   bool
}
type OwnershipTransferResult struct {
	privateOnboarding
	Transfer d.OwnershipTransfer
	Replayed bool
}

// OwnershipAssignments includes retained scheduled OWNER/ADMIN periods without
// broadening the live-only assignment candidates used by authorization.
type OwnershipAssignments interface {
	access.TransactionRepository
	RemainingOwnershipAssignmentsTx(context.Context, pgx.Tx, kernel.OrgScope, string) ([]roles.Assignment, error)
}

type OwnershipTransferRepository interface {
	Propose(context.Context, kernel.OrgScope, CompanyActor, string, d.OwnershipTransfer) (d.OwnershipTransfer, bool, error)
	Accept(context.Context, kernel.OrgScope, CompanyActor, string, string, string) (d.OwnershipTransfer, bool, error)
	Cancel(context.Context, kernel.OrgScope, CompanyActor, string, string, string) (d.OwnershipTransfer, bool, error)
	GetCurrent(context.Context, kernel.OrgScope, CompanyActor, string, string) (d.OwnershipTransfer, error)
}
type OwnershipTransferService struct {
	Enabled    bool
	Repository OwnershipTransferRepository
	NewID      func() (string, error)
	Now        func() time.Time
}

func (s OwnershipTransferService) check(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, policy access.Policy) error {
	if !s.Enabled {
		return ErrSurfaceDisabled
	}
	if scope.Validate() != nil {
		return d.ErrScope
	}
	if actor.Validate() != nil || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, scope.OrgID().String()) != nil {
		return ErrUnauthenticated
	}
	if s.Repository == nil {
		return ErrUnavailable
	}
	return nil
}
func (s OwnershipTransferService) Propose(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, in ProposeTransferInput) (OwnershipTransferResult, error) {
	if e := s.check(ctx, scope, actor, decision, OwnershipTransferProposePolicy()); e != nil {
		return OwnershipTransferResult{}, e
	}
	member := decision.Subject().MembershipID()
	if in.OrgID != "" && in.OrgID != scope.OrgID().String() || in.InitiatorMemberID != "" && in.InitiatorMemberID != member || !canonicalID(in.TargetMemberID) || !canonicalID(in.OperationID) {
		return OwnershipTransferResult{}, d.ErrInvalid
	}
	if member == in.TargetMemberID {
		return OwnershipTransferResult{}, d.ErrSelfTransferForbidden
	}
	if s.NewID == nil {
		return OwnershipTransferResult{}, ErrUnavailable
	}
	id, e := s.NewID()
	if e != nil || !canonicalID(id) {
		return OwnershipTransferResult{}, ErrUnavailable
	}
	t := d.OwnershipTransfer{OrgID: scope.OrgID().String(), TransferID: id, InitiatorMembershipID: member, TargetMembershipID: in.TargetMemberID, StepDownToAdmin: in.StepDownToAdmin, OperationID: in.OperationID}
	saved, replay, e := s.Repository.Propose(ctx, scope, actor, member, t)
	if e != nil {
		return OwnershipTransferResult{}, e
	}
	return OwnershipTransferResult{Transfer: saved, Replayed: replay}, nil
}
func (s OwnershipTransferService) Accept(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, in AcceptTransferInput) (OwnershipTransferResult, error) {
	if e := s.check(ctx, scope, actor, decision, OwnershipTransferAcceptPolicy()); e != nil {
		return OwnershipTransferResult{}, e
	}
	member := decision.Subject().MembershipID()
	if in.OrgID != "" && in.OrgID != scope.OrgID().String() || in.TargetMemberID != "" && in.TargetMemberID != member || !canonicalID(in.TransferID) || !canonicalID(in.OperationID) {
		return OwnershipTransferResult{}, d.ErrInvalid
	}
	t, replay, e := s.Repository.Accept(ctx, scope, actor, member, in.TransferID, in.OperationID)
	if e != nil {
		return OwnershipTransferResult{}, e
	}
	return OwnershipTransferResult{Transfer: t, Replayed: replay}, nil
}
func (s OwnershipTransferService) Cancel(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, in CancelTransferInput) (OwnershipTransferResult, error) {
	if e := s.check(ctx, scope, actor, decision, OwnershipTransferCancelPolicy()); e != nil {
		return OwnershipTransferResult{}, e
	}
	member := decision.Subject().MembershipID()
	if in.OrgID != "" && in.OrgID != scope.OrgID().String() || in.ActorMemberID != "" && in.ActorMemberID != member || !canonicalID(in.TransferID) || !canonicalID(in.OperationID) {
		return OwnershipTransferResult{}, d.ErrInvalid
	}
	// Caller input never decides whether this is cancellation or decline.
	t, replay, e := s.Repository.Cancel(ctx, scope, actor, member, in.TransferID, in.OperationID)
	if e != nil {
		return OwnershipTransferResult{}, e
	}
	return OwnershipTransferResult{Transfer: t, Replayed: replay}, nil
}
func (s OwnershipTransferService) GetCurrent(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, id string) (d.OwnershipTransfer, error) {
	if e := s.check(ctx, scope, actor, decision, OwnershipTransferReadPolicy()); e != nil {
		return d.OwnershipTransfer{}, e
	}
	if id != "" && !canonicalID(id) {
		return d.OwnershipTransfer{}, d.ErrInvalid
	}
	return s.Repository.GetCurrent(ctx, scope, actor, decision.Subject().MembershipID(), id)
}
