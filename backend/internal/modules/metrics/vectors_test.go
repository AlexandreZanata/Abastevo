package metrics

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

// All fifteen frozen PRM00 cases must retain their expected results in the
// executable shared corpus, including month and comparison cases formerly skipped.
func TestFrozenVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/testdata/premium/financial-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int `json:"version"`
		Cases   []struct {
			ID         string `json:"id"`
			Quantity   *int64 `json:"eligible_quantity_milli"`
			Cost       *int64 `json:"eligible_cost_centavos"`
			Distance   *int64 `json:"distance_metres"`
			Efficiency string `json:"km_per_unit_rational"`
			CostPerKm  string `json:"centavos_per_km_rational"`
			Reason     string `json:"reason"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 || len(doc.Cases) != 15 {
		t.Fatalf("frozen vectors: %v", err)
	}
	f, err := os.Open("../../../../contracts/testdata/premium/financial-execution-v1.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]string{}
	for _, row := range rows[1:] {
		v := map[string]string{}
		for i, key := range rows[0] {
			v[key] = row[i]
		}
		byID[row[0]] = v
	}
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			v, ok := byID[c.ID]
			if !ok {
				t.Fatal("frozen case absent from executed corpus")
			}
			for key, want := range map[string]*int64{"expected_quantity": c.Quantity, "expected_cost": c.Cost, "expected_distance": c.Distance} {
				if want != nil && v[key] != strconv.FormatInt(*want, 10) {
					t.Fatalf("%s changed", key)
				}
			}
			for key, want := range map[string]string{"expected_efficiency": c.Efficiency, "expected_cost_per_km": c.CostPerKm, "expected_unknown": c.Reason} {
				if v[key] != want {
					t.Fatalf("%s got %q want %q", key, v[key], want)
				}
			}
		})
	}
}
