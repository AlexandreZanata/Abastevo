package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbprivacy "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/privacy"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// ConsentStore persists the training-consent singleton (CS09B): one
// row per contributor holding the current training-derivative
// decision. It implements the CS09A application.ConsentStore port, so
// the domain revision guard holds end to end: the upsert moves state
// only on strictly greater revisions, and the resolve path below
// converges identical retries, conflicts divergent ones and refuses
// stale ones. Reads are owner-scoped by construction: the row key is
// the server-resolved contributor ID, so one contributor can never
// observe another's consent.
type ConsentStore struct {
	pool *pgxpool.Pool
}

// NewConsentStore wires the owned consent queries to a pool.
func NewConsentStore(pool *pgxpool.Pool) *ConsentStore {
	return &ConsentStore{pool: pool}
}

var _ privacyapp.ConsentStore = (*ConsentStore)(nil)

func rowToConsent(row dbprivacy.PrivacyTrainingConsent) domain.Consent {
	return domain.Consent{
		ContributorID: row.ContributorID, ContributorRef: row.ContributorRef,
		Purpose: row.Purpose, TermsVersion: row.TermsVersion, TermsDigest: row.TermsDigest,
		State: row.State, Revision: uint64(row.Revision),
		GrantedAt: row.GrantedAt.Time, RevokedAt: row.RevokedAt.Time,
		DecidedAt: row.DecidedAt.Time,
	}
}

// Get returns one owner's current consent. Absent consent reads as
// not found (default OFF at the application layer), sharing one shape
// so consent existence is never an ownership oracle.
func (s *ConsentStore) Get(ctx context.Context, contributorID string) (domain.Consent, error) {
	row, err := dbprivacy.New(s.pool).GetTrainingConsent(ctx, contributorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Consent{}, domain.ErrNotFound
		}
		return domain.Consent{}, err
	}
	return rowToConsent(row), nil
}

// Put moves one owner's consent through the revision-guarded upsert.
// Fresh rows and strictly greater revisions store; a no-row upsert
// resolves against the stored row: identical retries converge with
// replayed=true, divergent same-revision writes conflict, and smaller
// revisions refuse as stale — so a racing stale offline ON can never
// undo a revocation.
func (s *ConsentStore) Put(ctx context.Context, c domain.Consent) (bool, bool, error) {
	row, err := dbprivacy.New(s.pool).UpsertTrainingConsent(ctx, dbprivacy.UpsertTrainingConsentParams{
		ContributorID: c.ContributorID, ContributorRef: c.ContributorRef,
		Purpose: c.Purpose, TermsVersion: c.TermsVersion, TermsDigest: c.TermsDigest,
		State: c.State, Revision: int64(c.Revision),
		GrantedAt: pgTime(c.GrantedAt), RevokedAt: pgTime(c.RevokedAt),
		DecidedAt: pgTime(c.DecidedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.resolveConsentConflict(ctx, c)
		}
		return false, false, err
	}
	_ = row
	return true, false, nil
}

// resolveConsentConflict compares a guarded-out write against the
// stored row. Identical retries converge on the stored row regardless
// of the race; divergent same-revision payloads conflict instead of
// forking the singleton; smaller revisions are stale.
func (s *ConsentStore) resolveConsentConflict(ctx context.Context, c domain.Consent) (bool, bool, error) {
	stored, err := s.Get(ctx, c.ContributorID)
	if err != nil {
		return false, false, err
	}
	if c.Revision < stored.Revision {
		return false, false, domain.ErrStaleConsent
	}
	if c.Revision == stored.Revision {
		if consentEqual(c, stored) {
			return false, true, nil
		}
		if stored.State == domain.StateRevoked && c.State == domain.StateGranted {
			return false, false, domain.ErrStaleConsent
		}
		return false, false, domain.ErrConflict
	}
	// Unreachable: the guarded upsert writes every strictly greater
	// revision, so a no-row write with a greater revision means the
	// row changed under us. Fail closed instead of forking history.
	return false, false, domain.ErrConflict
}

// consentEqual compares two consent rows field by field. Time
// instants compare exactly: writers stamp server time at millisecond
// precision or coarser, and retries carry the identical instant.
func consentEqual(a, b domain.Consent) bool {
	return a.ContributorID == b.ContributorID &&
		a.ContributorRef == b.ContributorRef &&
		a.Purpose == b.Purpose &&
		a.TermsVersion == b.TermsVersion &&
		a.TermsDigest == b.TermsDigest &&
		a.State == b.State &&
		a.Revision == b.Revision &&
		a.GrantedAt.Equal(b.GrantedAt) &&
		a.RevokedAt.Equal(b.RevokedAt) &&
		a.DecidedAt.Equal(b.DecidedAt)
}
