package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	ledgerdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func uid(s string) pgtype.UUID { var id pgtype.UUID; _ = id.Scan(s); return id }

func rollback(tx pgx.Tx) { _ = tx.Rollback(context.Background()) }

func parsePayload(dataset string, raw json.RawMessage) (vehicleID string, occurredAt time.Time, odometer *int64, canonical []byte, err error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", time.Time{}, nil, nil, errInvalid()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dataset == "refuel" {
		var wire struct {
			ID             string `json:"id"`
			VehicleID      string `json:"vehicle_id"`
			OccurredAt     string `json:"occurred_at"`
			Timezone       string `json:"timezone"`
			Fuel           string `json:"fuel"`
			Unit           string `json:"unit"`
			QuantityMilli  int64  `json:"quantity_milli"`
			TotalCentavos  int64  `json:"total_centavos"`
			UnitPriceMilli int64  `json:"unit_price_milli"`
			OdometerMetres *int64 `json:"odometer_metres"`
			Fill           string `json:"fill"`
			StationID      string `json:"station_id"`
			Note           string `json:"note"`
		}
		if err := dec.Decode(&wire); err != nil {
			return "", time.Time{}, nil, nil, errInvalid()
		}
		if _, err := dec.Token(); err == nil {
			return "", time.Time{}, nil, nil, errInvalid()
		}
		at, err := time.Parse(time.RFC3339, wire.OccurredAt)
		if err != nil {
			return "", time.Time{}, nil, nil, errInvalid()
		}
		r := ledgerdomain.Refuel{ID: wire.ID, VehicleID: wire.VehicleID, OccurredAt: at, Timezone: wire.Timezone, Fuel: kernelProduct(wire.Fuel), Unit: kernelUnit(wire.Unit), QuantityMilli: wire.QuantityMilli, TotalCentavos: wire.TotalCentavos, UnitPriceMilli: wire.UnitPriceMilli, OdometerMetres: wire.OdometerMetres, Fill: ledgerdomain.Fill(wire.Fill), StationID: wire.StationID, Note: wire.Note}
		normalized, err := ledgerdomain.NormalizeRefuel(r)
		if err != nil {
			return "", time.Time{}, nil, nil, err
		}
		canonical, _ = json.Marshal(normalized)
		return normalized.VehicleID, normalized.OccurredAt, normalized.OdometerMetres, canonical, nil
	}
	var wire struct {
		ID             string `json:"id"`
		VehicleID      string `json:"vehicle_id"`
		OccurredAt     string `json:"occurred_at"`
		Timezone       string `json:"timezone"`
		AmountCentavos int64  `json:"amount_centavos"`
		Category       string `json:"category"`
		OdometerMetres *int64 `json:"odometer_metres"`
		Note           string `json:"note"`
	}
	if err := dec.Decode(&wire); err != nil {
		return "", time.Time{}, nil, nil, errInvalid()
	}
	if _, err := dec.Token(); err == nil {
		return "", time.Time{}, nil, nil, errInvalid()
	}
	at, err := time.Parse(time.RFC3339, wire.OccurredAt)
	if err != nil {
		return "", time.Time{}, nil, nil, errInvalid()
	}
	e := ledgerdomain.Expense{ID: wire.ID, VehicleID: wire.VehicleID, OccurredAt: at, Timezone: wire.Timezone, AmountCentavos: wire.AmountCentavos, Category: ledgerdomain.Category(wire.Category), OdometerMetres: wire.OdometerMetres, Note: wire.Note}
	normalized, err := ledgerdomain.NormalizeExpense(e)
	if err != nil {
		return "", time.Time{}, nil, nil, err
	}
	canonical, _ = json.Marshal(normalized)
	return normalized.VehicleID, normalized.OccurredAt, normalized.OdometerMetres, canonical, nil
}

func (s *Store) Apply(ctx context.Context, accountID string, batch application.Batch) (application.BatchResult, error) {
	if len(batch.Commands) < 1 || len(batch.Commands) > 50 {
		return application.BatchResult{}, errInvalid()
	}
	raw, _ := json.Marshal(batch)
	if len(raw) > 65536 {
		return application.BatchResult{}, errInvalid()
	}
	for _, c := range batch.Commands {
		if !validLedgerID(c.OperationID) || !validLedgerID(c.ID) || c.ExpectedRevision < 0 || (c.Dataset != "refuel" && c.Dataset != "expense") || (c.Delete && c.Void) {
			return application.BatchResult{}, errInvalid()
		}
		if c.Delete || c.Void {
			if len(c.Payload) > 0 && string(bytes.TrimSpace(c.Payload)) != "null" && len(bytes.TrimSpace(c.Payload)) > 0 {
				trimmed := bytes.TrimSpace(c.Payload)
				if len(trimmed) > 0 && string(trimmed) != "null" {
					return application.BatchResult{}, errInvalid()
				}
			}
		} else if len(bytes.TrimSpace(c.Payload)) == 0 {
			return application.BatchResult{}, errInvalid()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.BatchResult{}, errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	result := application.BatchResult{Results: []application.CommandResult{}}
	for _, c := range batch.Commands {
		hashInput, _ := json.Marshal(struct {
			Account string
			Command application.Command
		}{accountID, c})
		sum := sha256.Sum256(hashInput)
		hash := hex.EncodeToString(sum[:])
		old, err := query.GetLedgerOperation(ctx, q.GetLedgerOperationParams{AccountID: uid(accountID), OperationID: uid(c.OperationID)})
		if err == nil {
			if old.PayloadHash != hash {
				return application.BatchResult{}, errConflict()
			}
			var receipt application.CommandResult
			if json.Unmarshal(old.Receipt, &receipt) != nil {
				return application.BatchResult{}, errUnavailable()
			}
			receipt.Replayed = true
			result.Results = append(result.Results, receipt)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return application.BatchResult{}, errUnavailable()
		}
		revision, err := s.applyOne(ctx, query, accountID, c)
		if err != nil {
			return application.BatchResult{}, err
		}
		receipt := application.CommandResult{OperationID: c.OperationID, Dataset: c.Dataset, ID: c.ID, Revision: revision, Status: "accepted"}
		receiptBytes, _ := json.Marshal(receipt)
		if err = query.SaveLedgerOperation(ctx, q.SaveLedgerOperationParams{AccountID: uid(accountID), OperationID: uid(c.OperationID), PayloadHash: hash, Receipt: receiptBytes}); err != nil {
			return application.BatchResult{}, errUnavailable()
		}
		result.Results = append(result.Results, receipt)
	}
	if err = tx.Commit(ctx); err != nil {
		return application.BatchResult{}, errUnavailable()
	}
	return result, nil
}

func (s *Store) applyOne(ctx context.Context, query *q.Queries, accountID string, c application.Command) (int64, error) {
	var vehicleID string
	var occurredAt time.Time
	var odometer *int64
	var canonical []byte
	if !c.Delete && !c.Void {
		var err error
		vehicleID, occurredAt, odometer, canonical, err = parsePayload(c.Dataset, c.Payload)
		if err != nil {
			if errors.Is(err, errInvalidSentinel()) {
				return 0, errInvalid()
			}
			return 0, err
		}
		if _, err = query.LedgerVehicle(ctx, q.LedgerVehicleParams{AccountID: uid(accountID), ID: uid(vehicleID)}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, errInvalid()
			}
			return 0, errUnavailable()
		}
	}
	old, err := query.GetLedgerRecord(ctx, q.GetLedgerRecordParams{AccountID: uid(accountID), Dataset: c.Dataset, ID: uid(c.ID)})
	if err == nil {
		if old.Deleted || old.Revision != c.ExpectedRevision {
			return 0, errConflict()
		}
		if c.Delete {
			revision, err := query.DeleteLedgerRecord(ctx, q.DeleteLedgerRecordParams{AccountID: uid(accountID), Dataset: c.Dataset, ID: uid(c.ID)})
			if err != nil {
				return 0, errUnavailable()
			}
			return revision, nil
		}
		if c.Void {
			if old.Voided {
				return 0, errConflict()
			}
			revision, err := query.VoidLedgerRecord(ctx, q.VoidLedgerRecordParams{AccountID: uid(accountID), Dataset: c.Dataset, ID: uid(c.ID)})
			if err != nil {
				return 0, errUnavailable()
			}
			return revision, nil
		}
		if odometer != nil {
			floor, err := query.MaxSegmentOdometerExcluding(ctx, q.MaxSegmentOdometerExcludingParams{AccountID: uid(accountID), VehicleID: uid(vehicleID), SegmentNo: old.SegmentNo, ID: uid(c.ID)})
			if err != nil {
				return 0, errUnavailable()
			}
			if *odometer < floor {
				return 0, errInvalid()
			}
		}
		row, err := query.SaveLedgerRecord(ctx, q.SaveLedgerRecordParams{AccountID: uid(accountID), Dataset: c.Dataset, ID: uid(c.ID), VehicleID: uid(vehicleID), OccurredAt: pgTimestamptz(occurredAt), Payload: canonical, SegmentNo: old.SegmentNo})
		if err != nil {
			return 0, errUnavailable()
		}
		return row.Revision, nil
	} else if errors.Is(err, pgx.ErrNoRows) {
		if c.ExpectedRevision != 0 || c.Delete || c.Void {
			return 0, errConflict()
		}
		segment, err := query.MaxSegmentNo(ctx, q.MaxSegmentNoParams{AccountID: uid(accountID), VehicleID: uid(vehicleID)})
		if err != nil {
			return 0, errUnavailable()
		}
		if odometer != nil {
			floor, err := query.MaxSegmentOdometer(ctx, q.MaxSegmentOdometerParams{AccountID: uid(accountID), VehicleID: uid(vehicleID), SegmentNo: segment})
			if err != nil {
				return 0, errUnavailable()
			}
			if *odometer < floor {
				return 0, errInvalid()
			}
		}
		row, err := query.SaveLedgerRecord(ctx, q.SaveLedgerRecordParams{AccountID: uid(accountID), Dataset: c.Dataset, ID: uid(c.ID), VehicleID: uid(vehicleID), OccurredAt: pgTimestamptz(occurredAt), Payload: canonical, SegmentNo: segment})
		if err != nil {
			return 0, errUnavailable()
		}
		return row.Revision, nil
	}
	return 0, errUnavailable()
}

var _ application.LedgerStore = (*Store)(nil)
