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

const pixAutoLeaseTTL = 30 * time.Second

type PixAutoService struct {
	Store    PixAutoStore
	Provider PixAutoProvider
	Catalog  domain.Catalog
	Config   PixAutoConfig
	Clock    func() time.Time
}

func (s *PixAutoService) Start(ctx context.Context, principal Principal, request PixAutoStartRequest) (PixAutoAuthorization, error) {
	if s == nil || s.Store == nil || s.Provider == nil {
		return PixAutoAuthorization{}, domain.ErrUnavailable
	}
	if !domain.ValidID(principal.AccountID) || !domain.ValidID(request.OperationID) ||
		request.OfferID == "" || len(request.OfferID) > 128 || !request.Consent {
		return PixAutoAuthorization{}, domain.ErrInvalid
	}
	if request.AmountCentavos != nil || request.Currency != "" || request.CustomerID != "" ||
		request.OrderID != "" || len(request.Metadata) != 0 {
		return PixAutoAuthorization{}, domain.ErrInvalid
	}
	if err := s.validateConfig(); err != nil {
		return PixAutoAuthorization{}, err
	}
	eligibility, found, err := s.Store.Eligibility(ctx, s.Config.Environment, s.Config.MerchantID)
	if err != nil {
		return PixAutoAuthorization{}, err
	}
	if found && eligibility.Status == "INELIGIBLE" {
		return PixAutoAuthorization{}, domain.ErrUnavailable
	}
	if request.PayerName != "" || request.PayerTaxID != "" || request.PayerEmail != "" {
		if _, _, _, err := validatedPayer(PixStartRequest{
			PayerName: request.PayerName, PayerTaxID: request.PayerTaxID, PayerEmail: request.PayerEmail,
		}); err != nil {
			return PixAutoAuthorization{}, err
		}
	}
	now := s.now()
	quote, err := s.Catalog.Quote(domain.QuoteRequest{OfferID: request.OfferID, Channel: domain.ChannelPixAutomatic}, now)
	if err != nil {
		return PixAutoAuthorization{}, err
	}
	orderID := pixAutoOrderID(principal.AccountID, request.OperationID)
	existing, err := s.Store.MandateForAccount(ctx, principal.AccountID, request.OfferID)
	if err == nil && existing.OrderID != orderID && !pixAutoTerminal(existing.Status) &&
		(existing.Status == "CREATED" || existing.Status == "ACTIVE" || existing.CancelRequestedAt != nil) {
		return PixAutoAuthorization{}, domain.ErrConflict
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return PixAutoAuthorization{}, err
	}
	order, err := s.Store.GetOrder(ctx, orderID)
	if err == nil {
		if order.AccountID != principal.AccountID || order.OfferID != quote.OfferID ||
			order.Channel != domain.ChannelPixAutomatic || !order.Amount.Equal(quote.Amount) ||
			order.CatalogVersion != s.Config.CatalogVersion || !sameStrings(order.Capabilities, quote.Capabilities) {
			return PixAutoAuthorization{}, domain.ErrConflict
		}
	} else if errors.Is(err, domain.ErrNotFound) {
		order, err = domain.NewOrder(orderID, principal.AccountID, quote, now)
		if err != nil {
			return PixAutoAuthorization{}, err
		}
		if err := order.MarkPending(now); err != nil {
			return PixAutoAuthorization{}, err
		}
		if err := s.Store.CreateOrder(ctx, order); err != nil {
			if !errors.Is(err, domain.ErrConflict) {
				return PixAutoAuthorization{}, err
			}
			order, err = s.Store.GetOrder(ctx, orderID)
			if err != nil {
				return PixAutoAuthorization{}, err
			}
			if order.AccountID != principal.AccountID || order.OfferID != quote.OfferID ||
				order.Channel != domain.ChannelPixAutomatic || !order.Amount.Equal(quote.Amount) ||
				order.CatalogVersion != s.Config.CatalogVersion || !sameStrings(order.Capabilities, quote.Capabilities) {
				return PixAutoAuthorization{}, domain.ErrConflict
			}
		}
	} else {
		return PixAutoAuthorization{}, err
	}
	if order.State == domain.OrderPaid || order.State == domain.OrderCanceled {
		return PixAutoAuthorization{}, domain.ErrConflict
	}
	customerID, err := s.ensureCustomer(ctx, principal.AccountID, request)
	if err != nil {
		return PixAutoAuthorization{}, err
	}
	contractID := strings.ReplaceAll(order.ID, "-", "")
	startDate := order.QuotedAt
	finishDate := startDate.AddDate(0, s.months(), 0)
	payload := pixAutoPayload(order, s.Config, customerID, contractID, startDate, finishDate)
	claim, err := s.Store.ClaimAttempt(ctx, ClaimAttemptInput{
		AccountID: principal.AccountID, Provider: domain.ProviderAsaas,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		OperationKey: "asaas-pix-auto:" + request.OperationID, Kind: "create_attempt",
		PayloadHash: payload, OrderID: order.ID, LeaseUntil: now.Add(s.lease()),
	})
	if err != nil {
		return PixAutoAuthorization{}, err
	}
	if claim.Replay {
		if claim.State != "SUCCEEDED" || claim.RemoteID == "" {
			return PixAutoAuthorization{}, domain.ErrUnavailable
		}
		authorization, err := s.Provider.RetrievePixAutoAuthorization(ctx, claim.RemoteID)
		if err != nil {
			return PixAutoAuthorization{}, domain.ErrUnavailable
		}
		if err := s.persist(ctx, order, principal.AccountID, customerID, contractID, authorization, now); err != nil {
			return PixAutoAuthorization{}, err
		}
		return s.result(authorization, true), nil
	}
	if err := s.Store.MarkDispatched(ctx, claim); err != nil {
		return PixAutoAuthorization{}, err
	}
	if claim.Generation > 1 || claim.AttemptNo > 1 {
		authorization, found, definitive, err := s.Provider.FindPixAutoAuthorization(ctx, customerID, contractID)
		if err != nil {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return PixAutoAuthorization{}, domain.ErrUnavailable
		}
		if found {
			if err := s.persist(ctx, order, principal.AccountID, customerID, contractID, authorization, now); err != nil {
				_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, authorization.ID)
				return PixAutoAuthorization{}, err
			}
			if err := s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, authorization.ID); err != nil {
				return PixAutoAuthorization{}, err
			}
			return s.result(authorization, true), nil
		}
		if !definitive {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return PixAutoAuthorization{}, ErrAmbiguous
		}
	}
	authorization, err := s.Provider.CreatePixAutoAuthorization(ctx, PixAutoProviderRequest{
		CustomerID: customerID, ContractID: contractID, Environment: s.Config.Environment,
		MerchantID: s.Config.MerchantID, Value: order.Amount, StartDate: startDate,
		FinishDate: finishDate, ImmediateExpiration: time.Hour,
	})
	if err != nil {
		if errors.Is(err, ErrAmbiguous) {
			_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, "")
			return PixAutoAuthorization{}, ErrAmbiguous
		}
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptFailed, "")
		return PixAutoAuthorization{}, err
	}
	if err := validatePixAuto(authorization, customerID, contractID, order.Amount); err != nil {
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, authorization.ID)
		return PixAutoAuthorization{}, err
	}
	if err := s.persist(ctx, order, principal.AccountID, customerID, contractID, authorization, now); err != nil {
		_ = s.Store.MarkOutcome(ctx, claim, domain.AttemptUnknown, authorization.ID)
		return PixAutoAuthorization{}, err
	}
	if err := s.Store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, authorization.ID); err != nil {
		return PixAutoAuthorization{}, err
	}
	return s.result(authorization, false), nil
}

func (s *PixAutoService) Status(ctx context.Context, principal Principal, offerID string) (PixAutoAuthorization, error) {
	if s == nil || s.Store == nil || s.Provider == nil || !domain.ValidID(principal.AccountID) || offerID == "" {
		return PixAutoAuthorization{}, domain.ErrInvalid
	}
	mandate, err := s.Store.MandateForAccount(ctx, principal.AccountID, offerID)
	if err != nil {
		return PixAutoAuthorization{}, err
	}
	if mandate.AccountID != principal.AccountID || mandate.Environment != s.Config.Environment ||
		mandate.Merchant != s.Config.MerchantID {
		return PixAutoAuthorization{}, domain.ErrNotFound
	}
	authorization, err := s.Provider.RetrievePixAutoAuthorization(ctx, mandate.RemoteAuthorizationID)
	if err != nil {
		return PixAutoAuthorization{}, domain.ErrUnavailable
	}
	if authorization.ID != mandate.RemoteAuthorizationID || authorization.CustomerID != mandate.RemoteCustomerID ||
		authorization.ContractID != mandate.ContractID {
		return PixAutoAuthorization{}, domain.ErrBinding
	}
	mandate.Status = authorization.Status
	mandate.RemoteSubscriptionID = authorization.RemoteSubscriptionID
	mandate.LastEventAt = s.now()
	if err := s.Store.UpsertMandate(ctx, mandate); err != nil {
		return PixAutoAuthorization{}, err
	}
	if authorization.Status == "ACTIVE" {
		eligibility, found, err := s.Store.Eligibility(ctx, s.Config.Environment, s.Config.MerchantID)
		if err != nil {
			return PixAutoAuthorization{}, err
		}
		if found && eligibility.Status == "INELIGIBLE" {
			if err := cancelPixAutoMandate(ctx, s.Store, s.Provider, mandate, pixAutoCancelIneligible, s.now()); err != nil {
				return PixAutoAuthorization{}, err
			}
			authorization.Status = "CANCELLED"
		}
	}
	if mandate.CancelRequestedAt != nil && !pixAutoTerminal(authorization.Status) {
		authorization.CancellationPending = true
	}
	return s.result(authorization, true), nil
}

func (s *PixAutoService) Cancel(ctx context.Context, principal Principal, offerID string) error {
	if s == nil || s.Store == nil || s.Provider == nil || !domain.ValidID(principal.AccountID) || offerID == "" {
		return domain.ErrInvalid
	}
	mandate, err := s.Store.MandateForAccount(ctx, principal.AccountID, offerID)
	if err != nil {
		return err
	}
	if mandate.AccountID != principal.AccountID || mandate.Environment != s.Config.Environment ||
		mandate.Merchant != s.Config.MerchantID {
		return domain.ErrNotFound
	}
	if mandate.Status == "CANCELLED" || mandate.Status == "EXPIRED" || mandate.Status == "REFUSED" {
		return nil
	}
	return cancelPixAutoMandate(ctx, s.Store, s.Provider, mandate, "OWNER_REQUEST", s.now())
}

func (s *PixAutoService) persist(ctx context.Context, order domain.Order, accountID, customerID, contractID string, authorization PixAutoAuthorization, now time.Time) error {
	if err := validatePixAuto(authorization, customerID, contractID, order.Amount); err != nil {
		return err
	}
	return s.Store.UpsertMandate(ctx, MandateRecord{
		ID: mandateUUID(authorization.ID), AccountID: accountID, OfferID: order.OfferID,
		OrderID: order.ID, Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		Provider: domain.ProviderAsaas, RemoteAuthorizationID: authorization.ID,
		RemoteSubscriptionID: authorization.RemoteSubscriptionID, ContractID: contractID,
		RemoteCustomerID: customerID, Status: authorization.Status, Frequency: "MONTHLY",
		PaymentCreationMode: authorization.PaymentCreationMode, RetryPolicy: authorization.RetryPolicy,
		Value: authorization.Value, StartDate: authorization.StartDate, FinishDate: authorization.FinishDate,
		ConsentVersion: s.Config.ConsentVersion, ConsentAt: now, LastEventAt: now,
	})
}

func (s *PixAutoService) ensureCustomer(ctx context.Context, accountID string, request PixAutoStartRequest) (string, error) {
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
	name, taxID, email, err := validatedPayer(PixStartRequest{
		PayerName: request.PayerName, PayerTaxID: request.PayerTaxID, PayerEmail: request.PayerEmail,
	})
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
	mapping := CustomerMapping{AccountID: accountID, Provider: domain.ProviderAsaas,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID, RemoteCustomerID: customer.ID}
	if err := s.Store.MapCustomer(ctx, mapping); err != nil {
		existing, ok, readErr := s.Store.Customer(ctx, accountID, domain.ProviderAsaas, s.Config.Environment, s.Config.MerchantID)
		if readErr == nil && ok && existing == customer.ID {
			return existing, nil
		}
		return "", err
	}
	return customer.ID, nil
}

func (s *PixAutoService) validateConfig() error {
	if !s.Config.NewSalesEnabled {
		return domain.ErrSaleDisabled
	}
	if !s.Config.MerchantEligible {
		return domain.ErrUnavailable
	}
	if s.Config.Environment != domain.EnvironmentTest && s.Config.Environment != domain.EnvironmentProduction {
		return domain.ErrUnavailable
	}
	if s.Config.MerchantID == "" || s.Config.CatalogVersion == "" || s.Config.ConsentVersion == "" ||
		s.Catalog.Version != s.Config.CatalogVersion {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *PixAutoService) result(authorization PixAutoAuthorization, replayed bool) PixAutoAuthorization {
	authorization.Replayed = replayed
	return authorization
}

func (s *PixAutoService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *PixAutoService) months() int {
	if s.Config.MandateMonths > 0 {
		return s.Config.MandateMonths
	}
	return 12
}

func (s *PixAutoService) lease() time.Duration {
	if s.Config.LeaseTTL > 0 {
		return s.Config.LeaseTTL
	}
	return pixAutoLeaseTTL
}

func validatePixAuto(authorization PixAutoAuthorization, customerID, contractID string, amount domain.Money) error {
	switch authorization.Status {
	case "CREATED", "ACTIVE", "REFUSED", "CANCELLED", "EXPIRED":
	default:
		return domain.ErrUnavailable
	}
	if authorization.ID == "" || authorization.CustomerID != customerID ||
		authorization.ContractID != contractID || !authorization.Value.Equal(amount) ||
		authorization.Value.Currency != "BRL" ||
		authorization.PaymentCreationMode != "SUBSCRIPTION" || authorization.RetryPolicy != "NOT_ALLOWED" {
		return domain.ErrBinding
	}
	if (authorization.QRCode != "" && len(authorization.QRCode) > 4096) ||
		(authorization.QRImage != "" && len(authorization.QRImage) > 64*1024) {
		return domain.ErrBinding
	}
	return nil
}

func pixAutoOrderID(accountID, operationID string) string {
	sum := sha256.Sum256([]byte("payment-pix-auto-order\x00" + accountID + "\x00" + operationID))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func pixAutoPayload(order domain.Order, config PixAutoConfig, customerID, contractID string, start, finish time.Time) string {
	hash := sha256.New()
	for _, value := range []string{
		order.AccountID, order.ID, order.OfferID, order.CatalogVersion, string(order.Channel),
		order.Amount.Currency, config.MerchantID, string(config.Environment), customerID,
		contractID, config.ConsentVersion, start.UTC().Format("2006-01-02"), finish.UTC().Format("2006-01-02"),
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func mandateUUID(remoteID string) string {
	sum := sha256.Sum256([]byte("payment-mandate\x00" + remoteID))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}
