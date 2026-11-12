package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func OwnershipTransferProposePolicy() access.Policy {
	return access.Policy{
		Permission: "org.ownership.transfer",
		Capability: "organizations",
		Effect:     access.ConfigurationWrite,
		Fresh:      true,
	}
}

func OwnershipTransferReadPolicy() access.Policy {
	return access.Policy{
		Permission: "org.ownership.transfer",
		Capability: "organizations",
		Effect:     access.Read,
		Fresh:      true,
	}
}

type ProposeTransferInput struct {
	OrgID             string
	InitiatorMemberID string
	TargetMemberID    string
	StepDownToAdmin   bool
	OperationID       string
}

type AcceptTransferInput struct {
	OrgID          string
	TransferID     string
	TargetMemberID string
	OperationID    string
}

type CancelTransferInput struct {
	OrgID          string
	TransferID     string
	ActorMemberID  string
	OperationID    string
	IsInitiator    bool
}

type OwnershipTransferResult struct {
	Transfer d.OwnershipTransfer
	Replayed bool
}

type OwnershipTransferRepository interface {
	Propose(ctx context.Context, scope kernel.OrgScope, t d.OwnershipTransfer) (d.OwnershipTransfer, bool, error)
	Accept(ctx context.Context, scope kernel.OrgScope, transferID string, targetMemberID string, opID string) (d.OwnershipTransfer, bool, error)
	Cancel(ctx context.Context, scope kernel.OrgScope, transferID string, actorMemberID string, opID string, isInitiator bool) (d.OwnershipTransfer, bool, error)
	GetCurrent(ctx context.Context, scope kernel.OrgScope) (d.OwnershipTransfer, error)
}

type OwnershipTransferService struct {
	Enabled    bool
	Repository OwnershipTransferRepository
	NewID      func() (string, error)
	Now        func() time.Time
}

func (s OwnershipTransferService) Propose(ctx context.Context, scope kernel.OrgScope, in ProposeTransferInput) (OwnershipTransferResult, error) {
	if !s.Enabled || s.Repository == nil {
		return OwnershipTransferResult{}, errors.New("ownership transfer service disabled")
	}
	if in.InitiatorMemberID == in.TargetMemberID {
		return OwnershipTransferResult{}, d.ErrSelfTransferForbidden
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	id, err := s.NewID()
	if err != nil {
		return OwnershipTransferResult{}, err
	}
	transfer := d.OwnershipTransfer{
		TransferID:            id,
		OrgID:                 scope.OrgID().String(),
		InitiatorMembershipID: in.InitiatorMemberID,
		TargetMembershipID:    in.TargetMemberID,
		StepDownToAdmin:       in.StepDownToAdmin,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           in.OperationID,
	}
	saved, replayed, err := s.Repository.Propose(ctx, scope, transfer)
	if err != nil {
		return OwnershipTransferResult{}, err
	}
	return OwnershipTransferResult{Transfer: saved, Replayed: replayed}, nil
}

func (s OwnershipTransferService) Accept(ctx context.Context, scope kernel.OrgScope, in AcceptTransferInput) (OwnershipTransferResult, error) {
	if !s.Enabled || s.Repository == nil {
		return OwnershipTransferResult{}, errors.New("ownership transfer service disabled")
	}
	saved, replayed, err := s.Repository.Accept(ctx, scope, in.TransferID, in.TargetMemberID, in.OperationID)
	if err != nil {
		return OwnershipTransferResult{}, err
	}
	return OwnershipTransferResult{Transfer: saved, Replayed: replayed}, nil
}

func (s OwnershipTransferService) Cancel(ctx context.Context, scope kernel.OrgScope, in CancelTransferInput) (OwnershipTransferResult, error) {
	if !s.Enabled || s.Repository == nil {
		return OwnershipTransferResult{}, errors.New("ownership transfer service disabled")
	}
	saved, replayed, err := s.Repository.Cancel(ctx, scope, in.TransferID, in.ActorMemberID, in.OperationID, in.IsInitiator)
	if err != nil {
		return OwnershipTransferResult{}, err
	}
	return OwnershipTransferResult{Transfer: saved, Replayed: replayed}, nil
}

func (s OwnershipTransferService) GetCurrent(ctx context.Context, scope kernel.OrgScope) (d.OwnershipTransfer, error) {
	if !s.Enabled || s.Repository == nil {
		return d.OwnershipTransfer{}, errors.New("ownership transfer service disabled")
	}
	return s.Repository.GetCurrent(ctx, scope)
}
