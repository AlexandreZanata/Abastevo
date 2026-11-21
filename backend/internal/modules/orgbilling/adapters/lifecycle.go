package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	qcap "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SubscriptionLedger struct{ Capabilities capability.Catalog }

// ApplyTx appends a lifecycle decision in the caller's verified organization
// transaction. Provider receipts and audit writes can share its commit.
func (ledger SubscriptionLedger) ApplyTx(ctx context.Context, tx pgx.Tx, key domain.PlanKey, event domain.LifecycleEvent) (domain.Transition, error) {
	if err := database.ValidateOrgTx(ctx, tx, event.Scope); err != nil {
		return domain.Transition{}, err
	}
	sub, err := domain.NewSubscription(event.Scope, event.SubscriptionID, key)
	if err != nil {
		return domain.Transition{}, err
	}
	queries := db.New(tx)
	org := event.Scope.OrgID().String()
	if err := qcap.New(tx).LockEntitlementWrite(ctx, org); err != nil {
		return domain.Transition{}, err
	}
	row, err := queries.GetSubscription(ctx, db.GetSubscriptionParams{OrgID: org, SubscriptionID: event.SubscriptionID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := (CatalogStore{DB: tx, Capabilities: ledger.Capabilities}).GetPlan(ctx, key); err != nil {
			return domain.Transition{}, err
		}
	case err != nil:
		return domain.Transition{}, err
	default:
		durableKey := domain.PlanKey{Product: row.ProductCode, Plan: row.PlanCode, Revision: row.PlanRevision}
		if durableKey != key {
			return domain.Transition{}, domain.ErrUnboundEvidence
		}
		sub, err = domain.RestoreSubscription(domain.SubscriptionSnapshot{
			Scope: event.Scope, SubscriptionID: event.SubscriptionID, Plan: durableKey,
			State: domain.SubscriptionState(row.State), PriorState: domain.SubscriptionState(row.PriorState),
			Revision: row.Revision, LastEventAt: row.OccurredAt.Time,
			Period: domain.Period{Start: row.TermStart.Time, End: row.TermEnd.Time},
		})
		if err != nil {
			return domain.Transition{}, err
		}
	}

	next, transition, err := sub.Apply(event)
	if err != nil {
		return domain.Transition{}, err
	}
	state := next.Snapshot()
	rows, err := queries.RecordSubscriptionEvent(ctx, db.RecordSubscriptionEventParams{
		OrgID: org, SubscriptionID: state.SubscriptionID, Revision: state.Revision,
		ProductCode: state.Plan.Product, PlanCode: state.Plan.Plan, PlanRevision: state.Plan.Revision,
		FromState: string(transition.From), State: string(state.State), PriorState: string(state.PriorState),
		EventKind: string(transition.Kind), EventSource: string(transition.Source), EvidenceReference: transition.Reference,
		OccurredAt: pgtype.Timestamptz{Time: state.LastEventAt, Valid: true},
		TermStart:  pgtype.Timestamptz{Time: state.Period.Start, Valid: true},
		TermEnd:    pgtype.Timestamptz{Time: state.Period.End, Valid: true},
	})
	if err != nil {
		return domain.Transition{}, err
	}
	if rows != 1 {
		return domain.Transition{}, domain.ErrStaleSubscription
	}
	return transition, nil
}
