package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func ledgerIDString(id pgtype.UUID) string {
	s := hex.EncodeToString(id.Bytes[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

func (s *Store) OpenSegment(ctx context.Context, accountID, vehicleID, reason string) (int64, error) {
	if !validLedgerID(vehicleID) || (reason != "reset" && reason != "replacement") {
		return 0, errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = lockLedgerOwner(ctx, tx, accountID); err != nil {
		return 0, err
	}
	if _, err = query.LedgerVehicle(ctx, q.LedgerVehicleParams{AccountID: uid(accountID), ID: uid(vehicleID)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errInvalid()
		}
		return 0, errUnavailable()
	}
	current, err := query.MaxSegmentNo(ctx, q.MaxSegmentNoParams{AccountID: uid(accountID), VehicleID: uid(vehicleID)})
	if err != nil {
		return 0, errUnavailable()
	}
	next, err := query.OpenSegment(ctx, q.OpenSegmentParams{AccountID: uid(accountID), VehicleID: uid(vehicleID), SegmentNo: current + 1, Reason: reason})
	if err != nil {
		return 0, errConflict()
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, errUnavailable()
	}
	return next, nil
}

func (s *Store) DeclareGap(ctx context.Context, accountID, vehicleID, gapID, afterID, beforeID, note string) (string, error) {
	if !validLedgerID(vehicleID) || !validLedgerID(gapID) {
		return "", errInvalid()
	}
	if len(note) > 280 || !validGapNote(note) {
		return "", errInvalid()
	}
	for _, id := range []string{afterID, beforeID} {
		if id != "" && !validLedgerID(id) {
			return "", errInvalid()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = lockLedgerOwner(ctx, tx, accountID); err != nil {
		return "", err
	}
	if _, err = query.LedgerVehicle(ctx, q.LedgerVehicleParams{AccountID: uid(accountID), ID: uid(vehicleID)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errInvalid()
		}
		return "", errUnavailable()
	}
	for _, id := range []string{afterID, beforeID} {
		if id == "" {
			continue
		}
		row, err := query.GetLedgerRecord(ctx, q.GetLedgerRecordParams{AccountID: uid(accountID), Dataset: "refuel", ID: uid(id)})
		if err != nil {
			return "", errInvalid()
		}
		if ledgerIDString(row.VehicleID) != vehicleID {
			return "", errInvalid()
		}
	}
	returned, err := query.DeclareGap(ctx, q.DeclareGapParams{AccountID: uid(accountID), GapID: uid(gapID), VehicleID: uid(vehicleID), AfterID: uid(afterID), BeforeID: uid(beforeID), Note: note})
	if err != nil {
		return "", errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return "", errUnavailable()
	}
	return ledgerIDString(returned), nil
}

func validGapNote(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return false
		}
	}
	return true
}
