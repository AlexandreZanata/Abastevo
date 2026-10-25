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

	paymentsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func pay19EmptyConfig() config.Config {
	return config.Config{}
}

func TestPAY19UnconfiguredRuntimeFailsClosed(t *testing.T) {
	comp := composePayments(pay19EmptyConfig().Payments, nil, time.Now)
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
	comp := composePayments(pay19EmptyConfig().Payments, nil, time.Now)
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
