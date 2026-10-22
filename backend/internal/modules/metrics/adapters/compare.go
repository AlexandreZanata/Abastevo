package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/application"
)

func errInvalid() error { return errors.New("metrics: invalid input") }

func errUnavailable() error { return errors.New("metrics: service unavailable") }

func validUUID(s string) bool {
	if s == "00000000-0000-0000-0000-000000000000" || len(s) != 36 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}

type Store struct {
	Reader application.LedgerReader
}

func NewStore(reader application.LedgerReader) *Store { return &Store{Reader: reader} }

func parseSideRecords(rows []application.LedgerRecord) (metrics.Interval, string, error) {
	unit := ""
	in := metrics.Interval{}
	for i, row := range rows {
		var payload struct {
			Fuel           string `json:"fuel"`
			Unit           string `json:"unit"`
			QuantityMilli  int64  `json:"quantity_milli"`
			TotalCentavos  int64  `json:"total_centavos"`
			Fill           string `json:"fill"`
			OdometerMetres *int64 `json:"odometer_metres"`
		}
		if json.Unmarshal(row.Payload, &payload) != nil {
			return metrics.Interval{}, "", errInvalid()
		}
		if i == 0 {
			unit = payload.Unit
		}
		full := payload.Fill == "full"
		in.Fills = append(in.Fills, metrics.Fill{QuantityMilli: payload.QuantityMilli, TotalCentavos: payload.TotalCentavos, Full: full, Fuel: payload.Fuel, Unit: payload.Unit})
		if payload.OdometerMetres == nil {
			in.OdometerMissing = true
		} else if i == 0 {
			in.DistanceMetres = 0
		}
		_ = i
	}
	if len(rows) > 0 {
		var firstOdo, lastOdo *int64
		for _, row := range rows {
			var payload struct {
				OdometerMetres *int64 `json:"odometer_metres"`
			}
			_ = json.Unmarshal(row.Payload, &payload)
			if payload.OdometerMetres != nil {
				if firstOdo == nil {
					firstOdo = payload.OdometerMetres
				}
				lastOdo = payload.OdometerMetres
			}
		}
		if firstOdo != nil && lastOdo != nil {
			in.DistanceMetres = *lastOdo - *firstOdo
		}
		if len(in.Fills) > 0 {
			in.StartingFillExcluded = in.Fills[0]
			in.Fills = in.Fills[1:]
			if len(in.Fills) == 0 {
				in.Fills = []metrics.Fill{}
			}
		}
	}
	return in, unit, nil
}

func (s *Store) Compare(ctx context.Context, accountID string, baseline, current application.Side) (application.Comparison, error) {
	for _, side := range []application.Side{baseline, current} {
		if !validUUID(side.VehicleID) {
			return application.Comparison{}, errInvalid()
		}
		from, err := time.Parse(time.RFC3339, side.From)
		if err != nil {
			return application.Comparison{}, errInvalid()
		}
		to, err := time.Parse(time.RFC3339, side.To)
		if err != nil || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
			return application.Comparison{}, errInvalid()
		}
		if _, err = time.LoadLocation(side.Timezone); err != nil {
			return application.Comparison{}, errInvalid()
		}
		_ = from
	}
	if baseline.Method != current.Method || (baseline.Method != "fuel-purchase-interval" && baseline.Method != "matched-window") {
		return application.Comparison{Compatible: false, Issue: "mismatched-method"}, nil
	}
	if baseline.Timezone != current.Timezone {
		return application.Comparison{Compatible: false, Issue: "mismatched-timezone"}, nil
	}
	if s.Reader == nil {
		return application.Comparison{}, errUnavailable()
	}
	baseRows, err := s.Reader.ListRecords(ctx, accountID, "refuel", baseline.VehicleID, baseline.From, baseline.To, 1000)
	if err != nil {
		return application.Comparison{}, err
	}
	currentRows, err := s.Reader.ListRecords(ctx, accountID, "refuel", current.VehicleID, current.From, current.To, 1000)
	if err != nil {
		return application.Comparison{}, err
	}
	if len(baseRows) == 0 {
		return application.Comparison{Compatible: false, Issue: "missing-baseline"}, nil
	}
	if len(currentRows) == 0 {
		return application.Comparison{Compatible: false, Issue: "missing-current"}, nil
	}
	baseIn, baseUnit, err := parseSideRecords(baseRows)
	if err != nil {
		return application.Comparison{}, err
	}
	currentIn, currentUnit, err := parseSideRecords(currentRows)
	if err != nil {
		return application.Comparison{}, err
	}
	if baseUnit != currentUnit {
		return application.Comparison{Compatible: false, Issue: "incompatible-unit"}, nil
	}
	baseRes, _ := baseIn.Compute()
	currentRes, _ := currentIn.Compute()
	if baseRes.Unknown != "" {
		return application.Comparison{Compatible: false, Issue: "baseline-incomplete: " + baseRes.Unknown}, nil
	}
	if currentRes.Unknown != "" {
		return application.Comparison{Compatible: false, Issue: "current-incomplete: " + currentRes.Unknown}, nil
	}
	out := application.Comparison{Compatible: true, Baseline: baseRes.KmPerUnit, Current: currentRes.KmPerUnit}
	change := metrics.CompareConsumption(baseRes, currentRes)
	out.Issue = change.Unknown
	out.ChangePercent = change.ChangePercent
	return out, nil
}
