package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/stripe/stripe-go/v87"
)

func testStripeKey() string {
	return "sk_" + "test_" + strings.Repeat("synthetic", 4)
}

func stripeTestProvider(t *testing.T, handler http.Handler) *StripeProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := stripe.NewClient(testStripeKey(), stripe.WithBackends(stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		HTTPClient:        server.Client(),
		URL:               stripe.String(server.URL),
		EnableTelemetry:   stripe.Bool(false),
		MaxNetworkRetries: stripe.Int64(0),
	})))
	return NewStripeProviderWithClient(client, domain.EnvironmentTest)
}

func testOrder(t *testing.T) domain.Order {
	t.Helper()
	offer := domain.Offer{
		ID: "premium.monthly", Family: "premium", Term: domain.TermMonthly, Price: domain.BRL(1234),
		Capabilities: domain.PremiumCapabilityList(),
		Channels:     map[domain.Channel]bool{domain.ChannelCard: true},
		SaleEnabled:  true,
	}
	catalog, err := domain.NewCatalog("pay-catalog-v1", offer)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(domain.QuoteRequest{OfferID: offer.ID, Channel: domain.ChannelCard}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := domain.NewOrder("55555555-5555-4555-8555-555555555555", "11111111-1111-4111-8111-111111111111", quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	return order
}

func TestStripeEnsureCustomerBindsOwnerAndIdempotency(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/customers" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer sk_"+"test_") {
			t.Fatal("missing test key authorization")
		}
		if r.Header.Get("Idempotency-Key") != "pay:customer:11111111-1111-4111-8111-111111111111:test:acct_synthetic" {
			t.Fatalf("idempotency = %q", r.Header.Get("Idempotency-Key"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("metadata[account_id]") != "11111111-1111-4111-8111-111111111111" ||
			r.Form.Get("metadata[merchant_id]") != "acct_synthetic" {
			t.Fatalf("metadata = %v", r.Form)
		}
		if strings.Contains(r.Form.Encode(), "card") || strings.Contains(r.Form.Encode(), "cvc") {
			t.Fatal("customer request contains card data")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cus_synthetic", "object": "customer", "livemode": false,
			"metadata": map[string]string{
				"account_id":  "11111111-1111-4111-8111-111111111111",
				"merchant_id": "acct_synthetic", "environment": "test",
			},
		})
	}))
	customer, err := provider.EnsureCustomer(context.Background(), app.CustomerRequest{
		AccountID: "11111111-1111-4111-8111-111111111111", Environment: domain.EnvironmentTest, MerchantID: "acct_synthetic",
	})
	if err != nil || customer.ID != "cus_synthetic" {
		t.Fatalf("customer = %+v err=%v", customer, err)
	}
}

func TestStripeCreateCheckoutStrictShape(t *testing.T) {
	order := testOrder(t)
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/checkout/sessions" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") != "pay:checkout:op_1" {
			t.Fatalf("idempotency = %q", r.Header.Get("Idempotency-Key"))
		}
		if r.Header.Get("Stripe-Version") != "2026-09-30.endive" {
			t.Fatalf("stripe version = %q", r.Header.Get("Stripe-Version"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form := r.Form
		checks := map[string]string{
			"mode": "subscription", "customer": "cus_synthetic",
			"client_reference_id": order.ID, "allow_promotion_codes": "false",
			"line_items[0][price]": "price_synthetic", "line_items[0][quantity]": "1",
			"metadata[order_id]": order.ID, "metadata[account_id]": order.AccountID,
			"subscription_data[metadata][order_id]": order.ID,
			"success_url":                           "https://app.example.invalid/billing/checkout/return?session_id={CHECKOUT_SESSION_ID}",
			"cancel_url":                            "https://app.example.invalid/billing/checkout/cancel",
		}
		for key, want := range checks {
			if got := form.Get(key); got != want {
				t.Fatalf("%s = %q want %q\nall=%v", key, got, want, form)
			}
		}
		if form.Get("allowed_payment_method_types[0]") != "card" {
			t.Fatalf("payment methods = %v", form)
		}
		if strings.Contains(form.Encode(), "number=") || strings.Contains(form.Encode(), "cvc=") {
			t.Fatal("checkout request carries raw card data")
		}
		expires, err := strconv.ParseInt(form.Get("expires_at"), 10, 64)
		if err != nil || time.Until(time.Unix(expires, 0)) < 30*time.Minute {
			t.Fatalf("expires_at = %q err=%v", form.Get("expires_at"), err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cs_test_synthetic", "object": "checkout.session", "mode": "subscription",
			"status": "open", "url": "https://checkout.example.invalid/cs",
			"customer":     map[string]any{"id": "cus_synthetic", "object": "customer"},
			"amount_total": 1234, "currency": "brl", "livemode": false,
		})
	}))
	session, err := provider.CreateCheckout(context.Background(), app.CheckoutRequest{
		Order: order, CustomerID: "cus_synthetic", PriceID: "price_synthetic",
		MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
		SuccessURL:     "https://app.example.invalid/billing/checkout/return?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:      "https://app.example.invalid/billing/checkout/cancel",
		IdempotencyKey: "pay:checkout:op_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "cs_test_synthetic" || session.URL != "https://checkout.example.invalid/cs" ||
		session.Mode != "subscription" || session.CustomerID != "cus_synthetic" ||
		session.AmountTotal != 1234 || session.Currency != "brl" || session.Livemode {
		t.Fatalf("session = %+v", session)
	}
}

func TestStripeFindCheckoutMatchesClientReference(t *testing.T) {
	order := testOrder(t)
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/checkout/sessions" || r.Method != http.MethodGet {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("customer") != "cus_synthetic" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list", "url": "/v1/checkout/sessions", "has_more": false,
			"data": []map[string]any{{
				"id": "cs_test_synthetic", "object": "checkout.session", "mode": "subscription",
				"status": "open", "url": "https://checkout.example.invalid/cs",
				"customer":            map[string]any{"id": "cus_synthetic"},
				"client_reference_id": order.ID, "amount_total": 1234,
				"currency": "brl", "livemode": false,
			}},
		})
	}))
	session, found, definitive, err := provider.FindCheckout(context.Background(), "cus_synthetic", order.ID)
	if err != nil || !found || !definitive || session.ID != "cs_test_synthetic" {
		t.Fatalf("session=%+v found=%v definitive=%v err=%v", session, found, definitive, err)
	}
}

func TestStripeFindCheckoutNoMatchIsDefinitive(t *testing.T) {
	order := testOrder(t)
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list", "url": "/v1/checkout/sessions", "has_more": false, "data": []any{},
		})
	}))
	_, found, definitive, err := provider.FindCheckout(context.Background(), "cus_synthetic", order.ID)
	if err != nil || found || !definitive {
		t.Fatalf("found=%v definitive=%v err=%v", found, definitive, err)
	}
}

func TestStripeErrorMapping(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"invalid":     {400, `{"error":{"type":"invalid_request_error","message":"bad"}}`, domain.ErrInvalid},
		"server":      {500, `{"error":{"type":"api_error","message":"down"}}`, app.ErrAmbiguous},
		"idempotency": {409, `{"error":{"type":"idempotency_error","message":"changed"}}`, domain.ErrConflict},
	} {
		t.Run(name, func(t *testing.T) {
			order := testOrder(t)
			provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			_, err := provider.CreateCheckout(context.Background(), app.CheckoutRequest{
				Order: order, CustomerID: "cus_synthetic", PriceID: "price_synthetic",
				MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
				SuccessURL: "https://app.example.invalid/return", CancelURL: "https://app.example.invalid/cancel",
				IdempotencyKey: "pay:checkout:op_1",
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
		})
	}
}

func TestStripeRejectsWrongCustomerOrAmount(t *testing.T) {
	order := testOrder(t)
	for name, response := range map[string]map[string]any{
		"customer": {
			"id": "cs_test_synthetic", "object": "checkout.session", "mode": "subscription",
			"status": "open", "url": "https://checkout.example.invalid/cs",
			"customer": map[string]any{"id": "cus_other"}, "amount_total": 1234,
			"currency": "brl", "livemode": false,
		},
		"amount": {
			"id": "cs_test_synthetic", "object": "checkout.session", "mode": "subscription",
			"status": "open", "url": "https://checkout.example.invalid/cs",
			"customer": map[string]any{"id": "cus_synthetic"}, "amount_total": 1235,
			"currency": "brl", "livemode": false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(response)
			}))
			_, err := provider.CreateCheckout(context.Background(), app.CheckoutRequest{
				Order: order, CustomerID: "cus_synthetic", PriceID: "price_synthetic",
				MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
				SuccessURL: "https://app.example.invalid/return", CancelURL: "https://app.example.invalid/cancel",
				IdempotencyKey: "pay:checkout:op_1",
			})
			if !errors.Is(err, domain.ErrBinding) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestStripeFindCheckoutListErrorIsNotAbsence(t *testing.T) {
	order := testOrder(t)
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"api_error","message":"down"}}`))
	}))
	_, found, definitive, err := provider.FindCheckout(context.Background(), "cus_synthetic", order.ID)
	if found || definitive || !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("found=%v definitive=%v err=%v", found, definitive, err)
	}
}

func TestStripeEnsureCustomerRejectsForeignMetadata(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cus_synthetic", "object": "customer", "livemode": false,
			"metadata": map[string]string{"account_id": "foreign"},
		})
	}))
	_, err := provider.EnsureCustomer(context.Background(), app.CustomerRequest{
		AccountID: "11111111-1111-4111-8111-111111111111", Environment: domain.EnvironmentTest, MerchantID: "acct_synthetic",
	})
	if !errors.Is(err, domain.ErrBinding) {
		t.Fatalf("err = %v", err)
	}
}
