package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/driver"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.ReportSnapshotStore = (*ReportStore)(nil)

type ReportStore struct {
	pool *pgxpool.Pool
}

func NewReportStore(pool *pgxpool.Pool) *ReportStore { return &ReportStore{pool: pool} }

func (s *ReportStore) SaveReportSnapshot(ctx context.Context, owner string, key app.ReportSnapshotKey, revision string, payload []byte) (int64, time.Time, error) {
	if s == nil || s.pool == nil || !domain.ValidID(owner) {
		return 0, time.Time{}, app.ErrUnavailable
	}
	from, e := day(key.From)
	if e != nil {
		return 0, time.Time{}, e
	}
	to, e := day(key.To)
	if e != nil {
		return 0, time.Time{}, e
	}
	id, e := jobs.NewUUIDv4()
	if e != nil {
		return 0, time.Time{}, app.ErrUnavailable
	}
	row, e := q.New(s.pool).UpsertDriverReportSnapshot(ctx, q.UpsertDriverReportSnapshotParams{
		AccountID: uid(owner), ID: uid(id), VehicleID: uid(key.VehicleID), Period: key.Period, FromDay: from, ToDay: to,
		Timezone: key.Timezone, Platform: key.Platform, Basis: key.Basis, SourceRevision: revision, Payload: payload,
	})
	if e != nil {
		return 0, time.Time{}, storageError(ctx, e)
	}
	return row.Version, row.GeneratedAt.Time.UTC(), nil
}

func (s *ReportStore) GetReportSnapshot(ctx context.Context, owner string, key app.ReportSnapshotKey) (app.StoredReportSnapshot, error) {
	if s == nil || s.pool == nil || !domain.ValidID(owner) {
		return app.StoredReportSnapshot{}, app.ErrUnavailable
	}
	from, e := day(key.From)
	if e != nil {
		return app.StoredReportSnapshot{}, e
	}
	to, e := day(key.To)
	if e != nil {
		return app.StoredReportSnapshot{}, e
	}
	row, e := q.New(s.pool).GetDriverReportSnapshot(ctx, q.GetDriverReportSnapshotParams{
		AccountID: uid(owner), VehicleID: uid(key.VehicleID), Period: key.Period, FromDay: from, ToDay: to,
		Timezone: key.Timezone, Platform: key.Platform, Basis: key.Basis,
	})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.StoredReportSnapshot{}, app.ErrNotFound
	}
	if e != nil {
		return app.StoredReportSnapshot{}, storageError(ctx, e)
	}
	return app.StoredReportSnapshot{
		Key: key, SourceRevision: row.SourceRevision, SnapshotVersion: row.Version,
		GeneratedAt: row.GeneratedAt.Time.UTC(), Payload: row.Payload,
	}, nil
}

func (s *ReportStore) DeleteExpiredReportSnapshots(ctx context.Context, before time.Time, limit int) (int64, error) {
	if s == nil || s.pool == nil || limit < 1 || limit > 1000 {
		return 0, app.ErrUnavailable
	}
	return q.New(s.pool).DeleteExpiredDriverReportSnapshots(ctx, q.DeleteExpiredDriverReportSnapshotsParams{
		GeneratedAt: pgtype.Timestamptz{Time: before.UTC(), Valid: true}, Limit: int32(limit),
	})
}
