package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/media"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// VerifyForward validates one VERIFYING session through the frozen
// P15-T01 forward budgets (B-BR-M02/M03): exactly one bounded
// download capped one byte past the 256 KiB server cap, checksum
// claim, strict structure, forward pixel caps (1600 edge, 2 MP),
// then bounded sanitize to the server-only final key.
//
// No client value widens anything: the download bound, the pixel
// caps and the quality schedule are server constants; the session
// MaxBytes (3 MiB v1 reservation bound) only ever narrows the read
// it no longer governs. The copy deadline is immutable from first
// receipt — the VERIFYING transition instant (completion intent),
// the closest marker before the T04 received_at migration — and no
// retry, copy, dispute or download extends it. Permanent media
// refusals return *RejectionError with the stable v1 reason codes;
// transport and unexpected failures stay transient.
//
// The deployed v1 Verify path stays live until the P15-T04
// migration/rollout; this lane is proven here and cut over there.
func VerifyForward(ctx context.Context, p VerifyPorts, session domain.Session) (Outcome, error) {
	if session.Status != domain.StateVerifying {
		return Outcome{}, ErrBadState
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	// Server cap only: one byte past 256 KiB so overflow is
	// detectable instead of silently truncated. The session's own
	// MaxBytes never widens this read.
	data, err := p.Download(ctx, session.QuarantineKey, domain.ForwardCapBytes+1)
	if err != nil {
		return Outcome{}, err
	}
	snap, err := media.Snapshot(data, domain.ForwardCapBytes)
	if err != nil {
		if errors.Is(err, media.ErrTooLarge) {
			return Outcome{}, &RejectionError{Reasons: []string{ReasonOversize}}
		}
		return Outcome{}, err
	}
	if err := checkClaim(snap, session.ClaimedSHA256); err != nil {
		return Outcome{}, err
	}
	dims, err := media.InspectForward(snap.Bytes)
	if err != nil {
		return Outcome{}, rejectionFor(err)
	}
	clean, err := media.SanitizeForward(snap.Bytes)
	if err != nil {
		if errors.Is(err, media.ErrTooLarge) {
			return Outcome{}, &RejectionError{Reasons: []string{ReasonOversize}}
		}
		return Outcome{}, rejectionFor(err)
	}
	dhash, err := media.DifferenceHash(snap.Bytes)
	if err != nil {
		return Outcome{}, err
	}
	finalKey := media.FinalKey(snap.Bytes)
	if err := p.Upload(ctx, finalKey, domain.ForwardWireMIME, clean); err != nil {
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
		ExpiresAt: domain.ForwardDeadlineAfter(session.UpdatedAt),
	}, nil
}
