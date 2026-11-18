package domain

import (
	"errors"
	"fmt"
	"math/big"
)

type RatioInterval struct {
	AssetID       string
	Product       string
	DriverID      string
	DistanceM     int64
	HourSeconds   int64
	QuantityMilli int64
	CostCentavos  int64
}

type ConsumptionSummary struct {
	Status              string
	Reason              InsufficiencyReason
	DistanceM           int64
	HourSeconds         int64
	QuantityMilli       int64
	CostCentavos        int64
	KmPerUnitRat        string
	KmPerUnit2DP        string
	LitresPer100Km2DP   string
	LitresPerHourRat    string
	LitresPerHour2DP    string
	CentsPerKmRounded   int64
	CentsPerHourRounded int64
	SampleCount         int
	SmallSample         bool
}

func formatScaled2DP(num, scale, den int64) string {
	r := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(num), big.NewInt(scale)), big.NewInt(den))
	return formatRatDecimal(r, 2)
}

func formatRatDecimal(r *big.Rat, dp int) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dp)), nil)
	n := new(big.Int).Mul(r.Num(), scale)
	d := r.Denom()
	neg := n.Sign() < 0
	if neg {
		n = new(big.Int).Neg(n)
	}
	q, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	if new(big.Int).Lsh(rem, 1).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	whole := new(big.Int)
	frac := new(big.Int)
	whole.QuoRem(q, scale, frac)
	prefix := ""
	if neg && (whole.Sign() != 0 || frac.Sign() != 0) {
		prefix = "-"
	}
	if dp == 0 {
		return fmt.Sprintf("%s%s", prefix, whole.String())
	}
	return fmt.Sprintf("%s%s.%0*d", prefix, whole.String(), dp, frac.Int64())
}

func roundHalfUpBig(num, den *big.Int) int64 {
	half := new(big.Int).Rsh(den, 1)
	return new(big.Int).Quo(new(big.Int).Add(num, half), den).Int64()
}

// ComputeDistanceConsumption evaluates IND-003, IND-004, IND-006 using ratio-of-sums.
func ComputeDistanceConsumption(intervals []RatioInterval) ConsumptionSummary {
	if len(intervals) == 0 {
		return ConsumptionSummary{Status: "INSUFFICIENT_DATA", Reason: ReasonNoClosedInterval}
	}
	var sumDist, sumQty, sumCost int64
	products := map[string]bool{}
	for _, iv := range intervals {
		if iv.Product == "CNG" {
			return ConsumptionSummary{Status: "INSUFFICIENT_DATA", Reason: ReasonMethodUnsupported}
		}
		if iv.DistanceM <= 0 || iv.QuantityMilli <= 0 {
			continue
		}
		products[iv.Product] = true
		sumDist += iv.DistanceM
		sumQty += iv.QuantityMilli
		sumCost += iv.CostCentavos
	}
	if len(products) > 1 {
		return ConsumptionSummary{Status: "INSUFFICIENT_DATA", Reason: ReasonMethodUnsupported}
	}
	if sumDist <= 0 || sumQty <= 0 {
		return ConsumptionSummary{Status: "INSUFFICIENT_DATA", Reason: ReasonNoClosedInterval}
	}
	kmPerUnitRat := new(big.Rat).SetFrac64(sumDist, sumQty).RatString()
	return ConsumptionSummary{
		Status:            "COMPUTED",
		DistanceM:         sumDist,
		QuantityMilli:     sumQty,
		CostCentavos:      sumCost,
		KmPerUnitRat:      kmPerUnitRat,
		KmPerUnit2DP:      formatScaled2DP(sumDist, 1, sumQty),
		LitresPer100Km2DP: formatScaled2DP(sumQty, 100, sumDist),
		CentsPerKmRounded: roundHalfUpBig(new(big.Int).Mul(big.NewInt(sumCost), big.NewInt(1000)), big.NewInt(sumDist)),
		SampleCount:       len(intervals),
		SmallSample:       len(intervals) < 3,
	}
}

// ComputeHourConsumption evaluates IND-005 and IND-007 using exact seconds-to-hours ratios.
func ComputeHourConsumption(intervals []RatioInterval) ConsumptionSummary {
	if len(intervals) == 0 {
		return ConsumptionSummary{Status: "INSUFFICIENT_DATA", Reason: ReasonNoHourBasis}
	}
	var sumSec, sumQty, sumCost int64
	for _, iv := range intervals {
		if iv.HourSeconds <= 0 || iv.QuantityMilli <= 0 {
			continue
		}
		sumSec += iv.HourSeconds
		sumQty += iv.QuantityMilli
		sumCost += iv.CostCentavos
	}
	if sumSec <= 0 || sumQty <= 0 {
		return ConsumptionSummary{Status: "INSUFFICIENT_DATA", Reason: ReasonNoHourBasis}
	}
	// (sumQty * 3600) / (sumSec * 1000)
	lphNum := new(big.Int).Mul(big.NewInt(sumQty), big.NewInt(3600))
	lphDen := new(big.Int).Mul(big.NewInt(sumSec), big.NewInt(1000))
	lphRat := new(big.Rat).SetFrac(lphNum, lphDen)
	cphNum := new(big.Int).Mul(big.NewInt(sumCost), big.NewInt(3600))
	cphDen := big.NewInt(sumSec)
	return ConsumptionSummary{
		Status:              "COMPUTED",
		HourSeconds:         sumSec,
		QuantityMilli:       sumQty,
		CostCentavos:        sumCost,
		LitresPerHourRat:    lphRat.RatString(),
		LitresPerHour2DP:    formatRatDecimal(lphRat, 2),
		CentsPerHourRounded: roundHalfUpBig(cphNum, cphDen),
		SampleCount:         len(intervals),
		SmallSample:         len(intervals) < 3,
	}
}

type AvailabilitySummary struct {
	Status             string
	Reason             InsufficiencyReason
	ActiveSeconds      int64
	UnavailableSeconds int64
	AvailableSeconds   int64
	Ratio              string
	Percent1DP         string
}

func ComputeAvailability(activeS, unavailS int64) AvailabilitySummary {
	if activeS <= 0 {
		return AvailabilitySummary{Status: "INSUFFICIENT_DATA", Reason: ReasonNoAvailabilityHistory}
	}
	if unavailS < 0 {
		unavailS = 0
	}
	if unavailS > activeS {
		unavailS = activeS
	}
	availS := activeS - unavailS
	rat := new(big.Rat).SetFrac64(availS, activeS)
	pct := new(big.Rat).SetFrac64(availS*100, activeS)
	return AvailabilitySummary{
		Status:             "COMPUTED",
		ActiveSeconds:      activeS,
		UnavailableSeconds: unavailS,
		AvailableSeconds:   availS,
		Ratio:              rat.RatString(),
		Percent1DP:         formatRatDecimal(pct, 1),
	}
}

type PeriodChange struct {
	Status     string
	Reason     InsufficiencyReason
	Percent1DP string
}

func ComputeVariation(baselineNum, currentNum, baselineDen, currentDen int64, baselineMethod, currentMethod string) PeriodChange {
	if baselineMethod != currentMethod {
		return PeriodChange{Status: "INSUFFICIENT_DATA", Reason: ReasonMethodMismatch}
	}
	if baselineDen == 0 || currentDen == 0 || baselineNum == 0 {
		return PeriodChange{Status: "INSUFFICIENT_DATA", Reason: ReasonZeroBaseline}
	}
	b := new(big.Rat).SetFrac64(baselineNum, baselineDen)
	c := new(big.Rat).SetFrac64(currentNum, currentDen)
	delta := new(big.Rat).Sub(c, b)
	delta.Quo(delta, b)
	delta.Mul(delta, big.NewRat(100, 1))
	return PeriodChange{Status: "COMPUTED", Percent1DP: formatRatDecimal(delta, 1)}
}

// AllocateIntervalToTrips allocates interval quantity and cost proportionally across
// trips using exact largest-remainder allocation (EMP-FUEL-010).
type TripShareInput struct {
	TripID    string
	DistanceM int64
}

type TripAllocation struct {
	TripID        string
	QuantityMilli int64
	CostCentavos  int64
}

func AllocateIntervalToTrips(totalQtyMilli, totalCostCentavos int64, totalIntervalDistanceM int64, trips []TripShareInput) ([]TripAllocation, TripAllocation, error) {
	if totalQtyMilli <= 0 || totalCostCentavos < 0 || totalIntervalDistanceM <= 0 {
		return nil, TripAllocation{}, errors.New("intelligence: invalid allocation basis")
	}
	out := make([]TripAllocation, 0, len(trips))
	var usedDist, usedQty, usedCost int64
	for _, t := range trips {
		if t.DistanceM < 0 {
			return nil, TripAllocation{}, errors.New("intelligence: negative trip distance")
		}
		if usedDist+t.DistanceM > totalIntervalDistanceM {
			return nil, TripAllocation{}, errors.New("intelligence: trips exceed interval distance")
		}
		q := (t.DistanceM * totalQtyMilli) / totalIntervalDistanceM
		c := (t.DistanceM * totalCostCentavos) / totalIntervalDistanceM
		usedDist += t.DistanceM
		usedQty += q
		usedCost += c
		out = append(out, TripAllocation{TripID: t.TripID, QuantityMilli: q, CostCentavos: c})
	}
	unassigned := TripAllocation{
		TripID:        "UNASSIGNED",
		QuantityMilli: totalQtyMilli - usedQty,
		CostCentavos:  totalCostCentavos - usedCost,
	}
	return out, unassigned, nil
}
