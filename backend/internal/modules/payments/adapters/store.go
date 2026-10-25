// Package adapters persists payment intents and fences remote attempts.
//
// No method performs provider I/O. Callers commit an attempt before dispatch and
// persist an outcome separately, so a crash leaves an explicit recoverable row.
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	qpay "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/payments"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
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

func (s *Store) GetOrder(ctx context.Context, id string) (domain.Order, error) {
	if s == nil || s.Pool == nil {
		return domain.Order{}, domain.ErrUnavailable
	}
	orderID, err := toUUID(id)
	if err != nil {
		return domain.Order{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentOrder(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Order{}, dbError(err)
	}
	return domainOrder(row)
}

func (s *Store) Customer(ctx context.Context, accountID string, provider domain.Provider, environment domain.Environment, merchant string) (string, bool, error) {
	if s == nil || s.Pool == nil {
		return "", false, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(merchant) {
		return "", false, domain.ErrInvalid
	}
	if err := provider.Validate(); err != nil {
		return "", false, err
	}
	if err := environment.Validate(); err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentCustomer(ctx, qpay.GetPaymentCustomerParams{
		AccountID: uid, Provider: string(provider), Environment: string(environment), Merchant: merchant,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, dbError(err)
	}
	return row.RemoteCustomerID, true, nil
}

func (s *Store) Inbox(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant, eventID string) (app.InboxRecord, error) {
	if s == nil || s.Pool == nil {
		return app.InboxRecord{}, domain.ErrUnavailable
	}
	if err := provider.Validate(); err != nil {
		return app.InboxRecord{}, err
	}
	if err := environment.Validate(); err != nil {
		return app.InboxRecord{}, err
	}
	if !validText(merchant) || !validText(eventID) {
		return app.InboxRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentInbox(ctx, qpay.GetPaymentInboxParams{
		Provider: string(provider), Environment: string(environment), Merchant: merchant, EventID: eventID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.InboxRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.InboxRecord{}, dbError(err)
	}
	return inboxRecord(row), nil
}

func (s *Store) PendingInbox(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant string, limit int) ([]app.InboxRecord, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	if err := provider.Validate(); err != nil {
		return nil, err
	}
	if err := environment.Validate(); err != nil {
		return nil, err
	}
	if !validText(merchant) || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ListPendingPaymentInbox(ctx, qpay.ListPendingPaymentInboxParams{
		Provider: string(provider), Environment: string(environment), Merchant: merchant, LimitCount: int32(limit),
	})
	if err != nil {
		return nil, dbError(err)
	}
	records := make([]app.InboxRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, inboxRecord(row))
	}
	return records, nil
}

func (s *Store) AttemptForOrder(ctx context.Context, orderID string) (app.AttemptRecord, error) {
	if s == nil || s.Pool == nil {
		return app.AttemptRecord{}, domain.ErrUnavailable
	}
	uid, err := toUUID(orderID)
	if err != nil {
		return app.AttemptRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentAttemptForOrder(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.AttemptRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.AttemptRecord{}, dbError(err)
	}
	return app.AttemptRecord{
		ID: uuidText(row.ID), OperationID: uuidText(row.OperationID),
		Provider: domain.Provider(row.Provider), Environment: domain.Environment(row.Environment),
		Merchant: row.Merchant, State: row.State, RemoteID: textValue(row.RemoteID),
		Generation: row.Generation,
	}, nil
}

func (s *Store) RevokeGrant(ctx context.Context, orderID string, now time.Time) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	uid, err := toUUID(orderID)
	if err != nil || now.IsZero() {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	q := qpay.New(s.Pool)
	rows, err := q.RevokePaymentGrant(ctx, qpay.RevokePaymentGrantParams{RevokedAt: stamp(now), OrderID: uid})
	if err != nil {
		return dbError(err)
	}
	if rows == 1 {
		return nil
	}
	grant, err := q.GetPaymentGrantByOrder(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return dbError(err)
	}
	if grant.State == "REVOKED" {
		return nil
	}
	return domain.ErrConflict
}

func inboxRecord(row qpay.PaymentInbox) app.InboxRecord {
	return app.InboxRecord{
		InboxEvent: app.InboxEvent{
			Provider: domain.Provider(row.Provider), Environment: domain.Environment(row.Environment),
			Merchant: row.Merchant, EventID: row.EventID, PayloadHash: row.PayloadHash,
			EventType: row.EventType, ResourceType: row.ResourceType, ResourceID: row.ResourceID,
			ResourceStatus: row.ResourceStatus,
		},
		State: row.State, ReceivedAt: row.ReceivedAt.Time,
	}
}

type CustomerMapping = app.CustomerMapping
type ClaimAttemptInput = app.ClaimAttemptInput
type Claim = app.Claim

type SettlementInput = app.SettlementInput
type SettlementOutcome = app.SettlementOutcome
type ContractRecord = app.ContractRecord

type InboxInput = app.InboxEvent
type InboxResult = app.InboxResult

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
	if len(order.Capabilities) == 0 && order.Family != domain.EnterpriseFamily {
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
		Family:         order.Family,
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
		return SettlementOutcome{}, fmt.Errorf("%w: no grant/excess", domain.ErrConflict)
	}
	rows, err := q.InsertPaymentSettlement(ctx, qpay.InsertPaymentSettlementParams{
		ID: mustUUID(settlement.ID), OrderID: orderID, AttemptID: attemptID,
		AccountID: accountID, OfferID: orderRow.OfferID, Channel: string(settlement.Channel),
		Provider: string(settlement.Provider), Environment: string(settlement.Environment),
		Merchant: settlement.MerchantID, RemotePaymentID: settlement.RemotePaymentID,
		RemoteChargeID: nullableText(settlement.RemoteChargeID),
		ProviderStatus: settlement.ProviderStatus, AmountCentavos: settlement.Amount.Centavos,
		Outcome: settlementOutcome(result.Excess), SettledAt: stamp(settlement.SettledAt),
	})
	if err != nil {
		return SettlementOutcome{}, dbError(err)
	}
	if rows != 1 {
		return SettlementOutcome{}, fmt.Errorf("%w: settlement insert", domain.ErrConflict)
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
			return SettlementOutcome{}, fmt.Errorf("%w: grant insert", domain.ErrConflict)
		}
		rows, err = q.InsertPaymentCycle(ctx, qpay.InsertPaymentCycleParams{
			ID: mustUUID(grant.ID), AccountID: accountID, OfferID: orderRow.OfferID,
			Source: string(grant.Source), PeriodStart: stamp(grant.Start), PeriodEnd: stamp(grant.End),
			State: "SETTLED", SettlementID: mustUUID(settlement.ID),
		})
		if err != nil {
			return SettlementOutcome{}, dbError(err)
		}
		if rows != 1 {
			existing, getErr := q.GetPaymentCycleByKey(ctx, qpay.GetPaymentCycleByKeyParams{
				AccountID: accountID, OfferID: orderRow.OfferID, PeriodStart: stamp(grant.Start),
			})
			if getErr != nil || existing.State != "SETTLED" ||
				!sameUUID(existing.SettlementID, mustUUID(settlement.ID)) ||
				!sameInstant(existing.PeriodEnd, grant.End) {
				return SettlementOutcome{}, fmt.Errorf("%w: cycle conflict", domain.ErrConflict)
			}
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
			return SettlementOutcome{}, fmt.Errorf("%w: order paid update", domain.ErrConflict)
		}
	}
	if err := resolveSettlement(ctx, q, attemptRow, opRow, settlement.RemotePaymentID, now); err != nil {
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
		!validText(input.EventType) || !validText(input.ResourceType) || len(input.ResourceID) > 255 ||
		len(input.ResourceStatus) > 64 || (input.ResourceStatus != "" && !validText(input.ResourceStatus)) {
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
		ResourceStatus: input.ResourceStatus,
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

func resolveSettlement(ctx context.Context, q *qpay.Queries, attempt qpay.PaymentAttempt, operation qpay.PaymentOperation, remoteID string, now time.Time) error {
	switch attempt.State {
	case "SUCCEEDED":
		// The attempt may be a checkout/session/authorization while settlement is
		// an invoice/payment; both are bound by order and unique identities.
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

func (s *Store) UpsertContract(ctx context.Context, contract app.ContractRecord) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if !validText(contract.ID) || !validText(contract.AccountID) || !validText(contract.OfferID) ||
		!validText(contract.Merchant) || !validText(contract.RemoteSubscriptionID) ||
		!validText(contract.RemoteCustomerID) || !validText(contract.ProviderStatus) ||
		contract.LastEventAt.IsZero() {
		return domain.ErrInvalid
	}
	if contract.Source != domain.ChannelCard || contract.Provider != domain.ProviderStripe {
		return domain.ErrInvalid
	}
	if err := contract.Environment.Validate(); err != nil {
		return err
	}
	uid, err := toUUID(contract.ID)
	if err != nil {
		return domain.ErrInvalid
	}
	accountID, err := toUUID(contract.AccountID)
	if err != nil {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_, err = qpay.New(s.Pool).UpsertPaymentContract(ctx, qpay.UpsertPaymentContractParams{
		ID: uid, AccountID: accountID, OfferID: contract.OfferID,
		Environment: string(contract.Environment), Merchant: contract.Merchant,
		RemoteSubscriptionID: contract.RemoteSubscriptionID, RemoteCustomerID: contract.RemoteCustomerID,
		ProviderStatus: contract.ProviderStatus, CancelAtPeriodEnd: contract.CancelAtPeriodEnd,
		LastEventAt: stamp(contract.LastEventAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) ContractForAccount(ctx context.Context, accountID, offerID string) (app.ContractRecord, error) {
	if s == nil || s.Pool == nil {
		return app.ContractRecord{}, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(offerID) {
		return app.ContractRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentContractForAccount(ctx, qpay.GetPaymentContractForAccountParams{AccountID: uid, OfferID: offerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ContractRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.ContractRecord{}, dbError(err)
	}
	return contractRecord(row), nil
}

func (s *Store) ContractByRemote(ctx context.Context, environment domain.Environment, merchant, remoteID string) (app.ContractRecord, error) {
	if s == nil || s.Pool == nil {
		return app.ContractRecord{}, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return app.ContractRecord{}, err
	}
	if !validText(merchant) || !validText(remoteID) {
		return app.ContractRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentContractByRemote(ctx, qpay.GetPaymentContractByRemoteParams{
		Environment: string(environment), Merchant: merchant, RemoteSubscriptionID: remoteID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ContractRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.ContractRecord{}, dbError(err)
	}
	return contractRecord(row), nil
}

func (s *Store) UpsertMandate(ctx context.Context, mandate app.MandateRecord) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if !validText(mandate.ID) || !validText(mandate.AccountID) || !validText(mandate.OfferID) ||
		!validText(mandate.OrderID) || !validText(mandate.Merchant) ||
		!validText(mandate.RemoteAuthorizationID) || !validText(mandate.ContractID) ||
		!validText(mandate.RemoteCustomerID) || !validText(mandate.Status) ||
		mandate.Provider != domain.ProviderAsaas || mandate.Value.Centavos <= 0 ||
		mandate.Value.Currency != "BRL" || mandate.StartDate.IsZero() ||
		mandate.ConsentAt.IsZero() || !validText(mandate.ConsentVersion) || mandate.LastEventAt.IsZero() ||
		(mandate.PaymentCreationMode != "" && mandate.PaymentCreationMode != "SUBSCRIPTION") ||
		(mandate.RetryPolicy != "" && mandate.RetryPolicy != "NOT_ALLOWED") {
		return domain.ErrInvalid
	}
	if err := mandate.Environment.Validate(); err != nil {
		return err
	}
	if len(mandate.ContractID) > 35 {
		return domain.ErrInvalid
	}
	uid, err := toUUID(mandate.ID)
	if err != nil {
		return domain.ErrInvalid
	}
	accountID, err := toUUID(mandate.AccountID)
	if err != nil {
		return domain.ErrInvalid
	}
	orderID, err := toUUID(mandate.OrderID)
	if err != nil {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_, err = qpay.New(s.Pool).UpsertPaymentMandate(ctx, qpay.UpsertPaymentMandateParams{
		ID: uid, AccountID: accountID, OfferID: mandate.OfferID, OrderID: orderID,
		Environment: string(mandate.Environment), Merchant: mandate.Merchant,
		RemoteAuthorizationID: mandate.RemoteAuthorizationID,
		RemoteSubscriptionID:  nullableText(mandate.RemoteSubscriptionID),
		ContractID:            mandate.ContractID, RemoteCustomerID: mandate.RemoteCustomerID,
		Status: mandate.Status, ValueCentavos: mandate.Value.Centavos,
		StartDate: dateValue(mandate.StartDate), FinishDate: dateValue(mandate.FinishDate),
		ConsentVersion: mandate.ConsentVersion, ConsentAt: stamp(mandate.ConsentAt),
		LastEventAt: stamp(mandate.LastEventAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) MandateForAccount(ctx context.Context, accountID, offerID string) (app.MandateRecord, error) {
	if s == nil || s.Pool == nil {
		return app.MandateRecord{}, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(offerID) {
		return app.MandateRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentMandateForAccount(ctx, qpay.GetPaymentMandateForAccountParams{AccountID: uid, OfferID: offerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.MandateRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.MandateRecord{}, dbError(err)
	}
	return mandateRecord(row), nil
}

func (s *Store) MandateByRemote(ctx context.Context, environment domain.Environment, merchant, remoteID string) (app.MandateRecord, error) {
	if s == nil || s.Pool == nil {
		return app.MandateRecord{}, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return app.MandateRecord{}, err
	}
	if !validText(merchant) || !validText(remoteID) {
		return app.MandateRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentMandateByRemote(ctx, qpay.GetPaymentMandateByRemoteParams{
		Environment: string(environment), Merchant: merchant, RemoteAuthorizationID: remoteID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.MandateRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.MandateRecord{}, dbError(err)
	}
	return mandateRecord(row), nil
}

func (s *Store) MandateForSubscription(ctx context.Context, environment domain.Environment, merchant, subscriptionID string) (app.MandateRecord, error) {
	if s == nil || s.Pool == nil {
		return app.MandateRecord{}, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return app.MandateRecord{}, err
	}
	if !validText(merchant) || !validText(subscriptionID) {
		return app.MandateRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentMandateForSubscription(ctx, qpay.GetPaymentMandateForSubscriptionParams{
		Environment: string(environment), Merchant: merchant,
		RemoteSubscriptionID: nullableText(subscriptionID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.MandateRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.MandateRecord{}, dbError(err)
	}
	return mandateRecord(row), nil
}

func (s *Store) RequestMandateCancel(ctx context.Context, environment domain.Environment, merchant, remoteAuthorizationID string, at time.Time, reason string) (app.MandateRecord, error) {
	if s == nil || s.Pool == nil {
		return app.MandateRecord{}, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return app.MandateRecord{}, err
	}
	if !validText(merchant) || !validText(remoteAuthorizationID) || at.IsZero() ||
		reason == "" || len(reason) > 64 {
		return app.MandateRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).RequestPaymentMandateCancel(ctx, qpay.RequestPaymentMandateCancelParams{
		RequestedAt: stamp(at), Reason: nullableText(reason), Environment: string(environment),
		Merchant: merchant, RemoteAuthorizationID: remoteAuthorizationID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.MandateRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.MandateRecord{}, dbError(err)
	}
	return mandateRecord(row), nil
}

func (s *Store) Eligibility(ctx context.Context, environment domain.Environment, merchant string) (app.PixAutoEligibility, bool, error) {
	if s == nil || s.Pool == nil {
		return app.PixAutoEligibility{}, false, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return app.PixAutoEligibility{}, false, err
	}
	if !validText(merchant) {
		return app.PixAutoEligibility{}, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentPixAutoEligibility(ctx, qpay.GetPaymentPixAutoEligibilityParams{
		Environment: string(environment), Merchant: merchant,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.PixAutoEligibility{}, false, nil
	}
	if err != nil {
		return app.PixAutoEligibility{}, false, dbError(err)
	}
	return eligibilityRecord(row), true, nil
}

func (s *Store) UpsertEligibility(ctx context.Context, eligibility app.PixAutoEligibility) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if err := eligibility.Environment.Validate(); err != nil {
		return err
	}
	if !validText(eligibility.Merchant) || (eligibility.Status != "ELIGIBLE" && eligibility.Status != "INELIGIBLE") ||
		len(eligibility.Reason) > 256 || eligibility.LastEventAt.IsZero() || eligibility.ObservedAt.IsZero() {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_, err := qpay.New(s.Pool).UpsertPaymentPixAutoEligibility(ctx, qpay.UpsertPaymentPixAutoEligibilityParams{
		Environment: string(eligibility.Environment), Merchant: eligibility.Merchant,
		Status: eligibility.Status, Reason: eligibility.Reason,
		LastEventAt: stamp(eligibility.LastEventAt), ObservedAt: stamp(eligibility.ObservedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) ActiveMandates(ctx context.Context, environment domain.Environment, merchant string, limit int) ([]app.MandateRecord, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return nil, err
	}
	if !validText(merchant) || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ListActivePaymentMandates(ctx, qpay.ListActivePaymentMandatesParams{
		Environment: string(environment), Merchant: merchant, LimitCount: int32(limit),
	})
	if err != nil {
		return nil, dbError(err)
	}
	records := make([]app.MandateRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, mandateRecord(row))
	}
	return records, nil
}

func (s *Store) UpsertInstruction(ctx context.Context, instruction app.InstructionRecord) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if err := instruction.Environment.Validate(); err != nil {
		return err
	}
	if !validText(instruction.ID) || !validText(instruction.AccountID) || !validText(instruction.OfferID) ||
		!validText(instruction.Merchant) || !validText(instruction.RemoteInstructionID) ||
		!validText(instruction.RemoteAuthorizationID) || (instruction.RemoteSubscriptionID != "" && !validText(instruction.RemoteSubscriptionID)) ||
		(instruction.RemotePaymentID != "" && !validText(instruction.RemotePaymentID)) ||
		!validText(instruction.Status) || !validText(instruction.Purpose) ||
		instruction.RetryAttempt < 0 || instruction.RetryAttempt > 3 || instruction.DueDate.IsZero() ||
		len(instruction.RefusalReason) > 128 || instruction.LastEventAt.IsZero() {
		return domain.ErrInvalid
	}
	uid, err := toUUID(instruction.ID)
	if err != nil {
		return domain.ErrInvalid
	}
	accountID, err := toUUID(instruction.AccountID)
	if err != nil {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	_, err = qpay.New(s.Pool).UpsertPaymentPixAutoInstruction(ctx, qpay.UpsertPaymentPixAutoInstructionParams{
		ID: uid, AccountID: accountID, OfferID: instruction.OfferID,
		Environment: string(instruction.Environment), Merchant: instruction.Merchant,
		RemoteInstructionID:   instruction.RemoteInstructionID,
		RemoteAuthorizationID: instruction.RemoteAuthorizationID,
		RemoteSubscriptionID:  nullableText(instruction.RemoteSubscriptionID),
		RemotePaymentID:       nullableText(instruction.RemotePaymentID),
		Status:                instruction.Status, Purpose: instruction.Purpose,
		RetryAttempt: int32(instruction.RetryAttempt), DueDate: dateValue(instruction.DueDate),
		RefusalReason: nullableText(instruction.RefusalReason), LastEventAt: stamp(instruction.LastEventAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) InstructionsForMandate(ctx context.Context, environment domain.Environment, merchant, remoteAuthorizationID string, limit int) ([]app.InstructionRecord, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	if err := environment.Validate(); err != nil {
		return nil, err
	}
	if !validText(merchant) || !validText(remoteAuthorizationID) || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ListPaymentPixAutoInstructions(ctx, qpay.ListPaymentPixAutoInstructionsParams{
		Environment: string(environment), Merchant: merchant,
		RemoteAuthorizationID: remoteAuthorizationID, LimitCount: int32(limit),
	})
	if err != nil {
		return nil, dbError(err)
	}
	records := make([]app.InstructionRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, instructionRecord(row))
	}
	return records, nil
}

func (s *Store) EnsurePixAutoCycleOrder(ctx context.Context, input app.PixAutoCycleOrderInput) (string, error) {
	if s == nil || s.Pool == nil {
		return "", domain.ErrUnavailable
	}
	if !validText(input.AccountID) || !validText(input.OfferID) || !validText(input.CatalogVersion) ||
		len(input.Capabilities) == 0 || input.Merchant == "" || !validText(input.Merchant) ||
		!validText(input.RemotePaymentID) || !validText(input.RemoteSubscriptionID) ||
		input.SettledAt.IsZero() || input.Amount.Positive() != nil {
		return "", domain.ErrInvalid
	}
	if err := input.Environment.Validate(); err != nil {
		return "", err
	}
	accountID, err := toUUID(input.AccountID)
	if err != nil {
		return "", domain.ErrInvalid
	}
	orderID := derivedUUID("payment-pix-auto-cycle-order", string(input.Environment), input.Merchant, input.RemotePaymentID)
	orderUID, _ := toUUID(orderID)
	operationID := derivedUUID("payment-pix-auto-cycle-operation", string(input.Environment), input.Merchant, input.RemotePaymentID)
	operationUID, _ := toUUID(operationID)
	attemptID := derivedUUID("payment-pix-auto-cycle-attempt", string(input.Environment), input.Merchant, input.RemotePaymentID)
	attemptUID, _ := toUUID(attemptID)
	operationKey := "pix-auto-payment:" + digestValue(input.RemotePaymentID)

	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	if err := q.LockPaymentKey(ctx, "payment-pix-auto-cycle:"+string(input.Environment)+":"+input.Merchant+":"+input.RemotePaymentID); err != nil {
		return "", dbError(err)
	}
	if row, err := q.GetPaymentSettlementByRemote(ctx, qpay.GetPaymentSettlementByRemoteParams{
		Provider: "asaas", Environment: string(input.Environment), Merchant: input.Merchant,
		RemotePaymentID: input.RemotePaymentID,
	}); err == nil {
		if row.Channel != string(domain.ChannelPixAutomatic) || row.AmountCentavos != input.Amount.Centavos {
			return "", domain.ErrConflict
		}
		return uuidText(row.OrderID), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", dbError(err)
	}
	if operation, err := q.GetPaymentOperationByKey(ctx, qpay.GetPaymentOperationByKeyParams{
		AccountID: accountID, Provider: "asaas", Environment: string(input.Environment),
		Merchant: input.Merchant, OperationKey: operationKey,
	}); err == nil {
		if !operation.OrderID.Valid {
			return "", domain.ErrBinding
		}
		return uuidText(operation.OrderID), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", dbError(err)
	}
	now := time.Now().UTC()
	rows, err := q.InsertPaymentOrder(ctx, qpay.InsertPaymentOrderParams{
		ID: orderUID, AccountID: accountID, OfferID: input.OfferID,
		Family: input.Family, CatalogVersion: input.CatalogVersion, Channel: string(domain.ChannelPixAutomatic),
		AmountCentavos: input.Amount.Centavos, Currency: "BRL", Capabilities: input.Capabilities,
		State: string(domain.OrderPending), QuotedAt: stamp(input.SettledAt),
		ExpiresAt: stamp(input.SettledAt.Add(domain.QuoteTTL)),
	})
	if err != nil {
		return "", dbError(err)
	}
	if rows != 1 {
		existing, getErr := q.GetPaymentOrder(ctx, orderUID)
		if getErr != nil {
			return "", dbError(getErr)
		}
		if uuidText(existing.AccountID) != input.AccountID || existing.OfferID != input.OfferID ||
			existing.Channel != string(domain.ChannelPixAutomatic) || existing.AmountCentavos != input.Amount.Centavos ||
			existing.CatalogVersion != input.CatalogVersion || !sameSet(existing.Capabilities, input.Capabilities) {
			return "", domain.ErrConflict
		}
	}
	if err := q.InsertPaymentOperation(ctx, qpay.InsertPaymentOperationParams{
		ID: operationUID, AccountID: accountID, Provider: "asaas",
		Environment: string(input.Environment), Merchant: input.Merchant,
		OperationKey: operationKey, Kind: "reconcile", PayloadHash: digestValue(input.RemotePaymentID),
		OrderID: orderUID, LeaseUntil: stamp(now.Add(time.Minute)),
	}); err != nil {
		return "", dbError(err)
	}
	resolved, err := q.ResolvePaymentOperation(ctx, qpay.ResolvePaymentOperationParams{
		State: "SUCCEEDED", RemoteID: nullableText(input.RemotePaymentID), LeaseUntil: stamp(now),
		CompletedAt: stamp(now), ID: operationUID, Generation: 1,
	})
	if err != nil {
		return "", dbError(err)
	}
	if resolved != 1 {
		return "", domain.ErrConflict
	}
	if err := q.InsertPaymentPixAutoCycleAttempt(ctx, qpay.InsertPaymentPixAutoCycleAttemptParams{
		ID: attemptUID, OperationID: operationUID, OrderID: orderUID,
		Environment: string(input.Environment), Merchant: input.Merchant,
		PayloadHash: digestValue(input.RemotePaymentID), RemoteID: nullableText(input.RemotePaymentID),
		LeaseUntil: stamp(input.SettledAt), CreatedAt: stamp(input.SettledAt),
	}); err != nil {
		return "", dbError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", domain.ErrUnavailable
	}
	return orderID, nil
}

func eligibilityRecord(row qpay.PaymentPixAutoEligibility) app.PixAutoEligibility {
	return app.PixAutoEligibility{
		Environment: domain.Environment(row.Environment), Merchant: row.Merchant,
		Status: row.Status, Reason: row.Reason,
		LastEventAt: row.LastEventAt.Time, ObservedAt: row.ObservedAt.Time,
	}
}

func instructionRecord(row qpay.PaymentPixAutoInstruction) app.InstructionRecord {
	return app.InstructionRecord{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		Environment: domain.Environment(row.Environment), Merchant: row.Merchant,
		RemoteInstructionID: row.RemoteInstructionID, RemoteAuthorizationID: row.RemoteAuthorizationID,
		RemoteSubscriptionID: textValue(row.RemoteSubscriptionID), RemotePaymentID: textValue(row.RemotePaymentID),
		Status: row.Status, Purpose: row.Purpose, RetryAttempt: int(row.RetryAttempt),
		DueDate: row.DueDate.Time, RefusalReason: textValue(row.RefusalReason),
		LastEventAt: row.LastEventAt.Time,
	}
}

func derivedUUID(namespace string, values ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(namespace))
	for _, value := range values {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(value))
	}
	raw := hash.Sum(nil)[:16]
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}

func digestValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Store) ActiveRecurringSource(ctx context.Context, accountID, offerID string) (app.RecurringSource, bool, error) {
	if s == nil || s.Pool == nil {
		return app.RecurringSource{}, false, domain.ErrUnavailable
	}
	contract, err := s.ContractForAccount(ctx, accountID, offerID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return app.RecurringSource{}, false, err
	}
	if err == nil && !contractFenced(contract) {
		return app.RecurringSource{
			Source: contract.Source, OfferID: offerID, LocalID: contract.ID,
			RemoteID: contract.RemoteSubscriptionID, ProviderStatus: contract.ProviderStatus,
			CancelAtPeriodEnd: contract.CancelAtPeriodEnd,
		}, true, nil
	}
	mandate, err := s.MandateForAccount(ctx, accountID, offerID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return app.RecurringSource{}, false, err
	}
	if err == nil && (mandate.Status == "CREATED" || mandate.Status == "ACTIVE") {
		return app.RecurringSource{
			Source: domain.ChannelPixAutomatic, OfferID: offerID, LocalID: mandate.ID,
			RemoteID: mandate.RemoteAuthorizationID, ProviderStatus: mandate.Status,
		}, true, nil
	}
	return app.RecurringSource{}, false, nil
}

func (s *Store) ActiveGrantEnd(ctx context.Context, accountID, offerID string) (time.Time, error) {
	if s == nil || s.Pool == nil {
		return time.Time{}, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(offerID) {
		return time.Time{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetActivePaymentGrantEnd(ctx, qpay.GetActivePaymentGrantEndParams{
		AccountID: uid, OfferID: offerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, dbError(err)
	}
	if !row.Valid {
		return time.Time{}, nil
	}
	return row.Time.UTC(), nil
}

func (s *Store) OpenSourceSwitch(ctx context.Context, accountID, offerID string) (app.SourceSwitchRecord, bool, error) {
	if s == nil || s.Pool == nil {
		return app.SourceSwitchRecord{}, false, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(offerID) {
		return app.SourceSwitchRecord{}, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetOpenPaymentSourceSwitch(ctx, qpay.GetOpenPaymentSourceSwitchParams{
		AccountID: uid, OfferID: offerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.SourceSwitchRecord{}, false, nil
	}
	if err != nil {
		return app.SourceSwitchRecord{}, false, dbError(err)
	}
	return sourceSwitchRecord(row), true, nil
}

func (s *Store) CreateSourceSwitch(ctx context.Context, input app.SourceSwitchRecord) (app.SourceSwitchRecord, bool, error) {
	if s == nil || s.Pool == nil {
		return app.SourceSwitchRecord{}, false, domain.ErrUnavailable
	}
	if err := input.Environment.Validate(); err != nil {
		return app.SourceSwitchRecord{}, false, err
	}
	if !validText(input.AccountID) || !validText(input.OfferID) || !validText(input.Merchant) ||
		!validText(input.OperationKey) || !validText(input.OldOfferID) || !validText(input.TargetCatalogVersion) ||
		len(input.TargetCapabilities) == 0 || input.TargetAmount.Positive() != nil ||
		input.RequestedAt.IsZero() || input.LastEventAt.IsZero() ||
		!validSwitchSource(input.OldSource) || !validSwitchTarget(input.NewSource) ||
		!validSwitchState(input.State) || !input.OldCancellation.Valid() {
		return app.SourceSwitchRecord{}, false, domain.ErrInvalid
	}
	if input.State == app.SourceSwitchReady && input.FirstDebitAt.IsZero() {
		return app.SourceSwitchRecord{}, false, domain.ErrInvalid
	}
	accountID, err := toUUID(input.AccountID)
	if err != nil {
		return app.SourceSwitchRecord{}, false, domain.ErrInvalid
	}
	id := input.ID
	if id == "" {
		id, err = newID()
		if err != nil {
			return app.SourceSwitchRecord{}, false, domain.ErrUnavailable
		}
	}
	uid, err := toUUID(id)
	if err != nil {
		return app.SourceSwitchRecord{}, false, domain.ErrInvalid
	}
	var currentEnd pgtype.Timestamptz
	if !input.CurrentEnd.IsZero() {
		currentEnd = stamp(input.CurrentEnd)
	}
	var firstDebit pgtype.Timestamptz
	if !input.FirstDebitAt.IsZero() {
		firstDebit = stamp(input.FirstDebitAt)
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).InsertPaymentSourceSwitch(ctx, qpay.InsertPaymentSourceSwitchParams{
		ID: uid, AccountID: accountID, OfferID: input.OfferID,
		Environment: string(input.Environment), Merchant: input.Merchant,
		OperationKey: input.OperationKey, OldSource: string(input.OldSource),
		OldOfferID: input.OldOfferID, NewSource: string(input.NewSource),
		OldLocalID:  nullableText(input.OldLocalID),
		OldRemoteID: nullableText(input.OldRemoteID), TargetCatalogVersion: input.TargetCatalogVersion,
		TargetAmountCentavos: input.TargetAmount.Centavos, TargetCapabilities: input.TargetCapabilities,
		CurrentEnd: currentEnd, FirstDebitAt: firstDebit, State: input.State,
		OldCancellation: string(input.OldCancellation), RequestedAt: stamp(input.RequestedAt),
		LastEventAt: stamp(input.LastEventAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := qpay.New(s.Pool).GetPaymentSourceSwitchByOperation(ctx, qpay.GetPaymentSourceSwitchByOperationParams{
			AccountID: accountID, Environment: string(input.Environment),
			Merchant: input.Merchant, OperationKey: input.OperationKey,
		})
		if err != nil {
			return app.SourceSwitchRecord{}, false, dbError(err)
		}
		return sourceSwitchRecord(existing), true, nil
	}
	if err != nil {
		return app.SourceSwitchRecord{}, false, dbError(err)
	}
	return sourceSwitchRecord(row), false, nil
}

func (s *Store) ClaimSourceSwitch(ctx context.Context, id, workerID string, leaseUntil time.Time) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil || !validText(workerID) || leaseUntil.IsZero() {
		return false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ClaimPaymentSourceSwitch(ctx, qpay.ClaimPaymentSourceSwitchParams{
		WorkerID: nullableText(workerID), LeaseUntil: stamp(leaseUntil), ID: uid,
	})
	if err != nil {
		return false, dbError(err)
	}
	return rows == 1, nil
}

func (s *Store) ResolveSourceSwitch(ctx context.Context, input app.SourceSwitchRecord) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	uid, err := toUUID(input.ID)
	if err != nil || !validSwitchState(input.State) || !input.OldCancellation.Valid() ||
		input.LastEventAt.IsZero() || (input.State == app.SourceSwitchReady && input.FirstDebitAt.IsZero()) ||
		len(input.LastError) > 500 {
		return domain.ErrInvalid
	}
	var firstDebit pgtype.Timestamptz
	if !input.FirstDebitAt.IsZero() {
		firstDebit = stamp(input.FirstDebitAt)
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ResolvePaymentSourceSwitch(ctx, qpay.ResolvePaymentSourceSwitchParams{
		State: input.State, OldCancellation: string(input.OldCancellation),
		FirstDebitAt: firstDebit, LastError: input.LastError,
		LastEventAt: stamp(input.LastEventAt), ID: uid,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	return nil
}

func (s *Store) ConsumeSourceSwitch(ctx context.Context, id string) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if _, err := qpay.New(s.Pool).ConsumePaymentSourceSwitch(ctx, uid); err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) CreateBillingExit(ctx context.Context, accountID, operationKey, reason string, at time.Time) (app.BillingExit, []app.BillingExitSource, bool, error) {
	if s == nil || s.Pool == nil {
		return app.BillingExit{}, nil, false, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(operationKey) || at.IsZero() ||
		(reason != app.BillingExitOwnerRequest && reason != app.BillingExitAccountDeletion) {
		return app.BillingExit{}, nil, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.BillingExit{}, nil, false, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	if existing, err := q.GetPaymentBillingExitByOperation(ctx, qpay.GetPaymentBillingExitByOperationParams{
		AccountID: uid, OperationKey: operationKey,
	}); err == nil {
		sources, err := q.ListPaymentBillingExitSources(ctx, existing.ID)
		if err != nil {
			return app.BillingExit{}, nil, false, dbError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return app.BillingExit{}, nil, false, domain.ErrUnavailable
		}
		return billingExitRecord(existing), billingExitSourceRecords(sources), true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.BillingExit{}, nil, false, dbError(err)
	}
	id, err := newID()
	if err != nil {
		return app.BillingExit{}, nil, false, domain.ErrUnavailable
	}
	exitUID, _ := toUUID(id)
	row, err := q.InsertPaymentBillingExit(ctx, qpay.InsertPaymentBillingExitParams{
		ID: exitUID, AccountID: uid, OperationKey: operationKey, Reason: reason,
		State: "PENDING", RequestedAt: stamp(at), LastEventAt: stamp(at),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := q.GetPaymentBillingExitByOperation(ctx, qpay.GetPaymentBillingExitByOperationParams{
			AccountID: uid, OperationKey: operationKey,
		})
		if err != nil {
			return app.BillingExit{}, nil, false, dbError(err)
		}
		sources, err := q.ListPaymentBillingExitSources(ctx, existing.ID)
		if err != nil {
			return app.BillingExit{}, nil, false, dbError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return app.BillingExit{}, nil, false, domain.ErrUnavailable
		}
		return billingExitRecord(existing), billingExitSourceRecords(sources), true, nil
	}
	if err != nil {
		return app.BillingExit{}, nil, false, dbError(err)
	}
	sources, err := billingSourcesTx(ctx, q, uid)
	if err != nil {
		return app.BillingExit{}, nil, false, err
	}
	records := make([]app.BillingExitSource, 0, len(sources))
	for _, source := range sources {
		sourceID, err := newID()
		if err != nil {
			return app.BillingExit{}, nil, false, domain.ErrUnavailable
		}
		sourceUID, _ := toUUID(sourceID)
		sourceRow, err := q.InsertPaymentBillingExitSource(ctx, qpay.InsertPaymentBillingExitSourceParams{
			ID: sourceUID, ExitID: exitUID, AccountID: uid, Source: string(source.Source),
			LocalID: source.LocalID, RemoteID: source.RemoteID, OfferID: source.OfferID,
			Provider: string(source.Provider), Environment: string(source.Environment),
			Merchant: source.Merchant, LastEventAt: stamp(at),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return app.BillingExit{}, nil, false, dbError(err)
		}
		records = append(records, billingExitSourceRecord(sourceRow))
	}
	state := "DONE"
	if len(records) > 0 {
		state = "PENDING"
	}
	if _, err := q.UpdatePaymentBillingExit(ctx, qpay.UpdatePaymentBillingExitParams{
		State: state, LastEventAt: stamp(at), LastError: "", ID: exitUID,
	}); err != nil {
		return app.BillingExit{}, nil, false, dbError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return app.BillingExit{}, nil, false, domain.ErrUnavailable
	}
	row.State = state
	return billingExitRecord(row), records, false, nil
}

func (s *Store) LatestBillingExit(ctx context.Context, accountID string) (app.BillingExit, bool, error) {
	if s == nil || s.Pool == nil {
		return app.BillingExit{}, false, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil {
		return app.BillingExit{}, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetLatestPaymentBillingExit(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.BillingExit{}, false, nil
	}
	if err != nil {
		return app.BillingExit{}, false, dbError(err)
	}
	return billingExitRecord(row), true, nil
}

func (s *Store) BillingExit(ctx context.Context, id string) (app.BillingExit, error) {
	if s == nil || s.Pool == nil {
		return app.BillingExit{}, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil {
		return app.BillingExit{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentBillingExitByID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.BillingExit{}, domain.ErrNotFound
	}
	if err != nil {
		return app.BillingExit{}, dbError(err)
	}
	return billingExitRecord(row), nil
}

func (s *Store) BillingExitSources(ctx context.Context, exitID string) ([]app.BillingExitSource, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	uid, err := toUUID(exitID)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ListPaymentBillingExitSources(ctx, uid)
	if err != nil {
		return nil, dbError(err)
	}
	return billingExitSourceRecords(rows), nil
}

func (s *Store) UpdateBillingExit(ctx context.Context, exit app.BillingExit) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	uid, err := toUUID(exit.ID)
	if err != nil || (exit.State != "PENDING" && exit.State != "DONE" && exit.State != "REVIEW") ||
		exit.LastEventAt.IsZero() || len(exit.LastError) > 500 {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).UpdatePaymentBillingExit(ctx, qpay.UpdatePaymentBillingExitParams{
		State: exit.State, LastEventAt: stamp(exit.LastEventAt),
		LastError: exit.LastError, ID: uid,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	return nil
}

func (s *Store) ClaimBillingExitSource(ctx context.Context, id string, leaseUntil time.Time) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil || leaseUntil.IsZero() {
		return false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ClaimPaymentBillingExitSource(ctx, qpay.ClaimPaymentBillingExitSourceParams{
		LeaseUntil: stamp(leaseUntil), ID: uid,
	})
	if err != nil {
		return false, dbError(err)
	}
	return rows == 1, nil
}

func (s *Store) ResolveBillingExitSource(ctx context.Context, source app.BillingExitSource) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	uid, err := toUUID(source.ID)
	if err != nil || (source.State != "DONE" && source.State != "PENDING" && source.State != "REVIEW") ||
		source.LastEventAt.IsZero() || len(source.LastError) > 500 {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ResolvePaymentBillingExitSource(ctx, qpay.ResolvePaymentBillingExitSourceParams{
		State: source.State, LastError: source.LastError,
		LastEventAt: stamp(source.LastEventAt), ID: uid,
	})
	if err != nil {
		return dbError(err)
	}
	if rows != 1 {
		return domain.ErrStale
	}
	return nil
}

func (s *Store) BillingSources(ctx context.Context, accountID string) ([]app.BillingSource, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return billingSourcesTx(ctx, qpay.New(s.Pool), uid)
}

func billingSourcesTx(ctx context.Context, q *qpay.Queries, accountID pgtype.UUID) ([]app.BillingSource, error) {
	contracts, err := q.ListPaymentBillingCardsForAccount(ctx, accountID)
	if err != nil {
		return nil, dbError(err)
	}
	mandates, err := q.ListPaymentBillingMandatesForAccount(ctx, accountID)
	if err != nil {
		return nil, dbError(err)
	}
	sources := make([]app.BillingSource, 0, len(contracts)+len(mandates))
	for _, contract := range contracts {
		sources = append(sources, app.BillingSource{
			Source: domain.ChannelCard, LocalID: uuidText(contract.ID),
			RemoteID: contract.RemoteSubscriptionID, OfferID: contract.OfferID,
			Provider: domain.ProviderStripe, Environment: domain.Environment(contract.Environment),
			Merchant: contract.Merchant,
		})
	}
	for _, mandate := range mandates {
		sources = append(sources, app.BillingSource{
			Source: domain.ChannelPixAutomatic, LocalID: uuidText(mandate.ID),
			RemoteID: mandate.RemoteAuthorizationID, OfferID: mandate.OfferID,
			Provider: domain.ProviderAsaas, Environment: domain.Environment(mandate.Environment),
			Merchant: mandate.Merchant,
		})
	}
	return sources, nil
}

func (s *Store) BillingHistory(ctx context.Context, accountID string, limit int) ([]app.BillingHistoryRow, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).ListPaymentBillingHistory(ctx, qpay.ListPaymentBillingHistoryParams{
		AccountID: uid, LimitCount: int32(limit),
	})
	if err != nil {
		return nil, dbError(err)
	}
	result := make([]app.BillingHistoryRow, 0, len(rows))
	for _, row := range rows {
		item := app.BillingHistoryRow{
			OrderID: uuidText(row.OrderID), OfferID: row.OfferID,
			Channel: domain.Channel(row.Channel), Amount: domain.BRL(row.AmountCentavos),
			OrderState: row.OrderState, OrderedAt: row.OrderedAt.Time,
			SettlementID: uuidText(row.SettlementID), RemotePaymentID: textValue(row.RemotePaymentID),
			SettlementState: textValue(row.SettlementState),
			SettledAt:       row.SettledAt.Time, GrantID: uuidText(row.GrantID),
			GrantStart: row.StartAt.Time, GrantEnd: row.EndAt.Time,
			GrantState: textValue(row.GrantState),
		}
		result = append(result, item)
	}
	return result, nil
}

func billingExitRecord(row qpay.PaymentBillingExit) app.BillingExit {
	return app.BillingExit{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OperationKey: row.OperationKey,
		Reason: row.Reason, State: row.State, RequestedAt: row.RequestedAt.Time,
		LastEventAt: row.LastEventAt.Time, LastError: row.LastError,
	}
}

func billingExitSourceRecord(row qpay.PaymentBillingExitSource) app.BillingExitSource {
	return app.BillingExitSource{
		ID: uuidText(row.ID), ExitID: uuidText(row.ExitID), AccountID: uuidText(row.AccountID),
		Source:  domain.Channel(row.Source),
		LocalID: row.LocalID, RemoteID: row.RemoteID, OfferID: row.OfferID,
		Provider: domain.Provider(row.Provider), Environment: domain.Environment(row.Environment),
		Merchant: row.Merchant, State: row.State, Attempts: int(row.Attempts),
		LastError: row.LastError, LastEventAt: row.LastEventAt.Time,
	}
}

func billingExitSourceRecords(rows []qpay.PaymentBillingExitSource) []app.BillingExitSource {
	records := make([]app.BillingExitSource, 0, len(rows))
	for _, row := range rows {
		records = append(records, billingExitSourceRecord(row))
	}
	return records
}

func sourceSwitchRecord(row qpay.PaymentSourceSwitch) app.SourceSwitchRecord {
	record := app.SourceSwitchRecord{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		Environment: domain.Environment(row.Environment), Merchant: row.Merchant,
		OperationKey: row.OperationKey, OldSource: domain.Channel(row.OldSource),
		OldOfferID: row.OldOfferID, NewSource: domain.Channel(row.NewSource),
		OldLocalID:  textValue(row.OldLocalID),
		OldRemoteID: textValue(row.OldRemoteID), TargetCatalogVersion: row.TargetCatalogVersion,
		TargetAmount:       domain.BRL(row.TargetAmountCentavos),
		TargetCapabilities: append([]string(nil), row.TargetCapabilities...),
		State:              row.State, OldCancellation: domain.CancellationState(row.OldCancellation),
		Attempts: int(row.Attempts), LastError: row.LastError,
		RequestedAt: row.RequestedAt.Time, LastEventAt: row.LastEventAt.Time,
	}
	if row.CurrentEnd.Valid {
		record.CurrentEnd = row.CurrentEnd.Time
	}
	if row.FirstDebitAt.Valid {
		record.FirstDebitAt = row.FirstDebitAt.Time
	}
	return record
}

func contractFenced(contract app.ContractRecord) bool {
	if contract.CancelAtPeriodEnd {
		return true
	}
	return contract.ProviderStatus == "canceled" || contract.ProviderStatus == "incomplete_expired"
}

func validSwitchSource(source domain.Channel) bool {
	return source == domain.ChannelCard || source == domain.ChannelPixAutomatic || source == domain.ChannelPixManual
}

func validSwitchTarget(target domain.Channel) bool {
	return target == domain.ChannelCard || target == domain.ChannelPixAutomatic
}

func validSwitchState(state string) bool {
	return state == app.SourceSwitchPending || state == app.SourceSwitchCanceling ||
		state == app.SourceSwitchReady || state == app.SourceSwitchConsumed ||
		state == app.SourceSwitchCanceled
}

func mandateRecord(row qpay.PaymentMandate) app.MandateRecord {
	mandate := app.MandateRecord{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		OrderID: uuidText(row.OrderID), Environment: domain.Environment(row.Environment),
		Merchant: row.Merchant, Provider: domain.Provider(row.Provider),
		RemoteAuthorizationID: row.RemoteAuthorizationID, RemoteSubscriptionID: textValue(row.RemoteSubscriptionID),
		ContractID: row.ContractID, RemoteCustomerID: row.RemoteCustomerID,
		Status: row.Status, Frequency: row.Frequency, Value: domain.BRL(row.ValueCentavos),
		PaymentCreationMode: row.PaymentCreationMode, RetryPolicy: row.RetryPolicy,
		StartDate: row.StartDate.Time, ConsentVersion: row.ConsentVersion,
		ConsentAt: row.ConsentAt.Time, LastEventAt: row.LastEventAt.Time,
		CancelReason: textValue(row.CancelReason),
	}
	if row.FinishDate.Valid {
		mandate.FinishDate = row.FinishDate.Time
	}
	if row.CancelRequestedAt.Valid {
		value := row.CancelRequestedAt.Time
		mandate.CancelRequestedAt = &value
	}
	if row.CancelledAt.Valid {
		value := row.CancelledAt.Time
		mandate.CancelledAt = &value
	}
	return mandate
}

func contractRecord(row qpay.PaymentContract) app.ContractRecord {
	return app.ContractRecord{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		Source: domain.Channel(row.Source), Environment: domain.Environment(row.Environment),
		Merchant: row.Merchant, Provider: domain.Provider(row.Provider),
		RemoteSubscriptionID: row.RemoteSubscriptionID, RemoteCustomerID: row.RemoteCustomerID,
		ProviderStatus: row.ProviderStatus, CancelAtPeriodEnd: row.CancelAtPeriodEnd,
		LastEventAt: row.LastEventAt.Time,
	}
}

func domainOrder(row qpay.PaymentOrder) (domain.Order, error) {
	if !row.QuotedAt.Valid || !row.ExpiresAt.Valid {
		return domain.Order{}, domain.ErrBinding
	}
	order := domain.Order{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		Family: row.Family, CatalogVersion: row.CatalogVersion, Channel: domain.Channel(row.Channel),
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

func dateValue(value time.Time) pgtype.Date {
	if value.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: value.UTC(), Valid: true}
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
