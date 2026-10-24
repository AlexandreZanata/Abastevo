package domain

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func testVehicle() Vehicle {
	return Vehicle{
		ID: "aaaaaaaa-1111-4111-8111-000000000001", Name: "Test",
		CapacityMilli: 50000, Fuel: kernel.GasolineRegular, Unit: kernel.Liter,
		PriceSource: "CHEAPEST_STATION", SortOrder: 1,
	}
}

func strPtr(s string) *string { return &s }
func intPtr(n int64) *int64   { return &n }

func TestNormalizeVehicleAcceptsLegacyICEWithoutBattery(t *testing.T) {
	v, err := NormalizeVehicle(testVehicle())
	if err != nil {
		t.Fatal(err)
	}
	if v.Propulsion != nil || v.BatteryWh != nil {
		t.Fatal("legacy ICE gained energy fields")
	}
}

func TestNormalizeVehicleEnforcesPropulsionRules(t *testing.T) {
	bev := testVehicle()
	bev.Propulsion = strPtr(PropulsionBEV)
	bev.BatteryWh = intPtr(38000)
	bev.CapacityMilli = 0
	if _, err := NormalizeVehicle(bev); err != nil {
		t.Fatal("valid BEV refused", err)
	}
	bev.CapacityMilli = 50000
	if _, err := NormalizeVehicle(bev); err == nil {
		t.Fatal("BEV with tank accepted")
	}
	phev := testVehicle()
	phev.Propulsion = strPtr(PropulsionPHEV)
	phev.BatteryWh = intPtr(10000)
	if _, err := NormalizeVehicle(phev); err != nil {
		t.Fatal("valid PHEV refused", err)
	}
	phev.BatteryWh = nil
	if _, err := NormalizeVehicle(phev); err == nil {
		t.Fatal("PHEV without battery accepted")
	}
	hev := testVehicle()
	hev.Propulsion = strPtr(PropulsionHEV)
	if _, err := NormalizeVehicle(hev); err != nil {
		t.Fatal("valid HEV refused", err)
	}
	hev.BatteryWh = intPtr(1000)
	if _, err := NormalizeVehicle(hev); err == nil {
		t.Fatal("HEV with battery accepted")
	}
	bogus := testVehicle()
	bogus.Propulsion = strPtr("NUCLEAR")
	if _, err := NormalizeVehicle(bogus); err == nil {
		t.Fatal("unknown propulsion accepted")
	}
}

func TestMergeVehicleEnergyPreservesStoredFields(t *testing.T) {
	stored := testVehicle()
	stored.Propulsion = strPtr(PropulsionBEV)
	stored.BatteryWh = intPtr(38000)
	stored.CapacityMilli = 0
	incoming := testVehicle()
	incoming.CapacityMilli = 0
	merged := MergeVehicleEnergy(stored, incoming)
	if merged.Propulsion == nil || *merged.Propulsion != PropulsionBEV || merged.BatteryWh == nil || *merged.BatteryWh != 38000 {
		t.Fatal("stored energy erased", merged)
	}
	if _, err := NormalizeVehicle(merged); err != nil {
		t.Fatal("merged BEV invalid", err)
	}
	explicit := testVehicle()
	explicit.Propulsion = strPtr(PropulsionICE)
	explicit.BatteryWh = nil
	explicit.CapacityMilli = 50000
	merged = MergeVehicleEnergy(stored, explicit)
	if merged.Propulsion == nil || *merged.Propulsion != PropulsionICE {
		t.Fatal("explicit propulsion overridden")
	}
}
