package adapters

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"time"
)

// ConsumptionRows is an owner-private bounded projection, separate from public paging.
func (s *Store) ConsumptionRows(ctx context.Context, accountID, vehicleID, from, to string, limit int) ([]application.ConsumptionRecord, error) {
	if !validLedgerID(vehicleID) || limit < 1 || limit > 1000 {
		return nil, errInvalid()
	}
	start, e := time.Parse(time.RFC3339, from)
	if e != nil {
		return nil, errInvalid()
	}
	end, e := time.Parse(time.RFC3339, to)
	if e != nil || !end.After(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, errUnavailable()
	}
	defer rollback(tx)
	if e = lockLedgerOwner(ctx, tx, accountID); e != nil {
		return nil, e
	}
	rows, e := q.New(tx).MetricRows(ctx, q.MetricRowsParams{AccountID: uid(accountID), VehicleID: uid(vehicleID), FromTime: pgTimestamptz(start), ToTime: pgTimestamptz(end), RowLimit: int32(limit + 1)})
	if e != nil {
		return nil, errUnavailable()
	}
	if len(rows) > limit {
		return nil, errInvalid()
	}
	out := make([]application.ConsumptionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.ConsumptionRecord{ID: ledgerIDString(r.ID), VehicleID: ledgerIDString(r.VehicleID), Payload: r.Payload, SegmentNo: r.SegmentNo, HasGap: r.HasGap})
	}
	return out, nil
}
