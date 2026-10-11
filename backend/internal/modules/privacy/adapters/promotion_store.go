package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbprivacy "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/privacy"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// PromotionStore persists the training-derivative inventory (CS10B):
// one row per image plus consent revision. It implements the CS10A
// application.PromotionInventory port: insert-once puts converging
// identical re-proposals, owner-scoped active reads, and the guarded
// ACTIVE-to-REVOKED mark. No row removal exists: revoked receipts
// stay for the restore filter while physical bytes purge in CS10C.
type PromotionStore struct {
	pool *pgxpool.Pool
}

// NewPromotionStore wires the owned derivative queries to a pool.
func NewPromotionStore(pool *pgxpool.Pool) *PromotionStore {
	return &PromotionStore{pool: pool}
}

var _ privacyapp.PromotionInventory = (*PromotionStore)(nil)

func rowToDerivative(row dbprivacy.PrivacyTrainingDerivative) domain.Derivative {
	return domain.Derivative{
		ImageID: row.ImageID, ContributorID: row.ContributorID,
		ContributorRef: row.ContributorRef, SourceSHA256: row.SourceSha256,
		DerivativeSHA256: row.DerivativeSha256, CapturedAt: row.CapturedAt.Time,
		TermsVersion: row.TermsVersion, TermsDigest: row.TermsDigest,
		State: row.State, ConsentRevision: uint64(row.ConsentRevision),
		PromotedAt: row.PromotedAt.Time, RevokedAt: row.RevokedAt.Time,
	}
}

// Put inserts one derivative once. Identical re-proposals converge on
// the stored row with replayed=true; divergent hashes on the same
// natural key conflict instead of forking inventory.
func (s *PromotionStore) Put(ctx context.Context, d domain.Derivative) (bool, bool, error) {
	row, err := dbprivacy.New(s.pool).PutDerivative(ctx, dbprivacy.PutDerivativeParams{
		ImageID: d.ImageID, ConsentRevision: int64(d.ConsentRevision),
		ContributorID: d.ContributorID, ContributorRef: d.ContributorRef,
		SourceSha256: d.SourceSHA256, DerivativeSha256: d.DerivativeSHA256,
		CapturedAt: pgTime(d.CapturedAt), TermsVersion: d.TermsVersion,
		TermsDigest: d.TermsDigest, State: d.State,
		PromotedAt: pgTime(d.PromotedAt), RevokedAt: pgTime(d.RevokedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.resolveDerivativeConflict(ctx, d)
		}
		return false, false, err
	}
	_ = row
	return true, false, nil
}

// resolveDerivativeConflict compares a natural-key hit against the
// stored row. Same owner and hashes converge; anything else on the
// same key conflicts — inventory never forks and never leaks across
// owners through a shared image reference.
func (s *PromotionStore) resolveDerivativeConflict(ctx context.Context, d domain.Derivative) (bool, bool, error) {
	stored, err := dbprivacy.New(s.pool).GetDerivative(ctx, dbprivacy.GetDerivativeParams{
		ImageID: d.ImageID, ConsentRevision: int64(d.ConsentRevision),
	})
	if err != nil {
		return false, false, err
	}
	got := rowToDerivative(stored)
	if got.ContributorID != d.ContributorID || got.ContributorRef != d.ContributorRef ||
		got.SourceSHA256 != d.SourceSHA256 || got.DerivativeSHA256 != d.DerivativeSHA256 {
		return false, false, domain.ErrConflict
	}
	return false, true, nil
}

// ListActiveByOwner returns one owner's selectable derivatives for
// revocation and worker reads. The owner key scopes the read, so one
// contributor can never observe another's inventory.
func (s *PromotionStore) ListActiveByOwner(ctx context.Context, contributorID string) ([]domain.Derivative, error) {
	rows, err := dbprivacy.New(s.pool).ListActiveDerivativesByOwner(ctx, contributorID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDerivative(row))
	}
	return out, nil
}

// ListRevokedBefore returns REVOKED rows whose revocation is older
// than the deadline, oldest first, for the purge sweeper. Receipts
// stay in place after the sweep; only bytes go.
func (s *PromotionStore) ListRevokedBefore(ctx context.Context, before time.Time) ([]domain.Derivative, error) {
	rows, err := dbprivacy.New(s.pool).ListRevokedDerivativesBefore(ctx, pgTime(before))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDerivative(row))
	}
	return out, nil
}

// ListByOwner returns every owned derivative, active and tombstoned,
// for erasure. The owner key scopes the read like every other method
// here; receipts leave only through revocation and purge paths.
func (s *PromotionStore) ListByOwner(ctx context.Context, contributorID string) ([]domain.Derivative, error) {
	rows, err := dbprivacy.New(s.pool).ListDerivativesByOwner(ctx, contributorID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Derivative, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDerivative(row))
	}
	return out, nil
}

// MarkRevoked flips one row ACTIVE to REVOKED through the guarded
// transition. Repeating the mark converges silently; unknown rows
// read as not found.
func (s *PromotionStore) MarkRevoked(ctx context.Context, imageID string, revision uint64, at time.Time) error {
	_, err := dbprivacy.New(s.pool).MarkDerivativeRevoked(ctx, dbprivacy.MarkDerivativeRevokedParams{
		ImageID: imageID, ConsentRevision: int64(revision), RevokedAt: pgTime(at),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.resolveRevokeMark(ctx, imageID, revision)
		}
		return err
	}
	return nil
}

// resolveRevokeMark distinguishes a converged repeat (already REVOKED)
// from an unknown row after a guarded no-op.
func (s *PromotionStore) resolveRevokeMark(ctx context.Context, imageID string, revision uint64) error {
	row, err := dbprivacy.New(s.pool).GetDerivative(ctx, dbprivacy.GetDerivativeParams{
		ImageID: imageID, ConsentRevision: int64(revision),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if row.State == domain.DerivativeRevoked {
		return nil
	}
	return domain.ErrBadState
}
