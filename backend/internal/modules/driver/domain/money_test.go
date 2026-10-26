package domain

import (
	"math"
	"testing"
)

func ptr(v int64) *int64 { return &v }
func TestDetailedAndInclusiveNet(t *testing.T) {
	for _, e := range []Earnings{
		{Basis: "DETAILED", Gross: ptr(50000), Fees: ptr(10000), Tips: ptr(2000), Bonus: ptr(3000), Refund: ptr(0)},
		{Basis: "INCLUSIVE_NET", Net: ptr(45000)},
	} {
		got, err := e.Earned()
		if err != nil || got != 45000 {
			t.Fatalf("earned=%d err=%v; want 45000", got, err)
		}
	}
}
func TestMoneyRefusesAmbiguityAndOverflow(t *testing.T) {
	for _, e := range []Earnings{{Basis: "INCLUSIVE_NET", Net: ptr(40000), Tips: ptr(3000)}, {Basis: "DETAILED", Gross: ptr(1)}, {Basis: "INCLUSIVE_NET", Net: ptr(math.MaxInt64)}, {Basis: "UNKNOWN"}} {
		if _, err := e.Earned(); err == nil {
			t.Fatalf("accepted ambiguous/out-of-bound %+v", e)
		}
	}
}

func window() Window {
	return Window{Owner: "owner", Vehicle: "vehicle", Currency: "BRL", FromMillis: 1, ToMillis: 2}
}
func TestMatchedResultsPayoutAndWeightedRatios(t *testing.T) {
	w := window()
	costs := []Cost{{"fuel", 13000, w}, {"toll", 2000, w}, {"lease", 5000, w}}
	r, err := Result(45000, w, w, costs, true, []Cost{{"reserve", 4000, w}, {"depreciation", 6000, w}}, ptr(200000), ptr(28800))
	if err != nil || *r.Recorded != 25000 || *r.Modeled != 15000 || r.RecordedRatios.PerKm != "125" || r.ModeledRatios.PerHour != "1875" {
		t.Fatalf("result %+v err=%v", r, err)
	}
	v, err := Views([]int64{45000}, []CashMovement{{"payout", "PAYOUT", "BANK", 30000}})
	if err != nil || v.Earned != 45000 || v.Received["BANK"] != 30000 {
		t.Fatal(v, err)
	}
	ratios, err := WeightedRatios([]int64{10000, 60000}, []int64{100000, 900000}, []int64{3600, 7200})
	if err != nil || ratios.PerKm != "70" || ratios.PerHour != "70000/3" {
		t.Fatal(ratios, err)
	}
	wrong := w
	wrong.Vehicle = "foreign"
	r, err = Result(45000, wrong, w, costs, true, nil, nil, nil)
	if err != nil || r.Recorded != nil || r.Unknown != "unmatched-period-window" {
		t.Fatal(r, err)
	}
	r, err = Result(45000, w, w, costs, false, nil, nil, nil)
	if err != nil || r.Recorded != nil || r.Unknown != "cost-coverage-incomplete" {
		t.Fatal(r, err)
	}
	if _, err = Result(45000, w, w, append(costs, costs[0]), true, nil, nil, nil); err == nil {
		t.Fatal("duplicate cost")
	}
	if _, err = WeightedRatios([]int64{MaxAmount, 1}, []int64{1, 1}, []int64{1, 1}); err == nil {
		t.Fatal("sum overflow")
	}
	for _, a := range []Adjustment{{"a", "target", "REVERSAL", 1, -100}, {"a", "target", "CORRECTION", 1, 100}} {
		if a.Validate() != nil {
			t.Fatal(a)
		}
	}
	for _, a := range []Adjustment{{"a", "target", "REVERSAL", 1, 100}, {"a", "target", "CORRECTION", 0, 1}, {"a", "target", "OTHER", 1, -1}} {
		if a.Validate() == nil {
			t.Fatal("bad adjustment", a)
		}
	}
}
