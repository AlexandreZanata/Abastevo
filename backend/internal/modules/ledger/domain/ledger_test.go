package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	testID      = "11111111-1111-4111-8111-111111111111"
	testVehicle = "22222222-2222-4222-8222-222222222222"
	testStation = "33333333-3333-4333-8333-333333333333"
)

func testTime(t *testing.T) (time.Time, string) {
	t.Helper()
	at, err := time.Parse(time.RFC3339, "2026-03-15T10:30:00-03:00")
	if err != nil {
		t.Fatal(err)
	}
	return at, "America/Sao_Paulo"
}

func baseRefuel(t *testing.T) Refuel {
	t.Helper()
	at, tz := testTime(t)
	odometer := int64(1400000)
	return Refuel{ID: testID, VehicleID: testVehicle, OccurredAt: at, Timezone: tz, Fuel: kernel.Ethanol, Unit: kernel.Liter, QuantityMilli: 20000, TotalCentavos: 12400, UnitPriceMilli: 6200, OdometerMetres: &odometer, Fill: Full, StationID: testStation, Note: "Full tank"}
}

func TestRefuelExactValuesAndDiscrepancy(t *testing.T) {
	r, err := NormalizeRefuel(baseRefuel(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.QuantityMilli != 20000 || r.TotalCentavos != 12400 {
		t.Fatal("inputs rewritten")
	}
	if r.HasPriceDiscrepancy() {
		t.Fatal("exact price flagged")
	}
	discounted := baseRefuel(t)
	discounted.TotalCentavos = 11500
	if _, err = NormalizeRefuel(discounted); err != nil {
		t.Fatal(err)
	}
	if !discounted.HasPriceDiscrepancy() {
		t.Fatal("discount not flagged")
	}
	rounded := baseRefuel(t)
	rounded.UnitPriceMilli = 6199
	if _, err = NormalizeRefuel(rounded); err != nil {
		t.Fatal(err)
	}
	if !rounded.HasPriceDiscrepancy() {
		t.Fatal("rounding not flagged")
	}
	absent := baseRefuel(t)
	absent.UnitPriceMilli = 0
	if _, err = NormalizeRefuel(absent); err != nil {
		t.Fatal(err)
	}
	if absent.HasPriceDiscrepancy() {
		t.Fatal("absent price flagged")
	}
}

func TestRefuelBoundsAndUnits(t *testing.T) {
	at, tz := testTime(t)
	odometer := int64(0)
	maximum := Refuel{ID: testID, VehicleID: testVehicle, OccurredAt: at, Timezone: tz, Fuel: kernel.CNG, Unit: kernel.CubicMetre, QuantityMilli: MaxQuantityMilli, TotalCentavos: MaxTotalCentavos, UnitPriceMilli: MaxUnitPriceMilli, OdometerMetres: &odometer, Fill: Unknown}
	if _, err := NormalizeRefuel(maximum); err != nil {
		t.Fatal("bounds refused", err)
	}
	for _, mutate := range []func(*Refuel){
		func(r *Refuel) { r.QuantityMilli = 0 },
		func(r *Refuel) { r.QuantityMilli = MaxQuantityMilli + 1 },
		func(r *Refuel) { r.TotalCentavos = -1 },
		func(r *Refuel) { r.TotalCentavos = MaxTotalCentavos + 1 },
		func(r *Refuel) { r.UnitPriceMilli = -1 },
		func(r *Refuel) { r.UnitPriceMilli = MaxUnitPriceMilli + 1 },
		func(r *Refuel) { r.Fuel = kernel.LPGP13 },
		func(r *Refuel) { r.Unit = kernel.CubicMetre },
		func(r *Refuel) { r.Fuel = "AVIATION" },
		func(r *Refuel) { r.Fill = "empty" },
	} {
		r := baseRefuel(t)
		mutate(&r)
		if _, err := NormalizeRefuel(r); err == nil {
			t.Fatalf("invalid refuel admitted %+v", r)
		}
	}
}

func TestRefuelIncompleteInputs(t *testing.T) {
	r := baseRefuel(t)
	r.OdometerMetres = nil
	r.StationID = ""
	r.Note = ""
	r.Fill = Unknown
	if _, err := NormalizeRefuel(r); err != nil {
		t.Fatal("incomplete refused", err)
	}
	badOdometer := int64(-1)
	r.OdometerMetres = &badOdometer
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("negative odometer admitted")
	}
	huge := int64(MaxOdometerMetres + 1)
	r.OdometerMetres = &huge
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("overflowing odometer admitted")
	}
}

func TestRefuelTimeStationNote(t *testing.T) {
	r := baseRefuel(t)
	r.OccurredAt = time.Time{}
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("zero time admitted")
	}
	r = baseRefuel(t)
	r.Timezone = "Mars/Olympus"
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("unknown timezone admitted")
	}
	r = baseRefuel(t)
	r.StationID = "not-a-uuid"
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("bad station admitted")
	}
	r = baseRefuel(t)
	r.Note = strings.Repeat("n", MaxNoteRunes+1)
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("oversize note admitted")
	}
	r = baseRefuel(t)
	r.Note = "bad\x00note"
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("control note admitted")
	}
	r = baseRefuel(t)
	r.ID = "00000000-0000-0000-0000-000000000000"
	if _, err := NormalizeRefuel(r); err == nil {
		t.Fatal("nil id admitted")
	}
}

func TestExpenseCategoriesAndBounds(t *testing.T) {
	at, tz := testTime(t)
	for _, category := range []Category{Maintenance, Insurance, Taxes, Parking, Tolls, Cleaning, Other} {
		e, err := NormalizeExpense(Expense{ID: testID, VehicleID: testVehicle, OccurredAt: at, Timezone: tz, AmountCentavos: 5000, Category: category})
		if err != nil {
			t.Fatalf("category %s refused %v", category, err)
		}
		if e.AmountCentavos != 5000 {
			t.Fatal("amount rewritten")
		}
	}
	e, err := NormalizeExpense(Expense{ID: testID, VehicleID: testVehicle, OccurredAt: at, Timezone: tz, AmountCentavos: 0, Category: Other})
	if err != nil {
		t.Fatal("zero expense refused", err)
	}
	_ = e
	for _, category := range []Category{"fuel", "Food", "", "MAINTENANCE"} {
		if _, err := NormalizeExpense(Expense{ID: testID, VehicleID: testVehicle, OccurredAt: at, Timezone: tz, AmountCentavos: 100, Category: category}); err == nil {
			t.Fatalf("invalid category admitted %q", category)
		}
	}
	if _, err := NormalizeExpense(Expense{ID: testID, VehicleID: testVehicle, OccurredAt: at, Timezone: tz, AmountCentavos: MaxTotalCentavos + 1, Category: Tolls}); err == nil {
		t.Fatal("overflowing amount admitted")
	}
}
