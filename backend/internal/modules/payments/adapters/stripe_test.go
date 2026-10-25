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
		LeveledLogger:     &stripe.LeveledLogger{Level: stripe.LevelNull},
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

func stripeInvoice(invoiceID string, mutate func(map[string]any)) map[string]any {
	body := map[string]any{
		"id": invoiceID, "object": "invoice", "status": "paid", "currency": "brl",
		"amount_due": 1234, "amount_paid": 1234, "amount_paid_off_stripe": 0,
		"collection_method": "charge_automatically", "billing_reason": "subscription_cycle",
		"customer":           map[string]any{"id": "cus_synthetic", "object": "customer"},
		"livemode":           false,
		"status_transitions": map[string]any{"paid_at": 1700000000},
		"parent": map[string]any{
			"type": "subscription_details",
			"subscription_details": map[string]any{
				"metadata":     map[string]string{"order_id": "66666666-6666-4666-8666-666666666666", "account_id": "11111111-1111-4111-8111-111111111111"},
				"subscription": map[string]any{"id": "sub_synthetic", "metadata": map[string]string{}},
			},
		},
	}
	if mutate != nil {
		mutate(body)
	}
	return body
}

func TestStripeVerifySettlementCanonicalInvoice(t *testing.T) {
	event := app.InboxRecord{
		InboxEvent: app.InboxEvent{
			Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest,
			Merchant: "acct_synthetic", EventID: "evt_invoice", EventType: "invoice.paid",
			ResourceType: "invoice", ResourceID: "in_1",
		},
		State: "PENDING", ReceivedAt: time.Now().UTC(),
	}
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/invoices/in_1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(stripeInvoice("in_1", nil))
	}))
	candidate, err := provider.VerifySettlement(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.OrderID != "66666666-6666-4666-8666-666666666666" || candidate.CustomerID != "cus_synthetic" ||
		candidate.RemotePaymentID != "in_1" || !candidate.Amount.Equal(domain.BRL(1234)) ||
		candidate.Channel != domain.ChannelCard || candidate.Provider != domain.ProviderStripe {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestStripeVerifySettlementRefusals(t *testing.T) {
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest,
		Merchant: "acct_synthetic", EventID: "evt_invoice", EventType: "invoice.paid",
		ResourceType: "invoice", ResourceID: "in_1",
	}}
	for name, tc := range map[string]struct {
		mutate func(map[string]any)
		want   error
	}{
		"manual":         {func(b map[string]any) { b["amount_paid_off_stripe"] = 1 }, domain.ErrBinding},
		"send-invoice":   {func(b map[string]any) { b["collection_method"] = "send_invoice" }, domain.ErrBinding},
		"wrong-currency": {func(b map[string]any) { b["currency"] = "usd" }, domain.ErrBinding},
		"partial":        {func(b map[string]any) { b["amount_paid"] = 1000 }, domain.ErrBinding},
		"pending":        {func(b map[string]any) { b["status"] = "open" }, app.ErrNotSettled},
		"manual-reason":  {func(b map[string]any) { b["billing_reason"] = "manual" }, domain.ErrBinding},
		"missing-meta":   {func(b map[string]any) { b["parent"] = nil }, domain.ErrBinding},
	} {
		t.Run(name, func(t *testing.T) {
			provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(stripeInvoice("in_1", tc.mutate))
			}))
			if _, err := provider.VerifySettlement(context.Background(), event); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
		})
	}
	event.EventType = "checkout.session.completed"
	if _, err := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ignored event called provider")
	})).VerifySettlement(context.Background(), event); !errors.Is(err, app.ErrIgnored) {
		t.Fatalf("ignored err = %v", err)
	}
}

func stripeSubscription(id, status string) map[string]any {
	return map[string]any{
		"id": id, "object": "subscription", "status": status, "cancel_at_period_end": true,
		"customer": map[string]any{"id": "cus_synthetic"},
		"livemode": false,
		"metadata": map[string]string{"account_id": "11111111-1111-4111-8111-111111111111", "offer_id": "premium.monthly"},
	}
}

func TestStripeLifecycleAndPortal(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/subscriptions/sub_1" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(stripeSubscription("sub_1", "active"))
		case r.URL.Path == "/v1/invoices/in_1" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "in_1", "object": "invoice", "status": "open",
				"parent": map[string]any{"subscription_details": map[string]any{
					"subscription": map[string]any{"id": "sub_1"},
				}},
			})
		case r.URL.Path == "/v1/checkout/sessions/cs_1" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "cs_1", "object": "checkout.session", "subscription": map[string]any{"id": "sub_1"},
			})
		case r.URL.Path == "/v1/billing_portal/sessions" && r.Method == http.MethodPost:
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("customer") != "cus_synthetic" || r.Form.Get("configuration") != "bpc_1" ||
				r.Form.Get("return_url") != "https://app.example.invalid/billing/manage" {
				t.Fatalf("portal form = %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "bps_1", "url": "https://billing.example.invalid/session"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	provider.Merchant = "acct_synthetic"
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest,
		Merchant: "acct_synthetic", EventID: "evt_sub", EventType: "customer.subscription.updated",
		ResourceType: "subscription", ResourceID: "sub_1",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	fact, err := provider.VerifyLifecycle(context.Background(), event)
	if err != nil || fact.RemoteSubscriptionID != "sub_1" || fact.ProviderStatus != "active" ||
		!fact.CancelAtPeriodEnd || fact.AccountID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("fact=%+v err=%v", fact, err)
	}
	for _, eventType := range []string{"invoice.payment_action_required", "checkout.session.completed"} {
		current := event
		current.EventType = eventType
		if eventType == "invoice.payment_action_required" {
			current.ResourceType, current.ResourceID = "invoice", "in_1"
		} else {
			current.ResourceType, current.ResourceID = "checkout.session", "cs_1"
		}
		if _, err := provider.VerifyLifecycle(context.Background(), current); err != nil {
			t.Fatalf("%s err=%v", eventType, err)
		}
	}
	if _, err := provider.RetrieveLifecycle(context.Background(), "sub_1"); err != nil {
		t.Fatal(err)
	}
	url, err := provider.CreatePortalSession(context.Background(), app.PortalRequest{
		CustomerID: "cus_synthetic", ReturnURL: "https://app.example.invalid/billing/manage", ConfigurationID: "bpc_1",
	})
	if err != nil || url != "https://billing.example.invalid/session" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestStripeLifecycleRefusals(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "sub_1", "object": "subscription", "status": "active", "customer": map[string]any{"id": "cus_1"},
			"livemode": false, "metadata": map[string]string{},
		})
	}))
	provider.Merchant = "acct_synthetic"
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest, Merchant: "acct_synthetic",
		EventID: "evt", EventType: "customer.subscription.updated", ResourceType: "subscription", ResourceID: "sub_1",
	}}
	if _, err := provider.VerifyLifecycle(context.Background(), event); !errors.Is(err, domain.ErrBinding) {
		t.Fatalf("missing metadata err=%v", err)
	}
	event.EventType = "invoice.created"
	event.ResourceType = "invoice"
	event.ResourceID = "in_1"
	if _, err := provider.VerifyLifecycle(context.Background(), event); !errors.Is(err, app.ErrIgnored) {
		t.Fatalf("invoice.created err=%v", err)
	}
	if _, err := provider.CreatePortalSession(context.Background(), app.PortalRequest{CustomerID: "cus_1"}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("portal missing config err=%v", err)
	}
}

func TestStripeCancelRenewalKeepsPaidThrough(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/subscriptions/sub_1" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("cancel_at_period_end") != "true" {
			t.Fatalf("form=%v", r.Form)
		}
		if r.Header.Get("Idempotency-Key") != "switch-key" {
			t.Fatalf("idempotency=%q", r.Header.Get("Idempotency-Key"))
		}
		body := stripeSubscription("sub_1", "active")
		body["cancel_at_period_end"] = true
		_ = json.NewEncoder(w).Encode(body)
	}))
	if err := provider.CancelRenewal(context.Background(), "sub_1", "switch-key"); err != nil {
		t.Fatal(err)
	}
}

func stripeInvoiceWithPayment(invoiceID, paymentIntentID string) map[string]any {
	body := stripeInvoice(invoiceID, nil)
	body["payments"] = map[string]any{
		"object": "list", "url": "/v1/invoices/" + invoiceID + "/payments", "has_more": false,
		"data": []map[string]any{{
			"id": "ip_synthetic", "object": "invoice_payment", "status": "paid",
			"amount_paid": 1234, "amount_requested": 1234, "currency": "brl",
			"payment": map[string]any{
				"type":           "payment_intent",
				"payment_intent": map[string]any{"id": paymentIntentID, "object": "payment_intent"},
			},
		}},
	}
	return body
}

func TestStripeRefundCreateFindAndDispute(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/invoices/in_refund":
			_ = json.NewEncoder(w).Encode(stripeInvoiceWithPayment("in_refund", "pi_refund"))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			if r.Header.Get("Idempotency-Key") != "op_refund_1" {
				t.Fatalf("idempotency = %q", r.Header.Get("Idempotency-Key"))
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("amount") != "500" || r.Form.Get("payment_intent") != "pi_refund" ||
				r.Form.Get("metadata[refund_id]") != "refund_local_1" {
				t.Fatalf("refund form = %v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "re_test_1", "object": "refund", "amount": 500, "currency": "brl",
				"status": "succeeded", "created": 1700000000,
				"payment_intent": map[string]any{"id": "pi_refund"},
				"metadata":       map[string]string{"refund_id": "refund_local_1"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/refunds":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list", "url": "/v1/refunds", "has_more": false,
				"data": []map[string]any{{
					"id": "re_test_1", "object": "refund", "amount": 500, "currency": "brl",
					"status": "succeeded", "created": 1700000000,
					"payment_intent": map[string]any{"id": "pi_refund"},
					"metadata":       map[string]string{"refund_id": "refund_local_1"},
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/disputes/dp_test_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "dp_test_1", "object": "dispute", "amount": 1234, "currency": "brl",
				"livemode": false, "status": "lost", "created": 1700000000,
				"charge":               map[string]any{"id": "ch_test_1"},
				"payment_intent":       map[string]any{"id": "pi_test_1"},
				"is_charge_refundable": false,
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	request := app.RefundProviderRequest{
		RefundID: "refund_local_1", RemotePaymentID: "in_refund",
		Amount: domain.BRL(500), IdempotencyKey: "op_refund_1",
		Metadata: map[string]string{"refund_id": "refund_local_1"},
	}
	created, err := provider.CreateRefund(context.Background(), request)
	if err != nil || created.ID != "re_test_1" || created.State != domain.RefundSucceeded {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	found, ok, complete, err := provider.FindRefund(context.Background(), request)
	if err != nil || !ok || !complete || found.ID != "re_test_1" {
		t.Fatalf("found=%+v ok=%v complete=%v err=%v", found, ok, complete, err)
	}
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest,
		Merchant: "acct_synthetic", EventID: "evt_dp", EventType: "charge.dispute.closed",
		ResourceType: "dispute", ResourceID: "dp_test_1",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	fact, err := provider.VerifyDispute(context.Background(), event)
	if err != nil || fact.RemoteDisputeID != "dp_test_1" || fact.RemoteChargeID != "pi_test_1" ||
		fact.State != domain.DisputeLost || !fact.Amount.Equal(domain.BRL(1234)) {
		t.Fatalf("fact=%+v err=%v", fact, err)
	}
	ignored := event
	ignored.EventType = "charge.succeeded"
	if _, err := provider.VerifyDispute(context.Background(), ignored); !errors.Is(err, app.ErrIgnored) {
		t.Fatalf("non-dispute err=%v", err)
	}
}
