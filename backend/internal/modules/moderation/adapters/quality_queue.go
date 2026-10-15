package adapters

import (
	"context"
	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/moderation"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// QualityQueue is restricted operator pagination over identifiers only.
func (s *Store) QualityQueue(ctx context.Context, after string) ([]domain.Case, string, error) {
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	id, e := mustUUID(after)
	if e != nil {
		return nil, "", e
	}
	rows, e := db.New(s.pool).ListQualityReviewCases(ctx, id)
	if e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > 100 {
		rows = rows[:100]
		next = uuidString(rows[99].ID)
	}
	out := make([]domain.Case, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToCase(r))
	}
	return out, next, nil
}
