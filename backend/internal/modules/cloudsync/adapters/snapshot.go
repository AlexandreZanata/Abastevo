package adapters

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"time"
)

func (s *Store) Snapshot(ctx context.Context, p app.Principal, queryPos app.SnapshotQuery, limit int) (app.SnapshotResult, error) {
	if datasetBit(queryPos.Dataset) == 0 || limit < 1 || limit > 100 {
		return app.SnapshotResult{}, domain.ErrInvalid
	}
	after, err := pagePosition(limit, queryPos.After)
	if err != nil {
		return app.SnapshotResult{}, domain.ErrInvalid
	}
	if queryPos.Through < 0 || queryPos.Epoch < 0 || queryPos.ConsentRevision < 0 {
		return app.SnapshotResult{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.SnapshotResult{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.SnapshotResult{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.SnapshotResult{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.SnapshotResult{}, domain.ErrUnavailable
	}
	if state.ConsentMask&datasetBit(queryPos.Dataset) == 0 {
		return app.SnapshotResult{}, domain.ErrDenied
	}
	if queryPos.Through == 0 && queryPos.Epoch == 0 && queryPos.ConsentRevision == 0 && queryPos.After == "" {
		queryPos.Through = state.Revision
		queryPos.Epoch = state.Epoch
		queryPos.ConsentRevision = state.ConsentRevision
	} else {
		if queryPos.Epoch != state.Epoch || queryPos.ConsentRevision != state.ConsentRevision {
			return app.SnapshotResult{}, domain.ErrResnapshot
		}
		if queryPos.Through != state.Revision {
			return app.SnapshotResult{}, domain.ErrResnapshot
		}
	}
	rows, err := query.ListRecords(ctx, q.ListRecordsParams{AccountID: uid(p.AccountID), Dataset: queryPos.Dataset, ID: uid(after), Limit: int32(limit)})
	if err != nil {
		return app.SnapshotResult{}, domain.ErrUnavailable
	}
	records := make([]app.Record, 0, len(rows))
	for _, row := range rows {
		records = append(records, app.Record{Dataset: row.Dataset, ID: idstr(row.ID), Revision: row.Revision, Deleted: row.Deleted, Payload: row.Payload})
	}
	return app.SnapshotResult{Records: records, Through: queryPos.Through, Epoch: state.Epoch, ConsentRevision: state.ConsentRevision}, nil
}

var _ app.SnapshotStore = (*Store)(nil)
