package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const billingExitLeaseTTL = 30 * time.Second

type BillingExitService struct {
	Store    BillingExitStore
	Stripe   StripeRenewalFencer
	PixAuto  *PixAutoService
	Proof    OwnerProof
	Clock    func() time.Time
	WorkerID string
	LeaseTTL time.Duration
}

func (s *BillingExitService) Cancel(ctx context.Context, principal Principal, operationID, proof string) (BillingExit, error) {
	if s == nil || s.Store == nil || !domain.ValidID(principal.AccountID) ||
		!domain.ValidID(operationID) || proof == "" {
		return BillingExit{}, domain.ErrInvalid
	}
	if s.Proof == nil {
		return BillingExit{}, domain.ErrUnavailable
	}
	if err := s.Proof.VerifyFreshOwner(ctx, principal.AccountID, proof, billingCancelAction); err != nil {
		return BillingExit{}, domain.ErrUnauthorized
	}
	exit, _, _, err := s.Store.CreateBillingExit(ctx, principal.AccountID, operationID, BillingExitOwnerRequest, s.now())
	if err != nil {
		return BillingExit{}, err
	}
	exit, err = s.process(ctx, exit)
	if err != nil {
		return exit, err
	}
	if exit.State != "DONE" {
		return exit, ErrAmbiguous
	}
	return exit, nil
}

// RequestAccountExit records the durable intent used by account deletion. It
// never refunds and returns nil once the intent is stored even if provider
// cancellation is pending or unavailable.
func (s *BillingExitService) RequestAccountExit(ctx context.Context, accountID string) error {
	if s == nil || s.Store == nil || !domain.ValidID(accountID) {
		return domain.ErrInvalid
	}
	exit, _, _, err := s.Store.CreateBillingExit(ctx, accountID, "account-deletion", BillingExitAccountDeletion, s.now())
	if err != nil {
		return err
	}
	_, _ = s.process(ctx, exit)
	return nil
}

func (s *BillingExitService) Reconcile(ctx context.Context, exitID string) (BillingExit, error) {
	if s == nil || s.Store == nil || !domain.ValidID(exitID) {
		return BillingExit{}, domain.ErrInvalid
	}
	exit, err := s.Store.BillingExit(ctx, exitID)
	if err != nil {
		return BillingExit{}, err
	}
	return s.process(ctx, exit)
}

func (s *BillingExitService) Status(ctx context.Context, principal Principal) (BillingExitStatus, error) {
	if s == nil || s.Store == nil || !domain.ValidID(principal.AccountID) {
		return BillingExitStatus{}, domain.ErrInvalid
	}
	active, err := s.Store.BillingSources(ctx, principal.AccountID)
	if err != nil {
		return BillingExitStatus{}, err
	}
	status := BillingExitStatus{Active: active}
	exit, found, err := s.Store.LatestBillingExit(ctx, principal.AccountID)
	if err != nil {
		return BillingExitStatus{}, err
	}
	if !found {
		return status, nil
	}
	status.Exit = &exit
	sources, err := s.Store.BillingExitSources(ctx, exit.ID)
	if err != nil {
		return BillingExitStatus{}, err
	}
	status.Sources = sources
	return status, nil
}

func (s *BillingExitService) History(ctx context.Context, principal Principal, limit int) ([]BillingHistoryRow, error) {
	if s == nil || s.Store == nil || !domain.ValidID(principal.AccountID) {
		return nil, domain.ErrInvalid
	}
	return s.Store.BillingHistory(ctx, principal.AccountID, limit)
}

func (s *BillingExitService) Export(ctx context.Context, principal Principal, proof string, limit int) ([]BillingHistoryRow, error) {
	if s == nil || s.Store == nil || s.Proof == nil || !domain.ValidID(principal.AccountID) || proof == "" {
		return nil, domain.ErrInvalid
	}
	if err := s.Proof.VerifyFreshOwner(ctx, principal.AccountID, proof, billingExportAction); err != nil {
		return nil, domain.ErrUnauthorized
	}
	return s.Store.BillingHistory(ctx, principal.AccountID, limit)
}

func (s *BillingExitService) process(ctx context.Context, exit BillingExit) (BillingExit, error) {
	if exit.State == "DONE" {
		return exit, nil
	}
	sources, err := s.Store.BillingExitSources(ctx, exit.ID)
	if err != nil {
		return exit, err
	}
	for i := range sources {
		if sources[i].State == "DONE" {
			continue
		}
		claimed, err := s.Store.ClaimBillingExitSource(ctx, sources[i].ID, s.now().Add(s.leaseTTL()))
		if err != nil {
			return exit, err
		}
		if !claimed {
			continue
		}
		source := sources[i]
		done, lastErr := s.cancelSource(ctx, source)
		source.LastEventAt = s.now()
		if done {
			source.State = "DONE"
			source.LastError = ""
		} else if lastErr != nil && (errors.Is(lastErr, domain.ErrUnavailable) || errors.Is(lastErr, ErrAmbiguous)) {
			source.State = "PENDING"
			source.LastError = boundedError(lastErr)
		} else {
			source.State = "REVIEW"
			source.LastError = boundedError(lastErr)
		}
		if err := s.Store.ResolveBillingExitSource(ctx, source); err != nil {
			return exit, err
		}
		sources[i] = source
	}
	state := "PENDING"
	lastError := ""
	for _, source := range sources {
		if source.State == "REVIEW" {
			state = "REVIEW"
			lastError = source.LastError
			break
		}
		if source.State != "DONE" {
			state = "PENDING"
			if source.LastError != "" {
				lastError = source.LastError
			}
		}
	}
	allDone := true
	for _, source := range sources {
		if source.State != "DONE" {
			allDone = false
			break
		}
	}
	if allDone {
		state = "DONE"
		lastError = ""
	}
	exit.State = state
	exit.LastError = lastError
	exit.LastEventAt = s.now()
	if err := s.Store.UpdateBillingExit(ctx, exit); err != nil {
		return exit, err
	}
	return exit, nil
}

func (s *BillingExitService) cancelSource(ctx context.Context, source BillingExitSource) (bool, error) {
	switch source.Source {
	case domain.ChannelCard:
		if s.Stripe == nil {
			return false, domain.ErrUnavailable
		}
		fact, err := s.Stripe.RetrieveLifecycle(ctx, source.RemoteID)
		if err != nil {
			return false, err
		}
		if fact.AccountID != source.AccountID || fact.OfferID != source.OfferID ||
			fact.RemoteSubscriptionID != source.RemoteID || fact.Environment != source.Environment ||
			fact.Merchant != source.Merchant {
			return false, domain.ErrBinding
		}
		if stripeRenewalFenced(fact) {
			return true, s.persistBillingCard(ctx, source, fact)
		}
		if err := s.Stripe.CancelRenewal(ctx, source.RemoteID, "pay:exit:"+source.ID+":cancel"); err != nil {
			return false, err
		}
		fact, err = s.Stripe.RetrieveLifecycle(ctx, source.RemoteID)
		if err != nil {
			return false, err
		}
		if err := s.persistBillingCard(ctx, source, fact); err != nil {
			return false, err
		}
		return stripeRenewalFenced(fact), nil
	case domain.ChannelPixAutomatic:
		if s.PixAuto == nil || s.PixAuto.Provider == nil {
			return false, domain.ErrUnavailable
		}
		if s.PixAuto.Config.Environment != source.Environment || s.PixAuto.Config.MerchantID != source.Merchant {
			return false, domain.ErrBinding
		}
		authorization, err := s.PixAuto.Provider.RetrievePixAutoAuthorization(ctx, source.RemoteID)
		if err != nil {
			return false, err
		}
		if authorization.ID != source.RemoteID {
			return false, domain.ErrBinding
		}
		mandate, err := s.Store.MandateForAccount(ctx, source.AccountID, source.OfferID)
		if err == nil {
			if authorization.CustomerID != mandate.RemoteCustomerID ||
				authorization.ContractID != mandate.ContractID {
				return false, domain.ErrBinding
			}
		} else if !errors.Is(err, domain.ErrNotFound) {
			return false, err
		}
		if pixAutoTerminal(authorization.Status) {
			return true, nil
		}
		if err := s.PixAuto.Provider.CancelPixAutoAuthorization(ctx, source.RemoteID); err != nil {
			return false, err
		}
		authorization, err = s.PixAuto.Provider.RetrievePixAutoAuthorization(ctx, source.RemoteID)
		if err != nil {
			return false, err
		}
		if !pixAutoTerminal(authorization.Status) {
			return false, ErrAmbiguous
		}
		if mandate.RemoteAuthorizationID != "" {
			mandate.Status = authorization.Status
			mandate.LastEventAt = s.now()
			_ = s.Store.UpsertMandate(ctx, mandate)
		}
		return true, nil
	default:
		return false, domain.ErrInvalid
	}
}

func (s *BillingExitService) persistBillingCard(ctx context.Context, source BillingExitSource, fact LifecycleFact) error {
	return s.Store.UpsertContract(ctx, ContractRecord{
		ID: source.LocalID, AccountID: source.AccountID, OfferID: source.OfferID,
		Source: domain.ChannelCard, Environment: source.Environment, Merchant: source.Merchant,
		Provider: domain.ProviderStripe, RemoteSubscriptionID: fact.RemoteSubscriptionID,
		RemoteCustomerID: fact.RemoteCustomerID, ProviderStatus: fact.ProviderStatus,
		CancelAtPeriodEnd: fact.CancelAtPeriodEnd, LastEventAt: fact.LastEventAt,
	})
}

func (s *BillingExitService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *BillingExitService) leaseTTL() time.Duration {
	if s.LeaseTTL > 0 {
		return s.LeaseTTL
	}
	return billingExitLeaseTTL
}
