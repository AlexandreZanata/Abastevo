// Package adapters persists organization audit through scoped transactions.
package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	qorgaudit "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaudit"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

type Store struct {
	pool    *pgxpool.Pool
	catalog domain.Catalog
}

var (
	_ application.Runner = (*Store)(nil)
	_ application.Reader = (*Store)(nil)
)

func NewStore(pool *pgxpool.Pool, catalog domain.Catalog) *Store {
	return &Store{pool: pool, catalog: catalog}
}

func (store *Store) RecordChange(ctx context.Context, scope kernel.OrgScope, event domain.Event, work func(context.Context, pgx.Tx) error) (bool, error) {
	if err := scope.Validate(); err != nil {
		return false, err
	}
	if err := store.catalog.Validate(event); err != nil {
		return false, err
	}
	if work == nil {
		return false, domain.ErrInvalid
	}
	var replayed bool
	var workErr error
	err := database.WithOrgTx(ctx, store.pool, scope, func(tx pgx.Tx) error {
		queries := qorgaudit.New(tx)
		_, err := queries.ReserveEvent(ctx, qorgaudit.ReserveEventParams{
			OrgID: scope.OrgID().String(), OperationID: event.OperationID,
			ActorID: event.ActorID, DelegationID: event.DelegationID,
			Action: event.Action, TargetType: event.TargetType, TargetID: event.TargetID,
			ReasonCode: event.ReasonCode, RequestHash: event.RequestHash,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			row, err := queries.GetEvent(ctx, qorgaudit.GetEventParams{
				OrgID: scope.OrgID().String(), OperationID: event.OperationID,
			})
			if err != nil {
				return domain.ErrUnavailable
			}
			if record(row).Event != event {
				return domain.ErrReused
			}
			replayed = true
			return nil
		}
		if err != nil {
			return domain.ErrUnavailable
		}
		workErr = work(ctx, tx)
		return workErr
	})
	if err != nil {
		if workErr != nil {
			return false, workErr
		}
		if errors.Is(err, domain.ErrReused) {
			return false, domain.ErrReused
		}
		return false, domain.ErrUnavailable
	}
	return replayed, nil
}

// List is the storage port. Callers enter through application.Service.List,
// which checks current organization-wide audit authority before storage.
func (store *Store) List(ctx context.Context, scope kernel.OrgScope, filter domain.Filter) ([]domain.Record, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	filter, err := filter.Normalize()
	if err != nil {
		return nil, err
	}
	records := make([]domain.Record, 0)
	err = database.WithOrgTx(ctx, store.pool, scope, func(tx pgx.Tx) error {
		rows, err := qorgaudit.New(tx).ListEvents(ctx, qorgaudit.ListEventsParams{
			OrgID: scope.OrgID().String(), RowLimit: int32(filter.Limit),
			BeforeAt: pgtype.Timestamptz{Time: filter.BeforeAt, Valid: !filter.BeforeAt.IsZero()},
			BeforeID: filter.BeforeOperationID,
		})
		if err != nil {
			return domain.ErrUnavailable
		}
		for _, row := range rows {
			records = append(records, record(qorgaudit.GetEventRow(row)))
		}
		return nil
	})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	return records, nil
}

func record(row qorgaudit.GetEventRow) domain.Record {
	return domain.Record{
		Event: domain.Event{
			OperationID: row.OperationID, ActorID: row.ActorID, DelegationID: row.DelegationID,
			Action: row.Action, TargetType: row.TargetType, TargetID: row.TargetID,
			ReasonCode: row.ReasonCode, RequestHash: row.RequestHash,
		},
		At: row.RecordedAt.Time,
	}
}
