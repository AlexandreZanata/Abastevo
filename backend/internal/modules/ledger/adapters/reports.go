package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func monthBounds(year, month int, timezone string) (time.Time, time.Time, error) {
	if year < 2000 || year > 2100 || month < 1 || month > 12 {
		return time.Time{}, time.Time{}, errInvalid()
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil || len(timezone) == 0 || len(timezone) > 64 {
		return time.Time{}, time.Time{}, errInvalid()
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0), nil
}

func (s *Store) BuildReport(ctx context.Context, accountID string, year, month int, timezone string) (application.Report, error) {
	start, end, err := monthBounds(year, month, timezone)
	if err != nil {
		return application.Report{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	defer rollback(tx)
	if s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return application.Report{}, err
		}
	}
	query := q.New(tx)
	lockKey := fmt.Sprintf("%s|%d|%d|%s", accountID, year, month, timezone)
	if _, err = query.LockReport(ctx, lockKey); err != nil {
		return application.Report{}, errUnavailable()
	}
	base, err := query.BaseLedgerUpdated(ctx, uid(accountID))
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	sameBase := func(a, b pgtype.Timestamptz) bool {
		if !a.Valid && !b.Valid {
			return true
		}
		return a.Valid && b.Valid && a.Time.Equal(b.Time)
	}
	if existing, err := query.GetReport(ctx, q.GetReportParams{AccountID: uid(accountID), Year: int32(year), Month: int32(month), Timezone: timezone}); err == nil {
		if sameBase(existing.BaseUpdatedAt, base) {
			return decodeReport(year, month, timezone, existing.Totals, existing.Revision, existing.GeneratedAt.Time)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return application.Report{}, errUnavailable()
	}
	rows, err := query.ReportRows(ctx, q.ReportRowsParams{AccountID: uid(accountID), OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end)})
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	totals := application.ReportTotals{ByVehicle: map[string]int64{}, ByCategory: map[string]int64{}, Coverage: map[string]string{}}
	byVehicleFills := map[string][]metrics.Fill{}
	byVehicleOdo := map[string][]int64{}
	for _, row := range rows {
		var payload struct {
			TotalCentavos  int64  `json:"total_centavos"`
			AmountCentavos int64  `json:"amount_centavos"`
			VehicleID      string `json:"vehicle_id"`
			Category       string `json:"category"`
			Fuel           string `json:"fuel"`
			Unit           string `json:"unit"`
			QuantityMilli  int64  `json:"quantity_milli"`
			Fill           string `json:"fill"`
			OdometerMetres *int64 `json:"odometer_metres"`
		}
		if json.Unmarshal(row.Payload, &payload) != nil {
			return application.Report{}, errUnavailable()
		}
		vehicle := ledgerIDString(row.VehicleID)
		totals.RecordCount++
		if row.Dataset == "refuel" {
			totals.PurchaseCentavos += payload.TotalCentavos
			totals.ByVehicle[vehicle] += payload.TotalCentavos
			byVehicleFills[vehicle] = append(byVehicleFills[vehicle], metrics.Fill{QuantityMilli: payload.QuantityMilli, TotalCentavos: payload.TotalCentavos, Full: payload.Fill == "full", Fuel: payload.Fuel, Unit: payload.Unit})
			if payload.OdometerMetres != nil {
				byVehicleOdo[vehicle] = append(byVehicleOdo[vehicle], *payload.OdometerMetres)
			}
		} else {
			totals.ExpenseCentavos += payload.AmountCentavos
			totals.ByVehicle[vehicle] += payload.AmountCentavos
			totals.ByCategory[payload.Category]++
		}
	}
	for vehicle, fills := range byVehicleFills {
		fulls := 0
		for _, f := range fills {
			if f.Full {
				fulls++
			}
		}
		if fulls < 2 {
			totals.Coverage[vehicle] = "full-anchors-required"
			continue
		}
		odos := byVehicleOdo[vehicle]
		if len(odos) < 2 {
			totals.Coverage[vehicle] = "odometer-missing"
			continue
		}
		distance := odos[len(odos)-1] - odos[0]
		if distance <= 0 {
			totals.Coverage[vehicle] = "distance-nonpositive"
			continue
		}
		in := metrics.Interval{Fills: fills[1:], DistanceMetres: distance}
		if len(fills) > 0 {
			in.StartingFillExcluded = fills[0]
			in.Fills = fills[1:]
		}
		res, _ := in.Compute()
		if res.Unknown != "" {
			totals.Coverage[vehicle] = res.Unknown
		} else {
			totals.Coverage[vehicle] = res.KmPerUnit
		}
	}
	rawTotals, _ := json.Marshal(totals)
	fresh, err := query.BaseLedgerUpdated(ctx, uid(accountID))
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	if !sameBase(fresh, base) {
		return application.Report{}, errConflict()
	}
	upserted, err := query.UpsertReport(ctx, q.UpsertReportParams{AccountID: uid(accountID), Year: int32(year), Month: int32(month), Timezone: timezone, Totals: rawTotals, BaseUpdatedAt: base})
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	if s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return application.Report{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.Report{}, errUnavailable()
	}
	return decodeReport(year, month, timezone, upserted.Totals, upserted.Revision, upserted.GeneratedAt.Time)
}

func decodeReport(year, month int, timezone string, raw []byte, revision int64, generated time.Time) (application.Report, error) {
	var totals application.ReportTotals
	if json.Unmarshal(raw, &totals) != nil {
		return application.Report{}, errUnavailable()
	}
	if totals.ByVehicle == nil {
		totals.ByVehicle = map[string]int64{}
	}
	if totals.ByCategory == nil {
		totals.ByCategory = map[string]int64{}
	}
	if totals.Coverage == nil {
		totals.Coverage = map[string]string{}
	}
	return application.Report{Year: year, Month: month, Timezone: timezone, Totals: totals, Revision: revision, GeneratedAt: generated.Format(time.RFC3339)}, nil
}

func (s *Store) GetReport(ctx context.Context, accountID string, year, month int, timezone string) (application.Report, error) {
	if _, _, err := monthBounds(year, month, timezone); err != nil {
		return application.Report{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	defer rollback(tx)
	row, err := q.New(tx).GetReport(ctx, q.GetReportParams{AccountID: uid(accountID), Year: int32(year), Month: int32(month), Timezone: timezone})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.Report{}, errNotFound()
		}
		return application.Report{}, errUnavailable()
	}
	return decodeReport(year, month, timezone, row.Totals, row.Revision, row.GeneratedAt.Time)
}

var _ application.ReportStore = (*Store)(nil)
