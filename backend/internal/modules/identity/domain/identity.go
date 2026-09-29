package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Challenge purposes. REGISTER binds a candidate key; SIGN authorizes later
// actions. A challenge never crosses purposes.
const (
	PurposeRegister = "REGISTER"
	PurposeSign     = "SIGN"
)

// ChallengeTTL bounds proof freshness (API_PLAN: 5-minute expiry).
const ChallengeTTL = 5 * time.Minute

var (
	ErrUnknownPurpose     = errors.New("identity: unknown challenge purpose")
	ErrBadFingerprint     = errors.New("identity: malformed fingerprint")
	ErrBadNonce           = errors.New("identity: malformed nonce")
	ErrChallengeExpired   = errors.New("identity: challenge expired")
	ErrChallengeSpent     = errors.New("identity: challenge already consumed")
	ErrProofRequired      = errors.New("identity: valid key proof required")
	ErrDuplicateKey       = errors.New("identity: key already registered")
	ErrIdempotentConflict = errors.New("identity: same key, different body")
	ErrKeyTakeover        = errors.New("identity: key owned by another contributor")
)

// ParsePurpose accepts exactly the stored purposes.
func ParsePurpose(s string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case PurposeRegister:
		return PurposeRegister, nil
	case PurposeSign:
		return PurposeSign, nil
	default:
		return "", ErrUnknownPurpose
	}
}

// ParseFingerprint accepts "fp:" plus 64 lowercase hex characters.
func ParseFingerprint(s string) (string, error) {
	fp := strings.TrimSpace(s)
	if !strings.HasPrefix(fp, "fp:") || len(fp) != 67 {
		return "", ErrBadFingerprint
	}
	for _, r := range fp[3:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", ErrBadFingerprint
		}
	}
	return fp, nil
}

// SplitNonce separates "<challenge-id>.<client-nonce>"; both sides must be
// non-empty printable matter without whitespace.
func SplitNonce(nonce string) (challengeID, clientNonce string, err error) {
	id, cn, ok := strings.Cut(nonce, ".")
	if !ok || id == "" || cn == "" {
		return "", "", ErrBadNonce
	}
	for _, r := range nonce {
		if r <= ' ' || r > '~' {
			return "", "", ErrBadNonce
		}
	}
	return id, cn, nil
}

// Challenge is a bound proof opportunity: one fingerprint, one purpose,
// one expiry. Consumption is single-shot and atomic in the adapters.
type Challenge struct {
	ID          string
	Nonce       string
	Fingerprint string
	Purpose     string
	ExpiresAt   time.Time
}

// Validate checks binding shape and liveness without touching storage.
func (c Challenge) Validate(now time.Time) error {
	if _, err := ParseFingerprint(c.Fingerprint); err != nil {
		return err
	}
	if _, err := ParsePurpose(c.Purpose); err != nil {
		return err
	}
	if _, _, err := SplitNonce(c.Nonce); err != nil {
		return err
	}
	if !now.Before(c.ExpiresAt) {
		return ErrChallengeExpired
	}
	return nil
}

// Contributor is a key-owned anonymous identity. No email, phone or personal
// fields exist in MVP; the private key never leaves the client (BUC-002).
type Contributor struct {
	ID        string
	Status    string
	CreatedAt time.Time
}

// PublicKey binds one JWK to one contributor through its fingerprint.
type PublicKey struct {
	ID            string
	ContributorID string
	Algorithm     string
	Fingerprint   string
}

// Registration is the outcome: always the same contributor for the same key.
type Registration struct {
	ContributorID string
	KeyID         string
	Existed       bool
}

// IdempotencyTTL bounds how long a stored outcome replays (API_PLAN: 7 days).
const IdempotencyTTL = 7 * 24 * time.Hour

// IdempotencyKey scopes one operation: contributor plus method plus route
// template plus client key. Route templates never carry IDs or query values.
type IdempotencyKey struct {
	ContributorID string
	Method        string
	Route         string
	Key           string
}

// AttemptOutcome is the stored safe result. Only the hash of the request is
// kept, never the body, so retries cannot leak sensitive payloads from the
// ledger.
type AttemptOutcome struct {
	StatusCode int
	Response   []byte
	ExpiresAt  time.Time
	Completed  bool
}

// HashBody digests the exact transmitted bytes for equality checks.
func HashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Registrar persists challenges and keys atomically.
type Registrar interface {
	IssueChallenge(ctx context.Context, fingerprint, purpose string) (Challenge, error)
	Register(ctx context.Context, req RegistrationRequest) (Registration, error)
}

// RegistrationRequest carries a candidate key plus its challenge proof. The
// base lines are rebuilt server-side from the request envelope; clients
// never submit prebuilt bases.
type RegistrationRequest struct {
	JWKX       string
	JWKY       string
	Challenge  Challenge
	BaseLines  []string
	Signature  string
	VerifiedAt time.Time
}

// RotationProof is one side of a rotation: the challenge that binds it,
// the covered base lines and the signature. Old and new proofs travel
// together so neither key alone can move the identity.
type RotationProof struct {
	Challenge Challenge
	BaseLines []string
	Signature string
}

// RotationRequest rotates a contributor from an old key to a new one. Both
// proofs verify before anything mutates; without the old private key there
// is no recovery, by design.
type RotationRequest struct {
	Old        RotationProof
	NewJWKX    string
	NewJWKY    string
	New        RotationProof
	VerifiedAt time.Time
}

// Rotation is the outcome: the contributor preserved, the old key revoked,
// the new key bound.
type Rotation struct {
	ContributorID string
	OldKeyID      string
	NewKeyID      string
	Existed       bool
}
