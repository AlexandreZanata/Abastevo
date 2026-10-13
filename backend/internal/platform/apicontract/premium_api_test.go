package apicontract

import (
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3"
	"os"
	"path/filepath"
	"testing"
)

func TestPremiumCurrentAndPlannedContracts(t *testing.T) {
	doc := loadSpec(t)
	for _, path := range []string{"/v1/premium/status", "/v1/premium/export", "/v1/premium/purchases/verify", "/v1/premium/purchases/restore", "/v1/premium/provider-notifications"} {
		if doc.Paths.Find(path) == nil {
			t.Fatalf("missing frozen route %s", path)
		}
	}
	loader := openapi3.NewLoader()
	future, err := loader.LoadFromFile(filepath.Join(contractsDir(t), "openapi", "premium-planned.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = future.Validate(loader.Context); err != nil {
		t.Fatal(err)
	}
	if len(future.Paths.Map()) < 10 {
		t.Fatal("cloud/professional contracts incomplete")
	}
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "premium", "policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err = json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	if policy["sales_enabled"] != false || policy["initial_global_paid_admission_budget"] != float64(0) {
		t.Fatal("real sales must remain off")
	}
}
