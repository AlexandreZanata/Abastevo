package adapters

import (
	"errors"
	"testing"
)

func TestSandboxConfigGating(t *testing.T) {
	env := map[string]string{}
	stripe, asaas, err := readSandboxConfig(func(key string) string { return env[key] })
	if err != nil || stripe.journeysReady() || asaas.journeysReady() {
		t.Fatalf("empty config ready: %+v %+v err=%v", stripe, asaas, err)
	}
	// Live-looking values use the bare marker: the guard refuses any
	// "_live_" material, and the test itself must not carry live-key
	// literals the secret scanner would flag.
	for _, value := range []string{"synthetic_live_key", "other_live_value"} {
		env["ANPFUEL_SANDBOX_STRIPE_SECRET"] = value
		if _, _, err := readSandboxConfig(func(key string) string { return env[key] }); !errors.Is(err, errSandboxLiveRefused) {
			t.Fatalf("live value admitted: %q", value)
		}
		delete(env, "ANPFUEL_SANDBOX_STRIPE_SECRET")
	}
	env["ANPFUEL_SANDBOX_STRIPE_SECRET"] = "sk_test_synthetic"
	env["ANPFUEL_SANDBOX_STRIPE_PRICE"] = "price_synthetic"
	env["ANPFUEL_SANDBOX_STRIPE_MERCHANT"] = "acct_synthetic"
	stripe, _, err = readSandboxConfig(func(key string) string { return env[key] })
	if err != nil || !stripe.journeysReady() || !stripe.testKey() {
		t.Fatalf("stripe not ready: %+v err=%v", stripe, err)
	}
	if stripe.subscription != "" {
		t.Fatal("subscription defaulted")
	}
	env["ANPFUEL_SANDBOX_ASAAS_KEY"] = "synthetic"
	if _, asaas, err := readSandboxConfig(func(key string) string { return env[key] }); err != nil || asaas.journeysReady() {
		t.Fatalf("partial asaas ready: %+v err=%v", asaas, err)
	}
	env["ANPFUEL_SANDBOX_ASAAS_MERCHANT"] = "11111111-1111-4111-8111-111111111111"
	if _, asaas, err := readSandboxConfig(func(key string) string { return env[key] }); err != nil || !asaas.journeysReady() {
		t.Fatalf("asaas not ready: %+v err=%v", asaas, err)
	}
}

func TestSandboxOperationKeysArePerOperation(t *testing.T) {
	first := sandboxOperationKey("checkout", "11111111-1111-4111-8111-111111111111")
	if again := sandboxOperationKey("checkout", "11111111-1111-4111-8111-111111111111"); again != first {
		t.Fatalf("same operation changed key: %q vs %q", first, again)
	}
	second := sandboxOperationKey("checkout", "22222222-2222-4222-8222-222222222222")
	if second == first {
		t.Fatalf("independent operations share key %q", first)
	}
	if other := sandboxOperationKey("cancel", "11111111-1111-4111-8111-111111111111"); other == first {
		t.Fatalf("scopes share key %q", first)
	}
}
