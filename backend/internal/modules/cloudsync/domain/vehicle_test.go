package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"testing"
)

func TestVehicleKeepsLegacyIdentityAndFuelPrecision(t *testing.T) {
	v := Vehicle{ID: "11111111-1111-4111-8111-111111111111", Name: "Work", CapacityMilli: 50000, Fuel: "GASOLINE_PREMIUM", Unit: "L", PriceSource: "CHEAPEST_STATION"}
	got, err := NormalizeVehicle(v)
	if err != nil || got.ID != v.ID || got.Fuel != kernel.GasolineAdditived {
		t.Fatal("legacy mapping", err)
	}
	for _, bad := range []Vehicle{
		{ID: v.ID, Name: "", CapacityMilli: 50000, Fuel: kernel.Ethanol, Unit: "L", PriceSource: v.PriceSource},
		{ID: v.ID, Name: "Work", CapacityMilli: 50001, Fuel: kernel.Ethanol, Unit: "L", PriceSource: v.PriceSource},
		{ID: v.ID, Name: "Work", CapacityMilli: 50000, Fuel: kernel.CNG, Unit: "L", PriceSource: v.PriceSource},
		{ID: v.ID, Name: "Work", CapacityMilli: 50000, Fuel: kernel.Ethanol, Unit: "L", PriceSource: "SPECIFIC_STATION"},
	} {
		if _, err := NormalizeVehicle(bad); err == nil {
			t.Fatal("invalid vehicle accepted")
		}
	}
}

func TestVehicleRegistrationV2RetainsOneFreeAndThreeCeiling(t *testing.T) {
	for _, tc := range []struct {
		count    int64
		multiple bool
		want     error
	}{
		{-1, false, ErrInvalid}, {0, false, nil}, {1, false, ErrDenied}, {2, false, ErrDenied}, {1, true, nil}, {2, true, nil}, {3, true, ErrQuota}, {4, true, ErrQuota}, {4, false, ErrQuota},
	} {
		if got := RequireVehicleRegistration(tc.count, tc.multiple); got != tc.want {
			t.Fatalf("count %d multiple %v got %v want %v", tc.count, tc.multiple, got, tc.want)
		}
	}
}
