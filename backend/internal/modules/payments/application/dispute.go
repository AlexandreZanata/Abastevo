package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// DisputeService reconciles chargebacks and Pix MED/hold lifecycles against
// verified original payments. A pending dispute never revokes access; only a
// confirmed loss on a matched unexpired grant revokes. Unmatched money becomes
// an explicit provider hold for operator review, never access.
type DisputeService struct {
	Store   DisputeStore
	Stripe  DisputeVerifier
	Asaas   DisputeVerifier
	Proof   OperatorProof
	Clock   func() time.Time
	nowFunc func() time.Time
}

func (s *DisputeService) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc().UTC()
	}
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *DisputeService) verifierFor(provider domain.Provider) (DisputeVerifier, error) {
	switch provider {
	case domain.ProviderStripe:
		if s.Stripe == nil {
			return nil, domain.ErrUnavailable
		}
		return s.Stripe, nil
	case domain.ProviderAsaas:
		if s.Asaas == nil {
			return nil, domain.ErrUnavailable
		}
		return s.Asaas, nil
	default:
		return nil, domain.ErrInvalid
	}
}

func (s *DisputeService) Process(ctx context.Context, event InboxRecord) error {
	if s == nil || s.Store == nil {
		return domain.ErrUnavailable
	}
	if event.State != "" && event.State != "PENDING" {
		return nil
	}
	verifier, err := s.verifierFor(event.Provider)
	if err != nil {
		return s.conflict(ctx, event, err)
	}
	fact, err := verifier.VerifyDispute(ctx, event)
	if err != nil {
		if errors.Is(err, ErrIgnored) {
			return s.markProcessed(ctx, event)
		}
		if errors.Is(err, domain.ErrUnavailable) || errors.Is(err, ErrAmbiguous) {
			return err
		}
		return s.conflict(ctx, event, err)
	}
	if fact.Provider != event.Provider || fact.Environment != event.Environment || fact.Merchant != event.Merchant {
		return s.conflict(ctx, event, domain.ErrBinding)
	}
	record, created, err := s.Store.RecordDispute(ctx, DisputeRecord{
		Provider: fact.Provider, Environment: fact.Environment, Merchant: fact.Merchant,
		RemoteDisputeID: fact.RemoteDisputeID, RemotePaymentID: fact.RemotePaymentID,
		RemoteChargeID: fact.RemoteChargeID, Amount: fact.Amount, State: fact.State,
		OpenedAt: fact.OpenedAt, LastEventAt: fact.LastEventAt,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return s.recordUnmatched(ctx, event, fact)
		}
		if errors.Is(err, domain.ErrUnavailable) || errors.Is(err, ErrAmbiguous) {
			return err
		}
		return s.conflict(ctx, event, err)
	}
	if !created && record.State != fact.State {
		if !domain.DisputeTransitionAllowed(record.State, fact.State) {
			return s.markProcessed(ctx, event)
		}
		if _, err := s.Store.UpdateDisputeState(ctx, record.ID, record.State, fact.State, s.now()); err != nil {
			if errors.Is(err, domain.ErrUnavailable) || errors.Is(err, ErrAmbiguous) {
				return err
			}
			return s.conflict(ctx, event, err)
		}
	}
	return s.markProcessed(ctx, event)
}

func (s *DisputeService) recordUnmatched(ctx context.Context, event InboxRecord, fact DisputeFact) error {
	holdID := "unmatched:" + fact.RemoteDisputeID
	if holdID == "unmatched:" {
		holdID = "unmatched:" + event.EventID
	}
	_, _, err := s.Store.RecordProviderHold(ctx, ProviderHold{
		Provider: fact.Provider, Environment: fact.Environment, Merchant: event.Merchant,
		RemoteHoldID: holdID, RemotePaymentID: fact.RemotePaymentID,
		Reason: "unmatched-provider-money", ObservedAt: s.now(),
	})
	if err != nil {
		return err
	}
	return s.markProcessed(ctx, event)
}

func (s *DisputeService) ReviewHold(ctx context.Context, actor, proof, holdID, decision, reviewReason string) (ProviderHold, error) {
	if s == nil || s.Store == nil || s.Proof == nil {
		return ProviderHold{}, domain.ErrUnavailable
	}
	if !validActor(actor) || proof == "" || !validID(holdID) || !validRefundReason(reviewReason) ||
		(decision != "REVIEWED" && decision != "DISMISSED") {
		return ProviderHold{}, domain.ErrInvalid
	}
	if err := s.Proof.VerifyOperator(ctx, actor, proof, OperatorActionHoldReview); err != nil {
		return ProviderHold{}, domain.ErrUnauthorized
	}
	hold, err := s.Store.ReviewProviderHold(ctx, holdID, actor, decision, reviewReason, s.now())
	if err != nil {
		return ProviderHold{}, err
	}
	_ = s.Store.RecordOperatorAction(ctx, OperatorAction{
		Actor: actor, Action: OperatorActionHoldReview, TargetID: hold.ID,
		Reason: reviewReason, Result: decision, CreatedAt: s.now(),
	})
	return hold, nil
}

func (s *DisputeService) markProcessed(ctx context.Context, event InboxRecord) error {
	return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
}

func (s *DisputeService) conflict(ctx context.Context, event InboxRecord, cause error) error {
	if err := s.Store.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "dispute-binding"); err != nil {
		return err
	}
	return errors.Join(domain.ErrConflict, cause)
}
