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
	pixAutoAuthCreated   = "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_CREATED"
	pixAutoAuthActivated = "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_ACTIVATED"
	pixAutoAuthRefused   = "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_REFUSED"
	pixAutoAuthCancelled = "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_CANCELLED"
	pixAutoAuthExpired   = "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_EXPIRED"

	pixAutoInstructionCreated   = "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_CREATED"
	pixAutoInstructionScheduled = "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_SCHEDULED"
	pixAutoInstructionRefused   = "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_REFUSED"
	pixAutoInstructionCancelled = "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_CANCELLED"

	pixAutoEligibilityUpdated = "PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED"

	pixAutoCancelOwner        = "OWNER_REQUEST"
	pixAutoCancelIneligible   = "MERCHANT_INELIGIBLE"
	pixAutoInstructionLimit   = 100
	pixAutoMandateBatch       = 50
	pixAutoMandateBatchRounds = 5
)

// PixAutoCycleService consumes provider-owned Pix Automático lifecycle facts.
// It never creates a charge or a recurring instruction; Asaas SUBSCRIPTION is
// the only cycle owner.
type PixAutoCycleService struct {
	Store       PixAutoStore
	Provider    PixAutoProvider
	Settlements *SettlementService
	Config      PixAutoConfig
	Clock       func() time.Time
}

func (s *PixAutoCycleService) Process(ctx context.Context, event InboxRecord) error {
	if s == nil || s.Store == nil || s.Provider == nil {
		return domain.ErrUnavailable
	}
	if event.Provider != domain.ProviderAsaas || event.Environment != s.Config.Environment ||
		event.Merchant != s.Config.MerchantID {
		return domain.ErrBinding
	}
	if event.State != "" && event.State != "PENDING" {
		return nil
	}
	var err error
	switch event.EventType {
	case pixAutoAuthCreated, pixAutoAuthActivated, pixAutoAuthRefused, pixAutoAuthCancelled, pixAutoAuthExpired:
		err = s.processAuthorization(ctx, event)
	case pixAutoInstructionCreated, pixAutoInstructionScheduled, pixAutoInstructionRefused, pixAutoInstructionCancelled:
		err = s.processInstruction(ctx, event)
	case pixAutoEligibilityUpdated:
		err = s.processEligibility(ctx, event)
	case "PAYMENT_RECEIVED", "PAYMENT_CONFIRMED", "PAYMENT_CREATED", "PAYMENT_OVERDUE":
		return ErrIgnored
	default:
		if !strings.HasPrefix(event.EventType, "PIX_AUTOMATIC_RECURRING_") {
			return ErrIgnored
		}
		return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
	}
	if err != nil {
		return s.fail(ctx, event, err)
	}
	return s.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
}

func (s *PixAutoCycleService) processAuthorization(ctx context.Context, event InboxRecord) error {
	mandate, err := s.Store.MandateByRemote(ctx, s.Config.Environment, s.Config.MerchantID, event.ResourceID)
	if err != nil {
		return err
	}
	authorization, err := s.Provider.RetrievePixAutoAuthorization(ctx, mandate.RemoteAuthorizationID)
	if err != nil {
		return err
	}
	if err := validatePixAutoBinding(authorization, mandate); err != nil {
		return err
	}
	mandate.Status = authorization.Status
	if authorization.RemoteSubscriptionID != "" {
		mandate.RemoteSubscriptionID = authorization.RemoteSubscriptionID
	}
	mandate.LastEventAt = eventTime(event, s.now())
	if err := s.Store.UpsertMandate(ctx, mandate); err != nil {
		return err
	}
	if pixAutoTerminal(authorization.Status) {
		_, err = s.reconcileInstructions(ctx, mandate)
		return err
	}
	if authorization.Status == "ACTIVE" {
		ineligible, err := s.ineligible(ctx)
		if err != nil {
			return err
		}
		if ineligible {
			return cancelPixAutoMandate(ctx, s.Store, s.Provider, mandate, pixAutoCancelIneligible, s.now())
		}
	}
	return nil
}

func (s *PixAutoCycleService) processInstruction(ctx context.Context, event InboxRecord) error {
	instruction, err := s.Provider.RetrievePixAutoInstruction(ctx, event.ResourceID)
	if err != nil {
		return err
	}
	mandate, err := s.Store.MandateByRemote(ctx, s.Config.Environment, s.Config.MerchantID, instruction.AuthorizationID)
	if err != nil {
		return err
	}
	if err := validatePixAutoInstruction(instruction, mandate); err != nil {
		return err
	}
	return s.Store.UpsertInstruction(ctx, instructionRecord(instruction, mandate, eventTime(event, s.now())))
}

func (s *PixAutoCycleService) processEligibility(ctx context.Context, event InboxRecord) error {
	status := event.ResourceStatus
	if status != "ELIGIBLE" && status != "INELIGIBLE" {
		return domain.ErrInvalid
	}
	now := s.now()
	if err := s.Store.UpsertEligibility(ctx, PixAutoEligibility{
		Environment: s.Config.Environment, Merchant: s.Config.MerchantID,
		Status: status, LastEventAt: eventTime(event, now), ObservedAt: now,
	}); err != nil {
		return err
	}
	if status != "INELIGIBLE" {
		return nil
	}
	return s.cancelActiveMandates(ctx, now)
}

func (s *PixAutoCycleService) cancelActiveMandates(ctx context.Context, now time.Time) error {
	for round := 0; round < pixAutoMandateBatchRounds; round++ {
		mandates, err := s.Store.ActiveMandates(ctx, s.Config.Environment, s.Config.MerchantID, pixAutoMandateBatch)
		if err != nil {
			return err
		}
		if len(mandates) == 0 {
			return nil
		}
		for _, mandate := range mandates {
			if err := cancelPixAutoMandate(ctx, s.Store, s.Provider, mandate, pixAutoCancelIneligible, now); err != nil {
				return err
			}
		}
		if len(mandates) < pixAutoMandateBatch {
			return nil
		}
	}
	return domain.ErrBusy
}

// Reconcile repairs missed Pix Automático callbacks for one mandate. It reads
// canonical provider state, records instructions, retries an explicit local
// cancellation and settles every received subscription payment idempotently.
func (s *PixAutoCycleService) Reconcile(ctx context.Context, mandate MandateRecord) (int, int, error) {
	if s == nil || s.Store == nil || s.Provider == nil || s.Settlements == nil {
		return 0, 0, domain.ErrUnavailable
	}
	authorization, err := s.Provider.RetrievePixAutoAuthorization(ctx, mandate.RemoteAuthorizationID)
	if err != nil {
		return 0, 0, err
	}
	if err := validatePixAutoBinding(authorization, mandate); err != nil {
		return 0, 0, err
	}
	mandate.Status = authorization.Status
	if authorization.RemoteSubscriptionID != "" {
		mandate.RemoteSubscriptionID = authorization.RemoteSubscriptionID
	}
	now := s.now()
	mandate.LastEventAt = now
	if err := s.Store.UpsertMandate(ctx, mandate); err != nil {
		return 0, 0, err
	}
	instructions, err := s.reconcileInstructions(ctx, mandate)
	if err != nil {
		return instructions, 0, err
	}
	var cancelErr error
	if mandate.CancelRequestedAt != nil && !pixAutoTerminal(authorization.Status) {
		cancelErr = cancelPixAutoMandate(ctx, s.Store, s.Provider, mandate, mandate.CancelReason, now)
	}
	if cancelErr == nil && mandate.Status == "ACTIVE" {
		ineligible, eligibilityErr := s.ineligible(ctx)
		if eligibilityErr != nil {
			cancelErr = eligibilityErr
		} else if ineligible {
			cancelErr = cancelPixAutoMandate(ctx, s.Store, s.Provider, mandate, pixAutoCancelIneligible, now)
		}
	}
	payments, paymentErr := s.reconcilePayments(ctx, mandate, now)
	return instructions, payments, errors.Join(cancelErr, paymentErr)
}

func (s *PixAutoCycleService) ineligible(ctx context.Context) (bool, error) {
	eligibility, found, err := s.Store.Eligibility(ctx, s.Config.Environment, s.Config.MerchantID)
	if err != nil {
		return false, err
	}
	return found && eligibility.Status == "INELIGIBLE", nil
}

func (s *PixAutoCycleService) reconcileInstructions(ctx context.Context, mandate MandateRecord) (int, error) {
	instructions, complete, err := s.Provider.ListPixAutoInstructions(ctx, mandate.RemoteAuthorizationID)
	if err != nil {
		return 0, err
	}
	if !complete || len(instructions) > pixAutoInstructionLimit {
		return 0, ErrAmbiguous
	}
	processed := 0
	for _, instruction := range instructions {
		if err := validatePixAutoInstruction(instruction, mandate); err != nil {
			return processed, err
		}
		if err := s.Store.UpsertInstruction(ctx, instructionRecord(instruction, mandate, s.now())); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *PixAutoCycleService) reconcilePayments(ctx context.Context, mandate MandateRecord, now time.Time) (int, error) {
	if mandate.RemoteSubscriptionID == "" {
		return 0, nil
	}
	payments, complete, err := s.Provider.ListSubscriptionPayments(ctx, mandate.RemoteSubscriptionID)
	if err != nil {
		return 0, err
	}
	if !complete || len(payments) > pixAutoInstructionLimit {
		return 0, ErrAmbiguous
	}
	settled := 0
	for _, payment := range payments {
		if payment.Class != PixSettled || payment.ProviderStatus != "RECEIVED" {
			continue
		}
		if payment.CustomerID != mandate.RemoteCustomerID || payment.SubscriptionID != mandate.RemoteSubscriptionID ||
			!payment.Amount.Equal(mandate.Value) {
			return settled, domain.ErrBinding
		}
		settledAt := payment.SettledAt
		if settledAt.IsZero() {
			settledAt = now
		}
		if err := s.Settlements.Settle(ctx, SettlementCandidate{
			Provider: domain.ProviderAsaas, Environment: s.Config.Environment,
			Merchant: s.Config.MerchantID, Channel: domain.ChannelPixAutomatic,
			CustomerID: payment.CustomerID, RemotePaymentID: payment.ID,
			RemoteSubscriptionID: payment.SubscriptionID, ProviderStatus: payment.ProviderStatus,
			Amount: payment.Amount, SettledAt: settledAt,
		}); err != nil {
			return settled, err
		}
		settled++
	}
	return settled, nil
}

func (s *PixAutoCycleService) fail(ctx context.Context, event InboxRecord, cause error) error {
	if errors.Is(cause, domain.ErrUnavailable) || errors.Is(cause, ErrAmbiguous) ||
		errors.Is(cause, domain.ErrBusy) {
		return cause
	}
	if errors.Is(cause, domain.ErrNotFound) {
		// A provider event can arrive before the local create transaction
		// commits. Leave it pending instead of recording a permanent conflict.
		return errors.Join(domain.ErrUnavailable, cause)
	}
	if err := s.Store.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "pix-auto-binding"); err != nil {
		return errors.Join(cause, err)
	}
	return errors.Join(domain.ErrConflict, cause)
}

func (s *PixAutoCycleService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func cancelPixAutoMandate(ctx context.Context, store PixAutoStore, provider PixAutoProvider, mandate MandateRecord, reason string, now time.Time) error {
	if pixAutoTerminal(mandate.Status) {
		return nil
	}
	if reason == "" {
		reason = pixAutoCancelOwner
	}
	requested, err := store.RequestMandateCancel(ctx, mandate.Environment, mandate.Merchant, mandate.RemoteAuthorizationID, now, reason)
	if err != nil {
		return err
	}
	authorization, err := provider.RetrievePixAutoAuthorization(ctx, requested.RemoteAuthorizationID)
	if err != nil {
		if errors.Is(err, domain.ErrUnavailable) {
			return err
		}
		return ErrAmbiguous
	}
	if err := validatePixAutoBinding(authorization, requested); err != nil {
		return err
	}
	if pixAutoTerminal(authorization.Status) {
		return persistPixAutoStatus(ctx, store, requested, authorization, now)
	}
	if err := provider.CancelPixAutoAuthorization(ctx, requested.RemoteAuthorizationID); err != nil {
		if errors.Is(err, domain.ErrUnavailable) {
			return err
		}
		return ErrAmbiguous
	}
	authorization, err = provider.RetrievePixAutoAuthorization(ctx, requested.RemoteAuthorizationID)
	if err != nil {
		if errors.Is(err, domain.ErrUnavailable) {
			return err
		}
		return ErrAmbiguous
	}
	if err := validatePixAutoBinding(authorization, requested); err != nil {
		return err
	}
	if !pixAutoTerminal(authorization.Status) {
		return ErrAmbiguous
	}
	return persistPixAutoStatus(ctx, store, requested, authorization, now)
}

func persistPixAutoStatus(ctx context.Context, store PixAutoStore, mandate MandateRecord, authorization PixAutoAuthorization, now time.Time) error {
	mandate.Status = authorization.Status
	if authorization.RemoteSubscriptionID != "" {
		mandate.RemoteSubscriptionID = authorization.RemoteSubscriptionID
	}
	mandate.LastEventAt = now
	return store.UpsertMandate(ctx, mandate)
}

func validatePixAutoBinding(authorization PixAutoAuthorization, mandate MandateRecord) error {
	if authorization.ID != mandate.RemoteAuthorizationID || authorization.CustomerID != mandate.RemoteCustomerID ||
		authorization.ContractID != mandate.ContractID || !authorization.Value.Equal(mandate.Value) ||
		authorization.PaymentCreationMode != "SUBSCRIPTION" || authorization.RetryPolicy != "NOT_ALLOWED" {
		return domain.ErrBinding
	}
	switch authorization.Status {
	case "CREATED", "ACTIVE", "REFUSED", "CANCELLED", "EXPIRED":
	default:
		return domain.ErrBinding
	}
	if mandate.RemoteSubscriptionID != "" && authorization.RemoteSubscriptionID != "" &&
		authorization.RemoteSubscriptionID != mandate.RemoteSubscriptionID {
		return domain.ErrBinding
	}
	return nil
}

func validatePixAutoInstruction(instruction PixAutoInstruction, mandate MandateRecord) error {
	if instruction.ID == "" || instruction.AuthorizationID != mandate.RemoteAuthorizationID ||
		instruction.DueDate.IsZero() || instruction.RetryAttempt < 0 || instruction.RetryAttempt > 3 ||
		len(instruction.RefusalReason) > 128 {
		return domain.ErrBinding
	}
	switch instruction.Status {
	case "AWAITING_REQUEST", "SCHEDULED", "DONE", "REFUSED", "CANCELLED":
	default:
		return domain.ErrBinding
	}
	if instruction.Purpose != "SCHEDULE" && instruction.Purpose != "RETRY_AFTER_DUE_DATE" {
		return domain.ErrBinding
	}
	return nil
}

func instructionRecord(instruction PixAutoInstruction, mandate MandateRecord, at time.Time) InstructionRecord {
	return InstructionRecord{
		ID: derivedInstructionID(instruction.ID), AccountID: mandate.AccountID, OfferID: mandate.OfferID,
		Environment: mandate.Environment, Merchant: mandate.Merchant,
		RemoteInstructionID: instruction.ID, RemoteAuthorizationID: instruction.AuthorizationID,
		RemoteSubscriptionID: mandate.RemoteSubscriptionID, RemotePaymentID: instruction.PaymentID,
		Status: instruction.Status, Purpose: instruction.Purpose, RetryAttempt: instruction.RetryAttempt,
		DueDate: instruction.DueDate, RefusalReason: instruction.RefusalReason, LastEventAt: at,
	}
}

func pixAutoTerminal(status string) bool {
	return status == "CANCELLED" || status == "EXPIRED" || status == "REFUSED"
}

func eventTime(event InboxRecord, fallback time.Time) time.Time {
	if !event.ReceivedAt.IsZero() {
		return event.ReceivedAt.UTC()
	}
	return fallback
}

func derivedInstructionID(remoteID string) string {
	return deterministicPaymentUUID("payment-pix-auto-instruction", remoteID)
}

func deterministicPaymentUUID(namespace, value string) string {
	sum := sha256.Sum256([]byte(namespace + "\x00" + value))
	raw := sum[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}
