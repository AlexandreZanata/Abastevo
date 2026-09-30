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
type VerifyPorts struct {
	Clock    func() time.Time
	Download func(ctx context.Context, key string, maxBytes int64) ([]byte, error)
	Upload   func(ctx context.Context, key, contentType string, body []byte) error
}

// Outcome binds one verified snapshot: the READY session plus the
// content-addressed final key and the server-computed signals. Only these
// bytes may become READY downstream (T04 persists them). ExpiresAt
// carries the forward 24 h copy deadline (P15-T03); the v1 path leaves
// it zero and the retention sweeper owns that lifecycle instead.
type Outcome struct {
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
	if err := p.Upload(ctx, finalKey, domain.AllowedMIME, clean); err != nil {
		return Outcome{}, err
	}
	ready, _, err := domain.MarkReady(session, now)
	if err != nil {
		return Outcome{}, err
	}
	sanitizedSum := sha256.Sum256(clean)
	return Outcome{
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
