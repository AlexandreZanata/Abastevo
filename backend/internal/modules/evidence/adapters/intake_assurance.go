package adapters

import (
	"context"
	"errors"
	"time"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

const MaxPendingPhotos = 100

func (s *Store) BeginSweepObservation(ctx context.Context) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockIntakeAdmission(ctx); err != nil {
		return 0, err
	}
	epoch, err := q.BeginSweepObservation(ctx)
	if err != nil {
		return 0, err
	}
	return epoch, tx.Commit(ctx)
}

func (s *Store) FinishSweepObservation(ctx context.Context, epoch int64, healthy bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockIntakeAdmission(ctx); err != nil {
		return err
	}
	n, err := q.FinishSweepObservation(ctx, evidence.FinishSweepObservationParams{Epoch: epoch, Healthy: healthy})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrBadTransition
	}
	return tx.Commit(ctx)
}

// CheckIntake precedes quota use. The actual fresh reservation checks capacity
// and inserts under the same global transaction lock; full-budget replays
// still converge without adding another session.
func (s *Store) CheckIntake(ctx context.Context) error {
	if !s.intakeConfigured || s.maxPendingPhotos < 1 || s.maxPendingPhotos > MaxPendingPhotos {
		return application.ErrEnforcementUnhealthy
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockIntakeAdmission(ctx); err != nil {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	if err := s.requireIntake(ctx, q, -1); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	return nil
}

// requireIntake runs inside the caller's admission transaction/global lock.
// additional=-1 observes health only, 0 checks an existing reservation's byte
// lease, and 1 reserves a new pending-photo slot atomically with insertion.
func (s *Store) requireIntake(ctx context.Context, q *evidence.Queries, additional int64) error {
	if !s.intakeConfigured || s.maxPendingPhotos < 1 || s.maxPendingPhotos > MaxPendingPhotos {
		return application.ErrEnforcementUnhealthy
	}
	healthy, err := q.IntakeObservationHealthy(ctx)
	if err != nil || !healthy.Valid || !healthy.Bool {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	now := time.Now()
	cutoff := pgTime(now.Add(-application.CopyRetention + application.CleanupMargin))
	originals, err := q.OverdueOriginals(ctx, evidence.OverdueOriginalsParams{Cutoff: cutoff, Batch: 1})
	if err != nil || len(originals) != 0 {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	finals, err := q.OverdueFinals(ctx, evidence.OverdueFinalsParams{Cutoff: cutoff, Batch: 1})
	if err != nil || len(finals) != 0 {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	unconfirmed, err := q.OverdueUnconfirmedFinalOutputs(ctx, evidence.OverdueUnconfirmedFinalOutputsParams{Deadline: pgTime(now.Add(application.CleanupMargin)), Batch: 1})
	if err != nil || len(unconfirmed) != 0 {
		return errors.Join(application.ErrEnforcementUnhealthy, err)
	}
	if additional >= 0 {
		count, err := q.CountPendingPhotoSessions(ctx, int32(s.maxPendingPhotos+1))
		if err != nil || count+additional > int64(s.maxPendingPhotos) {
			return errors.Join(application.ErrEnforcementUnhealthy, err)
		}
	}
	return nil
}
