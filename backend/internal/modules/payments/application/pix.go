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
	pixLeaseTTL     = 30 * time.Second
	pixQRCodeLimit  = 4096
	pixQRImageLimit = 64 * 1024
)

type PixService struct {
	Store    Store
	Provider PixProvider
	Catalog  domain.Catalog
	Config   PixConfig
	Clock    func() time.Time
}

func (s *PixService) Start(ctx context.Context, principal Principal, request PixStartRequest) (PixOrder, error) {
	if s == nil || s.Store == nil || s.Provider == nil {
		return PixOrder{}, domain.ErrUnavailable
	}
	if !domain.ValidID(principal.AccountID) || !domain.ValidID(request.OperationID) ||
		request.OfferID == "" || len(request.OfferID) > 128 {
		return PixOrder{}, domain.ErrInvalid
	}
	if request.AmountCentavos != nil || request.Currency != "" || request.CustomerID != "" ||
		request.OrderID != "" || len(request.Metadata) != 0 {
		return PixOrder{}, domain.ErrInvalid
	}
	if request.PayerName != "" || request.PayerTaxID != "" || request.PayerEmail != "" {
		if _, _, _, err := validatedPayer(request); err != nil {
			return PixOrder{}, err
		}
	}
	if err := s.validateConfig(); err != nil {
		return PixOrder{}, err
	}
	now := s.now()
	quote, err := s.Catalog.Quote(domain.QuoteRequest{
		OfferID: request.OfferID, Channel: domain.ChannelPixManual,
	}, now)
	if err != nil {
		return PixOrder{}, err
	}
	offer, ok := s.Catalog.Offers[quote.OfferID]
	if !ok {
		return PixOrder{}, domain.ErrBinding
	}
	if err := AuthorizeEnterprisePurchase(principal.AccountID, principal.AccountID, quote, offer); err != nil {
		return PixOrder{}, err
	}
	orderID := pixOrderID(principal.AccountID, request.OperationID)
	order, err := s.Store.GetOrder(ctx, orderID)
	if err == nil {
		if order.AccountID != principal.AccountID || order.OfferID != quote.OfferID ||
			order.Channel != domain.ChannelPixManual || !order.Amount.Equal(quote.Amount) ||
			order.CatalogVersion != s.Config.CatalogVersion || !sameStrings(order.Capabilities, quote.Capabilities) {
			return PixOrder{}, domain.ErrConflict
		}
	} else if errors.Is(err, domain.ErrNotFound) {
		order, err = domain.NewOrder(orderID, principal.AccountID, quote, now)
		if err != nil {
			return PixOrder{}, err
		}
		if err := order.MarkPending(now); err != nil {
			return PixOrder{}, err
		}
		if err := s.Store.CreateOrder(ctx, order); err != nil {
			if !errors.Is(err, domain.ErrConflict) {
				return PixOrder{}, err
			}
			order, err = s.Store.GetOrder(ctx, orderID)
			if err != nil {
				return PixOrder{}, err
			}
			if order.AccountID != principal.AccountID || order.OfferID != quote.OfferID ||
				order.Channel != domain.ChannelPixManual || !order.Amount.Equal(quote.Amount) ||
				order.CatalogVersion != s.Config.CatalogVersion || !sameStrings(order.Capabilities, quote.Capabilities) {
				return PixOrder{}, domain.ErrConflict
			}
		}
	} else {
		return PixOrder{}, err
	}
	if order.State == domain.OrderPaid || order.State == domain.OrderCanceled {
		return PixOrder{}, domain.ErrConflict
	}
	customerID, err := s.ensureCustomer(ctx, principal.AccountID, request)
	if err != nil {
		return PixOrder{}, err
	}
	dueDate := order.QuotedAt
	payload := pixPayload(order, s.Config, customerID, dueDate)
	claim, err := s.Store.ClaimAttempt(ctx, ClaimAttemptInput{
		AccountID: principal.AccountID, Provider: domain.ProviderAsaas,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		OperationKey: "asaas-pix:" + request.OperationID, Kind: "create_attempt",
		PayloadHash: payload, OrderID: order.ID, LeaseUntil: now.Add(s.configLease()),
	})
	if err != nil {
		return PixOrder{}, err
	}
	if claim.Replay {
		if claim.State != "SUCCEEDED" || claim.RemoteID == "" {
			return PixOrder{}, domain.ErrUnavailable
		}
		return s.paymentStatus(ctx, order, customerID, claim.RemoteID)
	}
	if err := s.Store.MarkDispatched(ctx, claim); err != nil {
		return PixOrder{}, err
	}
	if claim.Generation > 1 || claim.AttemptNo > 1 {
		payment, found, definitive, err := s.Provider.FindPayment(ctx, order.ID)
		if err != nil {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return PixOrder{}, domain.ErrUnavailable
		}
		if found {
			if err := validatePixPayment(payment, order, customerID); err != nil {
				_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, payment.ID)
				return PixOrder{}, err
			}
			if err := s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, payment.ID); err != nil {
				return PixOrder{}, err
			}
			return s.pixOrder(ctx, order, payment, true)
		}
		if !definitive {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return PixOrder{}, ErrAmbiguous
		}
	}
	payment, err := s.Provider.CreatePayment(ctx, PixPaymentRequest{
		Order: order, CustomerID: customerID, Environment: s.Config.Environment,
		MerchantID: s.Config.MerchantID, DueDate: dueDate,
	})
	if err != nil {
		if errors.Is(err, ErrAmbiguous) || errors.Is(err, ErrProviderUnauthorized) {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return PixOrder{}, ErrAmbiguous
		}
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptFailed, "")
		return PixOrder{}, err
	}
	if err := validatePixPayment(payment, order, customerID); err != nil {
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, payment.ID)
		return PixOrder{}, err
	}
	if err := s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, payment.ID); err != nil {
		return PixOrder{}, err
	}
	return s.pixOrder(ctx, order, payment, false)
}

func (s *PixService) Status(ctx context.Context, principal Principal, orderID string) (PixOrder, error) {
	if s == nil || s.Store == nil || s.Provider == nil || !domain.ValidID(principal.AccountID) || !domain.ValidID(orderID) {
		return PixOrder{}, domain.ErrInvalid
	}
	order, err := s.Store.GetOrder(ctx, orderID)
	if err != nil {
		return PixOrder{}, err
	}
	if order.AccountID != principal.AccountID || order.Channel != domain.ChannelPixManual {
		return PixOrder{}, domain.ErrNotFound
	}
	if order.State == domain.OrderCanceled {
		return PixOrder{OrderID: order.ID, Status: "canceled"}, nil
	}
	payment, found, definitive, err := s.Provider.FindPayment(ctx, order.ID)
	if err != nil {
		return PixOrder{}, domain.ErrUnavailable
	}
	if !found {
		if !definitive {
			return PixOrder{}, domain.ErrUnavailable
		}
		return PixOrder{OrderID: order.ID, Status: PixPending}, nil
	}
	customerID, ok, err := s.Store.Customer(ctx, principal.AccountID, domain.ProviderAsaas, s.Config.Environment, s.Config.MerchantID)
	if err != nil || !ok {
		return PixOrder{}, domain.ErrUnavailable
	}
	if err := validatePixPayment(payment, order, customerID); err != nil {
		return PixOrder{}, err
	}
	return s.pixOrder(ctx, order, payment, true)
}

func (s *PixService) paymentStatus(ctx context.Context, order domain.Order, customerID, paymentID string) (PixOrder, error) {
	payment, err := s.Provider.RetrievePayment(ctx, paymentID)
	if err != nil {
		return PixOrder{}, domain.ErrUnavailable
	}
	if err := validatePixPayment(payment, order, customerID); err != nil {
		return PixOrder{}, err
	}
	return s.pixOrder(ctx, order, payment, true)
}

func (s *PixService) pixOrder(ctx context.Context, order domain.Order, payment PixPayment, replayed bool) (PixOrder, error) {
	result := PixOrder{
		OrderID: order.ID, PaymentID: payment.ID, Status: payment.Class,
		ProviderStatus: payment.ProviderStatus, Replayed: replayed,
	}
	if payment.Class != PixPending {
		return result, nil
	}
	qr, err := s.Provider.GetQR(ctx, payment.ID)
	if err != nil {
		return PixOrder{}, domain.ErrUnavailable
	}
	if !validQR(qr) {
		return PixOrder{}, domain.ErrBinding
	}
	result.QRCode = qr.Payload
	result.QRImage = qr.Image
	if !qr.ExpiresAt.IsZero() {
		expires := qr.ExpiresAt.UTC()
		result.ExpiresAt = &expires
	}
	return result, nil
}

func (s *PixService) ensureCustomer(ctx context.Context, accountID string, request PixStartRequest) (string, error) {
	customerID, found, err := s.Store.Customer(ctx, accountID, domain.ProviderAsaas, s.Config.Environment, s.Config.MerchantID)
	if err != nil {
		return "", err
	}
	if found {
		if !validRemoteID(customerID) {
			return "", domain.ErrBinding
		}
		return customerID, nil
	}
	name, taxID, email, err := validatedPayer(request)
	if err != nil {
		return "", err
	}
	customer, err := s.Provider.EnsureCustomer(ctx, PixCustomerRequest{
		AccountID: accountID, Environment: s.Config.Environment, MerchantID: s.Config.MerchantID,
		Name: name, TaxID: taxID, Email: email,
	})
	if err != nil {
		return "", err
	}
	if !validRemoteID(customer.ID) {
		return "", domain.ErrBinding
	}
	mapping := CustomerMapping{
		AccountID: accountID, Provider: domain.ProviderAsaas,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		RemoteCustomerID: customer.ID,
	}
	if err := s.Store.MapCustomer(ctx, mapping); err != nil {
		existing, ok, readErr := s.Store.Customer(ctx, accountID, domain.ProviderAsaas, s.Config.Environment, s.Config.MerchantID)
		if readErr == nil && ok && existing == customer.ID {
			return existing, nil
		}
		return "", err
	}
	return customer.ID, nil
}

func (s *PixService) validateConfig() error {
	if !s.Config.NewSalesEnabled {
		return domain.ErrSaleDisabled
	}
	if s.Config.Environment != domain.EnvironmentTest && s.Config.Environment != domain.EnvironmentProduction {
		return domain.ErrUnavailable
	}
	if s.Config.MerchantID == "" || s.Config.CatalogVersion == "" || s.Catalog.Version != s.Config.CatalogVersion {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *PixService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *PixService) configLease() time.Duration {
	if s.Config.LeaseTTL > 0 {
		return s.Config.LeaseTTL
	}
	return pixLeaseTTL
}

func validatePixPayment(payment PixPayment, order domain.Order, customerID string) error {
	switch payment.Class {
	case PixPending, PixSettled, PixExpired, PixRefunded, PixDisputed:
	default:
		return domain.ErrUnavailable
	}
	if payment.ID == "" || payment.ExternalReference != order.ID || payment.CustomerID != customerID ||
		!payment.Amount.Equal(order.Amount) || payment.Amount.Currency != "BRL" || payment.Status == "" {
		return domain.ErrBinding
	}
	return nil
}

func validQR(qr PixQR) bool {
	return qr.Payload != "" && len(qr.Payload) <= pixQRCodeLimit &&
		qr.Image != "" && len(qr.Image) <= pixQRImageLimit
}

func validatedPayer(request PixStartRequest) (string, string, string, error) {
	name := strings.TrimSpace(request.PayerName)
	taxID, ok := normalizeTaxID(request.PayerTaxID)
	if !validPayerName(name) || !ok {
		return "", "", "", domain.ErrInvalid
	}
	email := strings.TrimSpace(request.PayerEmail)
	if email != "" && !validEmail(email) {
		return "", "", "", domain.ErrInvalid
	}
	return name, taxID, email, nil
}

func validPayerName(value string) bool {
	if value == "" || len(value) > 150 {
		return false
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

func normalizeTaxID(value string) (string, bool) {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '.' || r == '-' || r == '/' || r == ' ':
		default:
			return "", false
		}
	}
	result := digits.String()
	if len(result) != 11 && len(result) != 14 {
		return "", false
	}
	return result, true
}

func validEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 254 || strings.Count(value, "@") != 1 {
		return false
	}
	parts := strings.SplitN(value, "@", 2)
	return parts[0] != "" && parts[1] != "" && strings.Contains(parts[1], ".")
}

func pixOrderID(accountID, operationID string) string {
	sum := sha256.Sum256([]byte("payment-pix-order\x00" + accountID + "\x00" + operationID))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func pixPayload(order domain.Order, config PixConfig, customerID string, dueDate time.Time) string {
	hash := sha256.New()
	for _, value := range []string{
		order.AccountID, order.ID, order.OfferID, order.CatalogVersion,
		string(order.Channel), order.Amount.Currency, config.MerchantID,
		string(config.Environment), customerID, dueDate.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
