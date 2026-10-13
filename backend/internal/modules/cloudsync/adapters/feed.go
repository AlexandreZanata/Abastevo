package adapters

import (
	"context"
	"encoding/json"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"time"
)

func (s *Store) Pull(ctx context.Context, p app.Principal, pos app.Position, limit int) (app.Feed, error) {
	if pos.Version != 1 || limit < 1 || limit > 100 {
		return app.Feed{}, domain.ErrInvalid
	}
	if pos.After < 0 || pos.Through < 0 || pos.Epoch < 0 || pos.ConsentRevision < 0 {
		return app.Feed{}, domain.ErrInvalid
	}
	if pos.Through != 0 && pos.After > pos.Through {
		return app.Feed{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.Feed{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.Feed{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.Feed{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.Feed{}, domain.ErrUnavailable
	}
	initial := pos.After == 0 && pos.Through == 0 && pos.Epoch == 0 && pos.ConsentRevision == 0
	if initial {
		if state.CompactedThrough > 0 {
			return app.Feed{}, domain.ErrResnapshot
		}
	} else {
		if pos.Epoch != state.Epoch || pos.ConsentRevision != state.ConsentRevision {
			return app.Feed{}, domain.ErrResnapshot
		}
		if pos.After < state.CompactedThrough {
			return app.Feed{}, domain.ErrResnapshot
		}
		if pos.Through != 0 {
			if pos.Through > state.Revision {
				return app.Feed{}, domain.ErrInvalid
			}
			if pos.Through <= state.CompactedThrough {
				return app.Feed{}, domain.ErrResnapshot
			}
		}
	}
	watermark := pos.Through
	if watermark == 0 {
		watermark = state.Revision
	}
	rows, err := query.ListChanges(ctx, q.ListChangesParams{AccountID: uid(p.AccountID), Sequence: pos.After, Sequence_2: watermark, Limit: int32(limit)})
	if err != nil {
		return app.Feed{}, domain.ErrUnavailable
	}
	changes := make([]app.Change, 0, len(rows))
	for _, row := range rows {
		if state.ConsentMask&datasetBit(row.Dataset) == 0 {
			continue
		}
		var payload json.RawMessage
		if len(row.Payload) > 0 {
			payload = json.RawMessage(append([]byte(nil), row.Payload...))
		}
		changes = append(changes, app.Change{
			Sequence: row.Sequence,
			Epoch:    row.Epoch,
			Dataset:  row.Dataset,
			ID:       idstr(row.ID),
			Revision: row.RecordRevision,
			Deleted:  row.Deleted,
			Payload:  payload,
		})
	}
	return app.Feed{Changes: changes, Through: watermark, Epoch: state.Epoch, ConsentRevision: state.ConsentRevision}, nil
}

var _ app.PullStore = (*Store)(nil)
