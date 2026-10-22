// Energy metrics extend the PRM06 interval engine (EE-16). Bought energy
// (metered Wh at the charger) and paid session totals are exact sums;
// efficiency (Wh/km) needs an unshared odometer distance. A vehicle that
// burned fuel and charged in the same window shares one distance:
// per-carrier ratios would double-attribute it, so both report
// "phev-shared-distance" while money totals and the combined cost/km stay
// exact. Session fees are reported separately and never auto-added to
// session totals (fee inclusion in the stated total is unknown).
package metrics

const MaxEnergyWh = 1000000000000

// EnergyInterval is one vehicle's electric sessions over a shared window.
// PurchasedWh/CostCentavos sum stated session values; Complete is false
// when any session omits energy or total (sums then cover stated values
// only and no ratio is produced).
type EnergyInterval struct {
	PurchasedWh     int64
	CostCentavos    int64
	DistanceMetres  int64
	HasEnergy       bool
	Complete        bool
	HasGap          bool
	CrossesSegment  bool
	OdometerMissing bool
	SocInconsistent bool
	MixedCarrier    bool
}

// EnergyResult carries exact sums with at most one unknown reason.
// Ratios are empty whenever Unknown is set: no invented efficiency.
type EnergyResult struct {
	PurchasedWh    int64
	CostCentavos   int64
	DistanceMetres int64
	WhPerKm        string
	CentavosPerKm  string
	Unknown        string
}

// Compute applies the shared unknown vocabulary first (evidence integrity
// before carrier attribution), then exact sum-first ratios.
func (in EnergyInterval) Compute() EnergyResult {
	out := EnergyResult{PurchasedWh: in.PurchasedWh, CostCentavos: in.CostCentavos, DistanceMetres: in.DistanceMetres}
	if in.PurchasedWh < 0 || in.PurchasedWh > MaxEnergyWh || in.CostCentavos < 0 || in.CostCentavos > MaxTotalCentavos {
		return EnergyResult{Unknown: "input-out-of-bounds"}
	}
	if in.DistanceMetres < 0 || in.DistanceMetres > MaxDistanceMetres {
		return EnergyResult{Unknown: "input-out-of-bounds"}
	}
	switch {
	case in.OdometerMissing:
		out.Unknown = "odometer-missing"
	case in.HasGap:
		out.Unknown = "declared-fill-gap"
	case in.CrossesSegment:
		out.Unknown = "odometer-segment-boundary"
	case in.SocInconsistent:
		out.Unknown = "soc-inconsistent"
	case in.MixedCarrier:
		out.Unknown = "phev-shared-distance"
	case in.DistanceMetres <= 0:
		out.Unknown = "distance-nonpositive"
	case !in.HasEnergy:
		out.Unknown = "energy-unmeasured"
	case !in.Complete:
		out.Unknown = "partial-measurement"
	default:
		out.WhPerKm = scaledRational(in.PurchasedWh, 1000, in.DistanceMetres)
		out.CentavosPerKm = scaledRational(in.CostCentavos, 1000, in.DistanceMetres)
	}
	return out
}

// CombinedCostPerKm is the compatible monetary cost/km over a shared
// distance (fuel purchase plus stated charge totals, counted once each).
// Fees stay out: their inclusion in session totals is unknown.
func CombinedCostPerKm(fuelCostCentavos, chargeCostCentavos, distanceMetres int64) CostResult {
	fuel, ok := addNonNegative(0, fuelCostCentavos)
	if !ok || fuelCostCentavos > MaxTotalCentavos || chargeCostCentavos < 0 || chargeCostCentavos > MaxTotalCentavos {
		return CostResult{Version: CalculationVersion, Unknown: "input-out-of-bounds"}
	}
	total, ok := addNonNegative(fuel, chargeCostCentavos)
	if !ok {
		return CostResult{Version: CalculationVersion, Unknown: "input-out-of-bounds"}
	}
	if distanceMetres < 0 || distanceMetres > MaxDistanceMetres {
		return CostResult{Version: CalculationVersion, Unknown: "input-out-of-bounds"}
	}
	if distanceMetres <= 0 {
		return CostResult{Version: CalculationVersion, Unknown: "distance-nonpositive"}
	}
	return CostResult{Version: CalculationVersion, TotalCentavos: total, DistanceMetres: distanceMetres, CentavosPerKm: scaledRational(total, 1000, distanceMetres), Method: "combined-purchase-interval"}
}
