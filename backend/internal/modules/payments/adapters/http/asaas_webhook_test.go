package http

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	asaasToken    = "asaas_webhook_token_synthetic_1234567890"
	asaasPrevious = "asaas_webhook_previous_synthetic_123456"
	asaasAPIKey   = "asaas_api_key_synthetic_1234567890"
	asaasMerchant = "11111111-1111-4111-8111-111111111111"
)

func asaasBody(id, eventType, resource, resourceID string) []byte {
	resourceField := ""
	if resource != "" {
		resourceField = fmt.Sprintf(`,%q:{"id":%q}`, resource, resourceID)
	}
	return []byte(fmt.Sprintf(`{"id":%q,"event":%q,"dateCreated":"2026-10-24 12:00:00","account":{"id":%q}%s}`,
		id, eventType, asaasMerchant, resourceField))
}

func validAsaasHandler(inbox *fakeInbox) AsaasWebhookHandler {
	return AsaasWebhookHandler{
		Inbox: inbox, WebhookToken: asaasToken, APIKey: asaasAPIKey,
		Environment: domain.EnvironmentTest, Merchant: asaasMerchant,
	}
}

func serveAsaas(t *testing.T, handler AsaasWebhookHandler, body []byte, token, contentType string, secure bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/payments/asaas/webhook", bytes.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		request.Header.Set("asaas-access-token", token)
	}
	if secure {
		request.TLS = &tls.ConnectionState{}
	}
	response := httptest.NewRecorder()
	handler.handle(response, request)
	return response
}

func TestAsaasWebhookValidAndUnknownEvents(t *testing.T) {
	inbox := &fakeInbox{result: app.InboxResult{Created: true, State: "PENDING"}}
	body := asaasBody("evt_pay", "PAYMENT_RECEIVED", "payment", "pay_1")
	response := serveAsaas(t, validAsaasHandler(inbox), body, asaasToken, "application/json", false)
	if response.Code != http.StatusOK || inbox.calls != 1 {
		t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
	}
	if inbox.last.Provider != domain.ProviderAsaas || inbox.last.EventID != "evt_pay" ||
		inbox.last.EventType != "PAYMENT_RECEIVED" || inbox.last.ResourceType != "payment" ||
		inbox.last.ResourceID != "pay_1" || inbox.last.PayloadHash != sha256Hex(body) {
		t.Fatalf("event = %+v", inbox.last)
	}

	inbox = &fakeInbox{result: app.InboxResult{Created: true, State: "PENDING"}}
	body = asaasEligibilityBody("evt_eligibility", "INELIGIBLE")
	response = serveAsaas(t, validAsaasHandler(inbox), body, asaasToken, "application/json", false)
	if response.Code != http.StatusOK || inbox.last.ResourceType != "event" ||
		inbox.last.ResourceID != "evt_eligibility" || inbox.last.ResourceStatus != "INELIGIBLE" {
		t.Fatalf("unknown status=%d event=%+v", response.Code, inbox.last)
	}
}

func asaasEligibilityBody(id, status string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"event":"PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED","account":{"id":%q},"eligibility":{"status":%q,"ineligibleReasons":[]}}`,
		id, asaasMerchant, status))
}

func TestAsaasWebhookDuplicateAndConflict(t *testing.T) {
	body := asaasBody("evt_dup", "PAYMENT_CONFIRMED", "payment", "pay_1")
	inbox := &fakeInbox{result: app.InboxResult{State: "PENDING"}}
	response := serveAsaas(t, validAsaasHandler(inbox), body, asaasToken, "application/json", false)
	if response.Code != http.StatusOK || inbox.calls != 1 {
		t.Fatalf("duplicate status=%d calls=%d", response.Code, inbox.calls)
	}
	inbox.err = domain.ErrConflict
	response = serveAsaas(t, validAsaasHandler(inbox), body, asaasToken, "application/json", false)
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d", response.Code)
	}
}

func TestAsaasWebhookAuthAndBindingRefusals(t *testing.T) {
	body := asaasBody("evt_auth", "PAYMENT_RECEIVED", "payment", "pay_1")
	cases := map[string]struct {
		body        []byte
		token       string
		contentType string
		status      int
	}{
		"missing-token":       {body, "", "application/json", http.StatusUnauthorized},
		"wrong-token":         {body, "wrong-token-but-long-enough-123456", "application/json", http.StatusUnauthorized},
		"wrong-content":       {body, asaasToken, "text/plain", http.StatusBadRequest},
		"invalid-json":        {[]byte(`{`), asaasToken, "application/json", http.StatusBadRequest},
		"wrong-account":       {[]byte(`{"id":"evt","event":"PAYMENT_RECEIVED","account":{"id":"other"},"payment":{"id":"pay"}}`), asaasToken, "application/json", http.StatusBadRequest},
		"missing-eligibility": {asaasEligibilityBody("evt_elig", ""), asaasToken, "application/json", http.StatusBadRequest},
		"unknown-eligibility": {asaasEligibilityBody("evt_elig", "MAYBE"), asaasToken, "application/json", http.StatusBadRequest},
		"missing-event-id":    {[]byte(fmt.Sprintf(`{"event":"PAYMENT_RECEIVED","account":{"id":%q},"payment":{"id":"pay"}}`, asaasMerchant)), asaasToken, "application/json", http.StatusBadRequest},
		"missing-type":        {[]byte(fmt.Sprintf(`{"id":"evt","account":{"id":%q},"payment":{"id":"pay"}}`, asaasMerchant)), asaasToken, "application/json", http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			inbox := &fakeInbox{result: app.InboxResult{Created: true}}
			response := serveAsaas(t, validAsaasHandler(inbox), tc.body, tc.token, tc.contentType, false)
			if response.Code != tc.status || inbox.calls != 0 {
				t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
			}
		})
	}
}

func TestAsaasWebhookMultipleTokenHeadersRefused(t *testing.T) {
	body := asaasBody("evt_multi", "PAYMENT_RECEIVED", "payment", "pay_1")
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Add("asaas-access-token", asaasToken)
	request.Header.Add("asaas-access-token", asaasToken)
	response := httptest.NewRecorder()
	validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}}).handle(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestAsaasWebhookTokenRotationAndAPIKeySeparation(t *testing.T) {
	body := asaasBody("evt_rotate", "PAYMENT_RECEIVED", "payment", "pay_1")
	handler := validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}})
	handler.PreviousWebhookToken = asaasPrevious
	if response := serveAsaas(t, handler, body, asaasToken, "application/json", false); response.Code != http.StatusOK {
		t.Fatalf("current token status=%d", response.Code)
	}
	handler = validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}})
	handler.PreviousWebhookToken = asaasPrevious
	if response := serveAsaas(t, handler, body, asaasPrevious, "application/json", false); response.Code != http.StatusOK {
		t.Fatalf("previous token status=%d", response.Code)
	}
	handler.PreviousWebhookToken = ""
	if response := serveAsaas(t, handler, body, asaasPrevious, "application/json", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("retired token status=%d", response.Code)
	}
	reused := validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}})
	reused.WebhookToken = asaasAPIKey
	if response := serveAsaas(t, reused, body, asaasAPIKey, "application/json", false); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("reused API key status=%d", response.Code)
	}
	reusedPrevious := validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}})
	reusedPrevious.PreviousWebhookToken = asaasAPIKey
	if response := serveAsaas(t, reusedPrevious, body, asaasAPIKey, "application/json", false); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("previous API key status=%d", response.Code)
	}
}

func TestAsaasWebhookTLSAndBodyBounds(t *testing.T) {
	body := asaasBody("evt_bound", "PAYMENT_RECEIVED", "payment", "pay_1")
	handler := validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}})
	handler.RequireTLS = true
	if response := serveAsaas(t, handler, body, asaasToken, "application/json", false); response.Code != http.StatusForbidden {
		t.Fatalf("insecure status=%d", response.Code)
	}
	if response := serveAsaas(t, handler, body, asaasToken, "application/json", true); response.Code != http.StatusOK {
		t.Fatalf("secure status=%d", response.Code)
	}
	handler = validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}})
	handler.MaxBodyBytes = 16
	if response := serveAsaas(t, handler, body, asaasToken, "application/json", false); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status=%d", response.Code)
	}
}

func TestAsaasWebhookDatabaseFailureAndBacklogRetry(t *testing.T) {
	body := asaasBody("evt_db", "PAYMENT_RECEIVED", "payment", "pay_1")
	inbox := &fakeInbox{err: domain.ErrUnavailable}
	handler := validAsaasHandler(inbox)
	if response := serveAsaas(t, handler, body, asaasToken, "application/json", false); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("db failure status=%d", response.Code)
	}
	inbox.err = nil
	inbox.result = app.InboxResult{Created: true, State: "PENDING"}
	if response := serveAsaas(t, handler, body, asaasToken, "application/json", false); response.Code != http.StatusOK {
		t.Fatalf("retry status=%d", response.Code)
	}
	if inbox.calls != 2 {
		t.Fatalf("calls=%d", inbox.calls)
	}
}

func TestAsaasWebhookNoIPTrust(t *testing.T) {
	body := asaasBody("evt_ip", "PAYMENT_RECEIVED", "payment", "pay_1")
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	request.RemoteAddr = "10.0.0.1:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("asaas-access-token", "wrong-token-but-long-enough-123456")
	response := httptest.NewRecorder()
	validAsaasHandler(&fakeInbox{result: app.InboxResult{Created: true}}).handle(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
}
