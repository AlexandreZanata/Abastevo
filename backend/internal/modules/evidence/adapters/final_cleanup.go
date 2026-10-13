package adapters

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func finalCleanupNonce() (pgtype.UUID, error) {
	var id pgtype.UUID
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		return id, err
	}
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x40
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	id.Valid = true
	return id, nil
}

// claimFinalCleanup serializes only admission/receipts. The transaction ends
// before private storage is contacted. Lease time always uses the live clock,
// independently of an injected policy cutoff used by retention tests.
func (s *Store) claimFinalCleanup(ctx context.Context, key, owner string, policyNow time.Time) (pgtype.UUID, bool, error) {
	var none pgtype.UUID
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return none, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	acquired, err := q.TryLockFinalOutputKey(ctx, key)
	if err != nil || !acquired {
		return none, false, err
	}
	row, err := q.GetFinalOutputForUpdate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, err
	}
	sess, err := q.GetUploadSessionForUpdate(ctx, row.SessionID)
	if err != nil {
		return none, false, err
	}
	if owner != "" && sess.ContributorRef != owner {
		return none, false, domain.ErrBadTransition
	}
	if owner != "" {
		if err := q.ExpireErasureSession(ctx, sess.ID); err != nil {
			return none, false, err
		}
		sess.Status = domain.StateExpired
	}
	now := time.Now()
	if row.State == "PURGED" && (owner != "" || !policyNow.Add(application.CleanupMargin).Before(row.DeadlineAt.Time)) {
		reopened, err := q.ReopenRestoredFinalOutput(ctx, key)
		if err != nil {
			return none, false, err
		}
		if reopened == 1 {
			row, err = q.GetFinalOutputForUpdate(ctx, key)
			if err != nil {
				return none, false, err
			}
		}
	}
	if row.State == "PURGED" || (row.LeaseUntil.Valid && now.Before(row.LeaseUntil.Time)) {
		return none, false, tx.Commit(ctx)
	}
	due := owner != "" || row.State == "WRITING" || row.State == "PURGING" || row.Attempts >= 5 || !policyNow.Add(application.CleanupMargin).Before(row.DeadlineAt.Time) || sess.Status == domain.StateExpired || sess.Status == domain.StateRejected
	if !due {
		return none, false, nil
	}
	nonce, err := finalCleanupNonce()
	if err != nil {
		return none, false, err
	}
	n, err := q.ClaimFinalOutputCleanup(ctx, evidence.ClaimFinalOutputCleanupParams{FinalKey: key, LeaseNonce: nonce, LeaseUntil: pgTime(now.Add(finalWriteLease)), At: pgTime(now)})
	if err != nil {
		return none, false, err
	}
	if n != 1 {
		return none, false, domain.ErrBadTransition
	}
	return nonce, true, tx.Commit(ctx)
}

func (s *Store) finishFinalCleanup(ctx context.Context, key string, nonce pgtype.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockFinalOutputKey(ctx, key); err != nil {
		return err
	}
	row, err := q.GetFinalOutputForUpdate(ctx, key)
	if err != nil {
		return err
	}
	if row.State != "PURGING" || row.LeaseNonce != nonce || !time.Now().Before(row.LeaseUntil.Time) {
		return domain.ErrBadTransition
	}
	sess, err := q.GetUploadSessionForUpdate(ctx, row.SessionID)
	if err != nil {
		return err
	}
	next := "PURGED"
	_, objectErr := q.GetObjectByFinalKey(ctx, key)
	if objectErr != nil && !errors.Is(objectErr, pgx.ErrNoRows) {
		return objectErr
	}
	if errors.Is(objectErr, pgx.ErrNoRows) && sess.Status == domain.StateVerifying && row.Attempts < 5 && time.Now().Before(row.DeadlineAt.Time.Add(-application.CleanupMargin)) {
		next = "PENDING"
	}
	if err := q.MarkFinalKeyDeleted(ctx, key); err != nil {
		return err
	}
	n, err := q.FinishFinalOutputCleanup(ctx, evidence.FinishFinalOutputCleanupParams{FinalKey: key, LeaseNonce: nonce, NextState: next})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrBadTransition
	}
	return tx.Commit(ctx)
}

// disposeFinalClaim requires DELETE + confirmed absence from its private
// storage port. A failure retains PURGING for restart/lease-expiry recovery.
func (s *Store) disposeFinalClaim(ctx context.Context, key string, nonce pgtype.UUID, deleteFinal func(context.Context, string) error) error {
	copyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	row, readErr := evidence.New(s.pool).GetFinalOutputForUpdate(copyCtx, key)
	if readErr != nil {
		cancel()
		return readErr
	}
	if row.State != "PURGING" || row.LeaseNonce != nonce || !time.Now().Add(8*time.Second).Before(row.LeaseUntil.Time) {
		cancel()
		return domain.ErrBadTransition
	}
	err := deleteFinal(copyCtx, key)
	cancel()
	if err != nil {
		return err
	}
	ackCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	return s.finishFinalCleanup(ackCtx, key, nonce)
}

func (s *Store) ReconcileFinalOutputs(ctx context.Context, policyNow time.Time, batch int, deleteFinal func(context.Context, string) error) (application.FinalCleanupReport, error) {
	return s.reconcileFinalOutputs(ctx, "", policyNow, batch, deleteFinal)
}

func (s *Store) reconcileFinalOutputs(ctx context.Context, owner string, policyNow time.Time, batch int, deleteFinal func(context.Context, string) error) (application.FinalCleanupReport, error) {
	var rep application.FinalCleanupReport
	if batch < 1 || deleteFinal == nil {
		return rep, domain.ErrBadTransition
	}
	if batch > application.FinalCleanupBatch {
		batch = application.FinalCleanupBatch
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	q := evidence.New(s.pool)
	var failures error
	for rep.Checked < batch {
		if err := ctx.Err(); err != nil {
			return rep, errors.Join(failures, err)
		}
		var key string
		var err error
		if owner == "" {
			key, err = q.NextFinalOutputCleanup(ctx, evidence.NextFinalOutputCleanupParams{At: pgTime(time.Now()), DueCutoff: pgTime(policyNow.Add(application.CleanupMargin))})
		} else {
			key, err = q.NextOwnerFinalOutputCleanup(ctx, evidence.NextOwnerFinalOutputCleanupParams{At: pgTime(time.Now()), ContributorRef: owner})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return rep, errors.Join(failures, err)
		}
		nonce, claimed, err := s.claimFinalCleanup(ctx, key, owner, policyNow)
		if err != nil {
			return rep, errors.Join(failures, err)
		}
		if !claimed {
			rep.Skipped++
			break
		}
		rep.Checked++
		if err := s.disposeFinalClaim(ctx, key, nonce, deleteFinal); err != nil {
			rep.Failures++
			failures = errors.Join(failures, err)
			continue
		}
		rep.Purged++
	}
	return rep, failures
}

// Legacy object rows acquire inventory before using the same cleanup lease.
func (s *Store) deleteFinalCopy(ctx context.Context, key, owner string, policyNow time.Time, deleteFinal func(context.Context, string) error) (bool, error) {
	if deleteFinal == nil {
		return false, domain.ErrBadTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockFinalOutputKey(ctx, key); err != nil {
		return false, err
	}
	if err := q.InventoryExistingFinalOutput(ctx, key); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	nonce, claimed, err := s.claimFinalCleanup(ctx, key, owner, policyNow)
	if err != nil || !claimed {
		return false, err
	}
	if err := s.disposeFinalClaim(ctx, key, nonce, deleteFinal); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) DeleteFinalCopy(ctx context.Context, id, key string, policyNow time.Time, deleteFinal func(context.Context, string) error) (bool, error) {
	// Resolve the supplied object identity too: a caller cannot substitute a key.
	uid, err := mustUUID(id)
	if err != nil {
		return false, err
	}
	obj, err := evidence.New(s.pool).GetObject(ctx, uid)
	if err != nil {
		return false, err
	}
	if obj.FinalKey != key {
		return false, domain.ErrBadTransition
	}
	return s.deleteFinalCopy(ctx, key, "", policyNow, deleteFinal)
}
