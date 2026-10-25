package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	paymentsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func pay19EmptyConfig() config.Config {
	return config.Config{}
}

func TestPAY19UnconfiguredRuntimeFailsClosed(t *testing.T) {
	comp := composePayments(pay19EmptyConfig().Payments, nil, time.Now, nil)
	if comp.Stripe != nil || comp.Asaas != nil {
		t.Fatal("providers exist without credentials")
	}
	if comp.Catalog.Version != payCatalogVersion || len(comp.Catalog.Offers) != 0 {
		t.Fatalf("catalog=%+v", comp.Catalog)
	}
	if comp.Preflight.Stripe.MaintenanceReady || comp.Preflight.PixManual.MaintenanceReady ||
		comp.Preflight.PixAutomatic.MaintenanceReady || comp.Preflight.Stripe.NewSalesReady ||
		comp.Preflight.PixManual.NewSalesReady || comp.Preflight.PixAutomatic.NewSalesReady {
		t.Fatalf("preflight ready without configuration: %+v", comp.Preflight)
	}
	principal := paymentsapp.Principal{AccountID: "11111111-1111-4111-8111-111111111111"}
	if _, err := comp.Checkout.Start(context.Background(), principal, paymentsapp.StartRequest{
		OperationID: "11111111-1111-4111-8111-111111111112", OfferID: "premium.monthly",
	}); err == nil {
		t.Fatal("card checkout started without configuration")
	}
	if _, err := comp.Pix.Start(context.Background(), principal, paymentsapp.PixStartRequest{
		OperationID: "11111111-1111-4111-8111-111111111112", OfferID: "premium.monthly",
	}); err == nil {
		t.Fatal("pix checkout started without configuration")
	}
	if _, err := comp.Switch.Request(context.Background(), principal, paymentsapp.SwitchRequest{
		OperationID: "11111111-1111-4111-8111-111111111112", OfferID: "premium.monthly",
	}); err == nil {
		t.Fatal("switch requested without configuration")
	}
	if _, err := comp.Portal.Manage(context.Background(), principal, "premium.monthly"); err == nil {
		t.Fatal("portal opened without configuration")
	}
	if _, err := comp.Billing.Cancel(context.Background(), principal,
		"11111111-1111-4111-8111-111111111112", "proof"); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("owner cancel err=%v", err)
	}
	if _, err := comp.Refunds.Request(context.Background(), paymentsapp.RefundRequest{
		Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "merchant", RemotePaymentID: "pay", Reason: "reason",
		ActorID: "op", Proof: "proof", OperationKey: "op",
	}); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("refund err=%v", err)
	}
}

func TestPAY19WebhookRoutesMountedWithoutCommandSurfaces(t *testing.T) {
	comp := composePayments(pay19EmptyConfig().Payments, nil, time.Now, nil)
	router := chi.NewRouter()
	registerPaymentRoutes(router, comp)
	found := map[string]string{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		found[method+" "+route] = route
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		"POST /v1/payments/stripe/webhook",
		"POST /v1/payments/asaas/webhook",
	} {
		if _, ok := found[route]; !ok {
			t.Fatalf("missing route %q in %v", route, found)
		}
	}
	if len(found) != 2 {
		t.Fatalf("unexpected payment routes: %v", found)
	}
	// Unconfigured intake fails closed with a static code and no-store.
	body := []byte(`{"id":"evt","event":"PAYMENT_RECEIVED"}`)
	for path, token := range map[string]string{
		"/v1/payments/stripe/webhook": "",
		"/v1/payments/asaas/webhook":  "synthetic-token-but-long-enough-123456",
	} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("asaas-access-token", token)
		}
		if token == "" {
			request.Header.Set("Stripe-Signature", "synthetic")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code/100 == 2 {
			t.Fatalf("%s acknowledged without configuration: %d", path, response.Code)
		}
		if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
			t.Fatalf("%s Cache-Control=%q", path, cache)
		}
	}
}

func TestPAY19RuntimeHasNoPlayOrCookieSurface(t *testing.T) {
	markers := []string{"google", "playstore", "play-store", "play-billing", "androidpublisher", "setcookie", "readcookie", "r.cookie", "cookiejar", "http.redirect"}
	for _, file := range []string{"payments.go", "../worker/payments.go", "../worker/dispatchers.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lowered := strings.ToLower(string(raw))
		for _, marker := range markers {
			if strings.Contains(lowered, marker) {
				t.Fatalf("%s references forbidden surface %q", file, marker)
			}
		}
	}
	// main.go keeps its pre-existing subscription Play wiring; the PAY-19
	// payment lines must not add any Play, cookie or redirect surface.
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		lowered := strings.ToLower(line)
		if !strings.Contains(lowered, "pay") {
			continue
		}
		for _, marker := range markers {
			if strings.Contains(lowered, marker) {
				t.Fatalf("main.go payment line references %q: %q", marker, strings.TrimSpace(line))
			}
		}
	}
	for _, file := range []string{
		"../../internal/modules/payments/adapters/jobs/router.go",
		"../../internal/modules/payments/adapters/verify_mux.go",
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		lowered := strings.ToLower(string(raw))
		for _, marker := range []string{"google", "playstore", "play-store", "play-billing", "androidpublisher", "setcookie", "readcookie", "r.cookie", "cookiejar"} {
			if strings.Contains(lowered, marker) {
				t.Fatalf("%s references forbidden surface %q", file, marker)
			}
		}
	}
}

func pay20DualConfig() config.PaymentsConfig {
	return config.PaymentsConfig{
		CatalogVersion: payCatalogVersion,
		ReturnOrigin:   "https://app.example.invalid",
		Stripe: config.StripePaymentConfig{
			Mode: "test", ExpectedMode: "test",
			SecretKey:     "sk_test_" + strings.Repeat("synthetic", 4),
			WebhookSecret: "whsec_" + strings.Repeat("w", 32),
			MerchantID:    "acct_synthetic_merchant", PriceID: "price_synthetic",
			PortalConfigID: "bpc_synthetic_portal", MaintenanceEnabled: true,
			Configured: true,
		},
		Asaas: config.AsaasPaymentConfig{
			Mode: "test", ExpectedMode: "test",
			APIKey: strings.Repeat("a", 32), WebhookToken: strings.Repeat("b", 32),
			MerchantID:         "11111111-1111-4111-8111-111111111111",
			MaintenanceEnabled: true, Configured: true,
		},
	}
}

func TestPAY20OwnerProofVerifiesLiveSessionPerAction(t *testing.T) {
	const account = "11111111-1111-4111-8111-111111111111"
	live := accountOwnerProof{validate: func(_ context.Context, family, token string) (string, error) {
		if family == "family-live" && token == "token-live" {
			return account, nil
		}
		if family == "family-foreign" && token == "token-live" {
			return "99999999-9999-4999-8999-999999999999", nil
		}
		return "", accountdomain.ErrCodeUnknown
	}}
	if err := live.VerifyFreshOwner(context.Background(), account, "family-live:token-live", "billing.cancel"); err != nil {
		t.Fatalf("live proof err=%v", err)
	}
	for name, proof := range map[string]string{
		"empty":        "",
		"no-separator": "family-livetoken-live",
		"empty-family": ":token-live",
		"empty-token":  "family-live:",
		"extra-colon":  "family-live:token:live",
		"unknown":      "family-nope:token-nope",
		"foreign":      "family-foreign:token-live",
	} {
		if err := live.VerifyFreshOwner(context.Background(), account, proof, "billing.cancel"); !errors.Is(err, paydomain.ErrUnauthorized) {
			t.Fatalf("%s err=%v", name, err)
		}
	}
	if err := live.VerifyFreshOwner(context.Background(), "", "family-live:token-live", "billing.cancel"); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("empty account err=%v", err)
	}
	if err := live.VerifyFreshOwner(context.Background(), account, "family-live:token-live", ""); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("empty action err=%v", err)
	}
	outage := accountOwnerProof{validate: func(context.Context, string, string) (string, error) {
		return "", errors.New("synthetic transport failure")
	}}
	if err := outage.VerifyFreshOwner(context.Background(), account, "family-live:token-live", "billing.cancel"); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("outage err=%v", err)
	}
	revoked := accountOwnerProof{validate: func(context.Context, string, string) (string, error) {
		return "", accountdomain.ErrSessionRevoked
	}}
	if err := revoked.VerifyFreshOwner(context.Background(), account, "family-live:token-live", "billing.cancel"); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("revoked err=%v", err)
	}
	if err := (accountOwnerProof{}).VerifyFreshOwner(context.Background(), account, "family-live:token-live", "billing.cancel"); !errors.Is(err, paydomain.ErrUnauthorized) {
		t.Fatalf("nil closure err=%v", err)
	}
}

func TestPAY20MaintenanceOffDarkensChannel(t *testing.T) {
	cfg := pay20DualConfig()
	cfg.Asaas.MaintenanceEnabled = false
	comp := composePayments(cfg, nil, time.Now, nil)
	if comp.Asaas != nil {
		t.Fatal("provider exists with maintenance off")
	}
	if comp.Preflight.PixManual.MaintenanceReady {
		t.Fatal("preflight ready with maintenance off")
	}
	if comp.Preflight.PixManual.MaintenanceReason != config.PaymentReasonMaintenanceDisabled {
		t.Fatalf("reason=%q", comp.Preflight.PixManual.MaintenanceReason)
	}
	if comp.Stripe == nil {
		t.Fatal("maintenance-off Asaas darkened Stripe")
	}
	// Intake refuses despite present credentials; the sibling channel is
	// untouched.
	router := chi.NewRouter()
	registerPaymentRoutes(router, comp)
	body := []byte(`{"id":"evt","event":"PAYMENT_RECEIVED","account":{"id":"11111111-1111-4111-8111-111111111111"},"payment":{"id":"pay"}}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/payments/asaas/webhook", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("asaas-access-token", strings.Repeat("b", 32))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("dark channel status=%d", response.Code)
	}
}

func TestPAY20DualChannelMerchantsAndPortalWired(t *testing.T) {
	comp := composePayments(pay20DualConfig(), nil, time.Now, nil)
	if comp.Stripe == nil || comp.Asaas == nil {
		t.Fatal("dual providers missing")
	}
	if comp.Switch.Config.CardMerchantID != "acct_synthetic_merchant" ||
		comp.Switch.Config.PixAutoMerchantID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("switch merchants=%+v", comp.Switch.Config)
	}
	if comp.Portal.Config.ConfigurationID != "bpc_synthetic_portal" {
		t.Fatalf("portal config=%+v", comp.Portal.Config)
	}
	if !comp.Preflight.Stripe.MaintenanceReady || !comp.Preflight.PixManual.MaintenanceReady ||
		!comp.Preflight.PixAutomatic.MaintenanceReady {
		t.Fatalf("preflight=%+v", comp.Preflight)
	}
	if !comp.Preflight.Stripe.PortalConfigured {
		t.Fatal("portal not reported configured")
	}
	// Mixed modes refuse with actionable codes and no provider (issue
	// text itself is covered against the real loader in the config
	// package; composition consumes readiness and reasons).
	mixed := pay20DualConfig()
	mixed.Stripe.Mode = "live"
	mixedComp := composePayments(mixed, nil, time.Now, nil)
	if mixedComp.Stripe != nil {
		t.Fatal("mixed-mode provider exists")
	}
	if mixedComp.Preflight.Stripe.MaintenanceReady ||
		mixedComp.Preflight.Stripe.MaintenanceReason != config.PaymentReasonModeMismatch {
		t.Fatalf("mixed preflight=%+v", mixedComp.Preflight.Stripe)
	}
	// Portal without configuration ID stays unavailable, not silently open.
	if _, err := comp.Portal.Manage(context.Background(),
		paymentsapp.Principal{AccountID: "11111111-1111-4111-8111-111111111111"}, "premium.monthly"); err == nil {
		t.Fatal("portal opened in test without contract")
	}
	comp.Portal.Config.ConfigurationID = ""
	if _, err := comp.Portal.Manage(context.Background(),
		paymentsapp.Principal{AccountID: "11111111-1111-4111-8111-111111111111"}, "premium.monthly"); !errors.Is(err, paydomain.ErrUnavailable) {
		t.Fatalf("portal without configuration err=%v", err)
	}
}
