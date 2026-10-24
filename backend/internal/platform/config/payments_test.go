package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func stripeSecret(mode string) string {
	return "sk_" + mode + "_" + strings.Repeat("synthetic", 4)
}

func validPaymentEnv(env string) map[string]string {
	vars := map[string]string{
		keyEnv:                    env,
		keyDatabaseURL:            "postgres://app:synthetic@db.internal:5432/app?sslmode=require",
		keyCursorSecret:           strings.Repeat("c", 32),
		keyCanonicalHost:          "api.abastevo.com.br",
		keyPaymentsReturnOrigin:   "https://app.abastevo.com.br",
		keyStripeMode:             "test",
		keyStripeSecret:           stripeSecret("test"),
		keyStripeWebhook:          "whsec_" + strings.Repeat("w", 32),
		keyStripeMerchant:         "acct_synthetic",
		keyStripePrice:            "price_synthetic",
		keyStripeNewSales:         "true",
		keyAsaasMode:              "test",
		keyAsaasAPIKey:            strings.Repeat("a", 32),
		keyAsaasWebhook:           strings.Repeat("b", 32),
		keyAsaasMerchant:          "11111111-1111-4111-8111-111111111111",
		keyAsaasManualNewSales:    "true",
		keyAsaasAutomaticNewSales: "true",
	}
	if env == "production" {
		vars[keyStripeMode] = "live"
		vars[keyStripeSecret] = stripeSecret("live")
		vars[keyAsaasMode] = "production"
	}
	return vars
}

func paymentConfig(t *testing.T, vars map[string]string) Config {
	t.Helper()
	cfg, err := load(envFunc(vars))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func TestPaymentsDefaultDisabledButMaintenanceSwitchIndependent(t *testing.T) {
	cfg := paymentConfig(t, nil)
	preflight := cfg.Payments.Preflight()
	for _, channel := range []PaymentChannelPreflight{preflight.Stripe, preflight.PixManual, preflight.PixAutomatic} {
		if channel.NewSalesRequested || channel.NewSalesReady {
			t.Fatalf("%s sales defaulted on: %+v", channel.Channel, channel)
		}
		if channel.Reason != PaymentReasonDisabledByFlag {
			t.Fatalf("%s reason = %s", channel.Channel, channel.Reason)
		}
		if channel.MaintenanceReady {
			t.Fatalf("%s maintenance ready without credentials: %+v", channel.Channel, channel)
		}
	}
	if cfg.Payments.Stripe.MaintenanceEnabled != true || cfg.Payments.Asaas.MaintenanceEnabled != true {
		t.Fatal("maintenance must default on")
	}
}

func TestPaymentsFullTestConfigReady(t *testing.T) {
	cfg := paymentConfig(t, validPaymentEnv("staging"))
	preflight := cfg.Payments.Preflight()
	if !preflight.Stripe.NewSalesReady || !preflight.Stripe.MaintenanceReady {
		t.Fatalf("stripe = %+v", preflight.Stripe)
	}
	if !preflight.PixManual.NewSalesReady || !preflight.PixManual.MaintenanceReady {
		t.Fatalf("pix manual = %+v", preflight.PixManual)
	}
	if !preflight.PixAutomatic.NewSalesReady || !preflight.PixAutomatic.MaintenanceReady {
		t.Fatalf("pix automatic = %+v", preflight.PixAutomatic)
	}
	if preflight.Stripe.Reason != "" || preflight.PixManual.Reason != "" || preflight.PixAutomatic.Reason != "" ||
		preflight.Stripe.MaintenanceReason != "" || preflight.PixManual.MaintenanceReason != "" {
		t.Fatalf("unexpected reasons: %+v", preflight)
	}
}

func TestPaymentsSalesOffPreservesMaintenance(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyStripeNewSales] = "false"
	vars[keyAsaasManualNewSales] = "false"
	vars[keyAsaasAutomaticNewSales] = "false"
	preflight := paymentConfig(t, vars).Payments.Preflight()
	if !preflight.Stripe.MaintenanceReady || !preflight.PixManual.MaintenanceReady || !preflight.PixAutomatic.MaintenanceReady {
		t.Fatalf("maintenance lost when sales off: %+v", preflight)
	}
	if preflight.Stripe.NewSalesReady || preflight.PixManual.NewSalesReady || preflight.PixAutomatic.NewSalesReady {
		t.Fatalf("sales ready while off: %+v", preflight)
	}
	if preflight.Stripe.Reason != PaymentReasonDisabledByFlag {
		t.Fatalf("stripe reason = %s", preflight.Stripe.Reason)
	}
}

func TestPaymentsMixedModeRefusesOnlyUnsafeChannel(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyStripeMode] = "live"
	vars[keyStripeSecret] = stripeSecret("live")
	preflight := paymentConfig(t, vars).Payments.Preflight()
	if preflight.Stripe.NewSalesReady || preflight.Stripe.MaintenanceReady {
		t.Fatalf("staging accepted live stripe: %+v", preflight.Stripe)
	}
	if preflight.Stripe.Reason != PaymentReasonModeMismatch ||
		preflight.Stripe.MaintenanceReason != PaymentReasonModeMismatch {
		t.Fatalf("stripe reasons = %s / %s", preflight.Stripe.Reason, preflight.Stripe.MaintenanceReason)
	}
	if !preflight.PixManual.NewSalesReady || !preflight.PixAutomatic.NewSalesReady {
		t.Fatalf("mixed stripe disabled safe asaas: %+v", preflight)
	}

	prod := validPaymentEnv("production")
	prod[keyAsaasMode] = "test"
	preflight = paymentConfig(t, prod).Payments.Preflight()
	if preflight.PixManual.NewSalesReady || preflight.PixAutomatic.NewSalesReady {
		t.Fatalf("production accepted test asaas: %+v", preflight)
	}
	if !preflight.Stripe.NewSalesReady {
		t.Fatalf("production live stripe not ready: %+v", preflight.Stripe)
	}
}

func TestPaymentsMissingAndMalformedValuesFailClosedWithoutCrash(t *testing.T) {
	vars := validPaymentEnv("staging")
	delete(vars, keyStripeSecret)
	vars[keyStripeNewSales] = "not-a-bool"
	cfg := paymentConfig(t, vars)
	preflight := cfg.Payments.Preflight()
	if preflight.Stripe.NewSalesReady || preflight.Stripe.MaintenanceReady {
		t.Fatalf("malformed stripe ready: %+v", preflight.Stripe)
	}
	if !containsIssue(preflight.Stripe.Issues, keyStripeSecret) ||
		!containsIssue(preflight.Stripe.Issues, keyStripeNewSales) {
		t.Fatalf("issues = %v", preflight.Stripe.Issues)
	}
	if !preflight.PixManual.NewSalesReady {
		t.Fatalf("malformed stripe disabled asaas: %+v", preflight.PixManual)
	}
}

func TestPaymentsMissingReturnOriginDisablesCardOnly(t *testing.T) {
	vars := validPaymentEnv("staging")
	delete(vars, keyPaymentsReturnOrigin)
	preflight := paymentConfig(t, vars).Payments.Preflight()
	if preflight.Stripe.NewSalesReady || preflight.Stripe.Reason != PaymentReasonMissingReturnOrigin {
		t.Fatalf("stripe = %+v", preflight.Stripe)
	}
	if !preflight.PixManual.NewSalesReady || !preflight.PixAutomatic.NewSalesReady {
		t.Fatalf("missing card origin disabled pix: %+v", preflight)
	}
}

func TestPaymentsInvalidOriginRejectedOutsideDevelopment(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:8080", "https://app.example/path", "https://app.example?x=1"} {
		vars := validPaymentEnv("staging")
		vars[keyPaymentsReturnOrigin] = origin
		preflight := paymentConfig(t, vars).Payments.Preflight()
		if preflight.Stripe.NewSalesReady {
			t.Fatalf("origin %q accepted", origin)
		}
	}
	vars := validPaymentEnv("development")
	vars[keyPaymentsReturnOrigin] = "http://127.0.0.1:8080"
	if !paymentConfig(t, vars).Payments.Preflight().Stripe.NewSalesReady {
		t.Fatal("development loopback origin refused")
	}
}

func TestPaymentsCatalogRequiredForSales(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyPaymentsCatalogVersion] = ""
	preflight := paymentConfig(t, vars).Payments.Preflight()
	if preflight.Stripe.NewSalesReady || preflight.Stripe.Reason != PaymentReasonMissingCatalog {
		t.Fatalf("stripe = %+v", preflight.Stripe)
	}
	if preflight.PixManual.NewSalesReady || preflight.PixManual.Reason != PaymentReasonMissingCatalog {
		t.Fatalf("pix = %+v", preflight.PixManual)
	}
	if !preflight.Stripe.MaintenanceReady || !preflight.PixManual.MaintenanceReady {
		t.Fatal("missing catalog must not disable maintenance")
	}
}

func TestPaymentsWebhookTokenCannotReuseAPIKey(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyAsaasWebhook] = vars[keyAsaasAPIKey]
	preflight := paymentConfig(t, vars).Payments.Preflight()
	if preflight.PixManual.NewSalesReady || preflight.PixManual.MaintenanceReady {
		t.Fatalf("reused token ready: %+v", preflight.PixManual)
	}
	if !containsIssue(preflight.PixManual.Issues, keyAsaasWebhook) {
		t.Fatalf("issues = %v", preflight.PixManual.Issues)
	}
}

func TestPaymentsMaintenanceOffDisablesSalesForThatProvider(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyStripeMaintenance] = "false"
	preflight := paymentConfig(t, vars).Payments.Preflight()
	if preflight.Stripe.MaintenanceReady || preflight.Stripe.NewSalesReady {
		t.Fatalf("stripe = %+v", preflight.Stripe)
	}
	if preflight.Stripe.Reason != PaymentReasonMaintenanceDisabled ||
		preflight.Stripe.MaintenanceReason != PaymentReasonMaintenanceDisabled {
		t.Fatalf("stripe reasons = %s / %s", preflight.Stripe.Reason, preflight.Stripe.MaintenanceReason)
	}
	if !preflight.PixManual.NewSalesReady {
		t.Fatalf("stripe maintenance disabled asaas: %+v", preflight.PixManual)
	}
}

func TestPaymentsSecretsAreNotPreflightedOrLogged(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyStripeSecret] = stripeSecret("test") + "TOP_SECRET_STRIPE"
	vars[keyStripeWebhook] = "whsec_" + strings.Repeat("TOP_SECRET_WEBHOOK", 3)
	vars[keyAsaasAPIKey] = strings.Repeat("A", 24) + "TOP_SECRET_ASAAS"
	vars[keyAsaasWebhook] = strings.Repeat("B", 24) + "TOP_SECRET_ASAAS_WEBHOOK"
	cfg := paymentConfig(t, vars)
	preflightJSON, err := json.Marshal(cfg.Payments.Preflight())
	if err != nil {
		t.Fatal(err)
	}
	logged := fmt.Sprintf("%v", cfg.LogValue())
	for _, secret := range []string{
		vars[keyStripeSecret], vars[keyStripeWebhook], vars[keyAsaasAPIKey], vars[keyAsaasWebhook],
		"TOP_SECRET_STRIPE", "TOP_SECRET_WEBHOOK", "TOP_SECRET_ASAAS",
	} {
		if strings.Contains(string(preflightJSON), secret) {
			t.Fatalf("preflight leaked %q", secret)
		}
		if strings.Contains(logged, secret) {
			t.Fatalf("log value leaked %q", secret)
		}
	}
}

func TestPaymentsRotationKeepsReadyAndRedacted(t *testing.T) {
	firstVars := validPaymentEnv("staging")
	firstVars[keyStripeSecret] = stripeSecret("test") + "FIRST"
	first := paymentConfig(t, firstVars)
	secondVars := validPaymentEnv("staging")
	secondVars[keyStripeSecret] = stripeSecret("test") + "SECOND"
	second := paymentConfig(t, secondVars)
	if first.Payments.Stripe.SecretKey == second.Payments.Stripe.SecretKey {
		t.Fatal("rotation did not load the new key")
	}
	if !first.Payments.Preflight().Stripe.NewSalesReady || !second.Payments.Preflight().Stripe.NewSalesReady {
		t.Fatal("rotation broke readiness")
	}
	firstJSON, _ := json.Marshal(first.Payments.Preflight())
	secondJSON, _ := json.Marshal(second.Payments.Preflight())
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("preflight exposed rotation-sensitive material")
	}
}

func containsIssue(issues []string, key string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, key) {
			return true
		}
	}
	return false
}

func TestPaymentsAcceptsStripeRestrictedKey(t *testing.T) {
	vars := validPaymentEnv("staging")
	vars[keyStripeSecret] = "rk_" + "test_" + strings.Repeat("synthetic", 4)
	if !paymentConfig(t, vars).Payments.Preflight().Stripe.NewSalesReady {
		t.Fatal("restricted Stripe key refused")
	}
}
