// Cost aggregates extend the T01 interval engine (PRM06-T02). Purchase-based
// fuel cost uses actual paid totals for the same fills as consumption.
// All-expense matched-window cost uses explicit odometer endpoints with
// included expenses for exactly the same documented window. Every ratio
// exposes its numerator, denominator, window and method; absent coverage
// is UNKNOWN. Weighted totals sum first and divide once.
package metrics

import "time"

const CalculationVersion = 1

type Window struct {
	StartOdometer  int64
	EndOdometer    int64
	StartTime      time.Time
	EndTime        time.Time
	HasGap         bool
	CrossesSegment bool
}

type CostResult struct {
	Version        int
	TotalCentavos  int64
	DistanceMetres int64
	CentavosPerKm  string
	Method         string
	Unknown        string
}

type ExpenseEntry struct {
	ID             string
	AmountCentavos int64
	OccurredAt     time.Time
	OdometerMetres *int64
}

func FuelPurchasePerKm(eligibleCostCentavos, distanceMetres int64) CostResult {
	if eligibleCostCentavos < 0 || eligibleCostCentavos > MaxTotalCentavos || distanceMetres < 0 || distanceMetres > MaxDistanceMetres {
		return CostResult{Version: CalculationVersion, Unknown: "input-out-of-bounds"}
	}
	if distanceMetres <= 0 {
		return CostResult{Version: CalculationVersion, Unknown: "distance-nonpositive"}
	}
	return CostResult{Version: CalculationVersion, TotalCentavos: eligibleCostCentavos, DistanceMetres: distanceMetres, CentavosPerKm: rational(eligibleCostCentavos*1000, distanceMetres), Method: "fuel-purchase-interval"}
}

func MatchedWindowCost(fuelCostCentavos, fuelDistanceMetres int64, expenses []ExpenseEntry, window Window) CostResult {
	if !window.EndTime.After(window.StartTime) {
		return CostResult{Version: CalculationVersion, Unknown: "unmatched-period-window"}
	}
	if window.HasGap {
		return CostResult{Version: CalculationVersion, Unknown: "declared-fill-gap"}
	}
	if window.CrossesSegment {
		return CostResult{Version: CalculationVersion, Unknown: "odometer-segment-boundary"}
	}
	distance := window.EndOdometer - window.StartOdometer
	if distance <= 0 {
		return CostResult{Version: CalculationVersion, Unknown: "distance-nonpositive"}
	}
	if fuelDistanceMetres != distance {
		return CostResult{Version: CalculationVersion, Unknown: "unmatched-period-window"}
	}
	seen := map[string]bool{}
	total := fuelCostCentavos
	for _, e := range expenses {
		if e.AmountCentavos < 0 || e.AmountCentavos > MaxTotalCentavos {
			return CostResult{Version: CalculationVersion, Unknown: "input-out-of-bounds"}
		}
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		if e.OccurredAt.Before(window.StartTime) || !e.OccurredAt.Before(window.EndTime) {
			continue
		}
		if e.OdometerMetres != nil && (*e.OdometerMetres < window.StartOdometer || *e.OdometerMetres > window.EndOdometer) {
			continue
		}
		total += e.AmountCentavos
	}
	return CostResult{Version: CalculationVersion, TotalCentavos: total, DistanceMetres: distance, CentavosPerKm: rational(total*1000, distance), Method: "matched-window"}
}

type AggregateTotals struct {
	DistanceMetres        int64
	EligibleQuantityMilli int64
	EligibleCostCentavos  int64
	KmPerUnit             string
	CentavosPerKm         string
}

func SumIntervals(results []Result) AggregateTotals {
	var totals AggregateTotals
	for _, r := range results {
		if r.Unknown != "" {
			continue
		}
		totals.DistanceMetres += r.DistanceMetres
		totals.EligibleQuantityMilli += r.EligibleQuantityMilli
		totals.EligibleCostCentavos += r.EligibleCostCentavos
	}
	if totals.DistanceMetres > 0 && totals.EligibleQuantityMilli > 0 {
		totals.KmPerUnit = rational(totals.DistanceMetres, totals.EligibleQuantityMilli)
	}
	if totals.DistanceMetres > 0 {
		totals.CentavosPerKm = rational(totals.EligibleCostCentavos*1000, totals.DistanceMetres)
	}
	return totals
}
