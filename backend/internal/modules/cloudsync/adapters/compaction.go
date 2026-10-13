package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

// Cloud retention bounds (contracts/premium/policy-v1.json): 30 days for
// changes and receipts. Unknown proof never authorizes a purge: deleted or
// unknown owners fail closed with no deletes.
const cloudRetention = 30 * 24 * time.Hour

func (s *Store) Compact(ctx context.Context, accountID string, now time.Time) (int64, error) {
	if !domain.ValidID(accountID) || s == nil || s.Pool == nil {
		return 0, domain.ErrInvalid
	}
	if now.IsZero() {
		return 0, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	defer rollback(tx)
	query := q.New(tx)
	if _, err = query.LockOwner(ctx, uid(accountID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrUnauthorized
		}
		return 0, domain.ErrUnavailable
	}
	if err = query.EnsureState(ctx, uid(accountID)); err != nil {
		return 0, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(accountID))
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	cutoff := pgtype.Timestamptz{Time: now.Add(-cloudRetention), Valid: true}
	floor, err := query.CompactFloor(ctx, q.CompactFloorParams{AccountID: uid(accountID), CreatedAt: cutoff})
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	if floor < 0 || floor > state.Revision {
		return 0, domain.ErrUnavailable
	}
	if floor > state.CompactedThrough {
		floor, err = query.AdvanceFloor(ctx, q.AdvanceFloorParams{Floor: floor, AccountID: uid(accountID)})
		if err != nil {
			return 0, domain.ErrUnavailable
		}
	} else {
		floor = state.CompactedThrough
	}
	if _, err = query.DeleteOldChanges(ctx, q.DeleteOldChangesParams{AccountID: uid(accountID), Sequence: floor, CreatedAt: cutoff}); err != nil {
		return 0, domain.ErrUnavailable
	}
	if _, err = query.DeleteOldReceipts(ctx, q.DeleteOldReceiptsParams{AccountID: uid(accountID), CreatedAt: cutoff}); err != nil {
		return 0, domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, domain.ErrUnavailable
	}
	return floor, nil
}
