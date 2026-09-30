package adapters

import (
	"context"
	"sort"

	feedback "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/feedback"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// ListRatingsByAccount returns every rating row of one account,
// including tombstoned history, ordered for deterministic export.
func (s *PGStore) ListRatingsByAccount(ctx context.Context, accountID string) ([]domain.StoredRating, error) {
	account, err := mustUUID(accountID)
	if err != nil {
		return nil, nil
	}
	rows, err := feedback.New(s.pool).ListRatingsByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	out := make([]domain.StoredRating, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRating(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListCommentsByAccount returns every comment/reply row of one
// account, including tombstoned history, ordered for export.
func (s *PGStore) ListCommentsByAccount(ctx context.Context, accountID string) ([]domain.StoredComment, error) {
	account, err := mustUUID(accountID)
	if err != nil {
		return nil, nil
	}
	rows, err := feedback.New(s.pool).ListCommentsByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	out := make([]domain.StoredComment, 0, len(rows))
	for _, row := range rows {
		parent := ""
		if row.ParentID.Valid {
			parent = uuidString(row.ParentID)
		}
		out = append(out, domain.StoredComment{
			ID:         uuidString(row.ID),
			AccountID:  uuidString(row.AccountID),
			StationID:  uuidString(row.StationID),
			Product:    row.Product,
			ParentID:   parent,
			Depth:      int(row.Depth),
			Text:       row.Text,
			Revision:   int(row.Revision),
			CreatedAt:  unix(row.CreatedAt),
			UpdatedAt:  unix(row.UpdatedAt),
			DeletedAt:  unix(row.DeletedAt),
			Visibility: row.Visibility,
		})
	}
	return out, nil
}

// ListVotesByAccount returns every vote row of one account,
// including tombstoned history, ordered for export.
func (s *PGStore) ListVotesByAccount(ctx context.Context, accountID string) ([]domain.StoredVote, error) {
	account, err := mustUUID(accountID)
	if err != nil {
		return nil, nil
	}
	rows, err := feedback.New(s.pool).ListVotesByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	out := make([]domain.StoredVote, 0, len(rows))
	for _, row := range rows {
		out = append(out, toVote(row))
	}
	return out, nil
}
