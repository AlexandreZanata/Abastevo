package http

import (
	"net/http"
	"testing"
	"time"
)

func FuzzStripeWebhook(f *testing.F) {
	event := eventBody("evt_fuzz", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_fuzz")
	f.Add(event, "", "application/json", true)
	f.Add(event, "t=1,v1=invalid", "application/json", false)
	f.Add([]byte("{}"), "t=1,t=2", "text/plain", true)
	f.Fuzz(func(t *testing.T, body []byte, header, contentType string, sign bool) {
		inbox := &fakeInbox{}
		if sign {
			header = signed(body, webhookSecret, time.Now())
		}
		response := serve(t, validHandler(inbox), body, header, contentType)
		if (response.Code == http.StatusOK) != (inbox.calls == 1) {
			t.Fatal("Stripe acknowledgement and receipt diverged")
		}
		if inbox.calls == 1 && (inbox.last.PayloadHash != sha256Hex(body) || inbox.last.Merchant != "acct_synthetic") {
			t.Fatal("Stripe receipt changed the authenticated binding")
		}
	})
}

func FuzzAsaasWebhook(f *testing.F) {
	event := asaasBody("evt_fuzz", "PAYMENT_RECEIVED", "payment", "pay_fuzz")
	f.Add(event, "", "application/json", true)
	f.Add(event, "invalid", "application/json", false)
	f.Add([]byte("{}"), asaasToken, "text/plain", false)
	f.Fuzz(func(t *testing.T, body []byte, token, contentType string, useValidToken bool) {
		inbox := &fakeInbox{}
		if useValidToken {
			token = asaasToken
		}
		response := serveAsaas(t, validAsaasHandler(inbox), body, token, contentType, true)
		if (response.Code == http.StatusOK) != (inbox.calls == 1) {
			t.Fatal("Asaas acknowledgement and receipt diverged")
		}
		if inbox.calls == 1 && (token != asaasToken || inbox.last.PayloadHash != sha256Hex(body) || inbox.last.Merchant != asaasMerchant) {
			t.Fatal("Asaas receipt changed the authenticated binding")
		}
	})
}
