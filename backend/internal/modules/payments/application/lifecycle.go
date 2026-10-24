package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type LifecycleService struct {
	Store    LifecycleStore
	Verifier LifecycleVerifier
}

func (s *LifecycleService) Process(ctx context.Context, event InboxRecord) error {
	if s == nil || s.Store == nil || s.Verifier == nil {
		return domain.ErrUnavailable
	}
	if event.State != "" && event.State != "PENDING" {
		return nil
	}
	fact, err := s.Verifier.VerifyLifecycle(ctx, event)
	if err != nil {
		if errors.Is(err, ErrIgnored) {
			return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
		}
		if errors.Is(err, domain.ErrUnavailable) || errors.Is(err, ErrAmbiguous) {
			return err
		}
		_ = s.Store.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "lifecycle-binding")
		return domain.ErrConflict
	}
	if fact.Provider != event.Provider || fact.Environment != event.Environment || fact.Merchant != event.Merchant {
		_ = s.Store.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "lifecycle-binding")
		return domain.ErrConflict
	}
	if err := s.upsert(ctx, fact); err != nil {
		return err
	}
	return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
}

func (s *LifecycleService) ProcessPending(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant string, limit int) (int, error) {
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

func (s *LifecycleService) Reconcile(ctx context.Context, contract ContractRecord) error {
	if s == nil || s.Verifier == nil || contract.RemoteSubscriptionID == "" {
		return domain.ErrInvalid
	}
	fact, err := s.Verifier.RetrieveLifecycle(ctx, contract.RemoteSubscriptionID)
	if err != nil {
		return err
	}
	if fact.AccountID != contract.AccountID || fact.OfferID != contract.OfferID ||
		fact.Provider != contract.Provider || fact.Environment != contract.Environment ||
		fact.Merchant != contract.Merchant || fact.RemoteCustomerID != contract.RemoteCustomerID {
		return domain.ErrBinding
	}
	return s.upsert(ctx, fact)
}

func (s *LifecycleService) upsert(ctx context.Context, fact LifecycleFact) error {
	if fact.AccountID == "" || fact.OfferID == "" || fact.RemoteSubscriptionID == "" ||
		fact.RemoteCustomerID == "" || fact.ProviderStatus == "" || fact.LastEventAt.IsZero() {
		return domain.ErrBinding
	}
	return s.Store.UpsertContract(ctx, ContractRecord{
		ID: contractUUID(fact.Provider, fact.RemoteSubscriptionID), AccountID: fact.AccountID,
		OfferID: fact.OfferID, Source: domain.ChannelCard, Environment: fact.Environment,
		Merchant: fact.Merchant, Provider: fact.Provider, RemoteSubscriptionID: fact.RemoteSubscriptionID,
		RemoteCustomerID: fact.RemoteCustomerID, ProviderStatus: fact.ProviderStatus,
		CancelAtPeriodEnd: fact.CancelAtPeriodEnd, LastEventAt: fact.LastEventAt,
	})
}

type PortalService struct {
	Store    LifecycleStore
	Provider PortalProvider
	Config   PortalConfig
}

func (s *PortalService) Manage(ctx context.Context, principal Principal, offerID string) (string, error) {
	if s == nil || s.Store == nil || s.Provider == nil || !domain.ValidID(principal.AccountID) ||
		offerID == "" || len(offerID) > 128 {
		return "", domain.ErrInvalid
	}
	if strings.TrimSpace(s.Config.ReturnOrigin) == "" || strings.TrimSpace(s.Config.ConfigurationID) == "" ||
		s.Config.Merchant == "" || (s.Config.Environment != domain.EnvironmentTest && s.Config.Environment != domain.EnvironmentProduction) {
		return "", domain.ErrUnavailable
	}
	contract, err := s.Store.ContractForAccount(ctx, principal.AccountID, offerID)
	if err != nil {
		return "", err
	}
	if contract.AccountID != principal.AccountID || contract.Environment != s.Config.Environment ||
		contract.Merchant != s.Config.Merchant || contract.RemoteCustomerID == "" {
		return "", domain.ErrBinding
	}
	return s.Provider.CreatePortalSession(ctx, PortalRequest{
		CustomerID:      contract.RemoteCustomerID,
		ReturnURL:       strings.TrimRight(s.Config.ReturnOrigin, "/") + "/billing/manage",
		ConfigurationID: s.Config.ConfigurationID,
	})
}

func contractUUID(provider domain.Provider, remoteID string) string {
	sum := sha256.Sum256([]byte("payment-contract\x00" + string(provider) + "\x00" + remoteID))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}
