package adapters

import (
	"context"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaintenanceHolds implements fleetapp.MaintenanceHoldPort over org_work_orders
// (EMP-MAINT-005). When running against a pre-000259 schema it returns Sourced=false.
type MaintenanceHolds struct{}

var _ fleetapp.MaintenanceHoldPort = MaintenanceHolds{}

func (MaintenanceHolds) MaintenanceHoldsDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string, at time.Time) (fleetapp.HoldAnswer, error) {
	db = syncapp.Borrow(db)
	if db == nil || scope.Validate() != nil || !validMaintenanceID(asset) || at.IsZero() || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return fleetapp.HoldAnswer{}, fleetapp.ErrUnavailable
	}
	var installed bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('public.org_work_orders') IS NOT NULL`).Scan(&installed); err != nil {
		return fleetapp.HoldAnswer{}, fleetapp.ErrUnavailable
	}
	if !installed {
		return fleetapp.HoldAnswer{}, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id::text, hold_started_at, hold_ended_at
		FROM org_work_orders
		WHERE org_id = $1::uuid
		  AND asset_id = $2::uuid
		  AND hold_started_at IS NOT NULL
		  AND hold_started_at <= $3::timestamptz
		  AND (hold_ended_at IS NULL OR $3::timestamptz < hold_ended_at)
		ORDER BY hold_started_at, id`,
		scope.OrgID().String(), asset, at.UTC(),
	)
	if err != nil {
		return fleetapp.HoldAnswer{}, fleetapp.ErrUnavailable
	}
	defer rows.Close()
	out := fleetapp.HoldAnswer{Sourced: true}
	for rows.Next() {
		var id string
		var from pgtype.Timestamptz
		var to pgtype.Timestamptz
		if err := rows.Scan(&id, &from, &to); err != nil || !from.Valid {
			return fleetapp.HoldAnswer{}, fleetapp.ErrUnavailable
		}
		fact := fleet.HoldFact{ID: id, From: from.Time.UTC()}
		if to.Valid {
			ended := to.Time.UTC()
			fact.To = &ended
		}
		out.Holds = append(out.Holds, fact)
	}
	if err := rows.Err(); err != nil {
		return fleetapp.HoldAnswer{}, fleetapp.ErrUnavailable
	}
	return out, nil
}

func validMaintenanceID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}
