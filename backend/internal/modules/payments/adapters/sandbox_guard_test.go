package adapters

import (
	"os"
	"strings"
	"testing"
)

// The sandbox harness handles real credentials when operators supply them:
// this guard pins that it can only address test endpoints and never logs
// identifying material, so a future edit cannot silently widen it.
func TestSandboxHarnessCannotLeakOrGoLive(t *testing.T) {
	raw, err := os.ReadFile("sandbox_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.HasPrefix(source, "//go:build sandbox\n") {
		t.Fatal("sandbox harness lost its build tag")
	}
	if strings.Count(source, "EnvironmentTest") < 2 {
		t.Fatal("harness must pin the test environment")
	}
	if strings.Contains(source, "EnvironmentProduction") {
		t.Fatal("harness references the production environment")
	}
	for _, name := range []string{
		"ANPFUEL_SANDBOX_STRIPE_SECRET", "ANPFUEL_SANDBOX_STRIPE_PRICE",
		"ANPFUEL_SANDBOX_STRIPE_MERCHANT", "ANPFUEL_SANDBOX_ASAAS_KEY",
		"ANPFUEL_SANDBOX_ASAAS_MERCHANT",
	} {
		if !strings.Contains(source, name) {
			t.Fatalf("credential gate %s undocumented", name)
		}
	}
	for _, line := range strings.Split(source, "\n") {
		if !strings.Contains(line, "t.Log") {
			continue
		}
		lowered := strings.ToLower(line)
		for _, marker := range []string{"secret", "apikey", "api_key", "token", "password", "passphrase", "merchant", "acct_", "cus_", "price_"} {
			if strings.Contains(lowered, marker) {
				t.Fatalf("harness logs identifying material: %q", strings.TrimSpace(line))
			}
		}
	}
}
