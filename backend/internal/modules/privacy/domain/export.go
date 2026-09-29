package domain

import (
	"errors"
	"strings"
	"time"
)

// Privacy policy and archive-format versions frozen with this ledger.
// Changes require a new version, never silent reinterpretation.
const (
	PolicyV1 = "privacy-v1"
	FormatV1 = "privacy-export-v1"
)

// Request types. EXPORT assembles the owner's inventory (P07-T03);
// DELETION revokes and purges it (P07-T04, same ledger).
const (
	TypeExport   = "EXPORT"
	TypeDeletion = "DELETION"
)

// Request statuses. EXPIRED is derived at read time from ExpiresAt, not
// a stored state: a READY archive past its window refuses downloads
// while its receipt stays auditable.
const (
	StatusRequested = "REQUESTED"
	StatusReady     = "READY"
	StatusFailed    = "FAILED"
)

// DownloadTTL bounds owner access to a READY archive (API_PLAN: audit
// and expiry 24 h). MaxArchiveBytes bounds the stored archive so one
// export cannot become an unbounded data dump.
const (
	DownloadTTL     = 24 * time.Hour
	MaxArchiveBytes = 1 << 20
	MaxReasonChars  = 200
)

var (
	ErrInvalidRequest = errors.New("privacy: invalid request")
	ErrUnknownType    = errors.New("privacy: unknown request type")
	ErrBadState       = errors.New("privacy: request not actionable")
	ErrTooLarge       = errors.New("privacy: archive exceeds bound")
	ErrExpired        = errors.New("privacy: export expired")
	// ErrConflict marks a divergent-payload retry on a converged
	// natural key. Callers map it onto 409 without disclosing the
	// stored row.
	ErrConflict = errors.New("privacy: conflicting request")
	// ErrMismatch marks an archive whose bytes do not match its
	// recorded hash. Callers fail the build, never store it.
	ErrMismatch = errors.New("privacy: archive hash mismatch")
	// ErrNotFound marks a missing request or a foreign-owner read.
	// Both share one shape so export existence is never an
	// ownership oracle.
	ErrNotFound = errors.New("privacy: request not found")
)

// RequestParams carries one privacy intent. ContributorID is the
// server-derived owner identity (from proof, never a body-supplied
// trust decision); ContributorRef is its attribution token scoping
// owner rows; ClientSubmissionID makes mobile retries safe.
type RequestParams struct {
	ID                 string
	ContributorID      string
	ContributorRef     string
	ClientSubmissionID string
	Type               string
	RequestedAt        time.Time
	PolicyVersion      string
}

// Request is one durable privacy intent with its bounded outcome.
type Request struct {
	ID                 string
	ContributorID      string
	ContributorRef     string
	ClientSubmissionID string
	Type               string
	Status             string
	ArchiveSHA256      string
	RequestedAt        time.Time
	ReadyAt            time.Time
	ExpiresAt          time.Time
	CompletedAt        time.Time
	PolicyVersion      string
}

// PrivacyRequestCompleted is the minimal domain event recorded at
// completion: identifiers and outcome only, never payload (B-BR-011).
type PrivacyRequestCompleted struct {
	RequestID     string
	ContributorID string
	Type          string
	Status        string
	OccurredAt    time.Time
}

// Name derives the emitted event name.
func (e PrivacyRequestCompleted) Name() string { return "PrivacyRequestCompleted" }

// NewRequest validates and freezes one intent in REQUESTED state.
func NewRequest(p RequestParams) (Request, error) {
	if strings.TrimSpace(p.ID) == "" ||
		strings.TrimSpace(p.ContributorID) == "" ||
		strings.TrimSpace(p.ContributorRef) == "" ||
		strings.TrimSpace(p.ClientSubmissionID) == "" {
		return Request{}, ErrInvalidRequest
	}
	switch p.Type {
	case TypeExport, TypeDeletion:
	default:
		return Request{}, ErrUnknownType
	}
	if p.RequestedAt.IsZero() {
		return Request{}, ErrInvalidRequest
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	return Request{
		ID: p.ID, ContributorID: p.ContributorID,
		ContributorRef:     p.ContributorRef,
		ClientSubmissionID: strings.TrimSpace(p.ClientSubmissionID),
		Type:               p.Type, Status: StatusRequested,
		RequestedAt: p.RequestedAt, PolicyVersion: p.PolicyVersion,
	}, nil
}

// Complete moves REQUESTED to READY with a bounded archive: non-empty,
// within the byte cap, carrying its recorded hash. The download window
// opens at completion and closes exactly DownloadTTL later.
func Complete(r Request, archive []byte, sha256Hex string, at time.Time) (Request, PrivacyRequestCompleted, error) {
	if r.Status != StatusRequested {
		return Request{}, PrivacyRequestCompleted{}, ErrBadState
	}
	if len(archive) == 0 || len(archive) > MaxArchiveBytes {
		return Request{}, PrivacyRequestCompleted{}, ErrTooLarge
	}
	if strings.TrimSpace(sha256Hex) == "" || at.IsZero() {
		return Request{}, PrivacyRequestCompleted{}, ErrInvalidRequest
	}
	r.Status = StatusReady
	r.ArchiveSHA256 = strings.ToLower(strings.TrimSpace(sha256Hex))
	r.ReadyAt = at
	r.ExpiresAt = at.Add(DownloadTTL)
	r.CompletedAt = at
	return r, PrivacyRequestCompleted{
		RequestID: r.ID, ContributorID: r.ContributorID,
		Type: r.Type, Status: r.Status, OccurredAt: at,
	}, nil
}

// Fail moves REQUESTED to FAILED with a bounded reason. Failures stay
// auditable; a new client submission ID starts a fresh attempt.
func Fail(r Request, reason string, at time.Time) (Request, PrivacyRequestCompleted, error) {
	if r.Status != StatusRequested {
		return Request{}, PrivacyRequestCompleted{}, ErrBadState
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > MaxReasonChars || at.IsZero() {
		return Request{}, PrivacyRequestCompleted{}, ErrInvalidRequest
	}
	r.Status = StatusFailed
	r.CompletedAt = at
	return r, PrivacyRequestCompleted{
		RequestID: r.ID, ContributorID: r.ContributorID,
		Type: r.Type, Status: r.Status, OccurredAt: at,
	}, nil
}

// Expired reports whether a READY request is past its download window
// at the given instant. Non-READY requests never count as expired:
// their outcome simply does not exist yet (or failed).
func Expired(r Request, now time.Time) bool {
	if r.Status != StatusReady {
		return false
	}
	return !now.Before(r.ExpiresAt)
}
