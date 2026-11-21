package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	qcap "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SettlementReceipt struct {
	Revision  int64
	SettledAt time.Time
	Replayed  bool
}

type BillingIntake struct {
	Pool   *pgxpool.Pool
	Ledger SubscriptionLedger
	Now    func() time.Time
}

// RecordSettlement accepts canonical provider facts from the verified inbox
// worker. A receipt and its lifecycle decision always commit together.
func (intake BillingIntake) RecordSettlement(ctx context.Context, notice domain.BillingNotice, fact domain.CanonicalBillingFact) (SettlementReceipt, error) {
	if intake.Pool == nil || intake.Now == nil {
		return SettlementReceipt{}, payment.ErrUnavailable
	}
	if notice.Validate() != nil || fact.Notice != notice || fact.Scope.Validate() != nil {
		return SettlementReceipt{}, domain.ErrBillingBinding
	}
	for range 2 {
		var receipt SettlementReceipt
		err := database.WithOrgTx(ctx, intake.Pool, fact.Scope, func(tx pgx.Tx) error {
			var err error
			receipt, err = intake.recordTx(ctx, tx, notice, fact)
			return err
		})
		if !errors.Is(err, domain.ErrStaleSubscription) {
			if err != nil {
				return SettlementReceipt{}, err
			}
			return receipt, nil
		}
	}
	return SettlementReceipt{}, domain.ErrStaleSubscription
}

func readChargeBinding(ctx context.Context, queries *db.Queries, scope kernel.OrgScope, notice domain.BillingNotice) (domain.BillingBinding, error) {
	org := scope.OrgID().String()
	provider, environment := string(notice.Provider), string(notice.Environment)
	row, err := queries.GetChargeBinding(ctx, db.GetChargeBindingParams{
		OrgID: org, Provider: provider, Environment: environment, Merchant: notice.Merchant, ResourceRef: notice.ResourceRef,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BillingBinding{}, domain.ErrChargeBindingMissing
	}
	if err != nil {
		return domain.BillingBinding{}, err
	}
	return domain.BillingBinding{
		Scope: scope, SubscriptionID: row.SubscriptionID,
		Offer: domain.BillingOffer{
			Plan:    domain.PlanKey{Product: row.ProductCode, Plan: row.PlanCode, Revision: row.PlanRevision},
			Channel: payment.Channel(row.Channel), Environment: payment.Environment(row.Environment),
			Merchant: row.Merchant, ProviderOfferRef: row.ProviderOfferRef,
			Price: payment.BRL(row.AmountCentavos), ApprovalRef: row.ApprovalRef, Term: payment.Term(row.Term),
		},
		ProviderCustomerRef: row.CustomerRef, ProviderContractRef: row.ContractRef, AsaasExternalRef: row.AsaasExternalReference,
		ExpectedTerm: domain.Period{Start: row.TermStart.Time, End: row.TermEnd.Time},
	}, nil
}

func (intake BillingIntake) recordTx(ctx context.Context, tx pgx.Tx, notice domain.BillingNotice, fact domain.CanonicalBillingFact) (SettlementReceipt, error) {
	org := fact.Scope.OrgID().String()
	provider, environment := string(notice.Provider), string(notice.Environment)
	queries := db.New(tx)
	binding, err := readChargeBinding(ctx, queries, fact.Scope, notice)
	if err != nil {
		return SettlementReceipt{}, err
	}
	event, eventErr := binding.SettlementEvent(notice, fact, 0, intake.Now())
	// Asaas provides the paid day, while the inbox supplies the observation
	// instant. A delayed notification can still identify an existing receipt.
	if eventErr != nil && !(notice.Provider == payment.ProviderAsaas && errors.Is(eventErr, domain.ErrSettlementAfterTerm)) {
		return SettlementReceipt{}, eventErr
	}
	// Serialize different notifications for the same canonical charge across
	// organizations, before checking the durable receipt.
	lock, _ := json.Marshal([5]string{"organization-billing-charge", provider, environment, notice.Merchant, fact.ChargeRef})
	if err := queries.LockBillingCharge(ctx, string(lock)); err != nil {
		return SettlementReceipt{}, err
	}
	previous, err := queries.GetSettlementReceipt(ctx, db.GetSettlementReceiptParams{
		OrgID: org, Provider: provider, Environment: environment, Merchant: notice.Merchant,
		EventID: notice.EventID, ChargeRef: fact.ChargeRef,
	})
	if err == nil {
		if previous.ResourceRef != notice.ResourceRef || previous.ChargeRef != fact.ChargeRef ||
			previous.SubscriptionID != fact.SubscriptionID ||
			previous.ProviderPaidOn.Valid != !fact.ProviderPaidOn.IsZero() ||
			previous.ProviderPaidOn.Valid && previous.ProviderPaidOn.Time.Format(time.DateOnly) != fact.ProviderPaidOn.Format(time.DateOnly) ||
			previous.EventID == notice.EventID && previous.PayloadHash != notice.PayloadHash {
			return SettlementReceipt{}, payment.ErrConflict
		}
		return SettlementReceipt{Revision: previous.AppliedRevision, SettledAt: previous.SettledAt.Time, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SettlementReceipt{}, err
	}
	if eventErr != nil {
		return SettlementReceipt{}, eventErr
	}

	// Keep the term-wide receipt check and lifecycle revision under the
	// subscription writer lock, including a concurrent past-due transition.
	if err := qcap.New(tx).LockEntitlementWrite(ctx, org); err != nil {
		return SettlementReceipt{}, err
	}
	paid, err := queries.ReadPlanTermReceipts(ctx, db.ReadPlanTermReceiptsParams{
		OrgID: org, SubscriptionID: binding.SubscriptionID,
		ProductCode: binding.Offer.Plan.Product, PlanCode: binding.Offer.Plan.Plan, PlanRevision: binding.Offer.Plan.Revision,
		TermStart: pgtype.Timestamptz{Time: binding.ExpectedTerm.Start, Valid: true},
		TermEnd:   pgtype.Timestamptz{Time: binding.ExpectedTerm.End, Valid: true},
	})
	if err != nil {
		return SettlementReceipt{}, err
	}
	if len(paid) != 0 {
		return SettlementReceipt{}, payment.ErrConflict
	}

	current, err := queries.GetSubscription(ctx, db.GetSubscriptionParams{
		OrgID: org, SubscriptionID: binding.SubscriptionID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SettlementReceipt{}, err
	}
	event.ExpectedRevision = current.Revision
	if current.Revision > 0 {
		previousPlan := domain.PlanKey{Product: current.ProductCode, Plan: current.PlanCode, Revision: current.PlanRevision}
		if previousPlan != binding.Offer.Plan {
			event.PlanChange = binding.Offer.Plan
		}
	}
	transition, err := intake.Ledger.ApplyTx(ctx, tx, binding.Offer.Plan, event)
	if err != nil {
		return SettlementReceipt{}, err
	}
	rows, err := queries.RecordSettlementReceipt(ctx, db.RecordSettlementReceiptParams{
		OrgID: org, Provider: provider, Environment: environment, Merchant: notice.Merchant,
		EventID: notice.EventID, PayloadHash: notice.PayloadHash, ResourceRef: notice.ResourceRef,
		ChargeRef: fact.ChargeRef, SubscriptionID: transition.SubscriptionID, AppliedRevision: transition.Revision,
		SettledAt:      pgtype.Timestamptz{Time: fact.At.UTC(), Valid: true},
		ProviderPaidOn: pgtype.Date{Time: fact.ProviderPaidOn, Valid: !fact.ProviderPaidOn.IsZero()},
	})
	if err != nil {
		return SettlementReceipt{}, err
	}
	if rows != 1 {
		// The receipt's immutable provider identity already exists. Roll back
		// the lifecycle write and let the inbox record a reconciliation conflict.
		return SettlementReceipt{}, payment.ErrConflict
	}
	return SettlementReceipt{Revision: transition.Revision, SettledAt: fact.At.UTC()}, nil
}
