package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/account"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SaveProviderGrant stores one sealed provider refresh token (P13-T05).
// The account row is share-locked first, so the save serializes with
// DeleteAccount's status update: before deletion the grant is upserted
// (replacing the previous grant of the same subject) and deletion later
// queues it; after deletion it goes straight to the revocation queue.
func (s *PGStore) SaveProviderGrant(ctx context.Context, g domain.ProviderGrant) error {
	if g.Provider != "apple" || g.Subject == "" || len(g.Sealed) == 0 {
		return domain.ErrProviderProofMalformed
	}
	id, err := mustUUID(g.ID)
	if err != nil {
		return err
	}
	accountID, err := mustUUID(g.AccountID)
	if err != nil {
		return domain.ErrAccountNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := account.New(tx)
	status, err := q.LockAccountForGrant(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAccountNotFound
		}
		return err
	}
	if status == domain.StatusDeleted {
		if err := q.InsertGrantRevocation(ctx, account.InsertGrantRevocationParams{
			ID: id, Provider: g.Provider, SealedToken: g.Sealed, At: stamp(g.CapturedAt),
		}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := q.UpsertProviderGrant(ctx, account.UpsertProviderGrantParams{
		ID: id, AccountID: accountID, Provider: g.Provider, Subject: g.Subject,
		SealedToken: g.Sealed, CapturedAt: stamp(g.CapturedAt),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ClaimGrantRevocation leases the oldest due revocation for
// domain.GrantRevocationLeaseSeconds; concurrent workers skip locked
// rows and never share a lease.
func (s *PGStore) ClaimGrantRevocation(ctx context.Context, nonce string, nowUnix int64) (domain.GrantRevocation, bool, error) {
	n, err := mustUUID(nonce)
	if err != nil {
		return domain.GrantRevocation{}, false, err
	}
	row, err := account.New(s.pool).ClaimGrantRevocation(ctx, account.ClaimGrantRevocationParams{
		Nonce: n, At: stamp(nowUnix), LeaseUntil: stamp(nowUnix + domain.GrantRevocationLeaseSeconds),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GrantRevocation{}, false, nil
	}
	if err != nil {
		return domain.GrantRevocation{}, false, err
	}
	return domain.GrantRevocation{
		ID:          uuidString(row.ID),
		Provider:    row.Provider,
		Sealed:      row.SealedToken,
		RequestedAt: unix(row.RequestedAt),
		Attempts:    int(row.Attempts),
		LeaseNonce:  nonce,
	}, true, nil
}

// FinishGrantRevocation settles a leased revocation under its lease
// nonce: revoked and abandoned rows are deleted (the sealed token goes
// with them), retries back off. A lease lost to another worker is a
// no-op.
func (s *PGStore) FinishGrantRevocation(ctx context.Context, rev domain.GrantRevocation, outcome application.GrantOutcome, nowUnix int64) error {
	id, err := mustUUID(rev.ID)
	if err != nil {
		return err
	}
	nonce, err := mustUUID(rev.LeaseNonce)
	if err != nil {
		return err
	}
	q := account.New(s.pool)
	if outcome == application.GrantRetry {
		_, err = q.RetryGrantRevocation(ctx, account.RetryGrantRevocationParams{
			ID: id, Nonce: nonce, NextAt: stamp(nowUnix + domain.GrantRevocationBackoff(rev.Attempts+1)),
		})
		return err
	}
	_, err = q.DeleteGrantRevocation(ctx, account.DeleteGrantRevocationParams{ID: id, Nonce: nonce})
	return err
}
