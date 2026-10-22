package metrics

import "testing"

func TestEnergyExactSumsFirst(t *testing.T) {
	// Unequal sessions: A 10000Wh/100000m (100Wh/km), B 30000Wh/50000m
	// (600Wh/km). Sum-first gives 40000Wh/150000m = 800/3 Wh/km, never
	// the naive mean of ratios (350).
	got := EnergyInterval{PurchasedWh: 40000, CostCentavos: 4000, DistanceMetres: 150000, HasEnergy: true, Complete: true}.Compute()
	if got.Unknown != "" || got.WhPerKm != "800/3" || got.CentavosPerKm != "80/3" {
		t.Fatalf("weighted %+v", got)
	}
	if got.PurchasedWh != 40000 || got.CostCentavos != 4000 || got.DistanceMetres != 150000 {
		t.Fatalf("sums moved %+v", got)
	}
}

func TestEnergyUnknownPrecedence(t *testing.T) {
	cases := map[string]struct {
		in     EnergyInterval
		reason string
	}{
		"odo":        {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 50000, HasEnergy: true, Complete: true, OdometerMissing: true}, "odometer-missing"},
		"gap":        {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 50000, HasEnergy: true, Complete: true, HasGap: true}, "declared-fill-gap"},
		"segment":    {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 50000, HasEnergy: true, Complete: true, CrossesSegment: true}, "odometer-segment-boundary"},
		"soc":        {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 50000, HasEnergy: true, Complete: true, SocInconsistent: true}, "soc-inconsistent"},
		"phev":       {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 50000, HasEnergy: true, Complete: true, MixedCarrier: true}, "phev-shared-distance"},
		"zero":       {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 0, HasEnergy: true, Complete: true}, "distance-nonpositive"},
		"unmeasured": {EnergyInterval{CostCentavos: 1000, DistanceMetres: 50000, Complete: true}, "energy-unmeasured"},
		"partial":    {EnergyInterval{PurchasedWh: 10000, CostCentavos: 1000, DistanceMetres: 50000, HasEnergy: true}, "partial-measurement"},
	}
	for name, tc := range cases {
		got := tc.in.Compute()
		if got.Unknown != tc.reason || got.WhPerKm != "" || got.CentavosPerKm != "" {
			t.Fatalf("%s got %+v want %q", name, got, tc.reason)
		}
		if got.PurchasedWh != tc.in.PurchasedWh || got.CostCentavos != tc.in.CostCentavos {
			t.Fatalf("%s sums hidden %+v", name, got)
		}
	}
}

func TestEnergyBoundsRefuse(t *testing.T) {
	for _, in := range []EnergyInterval{
		{PurchasedWh: -1, DistanceMetres: 1},
		{PurchasedWh: MaxEnergyWh + 1, DistanceMetres: 1},
		{PurchasedWh: 1, CostCentavos: MaxTotalCentavos + 1, DistanceMetres: 1},
		{PurchasedWh: 1, DistanceMetres: MaxDistanceMetres + 1},
	} {
		if got := in.Compute(); got.Unknown != "input-out-of-bounds" {
			t.Fatalf("bounds admitted %+v", got)
		}
	}
}

func TestCombinedCostCountsOnce(t *testing.T) {
	got := CombinedCostPerKm(24000, 1275, 100000)
	if got.Unknown != "" || got.TotalCentavos != 25275 || got.CentavosPerKm != "1011/4" || got.Method != "combined-purchase-interval" {
		t.Fatalf("combined %+v", got)
	}
	if bad := CombinedCostPerKm(0, 0, 0); bad.Unknown != "distance-nonpositive" {
		t.Fatalf("zero distance %+v", bad)
	}
	if bad := CombinedCostPerKm(-1, 0, 1); bad.Unknown != "input-out-of-bounds" {
		t.Fatalf("negative fuel %+v", bad)
	}
}
