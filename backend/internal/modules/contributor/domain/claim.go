package domain

import (
	"crypto/rand"
	"encoding/base32"
	"io"
	"time"
)

const (
	ClaimCodePrefix = "abastevo-contributor-"
	ClaimCodeBytes  = 10
	ClaimLifetime   = 30 * 24 * time.Hour
	MaxClaimsPerDay = 3
	MaxProfileEdits = 10
)

type ClaimState string

const (
	ClaimPending   ClaimState = "PENDING"
	ClaimApproved  ClaimState = "APPROVED"
	ClaimRejected  ClaimState = "REJECTED"
	ClaimWithdrawn ClaimState = "WITHDRAWN"
	ClaimExpired   ClaimState = "EXPIRED"
)

type ProofStatus string

const (
	ProofUnverified ProofStatus = "UNVERIFIED"
	ProofVerified   ProofStatus = "VERIFIED"
)

type Claim struct {
	ID             string
	AccountID      string
	OperationID    string
	GitHubLogin    string
	Code           string
	State          ClaimState
	ProofStatus    ProofStatus
	GistID         string
	Override       bool
	DecisionReason string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	DecidedAt      *time.Time
}

// GenerateClaimCode returns abastevo-contributor- plus 16 base32 characters.
func GenerateClaimCode(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	raw := make([]byte, ClaimCodeBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return ClaimCodePrefix + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func ValidClaimCode(raw string) bool {
	if len(raw) != len(ClaimCodePrefix)+16 || raw[:len(ClaimCodePrefix)] != ClaimCodePrefix {
		return false
	}
	_, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(raw[len(ClaimCodePrefix):])
	return err == nil
}

func ValidClaimState(raw string) bool {
	switch ClaimState(raw) {
	case ClaimPending, ClaimApproved, ClaimRejected, ClaimWithdrawn, ClaimExpired:
		return true
	}
	return false
}

// CanTransition covers the CTB-07 terminal state machine.
func CanTransition(from, to ClaimState) bool {
	if from != ClaimPending {
		return false
	}
	switch to {
	case ClaimApproved, ClaimRejected, ClaimWithdrawn, ClaimExpired:
		return true
	}
	return false
}

func ClaimOpen(claim Claim, now time.Time) bool {
	return claim.State == ClaimPending && now.Before(claim.ExpiresAt)
}

func TerminalState(claim Claim) bool {
	return claim.State != ClaimPending
}

// ClaimFilter bounds one admin claim list read.
type ClaimFilter struct {
	State         ClaimState
	BeforeCreated time.Time
	BeforeID      string
	Limit         int
}
