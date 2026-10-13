package adapters

import (
	"context"
	"errors"
	qsub "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/subscription"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) ForJob(ctx context.Context, digest string) (app.StoredToken, error) {
	if !digestOK(digest) || s == nil || s.Pool == nil {
		return app.StoredToken{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.StoredToken{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qsub.New(tx)
	p, err := q.GetPurchase(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.StoredToken{}, domain.ErrNotFound
	}
	if err != nil {
		return app.StoredToken{}, domain.ErrUnavailable
	}
	if p.Retired || p.State == string(domain.Revoked) || len(p.TokenCipher) == 0 {
		return app.StoredToken{}, domain.ErrNotFound
	}
	if err = s.authorize(ctx, q, app.Principal{AccountID: idstr(p.AccountID)}, false, false); err != nil {
		return app.StoredToken{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return app.StoredToken{}, domain.ErrUnavailable
	}
	return app.StoredToken{AccountID: idstr(p.AccountID), Environment: p.Environment, PackageName: p.PackageName, Digest: digest, CipherToken: p.TokenCipher, Snapshot: *snapshot(p), Revision: p.Revision}, nil
}
func (s *Store) MarkAcknowledged(ctx context.Context, digest string, revision int64) error {
	row, err := s.ForJob(ctx, digest)
	if err != nil {
		return err
	}
	if row.Revision != revision {
		return domain.ErrStale
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qsub.New(tx)
	if err = s.authorize(ctx, q, app.Principal{AccountID: row.AccountID}, false, false); err != nil {
		return err
	}
	n, err := q.MarkAcknowledged(ctx, qsub.MarkAcknowledgedParams{TokenDigest: digest, Revision: revision})
	if err != nil {
		return domain.ErrUnavailable
	}
	if n != 1 {
		return domain.ErrStale
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func (s *Store) ReceiveNotification(ctx context.Context, n app.Notification) (bool, error) {
	if s == nil || s.Pool == nil || !digestOK(n.Digest) || !digestOK(n.PayloadHash) || n.MessageID == "" || len(n.MessageID) > 128 || (n.Environment != "test" && n.Environment != "production") {
		return false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qsub.New(tx)
	// Resolve owner before acquiring any token locks, preserving the global
	// account -> token lock order used by admissions and erasure.
	purchase, lookup := q.GetPurchase(ctx, n.Digest)
	if lookup != nil && !errors.Is(lookup, pgx.ErrNoRows) {
		return false, domain.ErrUnavailable
	}
	known := lookup == nil
	if known {
		if purchase.Environment != n.Environment || purchase.PackageName != n.PackageName {
			return false, domain.ErrDenied
		}
		if err = s.authorize(ctx, q, app.Principal{AccountID: idstr(purchase.AccountID)}, false, false); err != nil {
			if errors.Is(err, domain.ErrUnauthorized) {
				known = false
			} else {
				return false, err
			}
		}
	}
	if err = q.LockToken(ctx, "subscription-notification-budget"); err != nil {
		return false, domain.ErrUnavailable
	}
	old, err := q.GetProviderReceipt(ctx, qsub.GetProviderReceiptParams{Environment: n.Environment, MessageID: n.MessageID})
	if err == nil {
		if old != n.PayloadHash {
			return false, domain.ErrConflict
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrUnavailable
	}
	count, err := q.NotificationAdmissions(ctx)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	if count >= 60 {
		return false, domain.ErrQuota
	}
	if err = q.InsertProviderReceipt(ctx, qsub.InsertProviderReceiptParams{Environment: n.Environment, MessageID: n.MessageID, PayloadHash: n.PayloadHash, TokenDigest: n.Digest}); err != nil {
		return false, domain.ErrUnavailable
	}
	if known && !purchase.Retired {
		if err = q.LockToken(ctx, n.Digest); err != nil {
			return false, domain.ErrUnavailable
		}
		if err = q.InvalidatePurchase(ctx, n.Digest); err != nil {
			return false, domain.ErrUnavailable
		}
		if err = q.InvalidateSlot(ctx, n.Digest); err != nil {
			return false, domain.ErrUnavailable
		}
		payload := []byte(`{"version":1,"digest":"` + n.Digest + `"}`)
		if _, err = jobs.Enqueue(ctx, tx, "subscription-refresh", payload, "subscription-refresh:"+n.Environment+":"+n.MessageID, 5, time.Time{}); err != nil {
			return false, domain.ErrUnavailable
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, domain.ErrUnavailable
	}
	return false, nil
}

var _ app.JobStore = (*Store)(nil)
