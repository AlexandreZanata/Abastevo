package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestFuelPurchaseZeroDenominatorAndBounds(t *testing.T) {
	if got := FuelPurchasePerKm(1000, 0); got.Unknown != "distance-nonpositive" {
		t.Fatalf("zero %+v", got)
	}
	if got := FuelPurchasePerKm(-1, 1000); got.Unknown != "input-out-of-bounds" {
		t.Fatalf("negative %+v", got)
	}
	if got := FuelPurchasePerKm(MaxTotalCentavos+1, 1000); got.Unknown != "input-out-of-bounds" {
		t.Fatalf("overflow %+v", got)
	}
	got := FuelPurchasePerKm(24400, 400000)
	if got.CentavosPerKm != "61" || got.Method != "fuel-purchase-interval" || got.Version != CalculationVersion {
		t.Fatalf("interval %+v", got)
	}
}

func TestMatchedWindowUnmatchedDoubleCountDiscount(t *testing.T) {
	start, _ := time.Parse(time.RFC3339, "2026-02-01T00:00:00-03:00")
	end, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00-03:00")
	window := Window{StartOdometer: 1000000, EndOdometer: 1400000, StartTime: start, EndTime: end}
	mid, _ := time.Parse(time.RFC3339, "2026-02-15T10:00:00-03:00")
	outside, _ := time.Parse(time.RFC3339, "2026-03-15T10:00:00-03:00")
	odometer := int64(1200000)
	expenses := []ExpenseEntry{
		{ID: "a", AmountCentavos: 5000, OccurredAt: mid, OdometerMetres: &odometer},
		{ID: "a", AmountCentavos: 5000, OccurredAt: mid, OdometerMetres: &odometer},
		{ID: "b", AmountCentavos: 7000, OccurredAt: outside},
	}
	got := MatchedWindowCost(24400, 400000, expenses, window)
	if got.Unknown != "" || got.TotalCentavos != 29400 || got.CentavosPerKm != rational(29400*1000, 400000) || got.Method != "matched-window" {
		t.Fatalf("matched %+v", got)
	}
	if got := MatchedWindowCost(24400, 300000, expenses, window); got.Unknown != "unmatched-period-window" {
		t.Fatalf("unmatched distance %+v", got)
	}
	badWindow := window
	badWindow.EndTime = start
	if got := MatchedWindowCost(24400, 400000, expenses, badWindow); got.Unknown != "unmatched-period-window" {
		t.Fatalf("unmatched time %+v", got)
	}
	gapWindow := window
	gapWindow.HasGap = true
	if got := MatchedWindowCost(24400, 400000, expenses, gapWindow); got.Unknown != "declared-fill-gap" {
		t.Fatalf("gap %+v", got)
	}
	discounted := MatchedWindowCost(23000, 400000, nil, window)
	if discounted.CentavosPerKm != "115/2" || discounted.TotalCentavos != 23000 {
		t.Fatalf("discount %+v", discounted)
	}
}

func TestWeightedTotalsUnequalDistancesAndBoundaries(t *testing.T) {
	a, _ := Interval{StartingFillExcluded: Fill{QuantityMilli: 1, TotalCentavos: 1, Full: true, Fuel: "ETHANOL", Unit: "L"}, Fills: []Fill{{QuantityMilli: 10000, TotalCentavos: 6000, Full: true, Fuel: "ETHANOL", Unit: "L"}}, DistanceMetres: 100000}.Compute()
	b, _ := Interval{StartingFillExcluded: Fill{QuantityMilli: 1, TotalCentavos: 1, Full: true, Fuel: "ETHANOL", Unit: "L"}, Fills: []Fill{{QuantityMilli: 60000, TotalCentavos: 36000, Full: true, Fuel: "ETHANOL", Unit: "L"}}, DistanceMetres: 900000}.Compute()
	totals := SumIntervals([]Result{a, b})
	if totals.DistanceMetres != 1000000 || totals.EligibleQuantityMilli != 70000 || totals.EligibleCostCentavos != 42000 {
		t.Fatalf("sums %+v", totals)
	}
	if totals.KmPerUnit != "100/7" || totals.CentavosPerKm != "42" {
		t.Fatalf("weighted %+v", totals)
	}
	skipped := SumIntervals([]Result{a, {Unknown: "odometer-missing"}, b})
	if skipped.DistanceMetres != totals.DistanceMetres {
		t.Fatalf("unknown skipped %+v", skipped)
	}
	before := Interval{StartingFillExcluded: Fill{QuantityMilli: 50000, TotalCentavos: 30000, Full: true, Fuel: "ETHANOL", Unit: "L"}, Fills: []Fill{{QuantityMilli: 20000, TotalCentavos: 12000, Full: true, Fuel: "ETHANOL", Unit: "L"}}, DistanceMetres: 400000}
	first, _ := before.Compute()
	regenerated, _ := before.Compute()
	if !reflect.DeepEqual(first, regenerated) {
		t.Fatal("rollback changed projection")
	}
	rawQuantity, rawCost := first.EligibleQuantityMilli, first.EligibleCostCentavos
	_ = rawQuantity
	_ = rawCost
}
