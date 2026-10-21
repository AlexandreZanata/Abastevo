package apicontract

import (
	"encoding/csv"
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestEVPlannedContract(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(contractsDir(t), "openapi", "ev-support-planned.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Validate(loader.Context); err != nil {
		t.Fatal(err)
	}
	if doc.Paths.Len() != 0 || doc.Extensions["x-implementation-status"] != "CONTRACT_FROZEN_RUNTIME_PENDING" {
		t.Fatal("no live paths in a contract checkpoint")
	}
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "testdata", "ev", "support-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Version int      `json:"version"`
		Cases   []vector `json:"cases"`
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.Version != 1 || len(data.Cases) < 25 {
		t.Fatal("missing contract cases")
	}
	seen := map[string]bool{}
	for _, v := range data.Cases {
		t.Run(v.ID, func(t *testing.T) {
			if v.ID == "" || seen[v.ID] {
				t.Fatal("empty/duplicate case")
			}
			seen[v.ID] = true
			schema, ok := doc.Components.Schemas[v.Schema]
			if !ok {
				t.Fatal(v.Schema)
			}
			err := schema.Value.VisitJSON(v.Value)
			if (err == nil) != v.Valid {
				t.Fatalf("valid=%v: %v", v.Valid, err)
			}
		})
	}
	for _, id := range []string{"bev-no-liquid-tank", "phev-one-vehicle", "hev-plug-in-refused", "legacy-additive-preserved", "private-home-not-public", "unknown-fee-not-free", "static-not-live", "supply-not-battery", "private-gps-refused"} {
		if !seen[id] {
			t.Error("missing", id)
		}
	}
	var policy map[string]any
	raw, err = os.ReadFile(filepath.Join(contractsDir(t), "ev", "numeric-policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	if policy["runtime_math_engine_accepted"] != false || policy["source_admission_enabled"] != false || policy["rate_numerator_max"] != "1000000000000" || policy["rate_denominator_max"] != "1000000000" {
		t.Fatal("unverified runtime or changed exact rate limits")
	}
	for field, key := range map[string]string{"energy_wh": "energy_wh_max", "actual_total_centavos": "actual_money_centavos_max", "odometer_metres": "odometer_metres_max"} {
		maximum := doc.Components.Schemas["PrivateSession"].Value.Properties[field].Value.Max
		if maximum == nil || *maximum != policy[key].(float64) {
			t.Fatal("schema/policy ceiling mismatch", field)
		}
	}
}

// Exact arithmetic oracle for reviewed cross-language examples, not EE-11's
// production tariff/metrics engine or source/provider/window acceptance.
func TestEVExactGoldenVectors(t *testing.T) {
	f, err := os.Open(filepath.Join(contractsDir(t), "testdata", "ev", "exact-v1.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 20 {
		t.Fatal("missing numeric examples")
	}
	seen := map[string]bool{}
	for _, r := range rows[1:] {
		t.Run(r[0], func(t *testing.T) {
			if len(r) != 8 || seen[r[0]] {
				t.Fatal("invalid vector")
			}
			seen[r[0]] = true
			n := func(s string) *big.Int {
				v, ok := new(big.Int).SetString(s, 10)
				if !ok {
					t.Fatal("bad integer")
				}
				return v
			}
			a, b, c, d := n(r[2]), n(r[3]), n(r[4]), n(r[5])
			num, den := new(big.Int), big.NewInt(1)
			switch r[1] {
			case "WH_KM_KWH100":
				num.Set(a)
				den.SetInt64(10)
			case "WH_KM_MJ":
				num.Mul(a, big.NewInt(36))
				den.SetInt64(10000)
			case "ENERGY_COST":
				den.Mul(big.NewInt(1000), c)
				num.Add(new(big.Int).Mul(a, b), new(big.Int).Mul(d, den))
			case "COST_KM":
				num.Mul(a, big.NewInt(1000))
				den.Set(b)
			case "ENERGY100":
				num.Mul(a, big.NewInt(100))
				den.Set(b)
			case "TIME_COST":
				num.Mul(a, b)
				den.SetInt64(60)
			case "IDLE":
				bill := new(big.Int).Sub(a, b)
				if bill.Sign() < 0 {
					bill.SetInt64(0)
				}
				bill.Add(bill, new(big.Int).Sub(c, big.NewInt(1)))
				bill.Quo(bill, c)
				bill.Mul(bill, c)
				num.Mul(bill, d)
				den.SetInt64(60)
			case "SUM":
				num.Add(a, b)
				num.Add(num, c)
				num.Add(num, d)
			case "PHEV_COST":
				num.Mul(new(big.Int).Add(a, b), big.NewInt(1000))
				den.Set(c)
			case "DIFFERENCE":
				num.Sub(a, b)
			case "SOC_MODEL":
				num.Mul(a, b)
				den.SetInt64(10000)
			case "ROUND_HALF_UP":
				num.Add(new(big.Int).Mul(a, big.NewInt(2)), b)
				den.Mul(b, big.NewInt(2))
				num.Quo(num, den)
				den.SetInt64(1)
			default:
				t.Fatal("unknown operation", r[1])
			}
			if den.Sign() <= 0 {
				t.Fatal("invalid reviewed denominator")
			}
			got := new(big.Rat).SetFrac(num, den)
			want := new(big.Rat).SetFrac(n(r[6]), n(r[7]))
			if got.Cmp(want) != 0 {
				t.Fatalf("%s != %s", got, want)
			}
		})
	}
}
