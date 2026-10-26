package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ObservationStore struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s ObservationStore) Submit(ctx context.Context, owner string, observation chargingdomain.Observation) (string, error) {
	_, err := s.command(ctx, "SUBMIT", owner, observation.ID, observation, 0)
	if err != nil {
		return "", err
	}
	return observation.ID, nil
}
func (s ObservationStore) Correct(ctx context.Context, owner string, observation chargingdomain.Observation, expected int64) (int64, error) {
	return s.CorrectCommand(ctx, owner, observation, expected, "")
}
func (s ObservationStore) CorrectCommand(ctx context.Context, owner string, observation chargingdomain.Observation, expected int64, operationID string) (int64, error) {
	return s.command(ctx, "CORRECT", owner, operationID, observation, expected)
}
func (s ObservationStore) Withdraw(ctx context.Context, owner, id string, expected int64) (int64, error) {
	return s.WithdrawCommand(ctx, owner, id, expected, "")
}
func (s ObservationStore) WithdrawCommand(ctx context.Context, owner, id string, expected int64, operationID string) (int64, error) {
	return s.command(ctx, "WITHDRAW", owner, operationID, chargingdomain.Observation{ID: id}, expected)
}

// An owner lock covers quota, row revision and operation receipt as one unit.
// Moderation may hide independently: none of these updates clears hidden_at.
func (s ObservationStore) command(ctx context.Context, kind, owner, operationID string, o chargingdomain.Observation, expected int64) (int64, error) {
	account, err := pgUUID(owner)
	if err != nil {
		return 0, chargingdomain.ErrNotOwner
	}
	target, err := pgUUID(o.ID)
	if err != nil {
		return 0, chargingdomain.ErrInvalidObservation
	}
	var operation pgtype.UUID
	if operationID != "" {
		operation, err = pgUUID(operationID)
		if err != nil {
			return 0, chargingdomain.ErrInvalidObservation
		}
	}
	if kind != "SUBMIT" && expected <= 0 {
		return 0, chargingdomain.ErrConflict
	}
	digest, err := observationCommandDigest(kind, o, expected)
	if err != nil {
		return 0, chargingdomain.ErrInvalidObservation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "charging-owner:"+owner); err != nil {
		return 0, err
	}
	q := charging.New(tx)
	if operation.Valid {
		receipt, e := q.ChargingCommandReceipt(ctx, charging.ChargingCommandReceiptParams{AccountID: account, OperationID: operation})
		if e == nil {
			if receipt.Kind != kind || receipt.TargetID != target || receipt.RequestSha256 != digest {
				return 0, chargingdomain.ErrConflict
			}
			return receipt.ResultRevision, tx.Commit(ctx)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return 0, e
		}
	}
	existing, findErr := q.ChargingObservationByID(ctx, target)
	if findErr != nil && !errors.Is(findErr, pgx.ErrNoRows) {
		return 0, findErr
	}
	if findErr == nil && existing.AccountID != account {
		return 0, chargingdomain.ErrNotOwner
	}
	if kind == "SUBMIT" && findErr == nil {
		previous := observationFromRow(existing)
		previousDigest, e := observationCommandDigest(kind, previous, 0)
		if e != nil || previousDigest != digest {
			return 0, chargingdomain.ErrConflict
		}
		// Legacy identical creates can gain a receipt; a deleted row stays deleted.
		if operation.Valid {
			err = q.RecordChargingCommand(ctx, charging.RecordChargingCommandParams{AccountID: account, OperationID: operation, TargetID: target, Kind: kind, RequestSha256: digest, ResultRevision: existing.Revision})
			if err != nil {
				return 0, err
			}
		}
		return existing.Revision, tx.Commit(ctx)
	}
	if kind != "SUBMIT" {
		if findErr != nil || existing.Deleted || existing.Revision != expected {
			return 0, chargingdomain.ErrConflict
		}
		if o.PlaceID != nil && (!existing.PlaceID.Valid || *o.PlaceID != hexUUID(existing.PlaceID)) {
			return 0, chargingdomain.ErrInvalidObservation
		}
		if existing.PlaceID.Valid {
			place := hexUUID(existing.PlaceID)
			o.PlaceID = &place
		}
		// Old clients have no explicit operational field/command identity. Do not
		// silently erase a new event when an old client edits a legacy price form.
		if !operation.Valid && o.Operational == nil {
			o.Operational = operationalFromColumns(existing.OperationalEvent, existing.OperationalScope, existing.OperationalGroupID, existing.OperationalObservedAt)
		}
	}
	var price pgtype.Int4
	var day pgtype.Date
	var place pgtype.UUID
	var ev, scope pgtype.Text
	var group pgtype.UUID
	var at pgtype.Timestamptz
	if kind != "WITHDRAW" {
		if err = chargingdomain.ValidateObservation(o); err != nil {
			return 0, err
		}
		now := time.Now()
		if s.Now != nil {
			now = s.Now()
		}
		if err = chargingdomain.ValidateOperationalEvent(o.Operational, o.PlaceID, now); err != nil {
			return 0, err
		}
		day, err = pgDate(o.ObservedDay)
		if err != nil {
			return 0, chargingdomain.ErrInvalidObservation
		}
		if o.PriceMilliBRL != nil {
			price = pgtype.Int4{Int32: int32(*o.PriceMilliBRL), Valid: true}
		}
		if o.PlaceID != nil {
			place, err = pgUUID(*o.PlaceID)
			if err != nil {
				return 0, chargingdomain.ErrInvalidObservation
			}
		}
		if o.Operational != nil {
			ev = pgtype.Text{String: o.Operational.Event, Valid: true}
			scope = pgtype.Text{String: o.Operational.TargetScope, Valid: true}
			parsed, _ := time.Parse(time.RFC3339Nano, o.Operational.ObservedAt)
			at = pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}
			if o.Operational.ConnectionGroupID != nil {
				group, err = pgUUID(*o.Operational.ConnectionGroupID)
				if err != nil {
					return 0, chargingdomain.ErrInvalidObservation
				}
				ref, e := q.OperationalChargingGroup(ctx, group)
				if e != nil {
					if errors.Is(e, pgx.ErrNoRows) {
						return 0, chargingdomain.ErrInvalidObservation
					}
					return 0, e
				}
				if ref.PlaceID != place || ref.ConnectorCode != o.ConnectorCode || (scope.String == "EQUIPMENT" && (!ref.ReportedQuantity.Valid || ref.ReportedQuantity.Int32 != 1)) {
					return 0, chargingdomain.ErrInvalidObservation
				}
			}
		}
	}
	var revision int64
	switch kind {
	case "SUBMIT":
		count, e := q.CountAccountObservations(ctx, account)
		if e != nil {
			return 0, e
		}
		if count >= chargingdomain.MaxObservationsPerAccount {
			return 0, chargingdomain.ErrQuotaExceeded
		}
		row, e := q.CreateChargingObservation(ctx, charging.CreateChargingObservationParams{ID: target, AccountID: account, PlaceID: place, ConnectorCode: o.ConnectorCode, Price: price, Condition: o.Condition, ObservedDay: day, Note: o.Note, OperationalEvent: ev, OperationalScope: scope, OperationalGroupID: group, OperationalObservedAt: at})
		if e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return 0, chargingdomain.ErrConflict
			}
			return 0, observationDBError(e)
		}
		revision = row.Revision
	case "CORRECT":
		revision, err = q.CorrectChargingObservation(ctx, charging.CorrectChargingObservationParams{AccountID: account, ID: target, Expected: expected, ConnectorCode: o.ConnectorCode, Price: price, Condition: o.Condition, ObservedDay: day, Note: o.Note, OperationalEvent: ev, OperationalScope: scope, OperationalGroupID: group, OperationalObservedAt: at})
	case "WITHDRAW":
		revision, err = q.WithdrawChargingObservation(ctx, charging.WithdrawChargingObservationParams{AccountID: account, ID: target, Expected: expected})
	default:
		return 0, chargingdomain.ErrInvalidObservation
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, chargingdomain.ErrConflict
		}
		return 0, observationDBError(err)
	}
	if operation.Valid {
		if err = q.RecordChargingCommand(ctx, charging.RecordChargingCommandParams{AccountID: account, OperationID: operation, TargetID: target, Kind: kind, RequestSha256: digest, ResultRevision: revision}); err != nil {
			return 0, err
		}
	}
	return revision, tx.Commit(ctx)
}

func observationCommandDigest(kind string, o chargingdomain.Observation, expected int64) (string, error) {
	// Enumerate v1 fields so adding unrelated model fields cannot change receipts.
	var event *chargingdomain.OperationalEvent
	if o.Operational != nil {
		copy := *o.Operational
		if at, e := time.Parse(time.RFC3339Nano, copy.ObservedAt); e == nil {
			copy.ObservedAt = at.UTC().Format(time.RFC3339Nano)
		}
		event = &copy
	}
	body := struct {
		Version              int
		Kind, ID             string
		Place                *string
		Connector            string
		Price                *int64
		Condition, Day, Note string
		Event                *chargingdomain.OperationalEvent
		Expected             int64
	}{1, kind, o.ID, o.PlaceID, o.ConnectorCode, o.PriceMilliBRL, o.Condition, o.ObservedDay, o.Note, event, expected}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func operationalFromColumns(event, scope pgtype.Text, group pgtype.UUID, at pgtype.Timestamptz) *chargingdomain.OperationalEvent {
	if !event.Valid {
		return nil
	}
	op := &chargingdomain.OperationalEvent{Event: event.String, TargetScope: scope.String, ObservedAt: at.Time.UTC().Format(time.RFC3339Nano)}
	if group.Valid {
		id := hexUUID(group)
		op.ConnectionGroupID = &id
	}
	return op
}
func observationFromRow(row charging.ChargingObservation) chargingdomain.Observation {
	o := chargingdomain.Observation{ID: hexUUID(row.ID), ConnectorCode: row.ConnectorCode, Condition: row.Condition, ObservedDay: row.ObservedDay.Time.Format("2006-01-02"), Note: row.Note, Operational: operationalFromColumns(row.OperationalEvent, row.OperationalScope, row.OperationalGroupID, row.OperationalObservedAt)}
	if row.PlaceID.Valid {
		id := hexUUID(row.PlaceID)
		o.PlaceID = &id
	}
	if row.PriceMilliBrlPerKwh.Valid {
		price := int64(row.PriceMilliBrlPerKwh.Int32)
		o.PriceMilliBRL = &price
	}
	return o
}
func observationDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23514", "22007", "22008":
			return chargingdomain.ErrInvalidObservation
		case "23505":
			return chargingdomain.ErrConflict
		}
	}
	return err
}

func pgUUID(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(text); err != nil || !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID")
	}
	return id, nil
}

func pgDate(day string) (pgtype.Date, error) {
	var date pgtype.Date
	if err := date.Scan(day); err != nil || !date.Valid {
		return pgtype.Date{}, fmt.Errorf("invalid day")
	}
	return date, nil
}
