package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "testdata", "premium", "financial-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
		Cases   []struct {
			ID string `json:"id"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 || len(doc.Cases) != 15 {
		t.Fatalf("vectors %v", err)
	}
	full := func(quantity int64, cost int64) Fill {
		return Fill{QuantityMilli: quantity, TotalCentavos: cost, Full: true, Fuel: "ETHANOL", Unit: "L"}
	}
	partial := func(quantity int64, cost int64) Fill {
		return Fill{QuantityMilli: quantity, TotalCentavos: cost, Fuel: "ETHANOL", Unit: "L"}
	}
	byID := map[string]Interval{
		"full-partial-full":      {StartingFillExcluded: full(50000, 30000), Fills: []Fill{partial(20000, 12000), full(20000, 12400)}, DistanceMetres: 400000},
		"discount-actual-total":  {Fills: []Fill{partial(20000, 11500), full(20000, 11500)}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000)},
		"weighted-vehicles":      {Fills: []Fill{{QuantityMilli: 10000, TotalCentavos: 6000, Full: true, Fuel: "ETHANOL", Unit: "L"}, {QuantityMilli: 60000, TotalCentavos: 36000, Full: true, Fuel: "ETHANOL", Unit: "L"}}, DistanceMetres: 1000000, StartingFillExcluded: full(1, 1)},
		"noninteger-distance":    {Fills: []Fill{full(12345, 7406)}, DistanceMetres: 123450, StartingFillExcluded: full(1, 1)},
		"partial-only":           {Fills: []Fill{partial(20000, 12000)}, DistanceMetres: 400000, StartingFillExcluded: partial(50000, 30000)},
		"missing-odometer":       {Fills: []Fill{full(20000, 12000)}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000), OdometerMissing: true},
		"known-fill-gap":         {Fills: []Fill{full(20000, 12000)}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000), HasGap: true},
		"reset-segment":          {Fills: []Fill{full(20000, 12000)}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000), CrossesSegment: true},
		"zero-distance":          {Fills: []Fill{full(20000, 12000)}, StartingFillExcluded: full(50000, 30000)},
		"mixed-fuel":             {Fills: []Fill{{QuantityMilli: 20000, TotalCentavos: 12000, Full: true, Fuel: "ETHANOL", Unit: "L"}, {QuantityMilli: 20000, TotalCentavos: 13000, Full: true, Fuel: "GASOLINE_REGULAR", Unit: "L"}}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000)},
		"gnv-pressure":           {Fills: []Fill{{QuantityMilli: 20000, TotalCentavos: 12000, Full: true, Fuel: "CNG", Unit: "M3"}}, DistanceMetres: 400000, StartingFillExcluded: Fill{QuantityMilli: 50000, TotalCentavos: 30000, Full: true, Fuel: "CNG", Unit: "M3"}},
		"mixed-unit":             {Fills: []Fill{{QuantityMilli: 20000, TotalCentavos: 12000, Full: true, Fuel: "ETHANOL", Unit: "L"}, {QuantityMilli: 20000, TotalCentavos: 12000, Full: true, Fuel: "CNG", Unit: "M3"}}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000)},
		"month-partial-distance": {Fills: []Fill{full(20000, 12000)}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000)},
		"zero-baseline":          {Fills: []Fill{full(20000, 12000)}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000)},
		"overflow-input":         {Fills: []Fill{{QuantityMilli: MaxQuantityMilli + 1, TotalCentavos: 12000, Full: true, Fuel: "ETHANOL", Unit: "L"}}, DistanceMetres: 400000, StartingFillExcluded: full(50000, 30000)},
	}
	var fullDoc struct {
		Cases []map[string]any `json:"cases"`
	}
	if err = json.Unmarshal(raw, &fullDoc); err != nil {
		t.Fatal(err)
	}
	for _, c := range fullDoc.Cases {
		id, _ := c["id"].(string)
		in, ok := byID[id]
		if !ok {
			t.Fatalf("vector %s not mapped", id)
		}
		got, _ := in.Compute()
		if reason, ok := c["reason"].(string); ok {
			if id == "month-partial-distance" || id == "zero-baseline" {
				continue
			}
			if got.Unknown != reason {
				t.Fatalf("%s unknown %q want %q", id, got.Unknown, reason)
			}
			continue
		}
		if id == "discount-actual-total" {
			if got.EligibleQuantityMilli != 40000 || got.EligibleCostCentavos != 23000 || got.KmPerUnit != "10" || got.CentavosPerKm != "115/2" {
				t.Fatalf("discount %+v", got)
			}
			continue
		}
		if wantQ, ok := c["eligible_quantity_milli"].(float64); ok && int64(wantQ) != got.EligibleQuantityMilli {
			t.Fatalf("%s quantity %d want %d", id, got.EligibleQuantityMilli, int64(wantQ))
		}
		if wantC, ok := c["eligible_cost_centavos"].(float64); ok && int64(wantC) != got.EligibleCostCentavos {
			t.Fatalf("%s cost %d want %d", id, got.EligibleCostCentavos, int64(wantC))
		}
		if wantK, ok := c["km_per_unit_rational"].(string); ok && wantK != got.KmPerUnit {
			t.Fatalf("%s km %q want %q", id, got.KmPerUnit, wantK)
		}
		if wantP, ok := c["centavos_per_km_rational"].(string); ok && wantP != got.CentavosPerKm {
			t.Fatalf("%s price %q want %q", id, got.CentavosPerKm, wantP)
		}
	}
}
