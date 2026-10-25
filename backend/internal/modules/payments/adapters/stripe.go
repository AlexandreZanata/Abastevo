package adapters

import (
	"context"
	"errors"
	"net/http"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/stripe/stripe-go/v87"
)

// StripeProvider is the pinned hosted-checkout adapter. It never accepts raw
// card data and never creates a Price/Product/webhook at startup.
type StripeProvider struct {
	Client      *stripe.Client
	Environment domain.Environment
	Merchant    string
	Now         func() time.Time
}

func NewStripeProvider(secretKey string, environment domain.Environment, httpClient *http.Client) *StripeProvider {
	// The SDK default logger prints provider errors (which carry resource
	// identifiers) to stderr. Payment diagnostics stay structured error
	// codes returned to callers — never raw SDK emission.
	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		HTTPClient:    httpClient,
		LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull},
	})
	return &StripeProvider{
		Client:      stripe.NewClient(secretKey, stripe.WithBackends(backends)),
		Environment: environment,
		Now:         time.Now,
	}
}

func NewStripeProviderWithClient(client *stripe.Client, environment domain.Environment) *StripeProvider {
	return &StripeProvider{Client: client, Environment: environment, Now: time.Now}
}

func (p *StripeProvider) EnsureCustomer(ctx context.Context, request app.CustomerRequest) (app.Customer, error) {
	if p == nil || p.Client == nil {
		return app.Customer{}, domain.ErrUnavailable
	}
	params := &stripe.CustomerCreateParams{
		Metadata: map[string]string{
			"account_id":  request.AccountID,
			"environment": string(request.Environment),
			"merchant_id": request.MerchantID,
		},
	}
	params.SetIdempotencyKey("pay:customer:" + request.AccountID + ":" + string(request.Environment) + ":" + request.MerchantID)
	customer, err := p.Client.V1Customers.Create(ctx, params)
	if err != nil {
		return app.Customer{}, stripeError(err)
	}
	if customer == nil || customer.ID == "" || customer.Livemode != (p.Environment == domain.EnvironmentProduction) ||
		customer.Metadata["account_id"] != request.AccountID {
		return app.Customer{}, domain.ErrBinding
	}
	return app.Customer{ID: customer.ID}, nil
}

func (p *StripeProvider) CreateCheckout(ctx context.Context, request app.CheckoutRequest) (app.CheckoutSession, error) {
	if p == nil || p.Client == nil {
		return app.CheckoutSession{}, domain.ErrUnavailable
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	metadata := map[string]string{
		"order_id":        request.Order.ID,
		"account_id":      request.Order.AccountID,
		"offer_id":        request.Order.OfferID,
		"catalog_version": request.Order.CatalogVersion,
	}
	params := &stripe.CheckoutSessionCreateParams{
		Mode:                      stripe.String("subscription"),
		Customer:                  stripe.String(request.CustomerID),
		ClientReferenceID:         stripe.String(request.Order.ID),
		SuccessURL:                stripe.String(request.SuccessURL),
		CancelURL:                 stripe.String(request.CancelURL),
		AllowPromotionCodes:       stripe.Bool(false),
		Locale:                    stripe.String("pt-BR"),
		ExpiresAt:                 stripe.Int64(now.Add(31 * time.Minute).Unix()),
		AllowedPaymentMethodTypes: []*string{stripe.String("card")},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Price: stripe.String(request.PriceID), Quantity: stripe.Int64(1),
		}},
		Metadata: metadata,
		SubscriptionData: &stripe.CheckoutSessionCreateSubscriptionDataParams{
			Metadata: metadata,
		},
	}
	params.SetIdempotencyKey(request.IdempotencyKey)
	session, err := p.Client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return app.CheckoutSession{}, stripeError(err)
	}
	result, err := mapCheckoutSession(session, p.Environment)
	if err != nil {
		return app.CheckoutSession{}, err
	}
	if result.CustomerID != request.CustomerID || result.AmountTotal != request.Order.Amount.Centavos ||
		result.Currency != "brl" {
		return app.CheckoutSession{}, domain.ErrBinding
	}
	return result, nil
}

// CancelRenewal stops future invoices while preserving the current paid-through
// period. It is idempotent under the caller's stable switch key; the caller must
// still retrieve canonical state before activating another source.
func (p *StripeProvider) CancelRenewal(ctx context.Context, subscriptionID, idempotencyKey string) error {
	if p == nil || p.Client == nil || subscriptionID == "" || idempotencyKey == "" {
		return domain.ErrUnavailable
	}
	params := &stripe.SubscriptionUpdateParams{CancelAtPeriodEnd: stripe.Bool(true)}
	params.SetIdempotencyKey(idempotencyKey)
	subscription, err := p.Client.V1Subscriptions.Update(ctx, subscriptionID, params)
	if err != nil {
		return stripeError(err)
	}
	if subscription == nil || subscription.ID != subscriptionID || subscription.Customer == nil ||
		subscription.Livemode != (p.Environment == domain.EnvironmentProduction) {
		return domain.ErrBinding
	}
	return nil
}

func (p *StripeProvider) VerifySettlement(ctx context.Context, event app.InboxRecord) (app.SettlementCandidate, error) {
	if event.EventType != "invoice.paid" || event.ResourceType != "invoice" {
		return app.SettlementCandidate{}, app.ErrIgnored
	}
	invoice, err := p.Client.V1Invoices.Retrieve(ctx, event.ResourceID, &stripe.InvoiceRetrieveParams{
		Expand: []*string{stripe.String("payments")},
	})
	if err != nil {
		return app.SettlementCandidate{}, stripeError(err)
	}
	if invoice.Status != stripe.InvoiceStatusPaid {
		return app.SettlementCandidate{}, app.ErrNotSettled
	}
	if string(invoice.Currency) != "brl" || invoice.AmountPaid <= 0 || invoice.AmountPaid != invoice.AmountDue ||
		invoice.AmountPaidOffStripe != 0 || invoice.CollectionMethod != stripe.InvoiceCollectionMethodChargeAutomatically ||
		invoice.Livemode != (p.Environment == domain.EnvironmentProduction) || invoice.Customer == nil {
		return app.SettlementCandidate{}, domain.ErrBinding
	}
	switch invoice.BillingReason {
	case stripe.InvoiceBillingReasonSubscriptionCreate, stripe.InvoiceBillingReasonSubscriptionCycle, stripe.InvoiceBillingReasonSubscriptionUpdate:
	default:
		return app.SettlementCandidate{}, domain.ErrBinding
	}
	metadata := map[string]string{}
	if invoice.Parent != nil && invoice.Parent.SubscriptionDetails != nil {
		if invoice.Parent.SubscriptionDetails.Metadata != nil {
			for key, value := range invoice.Parent.SubscriptionDetails.Metadata {
				metadata[key] = value
			}
		}
		if invoice.Parent.SubscriptionDetails.Subscription != nil {
			for key, value := range invoice.Parent.SubscriptionDetails.Subscription.Metadata {
				metadata[key] = value
			}
		}
	}
	orderID, accountID := metadata["order_id"], metadata["account_id"]
	if orderID == "" || accountID == "" || invoice.ID == "" {
		return app.SettlementCandidate{}, domain.ErrBinding
	}
	settledAt := event.ReceivedAt.UTC()
	if invoice.StatusTransitions != nil && invoice.StatusTransitions.PaidAt > 0 {
		settledAt = time.Unix(invoice.StatusTransitions.PaidAt, 0).UTC()
	}
	if settledAt.IsZero() {
		settledAt = time.Now().UTC()
	}
	chargeID := ""
	if invoice.Payments != nil {
		for _, payment := range invoice.Payments.Data {
			if payment == nil || payment.Payment == nil {
				continue
			}
			if payment.Payment.PaymentIntent != nil && payment.Payment.PaymentIntent.ID != "" {
				chargeID = payment.Payment.PaymentIntent.ID
				break
			}
			if payment.Payment.Charge != nil && payment.Payment.Charge.ID != "" {
				chargeID = payment.Payment.Charge.ID
				break
			}
		}
	}
	return app.SettlementCandidate{
		OrderID: orderID, Provider: domain.ProviderStripe, Environment: p.Environment,
		Merchant: event.Merchant, Channel: domain.ChannelCard,
		CustomerID: invoice.Customer.ID, RemotePaymentID: invoice.ID,
		RemoteChargeID: chargeID,
		ProviderStatus: string(invoice.Status), Amount: domain.BRL(invoice.AmountPaid),
		SettledAt: settledAt,
	}, nil
}

func (p *StripeProvider) CreateRefund(ctx context.Context, request app.RefundProviderRequest) (app.RefundProviderResult, error) {
	if p == nil || p.Client == nil || request.RefundID == "" || request.Amount.Centavos <= 0 {
		return app.RefundProviderResult{}, domain.ErrInvalid
	}
	target, err := p.stripeRefundTarget(ctx, request.RemotePaymentID, request.RemoteChargeID)
	if err != nil {
		return app.RefundProviderResult{}, err
	}
	params := &stripe.RefundCreateParams{
		Amount:   stripe.Int64(request.Amount.Centavos),
		Currency: stripe.String("brl"),
		Metadata: map[string]string{"refund_id": request.RefundID},
	}
	if target.paymentIntent != "" {
		params.PaymentIntent = stripe.String(target.paymentIntent)
	} else {
		params.Charge = stripe.String(target.charge)
	}
	params.SetIdempotencyKey(request.IdempotencyKey)
	refund, err := p.Client.V1Refunds.Create(ctx, params)
	if err != nil {
		return app.RefundProviderResult{}, stripeError(err)
	}
	return mapStripeRefund(refund)
}

func (p *StripeProvider) FindRefund(ctx context.Context, request app.RefundProviderRequest) (app.RefundProviderResult, bool, bool, error) {
	if p == nil || p.Client == nil || request.RefundID == "" {
		return app.RefundProviderResult{}, false, false, domain.ErrInvalid
	}
	target, err := p.stripeRefundTarget(ctx, request.RemotePaymentID, request.RemoteChargeID)
	if err != nil {
		return app.RefundProviderResult{}, false, false, err
	}
	params := &stripe.RefundListParams{Limit: stripe.Int64(100)}
	if target.paymentIntent != "" {
		params.PaymentIntent = stripe.String(target.paymentIntent)
	} else {
		params.Charge = stripe.String(target.charge)
	}
	list := p.Client.V1Refunds.List(ctx, params)
	var matches []*stripe.Refund
	for _, refund := range list.Data() {
		if refund == nil || refund.Metadata["refund_id"] != request.RefundID {
			continue
		}
		matches = append(matches, refund)
	}
	if err := list.Err(); err != nil {
		return app.RefundProviderResult{}, false, false, stripeError(err)
	}
	if len(matches) > 1 {
		return app.RefundProviderResult{}, false, false, domain.ErrConflict
	}
	if len(matches) == 1 {
		result, err := mapStripeRefund(matches[0])
		if err != nil {
			return app.RefundProviderResult{}, false, false, err
		}
		if err := validRefundAmount(result.Amount, request.Amount); err != nil {
			return app.RefundProviderResult{}, false, false, err
		}
		return result, true, true, nil
	}
	return app.RefundProviderResult{}, false, !list.Meta().HasMore, nil
}

func (p *StripeProvider) VerifyDispute(ctx context.Context, event app.InboxRecord) (app.DisputeFact, error) {
	if p == nil || p.Client == nil {
		return app.DisputeFact{}, domain.ErrUnavailable
	}
	switch event.EventType {
	case "charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed":
	default:
		return app.DisputeFact{}, app.ErrIgnored
	}
	if event.ResourceType != "dispute" || event.ResourceID == "" {
		return app.DisputeFact{}, domain.ErrBinding
	}
	dispute, err := p.Client.V1Disputes.Retrieve(ctx, event.ResourceID, nil)
	if err != nil {
		return app.DisputeFact{}, stripeError(err)
	}
	if dispute == nil || dispute.ID == "" || dispute.Amount <= 0 ||
		string(dispute.Currency) != "brl" ||
		dispute.Livemode != (p.Environment == domain.EnvironmentProduction) {
		return app.DisputeFact{}, domain.ErrBinding
	}
	chargeID := ""
	if dispute.PaymentIntent != nil {
		chargeID = dispute.PaymentIntent.ID
	} else if dispute.Charge != nil {
		chargeID = dispute.Charge.ID
	}
	if chargeID == "" {
		return app.DisputeFact{}, domain.ErrBinding
	}
	state := domain.DisputePending
	switch dispute.Status {
	case stripe.DisputeStatusNeedsResponse, stripe.DisputeStatusWarningNeedsResponse,
		stripe.DisputeStatusUnderReview, stripe.DisputeStatusWarningUnderReview:
		state = domain.DisputePending
	case stripe.DisputeStatusWon:
		state = domain.DisputeWon
	case stripe.DisputeStatusLost:
		state = domain.DisputeLost
	case stripe.DisputeStatusPrevented, stripe.DisputeStatusWarningClosed:
		state = domain.DisputeClosed
	default:
		return app.DisputeFact{}, app.ErrIgnored
	}
	opened := event.ReceivedAt.UTC()
	if dispute.Created > 0 {
		opened = time.Unix(dispute.Created, 0).UTC()
	}
	return app.DisputeFact{
		Provider: domain.ProviderStripe, Environment: p.Environment, Merchant: event.Merchant,
		RemoteDisputeID: dispute.ID, RemoteChargeID: chargeID,
		Amount: domain.BRL(dispute.Amount), State: state,
		OpenedAt: opened, LastEventAt: opened,
	}, nil
}

type stripeRefundTarget struct {
	paymentIntent string
	charge        string
}

// stripeRefundTarget resolves the canonical original-route reference for a
// stored invoice id: the invoice's payment intent, else its charge.
func (p *StripeProvider) stripeRefundTarget(ctx context.Context, invoiceID, chargeID string) (stripeRefundTarget, error) {
	if chargeID != "" {
		return stripeRefundTarget{charge: chargeID}, nil
	}
	if invoiceID == "" {
		return stripeRefundTarget{}, domain.ErrInvalid
	}
	invoice, err := p.Client.V1Invoices.Retrieve(ctx, invoiceID, &stripe.InvoiceRetrieveParams{
		Expand: []*string{stripe.String("payments")},
	})
	if err != nil {
		return stripeRefundTarget{}, stripeError(err)
	}
	if invoice == nil || invoice.ID != invoiceID || string(invoice.Currency) != "brl" ||
		invoice.Livemode != (p.Environment == domain.EnvironmentProduction) {
		return stripeRefundTarget{}, domain.ErrBinding
	}
	if invoice.Payments != nil {
		for _, payment := range invoice.Payments.Data {
			if payment == nil || payment.Payment == nil {
				continue
			}
			if payment.Payment.PaymentIntent != nil && payment.Payment.PaymentIntent.ID != "" {
				return stripeRefundTarget{paymentIntent: payment.Payment.PaymentIntent.ID}, nil
			}
			if payment.Payment.Charge != nil && payment.Payment.Charge.ID != "" {
				return stripeRefundTarget{charge: payment.Payment.Charge.ID}, nil
			}
		}
	}
	return stripeRefundTarget{}, domain.ErrUnavailable
}

func mapStripeRefund(refund *stripe.Refund) (app.RefundProviderResult, error) {
	if refund == nil || refund.ID == "" || refund.Amount <= 0 || string(refund.Currency) != "brl" {
		return app.RefundProviderResult{}, domain.ErrBinding
	}
	state := domain.RefundPending
	switch refund.Status {
	case stripe.RefundStatusSucceeded:
		state = domain.RefundSucceeded
	case stripe.RefundStatusPending, stripe.RefundStatusRequiresAction:
		state = domain.RefundPending
	case stripe.RefundStatusFailed, stripe.RefundStatusCanceled:
		state = domain.RefundFailed
	default:
		return app.RefundProviderResult{}, app.ErrIgnored
	}
	at := time.Now().UTC()
	if refund.Created > 0 {
		at = time.Unix(refund.Created, 0).UTC()
	}
	return app.RefundProviderResult{
		ID: refund.ID, State: state, RawStatus: string(refund.Status),
		Amount: domain.BRL(refund.Amount), At: at,
	}, nil
}

func validRefundAmount(actual, expected domain.Money) error {
	if expected.Centavos <= 0 {
		return nil
	}
	if !actual.Equal(expected) {
		return domain.ErrBinding
	}
	return nil
}

func (p *StripeProvider) VerifyLifecycle(ctx context.Context, event app.InboxRecord) (app.LifecycleFact, error) {
	if p == nil || p.Client == nil || !validLifecycleEvent(event) {
		return app.LifecycleFact{}, app.ErrIgnored
	}
	var subscriptionID string
	switch event.EventType {
	case "customer.subscription.updated", "customer.subscription.deleted":
		if event.ResourceType != "subscription" {
			return app.LifecycleFact{}, domain.ErrBinding
		}
		subscriptionID = event.ResourceID
	case "invoice.payment_failed", "invoice.payment_action_required", "invoice.paid":
		if event.ResourceType != "invoice" {
			return app.LifecycleFact{}, domain.ErrBinding
		}
		invoice, err := p.Client.V1Invoices.Retrieve(ctx, event.ResourceID, nil)
		if err != nil {
			return app.LifecycleFact{}, stripeError(err)
		}
		if invoice.Parent == nil || invoice.Parent.SubscriptionDetails == nil ||
			invoice.Parent.SubscriptionDetails.Subscription == nil {
			return app.LifecycleFact{}, domain.ErrBinding
		}
		subscriptionID = invoice.Parent.SubscriptionDetails.Subscription.ID
	case "checkout.session.completed":
		if event.ResourceType != "checkout.session" {
			return app.LifecycleFact{}, domain.ErrBinding
		}
		session, err := p.Client.V1CheckoutSessions.Retrieve(ctx, event.ResourceID, nil)
		if err != nil {
			return app.LifecycleFact{}, stripeError(err)
		}
		if session.Subscription == nil || session.Subscription.ID == "" {
			return app.LifecycleFact{}, domain.ErrBinding
		}
		subscriptionID = session.Subscription.ID
	default:
		return app.LifecycleFact{}, app.ErrIgnored
	}
	return p.lifecycleFor(ctx, subscriptionID, event.Merchant, event.ReceivedAt)
}

func (p *StripeProvider) RetrieveLifecycle(ctx context.Context, remoteSubscriptionID string) (app.LifecycleFact, error) {
	if p == nil || p.Client == nil || p.Merchant == "" {
		return app.LifecycleFact{}, domain.ErrUnavailable
	}
	return p.lifecycleFor(ctx, remoteSubscriptionID, p.Merchant, time.Now().UTC())
}

func (p *StripeProvider) lifecycleFor(ctx context.Context, id, merchant string, at time.Time) (app.LifecycleFact, error) {
	subscription, err := p.Client.V1Subscriptions.Retrieve(ctx, id, nil)
	if err != nil {
		return app.LifecycleFact{}, stripeError(err)
	}
	if subscription == nil || subscription.ID == "" || subscription.Customer == nil ||
		subscription.Status == "" || subscription.Livemode != (p.Environment == domain.EnvironmentProduction) {
		return app.LifecycleFact{}, domain.ErrBinding
	}
	accountID, offerID := subscription.Metadata["account_id"], subscription.Metadata["offer_id"]
	if accountID == "" || offerID == "" {
		return app.LifecycleFact{}, domain.ErrBinding
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return app.LifecycleFact{
		AccountID: accountID, OfferID: offerID, Environment: p.Environment, Merchant: merchant,
		Provider: domain.ProviderStripe, RemoteSubscriptionID: subscription.ID,
		RemoteCustomerID: subscription.Customer.ID, ProviderStatus: string(subscription.Status),
		CancelAtPeriodEnd: subscription.CancelAtPeriodEnd, LastEventAt: at.UTC(),
	}, nil
}

func (p *StripeProvider) CreatePortalSession(ctx context.Context, request app.PortalRequest) (string, error) {
	if p == nil || p.Client == nil || request.CustomerID == "" || request.ReturnURL == "" || request.ConfigurationID == "" {
		return "", domain.ErrUnavailable
	}
	session, err := p.Client.V1BillingPortalSessions.Create(ctx, &stripe.BillingPortalSessionCreateParams{
		Customer: stripe.String(request.CustomerID), ReturnURL: stripe.String(request.ReturnURL),
		Configuration: stripe.String(request.ConfigurationID),
	})
	if err != nil {
		return "", stripeError(err)
	}
	if session == nil || session.URL == "" {
		return "", domain.ErrBinding
	}
	return session.URL, nil
}

func validLifecycleEvent(event app.InboxRecord) bool {
	switch event.EventType {
	case "customer.subscription.updated", "customer.subscription.deleted",
		"invoice.payment_failed", "invoice.payment_action_required", "invoice.paid",
		"checkout.session.completed":
		return true
	default:
		return false
	}
}

func (p *StripeProvider) RetrieveCheckout(ctx context.Context, sessionID string) (app.CheckoutSession, error) {
	if p == nil || p.Client == nil || sessionID == "" {
		return app.CheckoutSession{}, domain.ErrUnavailable
	}
	session, err := p.Client.V1CheckoutSessions.Retrieve(ctx, sessionID, nil)
	if err != nil {
		return app.CheckoutSession{}, stripeError(err)
	}
	return mapCheckoutSession(session, p.Environment)
}

// FindCheckout scans all sessions for the bound customer and matches the
// durable client_reference_id. It is used only after an ambiguous/expired lease;
// a list error is not treated as proven absence.
func (p *StripeProvider) FindCheckout(ctx context.Context, customerID, orderID string) (app.CheckoutSession, bool, bool, error) {
	if p == nil || p.Client == nil || customerID == "" || orderID == "" {
		return app.CheckoutSession{}, false, false, domain.ErrUnavailable
	}
	list := p.Client.V1CheckoutSessions.List(ctx, &stripe.CheckoutSessionListParams{
		Customer:   stripe.String(customerID),
		ListParams: stripe.ListParams{Limit: stripe.Int64(100)},
	})
	for session, err := range list.All(ctx) {
		if err != nil {
			return app.CheckoutSession{}, false, false, stripeError(err)
		}
		if session != nil && session.ClientReferenceID == orderID {
			result, mapErr := mapCheckoutSession(session, p.Environment)
			if mapErr != nil {
				return app.CheckoutSession{}, false, false, mapErr
			}
			return result, true, true, nil
		}
	}
	return app.CheckoutSession{}, false, true, nil
}

func mapCheckoutSession(session *stripe.CheckoutSession, environment domain.Environment) (app.CheckoutSession, error) {
	if session == nil || session.ID == "" || session.URL == "" || session.Customer == nil ||
		session.Customer.ID == "" || string(session.Mode) != "subscription" ||
		session.Livemode != (environment == domain.EnvironmentProduction) {
		return app.CheckoutSession{}, domain.ErrBinding
	}
	result := app.CheckoutSession{
		ID: session.ID, URL: session.URL, Status: string(session.Status), Mode: string(session.Mode),
		CustomerID: session.Customer.ID, Livemode: session.Livemode,
		AmountTotal: session.AmountTotal, Currency: string(session.Currency),
	}
	if session.Subscription != nil {
		result.SubscriptionID = session.Subscription.ID
	}
	return result, nil
}

func stripeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return app.ErrAmbiguous
	}
	var stripeErr *stripe.Error
	if errors.As(err, &stripeErr) {
		switch {
		case stripeErr.HTTPStatusCode == http.StatusUnauthorized || stripeErr.HTTPStatusCode == http.StatusForbidden:
			return app.ErrProviderUnauthorized
		case stripeErr.HTTPStatusCode == 429, stripeErr.HTTPStatusCode >= 500, stripeErr.Type == stripe.ErrorTypeAPI:
			return app.ErrAmbiguous
		case stripeErr.Type == stripe.ErrorTypeIdempotency:
			return domain.ErrConflict
		case stripeErr.Type == stripe.ErrorTypeInvalidRequest:
			return domain.ErrInvalid
		case stripeErr.Type == stripe.ErrorTypeCard:
			return domain.ErrInvalid
		}
	}
	return app.ErrAmbiguous
}
