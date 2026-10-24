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
	Now         func() time.Time
}

func NewStripeProvider(secretKey string, environment domain.Environment, httpClient *http.Client) *StripeProvider {
	backends := stripe.NewBackends(httpClient)
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

func (p *StripeProvider) VerifySettlement(ctx context.Context, event app.InboxRecord) (app.SettlementCandidate, error) {
	if event.EventType != "invoice.paid" || event.ResourceType != "invoice" {
		return app.SettlementCandidate{}, app.ErrIgnored
	}
	invoice, err := p.Client.V1Invoices.Retrieve(ctx, event.ResourceID, nil)
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
	return app.SettlementCandidate{
		OrderID: orderID, Provider: domain.ProviderStripe, Environment: p.Environment,
		Merchant: event.Merchant, Channel: domain.ChannelCard,
		CustomerID: invoice.Customer.ID, RemotePaymentID: invoice.ID,
		ProviderStatus: string(invoice.Status), Amount: domain.BRL(invoice.AmountPaid),
		SettledAt: settledAt,
	}, nil
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
