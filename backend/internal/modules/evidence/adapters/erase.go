package adapters

import (
	"context"
	"errors"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
)

// EraseContributor purges at most 64 private objects per call. Storage
// failures or remaining rows refuse completion; the durable erasure intent
// retries without recording a successful evidence ledger scope.
func (s *Store) EraseContributor(ctx context.Context, ref string, deleteKey func(ctx context.Context, key, namespace string) error) (purgedSessions, purgedObjects int64, err error) {
	if deleteKey == nil {
		return 0, 0, errors.New("evidence: erasure storage not configured")
	}
	q := evidence.New(s.pool)
	sessions, err := q.PendingErasureSessions(ctx, ref)
	if err != nil {
		return 0, 0, err
	}
	for _, sess := range sessions {
		if err := q.ExpireErasureSession(ctx, sess.ID); err != nil {
			return purgedSessions, purgedObjects, err
		}
		if deleteKey != nil && sess.QuarantineKey != "" {
			if derr := deleteKey(ctx, sess.QuarantineKey, "q/"); derr != nil {
				return purgedSessions, purgedObjects, derr
			}
		}
		if _, merr := q.MarkQuarantineDeleted(ctx, sess.ID); merr != nil {
			return purgedSessions, purgedObjects, merr
		}
		purgedSessions++
	}
	objects, err := q.PendingErasureObjects(ctx, ref)
	if err != nil {
		return purgedSessions, purgedObjects, err
	}
	for _, obj := range objects {
		if deleteKey != nil && obj.FinalKey != "" {
			if derr := deleteKey(ctx, obj.FinalKey, "f/"); derr != nil {
				return purgedSessions, purgedObjects, derr
			}
		}
		if _, merr := q.MarkFinalDeleted(ctx, obj.ID); merr != nil {
			return purgedSessions, purgedObjects, merr
		}
		purgedObjects++
	}
	pending, err := q.HasPendingEvidenceErasure(ctx, ref)
	if err != nil {
		return purgedSessions, purgedObjects, err
	}
	if !pending.Valid || pending.Bool {
		return purgedSessions, purgedObjects, errors.New("evidence: erasure has remaining objects")
	}
	return purgedSessions, purgedObjects, nil
}
