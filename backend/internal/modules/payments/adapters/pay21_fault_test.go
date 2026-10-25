package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// PAY-21 fault fixtures: sandbox-relevant provider behaviors pinned against
// local doubles. Retryable faults stay ambiguous (bounded retry, never
// fake success); declines and binding violations are terminal; revoked
// keys are diagnosed as unavailable so maintenance stays operable.

func TestPAY21StripeRetryableFaultsStayAmbiguous(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"rate-limited": {429, `{"error":{"type":"api_error","message":"rate"}}`},
		"overloaded":   {503, `{"error":{"type":"api_error","message":"down"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			_, err := provider.CreateCheckout(context.Background(), app.CheckoutRequest{
				Order: testOrder(t), CustomerID: "cus_synthetic", PriceID: "price_synthetic",
				MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
				SuccessURL: "https://app.example.invalid/return", CancelURL: "https://app.example.invalid/cancel",
				IdempotencyKey: "pay:checkout:op_retry",
			})
			if !errors.Is(err, app.ErrAmbiguous) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	hanging := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := hanging.CreateCheckout(ctx, app.CheckoutRequest{
		Order: testOrder(t), CustomerID: "cus_synthetic", PriceID: "price_synthetic",
		MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
		SuccessURL: "https://app.example.invalid/return", CancelURL: "https://app.example.invalid/cancel",
		IdempotencyKey: "pay:checkout:op_deadline",
	})
	if !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("deadline err = %v", err)
	}
}

func TestPAY21StripeRevokedKeyIsUnauthorized(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"type": "invalid_request_error", "message": "revoked",
			}})
		}))
		_, err := provider.CreateCheckout(context.Background(), app.CheckoutRequest{
			Order: testOrder(t), CustomerID: "cus_synthetic", PriceID: "price_synthetic",
			MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
			SuccessURL: "https://app.example.invalid/return", CancelURL: "https://app.example.invalid/cancel",
			IdempotencyKey: "pay:checkout:op_revoked",
		})
		if !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("status=%d err = %v", status, err)
		}
	}
}

func TestPAY21StripeDeclineNeverRetries(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"type": "card_error", "code": "card_declined", "message": "declined",
		}})
	}))
	_, err := provider.CreateCheckout(context.Background(), app.CheckoutRequest{
		Order: testOrder(t), CustomerID: "cus_synthetic", PriceID: "price_synthetic",
		MerchantID: "acct_synthetic", Environment: domain.EnvironmentTest,
		SuccessURL: "https://app.example.invalid/return", CancelURL: "https://app.example.invalid/cancel",
		IdempotencyKey: "pay:checkout:op_decline",
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("decline err = %v", err)
	}
}

func TestPAY21StripeDisputeWonClosesRenewalLoop(t *testing.T) {
	provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "dp_test_won", "object": "dispute", "amount": 1234, "currency": "brl",
			"livemode": false, "status": "won", "created": 1700000000,
			"charge":               map[string]any{"id": "ch_test_1"},
			"payment_intent":       map[string]any{"id": "pi_test_1"},
			"is_charge_refundable": true,
		})
	}))
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest,
		Merchant: "acct_synthetic", EventID: "evt_dp_won", EventType: "charge.dispute.closed",
		ResourceType: "dispute", ResourceID: "dp_test_won",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	fact, err := provider.VerifyDispute(context.Background(), event)
	if err != nil || fact.State != domain.DisputeWon || fact.RemoteDisputeID != "dp_test_won" {
		t.Fatalf("fact=%+v err=%v", fact, err)
	}
}

func TestPAY21AsaasRetryableFaultsStayAmbiguous(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
	}{
		"rate-limited": {http.StatusTooManyRequests},
		"overloaded":   {http.StatusBadGateway},
	} {
		t.Run(name, func(t *testing.T) {
			provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			if _, _, _, err := provider.FindPayment(context.Background(), "pay_fault"); !errors.Is(err, app.ErrAmbiguous) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	hanging := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, _, err := hanging.FindPayment(ctx, "pay_fault"); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("deadline err = %v", err)
	}
}

func TestPAY21AsaasRevokedKeyStaysUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		if _, _, _, err := provider.FindPayment(context.Background(), "pay_revoked"); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("status=%d err = %v", status, err)
		}
	}
}

func TestPAY21AsaasOverdueNeverSettles(t *testing.T) {
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
		Merchant: "merchant_1", EventID: "evt_overdue", EventType: "PAYMENT_RECEIVED",
		ResourceType: "payment", ResourceID: "pay_overdue",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "pay_overdue", "customer": "cus_synthetic", "status": "OVERDUE",
			"externalReference": "66666666-6666-4666-8666-666666666666",
			"value":             12.34, "dueDate": "2026-10-20",
		})
	}))
	if _, err := provider.VerifySettlement(context.Background(), event); !errors.Is(err, app.ErrNotSettled) {
		t.Fatalf("overdue err = %v", err)
	}
}

func TestPAY21AsaasChargebackReversalCloses(t *testing.T) {
	payment := func(status string, chargeback map[string]any) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := map[string]any{
				"id": "pay_cb", "customer": "cus_synthetic", "status": status,
				"value": 12.34, "dueDate": "2026-10-26",
			}
			if chargeback != nil {
				body["chargeback"] = chargeback
			}
			_ = json.NewEncoder(w).Encode(body)
		})
	}
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
		Merchant: "merchant_1", EventID: "evt_cb", EventType: "PAYMENT_CHARGEBACK_REQUESTED",
		ResourceType: "payment", ResourceID: "pay_cb",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}

	requested, err := asaasTestProvider(t, payment("CHARGEBACK_REQUESTED", nil)).VerifyDispute(context.Background(), event)
	if err != nil || requested.State != domain.DisputePending {
		t.Fatalf("requested=%+v err=%v", requested, err)
	}

	reversal := event
	reversal.EventType = "PAYMENT_AWAITING_CHARGEBACK_REVERSAL"
	closed, err := asaasTestProvider(t, payment("RECEIVED", map[string]any{
		"status": "REVERSED", "value": "12.34",
	})).VerifyDispute(context.Background(), reversal)
	if err != nil || closed.State != domain.DisputeClosed {
		t.Fatalf("reversal=%+v err=%v", closed, err)
	}
}

func TestPAY21AsaasCancelAuthorizationExactCall(t *testing.T) {
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v3/pix/automatic/authorizations/auth_1" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	if err := provider.CancelPixAutoAuthorization(context.Background(), "auth_1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
