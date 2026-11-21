package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	orgdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	watchTarget  = "data.watch"
	watchCreate  = "data.watch.create"
	watchDisable = "data.watch.disable"
	watchObserve = "data.watch.observe"
	watchCadence = 24 * time.Hour
)

var ErrWatchConflict = errors.New("orgdata: conflicting watch")

func WatchAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{TargetType: watchTarget, Action: watchCreate},
		{TargetType: watchTarget, Action: watchDisable},
		{TargetType: watchTarget, Action: watchObserve},
	}
}

type DataPlanSource interface {
	DataPlanAt(context.Context, pgx.Tx, kernel.OrgScope, time.Time) (billing.PlanKey, error)
}

type WatchStore struct {
	Pool      *pgxpool.Pool
	Authority app.KeyAuthority
	Plans     DataPlanSource
	Audit     auditing.TransactionRunner
	Now       func() time.Time
	Enabled   bool
}

func (s WatchStore) check(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor organization.CompanyActor,
	member string, action app.Action,
) error {
	if !s.Enabled {
		return ErrDatasetNotFound
	}
	if s.Authority == nil || scope.Validate() != nil || actor.Validate() != nil || !audit.ValidID(member) {
		return app.ErrUnavailable
	}
	return s.Authority.CheckKeyActionTx(ctx, tx, scope, actor, member, action)
}

func (s WatchStore) CreateWatch(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string,
	input app.WatchInput,
) (app.Watch, bool, error) {
	if _, err := input.Fuel.Unit(); err != nil || !audit.ValidID(input.OperationID) ||
		!domain.ValidDatasetKey(input.Dataset) || input.Revision < 1 {
		return app.Watch{}, false, domain.ErrInvalidDataset
	}
	return s.change(ctx, scope, actor, member, input.OperationID, input.OperationID, &input)
}

func (s WatchStore) DisableWatch(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, id, operation string) (app.Watch, bool, error) {
	if !audit.ValidID(id) || !audit.ValidID(operation) {
		return app.Watch{}, false, domain.ErrInvalidDataset
	}
	return s.change(ctx, scope, actor, member, id, operation, nil)
}

func (s WatchStore) change(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, id, operation string,
	create *app.WatchInput,
) (app.Watch, bool, error) {
	if !s.Enabled {
		return app.Watch{}, false, ErrDatasetNotFound
	}
	if s.Pool == nil || s.Audit == nil || s.Plans == nil || s.Now == nil {
		return app.Watch{}, false, app.ErrUnavailable
	}
	permission, action := app.RevokeKey, watchDisable
	if create != nil {
		permission, action = app.IssueKey, watchCreate
	}
	org := scope.OrgID().String()
	hash := watchDigest(org, member, id, action, create)
	event := audit.Event{OperationID: operation, ActorID: actor.AccountID, TargetType: watchTarget, TargetID: id, Action: action, RequestHash: hash}
	var result app.Watch
	var replayed bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, actor, member, permission); err != nil {
			return err
		}
		var err error
		replayed, err = s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			q := db.New(tx)
			if create != nil {
				return s.create(ctx, tx, scope, actor.AccountID, member, *create)
			}
			row, err := q.ReadWatch(ctx, db.ReadWatchParams{OrgID: org, WatchID: id})
			if errors.Is(err, pgx.ErrNoRows) || err == nil && row.MemberID != member {
				return ErrDatasetNotFound
			}
			if err != nil {
				return err
			}
			if !row.Active {
				return ErrWatchConflict
			}
			count, err := q.DisableWatch(ctx, db.DisableWatchParams{OrgID: org, WatchID: id, Member: member})
			if err == nil && count != 1 {
				return ErrWatchConflict
			}
			return err
		})
		if err != nil {
			return err
		}
		row, err := db.New(tx).ReadWatch(ctx, db.ReadWatchParams{OrgID: org, WatchID: id})
		if errors.Is(err, pgx.ErrNoRows) || err == nil && row.MemberID != member {
			return ErrDatasetNotFound
		}
		if err != nil {
			return err
		}
		result = projectWatch(db.ListWatchesRow{ID: row.ID, DatasetKey: row.DatasetKey, FuelProduct: row.FuelProduct,
			BaselineRevision: row.BaselineRevision, LastRevision: row.LastRevision, Active: row.Active})
		return nil
	})
	if err != nil {
		return app.Watch{}, false, err
	}
	return result, replayed, nil
}

func (s WatchStore) create(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, account, member string, input app.WatchInput) error {
	q := db.New(tx)
	org := scope.OrgID().String()
	state, err := q.LockDataOrganization(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && state != string(orgdomain.Active) {
		return ErrDatasetNotFound
	}
	if err != nil {
		return err
	}
	at := s.Now().UTC()
	plan, err := s.Plans.DataPlanAt(ctx, tx, scope, at)
	if err != nil {
		return err
	}
	if _, err := watchObservation(ctx, tx, plan, input.Dataset, input.Revision, input.Fuel); err != nil {
		return err
	}
	due := at.Add(watchCadence).Unix()
	n, err := q.CreateWatch(ctx, db.CreateWatchParams{
		OrgID: org, WatchID: input.OperationID, Member: member, Account: account, DatasetKey: input.Dataset,
		FuelProduct: string(input.Fuel), BaselineRevision: input.Revision, NextCheck: due,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrWatchConflict
	}
	return scheduleWatch(ctx, tx, scope, input.OperationID, due)
}

func (s WatchStore) ListWatches(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, after string, limit int) (app.Watches, error) {
	if s.Pool == nil || limit < 1 || limit > 100 || after != "" && !audit.ValidID(after) {
		return app.Watches{}, domain.ErrInvalidDataset
	}
	page := app.Watches{Items: make([]app.Watch, 0, limit)}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.check(ctx, tx, scope, actor, member, app.ListKeys); err != nil {
			return err
		}
		rows, err := db.New(tx).ListWatches(ctx, db.ListWatchesParams{
			OrgID: scope.OrgID().String(), Member: member, AfterID: after, RowLimit: int32(limit + 1),
		})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			page.After = rows[limit-1].ID
		}
		for _, row := range rows {
			page.Items = append(page.Items, projectWatch(row))
		}
		return nil
	})
	if err != nil {
		return app.Watches{}, err
	}
	return page, nil
}

func watchObservation(ctx context.Context, tx pgx.Tx, plan billing.PlanKey, key string, revision int64, fuel kernel.Product) (domain.WatchObservation, error) {
	dataset, err := readScopedDataset(ctx, tx, plan, key, revision)
	if err != nil {
		return domain.WatchObservation{}, err
	}
	period, err := readPeriod(ctx, tx, dataset)
	if err != nil {
		return domain.WatchObservation{}, err
	}
	for _, sample := range period.Samples {
		if sample.Fuel == fuel {
			return domain.WatchObservation{Revision: revision, Region: dataset.Definition.MarketState, Method: dataset.Definition.Method,
				SurveyEnd: period.SurveyEnd, MinimumStations: period.MinimumStations, Sample: sample}, nil
		}
	}
	return domain.WatchObservation{}, ErrSeriesUnavailable
}

func projectWatch(row db.ListWatchesRow) app.Watch {
	return app.Watch{ID: row.ID, Dataset: row.DatasetKey, Fuel: kernel.Product(row.FuelProduct),
		Baseline: row.BaselineRevision, LastObserved: row.LastRevision, Active: row.Active}
}

func watchDigest(values ...any) string {
	payload, _ := json.Marshal(values)
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
