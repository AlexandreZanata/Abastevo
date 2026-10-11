package domain

import (
	"errors"
	"strings"
	"time"
)

// Promotion contract (CS10A, AT03): which images may enter the private
// training derivative, decided fail-closed. Promotion re-checks the
// current consent at commit time: a revocation landing between
// queueing and commit refuses, and a stale offline grant can never
// resurrect selection. Uncertain sanitization refuses exactly like
// failure — no promotion on doubt, no original-photo fallback.
//
// Scope: pure domain only. Derivative persistence, the sanitizer
// adapter, the worker and physical deletion land in later CS10 tasks;
// every layer must enforce this same contract.
const (
	// DerivativeActive marks inventory selectable for training while
	// the owner's consent stays granted on current terms.
	DerivativeActive = "ACTIVE"
	// DerivativeRevoked marks inventory excluded after consent
	// withdrawal: the receipt stays for the restore filter while the
	// purge path owns the bytes (CS10B/C).
	DerivativeRevoked = "REVOKED"
)

// SanitizeVerdict is the minimizer's frozen outcome for one source
// image: PASS admits the sanitized derivative, anything else refuses.
type SanitizeVerdict string

const (
	VerdictPass      SanitizeVerdict = "PASS"
	VerdictFail      SanitizeVerdict = "FAIL"
	VerdictUncertain SanitizeVerdict = "UNCERTAIN"
)

var (
	// ErrConsentRequired marks a promotion attempt without a currently
	// granted consent: absent, revoked or otherwise non-granted.
	// Callers refuse the commit with no inventory write.
	ErrConsentRequired = errors.New("privacy: training consent required")
	// ErrImageNotCovered marks a candidate outside the grant's scope:
	// captured before the grant or opted out per image. Grants never
	// authorize old photos retroactively.
	ErrImageNotCovered = errors.New("privacy: image not covered by consent")
	// ErrNotSanitizable marks a failed or uncertain minimization
	// verdict. Both refuse identically: fail-closed, no fallback.
	ErrNotSanitizable = errors.New("privacy: image not sanitizable")
)

// PromotionCandidate is one image offered to the derivative: the
// candidate reference, its original capture instant, whether the
// owner opted this image in at review, the hash binding the exact
// source bytes, and the consent revision the submitter saw when
// queueing (audit trail; commit re-checks current consent).
type PromotionCandidate struct {
	ImageID                 string
	ContributorID           string
	ContributorRef          string
	CapturedAt              time.Time
	PerImageAllowed         bool
	SourceSHA256            string
	QueuedAtConsentRevision uint64
}

// Derivative is one minimized inventory record: references and hashes
// that bind bytes, never pixels, GPS, keys or labels (B-BR-011).
type Derivative struct {
	ImageID          string
	ContributorID    string
	ContributorRef   string
	SourceSHA256     string
	DerivativeSHA256 string
	CapturedAt       time.Time
	TermsVersion     string
	TermsDigest      string
	State            string
	ConsentRevision  uint64
	PromotedAt       time.Time
	RevokedAt        time.Time
}

// PromotionReceipt is the minimal auditable outcome: identifiers and
// decision only, never payload.
type PromotionReceipt struct {
	ImageID         string
	ContributorID   string
	State           string
	ConsentRevision uint64
	PromotedAt      time.Time
	Replayed        bool
}

// ProposeDerivative admits one candidate to the derivative inventory
// when every gate holds at commit time: a currently granted consent
// on the current terms, future-only per-image scope, a PASS verdict
// and well-formed references/hashes. Anything else refuses with no
// partial record.
func ProposeDerivative(candidate PromotionCandidate, consent *Consent, verdict SanitizeVerdict, derivativeSHA string, at time.Time, currentVersion, currentDigest string) (Derivative, PromotionReceipt, error) {
	if strings.TrimSpace(candidate.ImageID) == "" ||
		len([]rune(candidate.ImageID)) > MaxImageIDChars ||
		strings.TrimSpace(candidate.ContributorID) == "" ||
		strings.TrimSpace(candidate.ContributorRef) == "" ||
		strings.TrimSpace(candidate.SourceSHA256) == "" ||
		strings.TrimSpace(derivativeSHA) == "" ||
		candidate.CapturedAt.IsZero() || at.IsZero() {
		return Derivative{}, PromotionReceipt{}, ErrInvalidRequest
	}
	switch verdict {
	case VerdictPass:
	case VerdictFail, VerdictUncertain:
		return Derivative{}, PromotionReceipt{}, ErrNotSanitizable
	default:
		return Derivative{}, PromotionReceipt{}, ErrInvalidRequest
	}
	if consent == nil || consent.State != StateGranted {
		return Derivative{}, PromotionReceipt{}, ErrConsentRequired
	}
	if consent.ContributorID != strings.TrimSpace(candidate.ContributorID) ||
		consent.ContributorRef != strings.TrimSpace(candidate.ContributorRef) {
		return Derivative{}, PromotionReceipt{}, ErrNotFound
	}
	if consent.TermsVersion != currentVersion || consent.TermsDigest != currentDigest {
		return Derivative{}, PromotionReceipt{}, ErrTermsOutdated
	}
	if !candidate.PerImageAllowed || candidate.CapturedAt.Before(consent.GrantedAt) {
		return Derivative{}, PromotionReceipt{}, ErrImageNotCovered
	}
	d := Derivative{
		ImageID: candidate.ImageID, ContributorID: consent.ContributorID,
		ContributorRef: consent.ContributorRef, SourceSHA256: strings.TrimSpace(candidate.SourceSHA256),
		DerivativeSHA256: strings.TrimSpace(derivativeSHA), CapturedAt: candidate.CapturedAt,
		TermsVersion: consent.TermsVersion, TermsDigest: consent.TermsDigest,
		State: DerivativeActive, ConsentRevision: consent.Revision, PromotedAt: at,
	}
	return d, PromotionReceipt{
		ImageID: d.ImageID, ContributorID: d.ContributorID,
		State: d.State, ConsentRevision: d.ConsentRevision, PromotedAt: at,
	}, nil
}

// RevokeDerivative excludes one derivative from selection while
// keeping its receipt for the restore filter. Repeating the stored
// revocation converges (changed=false).
func RevokeDerivative(d Derivative, at time.Time) (Derivative, bool, error) {
	if at.IsZero() {
		return Derivative{}, false, ErrInvalidRequest
	}
	if d.State == DerivativeRevoked {
		return d, false, nil
	}
	if d.State != DerivativeActive {
		return Derivative{}, false, ErrInvalidRequest
	}
	d.State = DerivativeRevoked
	d.RevokedAt = at
	return d, true, nil
}

// SelectableForTraining reports whether one derivative may be
// selected right now: active receipt plus a currently granted consent
// on current terms covering its capture. Denial is a plain false:
// revoked receipts, revoked/outdated consent and absent consent all
// deny without error, so restore replay and worker commits share one
// filter. Owner mismatch is an error (no oracle).
func SelectableForTraining(d Derivative, consent *Consent, currentVersion, currentDigest string) bool {
	if d.State != DerivativeActive {
		return false
	}
	if consent == nil || consent.State != StateGranted {
		return false
	}
	if consent.ContributorID != d.ContributorID || consent.ContributorRef != d.ContributorRef {
		return false
	}
	if consent.TermsVersion != currentVersion || consent.TermsDigest != currentDigest {
		return false
	}
	if !d.CapturedAt.After(consent.GrantedAt) && !d.CapturedAt.Equal(consent.GrantedAt) {
		return false
	}
	return true
}
