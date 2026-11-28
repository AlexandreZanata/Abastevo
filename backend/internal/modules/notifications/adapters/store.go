// Package adapters persists account notifications on the owned generated
// queries (db/queries/notifications).
package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/notifications"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

// Store implements application.Inbox and application.Notifier.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires the owned generated queries to a pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var (
	_ application.Inbox    = (*Store)(nil)
	_ application.Notifier = (*Store)(nil)
)

var errNoTx = errors.New("notifications: producer transaction required")

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func uuidOf(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if !domain.IsUUID(s) {
		return id, domain.ErrInvalid
	}
	if err := id.Scan(s); err != nil {
		return id, domain.ErrInvalid
	}
	return id, nil
}

func uuidText(id pgtype.UUID) string {
	h := hex.EncodeToString(id.Bytes[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// Notify inserts the event in the producer's transaction and purges a
// bounded batch of expired rows. A repeated (account, event key) and an
// account that is not active insert nothing.
func (s *Store) Notify(ctx context.Context, tx pgx.Tx, e domain.Event) error {
	if tx == nil {
		return errNoTx
	}
	account, err := uuidOf(e.AccountID)
	if err != nil {
		return err
	}
	text, err := newID()
	if err != nil {
		return err
	}
	id, err := uuidOf(text)
	if err != nil {
		return err
	}
	queries := q.New(tx)
	if _, err := queries.PurgeExpiredNotifications(ctx, q.PurgeExpiredNotificationsParams{Now: ts(e.CreatedAt), Batch: domain.PurgeBatch}); err != nil {
		return err
	}
	_, err = queries.InsertNotification(ctx, q.InsertNotificationParams{
		ID: id, AccountID: account, EventKey: e.EventKey, Kind: e.Kind, Payload: e.Payload,
		CreatedAt: ts(e.CreatedAt), ExpiresAt: ts(e.ExpiresAt),
	})
	return err
}

// Page reads one page and the unread count from a single read-only snapshot.
func (s *Store) Page(ctx context.Context, accountID string, now time.Time, before *domain.Position, limit int) ([]application.Row, int64, error) {
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, 0, err
	}
	params := q.ListNotificationsParams{AccountID: account, Now: ts(now), PageLimit: int32(limit)}
	if before != nil {
		beforeID, err := uuidOf(before.ID)
		if err != nil {
			return nil, 0, err
		}
		params.HasCursor, params.BeforeCreatedAt, params.BeforeID = true, ts(before.CreatedAt), beforeID
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := q.New(tx)
	got, err := queries.ListNotifications(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	unread, err := queries.CountUnreadNotifications(ctx, q.CountUnreadNotificationsParams{AccountID: account, Now: ts(now)})
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	out := make([]application.Row, 0, len(got))
	for _, r := range got {
		out = append(out, application.Row{
			ID: uuidText(r.ID), Kind: r.Kind, Payload: r.Payload,
			CreatedAt: r.CreatedAt.Time.UTC(), Read: r.ReadAt.Valid,
		})
	}
	return out, unread, nil
}

// MarkRead marks the caller's selected unexpired rows read, purging a
// bounded batch of expired rows in the same transaction.
func (s *Store) MarkRead(ctx context.Context, accountID string, sel domain.Selection, now time.Time) (int64, error) {
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	ids := make([]pgtype.UUID, 0, len(sel.IDs))
	for _, raw := range sel.IDs {
		id, err := uuidOf(raw)
		if err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := q.New(tx)
	if _, err := queries.PurgeExpiredNotifications(ctx, q.PurgeExpiredNotificationsParams{Now: ts(now), Batch: domain.PurgeBatch}); err != nil {
		return 0, err
	}
	var marked int64
	if sel.All {
		marked, err = queries.MarkAllNotificationsRead(ctx, q.MarkAllNotificationsReadParams{AccountID: account, Now: ts(now)})
	} else {
		marked, err = queries.MarkNotificationsRead(ctx, q.MarkNotificationsReadParams{AccountID: account, Ids: ids, Now: ts(now)})
	}
	if err != nil {
		return 0, err
	}
	return marked, tx.Commit(ctx)
}

// EraseAccountBatch deletes at most limit notifications of a deleted
// account; done reports that none remain.
func (s *Store) EraseAccountBatch(ctx context.Context, accountID string, limit int) (bool, error) {
	if limit < 1 {
		limit = 500
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	n, err := q.New(s.pool).EraseAccountNotifications(ctx, q.EraseAccountNotificationsParams{AccountID: account, Batch: int32(limit)})
	if err != nil {
		return false, err
	}
	return n < int64(limit), nil
}
