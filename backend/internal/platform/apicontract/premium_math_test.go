package apicontract

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

// These reviewed vectors freeze future Go/Kotlin arithmetic; no ledger engine
// is implemented by this contract task.
func TestPremiumFinancialGoldenContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "testdata", "premium", "financial-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Version int `json:"version"`
		Cases   []struct {
			ID          string `json:"id"`
			Reason      string `json:"reason"`
			Distance    int64  `json:"distance_metres"`
			Quantity    int64  `json:"eligible_quantity_milli"`
			Cost        int64  `json:"eligible_cost_centavos"`
			Consumption string `json:"km_per_unit_rational"`
			CostPerKM   string `json:"centavos_per_km_rational"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.Version != 1 || len(data.Cases) < 12 {
		t.Fatal("missing reviewed cases")
	}
	seen := map[string]bool{}
	for _, v := range data.Cases {
		t.Run(v.ID, func(t *testing.T) {
			if v.ID == "" || seen[v.ID] {
				t.Fatal("empty/duplicate id")
			}
			seen[v.ID] = true
			if v.Reason != "" {
				if v.Consumption != "" || v.CostPerKM != "" {
					t.Fatal("UNKNOWN cannot carry invented metric")
				}
				return
			}
			if v.Distance <= 0 || v.Quantity <= 0 || v.Cost < 0 {
				t.Fatal("invalid complete interval")
			}
			got := new(big.Rat).SetFrac(big.NewInt(v.Distance), big.NewInt(v.Quantity)).RatString()
			if got != v.Consumption {
				t.Fatalf("consumption %s != %s", got, v.Consumption)
			}
			amount := new(big.Int).Mul(big.NewInt(v.Cost), big.NewInt(1000))
			got = new(big.Rat).SetFrac(amount, big.NewInt(v.Distance)).RatString()
			if got != v.CostPerKM {
				t.Fatalf("cost/km %s != %s", got, v.CostPerKM)
			}
		})
	}
}
