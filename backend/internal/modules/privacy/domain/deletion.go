package domain

import (
	"errors"
	"strings"
	"time"
)

// Erasure scopes. Each scope owns its purge/unlink step and its ledger
// row, so restore replay can reapply scope by scope and report per-scope
// counts.
const (
	ScopeIdentity  = "IDENTITY"
	ScopeCommunity = "COMMUNITY"
	ScopeEvidence  = "EVIDENCE"
	ScopeTrust     = "TRUST"
	ScopeExports   = "EXPORTS"
)

// AllScopes lists every erasure scope in execution order: identity
// first (revoke writes immediately), then content unlinking, then the
// ledger's own archive purge last.
func AllScopes() []string {
	return []string{ScopeIdentity, ScopeCommunity, ScopeEvidence, ScopeTrust, ScopeExports}
}

var (
	// ErrUnknownScope marks a ledger scope outside the fixed set.
	ErrUnknownScope = errors.New("privacy: unknown erasure scope")
)

// LedgerParams carries one deletion-ledger intent: the erased owner,
// the scope it covers, and the reviewed reason behind it.
type LedgerParams struct {
	ID             string
	ContributorID  string
	ContributorRef string
	Scope          string
	Reason         string
	OccurredAt     time.Time
	PolicyVersion  string
}

// LedgerEntry is one minimal bounded record retained to reapply a
// removal after restore. It carries identifiers and scope only: enough
// to find reappeared rows, never payload (B-BR-016).
type LedgerEntry struct {
	ID             string
	ContributorID  string
	ContributorRef string
	Scope          string
	Reason         string
	OccurredAt     time.Time
	ReplayedAt     time.Time
	PolicyVersion  string
}

// NewLedgerEntry validates and freezes one ledger row.
func NewLedgerEntry(p LedgerParams) (LedgerEntry, error) {
	if strings.TrimSpace(p.ID) == "" ||
		strings.TrimSpace(p.ContributorID) == "" ||
		strings.TrimSpace(p.ContributorRef) == "" {
		return LedgerEntry{}, ErrInvalidRequest
	}
	switch p.Scope {
	case ScopeIdentity, ScopeCommunity, ScopeEvidence, ScopeTrust, ScopeExports:
	default:
		return LedgerEntry{}, ErrUnknownScope
	}
	reason := strings.TrimSpace(p.Reason)
	if reason == "" || len([]rune(reason)) > MaxReasonChars {
		return LedgerEntry{}, ErrInvalidRequest
	}
	if p.OccurredAt.IsZero() {
		return LedgerEntry{}, ErrInvalidRequest
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	return LedgerEntry{
		ID: p.ID, ContributorID: p.ContributorID,
		ContributorRef: p.ContributorRef, Scope: p.Scope,
		Reason: reason, OccurredAt: p.OccurredAt,
		PolicyVersion: p.PolicyVersion,
	}, nil
}

// CompleteDeletion moves a REQUESTED deletion intent to READY without
// an archive: erasure produces a receipt, not bytes. The auditable
// receipt is the ledger plus this completion event.
func CompleteDeletion(r Request, at time.Time) (Request, PrivacyRequestCompleted, error) {
	if r.Type != TypeDeletion {
		return Request{}, PrivacyRequestCompleted{}, ErrInvalidRequest
	}
	if r.Status != StatusRequested {
		return Request{}, PrivacyRequestCompleted{}, ErrBadState
	}
	if at.IsZero() {
		return Request{}, PrivacyRequestCompleted{}, ErrInvalidRequest
	}
	r.Status = StatusReady
	r.ReadyAt = at
	r.CompletedAt = at
	return r, PrivacyRequestCompleted{
		RequestID: r.ID, ContributorID: r.ContributorID,
		Type: r.Type, Status: r.Status, OccurredAt: at,
	}, nil
}
