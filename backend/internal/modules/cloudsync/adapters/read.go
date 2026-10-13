package adapters

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

const zeroID = "00000000-0000-0000-0000-000000000000"

func (s *Store) readTx(ctx context.Context, p app.Principal, signing bool) (pgx.Tx, *q.Queries, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, domain.ErrUnavailable
	}
	query := q.New(tx)
	if err = s.authorize(ctx, query, p, signing); err != nil {
		rollback(tx)
		return nil, nil, err
	}
	return tx, query, nil
}
func stateView(state q.CloudState) app.Status {
	enabled := []string{}
	for _, d := range []string{"vehicles", "favorites", "preferences"} {
		if state.ConsentMask&datasetBit(d) != 0 {
			enabled = append(enabled, d)
		}
	}
	return app.Status{Epoch: state.Epoch, Revision: state.Revision, ConsentRevision: state.ConsentRevision, EnabledDatasets: enabled}
}
func (s *Store) Status(ctx context.Context, p app.Principal) (app.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.readTx(ctx, p, false)
	if err != nil {
		return app.Status{}, err
	}
	defer rollback(tx)
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.Status{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.Status{}, domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Status{}, domain.ErrUnavailable
	}
	return stateView(state), nil
}
func pagePosition(limit int, after string) (string, error) {
	if limit < 1 || limit > 101 {
		return "", domain.ErrInvalid
	}
	if after == "" {
		return zeroID, nil
	}
	if !domain.ValidID(after) {
		return "", domain.ErrInvalid
	}
	return after, nil
}
func (s *Store) List(ctx context.Context, p app.Principal, dataset string, limit int, after string) ([]app.Record, error) {
	after, err := pagePosition(limit, after)
	if err != nil || datasetBit(dataset) == 0 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.readTx(ctx, p, false)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	rows, err := query.ListRecords(ctx, q.ListRecordsParams{AccountID: uid(p.AccountID), Dataset: dataset, ID: uid(after), Limit: int32(limit)})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	result := make([]app.Record, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.Record{Dataset: row.Dataset, ID: idstr(row.ID), Revision: row.Revision, Deleted: row.Deleted, Payload: row.Payload})
	}
	return result, nil
}
func (s *Store) Devices(ctx context.Context, p app.Principal, limit int, after string) ([]app.Device, error) {
	after, err := pagePosition(limit, after)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.readTx(ctx, p, false)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	rows, err := query.ListDevices(ctx, q.ListDevicesParams{AccountID: uid(p.AccountID), Origin: s.Origin, ID: uid(after), Limit: int32(limit)})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	result := make([]app.Device, 0, len(rows))
	for _, row := range rows {
		var last *time.Time
		if row.LastSuccessAt.Valid {
			v := row.LastSuccessAt.Time
			last = &v
		}
		result = append(result, app.Device{ID: idstr(row.ID), Origin: row.Origin, CreatedAt: row.CreatedAt.Time, LastSuccess: last, Revoked: row.RevokedAt.Valid})
	}
	return result, nil
}
func (s *Store) Erase(ctx context.Context, p app.Principal, dataset, id string) (int64, error) {
	if (dataset != "all" && datasetBit(dataset) == 0) || (id != "" && !domain.ValidID(id)) || (dataset == "all" && id != "") {
		return 0, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.readTx(ctx, p, true)
	if err != nil {
		return 0, err
	}
	defer rollback(tx)
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return 0, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	args := q.NeedsErasureParams{AccountID: uid(p.AccountID), Dataset: dataset, RecordID: uid(id)}
	needs, err := query.NeedsErasure(ctx, args)
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	mask := datasetBit(dataset)
	if dataset == "all" {
		mask = 7
	}
	if !needs.Valid {
		return 0, domain.ErrUnavailable
	}
	if !needs.Bool && (id != "" || state.ConsentMask&mask == 0) {
		return state.Epoch, nil
	}
	if err = query.EraseSelectedIDs(ctx, q.EraseSelectedIDsParams(args)); err != nil {
		return 0, domain.ErrUnavailable
	}
	if err = query.EraseSelectedRecords(ctx, q.EraseSelectedRecordsParams(args)); err != nil {
		return 0, domain.ErrUnavailable
	}
	if err = query.EraseSelectedChanges(ctx, q.EraseSelectedChangesParams(args)); err != nil {
		return 0, domain.ErrUnavailable
	}
	if dataset == "all" {
		if err = query.RevokeAllCloudDevices(ctx, uid(p.AccountID)); err != nil {
			return 0, domain.ErrUnavailable
		}
	}
	epoch, err := query.ErasureEpoch(ctx, q.ErasureEpochParams{AccountID: uid(p.AccountID), Mask: mask})
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, domain.ErrUnavailable
	}
	return epoch, nil
}

var _ app.ReadStore = (*Store)(nil)
