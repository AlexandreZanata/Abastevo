package domain

import "testing"

func TestCentavoResidualUsesStableIDWithoutLosingMoney(t *testing.T) {
	got, e := Allocate(ptr(100), []Weight{{"b", 1}, {"c", 1}, {"a", 1}})
	if e != nil || got.Amounts["a"] != 34 || got.Amounts["b"] != 33 || got.Amounts["c"] != 33 {
		t.Fatalf("got=%+v err=%v; want a34 b33 c33", got, e)
	}
}

func TestAllocationLimitsCoverageAndDeterminism(t *testing.T) {
	for total := int64(0); total < 100; total++ {
		weights := []Weight{{"a", 3}, {"b", 7}, {"c", 0}}
		r, e := Allocate(ptr(total), weights)
		if e != nil {
			t.Fatal(e)
		}
		var sum int64
		for _, v := range r.Amounts {
			sum += v
		}
		if sum != total {
			t.Fatal("lost money", r)
		}
		r2, e := Allocate(ptr(total), []Weight{weights[2], weights[0], weights[1]})
		if e != nil || r2.Amounts["a"] != r.Amounts["a"] {
			t.Fatal("order dependent")
		}
	}
	r, e := Allocate(ptr(MaxAmount), []Weight{{"a", MaxAmount}, {"b", MaxAmount}})
	if e != nil || r.Amounts["a"] != MaxAmount/2 {
		t.Fatal(r, e)
	}
	r, e = Allocate(nil, []Weight{{"a", 1}})
	if e != nil || r.Unknown != "source-amount-missing" {
		t.Fatal(r, e)
	}
	r, e = Allocate(ptr(100), []Weight{{"a", 0}})
	if e != nil || r.Unknown != "allocation-basis-missing" {
		t.Fatal(r, e)
	}
	if _, e = Allocate(ptr(100), []Weight{{"a", 1}, {"a", 2}}); e == nil {
		t.Fatal("duplicate bucket")
	}
	r, e = Prorate(ptr(100), TimeSpan{0, 4}, []AllocationSpan{{"work", TimeSpan{0, 1}}})
	if e != nil || r.Amounts["work"] != 25 || r.Amounts["_unallocated"] != 75 {
		t.Fatal(r, e)
	}
	if _, e = Prorate(ptr(100), TimeSpan{0, 4}, []AllocationSpan{{"a", TimeSpan{0, 3}}, {"b", TimeSpan{2, 4}}}); e == nil {
		t.Fatal("overlap accepted")
	}
	// March DST transition has 23-hour days, but civil day allocation still gives one third.
	r, e = ProrateDays(ptr(300), "2026-03-07", "2026-03-10", "America/New_York", []CalendarSpan{{"work", "2026-03-08", "2026-03-09"}})
	if e != nil || r.Amounts["work"] != 100 || r.Amounts["_unallocated"] != 200 {
		t.Fatal(r, e)
	}
}
func TestProfilesAndReserveReplacement(t *testing.T) {
	p := VehicleCostProfile{Vehicle: "car", Ownership: "FINANCED", Timezone: "UTC", From: "2026-01-01", Version: 1, Costs: []ProfileCost{
		{ID: "principal", Kind: "PRINCIPAL", Provenance: "ACTUAL", LedgerSourceID: "principal-ledger", SourceRevision: 1, From: "2026-01-01", Amount: ptr(10000)},
		{ID: "interest", Kind: "INTEREST", Provenance: "ACTUAL", LedgerSourceID: "interest-ledger", SourceRevision: 1, From: "2026-01-01", Amount: ptr(1000)},
		{ID: "depreciation", Kind: "DEPRECIATION", Provenance: "MODELED", From: "2026-01-01", Amount: nil},
	}}
	b, e := p.At("2026-02-01")
	if e != nil || len(b.Capital) != 1 || len(b.Operating) != 1 || len(b.Modeled) != 1 {
		t.Fatal(b, e)
	}
	if n, e := SumProfileCosts(b.Modeled); e != nil || n != nil {
		t.Fatal("missing assumption became zero")
	}
	bad := p
	bad.Ownership = "LEASED"
	if bad.Validate() == nil {
		t.Fatal("lease principal/depreciation duplicated")
	}
	w := window()
	r, e := ReconcileReserves(w, []Reserve{{"reserve", ptr(8000), w}}, []Cost{{"service", 2000, w}}, []ReserveReplacement{{"reserve", "service"}})
	if e != nil || *r.AdditionalModeled != 0 || r.ActualRecorded != 2000 {
		t.Fatal(r, e)
	}
	r, e = ReconcileReserves(w, []Reserve{{"reserve", ptr(8000), w}}, []Cost{{"service", 3000, w}}, []ReserveReplacement{{"reserve", "service"}})
	if e != nil || *r.AdditionalModeled != 0 || r.ActualRecorded != 3000 {
		t.Fatal("correction doubled reserve", r, e)
	}
	r, e = ReconcileReserves(w, []Reserve{{"missing", nil, w}}, nil, nil)
	if e != nil || r.AdditionalModeled != nil {
		t.Fatal(r, e)
	}
	if _, e = ReconcileReserves(w, []Reserve{{"reserve", ptr(1), w}}, []Cost{{"service", 1, w}}, []ReserveReplacement{{"reserve", "other"}}); e == nil {
		t.Fatal("unmatched replacement accepted")
	}
}
