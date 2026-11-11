// Package adapters persists only company-session verifiers. Account/family
// joins are explicitly listed in sqlc for current source authority.
package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/companysession"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct{ Pool *pgxpool.Pool }

var _ app.Store = (*Store)(nil)

func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}
func rollback(ctx context.Context, tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(cleanup)
}
func (s *Store) Create(ctx context.Context, row app.Row, proof app.Proof, oldLookup string, now time.Time) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(ctx, tx)
	queries := q.New(tx)
	if _, err := queries.LockAccount(ctx, row.AccountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return domain.ErrUnavailable
	}
	if _, err := queries.LockProof(ctx, q.LockProofParams{FamilyID: row.FamilyID, AccountID: row.AccountID, AccessHash: proof.AccessHash, AccessExpires: stamp(proof.AccessExpiresAt), Now: stamp(now)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return domain.ErrUnavailable
	}
	if oldLookup != "" {
		if err := queries.RevokeLookup(ctx, q.RevokeLookupParams{TokenLookup: oldLookup, AccountID: row.AccountID, Now: stamp(now)}); err != nil {
			return domain.ErrUnavailable
		}
	}
	if err := queries.PruneAccount(ctx, q.PruneAccountParams{AccountID: row.AccountID, Now: stamp(now), IdleCutoff: stamp(now.Add(-domain.IdleTimeout))}); err != nil {
		return domain.ErrUnavailable
	}
	count, err := queries.CountAccount(ctx, row.AccountID)
	if err != nil {
		return domain.ErrUnavailable
	}
	if count >= domain.MaxLiveSessions {
		return domain.ErrRateLimited
	}
	if err := queries.InsertSession(ctx, q.InsertSessionParams{ID: row.ID, AccountID: row.AccountID, FamilyID: row.FamilyID, TokenLookup: row.Lookup,
		TokenSalt: row.TokenSalt, TokenHash: row.TokenHash, CsrfSalt: row.CSRFSalt, CsrfHash: row.CSRFHash, AuthenticatedAt: stamp(row.AuthenticatedAt),
		CreatedAt: stamp(row.CreatedAt), LastSeenAt: stamp(row.LastSeenAt), ExpiresAt: stamp(row.ExpiresAt)}); err != nil {
		return domain.ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func (s *Store) Authenticate(ctx context.Context, lookup string, now time.Time, touch bool, verify func(app.Row) error) (app.Row, error) {
	if s == nil || s.Pool == nil {
		return app.Row{}, domain.ErrUnavailable
	}
	if !domain.ValidHash(lookup) || verify == nil {
		return app.Row{}, domain.ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.Row{}, domain.ErrUnavailable
	}
	defer rollback(ctx, tx)
	queries := q.New(tx)
	row, err := queries.LockSession(ctx, q.LockSessionParams{TokenLookup: lookup, Now: stamp(now), IdleCutoff: stamp(now.Add(-domain.IdleTimeout))})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.Row{}, domain.ErrUnauthorized
		}
		return app.Row{}, domain.ErrUnavailable
	}
	result := app.Row{ID: row.SID, AccountID: row.SAccountID, FamilyID: row.SFamilyID, Lookup: row.TokenLookup, TokenSalt: row.TokenSalt,
		TokenHash: row.TokenHash, CSRFSalt: row.CsrfSalt, CSRFHash: row.CsrfHash, AuthenticatedAt: row.AuthenticatedAt.Time,
		CreatedAt: row.CreatedAt.Time, LastSeenAt: row.LastSeenAt.Time, ExpiresAt: row.ExpiresAt.Time}
	if err := verify(result); err != nil {
		return app.Row{}, err
	}
	if touch {
		n, err := queries.TouchSession(ctx, q.TouchSessionParams{ID: result.ID, Now: stamp(now), IdleCutoff: stamp(now.Add(-domain.IdleTimeout))})
		if err != nil {
			return app.Row{}, domain.ErrUnavailable
		}
		if n != 1 {
			return app.Row{}, domain.ErrUnauthorized
		}
		result.LastSeenAt = now
	}
	if err := tx.Commit(ctx); err != nil {
		return app.Row{}, domain.ErrUnavailable
	}
	return result, nil
}
func (s *Store) Revoke(ctx context.Context, id, lookup string, now time.Time) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if !domain.ValidID(id) || !domain.ValidHash(lookup) {
		return domain.ErrInvalid
	}
	if err := q.New(s.Pool).RevokeSession(ctx, q.RevokeSessionParams{ID: id, TokenLookup: lookup, Now: stamp(now)}); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

// PurgeExpired is wired into the existing retention worker, also while the
// company surface is disabled. It never removes a live session.
func (s *Store) PurgeExpired(ctx context.Context, now time.Time, batch int) (int64, time.Time, error) {
	if s == nil || s.Pool == nil {
		return 0, time.Time{}, domain.ErrUnavailable
	}
	if batch < 1 || batch > 1000 {
		return 0, time.Time{}, domain.ErrInvalid
	}
	oldest, err := q.New(s.Pool).OldestExpired(ctx, q.OldestExpiredParams{Now: stamp(now), IdleCutoff: stamp(now.Add(-domain.IdleTimeout))})
	if err != nil {
		return 0, time.Time{}, domain.ErrUnavailable
	}
	var at time.Time
	if oldest.Valid {
		at = oldest.Time
	}
	n, err := q.New(s.Pool).PurgeExpired(ctx, q.PurgeExpiredParams{Now: stamp(now), IdleCutoff: stamp(now.Add(-domain.IdleTimeout)), BatchSize: int32(batch)})
	if err != nil {
		return 0, at, domain.ErrUnavailable
	}
	return n, at, nil
}
