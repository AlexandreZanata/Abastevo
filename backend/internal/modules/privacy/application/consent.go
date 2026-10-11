package application

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Training-consent server contract (CS09A, B-BR-CS06): owner-only
// grant/revoke/status plus the promotion gate. Consent is a per-owner
// singleton ordered by revision: the revision is the operation
// identity (offline queue ordering + lost-response convergence), so no
// separate client submission key is stored — unlike the multi-row
// export/deletion intents. Persistence rows, signed HTTP, derivative
// inventory purge and the 24 h physical-deletion proof land in later
// CS09/CS10 tasks; every layer must enforce this same contract.

// TermsProvider is the owned server terms source: the current
// version/digest pair a fresh grant must match. Rotation invalidates
// old grants until the owner re-grants explicitly.
type TermsProvider interface {
	CurrentVersion() string
	CurrentDigest() string
}

// ConsentStore is the owned persistence port for the consent
// singleton: one row per contributor with a revision guard. Get
// reports ErrNotFound when absent (default OFF); Put enforces the
// same ordering as the domain so racing writers converge instead of
// resurrecting stale decisions.
type ConsentStore interface {
	Get(ctx context.Context, contributorID string) (domain.Consent, error)
	Put(ctx context.Context, c domain.Consent) (stored bool, replayed bool, err error)
}

// ConsentPorts wires consent decisions through the server clock,
// owner attribution, the guarded store and current terms.
type ConsentPorts struct {
	Clock       func() time.Time
	Attribution func(ctx context.Context, contributorID string) (string, error)
	Store       ConsentStore
	Terms       TermsProvider
}

// GrantDTO carries one explicit ON decision for future disclosed
// images. Revision must move past the stored one; terms must match
// the server's current pair.
type GrantDTO struct {
	TermsVersion string
	TermsDigest  string
	Revision     uint64
}

// RevokeDTO carries one explicit OFF decision. Revocation atomically
// stops selection and requests deletion of owned training assets.
type RevokeDTO struct {
	Revision uint64
}

// ImageSelectionDTO carries one promotion-gate check. Owner identity
// is server-resolved from the caller, never body-supplied.
type ImageSelectionDTO struct {
	ImageID         string
	CapturedAt      time.Time
	PerImageAllowed bool
}

// ConsentResult is the safe acknowledgment: the decision identity,
// whether the call converged, and — on a fresh revocation only —
// whether owned training assets must now be purged (CS10 owns the
// purge; repeated OFF converges without a duplicate signal).
type ConsentResult struct {
	State                     string
	Revision                  uint64
	TermsVersion              string
	DecidedAt                 time.Time
	Replayed                  bool
	DeleteOwnedTrainingAssets bool
}

// ConsentView is the owner-safe read view. Absent consent reports
// Granted=false with empty terms (default OFF, not a stored state).
// NeedsReGrant tells Settings to prompt a fresh explicit grant after
// a terms rotation.
type ConsentView struct {
	Granted      bool
	State        string
	TermsVersion string
	Revision     uint64
	DecidedAt    time.Time
	NeedsReGrant bool
}

func consentNow(p ConsentPorts) time.Time {
	if p.Clock != nil {
		return p.Clock()
	}
	return time.Now()
}

// Grant records one explicit ON decision for the caller on the
// current terms. Absent consent starts at revision >= 1; stale and
// outdated grants refuse; identical retries converge.
func GrantConsent(ctx context.Context, p ConsentPorts, caller Caller, dto GrantDTO) (ConsentResult, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return ConsentResult{}, ErrUnauthorized
	}
	token, err := p.Attribution(ctx, caller.ContributorID)
	if err != nil {
		return ConsentResult{}, err
	}
	now := consentNow(p)
	stored, err := p.Store.Get(ctx, caller.ContributorID)
	var current *domain.Consent
	if err == nil {
		current = &stored
	} else if !isNotFound(err) {
		return ConsentResult{}, err
	}
	next, receipt, err := domain.GrantConsent(current, domain.ConsentParams{
		ContributorID: caller.ContributorID, ContributorRef: token,
		TermsVersion: strings.TrimSpace(dto.TermsVersion), TermsDigest: strings.TrimSpace(dto.TermsDigest),
		Revision: dto.Revision, DecidedAt: now,
	}, p.Terms.CurrentVersion(), p.Terms.CurrentDigest())
	if err != nil {
		return ConsentResult{}, err
	}
	if receipt.Replayed {
		return ConsentResult{
			State: next.State, Revision: next.Revision,
			TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt, Replayed: true,
		}, nil
	}
	if _, replayed, err := p.Store.Put(ctx, next); err != nil {
		return ConsentResult{}, err
	} else if replayed {
		return ConsentResult{
			State: next.State, Revision: next.Revision,
			TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt, Replayed: true,
		}, nil
	}
	return ConsentResult{
		State: next.State, Revision: next.Revision,
		TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt,
	}, nil
}

// Revoke records one explicit OFF decision for the caller: selection
// stops at this receipt and owned training-asset deletion is
// requested. Revoking absent consent establishes the baseline;
// repeating the stored revocation converges without a duplicate
// purge signal; stale revisions refuse.
func RevokeConsent(ctx context.Context, p ConsentPorts, caller Caller, dto RevokeDTO) (ConsentResult, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return ConsentResult{}, ErrUnauthorized
	}
	token, err := p.Attribution(ctx, caller.ContributorID)
	if err != nil {
		return ConsentResult{}, err
	}
	now := consentNow(p)
	stored, err := p.Store.Get(ctx, caller.ContributorID)
	var current *domain.Consent
	if err == nil {
		current = &stored
	} else if !isNotFound(err) {
		return ConsentResult{}, err
	}
	next, receipt, err := domain.RevokeConsent(current, caller.ContributorID, token, dto.Revision, now)
	if err != nil {
		return ConsentResult{}, err
	}
	if receipt.Replayed {
		return ConsentResult{
			State: next.State, Revision: next.Revision,
			TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt, Replayed: true,
		}, nil
	}
	if _, replayed, err := p.Store.Put(ctx, next); err != nil {
		return ConsentResult{}, err
	} else if replayed {
		return ConsentResult{
			State: next.State, Revision: next.Revision,
			TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt, Replayed: true,
		}, nil
	}
	return ConsentResult{
		State: next.State, Revision: next.Revision,
		TermsVersion: next.TermsVersion, DecidedAt: next.DecidedAt,
		DeleteOwnedTrainingAssets: true,
	}, nil
}

// Status returns the caller's own consent view. Callers only ever
// address their own row, so there is no cross-owner oracle: absent
// consent is default OFF.
func ReadConsent(ctx context.Context, p ConsentPorts, caller Caller) (ConsentView, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return ConsentView{}, ErrUnauthorized
	}
	stored, err := p.Store.Get(ctx, caller.ContributorID)
	if err != nil {
		if isNotFound(err) {
			return ConsentView{}, nil
		}
		return ConsentView{}, err
	}
	out := ConsentView{
		State: stored.State, TermsVersion: stored.TermsVersion,
		Revision: stored.Revision, DecidedAt: stored.DecidedAt,
	}
	if stored.State == domain.StateGranted &&
		stored.TermsVersion == p.Terms.CurrentVersion() &&
		stored.TermsDigest == p.Terms.CurrentDigest() {
		out.Granted = true
		return out, nil
	}
	if stored.State == domain.StateGranted {
		out.NeedsReGrant = true
	}
	return out, nil
}

// CanSelect is the promotion gate (CS10 consumes it): whether one
// image may enter the private training derivative right now. Denial
// is a normal (false, nil) outcome; only malformed input and store
// failures are errors.
func CanSelectTraining(ctx context.Context, p ConsentPorts, caller Caller, dto ImageSelectionDTO) (bool, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return false, ErrUnauthorized
	}
	token, err := p.Attribution(ctx, caller.ContributorID)
	if err != nil {
		return false, err
	}
	stored, err := p.Store.Get(ctx, caller.ContributorID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return domain.AuthorizesImage(&stored, domain.ImageSelection{
		ImageID: dto.ImageID, ContributorID: caller.ContributorID, ContributorRef: token,
		CapturedAt: dto.CapturedAt, PerImageAllowed: dto.PerImageAllowed,
	}, p.Terms.CurrentVersion(), p.Terms.CurrentDigest())
}

func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
