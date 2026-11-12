package domain

import (
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"time"
)

type OwnershipTransferState string

const (
	OwnershipTransferPending   OwnershipTransferState = "PENDING"
	OwnershipTransferAccepted  OwnershipTransferState = "ACCEPTED"
	OwnershipTransferCancelled OwnershipTransferState = "CANCELLED"
	OwnershipTransferDeclined  OwnershipTransferState = "DECLINED"
	OwnershipTransferExpired   OwnershipTransferState = "EXPIRED"
)

const OwnershipTransferValidity = 7 * 24 * time.Hour

var (
	ErrOwnerRequiresPersonalAccount = errors.New("owner requires personal account")
	ErrTransferPending              = errors.New("open ownership transfer already pending")
	ErrSelfTransferForbidden        = errors.New("cannot transfer ownership to oneself")
	ErrTargetAlreadyOwner           = errors.New("target member already has active owner role")
	ErrTargetNotActive              = errors.New("target member is not active")
	ErrTargetEmployeeNotActive      = errors.New("target employee is not active")
	ErrTransferNotFound             = errors.New("ownership transfer not found")
	ErrTransferNotPending           = errors.New("ownership transfer is not pending")
	ErrTransferExpired              = errors.New("ownership transfer has expired")
	ErrNotTransferTarget            = errors.New("only target member can accept ownership transfer")
	ErrNotTransferParticipant       = errors.New("caller is neither initiator nor target of ownership transfer")
	ErrTransferOperationReused      = errors.New("operation id already used with different parameters")
	ErrOrganizationNotActive        = errors.New("organization must be active for ownership transfer")
)

type OwnershipTransfer struct {
	privateSettings
	TransferID            string
	OrgID                 string
	InitiatorMembershipID string
	TargetMembershipID    string
	StepDownToAdmin       bool
	State                 OwnershipTransferState
	ExpiresAt             time.Time
	AcceptedAt            *time.Time
	CancelledAt           *time.Time
	DeclinedAt            *time.Time
	CreatedAt             time.Time
	OperationID           string
}

func (t OwnershipTransfer) IsPendingAt(now time.Time) bool {
	return t.State == OwnershipTransferPending && now.Before(t.ExpiresAt)
}

func ValidateTransferProposal(initiatorMembershipID, targetMembershipID string, targetHasOwner, targetIsManaged, targetActive, targetEmployeeActive bool) error {
	if initiatorMembershipID == "" || targetMembershipID == "" {
		return errors.New("membership ids must be provided")
	}
	if initiatorMembershipID == targetMembershipID {
		return ErrSelfTransferForbidden
	}
	if !targetActive {
		return ErrTargetNotActive
	}
	if !targetEmployeeActive {
		return ErrTargetEmployeeNotActive
	}
	if targetIsManaged {
		return ErrOwnerRequiresPersonalAccount
	}
	if targetHasOwner {
		return ErrTargetAlreadyOwner
	}
	return nil
}

// Validate accepts a retained scoped value, never permission or client time.
func (t OwnershipTransfer) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil || t.OrgID != scope.OrgID().String() {
		return ErrScope
	}
	for _, id := range []string{t.TransferID, t.InitiatorMembershipID, t.TargetMembershipID, t.OperationID} {
		if !canonicalLegalID(id) {
			return ErrInvalid
		}
	}
	if t.InitiatorMembershipID == t.TargetMembershipID || !settingsInstant(t.CreatedAt) || !settingsInstant(t.ExpiresAt) || !t.ExpiresAt.Equal(t.CreatedAt.Add(OwnershipTransferValidity)) {
		return ErrInvalid
	}
	for _, at := range []*time.Time{t.AcceptedAt, t.CancelledAt, t.DeclinedAt} {
		if at != nil && (!settingsInstant(*at) || at.Before(t.CreatedAt)) {
			return ErrInvalid
		}
	}
	switch t.State {
	case OwnershipTransferPending, OwnershipTransferExpired:
		if t.AcceptedAt != nil || t.CancelledAt != nil || t.DeclinedAt != nil {
			return ErrInvalid
		}
	case OwnershipTransferAccepted:
		if t.AcceptedAt == nil || t.CancelledAt != nil || t.DeclinedAt != nil || !t.AcceptedAt.Before(t.ExpiresAt) {
			return ErrInvalid
		}
	case OwnershipTransferCancelled:
		if t.CancelledAt == nil || t.AcceptedAt != nil || t.DeclinedAt != nil {
			return ErrInvalid
		}
	case OwnershipTransferDeclined:
		if t.DeclinedAt == nil || t.AcceptedAt != nil || t.CancelledAt != nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
