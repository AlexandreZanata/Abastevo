package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/feedback"
)

// Author lookups for community safety (B-BR-SAFE-02/06). They return plain
// values so the safety module never imports feedback types; the
// composition root adapts them. Account ids stay server-side.

// SafetyCommentAuthorTx returns the author and public alias of a station
// comment or reply that public reads serve now (not deleted, not hidden);
// anything else reports ok=false, indistinguishable from a missing one.
func (s *PGStore) SafetyCommentAuthorTx(ctx context.Context, tx pgx.Tx, commentID string) (accountID, alias string, ok bool, err error) {
	id, perr := mustUUID(commentID)
	if perr != nil {
		return "", "", false, nil
	}
	row, err := feedback.New(tx).GetSafetyCommentAuthor(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return uuidString(row.AccountID), row.Alias, true, nil
}

// SafetyCommentAuthors maps listed station comment ids to their author
// account ids, whatever their state; unknown ids are absent.
func (s *PGStore) SafetyCommentAuthors(ctx context.Context, ids []string) (map[string]string, error) {
	keys := make([]pgtype.UUID, 0, len(ids))
	for _, raw := range ids {
		if id, err := mustUUID(raw); err == nil {
			keys = append(keys, id)
		}
	}
	out := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := feedback.New(s.pool).ListSafetyCommentAuthors(ctx, keys)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[uuidString(r.ID)] = uuidString(r.AccountID)
	}
	return out, nil
}
