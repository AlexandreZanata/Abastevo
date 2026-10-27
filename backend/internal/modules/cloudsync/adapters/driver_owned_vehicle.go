package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RequireDriverOwnedVehicle implements the transaction-bound ownership port.
// Empty vehicle means a retained-data account read/erase, not a paid mutation.
// Callers still own signed proof, capability and dataset-consent admission.
func RequireDriverOwnedVehicle(ctx context.Context, tx pgx.Tx, owner, vehicle string) (bool, error) {
	var accountID pgtype.UUID
	if e := accountID.Scan(owner); e != nil || !accountID.Valid {
		return false, nil
	}
	query := q.New(tx)
	if _, e := query.LockDriverActiveAccount(ctx, accountID); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return false, nil
		}
		return false, e
	}
	if vehicle == "" {
		return true, nil
	}
	var vehicleID pgtype.UUID
	if e := vehicleID.Scan(vehicle); e != nil || !vehicleID.Valid {
		return false, nil
	}
	_, e := query.LockDriverOwnedVehicle(ctx, q.LockDriverOwnedVehicleParams{AccountID: accountID, ID: vehicleID})
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	return e == nil, e
}
