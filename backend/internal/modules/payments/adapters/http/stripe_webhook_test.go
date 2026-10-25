package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/stripe/stripe-go/v87/webhook"
)

const (
	webhookSecret = "whsec_synthetic_webhook_secret_123456"
	otherSecret   = "whsec_synthetic_webhook_secret_other_987"
)

type fakeInbox struct {
	calls  int
	last   app.InboxEvent
	result app.InboxResult
	err    error
}

func (f *fakeInbox) RecordInbox(_ context.Context, event app.InboxEvent) (app.InboxResult, error) {
	f.calls++
	f.last = event
	return f.result, f.err
}

func eventBody(id, eventType, apiVersion string, livemode bool, account, resourceType, resourceID string) []byte {
	accountField := ""
	if account != "" {
		accountField = fmt.Sprintf(`"account":%q,`, account)
	}
	return []byte(fmt.Sprintf(`{"id":%q,"object":"event","api_version":%q,"created":1700000000,%s"livemode":%t,"type":%q,"data":{"object":{"id":%q,"object":%q}}}`,
		id, apiVersion, accountField, livemode, eventType, resourceID, resourceType))
}

func signed(body []byte, secret string, timestamp time.Time) string {
	return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: body, Secret: secret, Timestamp: timestamp,
	}).Header
}

func serve(t *testing.T, handler StripeWebhookHandler, body []byte, signature, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/payments/stripe/webhook", bytes.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if signature != "" {
		request.Header.Set("Stripe-Signature", signature)
	}
	response := httptest.NewRecorder()
	handler.handle(response, request)
	return response
}

func validHandler(inbox *fakeInbox) StripeWebhookHandler {
	return StripeWebhookHandler{
		Inbox: inbox, Secret: webhookSecret, Environment: domain.EnvironmentTest,
		Merchant: "acct_synthetic", ExpectedLivemode: false,
	}
}

func TestStripeWebhookDurableValidEvent(t *testing.T) {
	inbox := &fakeInbox{result: app.InboxResult{Created: true, State: "PENDING"}}
	body := eventBody("evt_1", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	response := serve(t, validHandler(inbox), body, signed(body, webhookSecret, time.Now()), "application/json")
	if response.Code != http.StatusOK || inbox.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, inbox.calls, response.Body.String())
	}
	if inbox.last.Provider != domain.ProviderStripe || inbox.last.EventID != "evt_1" ||
		inbox.last.EventType != "invoice.paid" || inbox.last.ResourceType != "invoice" ||
		inbox.last.ResourceID != "in_1" || inbox.last.Merchant != "acct_synthetic" {
		t.Fatalf("event = %+v", inbox.last)
	}
	sum := sha256Hex(body)
	if inbox.last.PayloadHash != sum {
		t.Fatalf("payload hash = %s want %s", inbox.last.PayloadHash, sum)
	}
}

func TestStripeWebhookUnknownEventIsDurablyAccepted(t *testing.T) {
	inbox := &fakeInbox{result: app.InboxResult{Created: true, State: "PENDING"}}
	body := eventBody("evt_unknown", "synthetic.unknown", "2026-09-30.endive", false, "", "weird", "obj_1")
	response := serve(t, validHandler(inbox), body, signed(body, webhookSecret, time.Now()), "application/json")
	if response.Code != http.StatusOK || inbox.calls != 1 || inbox.last.EventType != "synthetic.unknown" {
		t.Fatalf("status=%d calls=%d event=%+v", response.Code, inbox.calls, inbox.last)
	}
}

func TestStripeWebhookDuplicateAndConflict(t *testing.T) {
	body := eventBody("evt_dup", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	inbox := &fakeInbox{result: app.InboxResult{State: "PENDING"}}
	response := serve(t, validHandler(inbox), body, signed(body, webhookSecret, time.Now()), "application/json")
	if response.Code != http.StatusOK || inbox.calls != 1 {
		t.Fatalf("duplicate status=%d calls=%d", response.Code, inbox.calls)
	}
	inbox.err = domain.ErrConflict
	response = serve(t, validHandler(inbox), body, signed(body, webhookSecret, time.Now()), "application/json")
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d", response.Code)
	}
}

func TestStripeWebhookDatabaseFailureNeverAcknowledges(t *testing.T) {
	inbox := &fakeInbox{err: domain.ErrUnavailable}
	body := eventBody("evt_db", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	response := serve(t, validHandler(inbox), body, signed(body, webhookSecret, time.Now()), "application/json")
	if response.Code != http.StatusServiceUnavailable || response.Code/100 == 2 {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestStripeWebhookSignatureAndBodyRefusals(t *testing.T) {
	body := eventBody("evt_sig", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	now := time.Now()
	cases := map[string]struct {
		body      []byte
		signature string
		status    int
	}{
		"forged":    {body, signed(body, otherSecret, now), http.StatusBadRequest},
		"mutated":   {append(append([]byte(nil), body...), '\n'), signed(body, webhookSecret, now), http.StatusBadRequest},
		"stale":     {body, signed(body, webhookSecret, now.Add(-301*time.Second)), http.StatusBadRequest},
		"missing":   {body, "", http.StatusBadRequest},
		"oversized": {body, strings.Repeat("v1=", 1400), http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			inbox := &fakeInbox{result: app.InboxResult{Created: true}}
			response := serve(t, validHandler(inbox), tc.body, tc.signature, "application/json")
			if response.Code != tc.status || inbox.calls != 0 {
				t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
			}
		})
	}
}

func TestStripeWebhookBindingRefusals(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		body []byte
	}{
		"wrong-mode":      {eventBody("evt_mode", "invoice.paid", "2026-09-30.endive", true, "", "invoice", "in_1")},
		"wrong-version":   {eventBody("evt_version", "invoice.paid", "2020-01-01.legacy", false, "", "invoice", "in_1")},
		"missing-version": {eventBody("evt_no_version", "invoice.paid", "", false, "", "invoice", "in_1")},
		"connect":         {eventBody("evt_connect", "invoice.paid", "2026-09-30.endive", false, "acct_other", "invoice", "in_1")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			inbox := &fakeInbox{result: app.InboxResult{Created: true}}
			response := serve(t, validHandler(inbox), tc.body, signed(tc.body, webhookSecret, now), "application/json")
			if response.Code != http.StatusBadRequest || inbox.calls != 0 {
				t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
			}
		})
	}
}

func TestStripeWebhookContentTypeAndSizeBounds(t *testing.T) {
	body := eventBody("evt_size", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	inbox := &fakeInbox{result: app.InboxResult{Created: true}}
	handler := validHandler(inbox)
	response := serve(t, handler, body, signed(body, webhookSecret, time.Now()), "text/plain")
	if response.Code != http.StatusBadRequest || inbox.calls != 0 {
		t.Fatalf("content type status=%d", response.Code)
	}
	handler.MaxBodyBytes = 16
	response = serve(t, handler, body, signed(body, webhookSecret, time.Now()), "application/json")
	if response.Code != http.StatusRequestEntityTooLarge || inbox.calls != 0 {
		t.Fatalf("size status=%d", response.Code)
	}
}

func TestStripeWebhookSignatureRotation(t *testing.T) {
	body := eventBody("evt_rotate", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	now := time.Now()
	newSig := webhook.ComputeSignature(now, body, webhookSecret)
	oldSig := webhook.ComputeSignature(now, body, otherSecret)
	rotation := fmt.Sprintf("t=%d,v1=%s,v1=%s", now.Unix(), hex.EncodeToString(newSig), hex.EncodeToString(oldSig))
	inbox := &fakeInbox{result: app.InboxResult{Created: true}}
	response := serve(t, validHandler(inbox), body, rotation, "application/json")
	if response.Code != http.StatusOK || inbox.calls != 1 {
		t.Fatalf("rotation status=%d calls=%d", response.Code, inbox.calls)
	}
	oldOnly := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(oldSig))
	response = serve(t, validHandler(inbox), body, oldOnly, "application/json")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("old-only status=%d", response.Code)
	}
}

func TestStripeWebhookMissingConfigAndEventID(t *testing.T) {
	inbox := &fakeInbox{result: app.InboxResult{Created: true}}
	handler := validHandler(inbox)
	handler.Secret = ""
	body := eventBody("evt_config", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	if response := serve(t, handler, body, "", "application/json"); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("config status=%d", response.Code)
	}
	handler = validHandler(inbox)
	body = eventBody("", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_1")
	if response := serve(t, handler, body, signed(body, webhookSecret, time.Now()), "application/json"); response.Code != http.StatusBadRequest {
		t.Fatalf("event id status=%d", response.Code)
	}
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
