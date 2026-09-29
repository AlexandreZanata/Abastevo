package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
)

// Runner executes business writes exactly once per idempotency key. The
// reservation, the business write and the outcome record share one
// transaction; concurrent duplicates block on the reserved row and then
// replay instead of executing. Only the body hash is stored, never the
// body, and outcomes expire after domain.IdempotencyTTL. Replayed bodies
// are semantically equal to the original; PostgreSQL jsonb normalization
// means byte-exact equality does not hold.
type Runner struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewRunner wires the owned generated queries to a pool.
func NewRunner(pool *pgxpool.Pool) *Runner {
	return &Runner{pool: pool, now: time.Now}
}

// Outcome is the executed or replayed safe result.
type Outcome struct {
	StatusCode int
	Response   []byte
	Replayed   bool
}

// Do runs fn once per key. Same key plus same body replays the stored
// outcome without executing; same key plus changed body conflicts; expired
// or aborted records re-execute. fn receives the business transaction and
// returns the safe outcome to store; fn must never include sensitive
// request content in its response.
func (r *Runner) Do(ctx context.Context, key domain.IdempotencyKey, body []byte, fn func(ctx context.Context, tx pgx.Tx) (int, []byte, error)) (Outcome, error) {
	hash := domain.HashBody(body)
	contributorID, err := mustUUID(key.ContributorID)
	if err != nil {
		return Outcome{}, err
	}
	for {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return Outcome{}, err
		}
		reserved, err := identity.New(tx).ReserveAttempt(ctx, identity.ReserveAttemptParams{
			ContributorID: contributorID,
			Method:        key.Method,
			Route:         key.Route,
			Key:           key.Key,
			RequestHash:   hash,
			ExpiresAt:     pgtype.Timestamptz{Time: r.now().Add(domain.IdempotencyTTL), Valid: true},
		})
		if err == nil {
			_ = reserved
			code, resp, err := fn(ctx, tx)
			if err != nil {
				_ = tx.Rollback(ctx)
				return Outcome{}, err
			}
			if err := complete(ctx, tx, key, contributorID, code, resp); err != nil {
				_ = tx.Rollback(ctx)
				return Outcome{}, err
			}
			if err := tx.Commit(ctx); err != nil {
				return Outcome{}, err
			}
			return Outcome{StatusCode: code, Response: resp}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			return Outcome{}, err
		}
		_ = tx.Rollback(ctx)
		// A record exists: lock it (waiting out any in-flight winner),
		// then decide on what survived.
		lockTx, err := r.pool.Begin(ctx)
		if err != nil {
			return Outcome{}, err
		}
		stored, err := identity.New(lockTx).LockAttempt(ctx, identity.LockAttemptParams{
			ContributorID: contributorID,
			Method:        key.Method,
			Route:         key.Route,
			Key:           key.Key,
		})
		if err != nil {
			_ = lockTx.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // Winner aborted; reserve again.
			}
			return Outcome{}, err
		}
		_ = lockTx.Rollback(ctx)
		completed := stored.ResponseCode.Valid
		expired := !stored.ExpiresAt.Time.After(r.now())
		switch application.Decide(true, completed, expired, stored.RequestHash == hash) {
		case application.ActionReplay:
			return Outcome{
				StatusCode: int(stored.ResponseCode.Int32),
				Response:   stored.ResponseJson,
				Replayed:   true,
			}, nil
		case application.ActionConflict:
			return Outcome{}, domain.ErrIdempotentConflict
		default:
			// Expired or aborted: delete and reserve again.
			if err := identity.New(r.pool).DeleteAttempt(ctx, identity.DeleteAttemptParams{
				ContributorID: contributorID,
				Method:        key.Method,
				Route:         key.Route,
				Key:           key.Key,
			}); err != nil {
				return Outcome{}, err
			}
		}
	}
}

func complete(ctx context.Context, tx pgx.Tx, key domain.IdempotencyKey, contributor pgtype.UUID, code int, resp []byte) error {
	return identity.New(tx).CompleteAttempt(ctx, identity.CompleteAttemptParams{
		ContributorID: contributor,
		Method:        key.Method,
		Route:         key.Route,
		Key:           key.Key,
		ResponseCode:  pgtype.Int4{Int32: int32(code), Valid: true},
		ResponseJson:  resp,
	})
}

// PurgeExpiredAttempts deletes expired idempotency windows in bounded
// batches (P07-T05 retention): each pass removes at most batch rows,
// oldest first, and reports the total plus the oldest overdue expiry
// observed before purging (zero time when nothing was overdue).
// Expired outcomes are never replayed, so removal only frees space.
func (r *Runner) PurgeExpiredAttempts(ctx context.Context, now time.Time, batch int32) (purged int64, oldest time.Time, err error) {
	q := identity.New(r.pool)
	ts, oerr := q.OldestExpiredAttempt(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if oerr != nil && !errors.Is(oerr, pgx.ErrNoRows) {
		return 0, time.Time{}, oerr
	}
	oldest = ts.Time
	for {
		n, derr := q.PurgeExpiredAttempts(ctx, identity.PurgeExpiredAttemptsParams{
			Now: pgtype.Timestamptz{Time: now, Valid: true}, Batch: batch,
		})
		if derr != nil {
			return purged, oldest, derr
		}
		purged += n
		if n < int64(batch) {
			return purged, oldest, nil
		}
	}
}
