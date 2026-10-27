package adapters

import (
	"context"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5"
)

// ListRevisions supports bounded retained-data export of original/corrected facts.
func (s *Store) ListRevisions(ctx context.Context, owner, target string, after int64, limit int) (app.RevisionPage, error) {
	if !domain.ValidID(target) || after < 0 || limit < 1 || limit > domain.MaxItems {
		return app.RevisionPage{}, domain.ErrInvalid
	}
	tx, query, e := s.begin(ctx, owner, "")
	if e != nil {
		return app.RevisionPage{}, e
	}
	defer rollback(tx)
	if _, e = query.GetDriverEarning(ctx, q.GetDriverEarningParams{AccountID: uid(owner), ID: uid(target)}); errors.Is(e, pgx.ErrNoRows) {
		return app.RevisionPage{}, app.ErrNotFound
	} else if e != nil {
		return app.RevisionPage{}, storageError(ctx, e)
	}
	rows, e := query.PageDriverRevisions(ctx, q.PageDriverRevisionsParams{AccountID: uid(owner), RecordID: uid(target), Revision: after, Limit: int32(limit + 1)})
	if e != nil {
		return app.RevisionPage{}, storageError(ctx, e)
	}
	out := app.RevisionPage{Revisions: []app.EarningVersion{}}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextRevision = rows[len(rows)-1].Revision
	}
	for _, r := range rows {
		if r.SchemaVersion != 1 {
			return app.RevisionPage{}, app.ErrUnavailable
		}
		var data domain.EarningRecord
		if e = json.Unmarshal(r.Payload, &data); e != nil {
			return app.RevisionPage{}, app.ErrUnavailable
		}
		data, e = data.Normalize()
		if e != nil {
			return app.RevisionPage{}, app.ErrUnavailable
		}
		out.Revisions = append(out.Revisions, app.EarningVersion{Record: data, Revision: r.Revision})
	}
	if e = s.commitOwner(ctx, tx, owner); e != nil {
		return app.RevisionPage{}, e
	}
	return out, nil
}
