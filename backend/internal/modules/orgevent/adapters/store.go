// Package adapters binds scoped enterprise intents to the existing job queue.
package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgevent"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ application.Writer = (*Store)(nil)

// Enqueue borrows the audit runner's transaction. RLS and its explicit scope
// check prevent a mismatched callback scope; transaction ownership stays there.
func (s *Store) Enqueue(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx, e domain.Envelope, commandHash string) error {
	if s == nil || tx == nil {
		return domain.ErrUnavailable
	}
	if scope.Validate() != nil || e.OrgID != scope.OrgID().String() {
		return domain.ErrInvalid
	}
	queries := q.New(tx)
	current, err := queries.CurrentScope(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	if current != e.OrgID {
		return domain.ErrDenied
	}
	result, err := jobs.Enqueue(ctx, tx, domain.JobKind, e.Payload(), e.DedupeKey(), domain.MaxAttempts, time.Time{})
	if err != nil {
		return domain.ErrUnavailable
	}
	// A replay is handled by the audited command before this callback. Never
	// adopt a pre-existing dedupe row whose immutable provenance is unknown.
	if !result.Created {
		return domain.ErrReused
	}
	if err := queries.InsertIntent(ctx, q.InsertIntentParams{
		OrgID: e.OrgID, EventID: e.ID, Consumer: e.Consumer, EventName: e.Name, TargetType: e.TargetType,
		TargetID: e.TargetID, Revision: e.Revision, CommandHash: commandHash, JobID: result.ID,
	}); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

type Effect func(context.Context, kernel.OrgScope, pgx.Tx, domain.Event) error

// apply fences the live queue row through the complete scoped transaction.
// The receipt commits with the database effect; queue acknowledgement remains
// with Dispatcher, whose lost acknowledgements can safely redeliver.
func (s *Store) apply(ctx context.Context, scope kernel.OrgScope, e domain.Envelope, job jobs.Job, effect Effect) error {
	if s == nil || s.pool == nil || effect == nil {
		return domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := database.WithOrgTx(ctx, s.pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		_, err := queries.LockLease(ctx, q.LockLeaseParams{
			JobID: job.ID, LeaseToken: job.LeaseToken, WorkerID: job.WorkerID, Payload: job.Payload,
			DedupeKey: pgtype.Text{String: e.DedupeKey(), Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return jobs.ErrStaleLease
		}
		if err != nil {
			return domain.ErrUnavailable
		}
		live := func() error {
			ok, err := queries.LeaseStillLive(ctx, q.LeaseStillLiveParams{JobID: job.ID, LeaseToken: job.LeaseToken, WorkerID: job.WorkerID})
			if err != nil {
				return domain.ErrUnavailable
			}
			if !ok {
				return jobs.ErrStaleLease
			}
			return nil
		}
		if err := live(); err != nil {
			return err
		}
		intent, err := queries.ReadIntent(ctx, q.ReadIntentParams{OrgID: e.OrgID, EventID: e.ID, Consumer: e.Consumer, JobID: job.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDenied
		}
		if err != nil {
			return domain.ErrUnavailable
		}
		if intent.EventName != e.Name || intent.TargetType != e.TargetType || intent.ITargetID != e.TargetID || intent.Revision != e.Revision {
			return domain.ErrDenied
		}
		_, err = queries.ReserveReceipt(ctx, q.ReserveReceiptParams{OrgID: e.OrgID, EventID: e.ID, Consumer: e.Consumer})
		if errors.Is(err, pgx.ErrNoRows) {
			return live()
		}
		if err != nil {
			return domain.ErrUnavailable
		}
		if err := effect(ctx, scope, tx, e.Event); err != nil {
			return application.SafeError(err)
		}
		return live()
	})
	if errors.Is(err, jobs.ErrStaleLease) {
		return jobs.ErrStaleLease
	}
	if err != nil {
		return application.SafeError(err)
	}
	return nil
}
