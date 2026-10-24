package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func asaasTestProvider(t *testing.T, handler http.Handler) *AsaasProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &AsaasProvider{
		BaseURL: server.URL, APIKey: strings.Repeat("synthetic", 4), Environment: domain.EnvironmentTest,
		HTTPClient: server.Client(), Now: time.Now,
	}
}

func TestAsaasEnsureCustomerSearchThenCreate(t *testing.T) {
	calls := 0
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("access_token") == "" || r.Header.Get("User-Agent") != asaasUserAgent {
			t.Fatal("missing Asaas auth/user-agent")
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("Asaas must use access_token, not Authorization")
		}
		calls++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v3/customers":
			if r.URL.Query().Get("externalReference") != "11111111-1111-4111-8111-111111111111" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "totalCount": 0, "hasMore": false})
		case r.Method == http.MethodPost && r.URL.Path == "/v3/customers":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["name"] != "Synthetic Payer" || body["cpfCnpj"] != "00000000000" ||
				body["externalReference"] != "11111111-1111-4111-8111-111111111111" {
				t.Fatalf("body = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "cus_synthetic", "externalReference": "11111111-1111-4111-8111-111111111111",
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	customer, err := provider.EnsureCustomer(context.Background(), app.PixCustomerRequest{
		AccountID: "11111111-1111-4111-8111-111111111111", Environment: domain.EnvironmentTest,
		MerchantID: "11111111-1111-4111-8111-111111111111", Name: "Synthetic Payer", TaxID: "00000000000",
	})
	if err != nil || customer.ID != "cus_synthetic" {
		t.Fatalf("customer=%+v err=%v", customer, err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestAsaasEnsureCustomerFoundDoesNotCreate(t *testing.T) {
	creates := 0
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			creates++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       []map[string]any{{"id": "cus_existing", "externalReference": "11111111-1111-4111-8111-111111111111"}},
			"totalCount": 1, "hasMore": false,
		})
	}))
	customer, err := provider.EnsureCustomer(context.Background(), app.PixCustomerRequest{
		AccountID: "11111111-1111-4111-8111-111111111111", Environment: domain.EnvironmentTest,
		MerchantID: "11111111-1111-4111-8111-111111111111", Name: "Payer", TaxID: "00000000000",
	})
	if err != nil || customer.ID != "cus_existing" || creates != 0 {
		t.Fatalf("customer=%+v err=%v creates=%d", customer, err, creates)
	}
}

func TestAsaasEnsureCustomerIncompleteSearchDoesNotCreate(t *testing.T) {
	creates := 0
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			creates++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "totalCount": 2, "hasMore": true})
	}))
	_, err := provider.EnsureCustomer(context.Background(), app.PixCustomerRequest{
		AccountID: "11111111-1111-4111-8111-111111111111", Environment: domain.EnvironmentTest,
		MerchantID: "11111111-1111-4111-8111-111111111111", Name: "Payer", TaxID: "00000000000",
	})
	if !errors.Is(err, app.ErrAmbiguous) || creates != 0 {
		t.Fatalf("err=%v creates=%d", err, creates)
	}
}

func TestAsaasCreatePaymentExactShape(t *testing.T) {
	order := testOrder(t)
	order.Channel = domain.ChannelPixManual
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/payments" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Customer          string      `json:"customer"`
			BillingType       string      `json:"billingType"`
			Value             json.Number `json:"value"`
			DueDate           string      `json:"dueDate"`
			ExternalReference string      `json:"externalReference"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Customer != "cus_synthetic" || body.BillingType != "PIX" || body.Value.String() != "12.34" ||
			body.ExternalReference != order.ID || body.DueDate == "" {
			t.Fatalf("body = %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "pay_synthetic", "customer": "cus_synthetic", "status": "PENDING",
			"externalReference": order.ID, "value": 12.34, "dueDate": body.DueDate,
		})
	}))
	payment, err := provider.CreatePayment(context.Background(), app.PixPaymentRequest{
		Order: order, CustomerID: "cus_synthetic", Environment: domain.EnvironmentTest,
		MerchantID: "11111111-1111-4111-8111-111111111111", DueDate: order.QuotedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payment.ID != "pay_synthetic" || payment.Class != app.PixPending ||
		!payment.Amount.Equal(domain.BRL(1234)) || payment.ExternalReference != order.ID {
		t.Fatalf("payment = %+v", payment)
	}
}

func TestAsaasFindPaymentCompleteAndIncomplete(t *testing.T) {
	order := testOrder(t)
	order.Channel = domain.ChannelPixManual
	cases := map[string]struct {
		response   map[string]any
		found      bool
		definitive bool
		wantErr    error
	}{
		"match": {
			response: map[string]any{"data": []map[string]any{{
				"id": "pay_1", "customer": "cus_synthetic", "status": "PENDING",
				"externalReference": order.ID, "value": 12.34, "dueDate": "2026-10-24",
			}}, "totalCount": 1, "hasMore": false},
			found: true, definitive: true,
		},
		"complete-empty": {
			response:   map[string]any{"data": []any{}, "totalCount": 0, "hasMore": false},
			definitive: true,
		},
		"incomplete-empty": {
			response:   map[string]any{"data": []any{}, "totalCount": 2, "hasMore": true},
			definitive: false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			payment, found, definitive, err := provider.FindPayment(context.Background(), order.ID)
			if !errors.Is(err, tc.wantErr) || found != tc.found || definitive != tc.definitive {
				t.Fatalf("payment=%+v found=%v definitive=%v err=%v", payment, found, definitive, err)
			}
		})
	}
}

func TestAsaasFindPaymentMultipleMatchesRefused(t *testing.T) {
	order := testOrder(t)
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "pay_1", "customer": "cus_1", "status": "PENDING", "externalReference": order.ID, "value": 12.34},
				{"id": "pay_2", "customer": "cus_1", "status": "PENDING", "externalReference": order.ID, "value": 12.34},
			},
			"totalCount": 2, "hasMore": false,
		})
	}))
	if _, _, _, err := provider.FindPayment(context.Background(), order.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestAsaasGetQRBoundedResponse(t *testing.T) {
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/payments/pay_1/pixQrCode" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"encodedImage": "image", "payload": "payload",
			"expirationDate": "2026-10-24 23:59:59",
		})
	}))
	qr, err := provider.GetQR(context.Background(), "pay_1")
	if err != nil || qr.Payload != "payload" || qr.Image != "image" || qr.ExpiresAt.IsZero() {
		t.Fatalf("qr=%+v err=%v", qr, err)
	}
}

func TestAsaasStatusAndAmountMapping(t *testing.T) {
	for status, want := range map[string]string{
		"RECEIVED": app.PixSettled, "PENDING": app.PixPending, "CONFIRMED": app.PixPending,
		"OVERDUE": app.PixExpired, "REFUNDED": app.PixRefunded,
		"CHARGEBACK_REQUESTED": app.PixDisputed, "RECEIVED_IN_CASH": app.PixUnknown,
	} {
		if got := asaasPixClass(status); got != want {
			t.Fatalf("%s -> %s want %s", status, got, want)
		}
	}
	for raw, want := range map[string]int64{"12.34": 1234, "12.3": 1230, "12": 1200, "0.05": 5} {
		got, err := parseBRLMinor(json.Number(raw))
		if err != nil || got != want {
			t.Fatalf("%s -> %d want %d err=%v", raw, got, want, err)
		}
	}
	for _, raw := range []string{"-1", "1.234", "1e2", "abc"} {
		if _, err := parseBRLMinor(json.Number(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestAsaasServerErrorAmbiguous(t *testing.T) {
	order := testOrder(t)
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	if _, _, _, err := provider.FindPayment(context.Background(), order.ID); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("err = %v", err)
	}
}

func TestAsaasVerifySettlementRequiresReceived(t *testing.T) {
	const orderID = "66666666-6666-4666-8666-666666666666"
	event := app.InboxRecord{InboxEvent: app.InboxEvent{
		Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
		Merchant: "merchant_1", EventID: "evt_pay", EventType: "PAYMENT_RECEIVED",
		ResourceType: "payment", ResourceID: "pay_1",
	}, State: "PENDING", ReceivedAt: time.Now().UTC()}
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/payments/pay_1" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "pay_1", "customer": "cus_synthetic", "status": "RECEIVED",
			"externalReference": orderID, "value": 12.34, "dueDate": "2026-10-24",
		})
	}))
	candidate, err := provider.VerifySettlement(context.Background(), event)
	if err != nil || candidate.OrderID != orderID || candidate.CustomerID != "cus_synthetic" ||
		candidate.Provider != domain.ProviderAsaas || !candidate.Amount.Equal(domain.BRL(1234)) {
		t.Fatalf("candidate=%+v err=%v", candidate, err)
	}

	provider = asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "pay_1", "customer": "cus_synthetic", "status": "PENDING",
			"externalReference": orderID, "value": 12.34, "dueDate": "2026-10-24",
		})
	}))
	if _, err := provider.VerifySettlement(context.Background(), event); !errors.Is(err, app.ErrNotSettled) {
		t.Fatalf("pending err=%v", err)
	}
	event.EventType = "PAYMENT_CREATED"
	if _, err := provider.VerifySettlement(context.Background(), event); !errors.Is(err, app.ErrIgnored) {
		t.Fatalf("ignored err=%v", err)
	}
}

func TestAsaasPixAutoAuthorizationContract(t *testing.T) {
	provider := asaasTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v3/pix/automatic/authorizations":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			immediate, _ := body["immediateQrCode"].(map[string]any)
			if body["frequency"] != "MONTHLY" || body["contractId"] != "contract_1" ||
				body["customerId"] != "cus_1" || body["paymentCreationMode"] != "SUBSCRIPTION" ||
				body["retryPolicy"] != "NOT_ALLOWED" || immediate == nil ||
				immediate["expirationSeconds"] != float64(3600) {
				t.Fatalf("body=%v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "auth_1", "customerId": "cus_1", "contractId": "contract_1", "status": "CREATED",
				"value": 12.34, "startDate": "2026-10-24", "finishDate": "2027-10-24",
				"subscriptionId": "sub_1", "payload": "qr", "encodedImage": "image",
				"immediateQrCode": map[string]any{"expirationDate": "2026-10-24 23:59:59"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v3/pix/automatic/authorizations/auth_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "auth_1", "customerId": "cus_1", "contractId": "contract_1", "status": "REFUSED",
				"value": 12.34, "startDate": "2026-10-24",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v3/pix/automatic/authorizations":
			if r.URL.Query().Get("customerId") != "cus_1" {
				t.Fatalf("query=%s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "auth_1", "customerId": "cus_1", "contractId": "contract_1",
					"status": "CREATED", "value": 12.34, "startDate": "2026-10-24"}},
				"totalCount": 1, "hasMore": false,
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v3/pix/automatic/authorizations/auth_1":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	created, err := provider.CreatePixAutoAuthorization(context.Background(), app.PixAutoProviderRequest{
		CustomerID: "cus_1", ContractID: "contract_1", Environment: domain.EnvironmentTest,
		MerchantID: "merchant_1", Value: domain.BRL(1234), StartDate: time.Now(),
		FinishDate: time.Now().AddDate(1, 0, 0), ImmediateExpiration: time.Hour,
	})
	if err != nil || created.ID != "auth_1" || created.Status != "CREATED" || created.QRCode != "qr" ||
		created.RemoteSubscriptionID != "sub_1" || !created.Value.Equal(domain.BRL(1234)) {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	retrieved, err := provider.RetrievePixAutoAuthorization(context.Background(), "auth_1")
	if err != nil || retrieved.Status != "REFUSED" {
		t.Fatalf("retrieved=%+v err=%v", retrieved, err)
	}
	found, ok, definitive, err := provider.FindPixAutoAuthorization(context.Background(), "cus_1", "contract_1")
	if err != nil || !ok || !definitive || found.ID != "auth_1" {
		t.Fatalf("found=%+v ok=%v definitive=%v err=%v", found, ok, definitive, err)
	}
	if err := provider.CancelPixAutoAuthorization(context.Background(), "auth_1"); err != nil {
		t.Fatal(err)
	}
}
