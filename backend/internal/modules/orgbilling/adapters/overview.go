package adapters

import (
	"context"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OverviewStore struct {
	Pool         *pgxpool.Pool
	Authority    app.ReadAuthority
	Capabilities capability.Catalog
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
	for _, current := range subscriptions {
		key := domain.PlanKey{Product: current.ProductCode, Plan: current.PlanCode, Revision: current.PlanRevision}
		plan, err := catalog.GetPlan(ctx, key)
		if err != nil {
			return app.Overview{}, err
		}
		term := domain.Period{Start: current.TermStart.Time.UTC(), End: current.TermEnd.Time.UTC()}
		state := domain.SubscriptionState(current.State)
		paid, err := queries.IsPlanTermSettled(ctx, db.IsPlanTermSettledParams{
			OrgID: org, SubscriptionID: current.ID, ProductCode: key.Product, PlanCode: key.Plan, PlanRevision: key.Revision,
			TermStart: current.TermStart, TermEnd: current.TermEnd,
		})
		if err != nil {
			return app.Overview{}, err
		}
		summary := app.PlanSummary{Catalog: plan, State: state, Term: term, ConfirmedPaid: paid,
			TermCurrent: !overview.ObservedAt.Before(term.Start) && overview.ObservedAt.Before(term.End)}
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
