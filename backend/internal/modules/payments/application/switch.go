package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const sourceSwitchLeaseTTL = 30 * time.Second

// SwitchService fences one recurring funding source before another may debit.
// Provider I/O happens outside DB transactions; the durable switch record keeps
// unknown cancellation pending.
type SwitchService struct {
	Store    SwitchStore
	Catalog  domain.Catalog
	Config   SwitchConfig
	Stripe   StripeRenewalFencer
	PixAuto  *PixAutoService
	Clock    func() time.Time
	WorkerID string
	LeaseTTL time.Duration
}

func (s *SwitchService) Request(ctx context.Context, principal Principal, request SwitchRequest) (SourceSwitchRecord, error) {
	if s == nil || s.Store == nil || s.Catalog.Version == "" {
		return SourceSwitchRecord{}, domain.ErrUnavailable
	}
	if !domain.ValidID(principal.AccountID) || !domain.ValidID(request.OperationID) ||
		request.OfferID == "" || len(request.OfferID) > 128 {
		return SourceSwitchRecord{}, domain.ErrInvalid
	}
	if err := s.validateConfig(request.NewChannel); err != nil {
		return SourceSwitchRecord{}, err
	}
	now := s.now()
	quote, err := s.Catalog.Quote(domain.QuoteRequest{OfferID: request.OfferID, Channel: request.NewChannel}, now)
	if err != nil {
		return SourceSwitchRecord{}, err
	}
	active, found, err := activeFamilySource(ctx, s.Store, s.Catalog, principal.AccountID, request.OfferID)
	if err != nil {
		return SourceSwitchRecord{}, err
	}
	currentEnd, err := activeFamilyGrantEnd(ctx, s.Store, s.Catalog, principal.AccountID, request.OfferID)
	if err != nil {
		return SourceSwitchRecord{}, err
	}
	if open, exists, err := openFamilySwitch(ctx, s.Store, s.Catalog, principal.AccountID, request.OfferID); err != nil {
		return SourceSwitchRecord{}, err
	} else if exists {
		if open.OperationKey == request.OperationID {
			if open.OfferID != quote.OfferID || open.NewSource != request.NewChannel ||
				open.TargetCatalogVersion != quote.CatalogVersion ||
				!open.TargetAmount.Equal(quote.Amount) ||
				!sameStrings(open.TargetCapabilities, quote.Capabilities) {
				return SourceSwitchRecord{}, domain.ErrConflict
			}
			return open, nil
		}
		if open.State == SourceSwitchReady && found && active.Source == open.NewSource {
			if err := s.Store.ConsumeSourceSwitch(ctx, open.ID); err != nil {
				return SourceSwitchRecord{}, err
			}
		} else {
			return SourceSwitchRecord{}, domain.ErrConflict
		}
	}
	if found && active.Source == request.NewChannel && active.OfferID == request.OfferID {
		return SourceSwitchRecord{}, domain.ErrConflict
	}
	if request.Proration {
		return SourceSwitchRecord{}, domain.ErrFeatureUnsupported
	}
	if found && active.OfferID != request.OfferID {
		currentOffer, currentOK := s.Catalog.Offers[active.OfferID]
		targetOffer, targetOK := s.Catalog.Offers[request.OfferID]
		if !currentOK || !targetOK {
			return SourceSwitchRecord{}, domain.ErrUnknownOffer
		}
		decision, err := domain.DecidePlanChange(domain.PlanChangeRequest{
			Current: currentOffer, Target: targetOffer, CurrentEnd: currentEnd,
			RequestedAt: now, Proration: request.Proration,
		})
		if err != nil {
			return SourceSwitchRecord{}, err
		}
		if !decision.Allowed {
			return SourceSwitchRecord{}, domain.ErrFeatureUnsupported
		}
	}
	record := SourceSwitchRecord{
		AccountID: principal.AccountID, OfferID: request.OfferID,
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		OperationKey: request.OperationID, NewSource: request.NewChannel,
		OldSource: domain.ChannelPixManual, OldOfferID: request.OfferID,
		OldCancellation:      domain.CancellationNotApplicable,
		TargetCatalogVersion: quote.CatalogVersion, TargetAmount: quote.Amount,
		TargetCapabilities: append([]string(nil), quote.Capabilities...),
		CurrentEnd:         currentEnd, State: SourceSwitchPending,
		RequestedAt: now, LastEventAt: now,
	}
	if found {
		record.OldSource = active.Source
		record.OldOfferID = active.OfferID
		record.OldLocalID = active.LocalID
		record.OldRemoteID = active.RemoteID
		record.OldCancellation = domain.CancellationUnknown
	}
	created, replay, err := s.Store.CreateSourceSwitch(ctx, record)
	if err != nil {
		return SourceSwitchRecord{}, err
	}
	if replay {
		if created.OfferID != record.OfferID || created.NewSource != record.NewSource ||
			created.TargetCatalogVersion != record.TargetCatalogVersion ||
			!created.TargetAmount.Equal(record.TargetAmount) ||
			!sameStrings(created.TargetCapabilities, record.TargetCapabilities) {
			return SourceSwitchRecord{}, domain.ErrConflict
		}
		return created, nil
	}
	record = created
	if record.OldCancellation == domain.CancellationNotApplicable {
		return s.resolveReady(ctx, record)
	}
	claimed, err := s.Store.ClaimSourceSwitch(ctx, record.ID, s.workerID(), now.Add(s.leaseTTL()))
	if err != nil {
		return record, err
	}
	if !claimed {
		return record, nil
	}
	return s.fenceAndResolve(ctx, record)
}

// Reconcile retries one pending switch using canonical provider/local state. It
// is safe after a crash because the intent and its cancellation key are durable.
func (s *SwitchService) Reconcile(ctx context.Context, record SourceSwitchRecord) (SourceSwitchRecord, error) {
	if s == nil || s.Store == nil {
		return SourceSwitchRecord{}, domain.ErrUnavailable
	}
	if record.State != SourceSwitchPending && record.State != SourceSwitchCanceling {
		return record, nil
	}
	claimed, err := s.Store.ClaimSourceSwitch(ctx, record.ID, s.workerID(), s.now().Add(s.leaseTTL()))
	if err != nil || !claimed {
		return record, err
	}
	active, found, err := activeFamilySource(ctx, s.Store, s.Catalog, record.AccountID, record.OfferID)
	if err != nil {
		return record, err
	}
	if found && (active.Source != record.OldSource || active.OfferID != record.OldOfferID) {
		record.LastError = "another recurring source is active"
		record.State = SourceSwitchPending
		record.OldCancellation = domain.CancellationUnknown
		record.LastEventAt = s.now()
		_ = s.Store.ResolveSourceSwitch(ctx, record)
		return record, domain.ErrConflict
	}
	return s.fenceAndResolve(ctx, record)
}

func (s *SwitchService) fenceAndResolve(ctx context.Context, record SourceSwitchRecord) (SourceSwitchRecord, error) {
	confirmed, cause := s.fence(ctx, record)
	now := s.now()
	record.LastEventAt = now
	if confirmed {
		return s.resolveReady(ctx, record)
	}
	record.State = SourceSwitchPending
	record.OldCancellation = domain.CancellationUnknown
	if cause != nil {
		record.LastError = boundedError(cause)
	}
	if err := s.Store.ResolveSourceSwitch(ctx, record); err != nil {
		return record, err
	}
	if cause == nil {
		cause = ErrAmbiguous
	}
	return record, cause
}

func (s *SwitchService) fence(ctx context.Context, record SourceSwitchRecord) (bool, error) {
	switch record.OldSource {
	case domain.ChannelCard:
		if s.Stripe == nil || record.OldRemoteID == "" || record.OldLocalID == "" {
			return false, domain.ErrUnavailable
		}
		fact, err := s.Stripe.RetrieveLifecycle(ctx, record.OldRemoteID)
		if err != nil {
			return false, err
		}
		if err := s.persistStripeFact(ctx, record, fact); err != nil {
			return false, err
		}
		if stripeRenewalFenced(fact) {
			return true, nil
		}
		if err := s.Stripe.CancelRenewal(ctx, record.OldRemoteID, "pay:switch:"+record.ID+":cancel"); err != nil {
			return false, err
		}
		fact, err = s.Stripe.RetrieveLifecycle(ctx, record.OldRemoteID)
		if err != nil {
			return false, err
		}
		if err := s.persistStripeFact(ctx, record, fact); err != nil {
			return false, err
		}
		return stripeRenewalFenced(fact), nil
	case domain.ChannelPixAutomatic:
		if s.PixAuto == nil {
			return false, domain.ErrUnavailable
		}
		oldOfferID := record.OldOfferID
		if oldOfferID == "" {
			oldOfferID = record.OfferID
		}
		mandate, err := s.Store.MandateForAccount(ctx, record.AccountID, oldOfferID)
		if err != nil {
			return false, err
		}
		if err := cancelPixAutoMandate(ctx, s.PixAuto.Store, s.PixAuto.Provider, mandate, sourceSwitchCancelReason, s.now()); err != nil {
			return false, err
		}
		mandate, err = s.Store.MandateForAccount(ctx, record.AccountID, oldOfferID)
		if err != nil {
			return false, err
		}
		return mandate.Status == "CANCELLED" || mandate.Status == "EXPIRED" || mandate.Status == "REFUSED", nil
	default:
		return true, nil
	}
}

func (s *SwitchService) persistStripeFact(ctx context.Context, record SourceSwitchRecord, fact LifecycleFact) error {
	if fact.AccountID != record.AccountID || fact.OfferID != record.OldOfferID ||
		fact.Provider != domain.ProviderStripe || fact.Environment != record.Environment ||
		fact.Merchant != record.Merchant || fact.RemoteSubscriptionID != record.OldRemoteID {
		return domain.ErrBinding
	}
	return s.Store.UpsertContract(ctx, ContractRecord{
		ID: record.OldLocalID, AccountID: fact.AccountID, OfferID: fact.OfferID,
		Source: domain.ChannelCard, Environment: fact.Environment, Merchant: fact.Merchant,
		Provider: fact.Provider, RemoteSubscriptionID: fact.RemoteSubscriptionID,
		RemoteCustomerID: fact.RemoteCustomerID, ProviderStatus: fact.ProviderStatus,
		CancelAtPeriodEnd: fact.CancelAtPeriodEnd, LastEventAt: fact.LastEventAt,
	})
}

func stripeRenewalFenced(fact LifecycleFact) bool {
	return fact.CancelAtPeriodEnd || fact.ProviderStatus == "canceled" || fact.ProviderStatus == "incomplete_expired"
}

func (s *SwitchService) resolveReady(ctx context.Context, record SourceSwitchRecord) (SourceSwitchRecord, error) {
	cancellation := domain.CancellationConfirmed
	if record.OldSource == domain.ChannelPixManual {
		cancellation = domain.CancellationNotApplicable
	}
	firstDebit := time.Time{}
	reason := ""
	if record.OldSource == record.NewSource && record.OldOfferID != record.OfferID {
		currentOffer, currentOK := s.Catalog.Offers[record.OldOfferID]
		targetOffer, targetOK := s.Catalog.Offers[record.OfferID]
		if !currentOK || !targetOK {
			return record, domain.ErrUnknownOffer
		}
		decision, err := domain.DecidePlanChange(domain.PlanChangeRequest{
			Current: currentOffer, Target: targetOffer, CurrentEnd: record.CurrentEnd,
			RequestedAt: record.RequestedAt,
		})
		if err != nil {
			return record, err
		}
		if !decision.Allowed {
			reason = decision.Reason
		} else {
			firstDebit = decision.EffectiveAt
		}
	} else {
		decision, err := domain.DecideSwitch(domain.SwitchRequest{
			OldSource: record.OldSource, NewSource: record.NewSource,
			OldCancellation: cancellation, CurrentEnd: record.CurrentEnd,
			RequestedAt: record.RequestedAt,
		})
		if err != nil {
			return record, err
		}
		if !decision.Allowed {
			reason = decision.Reason
		} else {
			firstDebit = decision.FirstDebitAt
		}
	}
	if reason != "" {
		record.State = SourceSwitchPending
		record.OldCancellation = domain.CancellationUnknown
		record.LastError = reason
		record.LastEventAt = s.now()
		if err := s.Store.ResolveSourceSwitch(ctx, record); err != nil {
			return record, err
		}
		return record, domain.ErrConflict
	}
	record.State = SourceSwitchReady
	record.OldCancellation = cancellation
	record.FirstDebitAt = firstDebit
	record.LastError = ""
	record.LastEventAt = s.now()
	if err := s.Store.ResolveSourceSwitch(ctx, record); err != nil {
		return record, err
	}
	return record, nil
}

func (s *SwitchService) validateConfig(channel domain.Channel) error {
	if s.Config.Environment != domain.EnvironmentTest && s.Config.Environment != domain.EnvironmentProduction {
		return domain.ErrUnavailable
	}
	if s.Config.MerchantID == "" {
		return domain.ErrUnavailable
	}
	switch channel {
	case domain.ChannelCard:
		if !s.Config.CardReady {
			return domain.ErrUnavailable
		}
	case domain.ChannelPixAutomatic:
		if !s.Config.PixAutoReady {
			return domain.ErrUnavailable
		}
	default:
		return domain.ErrInvalid
	}
	return nil
}

func (s *SwitchService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *SwitchService) workerID() string {
	if s.WorkerID != "" {
		return s.WorkerID
	}
	return "payment-switch"
}

func (s *SwitchService) leaseTTL() time.Duration {
	if s.LeaseTTL > 0 {
		return s.LeaseTTL
	}
	return sourceSwitchLeaseTTL
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 500 {
		return text[:500]
	}
	return text
}
