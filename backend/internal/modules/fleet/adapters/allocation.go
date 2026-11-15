package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.AllocationHistoryRepository = (*AssetStore)(nil)
var _ app.AllocationSnapshotPort = (*AssetStore)(nil)

// allocationTime reads one stored whole-microsecond boundary.
func allocationTime(v pgtype.Timestamptz) (time.Time, error) {
	at, err := exactMeterTime(v, 0)
	if err != nil || at.IsZero() {
		return time.Time{}, app.ErrUnavailable
	}
	return at, nil
}

func allocationPeriod(from, to pgtype.Timestamptz) (fleet.AllocationPeriod, error) {
	start, err := allocationTime(from)
	if err != nil {
		return fleet.AllocationPeriod{}, err
	}
	var end *time.Time
	if to.Valid {
		at, err := allocationTime(to)
		if err != nil {
			return fleet.AllocationPeriod{}, err
		}
		end = &at
	}
	period, err := fleet.NewAllocationPeriod(start, end)
	if err != nil {
		return fleet.AllocationPeriod{}, app.ErrUnavailable
	}
	return period, nil
}

type allocationRow struct {
	target             string
	from, to, recorded pgtype.Timestamptz
}

// ListAssetAllocations reads one EMP-FLEET-005 history of one visible asset
// in start order after the keyset, between two current authority checks.
func (s *AssetStore) ListAssetAllocations(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, history app.AllocationHistory, filter app.AllocationFilter) (app.AllocationPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AllocationPage{}, err
	}
	if !validID(asset) || history != app.UnitAllocationHistory && history != app.CostCentreAllocationHistory {
		return app.AllocationPage{}, fleet.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return app.AllocationPage{}, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.AllocationPage{}, app.ErrUnavailable
	}
	page := app.AllocationPage{Entries: []app.AllocationEntry{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead); err != nil {
			return err
		}
		queries := q.New(tx)
		org := scope.OrgID().String()
		if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: asset}); err != nil {
			return err
		}
		var after pgtype.Timestamptz
		if filter.After != nil {
			after = pgtype.Timestamptz{Time: filter.After.UTC(), Valid: true}
		}
		var rows []allocationRow
		if history == app.UnitAllocationHistory {
			found, err := queries.ListAssetUnitPeriods(ctx, q.ListAssetUnitPeriodsParams{OrgID: org, AssetID: asset, AfterFrom: after, RowLimit: int32(filter.Limit + 1)})
			if err != nil {
				return err
			}
			for _, r := range found {
				rows = append(rows, allocationRow{r.TargetID, r.ValidFrom, r.ValidTo, r.RecordedAt})
			}
		} else {
			found, err := queries.ListAssetCostCentrePeriods(ctx, q.ListAssetCostCentrePeriodsParams{OrgID: org, AssetID: asset, AfterFrom: after, RowLimit: int32(filter.Limit + 1)})
			if err != nil {
				return err
			}
			for _, r := range found {
				rows = append(rows, allocationRow{r.TargetID, r.ValidFrom, r.ValidTo, r.RecordedAt})
			}
		}
		more := len(rows) > filter.Limit
		if more {
			rows = rows[:filter.Limit]
		}
		for _, row := range rows {
			period, err := allocationPeriod(row.from, row.to)
			if err != nil {
				return err
			}
			recorded, err := exactMeterTime(row.recorded, 0)
			if err != nil {
				return err
			}
			page.Entries = append(page.Entries, app.AllocationEntry{TargetID: row.target, Period: period, RecordedAt: recorded})
		}
		if more {
			next := page.Entries[len(page.Entries)-1].Period.From
			page.Next = &next
		}
		return s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead)
	})
	if err != nil {
		return app.AllocationPage{}, storeError(ctx, err)
	}
	return page, nil
}

// UnitAndCostCentreAt answers the snapshot port in its own read transaction.
func (s *AssetStore) UnitAndCostCentreAt(ctx context.Context, scope kernel.OrgScope, asset string, at time.Time) (app.AllocationSnapshot, error) {
	if scope.Validate() != nil {
		return app.AllocationSnapshot{}, fleet.ErrScope
	}
	if _, err := fleet.AllocationInstant(at); err != nil || !validID(asset) {
		return app.AllocationSnapshot{}, fleet.ErrInvalid
	}
	if s == nil || s.Pool == nil {
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	var result app.AllocationSnapshot
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		result, err = s.UnitAndCostCentreAtTx(ctx, tx, scope, asset, at)
		return err
	})
	if err != nil {
		return app.AllocationSnapshot{}, storeError(ctx, err)
	}
	return result, nil
}

// UnitAndCostCentreAtTx answers the snapshot port inside the caller's
// organization transaction with one lock-free statement (EDR-155): a missing
// or foreign asset is not found, an uncovered instant is ErrUnallocated and a
// missing cost-centre period yields no cost centre.
func (s *AssetStore) UnitAndCostCentreAtTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, asset string, at time.Time) (app.AllocationSnapshot, error) {
	if tx == nil {
		if _, err := fleet.AllocationInstant(at); err != nil || !validID(asset) {
			return app.AllocationSnapshot{}, fleet.ErrInvalid
		}
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	return s.UnitAndCostCentreAtDBTX(ctx, tx, scope, asset, at)
}

// UnitAndCostCentreAtDBTX is the same lock-free snapshot statement on a
// borrowed organization port (EMP-FUEL-002, EDR-176): the signed intake
// never exposes a pgx.Tx to the owning record module (EDR-106).
func (s *AssetStore) UnitAndCostCentreAtDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (app.AllocationSnapshot, error) {
	instant, err := fleet.AllocationInstant(at)
	if err != nil || !validID(asset) {
		return app.AllocationSnapshot{}, fleet.ErrInvalid
	}
	db = syncapp.Borrow(db)
	if s == nil || db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	row, err := q.New(db).AllocationAt(ctx, q.AllocationAtParams{At: pgtype.Timestamptz{Time: instant, Valid: true}, OrgID: scope.OrgID().String(), AssetID: asset})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.AllocationSnapshot{}, app.ErrNotFound
	}
	if err != nil {
		return app.AllocationSnapshot{}, storeError(ctx, err)
	}
	if row.AssetID != asset {
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	if row.UnitID == "" {
		if row.UnitFrom.Valid || row.CostCentreID != "" {
			return app.AllocationSnapshot{}, app.ErrUnavailable
		}
		return app.AllocationSnapshot{}, app.ErrUnallocated
	}
	// The returned periods must contain the instant; anything else is an
	// unavailable source, never a guessed answer.
	unit, err := allocationPeriod(row.UnitFrom, row.UnitTo)
	if err != nil || !unit.Contains(instant) {
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	if row.CostCentreID != "" {
		centre, err := allocationPeriod(row.CentreFrom, row.CentreTo)
		if err != nil || !centre.Contains(instant) {
			return app.AllocationSnapshot{}, app.ErrUnavailable
		}
	} else if row.CentreFrom.Valid {
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	snapshot, err := app.NewAllocationSnapshot(asset, instant, row.UnitID, row.CostCentreID)
	if err != nil {
		return app.AllocationSnapshot{}, app.ErrUnavailable
	}
	return snapshot, nil
}
