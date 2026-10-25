package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// RefundService executes restricted, original-route refunds. A refund always
// resolves against a verified settled payment, reserves the refundable balance
// under a lock before any provider write, and never revokes more than the
// matched unexpired paid period.
type RefundService struct {
	Store  RefundStore
	Stripe RefundProvider
	Asaas  RefundProvider
	Proof  OperatorProof
	Config RefundServiceConfig
	Clock  func() time.Time
}

func (s *RefundService) providerFor(provider domain.Provider) (RefundProvider, error) {
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

func (s *RefundService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *RefundService) Request(ctx context.Context, input RefundRequest) (RefundRecord, error) {
	if s == nil || s.Store == nil {
		return RefundRecord{}, domain.ErrUnavailable
	}
	if err := input.Provider.Validate(); err != nil {
		return RefundRecord{}, err
	}
	if err := input.Environment.Validate(); err != nil {
		return RefundRecord{}, err
	}
	if !validText(input.Merchant) || !validText(input.RemotePaymentID) ||
		!validText(input.OperationKey) || !validRefundReason(input.Reason) || !validActor(input.ActorID) ||
		input.Proof == "" {
	}
	if s.Proof == nil {
		return RefundRecord{}, domain.ErrUnavailable
	}
	if err := s.Proof.VerifyOperator(ctx, input.ActorID, input.Proof, OperatorActionRefundRequest); err != nil {
		return RefundRecord{}, domain.ErrUnauthorized
	}
	target, err := s.Store.RefundTarget(ctx, input.Provider, input.Environment, input.Merchant, input.RemotePaymentID)
	if err != nil {
		return RefundRecord{}, err
	}
	amount := target.Paid
	if reserved := target.Reserved; reserved.Centavos > 0 {
		amount, err = target.Paid.Sub(reserved)
		if err != nil {
			return RefundRecord{}, domain.ErrRefundBounds
		}
	}
	if input.Amount != nil {
		amount = *input.Amount
	}
	if err := amount.Positive(); err != nil {
		return RefundRecord{}, err
	}
	remaining, err := target.Paid.Sub(target.Reserved)
	if err != nil || amount.Centavos > remaining.Centavos {
		return RefundRecord{}, domain.ErrRefundBounds
	}
	approvedBy := ""
	if amount.Centavos > s.Config.SingleApprovalAboveCents {
		if input.ApprovedBy == "" || input.ApprovedBy == input.ActorID || input.ApprovalProof == "" {
			return RefundRecord{}, domain.ErrUnauthorized
		}
		if err := s.Proof.VerifyOperator(ctx, input.ApprovedBy, input.ApprovalProof, OperatorActionRefundRequest); err != nil {
			return RefundRecord{}, domain.ErrUnauthorized
		}
		approvedBy = input.ApprovedBy
	}
	record, replay, err := s.Store.ReserveRefund(ctx, RefundReservation{
		Provider: input.Provider, Environment: input.Environment, Merchant: input.Merchant,
		RemotePaymentID: input.RemotePaymentID, Amount: amount, Reason: input.Reason,
		RequestedBy: input.ActorID, ApprovedBy: approvedBy, OperationKey: input.OperationKey,
	})
	if err != nil {
		return RefundRecord{}, err
	}
	if replay {
		if !record.Amount.Equal(amount) {
			return RefundRecord{}, domain.ErrConflict
		}
		if refundTerminal(record.State) {
			return record, nil
		}
	}
	return s.settle(ctx, record, input.ActorID)
}

// settle attempts one provider write for a locally reserved refund and
// records the outcome. Unknown provider outcomes stay PENDING; only a
// definitively absent remote refund is ever retried.
func (s *RefundService) settle(ctx context.Context, record RefundRecord, actor string) (RefundRecord, error) {
	provider, err := s.providerFor(record.Provider)
	if err != nil {
		return record, err
	}
	request := RefundProviderRequest{
		RefundID: record.ID, RemotePaymentID: record.RemotePaymentID,
		RemoteChargeID: record.RemoteChargeID, Amount: record.Amount, Reason: record.Reason,
		IdempotencyKey: record.OperationKey,
	}
	result, err := provider.CreateRefund(ctx, request)
	if err != nil {
		if errors.Is(err, ErrAmbiguous) || errors.Is(err, domain.ErrUnavailable) {
			updated, uerr := s.Store.UpdateRefundState(ctx, record.ID, record.State, domain.RefundPending, "", err.Error(), s.now())
			if uerr != nil {
				return record, uerr
			}
			return updated, ErrAmbiguous
		}
		updated, uerr := s.Store.UpdateRefundState(ctx, record.ID, record.State, domain.RefundFailed, "", err.Error(), s.now())
		if uerr != nil {
			return record, uerr
		}
		_ = s.audit(ctx, actor, OperatorActionRefundRequest, updated, "failed")
		return updated, err
	}
	return s.applyProviderResult(ctx, actor, OperatorActionRefundRequest, record, result)
}

func (s *RefundService) applyProviderResult(ctx context.Context, actor, action string, record RefundRecord, result RefundProviderResult) (RefundRecord, error) {
	if !result.Amount.Equal(record.Amount) {
		return record, domain.ErrBinding
	}
	updated, err := s.Store.UpdateRefundState(ctx, record.ID, record.State, result.State, result.ID, "", s.now())
	if err != nil {
		return record, err
	}
	_ = s.audit(ctx, actor, action, updated, string(result.State))
	if result.State == domain.RefundPending {
		return updated, ErrAmbiguous
	}
	return updated, nil
}

// Reconcile retries exactly one durable refund intent: it checks the provider
// for the reserved refund before any new write, so a lost reply never creates
// a second remote refund.
func (s *RefundService) Reconcile(ctx context.Context, actor, proof, refundID string) (RefundRecord, error) {
	if s == nil || s.Store == nil || s.Proof == nil {
		return RefundRecord{}, domain.ErrUnavailable
	}
	if !validActor(actor) || proof == "" || !validID(refundID) {
		return RefundRecord{}, domain.ErrInvalid
	}
	if err := s.Proof.VerifyOperator(ctx, actor, proof, OperatorActionRefundReplay); err != nil {
		return RefundRecord{}, domain.ErrUnauthorized
	}
	record, err := s.Store.GetRefund(ctx, refundID)
	if err != nil {
		return RefundRecord{}, err
	}
	if refundTerminal(record.State) {
		return record, nil
	}
	provider, err := s.providerFor(record.Provider)
	if err != nil {
		return record, err
	}
	result, found, complete, err := provider.FindRefund(ctx, RefundProviderRequest{
		RefundID: record.ID, RemotePaymentID: record.RemotePaymentID,
		RemoteChargeID: record.RemoteChargeID, Amount: record.Amount, Reason: record.Reason,
		IdempotencyKey: record.OperationKey,
	})
	if err != nil {
		if errors.Is(err, ErrAmbiguous) || errors.Is(err, domain.ErrUnavailable) {
			if errors.Is(err, domain.ErrNotFound) {
				failed, uerr := s.Store.UpdateRefundState(ctx, record.ID, record.State, domain.RefundFailed, "", "original payment not found", s.now())
				if uerr != nil {
					return record, uerr
				}
				_ = s.audit(ctx, actor, OperatorActionRefundReplay, failed, "failed")
				return failed, err
			}
			return record, ErrAmbiguous
		}
		return record, err
	}
	if found {
		return s.applyProviderResult(ctx, actor, OperatorActionRefundReplay, record, result)
	}
	if !complete {
		return record, ErrAmbiguous
	}
	return s.settle(ctx, record, actor)
}

func (s *RefundService) audit(ctx context.Context, actor, action string, record RefundRecord, result string) error {
	return s.Store.RecordOperatorAction(ctx, OperatorAction{
		Actor: actor, Action: action, TargetID: record.ID, AccountID: record.AccountID,
		Reason: record.Reason, Amount: record.Amount, HasAmount: true,
		Result: result, CreatedAt: s.now(),
	})
}

func refundTerminal(state domain.RefundState) bool {
	return state == domain.RefundSucceeded || state == domain.RefundFailed || state == domain.RefundDenied
}

func validActor(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}

func validRefundReason(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}

func validID(value string) bool {
	return domain.ValidID(value)
}
