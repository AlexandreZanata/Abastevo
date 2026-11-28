// Package application holds the account notification use cases and the
// ports they own: the inbox store read by the owner API, and the Notifier
// that producer modules reach (through composition-root glue) inside the
// transaction of the event they describe (B-BR-NOTIF-02).
package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

// ErrUnavailable marks a store failure; the API answers 503.
var ErrUnavailable = errors.New("notifications: service unavailable")

// Row is one stored, unexpired notification of the caller.
type Row struct {
	ID        string
	Kind      string
	Payload   []byte
	CreatedAt time.Time
	Read      bool
}

// Inbox is the owner-scoped store. Every method filters by accountID, so
// another account's rows are never listed, counted or marked.
type Inbox interface {
	// Page returns at most limit unexpired rows newest first, strictly
	// after before when set, plus the unread count, from one snapshot.
	Page(ctx context.Context, accountID string, now time.Time, before *domain.Position, limit int) ([]Row, int64, error)
	// MarkRead marks the selection read and returns how many of the
	// caller's unexpired notifications it covers.
	MarkRead(ctx context.Context, accountID string, sel domain.Selection, now time.Time) (int64, error)
}

// Notifier inserts one notice in the producer's open transaction, so the
// notice commits or rolls back with the event. A repeated event key is a
// no-op, and accounts that are not active receive nothing.
type Notifier interface {
	Notify(ctx context.Context, tx pgx.Tx, e domain.Event) error
}

// Item is one listed notification on the wire.
type Item struct {
	NotificationID string          `json:"notification_id"`
	Kind           string          `json:"kind"`
	CreatedAt      string          `json:"created_at"`
	Read           bool            `json:"read"`
	Payload        json.RawMessage `json:"payload"`
}

// Page is one newest-first slice. Next is the position after the last
// item when more items exist.
type Page struct {
	Items  []Item
	Next   *domain.Position
	Unread int64
}
