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

func (s *Store) ScheduleReconciliation(ctx context.Context, env, pkg string, limit int) (int, error) {
	if s == nil || s.Pool == nil {
		return 0, domain.ErrUnavailable
	}
	if limit < 1 || limit > 100 || (env != "test" && env != "production") || pkg == "" {
		return 0, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	q := qsub.New(s.Pool)
	rows, err := q.DuePurchases(ctx, qsub.DuePurchasesParams{Environment: env, PackageName: pkg, Limit: int32(limit)})
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	total := 0
	for _, r := range rows {
		tx, e := s.Pool.Begin(ctx)
		if e != nil {
			return total, domain.ErrUnavailable
		}
		e = func() error {
			defer rollback(tx)
			q := qsub.New(tx)
			if err := s.authorize(ctx, q, app.Principal{AccountID: idstr(r.AccountID)}, false, false); err != nil {
				if err == domain.ErrUnauthorized {
					return nil
				}
				return err
			}
			if err := q.LockToken(ctx, r.TokenDigest); err != nil {
				return domain.ErrUnavailable
			}
			n, err := q.DeferReconciliation(ctx, r.TokenDigest)
			if err != nil {
				return domain.ErrUnavailable
			}
			if n == 0 {
				return nil
			}
			// Durable due time is the checkpoint. No scan cursor can starve a failed token.
			payload := []byte(`{"version":1,"digest":"` + r.TokenDigest + `"}`)
			if _, err = jobs.Enqueue(ctx, tx, "subscription-refresh", payload, "", 5, time.Time{}); err != nil {
				return domain.ErrUnavailable
			}
			if err = tx.Commit(ctx); err != nil {
				return domain.ErrUnavailable
			}
			total++
			return nil
		}()
		if e != nil {
			return total, e
		}
	}
	return total, nil
}
func (s *Store) CleanupReceipts(ctx context.Context) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	q := qsub.New(s.Pool)
	if _, err := q.PurgeOldOperations(ctx); err != nil {
		return domain.ErrUnavailable
	}
	if _, err := q.PurgeOldNotifications(ctx); err != nil {
		return domain.ErrUnavailable
	}

	rows, err := q.TerminalPurchases(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	for _, r := range rows {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return domain.ErrUnavailable
		}
		err = func() error {
			defer rollback(tx)
			q := qsub.New(tx)
			if _, e := q.LockAnyOwner(ctx, r.AccountID); e != nil {
				return domain.ErrUnavailable
			}
			if e := q.LockToken(ctx, r.TokenDigest); e != nil {
				return domain.ErrUnavailable
			}
			// Recheck eligibility after locks; a concurrent renewal must not be retired.
			_, e := q.LockTerminalPurchase(ctx, r.TokenDigest)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			if e != nil {
				return domain.ErrUnavailable
			}
			if _, e = q.RetireToken(ctx, qsub.RetireTokenParams(r)); e != nil {
				return domain.ErrUnavailable
			}
			if e = q.PurgeTerminalPurchase(ctx, r.TokenDigest); e != nil {
				return domain.ErrUnavailable
			}
			return tx.Commit(ctx)
		}()
		if err != nil {
			return domain.ErrUnavailable
		}
	}
	return nil
}

func (s *Store) Backlog(ctx context.Context) (int64, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := qsub.New(s.Pool).SubscriptionBacklog(ctx)
	if err != nil {
		return 0, 0, domain.ErrUnavailable
	}
	return row.Pending, row.Dead, nil
}
func (s *Store) ReplayDead(ctx context.Context, id, operator, reason string) error {
	if !domain.ValidID(id) || operator == "" || len(operator) > 64 || (reason != "provider-fixed" && reason != "db-fixed" && reason != "key-rotated") {
		return domain.ErrInvalid
	}
	for _, c := range operator {
		if c <= 32 || c > 126 {
			return domain.ErrInvalid
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := qsub.New(s.Pool).SubscriptionDeadJob(ctx, uid(id)); err != nil {
		return domain.ErrDenied
	}
	if err := jobs.NewQueue(s.Pool).ReplayDead(ctx, id, "subscription:"+operator+":"+reason); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
