package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Author lookups for community safety (B-BR-SAFE-02/06). They return plain
// values so the safety module never imports forum types; the composition
// root adapts them. Account ids stay server-side.

// SafetyTopicAuthorTx returns the author and public alias of a topic that
// public reads serve now (published, visible, kind TOPIC). Any other post
// reports ok=false, indistinguishable from a missing one.
func (s *Store) SafetyTopicAuthorTx(ctx context.Context, tx pgx.Tx, postID string) (accountID, alias string, ok bool, err error) {
	id, perr := mustUUID(postID)
	if perr != nil {
		return "", "", false, nil
	}
	row, err := s.queries.WithTx(tx).GetSafetyTopicAuthor(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("forum: safety topic author: %w", err)
	}
	return uuidString(row.AuthorAccountID), row.PublicAlias, true, nil
}

// SafetyCommentAuthorTx returns the author and public alias of a comment or
// reply that public reads serve now (it, its post and any parent are
// published and visible); anything else reports ok=false.
func (s *Store) SafetyCommentAuthorTx(ctx context.Context, tx pgx.Tx, commentID string) (accountID, alias string, ok bool, err error) {
	id, perr := mustUUID(commentID)
	if perr != nil {
		return "", "", false, nil
	}
	row, err := s.queries.WithTx(tx).GetSafetyCommentAuthor(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("forum: safety comment author: %w", err)
	}
	return uuidString(row.AuthorAccountID), row.PublicAlias, true, nil
}

// SafetyTopicAuthors maps listed topic ids to their author account ids,
// whatever their state; unknown, non-topic and authorless ids are absent.
func (s *Store) SafetyTopicAuthors(ctx context.Context, ids []string) (map[string]string, error) {
	keys := safetyUUIDs(ids)
	out := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := s.queries.ListSafetyTopicAuthors(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("forum: safety topic authors: %w", err)
	}
	for _, r := range rows {
		out[uuidString(r.ID)] = uuidString(r.AuthorAccountID)
	}
	return out, nil
}

// SafetyCommentAuthors maps listed comment/reply ids to their author
// account ids, whatever their state; unknown and scrubbed ids are absent.
func (s *Store) SafetyCommentAuthors(ctx context.Context, ids []string) (map[string]string, error) {
	keys := safetyUUIDs(ids)
	out := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := s.queries.ListSafetyCommentAuthors(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("forum: safety comment authors: %w", err)
	}
	for _, r := range rows {
		out[uuidString(r.ID)] = uuidString(r.AuthorAccountID)
	}
	return out, nil
}

func safetyUUIDs(ids []string) []pgtype.UUID {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, raw := range ids {
		if id, err := mustUUID(raw); err == nil {
			out = append(out, id)
		}
	}
	return out
}
