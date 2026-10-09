package domain

import "testing"

func TestPremiumGradeIsDistinctAndExact(t *testing.T) {
	p := validParams()
	p.Product = "GASOLINE_PREMIUM_GRADE"
	p.AmountMilli = 9190
	p.RawText = "9,190"
	obs, _, err := NewObservation(p)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Product != p.Product || obs.AmountMilli != 9190 || obs.RawText != "9,190" {
		t.Fatal("premium fact changed")
	}
	for _, unit := range []string{"M3", "KG_13", ""} {
		p.Unit = unit
		if _, _, err := NewObservation(p); err == nil {
			t.Fatalf("accepted unit %q", unit)
		}
	}
	p.Unit = "L"
	for _, amount := range []int64{-1, 0, 1000001} {
		p.AmountMilli = amount
		if _, _, err := NewObservation(p); err == nil {
			t.Fatalf("accepted amount %d", amount)
		}
	}
}
