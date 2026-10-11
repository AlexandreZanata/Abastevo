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
	n, err := db.New(s.pool).CompleteTrainingPurge(ctx, db.CompleteTrainingPurgeParams{ImageID: c.Derivative.ImageID, ConsentRevision: int64(c.Derivative.ConsentRevision), Nonce: nonce, At: pgTime(now)})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrBadState
	}
	return nil
}
