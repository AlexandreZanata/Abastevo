package adapters

import (
	"context"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
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
		var err error
		governed, allowed, err = store.PlanCapabilityDBTX(ctx, tx, scope, code)
		return err
	})
	if err != nil {
		return false, false, err
	}
	return governed, allowed, nil
}

// PlanCapabilityDBTX reads the current plan in a retained organization
// transaction. Write callers hold the shared organization entitlement lock.
func (store EntitlementStore) PlanCapabilityDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, code string) (bool, bool, error) {
	governed, plans, err := store.entitledPlans(ctx, tx, scope, code)
	return governed, len(plans) > 0, err
}

// CheckVehicleActivation consumes the count from the fleet transaction
// holding the organization root lock. The transaction retains the entitlement
// lock from its authorization step through the asset write.
func (store EntitlementStore) CheckVehicleActivation(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, active int64) error {
	governed, plans, err := store.entitledPlans(ctx, tx, scope, "organizations")
	if err != nil || !governed {
		return err
	}
	if len(plans) == 0 {
		return access.ErrEntitlementRequired
	}
	var selected domain.Plan
	var ceiling int64
	for _, plan := range plans {
		candidate, err := plan.Limit(domain.ActiveVehicles)
		if errors.Is(err, domain.ErrLimitUndefined) {
			continue
		}
		if err != nil {
			return err
		}
		if selected.Key() != (domain.PlanKey{}) && candidate != ceiling {
			return capability.ErrUnavailable
		}
		selected, ceiling = plan, candidate
	}
	err = selected.CheckIncrease(domain.ActiveVehicles, active, 1)
	if errors.Is(err, domain.ErrLimitUndefined) || errors.Is(err, domain.ErrLimitReached) {
		return access.ErrLimitReached
	}
	return err
}

func (store EntitlementStore) entitledPlans(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, code string) (bool, []domain.Plan, error) {
	if store.Now == nil || store.Capabilities.Validate(code) != nil {
		return false, nil, capability.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return false, nil, err
	}
	current, err := db.New(tx).ListCurrentSubscriptions(ctx, scope.OrgID().String())
	if err != nil {
		return false, nil, err
	}
	now := store.Now()
	catalog := CatalogStore{DB: tx, Capabilities: store.Capabilities}
	var plans []domain.Plan
	for _, row := range current {
		key := domain.PlanKey{Product: row.ProductCode, Plan: row.PlanCode, Revision: row.PlanRevision}
		subscription, err := domain.RestoreSubscription(domain.SubscriptionSnapshot{
			Scope: scope, SubscriptionID: row.ID, Plan: key,
			State: domain.SubscriptionState(row.State), PriorState: domain.SubscriptionState(row.PriorState),
			Revision: row.Revision, LastEventAt: row.OccurredAt.Time,
			Period: domain.Period{Start: row.TermStart.Time, End: row.TermEnd.Time},
		})
		if err != nil {
			return false, nil, err
		}
		plan, err := catalog.GetPlan(ctx, key)
		if err != nil {
			return false, nil, err
		}
		if domain.CapabilityAllowed(scope, subscription, plan, code, now) {
			plans = append(plans, plan)
		}
	}
	return len(current) > 0, plans, nil
}
