package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	checkoutReturnPath = "/billing/checkout/return"
	checkoutCancelPath = "/billing/checkout/cancel"
	checkoutLeaseTTL   = 30 * time.Second
)

type Service struct {
	Store    Store
	Provider Provider
	Catalog  domain.Catalog
	Config   Config
	Clock    func() time.Time
}

func (s *Service) Start(ctx context.Context, principal Principal, request StartRequest) (Checkout, error) {
	if s == nil || s.Store == nil || s.Provider == nil {
		return Checkout{}, domain.ErrUnavailable
	}
	if !domain.ValidID(principal.AccountID) || !domain.ValidID(request.OperationID) ||
		request.OfferID == "" || len(request.OfferID) > 128 {
		return Checkout{}, domain.ErrInvalid
	}
	if request.AmountCentavos != nil || request.Currency != "" || request.CustomerID != "" ||
		request.OrderID != "" || request.SuccessURL != "" || request.CancelURL != "" ||
		len(request.Metadata) != 0 {
		return Checkout{}, domain.ErrInvalid
	}
	if err := s.validateConfig(); err != nil {
		return Checkout{}, err
	}
	now := s.now()
	quote, err := s.Catalog.Quote(domain.QuoteRequest{
		OfferID: request.OfferID, Channel: domain.ChannelCard,
	}, now)
	if err != nil {
		return Checkout{}, err
	}
	orderID := checkoutOrderID(principal.AccountID, request.OperationID)
	order, err := s.Store.GetOrder(ctx, orderID)
	if err == nil {
		if order.AccountID != principal.AccountID || order.OfferID != quote.OfferID ||
			order.Channel != domain.ChannelCard || !order.Amount.Equal(quote.Amount) ||
			order.CatalogVersion != s.Config.CatalogVersion || !sameStrings(order.Capabilities, quote.Capabilities) {
			return Checkout{}, domain.ErrConflict
		}
	} else if errors.Is(err, domain.ErrNotFound) {
		if err := allowNewRecurringSource(ctx, s.Store, s.Catalog, principal.AccountID, quote.OfferID,
			domain.ChannelCard, quote, now); err != nil {
			return Checkout{}, err
		}
		order, err = domain.NewOrder(orderID, principal.AccountID, quote, now)
		if err != nil {
			return Checkout{}, err
		}
		if err := order.MarkPending(now); err != nil {
			return Checkout{}, err
		}
		if err := s.Store.CreateOrder(ctx, order); err != nil {
			if !errors.Is(err, domain.ErrConflict) {
				return Checkout{}, err
			}
			order, err = s.Store.GetOrder(ctx, orderID)
			if err != nil {
				return Checkout{}, err
			}
			if order.AccountID != principal.AccountID || order.OfferID != quote.OfferID ||
				order.Channel != domain.ChannelCard || !order.Amount.Equal(quote.Amount) ||
				order.CatalogVersion != s.Config.CatalogVersion || !sameStrings(order.Capabilities, quote.Capabilities) {
				return Checkout{}, domain.ErrConflict
			}
		}
	} else {
		return Checkout{}, err
	}
	if order.State == domain.OrderPaid || order.State == domain.OrderCanceled {
		return Checkout{}, domain.ErrConflict
	}
	customerID, err := s.ensureCustomer(ctx, principal.AccountID)
	if err != nil {
		return Checkout{}, err
	}
	payload := checkoutPayload(order, s.Config, customerID)
	lease := now.Add(s.configLease())
	claim, err := s.Store.ClaimAttempt(ctx, ClaimAttemptInput{
		AccountID: principal.AccountID, Provider: domain.ProviderStripe,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		OperationKey: "stripe-checkout:" + request.OperationID, Kind: "create_attempt",
		PayloadHash: payload, OrderID: order.ID, LeaseUntil: lease,
	})
	if err != nil {
		return Checkout{}, err
	}
	if claim.Replay {
		if claim.State != "SUCCEEDED" || claim.RemoteID == "" {
			return Checkout{}, domain.ErrUnavailable
		}
		return s.retrieve(ctx, order, customerID, claim.RemoteID)
	}
	if err := s.Store.MarkDispatched(ctx, claim); err != nil {
		return Checkout{}, err
	}
	if claim.Generation > 1 || claim.AttemptNo > 1 {
		session, found, definitive, err := s.Provider.FindCheckout(ctx, customerID, order.ID)
		if err != nil {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return Checkout{}, domain.ErrUnavailable
		}
		if found {
			if session.Status != "open" {
				_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, session.ID)
				return Checkout{}, domain.ErrStale
			}
			if err := validateSession(session, order, customerID, s.Config.Environment); err != nil {
				_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, session.ID)
				return Checkout{}, err
			}
			if err := s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, session.ID); err != nil {
				return Checkout{}, err
			}
			return Checkout{OrderID: order.ID, SessionID: session.ID, URL: session.URL, Replayed: true}, nil
		}
		if !definitive {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return Checkout{}, ErrAmbiguous
		}
	}
	session, err := s.Provider.CreateCheckout(ctx, CheckoutRequest{
		Order: order, CustomerID: customerID, PriceID: s.Config.PriceID,
		MerchantID: s.Config.MerchantID, Environment: s.Config.Environment,
		SuccessURL: s.successURL(), CancelURL: s.cancelURL(),
		IdempotencyKey: "pay:checkout:" + request.OperationID,
	})
	if err != nil {
		if errors.Is(err, ErrAmbiguous) {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return Checkout{}, ErrAmbiguous
		}
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptFailed, "")
		return Checkout{}, err
	}
	if err := validateSession(session, order, customerID, s.Config.Environment); err != nil {
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, session.ID)
		return Checkout{}, err
	}
	if err := s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, session.ID); err != nil {
		return Checkout{}, err
	}
	return Checkout{OrderID: order.ID, SessionID: session.ID, URL: session.URL}, nil
}

func (s *Service) retrieve(ctx context.Context, order domain.Order, customerID, sessionID string) (Checkout, error) {
	session, err := s.Provider.RetrieveCheckout(ctx, sessionID)
	if err != nil {
		return Checkout{}, domain.ErrUnavailable
	}
	if session.Status != "open" {
		return Checkout{}, domain.ErrStale
	}
	if err := validateSession(session, order, customerID, s.Config.Environment); err != nil {
		return Checkout{}, err
	}
	return Checkout{OrderID: order.ID, SessionID: session.ID, URL: session.URL, Replayed: true}, nil
}

func (s *Service) ensureCustomer(ctx context.Context, accountID string) (string, error) {
	customerID, found, err := s.Store.Customer(ctx, accountID, domain.ProviderStripe, s.Config.Environment, s.Config.MerchantID)
	if err != nil {
		return "", err
	}
	if found {
		if !validRemoteID(customerID) {
			return "", domain.ErrBinding
		}
		return customerID, nil
	}
	customer, err := s.Provider.EnsureCustomer(ctx, CustomerRequest{
		AccountID: accountID, Environment: s.Config.Environment, MerchantID: s.Config.MerchantID,
	})
	if err != nil {
		return "", err
	}
	if !validRemoteID(customer.ID) {
		return "", domain.ErrBinding
	}
	mapping := CustomerMapping{
		AccountID: accountID, Provider: domain.ProviderStripe,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		RemoteCustomerID: customer.ID,
	}
	if err := s.Store.MapCustomer(ctx, mapping); err != nil {
		existing, ok, readErr := s.Store.Customer(ctx, accountID, domain.ProviderStripe, s.Config.Environment, s.Config.MerchantID)
		if readErr == nil && ok && existing == customer.ID {
			return existing, nil
		}
		return "", err
	}
	return customer.ID, nil
}

func (s *Service) validateConfig() error {
	if !s.Config.NewSalesEnabled {
		return domain.ErrSaleDisabled
	}
	if s.Config.Environment != domain.EnvironmentTest && s.Config.Environment != domain.EnvironmentProduction {
		return domain.ErrUnavailable
	}
	if s.Config.MerchantID == "" || s.Config.PriceID == "" || s.Config.ReturnOrigin == "" ||
		s.Config.CatalogVersion == "" || s.Catalog.Version != s.Config.CatalogVersion {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) configLease() time.Duration {
	if s.Config.LeaseTTL > 0 {
		return s.Config.LeaseTTL
	}
	return checkoutLeaseTTL
}

func (s *Service) successURL() string {
	return strings.TrimRight(s.Config.ReturnOrigin, "/") + checkoutReturnPath + "?session_id={CHECKOUT_SESSION_ID}"
}

func (s *Service) cancelURL() string {
	return strings.TrimRight(s.Config.ReturnOrigin, "/") + checkoutCancelPath
}

func validateSession(session CheckoutSession, order domain.Order, customerID string, environment domain.Environment) error {
	if !validRemoteID(session.ID) || !validRemoteID(session.URL) ||
		session.Status != "open" || session.Mode != "subscription" ||
		session.CustomerID != customerID || session.AmountTotal != order.Amount.Centavos ||
		session.Currency != "brl" || session.Livemode != (environment == domain.EnvironmentProduction) {
		return domain.ErrBinding
	}
	return nil
}

func checkoutOrderID(accountID, operationID string) string {
	sum := sha256.Sum256([]byte("payment-order\x00" + accountID + "\x00" + operationID))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func checkoutPayload(order domain.Order, config Config, customerID string) string {
	hash := sha256.New()
	for _, value := range []string{
		order.AccountID, order.ID, order.OfferID, order.CatalogVersion,
		string(order.Channel), order.Amount.Currency, config.MerchantID,
		string(config.Environment), config.PriceID, config.ReturnOrigin, customerID,
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

func validRemoteID(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}
