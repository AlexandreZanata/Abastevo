package metrics

import (
	"encoding/json"
	"os"
	"testing"
)

// EE-23 golden parity: every vector in contracts/testdata/premium/energy-v1.json
// must compute identical exact sums, rational strings and unknown reasons here
// and in Kotlin (domain/premium/PremiumEnergyTest executes the same file).
func TestEnergyGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/testdata/premium/energy-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
		Cases   []struct {
			ID                 string `json:"id"`
			PurchasedWh        int64  `json:"purchased_wh"`
			CostCentavos       int64  `json:"cost_centavos"`
			DistanceMetres     int64  `json:"distance_metres"`
			HasEnergy          bool   `json:"has_energy"`
			Complete           bool   `json:"complete"`
			HasGap             bool   `json:"has_gap"`
			CrossesSegment     bool   `json:"crosses_segment"`
			OdometerMissing    bool   `json:"odometer_missing"`
			SocInconsistent    bool   `json:"soc_inconsistent"`
			MixedCarrier       bool   `json:"mixed_carrier"`
			WantWhPerKm        string `json:"expected_wh_per_km"`
			WantCentavosPerKm  string `json:"expected_centavos_per_km"`
			WantUnknown        string `json:"expected_unknown"`
			FuelCost           int64  `json:"fuel_cost_centavos"`
			ChargeCost         int64  `json:"charge_cost_centavos"`
			WantCombined       string `json:"expected_combined_centavos_per_km"`
			WantCombinedUnk    string `json:"expected_combined_unknown"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 || len(doc.Cases) == 0 {
		t.Fatalf("energy corpus unreadable: %v", err)
	}
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			if c.WantCombined != "" || c.WantCombinedUnk != "" {
				got := CombinedCostPerKm(c.FuelCost, c.ChargeCost, c.DistanceMetres)
				if got.CentavosPerKm != c.WantCombined || got.Unknown != c.WantCombinedUnk {
					t.Fatalf("combined got %+v", got)
				}
				return
			}
			got := EnergyInterval{
				PurchasedWh:     c.PurchasedWh,
				CostCentavos:    c.CostCentavos,
				DistanceMetres:  c.DistanceMetres,
				HasEnergy:       c.HasEnergy,
				Complete:        c.Complete,
				HasGap:          c.HasGap,
				CrossesSegment:  c.CrossesSegment,
				OdometerMissing: c.OdometerMissing,
				SocInconsistent: c.SocInconsistent,
				MixedCarrier:    c.MixedCarrier,
			}.Compute()
			if got.PurchasedWh != c.PurchasedWh || got.CostCentavos != c.CostCentavos || got.DistanceMetres != c.DistanceMetres {
				t.Fatalf("sums moved %+v", got)
			}
			if got.WhPerKm != c.WantWhPerKm || got.CentavosPerKm != c.WantCentavosPerKm || got.Unknown != c.WantUnknown {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
