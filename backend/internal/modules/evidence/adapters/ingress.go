package adapters

import (
	"context"
	"errors"
	"time"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/jackc/pgx/v5"
)

// ClaimUpload charges every transfer attempt durably before reading bytes.
// No DB connection/transaction is held during client or storage I/O.
func (s *Store) ClaimUpload(ctx context.Context, id, nonce string, now time.Time) (domain.Session, bool, error) {
	if s.intakeConfigured {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
	}
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Session{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Session{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	row, err := q.GetUploadSessionForUpdate(ctx, uid)
	if err != nil {
		return domain.Session{}, false, err
	}
	session := mapSession(row.ID, row.ContributorRef, row.ClientSessionID, row.Mime, row.DeclaredBytes, row.ClaimedSha256, row.QuarantineKey, row.Status, row.CreatedAt, row.ExpiresAt, row.UpdatedAt, row.PolicyVersion, row.PhotoCaptureID)
	if nonce == "" || session.Status != domain.StateIssued || row.QuarantineDeletedAt.Valid || !now.Before(session.ExpiresAt) || !now.Add(30*time.Second).Before(session.CreatedAt.Add(application.CopyRetention-application.CleanupMargin)) {
		return domain.Session{}, false, domain.ErrBadTransition
	}
	receipt, err := q.GetUploadTransfer(ctx, uid)
	if err == nil && receipt.Valid {
		return session, true, tx.Commit(ctx)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, false, err
	}
	if s.intakeConfigured {
		if err := q.LockIntakeAdmission(ctx); err != nil {
			return domain.Session{}, false, errors.Join(application.ErrEnforcementUnhealthy, err)
		}
		if err := s.requireIntake(ctx, q, 0); err != nil {
			return domain.Session{}, false, err
		}
	}
	n, err := q.ClaimUploadTransfer(ctx, evidence.ClaimUploadTransferParams{SessionID: uid, LeaseNonce: nonce, LeaseUntil: pgTime(now.Add(30 * time.Second)), At: pgTime(now)})
	if err != nil {
		return domain.Session{}, false, err
	}
	if n != 1 {
		return domain.Session{}, false, domain.ErrQuotaDenied
	}
	return session, false, tx.Commit(ctx)
}

func (s *Store) FinishUpload(ctx context.Context, id, nonce string, completed bool, actualBytes int64) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	n, err := evidence.New(s.pool).FinishUploadTransfer(ctx, evidence.FinishUploadTransferParams{SessionID: uid, LeaseNonce: nonce, Completed: completed, ActualBytes: actualBytes})
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrBadTransition
	}
	return nil
}

// CanDeleteQuarantine uses a fresh snapshot after admission has been closed.
func (s *Store) CanDeleteQuarantine(ctx context.Context, id string) (bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return false, err
	}
	return evidence.New(s.pool).CanDeleteQuarantine(ctx, evidence.CanDeleteQuarantineParams{ID: uid, At: pgTime(time.Now())})
}
