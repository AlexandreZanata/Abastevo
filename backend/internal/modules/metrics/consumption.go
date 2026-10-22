// Package metrics implements the frozen full-to-full consumption and
// purchase-cost engine (PRM06-T01). Exact integer and rational math only:
// no floats, no sums across units, no consumption guessed from volume.
package metrics

import (
	"errors"
	"math"
	"math/big"
)

const (
	MaxQuantityMilli  = 1000000000
	MaxTotalCentavos  = 1000000000000
	MaxDistanceMetres = 1000000000000
)

type Fill struct {
	QuantityMilli int64
	TotalCentavos int64
	Full          bool
	Fuel          string
	Unit          string
}

type Interval struct {
	StartingFillExcluded Fill
	Fills                []Fill
	DistanceMetres       int64
	HasGap               bool
	CrossesSegment       bool
	OdometerMissing      bool
}

type Result struct {
	EligibleQuantityMilli int64
	EligibleCostCentavos  int64
	DistanceMetres        int64
	KmPerUnit             string
	CentavosPerKm         string
	Unknown               string
}

func rational(num, den int64) string { return scaledRational(num, 1, den) }

func scaledRational(num, scale, den int64) string {
	if den <= 0 {
		return ""
	}
	numerator := new(big.Int).Mul(big.NewInt(num), big.NewInt(scale))
	return new(big.Rat).SetFrac(numerator, big.NewInt(den)).RatString()
}

func addNonNegative(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return 0, false
	}
	return a + b, true
}

var errBounds = errors.New("metrics: input-out-of-bounds")

func (in Interval) Compute() (Result, error) {
	for _, f := range append(append([]Fill{}, in.StartingFillExcluded), in.Fills...) {
		if f.QuantityMilli < 0 || f.QuantityMilli > MaxQuantityMilli || f.TotalCentavos < 0 || f.TotalCentavos > MaxTotalCentavos {
			return Result{Unknown: "input-out-of-bounds"}, errBounds
		}
	}
	if in.DistanceMetres < 0 || in.DistanceMetres > MaxDistanceMetres {
		return Result{Unknown: "input-out-of-bounds"}, errBounds
	}
	if in.OdometerMissing {
		return Result{Unknown: "odometer-missing"}, nil
	}
	if in.HasGap {
		return Result{Unknown: "declared-fill-gap"}, nil
	}
	if in.CrossesSegment {
		return Result{Unknown: "odometer-segment-boundary"}, nil
	}
	if in.DistanceMetres <= 0 {
		return Result{Unknown: "distance-nonpositive"}, nil
	}
	units := map[string]bool{}
	fuels := map[string]bool{}
	for _, f := range append([]Fill{in.StartingFillExcluded}, in.Fills...) {
		switch f.Fuel {
		case "CNG":
			if f.Unit != "M3" {
				return Result{Unknown: "incompatible-unit"}, nil
			}
		case "ETHANOL", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "GASOLINE_PREMIUM_GRADE", "DIESEL_S500", "DIESEL_S10":
			if f.Unit != "L" {
				return Result{Unknown: "incompatible-unit"}, nil
			}
		default:
			return Result{Unknown: "input-out-of-bounds"}, errBounds
		}
		units[f.Unit] = true
		fuels[f.Fuel] = true
	}
	if len(units) > 1 {
		return Result{Unknown: "incompatible-unit"}, nil
	}
	for fuel := range fuels {
		if fuel == "CNG" {
			return Result{Unknown: "unsupported-gnv-method"}, nil
		}
	}
	if len(fuels) > 1 {
		return Result{Unknown: "unsupported-mixed-method"}, nil
	}
	if !in.StartingFillExcluded.Full || len(in.Fills) == 0 || !in.Fills[len(in.Fills)-1].Full {
		return Result{Unknown: "full-anchors-required"}, nil
	}
	var quantity, cost int64
	for _, f := range in.Fills {
		var ok bool
		quantity, ok = addNonNegative(quantity, f.QuantityMilli)
		if !ok {
			return Result{Unknown: "input-out-of-bounds"}, errBounds
		}
		cost, ok = addNonNegative(cost, f.TotalCentavos)
		if !ok {
			return Result{Unknown: "input-out-of-bounds"}, errBounds
		}
	}
	if quantity <= 0 {
		return Result{Unknown: "full-anchors-required"}, nil
	}
	return Result{
		EligibleQuantityMilli: quantity,
		EligibleCostCentavos:  cost,
		DistanceMetres:        in.DistanceMetres,
		KmPerUnit:             rational(in.DistanceMetres, quantity),
		CentavosPerKm:         scaledRational(cost, 1000, in.DistanceMetres),
	}, nil
}

// CompareConsumption reuses the existing PRM comparison math; method/unit/
// period compatibility remains the caller's typed contract responsibility.
type ConsumptionComparison struct {
	ChangePercent string
	Unknown       string
}

func CompareConsumption(base, current Result) ConsumptionComparison {
	if base.Unknown != "" {
		return ConsumptionComparison{Unknown: "baseline-incomplete: " + base.Unknown}
	}
	if current.Unknown != "" {
		return ConsumptionComparison{Unknown: "current-incomplete: " + current.Unknown}
	}
	if base.EligibleCostCentavos == 0 {
		return ConsumptionComparison{Unknown: "comparison-baseline-zero"}
	}
	if base.DistanceMetres <= 0 || base.EligibleQuantityMilli <= 0 || current.DistanceMetres <= 0 || current.EligibleQuantityMilli <= 0 || base.EligibleCostCentavos < 0 || current.EligibleCostCentavos < 0 {
		return ConsumptionComparison{Unknown: "input-out-of-bounds"}
	}
	numerator := new(big.Int).Sub(new(big.Int).Mul(big.NewInt(current.DistanceMetres), big.NewInt(base.EligibleQuantityMilli)), new(big.Int).Mul(big.NewInt(base.DistanceMetres), big.NewInt(current.EligibleQuantityMilli)))
	numerator.Mul(numerator, big.NewInt(100))
	denominator := new(big.Int).Mul(big.NewInt(base.DistanceMetres), big.NewInt(current.EligibleQuantityMilli))
	return ConsumptionComparison{ChangePercent: new(big.Rat).SetFrac(numerator, denominator).RatString()}
}
