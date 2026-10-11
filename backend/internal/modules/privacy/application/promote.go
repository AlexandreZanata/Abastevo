package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Promotion worker contract (CS10A, AT03): offer consented images to
// the private derivative through a fail-closed pipeline — current
// consent re-check at commit, minimization verdict, converged
// inventory write. Source bytes never persist here: the sanitizer
// returns the minimized bytes, their hash binds the inventory row,
// and both byte strings drop when the call returns. Persistence rows,
// the sanitizer adapter, scheduling and physical deletion land in
// later CS10 tasks; every layer must enforce this same contract.

// Sanitizer minimizes one source image to the fuel-price region with
// EXIF/GPS and identifying content removed. The verdict is
// authoritative: anything but PASS refuses the commit.
type Sanitizer interface {
	Sanitize(ctx context.Context, source []byte) ([]byte, domain.SanitizeVerdict, error)
}

// PromotionInventory is the owned derivative-inventory port: converged
// writes keyed by image plus consent revision, owner-scoped active
// reads, idempotent revocation marks, and revoked rows due for purge.
type PromotionInventory interface {
	Put(ctx context.Context, d domain.Derivative) (stored bool, replayed bool, err error)
	ListActiveByOwner(ctx context.Context, contributorID string) ([]domain.Derivative, error)
	MarkRevoked(ctx context.Context, imageID string, revision uint64, at time.Time) error
	ListRevokedBefore(ctx context.Context, before time.Time) ([]domain.Derivative, error)
}

// PromotePorts wires promotion through the shared consent ports
// (server clock, owner attribution, guarded consent reads, current
// terms) plus the inventory writer and the sanitizer.
type PromotePorts struct {
	ConsentPorts
	Inventory PromotionInventory
	Sanitizer Sanitizer
}

// PromotionDTO carries one promotion offer. Owner identity resolves
// server-side from the caller; the source bytes arrive from the
// evidence quarantine read, never from an owner-supplied store.
type PromotionDTO struct {
	ImageID         string
	CapturedAt      time.Time
	PerImageAllowed bool
	Source          []byte
}

// PromotionResult is the safe acknowledgment: the derivative identity
// and whether the call converged on an existing row. PromotedAt is
// set on fresh proposals (key derivation for the namespace write);
// replays carry the zero instant because no write follows.
type PromotionResult struct {
	ImageID          string
	DerivativeSHA256 string
	ConsentRevision  uint64
	PromotedAt       time.Time
	Replayed         bool
}

// Propose offers one image to the derivative inventory. It re-reads
// the current consent at commit time (a revocation between queueing
// and commit refuses), minimizes fail-closed, and converges identical
// re-proposals on the image-plus-revision key. Sanitizer outages and
// non-PASS verdicts refuse with no inventory write. On success it
// also returns the minimized bytes just produced so the caller can
// land them in the private namespace through the object port; the
// bytes are nil on replay (nothing new to store) and must never be
// persisted anywhere else — the API layer never calls Propose.
func Propose(ctx context.Context, p PromotePorts, caller Caller, dto PromotionDTO) (PromotionResult, []byte, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return PromotionResult{}, nil, ErrUnauthorized
	}
	if strings.TrimSpace(dto.ImageID) == "" || dto.CapturedAt.IsZero() || len(dto.Source) == 0 {
		return PromotionResult{}, nil, domain.ErrInvalidRequest
	}
	token, err := p.Attribution(ctx, caller.ContributorID)
	if err != nil {
		return PromotionResult{}, nil, err
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	consent, err := p.Store.Get(ctx, caller.ContributorID)
	if err != nil {
		if isNotFound(err) {
			return PromotionResult{}, nil, domain.ErrConsentRequired
		}
		return PromotionResult{}, nil, err
	}
	minimized, verdict, err := p.Sanitizer.Sanitize(ctx, dto.Source)
	if err != nil {
		return PromotionResult{}, nil, err
	}
	sum := sha256.Sum256(minimized)
	derivative, _, err := domain.ProposeDerivative(domain.PromotionCandidate{
		ImageID: dto.ImageID, ContributorID: caller.ContributorID, ContributorRef: token,
		CapturedAt: dto.CapturedAt, PerImageAllowed: dto.PerImageAllowed,
		SourceSHA256: hashSource(dto.Source), QueuedAtConsentRevision: consent.Revision,
	}, &consent, verdict, hex.EncodeToString(sum[:]), now,
		p.Terms.CurrentVersion(), p.Terms.CurrentDigest())
	if err != nil {
		return PromotionResult{}, nil, err
	}
	stored, replayed, err := p.Inventory.Put(ctx, derivative)
	if err != nil {
		return PromotionResult{}, nil, err
	}
	_ = stored
	if replayed {
		return PromotionResult{
			ImageID: derivative.ImageID, DerivativeSHA256: derivative.DerivativeSHA256,
			ConsentRevision: derivative.ConsentRevision, Replayed: true,
		}, nil, nil
	}
	return PromotionResult{
		ImageID: derivative.ImageID, DerivativeSHA256: derivative.DerivativeSHA256,
		ConsentRevision: derivative.ConsentRevision, PromotedAt: now,
	}, minimized, nil
}

// RevokeOwner excludes one owner's active derivatives from selection
// and reports how many rows flipped. Repeats converge to zero: the
// receipt tombstones stay for the restore filter while the purge path
// owns the bytes (CS10B/C).
func RevokeOwner(ctx context.Context, p PromotePorts, caller Caller) (int, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return 0, ErrUnauthorized
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	active, err := p.Inventory.ListActiveByOwner(ctx, caller.ContributorID)
	if err != nil {
		return 0, err
	}
	flipped := 0
	for _, d := range active {
		if _, changed, err := domain.RevokeDerivative(d, now); err != nil {
			return flipped, err
		} else if !changed {
			continue
		}
		if err := p.Inventory.MarkRevoked(ctx, d.ImageID, d.ConsentRevision, now); err != nil {
			return flipped, err
		}
		flipped++
	}
	return flipped, nil
}

// hashSource binds the exact source bytes a proposal minimizes, so a
// substituted object under the same image reference cannot slip into
// inventory (mirrors the evidence media snapshot convention in a few
// lines to preserve module ownership).
func hashSource(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}
