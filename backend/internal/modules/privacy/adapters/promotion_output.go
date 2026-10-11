package adapters

import (
	"context"
	"errors"
	dbprivacy "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/privacy"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

func checkOutputIdentity(d domain.Derivative, r privacyapp.PromotionResult) error {
	if d.ContributorID != r.ContributorID || d.ObjectKey != r.ObjectKey || d.DerivativeSHA256 != r.DerivativeSHA256 {
		return domain.ErrConflict
	}
	return nil
}

// Caller holds the owner transaction lock, which consent updates also hold.
func outputConsent(ctx context.Context, q *dbprivacy.Queries, d domain.Derivative, version, digest string) error {
	row, err := q.GetTrainingConsent(ctx, d.ContributorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConsentRequired
	}
	if err != nil {
		return err
	}
	c := rowToConsent(row)
	if !domain.CoveredByConsent(d, &c, version, digest) {
		return domain.ErrConsentRequired
	}
	fence, err := q.GetTrainingRevocation(ctx, d.ContributorID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && d.ConsentRevision <= uint64(fence.Revision) {
		return domain.ErrConsentRequired
	}
	return nil
}
func (s *PromotionStore) BeginOutput(ctx context.Context, r privacyapp.PromotionResult, nonce string, now time.Time, version, digest string) error {
	n, err := mustUUID(nonce)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbprivacy.New(tx)
	if err := q.LockTrainingOwner(ctx, r.ContributorID); err != nil {
		return err
	}
	row, err := q.GetDerivative(ctx, dbprivacy.GetDerivativeParams{ImageID: r.ImageID, ConsentRevision: int64(r.ConsentRevision)})
	if err != nil {
		return err
	}
	d := rowToDerivative(row)
	if err := checkOutputIdentity(d, r); err != nil {
		return err
	}
	if err := outputConsent(ctx, q, d, version, digest); err != nil {
		return err
	}
	changed, err := q.ClaimTrainingOutput(ctx, dbprivacy.ClaimTrainingOutputParams{ImageID: r.ImageID, ConsentRevision: int64(r.ConsentRevision), Nonce: n, At: pgTime(now), LeaseUntil: pgTime(now.Add(time.Minute))})
	if err != nil {
		return err
	}
	if changed != 1 {
		return domain.ErrBadState
	}
	return tx.Commit(ctx)
}
func (s *PromotionStore) FinishOutput(ctx context.Context, r privacyapp.PromotionResult, nonce string, success bool, now time.Time, version, digest string) error {
	n, err := mustUUID(nonce)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbprivacy.New(tx)
	if err := q.LockTrainingOwner(ctx, r.ContributorID); err != nil {
		return err
	}
	row, err := q.GetDerivative(ctx, dbprivacy.GetDerivativeParams{ImageID: r.ImageID, ConsentRevision: int64(r.ConsentRevision)})
	if err != nil {
		return err
	}
	d := rowToDerivative(row)
	if err := checkOutputIdentity(d, r); err != nil {
		return err
	}
	if success {
		if err := outputConsent(ctx, q, d, version, digest); err != nil {
			return err
		}
	}
	changed, err := q.FinishTrainingOutput(ctx, dbprivacy.FinishTrainingOutputParams{ImageID: r.ImageID, ConsentRevision: int64(r.ConsentRevision), Nonce: n, Succeeded: success, At: pgTime(now)})
	if err != nil {
		return err
	}
	if changed != 1 {
		return domain.ErrBadState
	}
	return tx.Commit(ctx)
}
func (s *PromotionStore) PrepareOutputCleanup(ctx context.Context, r privacyapp.PromotionResult, nonce string, now time.Time) error {
	n, err := mustUUID(nonce)
	if err != nil {
		return err
	}
	d, err := s.Get(ctx, r.ImageID, r.ConsentRevision)
	if err != nil {
		return err
	}
	if err := checkOutputIdentity(d, r); err != nil {
		return err
	}
	changed, err := dbprivacy.New(s.pool).ClaimTrainingOutputCleanup(ctx, dbprivacy.ClaimTrainingOutputCleanupParams{ImageID: r.ImageID, ConsentRevision: int64(r.ConsentRevision), Nonce: n, LeaseUntil: pgTime(now.Add(15 * time.Second))})
	if err != nil {
		return err
	}
	if changed != 1 {
		return domain.ErrBadState
	}
	return nil
}

func (s *PromotionStore) ValidateOutput(ctx context.Context, r privacyapp.PromotionResult, version, digest string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbprivacy.New(tx)
	if err := q.LockTrainingOwner(ctx, r.ContributorID); err != nil {
		return "", err
	}
	row, err := q.GetDerivative(ctx, dbprivacy.GetDerivativeParams{ImageID: r.ImageID, ConsentRevision: int64(r.ConsentRevision)})
	if err != nil {
		return "", err
	}
	d := rowToDerivative(row)
	if err := checkOutputIdentity(d, r); err != nil {
		return "", err
	}
	if err := outputConsent(ctx, q, d, version, digest); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return d.OutputState, nil
}
