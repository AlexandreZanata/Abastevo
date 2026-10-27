package adapters

import (
	"context"
	"strconv"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/jackc/pgx/v5"
)

// RequireDriverLedgerCost is the transaction-bound ledger port for DRV-09.
// It locks the authoritative ledger row for the duration of the driver
// transaction and returns its owner-entered amount. A missing/ambiguous row or
// a stale/deleted/voided revision is an explicit refusal; the driver never
// writes ledger_records.
func RequireDriverLedgerCost(ctx context.Context, tx pgx.Tx, owner, vehicle, sourceID string, revision int64) (*int64, string, error) {
	if !validLedgerID(owner) || !validLedgerID(vehicle) || !validLedgerID(sourceID) || revision < 1 {
		return nil, "not_found", nil
	}
	rows, err := q.New(tx).LockDriverLedgerCosts(ctx, q.LockDriverLedgerCostsParams{AccountID: uid(owner), ID: uid(sourceID)})
	if err != nil {
		return nil, "error", err
	}
	if len(rows) == 0 {
		return nil, "not_found", nil
	}
	if len(rows) != 1 {
		return nil, "conflict", nil
	}
	row := rows[0]
	if row.Deleted || row.Voided || row.Revision != revision || ledgerIDString(row.VehicleID) != vehicle {
		return nil, "conflict", nil
	}
	if row.AmountText == "" {
		return nil, "ok", nil
	}
	amount, err := strconv.ParseInt(row.AmountText, 10, 64)
	if err != nil || amount < 0 || amount > 1_000_000_000_000 {
		return nil, "conflict", nil
	}
	return &amount, "ok", nil
}
