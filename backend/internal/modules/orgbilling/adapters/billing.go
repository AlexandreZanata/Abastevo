package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
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

func (intake BillingIntake) recordTx(ctx context.Context, tx pgx.Tx, notice domain.BillingNotice, fact domain.CanonicalBillingFact) (SettlementReceipt, error) {
	org := fact.Scope.OrgID().String()
	provider, environment := string(notice.Provider), string(notice.Environment)
	queries := db.New(tx)
	row, err := queries.GetChargeBinding(ctx, db.GetChargeBindingParams{
		OrgID: org, Provider: provider, Environment: environment, Merchant: notice.Merchant, ResourceRef: notice.ResourceRef,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementReceipt{}, domain.ErrBillingBinding
	}
	if err != nil {
		return SettlementReceipt{}, err
	}
	binding := domain.BillingBinding{
		Scope: fact.Scope, SubscriptionID: row.SubscriptionID,
		Offer: domain.BillingOffer{
			Plan:    domain.PlanKey{Product: row.ProductCode, Plan: row.PlanCode, Revision: row.PlanRevision},
			Channel: payment.Channel(row.Channel), Environment: payment.Environment(row.Environment),
			Merchant: row.Merchant, ProviderOfferRef: row.ProviderOfferRef,
			Price: payment.BRL(row.AmountCentavos), ApprovalRef: row.ApprovalRef, Term: payment.Term(row.Term),
		},
		ProviderCustomerRef: row.CustomerRef, ProviderContractRef: row.ContractRef,
		ExpectedTerm: domain.Period{Start: row.TermStart.Time, End: row.TermEnd.Time},
	}
	event, err := binding.SettlementEvent(notice, fact, 0, intake.Now())
	if err != nil {
		return SettlementReceipt{}, err
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
			previous.EventID == notice.EventID && previous.PayloadHash != notice.PayloadHash {
			return SettlementReceipt{}, payment.ErrConflict
		}
		return SettlementReceipt{Revision: previous.AppliedRevision, SettledAt: previous.SettledAt.Time, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return SettlementReceipt{}, err
	}

	current, err := queries.GetSubscription(ctx, db.GetSubscriptionParams{
		OrgID: org, SubscriptionID: binding.SubscriptionID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SettlementReceipt{}, err
	}
	event.ExpectedRevision = current.Revision
	transition, err := intake.Ledger.ApplyTx(ctx, tx, binding.Offer.Plan, event)
	if err != nil {
		return SettlementReceipt{}, err
	}
	rows, err := queries.RecordSettlementReceipt(ctx, db.RecordSettlementReceiptParams{
		OrgID: org, Provider: provider, Environment: environment, Merchant: notice.Merchant,
		EventID: notice.EventID, PayloadHash: notice.PayloadHash, ResourceRef: notice.ResourceRef,
		ChargeRef: fact.ChargeRef, SubscriptionID: transition.SubscriptionID, AppliedRevision: transition.Revision,
		SettledAt: pgtype.Timestamptz{Time: fact.At.UTC(), Valid: true},
	})
	if err != nil {
		return SettlementReceipt{}, err
	}
	if rows != 1 {
		return SettlementReceipt{}, domain.ErrStaleSubscription
	}
	return SettlementReceipt{Revision: transition.Revision, SettledAt: fact.At.UTC()}, nil
}
