package adapters

import (
	"context"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EntitlementStore struct {
	Pool         *pgxpool.Pool
	Capabilities capability.Catalog
	Now          func() time.Time
}

// PlanCapability reads current plan facts after normal organization authority
// has resolved the scope. Pilot flags are authoritative only until an
// organization has subscription history.
func (store EntitlementStore) PlanCapability(ctx context.Context, scope kernel.OrgScope, code string) (governed, allowed bool, err error) {
	if store.Pool == nil || store.Now == nil || store.Capabilities.Validate(code) != nil {
		return false, false, capability.ErrUnavailable
	}

	err = database.WithOrgTx(ctx, store.Pool, scope, func(tx pgx.Tx) error {
		current, err := db.New(tx).ListCurrentSubscriptions(ctx, scope.OrgID().String())
		if err != nil {
			return err
		}
		governed = len(current) > 0
		now := store.Now()
		catalog := CatalogStore{DB: tx, Capabilities: store.Capabilities}
		for _, row := range current {
			key := domain.PlanKey{Product: row.ProductCode, Plan: row.PlanCode, Revision: row.PlanRevision}
			subscription, err := domain.RestoreSubscription(domain.SubscriptionSnapshot{
				Scope: scope, SubscriptionID: row.ID, Plan: key,
				State: domain.SubscriptionState(row.State), PriorState: domain.SubscriptionState(row.PriorState),
				Revision: row.Revision, LastEventAt: row.OccurredAt.Time,
				Period: domain.Period{Start: row.TermStart.Time, End: row.TermEnd.Time},
			})
			if err != nil {
				return err
			}
			plan, err := catalog.GetPlan(ctx, key)
			if err != nil {
				return err
			}
			if domain.CapabilityAllowed(scope, subscription, plan, code, now) {
				allowed = true
			}
		}
		return nil
	})
	if err != nil {
		return false, false, err
	}
	return governed, allowed, nil
}
