package adapters

import (
	"context"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
)

// EraseContributor purges one owner's evidence footprint in the narrow
// erasure workflow (P07-T04, B-BR-016): quarantine and final bytes
// delete through the storage port (best-effort with counts, like the
// retention sweeper), sessions expire, and object deletions mark.
// Database marks are fail-fast; storage failures report counts for
// retry without blocking the ledger.
func (s *Store) EraseContributor(ctx context.Context, ref string, deleteKey func(ctx context.Context, key, namespace string) error) (purgedSessions, purgedObjects int64, err error) {
	q := evidence.New(s.pool)
	sessions, err := q.ListSessionsByContributor(ctx, evidence.ListSessionsByContributorParams{
		ContributorRef: ref, PageLimit: 10000,
	})
	if err != nil {
		return 0, 0, err
	}
	for _, sess := range sessions {
		if deleteKey != nil && sess.QuarantineKey != "" {
			if derr := deleteKey(ctx, sess.QuarantineKey, "q/"); derr != nil {
				continue
			}
		}
		if _, merr := q.MarkQuarantineDeleted(ctx, sess.ID); merr != nil {
			return purgedSessions, purgedObjects, merr
		}
		purgedSessions++
	}
	if _, err := q.ExpireSessionsByContributor(ctx, ref); err != nil {
		return purgedSessions, purgedObjects, err
	}
	objects, err := q.ListObjectsByContributor(ctx, evidence.ListObjectsByContributorParams{
		ContributorRef: ref, PageLimit: 10000,
	})
	if err != nil {
		return purgedSessions, purgedObjects, err
	}
	for _, obj := range objects {
		if deleteKey != nil && obj.FinalKey != "" {
			if derr := deleteKey(ctx, obj.FinalKey, "f/"); derr != nil {
				continue
			}
		}
		if _, merr := q.MarkFinalDeleted(ctx, obj.ID); merr != nil {
			return purgedSessions, purgedObjects, merr
		}
		purgedObjects++
	}
	return purgedSessions, purgedObjects, nil
}
