// Package adapters persists payment intents and fences remote attempts.
//
// No method performs provider I/O. Callers commit an attempt before dispatch and
// persist an outcome separately, so a crash leaves an explicit recoverable row.
package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	qpay "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/payments"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultLease = 30 * time.Second
	opTimeout    = 5 * time.Second
)

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

type CustomerMapping struct {
	AccountID        string
	Provider         domain.Provider
	Environment      domain.Environment
	Merchant         string
	RemoteCustomerID string
}

type ClaimAttemptInput struct {
	AccountID    string
	Provider     domain.Provider
	Environment  domain.Environment
	Merchant     string
	OperationKey string
	Kind         string
	PayloadHash  string
	OrderID      string
	LeaseUntil   time.Time
}

type Claim struct {
	OperationID string
	AttemptID   string
	Generation  int64
	AttemptNo   int32
	State       string
	RemoteID    string
	LeaseUntil  time.Time
	Replay      bool
}

type SettlementInput struct {
	Offer      domain.Offer
	Settlement domain.Settlement
}

type SettlementOutcome struct {
	SettlementID string
	OrderID      string
	GrantID      string
	Excess       bool
	Replay       bool
}

type InboxInput struct {
	Provider     domain.Provider
	Environment  domain.Environment
	Merchant     string
	EventID      string
	PayloadHash  string
	EventType    string
	ResourceType string
	ResourceID   string
}

type InboxResult struct {
	Created bool
	State   string
}

type CycleInput struct {
	ID           string
	AccountID    string
	OfferID      string
	Source       domain.Channel
	PeriodStart  time.Time
	PeriodEnd    time.Time
	State        string
	SettlementID string
}

func (s *Store) CreateOrder(ctx context.Context, order domain.Order) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	id, err := toUUID(order.ID)
	if err != nil || !validText(order.AccountID) || !validText(order.OfferID) ||
		!validText(order.CatalogVersion) || !validText(string(order.State)) {
		return domain.ErrInvalid
	}
	accountID, err := toUUID(order.AccountID)
	if err != nil {
		return domain.ErrInvalid
	}
	if err := order.Channel.Validate(); err != nil {
		return err
	}
	if err := order.Amount.Positive(); err != nil {
		return err
	}
	if len(order.Capabilities) == 0 {
		return domain.ErrInvalid
	}
	if order.QuotedAt.IsZero() || order.ExpiresAt.IsZero() || !order.ExpiresAt.After(order.QuotedAt) {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	q := qpay.New(s.Pool)
	rows, err := q.InsertPaymentOrder(ctx, qpay.InsertPaymentOrderParams{
		ID:             id,
		AccountID:      accountID,
		OfferID:        order.OfferID,
		CatalogVersion: order.CatalogVersion,
		Channel:        string(order.Channel),
		AmountCentavos: order.Amount.Centavos,
		Currency:       order.Amount.Currency,
		Capabilities:   order.Capabilities,
		State:          string(order.State),
		QuotedAt:       stamp(order.QuotedAt),
		ExpiresAt:      stamp(order.ExpiresAt),
	})
	if err != nil {
		return dbError(err)
	}
	if rows == 1 {
		return nil
	}
	existing, err := q.GetPaymentOrder(ctx, id)
	if err != nil {
		return dbError(err)
	}
	if existing.AccountID != accountID || existing.OfferID != order.OfferID ||
		existing.CatalogVersion != order.CatalogVersion || existing.Channel != string(order.Channel) ||
		existing.AmountCentavos != order.Amount.Centavos || existing.Currency != order.Amount.Currency ||
		!sameSet(existing.Capabilities, order.Capabilities) ||
		!sameInstant(existing.QuotedAt, order.QuotedAt) || !sameInstant(existing.ExpiresAt, order.ExpiresAt) {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) MapCustomer(ctx context.Context, mapping CustomerMapping) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	accountID, err := toUUID(mapping.AccountID)
	if err != nil || !validText(mapping.Merchant) || !validText(mapping.RemoteCustomerID) {
		return domain.ErrInvalid
	}
	if err := mapping.Provider.Validate(); err != nil {
		return err
	}
	if err := mapping.Environment.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	q := qpay.New(s.Pool)
	rows, err := q.InsertPaymentCustomer(ctx, qpay.InsertPaymentCustomerParams{
		AccountID:        accountID,
		Provider:         string(mapping.Provider),
		Environment:      string(mapping.Environment),
		Merchant:         mapping.Merchant,
		RemoteCustomerID: mapping.RemoteCustomerID,
	})
	if err != nil {
		return dbError(err)
	}
	if rows == 1 {
		return nil
	}
	existing, err := q.GetPaymentCustomer(ctx, qpay.GetPaymentCustomerParams{
		AccountID: accountID, Provider: string(mapping.Provider),
		Environment: string(mapping.Environment), Merchant: mapping.Merchant,
	})
	if err != nil {
		return dbError(err)
	}
	if existing.RemoteCustomerID != mapping.RemoteCustomerID {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) ClaimAttempt(ctx context.Context, input ClaimAttemptInput) (Claim, error) {
	if s == nil || s.Pool == nil {
		return Claim{}, domain.ErrUnavailable
	}
	accountID, err := toUUID(input.AccountID)
	if err != nil || !validText(input.Merchant) || !validText(input.OperationKey) ||
		!validText(input.Kind) || !digestOK(input.PayloadHash) {
		return Claim{}, domain.ErrInvalid
	}
	orderID, err := toUUID(input.OrderID)
	if err != nil {
		return Claim{}, domain.ErrInvalid
	}
	if err := input.Provider.Validate(); err != nil {
		return Claim{}, err
	}
	if err := input.Environment.Validate(); err != nil {
		return Claim{}, err
	}
	now := time.Now().UTC()
	lease := input.LeaseUntil.UTC()
	if lease.IsZero() {
		lease = now.Add(defaultLease)
	}
	if !lease.After(now) {
		return Claim{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Claim{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	if err := q.LockPaymentKey(ctx, "payment-op:"+input.AccountID+":"+string(input.Provider)+":"+
		string(input.Environment)+":"+input.Merchant+":"+input.OperationKey); err != nil {
		return Claim{}, dbError(err)
	}
	order, err := q.GetPaymentOrder(ctx, orderID)
	if err != nil {
		return Claim{}, dbError(err)
	}
	if order.AccountID != accountID || domain.Channel(order.Channel).Provider() != input.Provider {
		return Claim{}, domain.ErrBinding
	}
	state := domain.OrderState(order.State)
	if state != domain.OrderQuoted && state != domain.OrderPending && state != domain.OrderExpired {
		return Claim{}, fmt.Errorf("%w: order state", domain.ErrTransition)
	}
	op, err := q.GetPaymentOperationByKey(ctx, qpay.GetPaymentOperationByKeyParams{
		AccountID: accountID, Provider: string(input.Provider), Environment: string(input.Environment),
		Merchant: input.Merchant, OperationKey: input.OperationKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		operationID, e := newID()
		if e != nil {
			return Claim{}, domain.ErrUnavailable
		}
		attemptID, e := newID()
		if e != nil {
			return Claim{}, domain.ErrUnavailable
		}
		opUID, _ := toUUID(operationID)
		attemptUID, _ := toUUID(attemptID)
		if err := q.InsertPaymentOperation(ctx, qpay.InsertPaymentOperationParams{
			ID: opUID, AccountID: accountID, Provider: string(input.Provider),
			Environment: string(input.Environment), Merchant: input.Merchant,
			OperationKey: input.OperationKey, Kind: input.Kind, PayloadHash: input.PayloadHash,
			OrderID: orderID, LeaseUntil: stamp(lease),
		}); err != nil {
			return Claim{}, dbError(err)
		}
		if err := q.InsertPaymentAttempt(ctx, qpay.InsertPaymentAttemptParams{
			ID: attemptUID, OperationID: opUID, OrderID: orderID, AttemptNo: 1, Generation: 1,
			Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, PayloadHash: input.PayloadHash, State: "CREATED",
			LeaseUntil: stamp(lease),
		}); err != nil {
			return Claim{}, dbError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Claim{}, domain.ErrUnavailable
		}
		return Claim{
			OperationID: operationID, AttemptID: attemptID, Generation: 1, AttemptNo: 1,
			State: "PENDING", LeaseUntil: lease,
		}, nil
	}
	if err != nil {
		return Claim{}, dbError(err)
	}
	if op.PayloadHash != input.PayloadHash || op.Kind != input.Kind || !sameUUID(op.OrderID, orderID) {
		return Claim{}, domain.ErrConflict
	}
	if op.State == "SUCCEEDED" || op.State == "FAILED" || op.State == "CANCELED" || op.State == "QUARANTINED" {
		if err := tx.Commit(ctx); err != nil {
			return Claim{}, domain.ErrUnavailable
		}
		return Claim{
			OperationID: uuidText(op.ID), Generation: op.Generation, AttemptNo: op.AttemptNo,
			State: op.State, RemoteID: textValue(op.RemoteID), Replay: true,
		}, nil
	}
	if op.LeaseUntil.Valid && op.LeaseUntil.Time.After(now) {
		return Claim{}, domain.ErrBusy
	}
	if op.AttemptNo >= 5 {
		return Claim{}, domain.ErrBusy
	}
	nextGeneration := op.Generation + 1
	nextAttempt := op.AttemptNo + 1
	rows, err := q.TakeoverPaymentOperation(ctx, qpay.TakeoverPaymentOperationParams{
		Generation: nextGeneration, AttemptNo: nextAttempt, LeaseUntil: stamp(lease),
		ID: op.ID, PreviousGeneration: op.Generation,
	})
	if err != nil {
		return Claim{}, dbError(err)
	}
	if rows != 1 {
		return Claim{}, domain.ErrBusy
	}
	attemptID, err := newID()
	if err != nil {
		return Claim{}, domain.ErrUnavailable
	}
	attemptUID, _ := toUUID(attemptID)
	if err := q.InsertPaymentAttempt(ctx, qpay.InsertPaymentAttemptParams{
		ID: attemptUID, OperationID: op.ID, OrderID: orderID, AttemptNo: nextAttempt,
		Generation: nextGeneration, Provider: string(input.Provider),
		Environment: string(input.Environment), Merchant: input.Merchant,
		PayloadHash: input.PayloadHash, State: "CREATED", LeaseUntil: stamp(lease),
	}); err != nil {
		return Claim{}, dbError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Claim{}, domain.ErrUnavailable
	}
	return Claim{
		OperationID: uuidText(op.ID), AttemptID: attemptID, Generation: nextGeneration,
		AttemptNo: nextAttempt, State: "PENDING", LeaseUntil: lease,
	}, nil
}

func (s *Store) MarkDispatched(ctx context.Context, claim Claim) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	operationID, attemptID, err := claimIDs(claim)
	if err != nil || claim.Replay {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	rows, err := q.MarkPaymentAttemptDispatched(ctx, qpay.MarkPaymentAttemptDispatchedParams{
		ID: attemptID, Generation: claim.Generation,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	rows, err = q.MarkPaymentOperationDispatched(ctx, qpay.MarkPaymentOperationDispatchedParams{
		ID: operationID, Generation: claim.Generation,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) MarkOutcome(ctx context.Context, claim Claim, state domain.AttemptState, remoteID string) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	operationID, attemptID, err := claimIDs(claim)
	if err != nil || claim.Replay {
		return domain.ErrInvalid
	}
	attemptState, operationState, terminal, err := outcomeStates(state)
	if err != nil {
		return err
	}
	if state == domain.AttemptSucceeded && !validText(remoteID) {
		return domain.ErrInvalid
	}
	if remoteID != "" && !validText(remoteID) {
		return domain.ErrInvalid
	}
	now := time.Now().UTC()
	lease := now
	completed := stamp(now)
	if state == domain.AttemptUnknown {
		lease = claim.LeaseUntil.UTC()
		if lease.IsZero() {
			lease = now.Add(defaultLease)
		}
		completed = pgtype.Timestamptz{}
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	rows, err := q.ResolvePaymentAttempt(ctx, qpay.ResolvePaymentAttemptParams{
		State: attemptState, RemoteID: nullableText(remoteID), CompletedAt: completed,
		ID: attemptID, Generation: claim.Generation,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	if terminal {
		completed = stamp(now)
	} else {
		completed = pgtype.Timestamptz{}
	}
	rows, err = q.ResolvePaymentOperation(ctx, qpay.ResolvePaymentOperationParams{
		State: operationState, RemoteID: nullableText(remoteID), LeaseUntil: stamp(lease),
		CompletedAt: completed, ID: operationID, Generation: claim.Generation,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) RecordSettlement(ctx context.Context, input SettlementInput) (SettlementOutcome, error) {
	if s == nil || s.Pool == nil {
		return SettlementOutcome{}, domain.ErrUnavailable
	}
	if err := input.Offer.Validate(); err != nil {
		return SettlementOutcome{}, err
	}
	if !validText(input.Settlement.ID) || !validText(input.Settlement.AttemptID) ||
		!validText(input.Settlement.RemotePaymentID) || !validText(input.Settlement.ProviderStatus) ||
		!validText(input.Settlement.MerchantID) || input.Settlement.SettledAt.IsZero() {
		return SettlementOutcome{}, domain.ErrInvalid
	}
	if err := input.Settlement.Provider.Validate(); err != nil {
		return SettlementOutcome{}, err
	}
	if err := input.Settlement.Environment.Validate(); err != nil {
		return SettlementOutcome{}, err
	}
	if err := input.Settlement.Channel.Validate(); err != nil {
		return SettlementOutcome{}, err
	}
	now := time.Now().UTC()
	if input.Settlement.SettledAt.After(now) {
		return SettlementOutcome{}, domain.ErrInvalid
	}
	orderID, err := toUUID(input.Settlement.OrderID)
	if err != nil {
		return SettlementOutcome{}, domain.ErrInvalid
	}
	attemptID, err := toUUID(input.Settlement.AttemptID)
	if err != nil {
		return SettlementOutcome{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SettlementOutcome{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	orderRow, err := q.GetPaymentOrderForUpdate(ctx, orderID)
	if err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	if outcome, found, err := existingSettlement(ctx, q, orderRow, input.Settlement); err != nil {
		return SettlementOutcome{}, err
	} else if found {
		if err := enqueueSettlement(ctx, tx, outcome); err != nil {
			return SettlementOutcome{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SettlementOutcome{}, domain.ErrUnavailable
		}
		return outcome, nil
	}
	accountID := orderRow.AccountID
	if err := q.LockPaymentKey(ctx, "payment-entitlement:"+uuidText(accountID)+":"+orderRow.OfferID); err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	attemptRow, err := q.GetPaymentAttempt(ctx, attemptID)
	if err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	if attemptRow.OrderID != orderID {
		return SettlementOutcome{}, domain.ErrBinding
	}
	if attemptRow.Provider != string(input.Settlement.Provider) && input.Settlement.Provider != "" {
		return SettlementOutcome{}, domain.ErrBinding
	}
	opRow, err := q.GetPaymentOperationForUpdate(ctx, attemptRow.OperationID)
	if err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	if opRow.OrderID != orderID {
		return SettlementOutcome{}, domain.ErrBinding
	}
	if opRow.State == "FAILED" || opRow.State == "CANCELED" || opRow.State == "QUARANTINED" {
		return SettlementOutcome{}, domain.ErrConflict
	}
	grantRows, err := q.ListPaymentGrantsForEntitlement(ctx, qpay.ListPaymentGrantsForEntitlementParams{
		AccountID: accountID, OfferID: orderRow.OfferID,
	})
	if err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	entitlement := domain.Entitlement{
		AccountID: uuidText(accountID), OfferID: orderRow.OfferID,
		PolicyVersion: domain.PolicyVersion,
	}
	entitlement.Grants = make([]domain.Grant, 0, len(grantRows))
	for _, row := range grantRows {
		grant, err := domainGrant(row)
		if err != nil {
			return SettlementOutcome{}, err
		}
		entitlement.Grants = append(entitlement.Grants, grant)
	}
	order, err := domainOrder(orderRow)
	if err != nil {
		return SettlementOutcome{}, err
	}
	attempt, err := domainAttempt(attemptRow, opRow, input.Settlement.RemotePaymentID)
	if err != nil {
		return SettlementOutcome{}, err
	}
	settlement, err := canonicalSettlement(input.Settlement, orderRow, attemptRow)
	if err != nil {
		return SettlementOutcome{}, err
	}
	result, err := entitlement.ApplySettlement(input.Offer, &order, &attempt, settlement, now)
	if err != nil {
		return SettlementOutcome{}, err
	}
	if result.Grant == nil && !result.Excess {
		return SettlementOutcome{}, domain.ErrConflict
	}
	rows, err := q.InsertPaymentSettlement(ctx, qpay.InsertPaymentSettlementParams{
		ID: mustUUID(settlement.ID), OrderID: orderID, AttemptID: attemptID,
		AccountID: accountID, OfferID: orderRow.OfferID, Channel: string(settlement.Channel),
		Provider: string(settlement.Provider), Environment: string(settlement.Environment),
		Merchant: settlement.MerchantID, RemotePaymentID: settlement.RemotePaymentID,
		ProviderStatus: settlement.ProviderStatus, AmountCentavos: settlement.Amount.Centavos,
		Outcome: settlementOutcome(result.Excess), SettledAt: stamp(settlement.SettledAt),
	})
	if err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	if rows != 1 {
		return SettlementOutcome{}, domain.ErrConflict
	}
	outcome := SettlementOutcome{SettlementID: settlement.ID, OrderID: uuidText(orderID), Excess: result.Excess}
	if result.Grant != nil {
		grant := *result.Grant
		rows, err = q.InsertPaymentGrant(ctx, qpay.InsertPaymentGrantParams{
			ID: mustUUID(grant.ID), OrderID: orderID, SettlementID: mustUUID(settlement.ID),
			RemotePaymentID: grant.RemotePaymentID, AccountID: accountID, OfferID: orderRow.OfferID,
			Source: string(grant.Source), PaidCentavos: grant.Paid.Centavos,
			StartAt: stamp(grant.Start), EndAt: stamp(grant.End),
			AnchorDay: int16(grant.AnchorDay), PolicyVersion: int32(grant.PolicyVersion),
			Capabilities: grant.Capabilities,
		})
		if err != nil {
			return SettlementOutcome{}, dbError(err)
		}
		if rows != 1 {
			return SettlementOutcome{}, domain.ErrConflict
		}
		outcome.GrantID = grant.ID
	}
	if orderRow.State != "PAID" && orderRow.State != "CANCELED" {
		rows, err = q.MarkPaymentOrderPaid(ctx, qpay.MarkPaymentOrderPaidParams{
			PaidAt: stamp(now), SettlementID: mustUUID(settlement.ID), ID: orderID,
		})
		if err != nil {
			return SettlementOutcome{}, dbError(err)
		}
		if rows != 1 {
			return SettlementOutcome{}, domain.ErrConflict
		}
	}
	if err := resolveSettlement(ctx, q, attemptRow, opRow, settlement.RemotePaymentID, now, !result.Excess); err != nil {
		return SettlementOutcome{}, err
	}
	if err := enqueueSettlement(ctx, tx, outcome); err != nil {
		return SettlementOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SettlementOutcome{}, domain.ErrUnavailable
	}
	return outcome, nil
}

func (s *Store) RecordInbox(ctx context.Context, input InboxInput) (InboxResult, error) {
	if s == nil || s.Pool == nil {
		return InboxResult{}, domain.ErrUnavailable
	}
	if err := input.Provider.Validate(); err != nil {
		return InboxResult{}, err
	}
	if err := input.Environment.Validate(); err != nil {
		return InboxResult{}, err
	}
	if !validText(input.Merchant) || !validText(input.EventID) || !digestOK(input.PayloadHash) ||
		!validText(input.EventType) || !validText(input.ResourceType) || len(input.ResourceID) > 255 {
		return InboxResult{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return InboxResult{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	rows, err := q.InsertPaymentInbox(ctx, qpay.InsertPaymentInboxParams{
		Provider: string(input.Provider), Environment: string(input.Environment),
		Merchant: input.Merchant, EventID: input.EventID, PayloadHash: input.PayloadHash,
		EventType: input.EventType, ResourceType: input.ResourceType, ResourceID: input.ResourceID,
	})
	if err != nil {
		return InboxResult{}, dbError(err)
	}
	if rows == 0 {
		existing, err := q.GetPaymentInbox(ctx, qpay.GetPaymentInboxParams{
			Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, EventID: input.EventID,
		})
		if err != nil {
			return InboxResult{}, dbError(err)
		}
		if existing.PayloadHash != input.PayloadHash {
			return InboxResult{}, domain.ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return InboxResult{}, domain.ErrUnavailable
		}
		return InboxResult{State: existing.State}, nil
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "event_id": input.EventID})
	dedupe := "payment-inbox:" + string(input.Provider) + ":" + string(input.Environment) + ":" +
		input.Merchant + ":" + input.EventID
	if _, err := jobs.Enqueue(ctx, tx, "payment-inbox", payload, dedupe, 5, time.Time{}); err != nil {
		return InboxResult{}, domain.ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return InboxResult{}, domain.ErrUnavailable
	}
	return InboxResult{Created: true, State: "PENDING"}, nil
}

func (s *Store) MarkInbox(ctx context.Context, input InboxInput, state, lastError string) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if state != "PROCESSED" && state != "CONFLICT" {
		return domain.ErrInvalid
	}
	if len(lastError) > 500 {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).MarkPaymentInbox(ctx, qpay.MarkPaymentInboxParams{
		State: state, ProcessedAt: stamp(time.Now()), LastError: lastError,
		Provider: string(input.Provider), Environment: string(input.Environment),
		Merchant: input.Merchant, EventID: input.EventID,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	return nil
}

func (s *Store) CreateCycle(ctx context.Context, input CycleInput) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	id, err := toUUID(input.ID)
	if err != nil {
		return domain.ErrInvalid
	}
	accountID, err := toUUID(input.AccountID)
	if err != nil || !validText(input.OfferID) || !validText(input.State) {
		return domain.ErrInvalid
	}
	if err := input.Source.Validate(); err != nil {
		return err
	}
	if input.PeriodStart.IsZero() || !input.PeriodEnd.After(input.PeriodStart) {
		return domain.ErrInvalid
	}
	var settlementID pgtype.UUID
	if input.SettlementID != "" {
		settlementID, err = toUUID(input.SettlementID)
		if err != nil {
			return domain.ErrInvalid
		}
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	q := qpay.New(s.Pool)
	rows, err := q.InsertPaymentCycle(ctx, qpay.InsertPaymentCycleParams{
		ID: id, AccountID: accountID, OfferID: input.OfferID, Source: string(input.Source),
		PeriodStart: stamp(input.PeriodStart), PeriodEnd: stamp(input.PeriodEnd),
		State: input.State, SettlementID: settlementID,
	})
	if err != nil {
		return dbError(err)
	}
	if rows == 1 {
		return nil
	}
	existing, err := q.GetPaymentCycleByKey(ctx, qpay.GetPaymentCycleByKeyParams{
		AccountID: accountID, OfferID: input.OfferID, PeriodStart: stamp(input.PeriodStart),
	})
	if err != nil {
		return dbError(err)
	}
	if existing.Source != string(input.Source) || !sameInstant(existing.PeriodEnd, input.PeriodEnd) ||
		existing.State != input.State || !sameUUID(existing.SettlementID, settlementID) {
		return domain.ErrConflict
	}
	return nil
}

func existingSettlement(ctx context.Context, q *qpay.Queries, order qpay.PaymentOrder, in domain.Settlement) (SettlementOutcome, bool, error) {
	row, err := q.GetPaymentSettlementByID(ctx, mustUUID(in.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = q.GetPaymentSettlementByRemote(ctx, qpay.GetPaymentSettlementByRemoteParams{
			Provider: string(in.Provider), Environment: string(in.Environment),
			Merchant: in.MerchantID, RemotePaymentID: in.RemotePaymentID,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementOutcome{}, false, nil
	}
	if err != nil {
		return SettlementOutcome{}, false, dbError(err)
	}
	if uuidText(row.ID) != in.ID || row.OrderID != order.ID || row.AccountID != order.AccountID ||
		row.OfferID != order.OfferID || row.AmountCentavos != order.AmountCentavos ||
		row.AttemptID != mustUUID(in.AttemptID) || row.Provider != string(in.Provider) ||
		row.Environment != string(in.Environment) || row.Merchant != in.MerchantID ||
		row.Channel != string(in.Channel) || row.RemotePaymentID != in.RemotePaymentID {
		return SettlementOutcome{}, false, domain.ErrConflict
	}
	outcome := SettlementOutcome{SettlementID: uuidText(row.ID), OrderID: uuidText(row.OrderID), Replay: true}
	if row.Outcome != "GRANTED" {
		outcome.Excess = true
		return outcome, true, nil
	}
	grant, err := q.GetPaymentGrantByOrder(ctx, row.OrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementOutcome{}, false, domain.ErrConflict
	}
	if err != nil {
		return SettlementOutcome{}, false, dbError(err)
	}
	outcome.GrantID = uuidText(grant.ID)
	return outcome, true, nil
}

func settlementOutcome(excess bool) string {
	if excess {
		return "EXCESS"
	}
	return "GRANTED"
}

func resolveSettlement(ctx context.Context, q *qpay.Queries, attempt qpay.PaymentAttempt, operation qpay.PaymentOperation, remoteID string, now time.Time, strictRemote bool) error {
	switch attempt.State {
	case "SUCCEEDED":
		if strictRemote && attempt.RemoteID.Valid && attempt.RemoteID.String != remoteID {
			return domain.ErrConflict
		}
	case "DISPATCHED", "UNKNOWN":
		rows, err := q.ResolvePaymentAttempt(ctx, qpay.ResolvePaymentAttemptParams{
			State: "SUCCEEDED", RemoteID: nullableText(remoteID), CompletedAt: stamp(now),
			ID: attempt.ID, Generation: attempt.Generation,
		})
		if err != nil {
			return dbError(err)
		}
		if rows != 1 {
			return domain.ErrStale
		}
	default:
		return domain.ErrConflict
	}
	switch operation.State {
	case "SUCCEEDED":
		if strictRemote && operation.RemoteID.Valid && operation.RemoteID.String != remoteID {
			return domain.ErrConflict
		}
	case "PENDING", "DISPATCHED", "OUTCOME_UNKNOWN":
		rows, err := q.ResolvePaymentOperation(ctx, qpay.ResolvePaymentOperationParams{
			State: "SUCCEEDED", RemoteID: nullableText(remoteID), LeaseUntil: stamp(now),
			CompletedAt: stamp(now), ID: operation.ID, Generation: operation.Generation,
		})
		if err != nil {
			return dbError(err)
		}
		if rows != 1 {
			return domain.ErrStale
		}
	default:
		return domain.ErrConflict
	}
	return nil
}

func enqueueSettlement(ctx context.Context, tx pgx.Tx, outcome SettlementOutcome) error {
	payload, err := json.Marshal(map[string]any{
		"version": 1, "settlement_id": outcome.SettlementID, "order_id": outcome.OrderID,
		"grant_id": outcome.GrantID, "excess": outcome.Excess,
	})
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = jobs.Enqueue(ctx, tx, "payment-settlement", payload,
		"payment-settlement:"+outcome.SettlementID, 5, time.Time{})
	if err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func domainOrder(row qpay.PaymentOrder) (domain.Order, error) {
	if !row.QuotedAt.Valid || !row.ExpiresAt.Valid {
		return domain.Order{}, domain.ErrBinding
	}
	order := domain.Order{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		CatalogVersion: row.CatalogVersion, Channel: domain.Channel(row.Channel),
		Term: domain.TermMonthly, Amount: domain.BRL(row.AmountCentavos),
		Capabilities: append([]string(nil), row.Capabilities...),
		State:        domain.OrderState(row.State), QuotedAt: row.QuotedAt.Time, ExpiresAt: row.ExpiresAt.Time,
	}
	if row.PaidAt.Valid {
		order.PaidAt = row.PaidAt.Time
	}
	if row.SettlementID.Valid {
		order.SettlementID = uuidText(row.SettlementID)
	}
	if !(domain.Money{Centavos: row.AmountCentavos, Currency: row.Currency}).Equal(order.Amount) {
		return domain.Order{}, domain.ErrBinding
	}
	return order, nil
}

func domainAttempt(row qpay.PaymentAttempt, operation qpay.PaymentOperation, remoteID string) (domain.Attempt, error) {
	if !row.CreatedAt.Valid {
		return domain.Attempt{}, domain.ErrBinding
	}
	return domain.Attempt{
		ID: uuidText(row.ID), OrderID: uuidText(row.OrderID),
		Provider: domain.Provider(row.Provider), Operation: operation.Kind,
		State: domain.AttemptSucceeded, RemoteID: remoteID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		DispatchedAt: row.CreatedAt.Time,
	}, nil
}

func domainGrant(row qpay.PaymentGrant) (domain.Grant, error) {
	if !row.StartAt.Valid || !row.EndAt.Valid {
		return domain.Grant{}, domain.ErrBinding
	}
	grant := domain.Grant{
		ID: uuidText(row.ID), OrderID: uuidText(row.OrderID), SettlementID: uuidText(row.SettlementID),
		RemotePaymentID: row.RemotePaymentID, Source: domain.Channel(row.Source),
		Paid: domain.BRL(row.PaidCentavos), Start: row.StartAt.Time, End: row.EndAt.Time,
		AnchorDay: int(row.AnchorDay), Capabilities: append([]string(nil), row.Capabilities...),
		State: domain.GrantState(row.State), PolicyVersion: int(row.PolicyVersion),
	}
	if !(domain.Money{Centavos: row.PaidCentavos, Currency: row.Currency}).Equal(grant.Paid) {
		return domain.Grant{}, domain.ErrBinding
	}
	return grant, nil
}

func canonicalSettlement(in domain.Settlement, order qpay.PaymentOrder, attempt qpay.PaymentAttempt) (domain.Settlement, error) {
	settlement := in
	if settlement.ID == "" || settlement.AttemptID == "" || settlement.RemotePaymentID == "" ||
		settlement.ProviderStatus == "" || settlement.SettledAt.IsZero() {
		return domain.Settlement{}, domain.ErrInvalid
	}
	if settlement.OrderID != "" && settlement.OrderID != uuidText(order.ID) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.AttemptID != uuidText(attempt.ID) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.AccountID != "" && settlement.AccountID != uuidText(order.AccountID) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.OfferID != "" && settlement.OfferID != order.OfferID {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.Channel != "" && settlement.Channel != domain.Channel(order.Channel) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.Provider != "" && settlement.Provider != domain.Provider(attempt.Provider) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.Environment != "" && settlement.Environment != domain.Environment(attempt.Environment) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.MerchantID != "" && settlement.MerchantID != attempt.Merchant {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.Amount.Centavos != 0 && !settlement.Amount.Equal(domain.BRL(order.AmountCentavos)) {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.Status != "" && settlement.Status != domain.SettlementSettled {
		return domain.Settlement{}, domain.ErrBinding
	}
	if settlement.Authority != "" && settlement.Authority != domain.AuthorityProviderVerified {
		return domain.Settlement{}, domain.ErrBinding
	}
	settlement.OrderID = uuidText(order.ID)
	settlement.AccountID = uuidText(order.AccountID)
	settlement.OfferID = order.OfferID
	settlement.Channel = domain.Channel(order.Channel)
	settlement.Provider = domain.Provider(attempt.Provider)
	settlement.Environment = domain.Environment(attempt.Environment)
	settlement.MerchantID = attempt.Merchant
	settlement.Amount = domain.BRL(order.AmountCentavos)
	settlement.Status = domain.SettlementSettled
	settlement.Authority = domain.AuthorityProviderVerified
	return settlement, nil
}

func outcomeStates(state domain.AttemptState) (attempt, operation string, terminal bool, err error) {
	switch state {
	case domain.AttemptSucceeded:
		return "SUCCEEDED", "SUCCEEDED", true, nil
	case domain.AttemptFailed:
		return "FAILED", "FAILED", true, nil
	case domain.AttemptUnknown:
		return "UNKNOWN", "OUTCOME_UNKNOWN", false, nil
	case domain.AttemptQuarantined:
		return "QUARANTINED", "QUARANTINED", true, nil
	case domain.AttemptCanceled:
		return "CANCELED", "CANCELED", true, nil
	default:
		return "", "", false, domain.ErrInvalid
	}
}

func claimIDs(claim Claim) (pgtype.UUID, pgtype.UUID, error) {
	operationID, err := toUUID(claim.OperationID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	attemptID, err := toUUID(claim.AttemptID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	if claim.Generation < 1 || claim.AttemptNo < 1 {
		return pgtype.UUID{}, pgtype.UUID{}, domain.ErrInvalid
	}
	return operationID, attemptID, nil
}

func toUUID(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(text); err != nil || !id.Valid {
		return pgtype.UUID{}, domain.ErrInvalid
	}
	return id, nil
}

func mustUUID(text string) pgtype.UUID {
	id, _ := toUUID(text)
	return id
}

func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	raw := make([]byte, 16)
	copy(raw, id.Bytes[:])
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func sameUUID(a, b pgtype.UUID) bool {
	return uuidText(a) == uuidText(b)
}

func stamp(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func sameInstant(a pgtype.Timestamptz, b time.Time) bool {
	if !a.Valid || b.IsZero() {
		return !a.Valid && b.IsZero()
	}
	return a.Time.UTC().Truncate(time.Microsecond).Equal(b.UTC().Truncate(time.Microsecond))
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
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

func sameSet(left, right []string) bool {
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

func digestOK(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func newID() (string, error) { return jobs.NewUUIDv4() }

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrBinding
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrConflict
		case "23503", "23514", "22P02":
			return domain.ErrInvalid
		}
	}
	return domain.ErrUnavailable
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
