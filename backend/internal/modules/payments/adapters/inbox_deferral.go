package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/payments"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// DeferInbox keeps one still-pending verified event and its next wake-up in
// the same transaction. The owner supplies the already bound term boundary.
func (s *Store) DeferInbox(ctx context.Context, event InboxInput, until time.Time) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if event.Provider.Validate() != nil || event.Environment.Validate() != nil ||
		!validText(event.Merchant) || !validText(event.EventID) || !digestOK(event.PayloadHash) ||
		until.IsZero() || until.Year() < 1 || until.Year() > 9999 {
		return domain.ErrInvalid
	}
	until = until.UTC().Truncate(time.Microsecond)
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		changed, err := q.DeferPaymentInbox(ctx, db.DeferPaymentInboxParams{
			Provider: string(event.Provider), Environment: string(event.Environment), Merchant: event.Merchant,
			EventID: event.EventID, PayloadHash: event.PayloadHash, DeferredUntil: stamp(until),
		})
		if err != nil {
			return err
		}
		if changed != 1 {
			return domain.ErrStale
		}
		payload, _ := json.Marshal(map[string]any{"version": 1, "event_id": event.EventID})
		binding, _ := json.Marshal([5]string{string(event.Provider), string(event.Environment), event.Merchant, event.EventID, until.Format(time.RFC3339Nano)})
		key := sha256.Sum256(binding)
		if _, err := jobs.Enqueue(ctx, tx, "payment-inbox", payload, "payment-inbox-deferred:"+hex.EncodeToString(key[:]), 5, until); err != nil {
			return domain.ErrUnavailable
		}
		return nil
	})
	if err == nil || errors.Is(err, domain.ErrStale) {
		return err
	}
	return dbError(err)
}
