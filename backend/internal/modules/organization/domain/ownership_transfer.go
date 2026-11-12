package domain

import (
	"errors"
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
