package adapters

import (
	"context"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/privacy"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

var _ app.RevokedInventory = (*PromotionStore)(nil)

func (s *PromotionStore) ClaimPurge(ctx context.Context, now time.Time) (app.PurgeClaim, bool, error) {
	token, err := jobs.NewUUIDv4()
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	nonce, err := mustUUID(token)
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	row, err := db.New(s.pool).ClaimTrainingPurge(ctx, db.ClaimTrainingPurgeParams{Nonce: nonce, At: pgTime(now), LeaseUntil: pgTime(now.Add(time.Minute))})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.PurgeClaim{}, false, nil
	}
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	return app.PurgeClaim{Derivative: rowToDerivative(row), Token: token}, true, nil
}
func (s *PromotionStore) CompletePurge(ctx context.Context, c app.PurgeClaim, now time.Time) error {
	nonce, err := mustUUID(c.Token)
	if err != nil {
		return err
	}
	if c.Owner != "" {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		q := db.New(tx)
		if err := q.LockTrainingOwner(ctx, c.Owner); err != nil {
			return err
		}
		row, err := q.GetDerivative(ctx, db.GetDerivativeParams{ImageID: c.Derivative.ImageID, ConsentRevision: int64(c.Derivative.ConsentRevision)})
		if err != nil {
			return err
		}
		if row.ContributorID != c.Owner || row.ObjectKey != c.Derivative.ObjectKey {
			return domain.ErrConflict
		}
		n, err := q.CompleteTrainingPurge(ctx, db.CompleteTrainingPurgeParams{ImageID: row.ImageID, ConsentRevision: row.ConsentRevision, Nonce: nonce, At: pgTime(now)})
		if err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrBadState
		}
		if err := q.AdvanceTrainingErasureCursor(ctx, db.AdvanceTrainingErasureCursorParams{ContributorID: c.Owner, ImageID: row.ImageID, ConsentRevision: row.ConsentRevision, At: pgTime(now)}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	n, err := db.New(s.pool).CompleteTrainingPurge(ctx, db.CompleteTrainingPurgeParams{ImageID: c.Derivative.ImageID, ConsentRevision: int64(c.Derivative.ConsentRevision), Nonce: nonce, At: pgTime(now)})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrBadState
	}
	return nil
}

// ClaimOwnerPurge checks every receipt in owner/key order, including previously
// PURGED rows on a restore replay. Only confirmed absence advances the cursor.
func (s *PromotionStore) ClaimOwnerPurge(ctx context.Context, owner string, now time.Time) (app.PurgeClaim, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	if err := q.LockTrainingOwner(ctx, owner); err != nil {
		return app.PurgeClaim{}, false, err
	}
	row, err := q.NextTrainingErasureCopy(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.PurgeClaim{}, false, nil
	}
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	token, err := jobs.NewUUIDv4()
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	nonce, err := mustUUID(token)
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	n, err := q.ClaimTrainingErasureCopy(ctx, db.ClaimTrainingErasureCopyParams{ImageID: row.ImageID, ConsentRevision: row.ConsentRevision, ContributorID: owner, Nonce: nonce, At: pgTime(now), LeaseUntil: pgTime(now.Add(time.Minute))})
	if err != nil {
		return app.PurgeClaim{}, false, err
	}
	if n != 1 {
		return app.PurgeClaim{}, false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return app.PurgeClaim{}, false, err
	}
	return app.PurgeClaim{Derivative: rowToDerivative(row), Owner: owner, Token: token, WasPurged: row.OutputState == domain.OutputPurged}, true, nil
}
func (s *PromotionStore) HasRemainingErasure(ctx context.Context, owner string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	if err := q.LockTrainingOwner(ctx, owner); err != nil {
		return false, err
	}
	_, err = q.NextTrainingErasureCopy(ctx, owner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	pending := err == nil
	if !pending {
		if err := q.DeleteTrainingErasureCursor(ctx, owner); err != nil {
			return false, err
		}
	}
	return pending, tx.Commit(ctx)
}
