package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const vehicleSelectionAction = "subs.plan.vehicles_selected"

func VehicleSelectionAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: vehicleSelectionAction, TargetType: "subscription"}}
}

type planFleet interface {
	TransitionAssetTx(context.Context, pgx.Tx, kernel.OrgScope, fleetapp.FleetActor, string, fleetapp.AssetLifecycle) (fleetapp.AssetResult, bool, error)
}

type VehicleSelectionStore struct {
	Enabled   bool
	Pool      *pgxpool.Pool
	Authority app.PlanSelectionAuthority
	Fleet     planFleet
	Audit     auditing.TransactionRunner
	NewID     func() (string, error)
}

func (s VehicleSelectionStore) SelectVehicles(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string, input app.VehicleSelection) (app.VehicleSelectionResult, error) {
	if !s.Enabled {
		return app.VehicleSelectionResult{}, app.ErrVehicleSelectionNotFound
	}
	if s.Pool == nil || s.Authority == nil || s.Audit == nil || s.Fleet == nil || s.NewID == nil {
		return app.VehicleSelectionResult{}, app.ErrUnavailable
	}
	c, err := input.Canonical()
	if err != nil {
		return app.VehicleSelectionResult{}, err
	}
	org := scope.OrgID().String()
	raw, err := json.Marshal([]any{1, org, c.SubscriptionID, c.Revision, c.Kept})
	if err != nil {
		return app.VehicleSelectionResult{}, app.ErrUnavailable
	}
	sum := sha256.Sum256(raw)
	event := audit.Event{OperationID: c.OperationID, ActorID: actor.AccountID, Action: vehicleSelectionAction,
		TargetType: "subscription", TargetID: c.SubscriptionID, RequestHash: hex.EncodeToString(sum[:])}
	var result app.VehicleSelectionResult
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckVehicleSelectionTx(ctx, tx, scope, actor, member); err != nil {
			return err
		}
		replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			return s.applySelection(ctx, tx, scope, actor, member, c)
		})
		if errors.Is(err, audit.ErrReused) {
			return app.ErrVehicleSelectionReused
		}
		if err != nil {
			return err
		}
		row, err := db.New(tx).ReadVehicleSelection(ctx, db.ReadVehicleSelectionParams{OrgID: org, OperationID: c.OperationID})
		if err != nil || len(row.Archived) == 0 || !slices.Equal(row.Kept, c.Kept) {
			return app.ErrUnavailable
		}
		result = app.VehicleSelectionResult{Kept: append([]string{}, row.Kept...), Archived: row.Archived, Replayed: replayed}
		return s.Authority.CheckVehicleSelectionTx(ctx, tx, scope, actor, member)
	})
	if err != nil {
		return app.VehicleSelectionResult{}, err
	}
	return result, nil
}

func (s VehicleSelectionStore) applySelection(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor organization.CompanyActor,
	member string, c app.VehicleSelection,
) error {
	q := db.New(tx)
	org := scope.OrgID().String()
	now, err := q.OrganizationBillingClock(ctx)
	if err != nil || !now.Valid {
		return app.ErrUnavailable
	}
	term, err := q.ReadVehicleDowngrade(ctx, db.ReadVehicleDowngradeParams{OrgID: org,
		SubscriptionID: c.SubscriptionID, SubscriptionRevision: c.Revision, EvaluatedAt: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrVehicleSelectionNotFound
	}
	if err != nil {
		return err
	}
	rows, err := q.PlanSelectionActiveVehicles(ctx, org)
	if err != nil {
		return err
	}
	active := make([]string, 0, len(rows))
	revisions := make(map[string]int64, len(rows))
	for _, row := range rows {
		active = append(active, row.ID)
		revisions[row.ID] = row.Revision
	}
	archives, err := domain.SelectPlanArchives(active, c.Kept, term.Ceiling)
	if err != nil {
		return app.ErrVehicleSelectionConflict
	}
	saved, err := q.WriteVehicleSelection(ctx, db.WriteVehicleSelectionParams{OrgID: org, OperationID: c.OperationID,
		SubscriptionID: c.SubscriptionID, SubscriptionRevision: c.Revision, Kept: c.Kept})
	if err != nil {
		return err
	}
	if saved != 1 {
		return app.ErrVehicleSelectionConflict
	}
	fleetActor := fleetapp.FleetActor{AccountID: actor.AccountID, SessionID: actor.SessionID}
	for _, asset := range archives {
		revision, err := s.archiveVehicle(ctx, tx, scope, fleetActor, member, asset, revisions[asset])
		if err != nil {
			return err
		}
		if err := q.WriteVehiclePlanArchive(ctx, db.WriteVehiclePlanArchiveParams{OrgID: org,
			SelectionID: c.OperationID, AssetID: asset, AssetRevision: revision}); err != nil {
			return err
		}
	}
	return nil
}

func (s VehicleSelectionStore) archiveVehicle(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope,
	actor fleetapp.FleetActor, member, asset string, revision int64,
) (int64, error) {
	operation, err := s.NewID()
	if err != nil {
		return 0, app.ErrUnavailable
	}
	c, err := fleetapp.NewAssetLifecycle(fleetapp.AssetLifecycleInput{Command: fleet.Deactivate, OperationID: operation,
		AssetID: asset, ExpectedRevision: revision, Reason: "Archived by vehicle plan selection"})
	if err != nil {
		return 0, err
	}
	result, replayed, err := s.Fleet.TransitionAssetTx(ctx, tx, scope, actor, member, c)
	if err != nil {
		return 0, err
	}
	if replayed || result.AssetID != asset || result.State != fleet.Inactive || result.Revision != revision+1 {
		return 0, app.ErrUnavailable
	}
	return result.Revision, nil
}
