package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/stripe/stripe-go/v87"
)

type stripeResources interface {
	RetrieveSettlementInvoice(context.Context, payment.InboxRecord) (*stripe.Invoice, error)
	RetrieveSettlementIntent(context.Context, string) (*stripe.PaymentIntent, error)
}

type settlementRecorder interface {
	RecordSettlement(context.Context, domain.BillingNotice, domain.CanonicalBillingFact) (SettlementReceipt, error)
}

type settlementProcessor interface {
	Process(context.Context, payment.InboxRecord) error
}

type settlementInbox interface {
	MarkInbox(context.Context, payment.InboxEvent, string, string) error
	DeferInbox(context.Context, payment.InboxEvent, time.Time) error
}

// StripeSettlement distinguishes the company subscription from a personal
// checkout using the finalized invoice's subscription metadata. Webhook body
// metadata is never used to select a private organization scope.
type StripeSettlement struct {
	Resources   stripeResources
	Record      settlementRecorder
	Personal    settlementProcessor
	Inbox       settlementInbox
	Merchant    string
	Environment paydomain.Environment
}

func (s StripeSettlement) Process(ctx context.Context, event payment.InboxRecord) error {
	if s.Resources == nil || s.Inbox == nil || s.Record == nil || s.Personal == nil {
		return paydomain.ErrUnavailable
	}
	if event.Provider != paydomain.ProviderStripe || event.EventType != "invoice.paid" || event.ResourceType != "invoice" {
		return s.Personal.Process(ctx, event)
	}
	if event.Merchant != s.Merchant || event.Environment != s.Environment {
		return s.Inbox.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "stripe-merchant-binding")
	}
	invoice, err := s.Resources.RetrieveSettlementInvoice(ctx, event)
	if err != nil {
		if errors.Is(err, paydomain.ErrInvalid) {
			return s.Inbox.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "stripe-invoice-reference")
		}
		return err
	}
	if invoice == nil {
		return paydomain.ErrUnavailable
	}
	if invoice.Parent == nil || invoice.Parent.SubscriptionDetails == nil ||
		invoice.Parent.SubscriptionDetails.Metadata["abastevo_billing_scope"] != "organization" {
		return s.Personal.Process(ctx, event)
	}

	fact, err := s.billingFact(ctx, event, invoice)
	if err == nil {
		_, err = s.Record.RecordSettlement(ctx, fact.Notice, fact)
	}
	return finishOrganizationSettlement(ctx, s.Inbox, event, err)
}

func finishOrganizationSettlement(ctx context.Context, inbox settlementInbox, event payment.InboxRecord, err error) error {
	var future domain.FutureTermError
	switch {
	case err == nil:
		return inbox.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
	case errors.As(err, &future):
		return inbox.DeferInbox(ctx, event.InboxEvent, future.EffectiveAt)
	case errors.Is(err, domain.ErrBillingBinding), errors.Is(err, domain.ErrOfferUnapproved),
		errors.Is(err, domain.ErrBillingUnsettled), errors.Is(err, paydomain.ErrAmountMismatch),
		errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrUnboundEvidence),
		errors.Is(err, paydomain.ErrConflict), errors.Is(err, paydomain.ErrInvalid):
		return inbox.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "organization-settlement-binding")
	default:
		return err
	}
}

func (s StripeSettlement) billingFact(ctx context.Context, event payment.InboxRecord, invoice *stripe.Invoice) (domain.CanonicalBillingFact, error) {
	notice := domain.BillingNotice{Provider: event.Provider, Environment: event.Environment,
		Merchant: event.Merchant, EventID: event.EventID, PayloadHash: event.PayloadHash,
		ResourceRef: event.ResourceID}
	if notice.Validate() != nil || invoice == nil || invoice.ID != notice.ResourceRef || invoice.Customer == nil ||
		invoice.Livemode != (event.Environment == paydomain.EnvironmentProduction) ||
		invoice.Parent == nil || invoice.Parent.Type != stripe.InvoiceParentTypeSubscriptionDetails ||
		invoice.Parent.SubscriptionDetails == nil {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}
	if invoice.Status != stripe.InvoiceStatusPaid {
		return domain.CanonicalBillingFact{}, domain.ErrBillingUnsettled
	}
	if invoice.Currency != stripe.CurrencyBRL || invoice.AmountPaid <= 0 || invoice.AmountPaid != invoice.AmountDue ||
		invoice.AmountPaidOffStripe != 0 || invoice.CollectionMethod != stripe.InvoiceCollectionMethodChargeAutomatically ||
		invoice.StatusTransitions == nil || invoice.StatusTransitions.PaidAt <= 0 {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}
	if invoice.BillingReason != stripe.InvoiceBillingReasonSubscriptionCreate &&
		invoice.BillingReason != stripe.InvoiceBillingReasonSubscriptionCycle {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}

	details := invoice.Parent.SubscriptionDetails
	org, err := kernel.ParseOrgID(details.Metadata["abastevo_organization_id"])
	if err != nil || details.Subscription == nil || details.Subscription.ID == "" ||
		!paydomain.ValidID(details.Metadata["abastevo_organization_subscription_id"]) ||
		details.Metadata["order_id"] != "" || details.Metadata["account_id"] != "" {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}
	providerOffer, term, err := stripeSubscriptionLine(invoice, details.Subscription.ID)
	if err != nil {
		return domain.CanonicalBillingFact{}, err
	}
	intentID, err := stripeInvoiceIntent(invoice)
	if err != nil {
		return domain.CanonicalBillingFact{}, err
	}
	intent, err := s.Resources.RetrieveSettlementIntent(ctx, intentID)
	if err != nil {
		return domain.CanonicalBillingFact{}, err
	}
	chargeID, err := settledStripeCharge(invoice, intentID, intent)
	if err != nil {
		return domain.CanonicalBillingFact{}, err
	}

	return domain.CanonicalBillingFact{
		Notice: notice, Scope: scope, SubscriptionID: details.Metadata["abastevo_organization_subscription_id"],
		Channel: paydomain.ChannelCard, CustomerRef: invoice.Customer.ID, ContractRef: details.Subscription.ID,
		ResourceRef: invoice.ID, ChargeRef: chargeID, ProviderOfferRef: providerOffer, Outcome: domain.BillingSettled,
		At: time.Unix(invoice.StatusTransitions.PaidAt, 0).UTC(), Gross: paydomain.BRL(invoice.AmountPaid), PaidTerm: term,
	}, nil
}

func stripeSubscriptionLine(invoice *stripe.Invoice, contract string) (string, domain.Period, error) {
	if invoice.Lines == nil || invoice.Lines.HasMore || len(invoice.Lines.Data) != 1 {
		return "", domain.Period{}, domain.ErrBillingBinding
	}
	line := invoice.Lines.Data[0]
	if line == nil || line.Amount != invoice.AmountPaid || line.Currency != stripe.CurrencyBRL ||
		line.Quantity != 1 || line.Period == nil || line.Period.Start <= 0 || line.Period.End <= line.Period.Start ||
		line.Parent == nil || line.Parent.Type != stripe.InvoiceLineItemParentTypeSubscriptionItemDetails ||
		line.Pricing == nil || line.Pricing.PriceDetails == nil || line.Pricing.PriceDetails.Price == nil {
		return "", domain.Period{}, domain.ErrBillingBinding
	}
	source := line.Parent.SubscriptionItemDetails
	if source == nil || source.Proration || source.Subscription != contract || line.Pricing.PriceDetails.Price.ID == "" {
		return "", domain.Period{}, domain.ErrBillingBinding
	}
	return line.Pricing.PriceDetails.Price.ID, domain.Period{
		Start: time.Unix(line.Period.Start, 0).UTC(), End: time.Unix(line.Period.End, 0).UTC(),
	}, nil
}

func stripeInvoiceIntent(invoice *stripe.Invoice) (string, error) {
	if invoice.Payments == nil || invoice.Payments.HasMore {
		return "", domain.ErrBillingBinding
	}
	intent := ""
	for _, payment := range invoice.Payments.Data {
		if payment == nil || payment.Status != "paid" {
			continue
		}
		if intent != "" || !payment.IsDefault || payment.Currency != stripe.CurrencyBRL ||
			payment.AmountPaid != invoice.AmountPaid || payment.Livemode != invoice.Livemode ||
			payment.Payment == nil || payment.Payment.Type != stripe.InvoicePaymentPaymentTypePaymentIntent ||
			payment.Payment.PaymentIntent == nil || payment.Payment.PaymentIntent.ID == "" {
			return "", domain.ErrBillingBinding
		}
		intent = payment.Payment.PaymentIntent.ID
	}
	if intent == "" {
		return "", domain.ErrBillingBinding
	}
	return intent, nil
}

func settledStripeCharge(invoice *stripe.Invoice, intentID string, intent *stripe.PaymentIntent) (string, error) {
	if intent == nil || intent.ID != intentID || intent.Status != stripe.PaymentIntentStatusSucceeded ||
		intent.AmountReceived != invoice.AmountPaid || intent.Currency != stripe.CurrencyBRL ||
		intent.Livemode != invoice.Livemode || intent.Customer == nil || intent.Customer.ID != invoice.Customer.ID {
		return "", domain.ErrBillingBinding
	}
	charge := intent.LatestCharge
	if charge == nil || charge.ID == "" || !charge.Paid || !charge.Captured || charge.Refunded ||
		charge.AmountRefunded != 0 || charge.Disputed || charge.AmountCaptured != invoice.AmountPaid ||
		charge.Currency != stripe.CurrencyBRL || charge.Livemode != invoice.Livemode ||
		charge.PaymentMethodDetails == nil || charge.PaymentMethodDetails.Type != stripe.ChargePaymentMethodDetailsTypeCard {
		return "", domain.ErrBillingBinding
	}
	return charge.ID, nil
}
