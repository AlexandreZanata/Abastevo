package adapters

import (
	"context"
	"errors"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/jackc/pgx/v5"
)

func (s *PGStore) ClaimErasure(ctx context.Context, nonce string, now int64) (application.ErasureClaim, bool, error) {
	n, err := mustUUID(nonce)
	if err != nil {
		return application.ErasureClaim{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.ErasureClaim{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	id, err := q.ClaimAccountErasure(ctx, account.ClaimAccountErasureParams{Nonce: n, At: stamp(now), LeaseUntil: stamp(now + 120)})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErasureClaim{}, false, nil
	}
	if err != nil {
		return application.ErasureClaim{}, false, err
	}
	contributors, err := q.PendingAccountErasureContributor(ctx, id)
	if err != nil {
		return application.ErasureClaim{}, false, err
	}
	claim := application.ErasureClaim{AccountID: uuidString(id), Nonce: nonce}
	if len(contributors) > 0 {
		claim.ContributorID = contributors[0]
	}
	if err := tx.Commit(ctx); err != nil {
		return application.ErasureClaim{}, false, err
	}
	return claim, true, nil
}

func (s *PGStore) FinishErasure(ctx context.Context, claim application.ErasureClaim, success bool, now int64) error {
	id, err := mustUUID(claim.AccountID)
	if err != nil {
		return err
	}
	nonce, err := mustUUID(claim.Nonce)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	if _, err := q.LockAccountErasureLease(ctx, account.LockAccountErasureLeaseParams{AccountID: id, Nonce: nonce, At: stamp(now)}); err != nil {
		return err
	}
	if success && claim.ContributorID != "" {
		if err := q.CompleteAccountErasureContributor(ctx, account.CompleteAccountErasureContributorParams{AccountID: id, ContributorID: claim.ContributorID, At: stamp(now)}); err != nil {
			return err
		}
	}
	next := now
	if !success {
		next += 60
	}
	if err := q.FinishAccountErasure(ctx, account.FinishAccountErasureParams{AccountID: id, Nonce: nonce, At: stamp(now), NextAt: stamp(next), Succeeded: success}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
