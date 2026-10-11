package adapters

import (
	"context"
	"errors"
	"time"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/jackc/pgx/v5"
)

// ClaimUpload charges every transfer attempt durably before reading bytes.
// No DB connection/transaction is held during client or storage I/O.
func (s *Store) ClaimUpload(ctx context.Context, id, nonce string, now time.Time) (domain.Session, bool, error) {
	session, err := s.Session(ctx, id)
	if err != nil {
		return domain.Session{}, false, err
	}
	if nonce == "" || session.Status != domain.StateIssued || !now.Before(session.ExpiresAt) {
		return domain.Session{}, false, domain.ErrBadTransition
	}
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Session{}, false, err
	}
	q := evidence.New(s.pool)
	receipt, err := q.GetUploadTransfer(ctx, uid)
	if err == nil && receipt.Valid {
		return session, true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, false, err
	}
	n, err := q.ClaimUploadTransfer(ctx, evidence.ClaimUploadTransferParams{SessionID: uid, LeaseNonce: nonce, LeaseUntil: pgTime(now.Add(30 * time.Second)), At: pgTime(now)})
	if err != nil {
		return domain.Session{}, false, err
	}
	if n != 1 {
		return domain.Session{}, false, domain.ErrQuotaDenied
	}
	return session, false, nil
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
