package adapters

import (
	"errors"
	"strings"
)

// Sandbox credential gating (PAY-21): pure, network-free and testable
// without credentials. The harness refuses live-looking material before
// any network use and pins the test environment by construction.

var errSandboxLiveRefused = errors.New("live credentials are refused by the sandbox harness")

type sandboxStripeConfig struct {
	secret       string
	price        string
	merchant     string
	portal       string
	subscription string
}

type sandboxAsaasConfig struct {
	key      string
	merchant string
}

func readSandboxConfig(getenv func(string) string) (sandboxStripeConfig, sandboxAsaasConfig, error) {
	stripe := sandboxStripeConfig{
		secret:       getenv("ANPFUEL_SANDBOX_STRIPE_SECRET"),
		price:        getenv("ANPFUEL_SANDBOX_STRIPE_PRICE"),
		merchant:     getenv("ANPFUEL_SANDBOX_STRIPE_MERCHANT"),
		portal:       getenv("ANPFUEL_SANDBOX_STRIPE_PORTAL"),
		subscription: getenv("ANPFUEL_SANDBOX_STRIPE_SUBSCRIPTION"),
	}
	asaas := sandboxAsaasConfig{
		key:      getenv("ANPFUEL_SANDBOX_ASAAS_KEY"),
		merchant: getenv("ANPFUEL_SANDBOX_ASAAS_MERCHANT"),
	}
	// Live-looking material is refused before any network use. The
	// pattern stays split so the guard itself never carries a live-key
	// literal the secret scanner could mistake for a credential.
	for _, value := range []string{stripe.secret, stripe.price, stripe.merchant, stripe.portal, stripe.subscription, asaas.key, asaas.merchant} {
		if strings.Contains(value, "_live_") {
			return sandboxStripeConfig{}, sandboxAsaasConfig{}, errSandboxLiveRefused
		}
	}
	return stripe, asaas, nil
}

func (c sandboxStripeConfig) journeysReady() bool {
	return c.secret != "" && c.price != "" && c.merchant != ""
}

func (c sandboxStripeConfig) testKey() bool {
	return strings.HasPrefix(c.secret, "sk_test_") || strings.HasPrefix(c.secret, "rk_test_")
}

func (c sandboxAsaasConfig) journeysReady() bool {
	return c.key != "" && c.merchant != ""
}
