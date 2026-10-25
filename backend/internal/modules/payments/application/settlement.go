package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type SettlementService struct {
	Store    SettlementStore
	Catalog  domain.Catalog
	Verifier SettlementVerifier
}

func (s *SettlementService) Process(ctx context.Context, event InboxRecord) error {
	if s == nil || s.Store == nil || s.Verifier == nil {
		return domain.ErrUnavailable
	}
	if event.State != "" && event.State != "PENDING" {
		return nil
	}
	candidate, err := s.Verifier.Verify(ctx, event)
	if err != nil {
		if errors.Is(err, ErrIgnored) || errors.Is(err, ErrNotSettled) {
			return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
		}
		if errors.Is(err, domain.ErrUnavailable) || errors.Is(err, ErrAmbiguous) || errors.Is(err, ErrProviderUnauthorized) {
			return err
		}
		return s.conflict(ctx, event, err)
	}
	if candidate.Provider != event.Provider || candidate.Environment != event.Environment ||
		candidate.Merchant != event.Merchant {
		return s.conflict(ctx, event, domain.ErrBinding)
	}
	if candidate.SettledAt.IsZero() {
		candidate.SettledAt = event.ReceivedAt
	}
	if candidate.SettledAt.IsZero() {
		candidate.SettledAt = time.Now().UTC()
	}
	if err := s.Settle(ctx, candidate); err != nil {
		if errors.Is(err, domain.ErrUnavailable) || errors.Is(err, domain.ErrStale) ||
			errors.Is(err, ErrAmbiguous) || errors.Is(err, ErrProviderUnauthorized) {
			return err
		}
		return s.conflict(ctx, event, err)
	}
	return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
}

// Settle records one already-verified provider payment. Provider-owned automatic
// Pix cycles always receive a fresh cycle order; the mandate order is
// authorization context, not a reusable paid period.
func (s *SettlementService) Settle(ctx context.Context, candidate SettlementCandidate) error {
	if s == nil || s.Store == nil || s.Catalog.Version == "" {
		return domain.ErrUnavailable
	}
	if candidate.Provider != domain.ProviderAsaas && candidate.Provider != domain.ProviderStripe {
		return domain.ErrInvalid
	}
	if err := candidate.Environment.Validate(); err != nil {
		return err
	}
	if candidate.RemotePaymentID == "" || len(candidate.RemotePaymentID) > 255 ||
		candidate.CustomerID == "" || candidate.ProviderStatus == "" || candidate.SettledAt.IsZero() {
		return domain.ErrInvalid
	}
	if err := candidate.Amount.Positive(); err != nil {
		return err
	}
	orderID := candidate.OrderID
	if candidate.Channel == domain.ChannelPixAutomatic {
		if candidate.Provider != domain.ProviderAsaas || candidate.RemoteSubscriptionID == "" {
			return domain.ErrBinding
		}
		mandate, err := s.Store.MandateForSubscription(ctx, candidate.Environment, candidate.Merchant, candidate.RemoteSubscriptionID)
		if err != nil {
			return err
		}
		if mandate.Provider != domain.ProviderAsaas || mandate.AccountID == "" || mandate.OrderID == "" ||
			mandate.RemoteSubscriptionID != candidate.RemoteSubscriptionID ||
			mandate.RemoteCustomerID != candidate.CustomerID || !mandate.Value.Equal(candidate.Amount) {
			return domain.ErrBinding
		}
		initial, err := s.Store.GetOrder(ctx, mandate.OrderID)
		if err != nil {
			return err
		}
		if initial.AccountID != mandate.AccountID || initial.OfferID != mandate.OfferID ||
			initial.Channel != domain.ChannelPixAutomatic || initial.CatalogVersion == "" ||
			len(initial.Capabilities) == 0 {
			return domain.ErrBinding
		}
		orderID, err = s.Store.EnsurePixAutoCycleOrder(ctx, PixAutoCycleOrderInput{
			AccountID: mandate.AccountID, OfferID: mandate.OfferID, Family: initial.Family,
			CatalogVersion: initial.CatalogVersion, Capabilities: initial.Capabilities,
			Amount: mandate.Value, Environment: candidate.Environment, Merchant: candidate.Merchant,
			RemotePaymentID: candidate.RemotePaymentID, RemoteSubscriptionID: candidate.RemoteSubscriptionID,
			SettledAt: candidate.SettledAt,
		})
		if err != nil {
			return err
		}
	} else {
		if orderID == "" || candidate.RemoteSubscriptionID != "" {
			return domain.ErrInvalid
		}
	}
	order, err := s.Store.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.AccountID == "" || order.Channel != candidate.Channel || !order.Amount.Equal(candidate.Amount) ||
		order.CatalogVersion != s.Catalog.Version || order.OfferID == "" {
		return domain.ErrBinding
	}
	offer, ok := s.Catalog.Offers[order.OfferID]
	if !ok || !offer.Price.Equal(candidate.Amount) || !sameStrings(offer.Capabilities, order.Capabilities) ||
		!offer.Channels[candidate.Channel] || !capabilitiesAdmit(offer, order) {
		return domain.ErrBinding
	}
	attempt, err := s.Store.AttemptForOrder(ctx, order.ID)
	if err != nil {
		return err
	}
	switch attempt.State {
	case "SUCCEEDED", "DISPATCHED", "UNKNOWN":
	default:
		return domain.ErrConflict
	}
	if attempt.Provider != candidate.Provider || attempt.Environment != candidate.Environment ||
		attempt.Merchant != candidate.Merchant {
		return domain.ErrBinding
	}
	customerID, found, err := s.Store.Customer(ctx, order.AccountID, candidate.Provider, candidate.Environment, candidate.Merchant)
	if err != nil {
		return err
	}
	if !found || customerID != candidate.CustomerID {
		return domain.ErrBinding
	}
	settlement := domain.Settlement{
		ID:      settlementUUID(candidate.Provider, candidate.RemotePaymentID),
		OrderID: order.ID, AttemptID: attempt.ID, AccountID: order.AccountID,
		OfferID: order.OfferID, Channel: candidate.Channel, Provider: candidate.Provider,
		Environment: candidate.Environment, MerchantID: candidate.Merchant,
		RemotePaymentID: candidate.RemotePaymentID, RemoteChargeID: candidate.RemoteChargeID,
		ProviderStatus: candidate.ProviderStatus,
		Amount:         candidate.Amount, Status: domain.SettlementSettled,
		Authority: domain.AuthorityProviderVerified, SettledAt: candidate.SettledAt,
	}
	_, err = s.Store.RecordSettlement(ctx, SettlementInput{Offer: offer, Settlement: settlement})
	return err
}

func (s *SettlementService) ProcessPending(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant string, limit int) (int, error) {
	if s == nil || s.Store == nil {
		return 0, domain.ErrUnavailable
	}
	events, err := s.Store.PendingInbox(ctx, provider, environment, merchant, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		if err := s.Process(ctx, event); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *SettlementService) conflict(ctx context.Context, event InboxRecord, cause error) error {
	if err := s.Store.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "settlement-binding"); err != nil {
		return err
	}
	return errors.Join(domain.ErrConflict, cause)
}

func settlementUUID(provider domain.Provider, remoteID string) string {
	sum := sha256.Sum256([]byte("payment-settlement\x00" + string(provider) + "\x00" + remoteID))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

// capabilitiesAdmit enforces capability-target isolation: capability lists must
// match, and only an approved enterprise offer may settle a period with no app
// capabilities. Premium periods always carry capabilities.
func capabilitiesAdmit(offer domain.Offer, order domain.Order) bool {
	if len(offer.Capabilities) != 0 || len(order.Capabilities) != 0 {
		return len(offer.Capabilities) != 0 && len(order.Capabilities) != 0
	}
	return offer.Enterprise && offer.Family == domain.EnterpriseFamily &&
		order.Family == domain.EnterpriseFamily
}
