package application

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/media"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Stable reason codes recorded when a snapshot cannot become READY. They
// are part of the contract: rename only with a policy-version bump.
const (
	ReasonOversize           = "oversize"
	ReasonInvalidImage       = "invalid-image"
	ReasonChecksumMismatch   = "checksum-mismatch"
	ReasonTrailingData       = "trailing-data"
	ReasonDimensionsExceeded = "dimensions-exceeded"
)

var (
	// ErrRejected marks permanent verification refusals. Use errors.As
	// for *RejectionError to read the stable reason codes.
	ErrRejected = errors.New("evidence: snapshot rejected")
	// ErrBadState marks programmer error: only VERIFYING sessions verify.
	ErrBadState = errors.New("evidence: session must be VERIFYING")
)

// RejectionError is the permanent refusal with stable reason codes. Any
// other error from Verify is transient: retry without terminal progress.
type RejectionError struct {
	Reasons []string
}

func (e *RejectionError) Error() string {
	return fmt.Sprintf("%s: %s", ErrRejected, e.Reasons)
}

// Unwrap matches ErrRejected so callers map by errors.Is.
func (e *RejectionError) Unwrap() error { return ErrRejected }

// VerifyPorts declares the worker collaborators. Download fetches the
// single bounded snapshot; Upload stores sanitized bytes under the
// server-only final key. Real transports land in T04; fakes prove the
// orchestration here, including the exactly-once download.
// FinalWrite is either a nonce-fenced new output or explicit read-only reuse.
// A zero value never grants permission to upload or commit an outcome.
type FinalWrite struct {
	Nonce  string
	Reused bool
}

type VerifyPorts struct {
	BeforeUpload func(ctx context.Context, key, sanitizedSHA256 string) (FinalWrite, error)
	Clock        func() time.Time
	Download     func(ctx context.Context, key string, maxBytes int64) ([]byte, error)
	Upload       func(ctx context.Context, key, contentType string, body []byte) error
}

// Outcome binds one verified snapshot: the READY session plus the
// content-addressed final key and the server-computed signals. Only these
// bytes may become READY downstream (T04 persists them). ExpiresAt
// carries the forward 24 h copy deadline (P15-T03); the v1 path leaves
// it zero and the retention sweeper owns that lifecycle instead.
type Outcome struct {
	Write           FinalWrite
	Session         domain.Session
	FinalKey        string
	SourceSHA256    string
	SanitizedSHA256 string
	Width           int
	Height          int
	DHash           uint64
	ExpiresAt       time.Time
}

// Verify validates one VERIFYING session through exactly one bounded
// download: size, checksum claim, strict structure, dimension caps, then
// sanitization to the server-only final key. Permanent media refusals
// return *RejectionError; transport and unexpected failures return
// transient errors with no terminal progress.
func Verify(ctx context.Context, p VerifyPorts, session domain.Session) (Outcome, error) {
	if session.Status != domain.StateVerifying {
		return Outcome{}, ErrBadState
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	// One download, capped one byte past the session bound so overflow is
	// detectable instead of silently truncated.
	data, err := p.Download(ctx, session.QuarantineKey, session.MaxBytes+1)
	if err != nil {
		return Outcome{}, err
	}
	snap, err := media.Snapshot(data, session.MaxBytes)
	if err != nil {
		if errors.Is(err, media.ErrTooLarge) {
			return Outcome{}, &RejectionError{Reasons: []string{ReasonOversize}}
		}
		return Outcome{}, err
	}
	if err := checkClaim(snap, session.ClaimedSHA256); err != nil {
		return Outcome{}, err
	}
	dims, err := media.Inspect(snap.Bytes)
	if err != nil {
		return Outcome{}, rejectionFor(err)
	}
	clean, err := media.Sanitize(snap.Bytes)
	if err != nil {
		return Outcome{}, rejectionFor(err)
	}
	dhash, err := media.DifferenceHash(snap.Bytes)
	if err != nil {
		return Outcome{}, err
	}
	finalKey := media.FinalKey(snap.Bytes)
	sanitizedSum := sha256.Sum256(clean)
	sanitizedHash := hex.EncodeToString(sanitizedSum[:])
	authority, err := writeFinal(ctx, p, finalKey, domain.AllowedMIME, sanitizedHash, clean)
	if err != nil {
		return Outcome{}, err
	}
	ready, _, err := domain.MarkReady(session, now)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		Write:   authority,
		Session: ready, FinalKey: finalKey,
		SourceSHA256:    snap.SourceSHA256,
		SanitizedSHA256: hex.EncodeToString(sanitizedSum[:]),
		Width:           dims.Width, Height: dims.Height, DHash: dhash,
	}, nil
}

// checkClaim binds the downloaded bytes to the reservation's hash claim.
// A mismatch means corruption or a substituted object, never a pass: the
// claim is untrusted, but disagreement with it is a hard refusal.
func checkClaim(snap media.Snap, claimHex string) error {
	claim, err := hex.DecodeString(claimHex)
	if err != nil {
		return fmt.Errorf("evidence: unreadable hash claim: %w", err)
	}
	actual, err := hex.DecodeString(snap.SourceSHA256)
	if err != nil {
		return err
	}
	if len(claim) != sha256.Size || subtle.ConstantTimeCompare(claim, actual) != 1 {
		return &RejectionError{Reasons: []string{ReasonChecksumMismatch}}
	}
	return nil
}

func rejectionFor(err error) error {
	var reasons []string
	switch {
	case errors.Is(err, media.ErrTrailingData):
		reasons = []string{ReasonTrailingData}
	case errors.Is(err, media.ErrDimensionsExceeded):
		reasons = []string{ReasonDimensionsExceeded}
	case errors.Is(err, media.ErrNotJPEG) || errors.Is(err, media.ErrInvalidImage):
		reasons = []string{ReasonInvalidImage}
	default:
		return err
	}
	return &RejectionError{Reasons: reasons}
}

// writeFinal commits inventory before invoking a transport. Reuse never sends
// bytes again, so duplicates cannot resurrect a copy or slide its deadline.
func writeFinal(ctx context.Context, p VerifyPorts, key, mime, sanitizedHash string, body []byte) (FinalWrite, error) {
	if p.BeforeUpload == nil {
		return FinalWrite{}, ErrBadState
	}
	authority, err := p.BeforeUpload(ctx, key, sanitizedHash)
	if err != nil {
		return FinalWrite{}, err
	}
	if authority.Reused {
		if authority.Nonce != "" {
			return FinalWrite{}, ErrBadState
		}
		return authority, nil
	}
	if authority.Nonce == "" || p.Upload == nil {
		return FinalWrite{}, ErrBadState
	}
	if err := p.Upload(ctx, key, mime, body); err != nil {
		return FinalWrite{}, err
	}
	return authority, nil
}
