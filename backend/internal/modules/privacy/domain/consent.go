package domain

import (
	"errors"
	"strings"
	"time"
)

// Training consent state machine (CS09A, B-BR-CS06): optional,
// default-OFF, versioned server grant/revoke for the private training
// derivative defined in the P37-PC05 consent amendment. Absent consent
// denies selection; a grant covers only future per-image selections;
// revocation stops selection atomically and a stale offline grant can
// never undo it. Timestamps are server decisions, never client claims.
//
// Scope: pure domain only. Persistence (append-only receipt/inventory
// rows), signed HTTP, promotion worker and physical deletion land in
// later CS09/CS10 tasks; this file freezes the transition contract
// they must all enforce.
const (
	// TrainingPurpose is the single frozen consent purpose. No other
	// purpose string is accepted.
	TrainingPurpose = "TRAINING"
	// TrainingTermsV1 is the current server terms revision pinned by
	// the golden fixtures. Rotation requires a fresh explicit grant;
	// old revisions refuse with ErrTermsOutdated.
	TrainingTermsV1 = "training-terms-v1"
	// MaxTermsDigestChars bounds the recorded terms digest so one
	// consent row cannot become an unbounded blob.
	MaxTermsDigestChars = 128
	// MaxImageIDChars bounds the per-image scope reference.
	MaxImageIDChars = 128
)

// Consent states. There is no stored OFF state: an absent record is
// OFF (default deny). REVOKED is the explicit OFF with history that
// stale grants must never resurrect.
const (
	StateGranted = "GRANTED"
	StateRevoked = "REVOKED"
)

var (
	// ErrTermsOutdated marks a grant on a superseded terms
	// version/digest. Callers refuse selection until a fresh grant on
	// the current revision lands.
	ErrTermsOutdated = errors.New("privacy: training terms outdated")
	// ErrStaleConsent marks a decision whose revision does not move
	// past the stored one. Callers keep the stored state, so an old
	// offline ON can never undo a revocation.
	ErrStaleConsent = errors.New("privacy: stale consent revision")
)

// Consent is one owner's current training-consent decision:
// identifiers, frozen purpose/terms and outcome only — never pixels,
// GPS, keys or image bytes (B-BR-CS06, B-BR-011).
type Consent struct {
	ContributorID  string
	ContributorRef string
	Purpose        string
	TermsVersion   string
	TermsDigest    string
	State          string
	Revision       uint64
	GrantedAt      time.Time
	RevokedAt      time.Time
	DecidedAt      time.Time
}

// ConsentParams carries one grant decision. Revision is the
// per-owner monotonic operation identity: it orders offline-queued
// decisions and makes lost-response retries safe without a separate
// submission key (singletons need ordering, not multi-row idempotency
// keys like export/deletion intents).
type ConsentParams struct {
	ContributorID  string
	ContributorRef string
	TermsVersion   string
	TermsDigest    string
	Revision       uint64
	DecidedAt      time.Time
}

// ConsentReceipt is the minimal auditable outcome: identifiers and
// decision only, never payload.
type ConsentReceipt struct {
	ContributorID string
	State         string
	Revision      uint64
	TermsVersion  string
	DecidedAt     time.Time
	Replayed      bool
}

// ImageSelection is one per-image training-scope check: the candidate
// image reference, its original capture instant and whether the owner
// opted this image in at review. Owner fields are server-resolved;
// raw fixes, pixels and EXIF never travel here.
type ImageSelection struct {
	ImageID         string
	ContributorID   string
	ContributorRef  string
	CapturedAt      time.Time
	PerImageAllowed bool
}

// GrantConsent records one explicit ON decision on the current terms.
// Absent consent starts at revision >= 1. A stored record requires a
// strictly greater revision to change state; an equal revision with
// the same state and terms converges (replayed) — attempt instants
// are server stamps, not decision identity, so a lost-response retry
// under a live clock still converges; an equal revision with a
// divergent state/terms conflicts; a smaller revision is stale.
// Owner mismatch reads as not found so consent existence is never an
// ownership oracle.
func GrantConsent(current *Consent, p ConsentParams, currentVersion, currentDigest string) (Consent, ConsentReceipt, error) {
	if strings.TrimSpace(p.ContributorID) == "" ||
		strings.TrimSpace(p.ContributorRef) == "" ||
		strings.TrimSpace(p.TermsVersion) == "" ||
		strings.TrimSpace(p.TermsDigest) == "" ||
		len([]rune(p.TermsDigest)) > MaxTermsDigestChars ||
		p.Revision == 0 || p.DecidedAt.IsZero() {
		return Consent{}, ConsentReceipt{}, ErrInvalidRequest
	}
	if strings.TrimSpace(currentVersion) == "" || strings.TrimSpace(currentDigest) == "" {
		return Consent{}, ConsentReceipt{}, ErrInvalidRequest
	}
	if p.TermsVersion != currentVersion || p.TermsDigest != currentDigest {
		return Consent{}, ConsentReceipt{}, ErrTermsOutdated
	}
	next := Consent{
		ContributorID: strings.TrimSpace(p.ContributorID), ContributorRef: strings.TrimSpace(p.ContributorRef),
		Purpose: TrainingPurpose, TermsVersion: p.TermsVersion, TermsDigest: p.TermsDigest,
		State: StateGranted, Revision: p.Revision, GrantedAt: p.DecidedAt, DecidedAt: p.DecidedAt,
	}
	if current == nil {
		return next, ConsentReceipt{
			ContributorID: next.ContributorID, State: next.State,
			Revision: next.Revision, TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt,
		}, nil
	}
	if current.ContributorID != next.ContributorID || current.ContributorRef != next.ContributorRef {
		return Consent{}, ConsentReceipt{}, ErrNotFound
	}
	if p.Revision < current.Revision {
		return Consent{}, ConsentReceipt{}, ErrStaleConsent
	}
	if p.Revision == current.Revision {
		if current.State == StateGranted &&
			current.TermsVersion == p.TermsVersion && current.TermsDigest == p.TermsDigest {
			return *current, ConsentReceipt{
				ContributorID: current.ContributorID, State: current.State,
				Revision: current.Revision, TermsVersion: current.TermsVersion,
				DecidedAt: current.DecidedAt, Replayed: true,
			}, nil
		}
		if current.State == StateRevoked {
			return Consent{}, ConsentReceipt{}, ErrStaleConsent
		}
		return Consent{}, ConsentReceipt{}, ErrConflict
	}
	next.GrantedAt = p.DecidedAt
	return next, ConsentReceipt{
		ContributorID: next.ContributorID, State: next.State,
		Revision: next.Revision, TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt,
	}, nil
}

// RevokeConsent records one explicit OFF decision and signals owned
// training-asset deletion to the (CS10) purge path. Revoking absent
// consent establishes the REVOKED baseline so a later stale grant
// cannot slip through. Repeating the stored revocation converges;
// a smaller revision is stale; owner mismatch reads as not found.
func RevokeConsent(current *Consent, contributorID, contributorRef string, revision uint64, at time.Time) (Consent, ConsentReceipt, error) {
	contributorID = strings.TrimSpace(contributorID)
	contributorRef = strings.TrimSpace(contributorRef)
	if contributorID == "" || contributorRef == "" || revision == 0 || at.IsZero() {
		return Consent{}, ConsentReceipt{}, ErrInvalidRequest
	}
	if current == nil {
		next := Consent{
			ContributorID: contributorID, ContributorRef: contributorRef,
			Purpose: TrainingPurpose, State: StateRevoked,
			Revision: revision, RevokedAt: at, DecidedAt: at,
		}
		return next, ConsentReceipt{
			ContributorID: contributorID, State: StateRevoked,
			Revision: revision, DecidedAt: at,
		}, nil
	}
	if current.ContributorID != contributorID || current.ContributorRef != contributorRef {
		return Consent{}, ConsentReceipt{}, ErrNotFound
	}
	if revision < current.Revision {
		return Consent{}, ConsentReceipt{}, ErrStaleConsent
	}
	if revision == current.Revision {
		if current.State == StateRevoked {
			return *current, ConsentReceipt{
				ContributorID: current.ContributorID, State: current.State,
				Revision: current.Revision, TermsVersion: current.TermsVersion,
				DecidedAt: current.DecidedAt, Replayed: true,
			}, nil
		}
		return Consent{}, ConsentReceipt{}, ErrConflict
	}
	next := *current
	next.State = StateRevoked
	next.Revision = revision
	next.RevokedAt = at
	next.DecidedAt = at
	return next, ConsentReceipt{
		ContributorID: next.ContributorID, State: next.State,
		Revision: next.Revision, TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt,
	}, nil
}

// AuthorizesImage reports whether one image is currently selectable
// for the private training derivative. Absent, revoked or outdated
// consent denies (false, nil): denial is a normal outcome, not an
// error. Invalid image input and owner mismatch are errors. A grant
// never authorizes captures from before its own decision instant, so
// ON cannot retroactively claim old photos.
func AuthorizesImage(consent *Consent, img ImageSelection, currentVersion, currentDigest string) (bool, error) {
	if strings.TrimSpace(img.ImageID) == "" ||
		len([]rune(img.ImageID)) > MaxImageIDChars ||
		strings.TrimSpace(img.ContributorID) == "" ||
		strings.TrimSpace(img.ContributorRef) == "" ||
		img.CapturedAt.IsZero() {
		return false, ErrInvalidRequest
	}
	if consent == nil {
		return false, nil
	}
	if consent.ContributorID != strings.TrimSpace(img.ContributorID) ||
		consent.ContributorRef != strings.TrimSpace(img.ContributorRef) {
		return false, ErrNotFound
	}
	if consent.State != StateGranted {
		return false, nil
	}
	if consent.TermsVersion != currentVersion || consent.TermsDigest != currentDigest {
		return false, nil
	}
	if !img.PerImageAllowed {
		return false, nil
	}
	if img.CapturedAt.Before(consent.GrantedAt) {
		return false, nil
	}
	return true, nil
}
