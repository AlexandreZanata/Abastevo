package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	qpay "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/payments"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func (s *Store) RefundTarget(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant, remotePaymentID string) (app.RefundTarget, error) {
	if s == nil || s.Pool == nil {
		return app.RefundTarget{}, domain.ErrUnavailable
	}
	if err := provider.Validate(); err != nil {
		return app.RefundTarget{}, err
	}
	if err := environment.Validate(); err != nil {
		return app.RefundTarget{}, err
	}
	if !validText(merchant) || !validText(remotePaymentID) {
		return app.RefundTarget{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentSettlementByRemote(ctx, qpay.GetPaymentSettlementByRemoteParams{
		Provider: string(provider), Environment: string(environment),
		Merchant: merchant, RemotePaymentID: remotePaymentID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RefundTarget{}, domain.ErrNotFound
	}
	if err != nil {
		return app.RefundTarget{}, dbError(err)
	}
	target := app.RefundTarget{
		AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		OrderID: uuidText(row.OrderID), SettlementID: uuidText(row.ID),
		Provider: provider, Environment: environment, Merchant: merchant,
		RemotePaymentID: remotePaymentID, RemoteChargeID: textValue(row.RemoteChargeID),
		Paid: domain.BRL(row.AmountCentavos),
	}
	grant, err := qpay.New(s.Pool).GetPaymentGrantBySettlement(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		refunds, err := qpay.New(s.Pool).ListPaymentRefundsForSettlement(ctx, row.ID)
		if err != nil {
			return app.RefundTarget{}, dbError(err)
		}
		target.Reserved = sumReserved(refundsToStates(refunds))
		return target, nil
	}
	if err != nil {
		return app.RefundTarget{}, dbError(err)
	}
	if grant.State != "ACTIVE" && grant.State != "REVOKED" {
		return app.RefundTarget{}, domain.ErrBinding
	}
	target.GrantID = uuidText(grant.ID)
	target.Paid = domain.BRL(grant.PaidCentavos)
	target.GrantState = grant.State
	refunds, err := qpay.New(s.Pool).ListPaymentRefundsForGrant(ctx, grant.ID)
	if err != nil {
		return app.RefundTarget{}, dbError(err)
	}
	target.Reserved = sumReserved(refundsToStates(refunds))
	return target, nil
}

func refundsToStates(rows []qpay.PaymentRefund) []domain.Refund {
	out := make([]domain.Refund, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Refund{
			ID: uuidText(row.ID), GrantID: uuidText(row.GrantID),
			Amount: domain.BRL(row.AmountCentavos),
			State:  domain.RefundState(row.State),
		})
	}
	return out
}

func sumReserved(refunds []domain.Refund) domain.Money {
	total := domain.BRL(0)
	for _, refund := range refunds {
		if refund.State != domain.RefundRequested && refund.State != domain.RefundPending &&
			refund.State != domain.RefundSucceeded {
			continue
		}
		next, err := total.Add(refund.Amount)
		if err != nil {
			return domain.BRL(0)
		}
		total = next
	}
	return total
}

func (s *Store) ReserveRefund(ctx context.Context, input app.RefundReservation) (app.RefundRecord, bool, error) {
	if s == nil || s.Pool == nil {
		return app.RefundRecord{}, false, domain.ErrUnavailable
	}
	if err := input.Provider.Validate(); err != nil {
		return app.RefundRecord{}, false, err
	}
	if err := input.Environment.Validate(); err != nil {
		return app.RefundRecord{}, false, err
	}
	if !validText(input.Merchant) || !validText(input.RemotePaymentID) ||
		!validText(input.OperationKey) || !validReason(input.Reason, 256) || !validText(input.RequestedBy) {
		return app.RefundRecord{}, false, domain.ErrInvalid
	}
	if err := input.Amount.Positive(); err != nil {
		return app.RefundRecord{}, false, err
	}
	if input.ApprovedBy != "" && !validText(input.ApprovedBy) {
		return app.RefundRecord{}, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.RefundRecord{}, false, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	if err := q.LockPaymentKey(ctx, "payment-refund:"+string(input.Provider)+":"+string(input.Environment)+":"+input.Merchant+":"+input.RemotePaymentID); err != nil {
		return app.RefundRecord{}, false, dbError(err)
	}
	settlement, err := q.GetPaymentSettlementByRemote(ctx, qpay.GetPaymentSettlementByRemoteParams{
		Provider: string(input.Provider), Environment: string(input.Environment),
		Merchant: input.Merchant, RemotePaymentID: input.RemotePaymentID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RefundRecord{}, false, domain.ErrNotFound
	}
	if err != nil {
		return app.RefundRecord{}, false, dbError(err)
	}
	if settlement.State == "REVERSED" {
		return app.RefundRecord{}, false, domain.ErrConflict
	}
	var grantID pgtype.UUID
	paid := settlement.AmountCentavos
	grant, err := q.GetPaymentGrantBySettlement(ctx, settlement.ID)
	if err == nil {
		grantID = grant.ID
		paid = grant.PaidCentavos
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.RefundRecord{}, false, dbError(err)
	}
	var existing []qpay.PaymentRefund
	if grantID.Valid {
		existing, err = q.ListPaymentRefundsForGrant(ctx, grantID)
	} else {
		existing, err = q.ListPaymentRefundsForSettlement(ctx, settlement.ID)
	}
	if err != nil {
		return app.RefundRecord{}, false, dbError(err)
	}
	for _, row := range existing {
		if row.OperationKey == input.OperationKey {
			return refundRecord(row), true, nil
		}
	}
	reserved := int64(0)
	for _, row := range existing {
		if row.State == "REQUESTED" || row.State == "PENDING" || row.State == "SUCCEEDED" {
			reserved += row.AmountCentavos
		}
	}
	if input.Amount.Centavos > paid-reserved {
		return app.RefundRecord{}, false, domain.ErrRefundBounds
	}
	id, err := newID()
	if err != nil {
		return app.RefundRecord{}, false, domain.ErrUnavailable
	}
	uid, _ := toUUID(id)
	accountID, err := toUUID(uuidText(settlement.AccountID))
	if err != nil {
		return app.RefundRecord{}, false, domain.ErrInvalid
	}
	row, err := q.InsertPaymentRefund(ctx, qpay.InsertPaymentRefundParams{
		ID: uid, AccountID: accountID, OfferID: settlement.OfferID, OrderID: settlement.OrderID,
		GrantID: grantID, SettlementID: settlement.ID, Provider: string(input.Provider),
		Environment: string(input.Environment), Merchant: input.Merchant,
		OperationKey: input.OperationKey, RemotePaymentID: input.RemotePaymentID,
		AmountCentavos: input.Amount.Centavos, State: string(domain.RefundRequested),
		Reason: input.Reason, RequestedBy: input.RequestedBy,
		ApprovedBy: nullableText(input.ApprovedBy), RequestedAt: stamp(time.Now().UTC()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := q.GetPaymentRefundByOperation(ctx, qpay.GetPaymentRefundByOperationParams{
			AccountID: accountID, Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, OperationKey: input.OperationKey,
		})
		if err != nil {
			return app.RefundRecord{}, false, dbError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return app.RefundRecord{}, false, domain.ErrUnavailable
		}
		return refundRecord(existing), true, nil
	}
	if err != nil {
		return app.RefundRecord{}, false, dbError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return app.RefundRecord{}, false, domain.ErrUnavailable
	}
	return refundRecord(row), false, nil
}

func (s *Store) GetRefund(ctx context.Context, id string) (app.RefundRecord, error) {
	if s == nil || s.Pool == nil {
		return app.RefundRecord{}, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil {
		return app.RefundRecord{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentRefund(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RefundRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return app.RefundRecord{}, dbError(err)
	}
	return refundRecord(row), nil
}

func (s *Store) GetRefundByOperation(ctx context.Context, accountID string, provider domain.Provider, environment domain.Environment, merchant, operationKey string) (app.RefundRecord, bool, error) {
	if s == nil || s.Pool == nil {
		return app.RefundRecord{}, false, domain.ErrUnavailable
	}
	uid, err := toUUID(accountID)
	if err != nil || !validText(merchant) || !validText(operationKey) {
		return app.RefundRecord{}, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentRefundByOperation(ctx, qpay.GetPaymentRefundByOperationParams{
		AccountID: uid, Provider: string(provider), Environment: string(environment),
		Merchant: merchant, OperationKey: operationKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RefundRecord{}, false, nil
	}
	if err != nil {
		return app.RefundRecord{}, false, dbError(err)
	}
	return refundRecord(row), true, nil
}

// UpdateRefundState transitions one refund row and, on success, revokes only
// the matched unexpired grant and marks the settlement state.
func (s *Store) UpdateRefundState(ctx context.Context, id string, previous, next domain.RefundState, remoteRefundID, lastError string, at time.Time) (app.RefundRecord, error) {
	if s == nil || s.Pool == nil {
		return app.RefundRecord{}, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil || !previous.Valid() || !next.Valid() {
		return app.RefundRecord{}, domain.ErrInvalid
	}
	if !domain.RefundTransitionAllowed(previous, next) {
		return app.RefundRecord{}, domain.ErrTransition
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.RefundRecord{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	var resolved pgtype.Timestamptz
	if next == domain.RefundSucceeded || next == domain.RefundFailed || next == domain.RefundDenied {
		resolved = stamp(at.UTC())
	}
	rows, err := q.UpdatePaymentRefundState(ctx, qpay.UpdatePaymentRefundStateParams{
		State: string(next), RemoteRefundID: nullableText(remoteRefundID),
		ResolvedAt: resolved, LastError: lastError, ID: uid,
		PreviousState: string(previous),
	})
	if err != nil {
		return app.RefundRecord{}, dbError(err)
	}
	if rows != 1 {
		return app.RefundRecord{}, domain.ErrConflict
	}
	row, err := q.GetPaymentRefund(ctx, uid)
	if err != nil {
		return app.RefundRecord{}, dbError(err)
	}
	if next == domain.RefundSucceeded {
		if err := s.applySuccessfulRefund(ctx, q, row, at); err != nil {
			return app.RefundRecord{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return app.RefundRecord{}, domain.ErrUnavailable
	}
	return refundRecord(row), nil
}

func (s *Store) applySuccessfulRefund(ctx context.Context, q *qpay.Queries, row qpay.PaymentRefund, now time.Time) error {
	if !row.GrantID.Valid {
		_, err := q.MarkPaymentSettlementRefunded(ctx, qpay.MarkPaymentSettlementRefundedParams{
			State: "PARTIALLY_REFUNDED", ID: row.SettlementID,
		})
		return dbError(err)
	}
	refunds, err := q.ListPaymentRefundsForGrant(ctx, row.GrantID)
	if err != nil {
		return dbError(err)
	}
	var succeeded int64
	for _, refund := range refunds {
		if refund.State == string(domain.RefundSucceeded) {
			succeeded += refund.AmountCentavos
		}
	}
	grant, err := q.GetPaymentGrantForUpdate(ctx, row.GrantID)
	if err != nil {
		return dbError(err)
	}
	state := "PARTIALLY_REFUNDED"
	if succeeded >= grant.PaidCentavos {
		state = "REFUNDED"
		if now.Before(grant.EndAt.Time) {
			if _, err := q.RevokePaymentGrant(ctx, qpay.RevokePaymentGrantParams{
				RevokedAt: stamp(now), OrderID: grant.OrderID,
			}); err != nil {
				return dbError(err)
			}
		}
	}
	if _, err := q.MarkPaymentSettlementRefunded(ctx, qpay.MarkPaymentSettlementRefundedParams{
		State: state, ID: row.SettlementID,
	}); err != nil {
		return dbError(err)
	}
	return nil
}

func refundRecord(row qpay.PaymentRefund) app.RefundRecord {
	record := app.RefundRecord{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		OrderID: uuidText(row.OrderID), GrantID: uuidText(row.GrantID),
		SettlementID: uuidText(row.SettlementID), Provider: domain.Provider(row.Provider),
		Environment: domain.Environment(row.Environment), Merchant: row.Merchant,
		OperationKey: row.OperationKey, RemotePaymentID: row.RemotePaymentID,
		RemoteRefundID: textValue(row.RemoteRefundID), Amount: domain.BRL(row.AmountCentavos),
		State: domain.RefundState(row.State), Reason: row.Reason,
		RequestedBy: row.RequestedBy, ApprovedBy: textValue(row.ApprovedBy),
		RequestedAt: row.RequestedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		LastError: row.LastError,
	}
	if row.ResolvedAt.Valid {
		record.ResolvedAt = row.ResolvedAt.Time
	}
	return record
}

func (s *Store) RecordDispute(ctx context.Context, input app.DisputeRecord) (app.DisputeRecord, bool, error) {
	if s == nil || s.Pool == nil {
		return app.DisputeRecord{}, false, domain.ErrUnavailable
	}
	if err := input.Provider.Validate(); err != nil {
		return app.DisputeRecord{}, false, err
	}
	if err := input.Environment.Validate(); err != nil {
		return app.DisputeRecord{}, false, err
	}
	if !validText(input.RemoteDisputeID) || !input.State.Valid() || input.OpenedAt.IsZero() ||
		input.LastEventAt.IsZero() || (input.RemotePaymentID == "" && input.RemoteChargeID == "") {
		return app.DisputeRecord{}, false, domain.ErrInvalid
	}
	if err := input.Amount.Positive(); err != nil {
		return app.DisputeRecord{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.DisputeRecord{}, false, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	var settlement qpay.PaymentSettlement
	if input.RemotePaymentID != "" {
		settlement, err = q.GetPaymentSettlementByRemote(ctx, qpay.GetPaymentSettlementByRemoteParams{
			Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, RemotePaymentID: input.RemotePaymentID,
		})
	} else {
		settlement, err = q.GetPaymentSettlementByCharge(ctx, qpay.GetPaymentSettlementByChargeParams{
			Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, RemoteChargeID: pgtype.Text{String: input.RemoteChargeID, Valid: true},
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DisputeRecord{}, false, domain.ErrNotFound
	}
	if err != nil {
		return app.DisputeRecord{}, false, dbError(err)
	}
	var grantID pgtype.UUID
	grant, err := q.GetPaymentGrantBySettlement(ctx, settlement.ID)
	if err == nil {
		grantID = grant.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.DisputeRecord{}, false, dbError(err)
	}
	id, err := newID()
	if err != nil {
		return app.DisputeRecord{}, false, domain.ErrUnavailable
	}
	uid, _ := toUUID(id)
	accountID, err := toUUID(uuidText(settlement.AccountID))
	if err != nil {
		return app.DisputeRecord{}, false, domain.ErrInvalid
	}
	row, err := q.InsertPaymentDispute(ctx, qpay.InsertPaymentDisputeParams{
		ID: uid, AccountID: accountID, OfferID: settlement.OfferID, GrantID: grantID,
		SettlementID: settlement.ID, Provider: string(input.Provider),
		Environment: string(input.Environment), Merchant: input.Merchant,
		RemoteDisputeID: input.RemoteDisputeID,
		RemotePaymentID: nullableText(input.RemotePaymentID),
		RemoteChargeID:  nullableText(input.RemoteChargeID),
		AmountCentavos:  input.Amount.Centavos, State: string(input.State),
		OpenedAt: stamp(input.OpenedAt), LastEventAt: stamp(input.LastEventAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := q.GetPaymentDisputeByRemote(ctx, qpay.GetPaymentDisputeByRemoteParams{
			Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, RemoteDisputeID: input.RemoteDisputeID,
		})
		if err != nil {
			return app.DisputeRecord{}, false, dbError(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return app.DisputeRecord{}, false, domain.ErrUnavailable
		}
		return disputeRecord(existing), true, nil
	}
	if err != nil {
		return app.DisputeRecord{}, false, dbError(err)
	}
	if input.State == domain.DisputeLost {
		if err := s.applyLostDispute(ctx, q, row, time.Now().UTC()); err != nil {
			return app.DisputeRecord{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return app.DisputeRecord{}, false, domain.ErrUnavailable
	}
	return disputeRecord(row), false, nil
}

func (s *Store) applyLostDispute(ctx context.Context, q *qpay.Queries, row qpay.PaymentDispute, now time.Time) error {
	if !row.GrantID.Valid {
		if _, err := q.MarkPaymentSettlementReversed(ctx, row.SettlementID); err != nil {
			return dbError(err)
		}
		return nil
	}
	grant, err := q.GetPaymentGrantForUpdate(ctx, row.GrantID)
	if err != nil {
		return dbError(err)
	}
	if grant.State == "ACTIVE" && now.Before(grant.EndAt.Time) {
		if _, err := q.RevokePaymentGrant(ctx, qpay.RevokePaymentGrantParams{
			RevokedAt: stamp(now), OrderID: grant.OrderID,
		}); err != nil {
			return dbError(err)
		}
	}
	if _, err := q.MarkPaymentSettlementReversed(ctx, row.SettlementID); err != nil {
		return dbError(err)
	}
	return nil
}

func (s *Store) UpdateDisputeState(ctx context.Context, id string, previous, next domain.DisputeState, at time.Time) (app.DisputeRecord, error) {
	if s == nil || s.Pool == nil {
		return app.DisputeRecord{}, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil || !previous.Valid() || !next.Valid() {
		return app.DisputeRecord{}, domain.ErrInvalid
	}
	if !domain.DisputeTransitionAllowed(previous, next) {
		return app.DisputeRecord{}, domain.ErrTransition
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.DisputeRecord{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qpay.New(tx)
	var resolved pgtype.Timestamptz
	if next == domain.DisputeWon || next == domain.DisputeLost || next == domain.DisputeClosed {
		resolved = stamp(at.UTC())
	}
	rows, err := q.UpdatePaymentDisputeState(ctx, qpay.UpdatePaymentDisputeStateParams{
		State: string(next), ResolvedAt: resolved, LastEventAt: stamp(at.UTC()),
		ID: uid, PreviousState: string(previous),
	})
	if err != nil {
		return app.DisputeRecord{}, dbError(err)
	}
	if rows != 1 {
		return app.DisputeRecord{}, domain.ErrConflict
	}
	row, err := q.GetPaymentDispute(ctx, uid)
	if err != nil {
		return app.DisputeRecord{}, dbError(err)
	}
	if next == domain.DisputeLost {
		if err := s.applyLostDispute(ctx, q, row, at); err != nil {
			return app.DisputeRecord{}, err
		}
		row, err = q.GetPaymentDispute(ctx, uid)
		if err != nil {
			return app.DisputeRecord{}, dbError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return app.DisputeRecord{}, domain.ErrUnavailable
	}
	return disputeRecord(row), nil
}

func disputeRecord(row qpay.PaymentDispute) app.DisputeRecord {
	record := app.DisputeRecord{
		ID: uuidText(row.ID), AccountID: uuidText(row.AccountID), OfferID: row.OfferID,
		GrantID: uuidText(row.GrantID), SettlementID: uuidText(row.SettlementID),
		Provider: domain.Provider(row.Provider), Environment: domain.Environment(row.Environment),
		Merchant: row.Merchant, RemoteDisputeID: row.RemoteDisputeID,
		RemotePaymentID: textValue(row.RemotePaymentID), RemoteChargeID: textValue(row.RemoteChargeID),
		Amount: domain.BRL(row.AmountCentavos), State: domain.DisputeState(row.State),
		OpenedAt: row.OpenedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		LastEventAt: row.LastEventAt.Time,
	}
	if row.ResolvedAt.Valid {
		record.ResolvedAt = row.ResolvedAt.Time
	}
	return record
}

func (s *Store) RecordProviderHold(ctx context.Context, input app.ProviderHold) (app.ProviderHold, bool, error) {
	if s == nil || s.Pool == nil {
		return app.ProviderHold{}, false, domain.ErrUnavailable
	}
	if err := input.Provider.Validate(); err != nil {
		return app.ProviderHold{}, false, err
	}
	if err := input.Environment.Validate(); err != nil {
		return app.ProviderHold{}, false, err
	}
	if !validText(input.RemoteHoldID) || len(input.Reason) > 256 || input.ObservedAt.IsZero() {
		return app.ProviderHold{}, false, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	id, err := newID()
	if err != nil {
		return app.ProviderHold{}, false, domain.ErrUnavailable
	}
	uid, _ := toUUID(id)
	var amount pgtype.Int8
	if input.AmountKnown {
		if err := input.Amount.Positive(); err != nil {
			return app.ProviderHold{}, false, err
		}
		amount = pgtype.Int8{Int64: input.Amount.Centavos, Valid: true}
	}
	row, err := qpay.New(s.Pool).InsertPaymentProviderHold(ctx, qpay.InsertPaymentProviderHoldParams{
		ID: uid, Provider: string(input.Provider), Environment: string(input.Environment),
		Merchant: input.Merchant, RemoteHoldID: input.RemoteHoldID,
		RemotePaymentID: nullableText(input.RemotePaymentID), AmountCentavos: amount,
		Reason: input.Reason, ObservedAt: stamp(input.ObservedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := qpay.New(s.Pool).GetPaymentProviderHoldByRemote(ctx, qpay.GetPaymentProviderHoldByRemoteParams{
			Provider: string(input.Provider), Environment: string(input.Environment),
			Merchant: input.Merchant, RemoteHoldID: input.RemoteHoldID,
		})
		if err != nil {
			return app.ProviderHold{}, false, dbError(err)
		}
		return holdRecord(existing), true, nil
	}
	if err != nil {
		return app.ProviderHold{}, false, dbError(err)
	}
	return holdRecord(row), false, nil
}

func (s *Store) ReviewProviderHold(ctx context.Context, id, reviewedBy, decision, reviewReason string, at time.Time) (app.ProviderHold, error) {
	if s == nil || s.Pool == nil {
		return app.ProviderHold{}, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil || !validText(reviewedBy) || len(reviewReason) > 256 ||
		(decision != "REVIEWED" && decision != "DISMISSED") {
		return app.ProviderHold{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	rows, err := qpay.New(s.Pool).UpdatePaymentProviderHold(ctx, qpay.UpdatePaymentProviderHoldParams{
		State: decision, ReviewedAt: stamp(at.UTC()), ReviewedBy: nullableText(reviewedBy),
		ReviewReason: reviewReason, ID: uid,
	})
	if err != nil {
		return app.ProviderHold{}, dbError(err)
	}
	if rows != 1 {
		return app.ProviderHold{}, domain.ErrConflict
	}
	row, err := qpay.New(s.Pool).GetPaymentProviderHold(ctx, uid)
	if err != nil {
		return app.ProviderHold{}, dbError(err)
	}
	return holdRecord(row), nil
}

func holdRecord(row qpay.PaymentProviderHold) app.ProviderHold {
	hold := app.ProviderHold{
		ID: uuidText(row.ID), Provider: domain.Provider(row.Provider),
		Environment: domain.Environment(row.Environment), Merchant: row.Merchant,
		RemoteHoldID: row.RemoteHoldID, RemotePaymentID: textValue(row.RemotePaymentID),
		Reason: row.Reason, State: row.State, ObservedAt: row.ObservedAt.Time,
		ReviewedBy: textValue(row.ReviewedBy), ReviewReason: row.ReviewReason,
	}
	if row.AmountCentavos.Valid {
		hold.Amount = domain.BRL(row.AmountCentavos.Int64)
		hold.AmountKnown = true
	}
	if row.ReviewedAt.Valid {
		hold.ReviewedAt = row.ReviewedAt.Time
	}
	return hold
}

func (s *Store) RecordOperatorAction(ctx context.Context, input app.OperatorAction) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	if !validText(input.Actor) || !validText(input.Action) || !validText(input.TargetID) ||
		(input.AccountID != "" && !validText(input.AccountID)) || !validReason(input.Reason, 256) || !validText(input.Result) ||
		input.CreatedAt.IsZero() {
		return domain.ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return domain.ErrUnavailable
	}
	uid, _ := toUUID(id)
	target, err := toUUID(input.TargetID)
	if err != nil {
		return domain.ErrInvalid
	}
	var account pgtype.UUID
	if input.AccountID != "" {
		account, err = toUUID(input.AccountID)
		if err != nil {
			return domain.ErrInvalid
		}
	}
	var amount pgtype.Int8
	if input.HasAmount {
		if err := input.Amount.Positive(); err != nil {
			return domain.ErrInvalid
		}
		amount = pgtype.Int8{Int64: input.Amount.Centavos, Valid: true}
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return dbError(qpay.New(s.Pool).InsertPaymentOperatorAction(ctx, qpay.InsertPaymentOperatorActionParams{
		ID: uid, Actor: input.Actor, Action: input.Action, TargetID: target,
		AccountID: account, Reason: input.Reason, AmountCentavos: amount, Result: input.Result,
	}))
}

func (s *Store) GetDisputeByRemote(ctx context.Context, provider domain.Provider, environment domain.Environment, merchant, remoteDisputeID string) (app.DisputeRecord, bool, error) {
	if s == nil || s.Pool == nil {
		return app.DisputeRecord{}, false, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentDisputeByRemote(ctx, qpay.GetPaymentDisputeByRemoteParams{
		Provider: string(provider), Environment: string(environment),
		Merchant: merchant, RemoteDisputeID: remoteDisputeID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DisputeRecord{}, false, nil
	}
	if err != nil {
		return app.DisputeRecord{}, false, dbError(err)
	}
	return disputeRecord(row), true, nil
}

func (s *Store) GetProviderHold(ctx context.Context, id string) (app.ProviderHold, error) {
	if s == nil || s.Pool == nil {
		return app.ProviderHold{}, domain.ErrUnavailable
	}
	uid, err := toUUID(id)
	if err != nil {
		return app.ProviderHold{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	row, err := qpay.New(s.Pool).GetPaymentProviderHold(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ProviderHold{}, domain.ErrNotFound
	}
	if err != nil {
		return app.ProviderHold{}, dbError(err)
	}
	return holdRecord(row), nil
}

func validReason(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}
