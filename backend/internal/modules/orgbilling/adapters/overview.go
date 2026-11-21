package adapters

import (
	"context"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OverviewStore struct {
	Pool           *pgxpool.Pool
	Authority      app.ReadAuthority
	Capabilities   capability.Catalog
	BillingEnabled bool
}

func (store OverviewStore) Read(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string) (app.Overview, error) {
	if store.Pool == nil || store.Authority == nil || actor.Validate() != nil {
		return app.Overview{}, app.ErrUnavailable
	}
	var view app.Overview
	err := database.WithOrgTx(ctx, store.Pool, scope, func(tx pgx.Tx) error {
		if err := store.Authority.CheckPlanReadTx(ctx, tx, scope, actor, member); err != nil {
			return err
		}
		var readErr error
		view, readErr = store.readTx(ctx, tx, scope)
		if readErr != nil {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.Overview{}, err
	}
	return view, nil
}

func (store OverviewStore) readTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) (app.Overview, error) {
	queries := db.New(tx)
	catalog := CatalogStore{DB: tx, Capabilities: store.Capabilities}
	instant, err := queries.OrganizationBillingClock(ctx)
	if err != nil || !instant.Valid {
		return app.Overview{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	subscriptions, err := queries.ListCurrentSubscriptions(ctx, org)
	if err != nil {
		return app.Overview{}, err
	}
	overview := app.Overview{ObservedAt: instant.Time.UTC(), Plans: make([]app.PlanSummary, 0, len(subscriptions))}
	var storageUsed *int64
	var vehicleCount, stationCount *int64
	for _, current := range subscriptions {
		key := domain.PlanKey{Product: current.ProductCode, Plan: current.PlanCode, Revision: current.PlanRevision}
		plan, err := catalog.GetPlan(ctx, key)
		if err != nil {
			return app.Overview{}, err
		}
		term := domain.Period{Start: current.TermStart.Time.UTC(), End: current.TermEnd.Time.UTC()}
		state := domain.SubscriptionState(current.State)
		receipts, err := queries.ReadPlanTermReceipts(ctx, db.ReadPlanTermReceiptsParams{
			OrgID: org, SubscriptionID: current.ID, ProductCode: key.Product, PlanCode: key.Plan, PlanRevision: key.Revision,
			TermStart: current.TermStart, TermEnd: current.TermEnd,
		})
		if err != nil {
			return app.Overview{}, err
		}
		summary := app.PlanSummary{Catalog: plan, State: state, Term: term,
			TermCurrent: !overview.ObservedAt.Before(term.Start) && overview.ObservedAt.Before(term.End)}
		if _, err := plan.Limit(domain.StorageBytes); err == nil {
			if storageUsed == nil {
				used, err := queries.CountDocumentStorageBytes(ctx, org)
				if err != nil {
					return app.Overview{}, err
				}
				storageUsed = &used
			}
			summary.StorageUsedBytes = storageUsed
		}
		if _, available := plan.Limit(domain.ActiveStations); store.BillingEnabled && available == nil {
			if stationCount == nil {
				total, err := queries.CountAdmittedStations(ctx, db.CountAdmittedStationsParams{OrgID: org, ObservedAt: instant})
				if err != nil {
					return app.Overview{}, err
				}
				stationCount = &total
			}
			summary.ActiveStationsUsed = stationCount
		}
		if _, available := plan.Limit(domain.ActiveVehicles); store.BillingEnabled && available == nil {
			if vehicleCount == nil {
				total, err := queries.CountPlanSelectionActiveVehicles(ctx, org)
				if err != nil {
					return app.Overview{}, err
				}
				vehicleCount = &total
			}
			summary.ActiveVehiclesUsed = vehicleCount
		}
		if summary.ActiveVehiclesUsed != nil && summary.TermCurrent && current.State == string(domain.Active) && current.Revision > 1 {
			basis, err := queries.ReadVehicleDowngrade(ctx, db.ReadVehicleDowngradeParams{
				OrgID: org, SubscriptionID: current.ID, SubscriptionRevision: current.Revision, EvaluatedAt: instant,
			})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return app.Overview{}, err
			}
			if err == nil && *vehicleCount > basis.Ceiling {
				summary.VehicleSelection = &app.VehicleSelectionBasis{SubscriptionID: current.ID, Revision: current.Revision,
					Active: *vehicleCount, Ceiling: basis.Ceiling}
			}
		}
		if len(receipts) > 1 {
			return app.Overview{}, app.ErrUnavailable
		}
		if len(receipts) == 1 {
			row := receipts[0]
			amount := payment.BRL(row.AmountCentavos)
			if !row.SettledAt.Valid || amount.Positive() != nil {
				return app.Overview{}, app.ErrUnavailable
			}
			summary.Receipt = &app.TermReceipt{Amount: amount, SettledAt: row.SettledAt.Time.UTC()}
			if row.ProviderPaidOn.Valid {
				summary.Receipt.PaidOn = row.ProviderPaidOn.Time.Format(time.DateOnly)
			}
		}
		if state != domain.Cancelled && state != domain.Cancelling {
			pending, err := queries.ListReviewedNextPlans(ctx, db.ListReviewedNextPlansParams{
				OrgID: org, SubscriptionID: current.ID, ProductCode: key.Product, PlanCode: key.Plan,
				PlanRevision: key.Revision, NextStart: current.TermEnd, ObservedAt: instant,
			})
			if err != nil {
				return app.Overview{}, err
			}
			if len(pending) == 1 {
				next := pending[0]
				target, err := catalog.GetPlan(ctx, domain.PlanKey{Product: next.ProductCode, Plan: next.PlanCode, Revision: next.PlanRevision})
				if err != nil {
					return app.Overview{}, err
				}
				summary.Pending = &app.PendingPlan{Catalog: target, EffectiveAt: next.TermStart.Time.UTC()}
			}
		}
		overview.Plans = append(overview.Plans, summary)
	}
	return overview, nil
}
