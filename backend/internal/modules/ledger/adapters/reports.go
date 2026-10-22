package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
	"github.com/jackc/pgx/v5"
)

const reportProjectionVersion = 3

type storedReportTotals struct {
	application.ReportTotals
	ProjectionVersion int    `json:"_projection_version"`
	SourceFingerprint string `json:"_source_fingerprint"`
}

func currentProjection(raw []byte, fingerprint string) bool {
	var v storedReportTotals
	return json.Unmarshal(raw, &v) == nil && v.ProjectionVersion == reportProjectionVersion && v.SourceFingerprint == fingerprint
}
func checkedReportSum(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return 0, errUnavailable()
	}
	return a + b, nil
}

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
	if err = lockLedgerOwner(ctx, tx, accountID); err != nil {
		return application.Report{}, err
	}
	lockKey := fmt.Sprintf("%s|%d|%d|%s", accountID, year, month, timezone)
	if _, err = query.LockReport(ctx, lockKey); err != nil {
		return application.Report{}, errUnavailable()
	}
	base, err := query.BaseLedgerUpdated(ctx, uid(accountID))
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	fingerprint, err := query.LedgerFingerprint(ctx, uid(accountID))
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	if existing, err := query.GetReport(ctx, q.GetReportParams{AccountID: uid(accountID), Year: int32(year), Month: int32(month), Timezone: timezone}); err == nil {
		if currentProjection(existing.Totals, fingerprint) {
			return decodeReport(year, month, timezone, existing.Totals, existing.Revision, existing.GeneratedAt.Time)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return application.Report{}, errUnavailable()
	}
	rows, err := query.ReportRows(ctx, q.ReportRowsParams{AccountID: uid(accountID), OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end)})
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	totals := application.ReportTotals{ByVehicle: map[string]int64{}, ByCategory: map[string]int64{}, Coverage: map[string]string{}, EnergyCoverage: map[string]string{}, CombinedCostPerKm: map[string]string{}}
	byVehicleFills := map[string][]metrics.Fill{}
	byVehicleOdo := map[string][]int64{}
	byVehicleSegment := map[string]int64{}
	byVehicleFlags := map[string]metrics.Interval{}
	byVehicleFuel := map[string]int64{}
	byVehicleEnergy := map[string]int64{}
	byVehicleChargeCost := map[string]int64{}
	chargeSessions := map[string]int{}
	chargeStated := map[string]int{}
	chargeEnergyStated := map[string]int{}
	byVehicleSocBad := map[string]bool{}
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
		if row.Dataset == "charge" {
			var charge struct {
				EnergyWh       *int64 `json:"energy_wh"`
				TotalCentavos  *int64 `json:"total_centavos"`
				FeesCentavos   *int64 `json:"fees_centavos"`
				SocStart       *int   `json:"soc_start_percent"`
				SocEnd         *int   `json:"soc_end_percent"`
				OdometerMetres *int64 `json:"odometer_metres"`
			}
			if json.Unmarshal(row.Payload, &charge) != nil {
				return application.Report{}, errUnavailable()
			}
			vehicle := ledgerIDString(row.VehicleID)
			totals.RecordCount++
			chargeSessions[vehicle]++
			if charge.EnergyWh != nil {
				totals.ChargeEnergyWh, err = checkedReportSum(totals.ChargeEnergyWh, *charge.EnergyWh)
				if err != nil {
					return application.Report{}, err
				}
				byVehicleEnergy[vehicle], err = checkedReportSum(byVehicleEnergy[vehicle], *charge.EnergyWh)
				if err != nil {
					return application.Report{}, err
				}
				chargeEnergyStated[vehicle]++
			}
			stated := charge.EnergyWh != nil && charge.TotalCentavos != nil
			if stated {
				chargeStated[vehicle]++
			}
			if charge.TotalCentavos != nil {
				totals.ChargeCostCentavos, err = checkedReportSum(totals.ChargeCostCentavos, *charge.TotalCentavos)
				if err != nil {
					return application.Report{}, err
				}
				byVehicleChargeCost[vehicle], err = checkedReportSum(byVehicleChargeCost[vehicle], *charge.TotalCentavos)
				if err != nil {
					return application.Report{}, err
				}
				totals.ByVehicle[vehicle], err = checkedReportSum(totals.ByVehicle[vehicle], *charge.TotalCentavos)
				if err != nil {
					return application.Report{}, err
				}
			}
			if charge.FeesCentavos != nil {
				totals.ChargeFeesCentavos, err = checkedReportSum(totals.ChargeFeesCentavos, *charge.FeesCentavos)
				if err != nil {
					return application.Report{}, err
				}
			}
			if charge.SocStart != nil && charge.SocEnd != nil && *charge.SocEnd < *charge.SocStart {
				byVehicleSocBad[vehicle] = true
			}
			flags := byVehicleFlags[vehicle]
			flags.OdometerMissing = flags.OdometerMissing || charge.OdometerMetres == nil
			flags.HasGap = flags.HasGap || row.HasGap
			if first, ok := byVehicleSegment[vehicle]; ok {
				flags.CrossesSegment = flags.CrossesSegment || first != row.SegmentNo
			} else {
				byVehicleSegment[vehicle] = row.SegmentNo
			}
			byVehicleFlags[vehicle] = flags
			if charge.OdometerMetres != nil {
				byVehicleOdo[vehicle] = append(byVehicleOdo[vehicle], *charge.OdometerMetres)
			}
			continue
		}
		if json.Unmarshal(row.Payload, &payload) != nil {
			return application.Report{}, errUnavailable()
		}
		vehicle := ledgerIDString(row.VehicleID)
		totals.RecordCount++
		if row.Dataset == "refuel" {
			totals.PurchaseCentavos, err = checkedReportSum(totals.PurchaseCentavos, payload.TotalCentavos)
			if err != nil {
				return application.Report{}, err
			}
			byVehicleFuel[vehicle], err = checkedReportSum(byVehicleFuel[vehicle], payload.TotalCentavos)
			if err != nil {
				return application.Report{}, err
			}
			totals.ByVehicle[vehicle], err = checkedReportSum(totals.ByVehicle[vehicle], payload.TotalCentavos)
			if err != nil {
				return application.Report{}, err
			}
			flags := byVehicleFlags[vehicle]
			flags.OdometerMissing = flags.OdometerMissing || payload.OdometerMetres == nil
			flags.HasGap = flags.HasGap || row.HasGap
			if first, ok := byVehicleSegment[vehicle]; ok {
				flags.CrossesSegment = flags.CrossesSegment || first != row.SegmentNo
			} else {
				byVehicleSegment[vehicle] = row.SegmentNo
			}
			byVehicleFlags[vehicle] = flags
			byVehicleFills[vehicle] = append(byVehicleFills[vehicle], metrics.Fill{QuantityMilli: payload.QuantityMilli, TotalCentavos: payload.TotalCentavos, Full: payload.Fill == "full", Fuel: payload.Fuel, Unit: payload.Unit})
			if payload.OdometerMetres != nil {
				byVehicleOdo[vehicle] = append(byVehicleOdo[vehicle], *payload.OdometerMetres)
			}
		} else if row.Dataset == "expense" {
			totals.ExpenseCentavos, err = checkedReportSum(totals.ExpenseCentavos, payload.AmountCentavos)
			if err != nil {
				return application.Report{}, err
			}
			totals.ByVehicle[vehicle], err = checkedReportSum(totals.ByVehicle[vehicle], payload.AmountCentavos)
			if err != nil {
				return application.Report{}, err
			}
			totals.ByCategory[payload.Category], err = checkedReportSum(totals.ByCategory[payload.Category], payload.AmountCentavos)
			if err != nil {
				return application.Report{}, err
			}
		} else {
			return application.Report{}, errUnavailable()
		}
	}
	vehicles := map[string]bool{}
	for vehicle := range byVehicleFills {
		vehicles[vehicle] = true
	}
	for vehicle := range chargeSessions {
		vehicles[vehicle] = true
	}
	for vehicle := range vehicles {
		fills := byVehicleFills[vehicle]
		charges := chargeSessions[vehicle]
		odos := byVehicleOdo[vehicle]
		flags := byVehicleFlags[vehicle]
		mixed := len(fills) > 0 && charges > 0
		if mixed {
			// One shared distance serves both carriers: attributing it
			// to either carrier alone would double-count. Money stays
			// exact; only the per-carrier ratios are refused.
			totals.Coverage[vehicle] = "phev-shared-distance"
			totals.EnergyCoverage[vehicle] = "phev-shared-distance"
		} else if len(fills) > 0 {
			fulls := 0
			for _, f := range fills {
				if f.Full {
					fulls++
				}
			}
			if fulls < 2 {
				totals.Coverage[vehicle] = "full-anchors-required"
			} else if flags.OdometerMissing || len(odos) < 2 {
				totals.Coverage[vehicle] = "odometer-missing"
			} else {
				distance := odos[len(odos)-1] - odos[0]
				if distance <= 0 {
					totals.Coverage[vehicle] = "distance-nonpositive"
				} else {
					in := flags
					in.Fills = fills[1:]
					in.DistanceMetres = distance
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
			}
		} else if charges > 0 {
			distance := int64(0)
			if len(odos) >= 2 {
				distance = odos[len(odos)-1] - odos[0]
			}
			in := metrics.EnergyInterval{
				PurchasedWh: byVehicleEnergy[vehicle], CostCentavos: byVehicleChargeCost[vehicle], DistanceMetres: distance,
				HasEnergy: chargeEnergyStated[vehicle] > 0, Complete: chargeStated[vehicle] == charges,
				HasGap: flags.HasGap, CrossesSegment: flags.CrossesSegment, OdometerMissing: flags.OdometerMissing || len(odos) < 1,
				SocInconsistent: byVehicleSocBad[vehicle],
			}
			res := in.Compute()
			if res.Unknown != "" {
				totals.EnergyCoverage[vehicle] = res.Unknown
			} else {
				totals.EnergyCoverage[vehicle] = res.WhPerKm
			}
		}
		if len(fills) > 0 || charges > 0 {
			// Combined money/km uses the shared distance once (fuel
			// purchase plus stated charge totals, each counted once;
			// fees excluded: their inclusion in totals is unknown).
			distance := int64(0)
			if len(odos) >= 2 {
				distance = odos[len(odos)-1] - odos[0]
			}
			switch {
			case flags.OdometerMissing || len(odos) < 1:
				totals.CombinedCostPerKm[vehicle] = "odometer-missing"
			case flags.HasGap:
				totals.CombinedCostPerKm[vehicle] = "declared-fill-gap"
			case flags.CrossesSegment:
				totals.CombinedCostPerKm[vehicle] = "odometer-segment-boundary"
			default:
				combined := metrics.CombinedCostPerKm(byVehicleFuel[vehicle], byVehicleChargeCost[vehicle], distance)
				if combined.Unknown != "" {
					totals.CombinedCostPerKm[vehicle] = combined.Unknown
				} else {
					totals.CombinedCostPerKm[vehicle] = combined.CentavosPerKm
				}
			}
		}
	}
	rawTotals, _ := json.Marshal(storedReportTotals{ReportTotals: totals, ProjectionVersion: reportProjectionVersion, SourceFingerprint: fingerprint})
	fresh, err := query.LedgerFingerprint(ctx, uid(accountID))
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	if fresh != fingerprint {
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
	if totals.EnergyCoverage == nil {
		totals.EnergyCoverage = map[string]string{}
	}
	if totals.CombinedCostPerKm == nil {
		totals.CombinedCostPerKm = map[string]string{}
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
	if err = lockLedgerOwner(ctx, tx, accountID); err != nil {
		return application.Report{}, err
	}
	fingerprint, err := q.New(tx).LedgerFingerprint(ctx, uid(accountID))
	if err != nil {
		return application.Report{}, errUnavailable()
	}
	row, err := q.New(tx).GetReport(ctx, q.GetReportParams{AccountID: uid(accountID), Year: int32(year), Month: int32(month), Timezone: timezone})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.Report{}, errNotFound()
		}
		return application.Report{}, errUnavailable()
	}
	if !currentProjection(row.Totals, fingerprint) {
		return application.Report{}, errConflict()
	}
	return decodeReport(year, month, timezone, row.Totals, row.Revision, row.GeneratedAt.Time)
}

var _ application.ReportStore = (*Store)(nil)
