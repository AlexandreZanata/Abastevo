package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	qcap "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// ObserveCompanyDispute runs only after the provider has confirmed the dispute
// facts. Matched company disputes are never written to the personal ledger.
func (intake BillingIntake) ObserveCompanyDispute(ctx context.Context, event payment.InboxEvent, fact payment.DisputeFact) (bool, error) {
	if fact.Provider != event.Provider || fact.Environment != event.Environment || fact.Merchant != event.Merchant {
		return false, paydomain.ErrBinding
	}
	charge := fact.OriginalChargeID
	if fact.Provider == paydomain.ProviderAsaas {
		charge = fact.RemotePaymentID
	}
	if charge == "" {
		return false, nil
	}
	return intake.observeReversal(ctx, event, charge, fact.ExternalReference, fact.Amount, fact.State == paydomain.DisputeLost, false)
}

func (intake BillingIntake) observeReversal(ctx context.Context, event payment.InboxEvent, charge, externalReference string, amount paydomain.Money, reverse, fullRefund bool) (bool, error) {
	notice := domain.BillingNotice{Provider: event.Provider, Environment: event.Environment, Merchant: event.Merchant,
		EventID: event.EventID, PayloadHash: event.PayloadHash, ResourceRef: event.ResourceID}
	if intake.Pool == nil || intake.Now == nil {
		return false, paydomain.ErrUnavailable
	}
	if notice.Validate() != nil || charge == "" || len(charge) > 255 || amount.Positive() != nil || amount.Currency != "BRL" {
		return false, domain.ErrBillingBinding
	}
	identity := db.LocateCompanySettlementParams{Provider: string(event.Provider), Environment: string(event.Environment),
		Merchant: event.Merchant, ChargeRef: charge}
	var org string
	err := database.WithOrgBootstrapTx(ctx, intake.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.SetCanonicalChargeLocator(ctx, db.SetCanonicalChargeLocatorParams(identity)); err != nil {
			return err
		}
		var err error
		org, err = q.LocateCompanySettlement(ctx, identity)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !reverse {
		return true, nil
	}
	orgID, err := kernel.ParseOrgID(org)
	if err != nil {
		return true, domain.ErrBillingBinding
	}
	scope, err := kernel.NewOrgScope(orgID)
	if err != nil {
		return true, err
	}
	return true, database.WithOrgTx(ctx, intake.Pool, scope, func(tx pgx.Tx) error {
		org := orgID.String()
		if err := qcap.New(tx).LockEntitlementWrite(ctx, org); err != nil {
			return err
		}
		q := db.New(tx)
		receipt, err := q.ReadCompanyReversibleSettlement(ctx, db.ReadCompanyReversibleSettlementParams{
			OrgID: org, Provider: identity.Provider, Environment: identity.Environment, Merchant: identity.Merchant, ChargeRef: charge,
		})
		if err != nil {
			return err
		}
		if event.Provider == paydomain.ProviderAsaas && (externalReference == "" || externalReference != receipt.AsaasExternalReference) {
			return paydomain.ErrBinding
		}
		if amount.Centavos > receipt.AmountCentavos || (fullRefund && amount.Centavos != receipt.AmountCentavos) {
			return paydomain.ErrBinding
		}
		current, err := q.GetSubscription(ctx, db.GetSubscriptionParams{OrgID: org, SubscriptionID: receipt.SubscriptionID})
		if err != nil {
			return err
		}
		now := intake.Now().UTC()
		if domain.SubscriptionState(current.State) != domain.Active ||
			!current.TermStart.Time.Equal(receipt.TermStart.Time) || !current.TermEnd.Time.Equal(receipt.TermEnd.Time) ||
			now.Before(current.OccurredAt.Time) || !now.Before(current.TermEnd.Time) {
			return nil
		}
		// Different provider event IDs for the same reversed charge converge
		// even if an operator intervenes between deliveries.
		notice.EventID = "reversal:" + charge
		ref := notice.Reference()
		already, err := q.CompanyReversalAlreadyObserved(ctx, db.CompanyReversalAlreadyObservedParams{
			OrgID: org, SubscriptionID: receipt.SubscriptionID, EvidenceReference: ref,
		})
		if err != nil || already {
			return err
		}
		_, err = intake.Ledger.ApplyTx(ctx, tx, domain.PlanKey{Product: current.ProductCode, Plan: current.PlanCode, Revision: current.PlanRevision},
			domain.LifecycleEvent{Scope: scope, SubscriptionID: receipt.SubscriptionID, ExpectedRevision: current.Revision,
				Kind: domain.ReportPastDue, Source: domain.VerifiedBilling, Reference: ref, At: now})
		return err
	})
}

type companyRefundVerifier interface {
	VerifyCompanyRefund(context.Context, payment.InboxRecord) (payment.ChargeRefundFact, error)
}

// CompanyRefund routes a verified refund notification to a canonical read, then
// changes company state only for the exact prior settlement of the current term.
type CompanyRefund struct {
	Stripe, Asaas companyRefundVerifier
	Intake        BillingIntake
	Inbox         settlementInbox
}

func (s CompanyRefund) Process(ctx context.Context, event payment.InboxRecord) error {
	var verifier companyRefundVerifier
	switch event.Provider {
	case paydomain.ProviderStripe:
		verifier = s.Stripe
	case paydomain.ProviderAsaas:
		verifier = s.Asaas
	}
	if verifier == nil || s.Inbox == nil {
		return paydomain.ErrUnavailable
	}
	fact, err := verifier.VerifyCompanyRefund(ctx, event)
	if err == nil && fact.Full {
		_, err = s.Intake.observeReversal(ctx, event.InboxEvent, fact.ChargeID, fact.ExternalReference, fact.Amount, true, true)
	}
	switch {
	case err == nil:
		return s.Inbox.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
	case errors.Is(err, paydomain.ErrBinding), errors.Is(err, paydomain.ErrInvalid),
		errors.Is(err, paydomain.ErrConflict), errors.Is(err, domain.ErrBillingBinding):
		return s.Inbox.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "organization-refund-binding")
	default:
		return err
	}
}
