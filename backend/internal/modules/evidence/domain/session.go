package domain

import (
	"errors"
	"strings"
	"time"
)

// Evidence policy version frozen with this session shape. Changes require
// fixtures and a new version, never silent reinterpretation.
const PolicyV1 = "evidence-v1"

// Structural reservation caps (COMMUNITY_PRICING_SPEC abuse defaults and
// the SECURITY_PRIVACY evidence protocol): JPEG intent only, at most 3 MiB
// declared per session, and an idle session expires 24 h after creation.
// The presigned URL itself lives only 5 min; that T02 credential TTL is
// separate from this reservation deadline.
const (
	AllowedMIME     = "image/jpeg"
	MaxUploadBytes  = 3 << 20
	SessionTTL      = 24 * time.Hour
	maxKeyLength    = 256
	sha256HexLength = 64
)

// Session states mirror the API contract: ISSUED→VERIFYING after the
// completion intent; VERIFYING→READY after frozen validation;
// VERIFYING→REJECTED on invalid media; idle ISSUED→EXPIRED.
const (
	StateIssued    = "ISSUED"
	StateVerifying = "VERIFYING"
	StateReady     = "READY"
	StateRejected  = "REJECTED"
	StateExpired   = "EXPIRED"
)

var (
	ErrInvalidSession   = errors.New("evidence: invalid session")
	ErrUnsupportedMedia = errors.New("evidence: only image/jpeg intent is allowed")
	ErrSizeOutOfBounds  = errors.New("evidence: declared size outside 1..3MiB")
	ErrBadHashClaim     = errors.New("evidence: hash claim must be 64 hex characters")
	ErrBadTransition    = errors.New("evidence: transition not allowed")
	ErrBadReason        = errors.New("evidence: stable reason codes required")
	ErrNotExpired       = errors.New("evidence: session still within its reservation deadline")
)

// Params carries reservation intent plus server-resolved ownership. Every
// server-controlled field (ID, contributor, quarantine key, clock, policy)
// arrives from the caller, never from the client body: the client supplies
// only its submission ID, media description and hash claim.
type Params struct {
	ID              string
	ContributorRef  string
	ClientSessionID string
	MIME            string
	DeclaredBytes   int64
	ClaimedSHA256   string
	QuarantineKey   string
	CreatedAt       time.Time
	PolicyVersion   string
}

// Session is one owned upload reservation. Status mutates only through the
// transition functions below, which return updated copies: nothing edits a
// session in place.
type Session struct {
	ID              string
	ContributorRef  string
	ClientSessionID string
	MIME            string
	DeclaredBytes   int64
	MaxBytes        int64
	ClaimedSHA256   string
	QuarantineKey   string
	Status          string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	PolicyVersion   string
}

// Event is the domain event recorded at each transition.
type Event struct {
	SessionID  string
	From       string
	To         string
	OccurredAt time.Time
}

// Name derives the emitted event name for a transition.
func (e Event) Name() string {
	switch e.From + ">" + e.To {
	case ">ISSUED":
		return "SessionReserved"
	case "ISSUED>VERIFYING":
		return "VerificationRequested"
	case "VERIFYING>READY":
		return "EvidenceReady"
	case "VERIFYING>REJECTED":
		return "EvidenceRejected"
	case "ISSUED>EXPIRED":
		return "SessionExpired"
	default:
		return ""
	}
}

// NewSession reserves one owned upload session under structural limits.
// Only JPEG intent within the byte cap and a well-formed hash claim reach
// quota and storage; everything else fails before expensive work starts.
func NewSession(p Params) (Session, Event, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ContributorRef) == "" ||
		strings.TrimSpace(p.ClientSessionID) == "" {
		return Session{}, Event{}, ErrInvalidSession
	}
	if p.MIME != AllowedMIME {
		return Session{}, Event{}, ErrUnsupportedMedia
	}
	if p.DeclaredBytes < 1 || p.DeclaredBytes > MaxUploadBytes {
		return Session{}, Event{}, ErrSizeOutOfBounds
	}
	claim, err := normalizeHashClaim(p.ClaimedSHA256)
	if err != nil {
		return Session{}, Event{}, err
	}
	if !validQuarantineKey(p.QuarantineKey) {
		return Session{}, Event{}, ErrInvalidSession
	}
	if p.CreatedAt.IsZero() {
		return Session{}, Event{}, ErrInvalidSession
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	s := Session{
		ID: p.ID, ContributorRef: p.ContributorRef,
		ClientSessionID: p.ClientSessionID, MIME: p.MIME,
		DeclaredBytes: p.DeclaredBytes, MaxBytes: MaxUploadBytes,
		ClaimedSHA256: claim, QuarantineKey: p.QuarantineKey,
		Status: StateIssued, CreatedAt: p.CreatedAt,
		ExpiresAt: p.CreatedAt.Add(SessionTTL), PolicyVersion: p.PolicyVersion,
	}
	return s, Event{SessionID: s.ID, To: StateIssued, OccurredAt: s.CreatedAt}, nil
}

// normalizeHashClaim admits exactly 64 hex characters (either case,
// stored lowercase): the client hash stays a claim verified later, but
// garbage never reserves quota.
func normalizeHashClaim(raw string) (string, error) {
	if len(raw) != sha256HexLength {
		return "", ErrBadHashClaim
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			continue
		}
		return "", ErrBadHashClaim
	}
	return strings.ToLower(raw), nil
}

// validQuarantineKey accepts the server-generated opaque key and refuses
// traversal or overlong values. Keys are never client-supplied; this guard
// keeps a miswired caller from persisting a dangerous one.
func validQuarantineKey(k string) bool {
	if k == "" || len(k) > maxKeyLength {
		return false
	}
	if strings.Contains(k, "..") {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x20 || k[i] == 0x7f {
			return false
		}
	}
	return true
}

// RequestVerification moves ISSUED→VERIFYING on the completion intent.
func RequestVerification(s Session, at time.Time) (Session, Event, error) {
	if s.Status != StateIssued {
		return Session{}, Event{}, ErrBadTransition
	}
	next := s
	next.Status = StateVerifying
	return next, Event{SessionID: s.ID, From: StateIssued, To: StateVerifying, OccurredAt: at}, nil
}

// MarkReady moves VERIFYING→READY after frozen server validation.
func MarkReady(s Session, at time.Time) (Session, Event, error) {
	if s.Status != StateVerifying {
		return Session{}, Event{}, ErrBadTransition
	}
	next := s
	next.Status = StateReady
	return next, Event{SessionID: s.ID, From: StateVerifying, To: StateReady, OccurredAt: at}, nil
}

// Reject moves VERIFYING→REJECTED with stable reason codes.
func Reject(s Session, reasons []string, at time.Time) (Session, Event, error) {
	if s.Status != StateVerifying {
		return Session{}, Event{}, ErrBadTransition
	}
	if len(reasons) == 0 {
		return Session{}, Event{}, ErrBadReason
	}
	for _, r := range reasons {
		if strings.TrimSpace(r) == "" {
			return Session{}, Event{}, ErrBadReason
		}
	}
	next := s
	next.Status = StateRejected
	return next, Event{SessionID: s.ID, From: StateVerifying, To: StateRejected, OccurredAt: at}, nil
}

// Expire moves an idle ISSUED session to EXPIRED past its deadline.
// Sessions under active verification never expire here; the retention
// sweeper (T05) owns their lifecycle from the database.
func Expire(s Session, now time.Time) (Session, Event, error) {
	if s.Status != StateIssued {
		return Session{}, Event{}, ErrBadTransition
	}
	if !now.After(s.ExpiresAt) {
		return Session{}, Event{}, ErrNotExpired
	}
	next := s
	next.Status = StateExpired
	return next, Event{SessionID: s.ID, From: StateIssued, To: StateExpired, OccurredAt: now}, nil
}
