// PAY-18 adversarial privacy acceptance: webhook refusals and durable
// intake must never echo card data, secrets, QR/payment URLs or tokens in
// response bodies, and the inbox must retain only hashes and bounded IDs.
package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

const (
	pay18PANMarker    = "4111111111111111"
	pay18CVVMarker    = "cvv_synthetic_321"
	pay18SecretMarker = "whsec_synthetic_leak_marker_987654321"
	pay18QRMarker     = "https://pay.asaas.example/synthetic-qr-marker-123"
	pay18TokenMarker  = "asaas-access-token-synthetic-marker-456"
)

var pay18Markers = []string{
	pay18PANMarker,
	pay18CVVMarker,
	pay18SecretMarker,
	pay18QRMarker,
	pay18TokenMarker,
}

func pay18MarkerBody() string {
	return strings.Join(pay18Markers, "|")
}

func assertPAY18Clean(t *testing.T, name string, response *httptest.ResponseRecorder) {
	t.Helper()
	body := response.Body.String()
	for _, marker := range pay18Markers {
		if strings.Contains(body, marker) {
			t.Fatalf("%s: response echoes %q in %q", name, marker, body)
		}
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("%s: Cache-Control=%q want no-store", name, cache)
	}
}

func assertPAY18InboxHashesOnly(t *testing.T, name string, event app.InboxEvent, raw []byte) {
	t.Helper()
	if event.PayloadHash != sha256Hex(raw) {
		t.Fatalf("%s: payload hash mismatch", name)
	}
	dump := fmt.Sprintf("%+v", event)
	for _, marker := range []string{pay18PANMarker, pay18CVVMarker, pay18SecretMarker, pay18TokenMarker} {
		if strings.Contains(dump, marker) {
			t.Fatalf("%s: inbox retains %q in %+v", name, marker, event)
		}
	}
}

func TestPAY18StripeRefusalsNeverEchoSecrets(t *testing.T) {
	tainted := eventBody("evt_p18", "invoice.paid", "2026-09-30.endive", false, "", "invoice", "in_p18")
	// Inject card/secret markers as additive unknown fields: signature stays
	// valid, the SDK ignores them, and nothing may be echoed or retained.
	tainted = []byte(strings.Replace(string(tainted), `{"id":`,
		fmt.Sprintf(`{"pan":%q,"cvv":%q,"secret":%q,"qr":%q,"token":%q,"id":`,
			pay18PANMarker, pay18CVVMarker, pay18SecretMarker, pay18QRMarker, pay18TokenMarker), 1))
	now := time.Now()

	t.Run("forged-signature", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true}}
		response := serve(t, validHandler(inbox), tainted, signed(tainted, otherSecret, now), "application/json")
		if response.Code != http.StatusBadRequest || inbox.calls != 0 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "forged-signature", response)
	})

	t.Run("oversized-body", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true}}
		handler := validHandler(inbox)
		handler.MaxBodyBytes = 16
		response := serve(t, handler, tainted, signed(tainted, webhookSecret, now), "application/json")
		if response.Code != http.StatusRequestEntityTooLarge || inbox.calls != 0 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "oversized-body", response)
	})

	t.Run("database-outage", func(t *testing.T) {
		inbox := &fakeInbox{err: domain.ErrUnavailable}
		response := serve(t, validHandler(inbox), tainted, signed(tainted, webhookSecret, now), "application/json")
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d", response.Code)
		}
		assertPAY18Clean(t, "database-outage", response)
	})

	t.Run("durable-intake-hashes-only", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true, State: "PENDING"}}
		response := serve(t, validHandler(inbox), tainted, signed(tainted, webhookSecret, now), "application/json")
		if response.Code != http.StatusOK || inbox.calls != 1 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "durable-intake", response)
		assertPAY18InboxHashesOnly(t, "durable-intake", inbox.last, tainted)
	})
}

func TestPAY18AsaasRefusalsNeverEchoSecrets(t *testing.T) {
	tainted := []byte(fmt.Sprintf(
		`{"id":"evt_p18","event":"PAYMENT_RECEIVED","dateCreated":"2026-10-24 12:00:00","account":{"id":%q},"payment":{"id":"pay_p18"},"pan":%q,"cvv":%q,"secret":%q,"qr":%q}`,
		asaasMerchant, pay18PANMarker, pay18CVVMarker, pay18SecretMarker, pay18QRMarker))

	t.Run("wrong-token", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true}}
		response := serveAsaas(t, validAsaasHandler(inbox), tainted,
			"wrong-token-but-long-enough-123456", "application/json", false)
		if response.Code != http.StatusUnauthorized || inbox.calls != 0 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "wrong-token", response)
	})

	t.Run("malformed-body", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true}}
		malformed := append(append([]byte(nil), tainted...), []byte(`,"pan":`+pay18MarkerBody())...)
		response := serveAsaas(t, validAsaasHandler(inbox), malformed, asaasToken, "application/json", false)
		if response.Code != http.StatusBadRequest || inbox.calls != 0 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "malformed-body", response)
	})

	t.Run("oversized-body", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true}}
		handler := validAsaasHandler(inbox)
		handler.MaxBodyBytes = 16
		response := serveAsaas(t, handler, tainted, asaasToken, "application/json", false)
		if response.Code != http.StatusRequestEntityTooLarge || inbox.calls != 0 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "oversized-body", response)
	})

	t.Run("database-outage", func(t *testing.T) {
		inbox := &fakeInbox{err: domain.ErrUnavailable}
		response := serveAsaas(t, validAsaasHandler(inbox), tainted, asaasToken, "application/json", false)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d", response.Code)
		}
		assertPAY18Clean(t, "database-outage", response)
	})

	t.Run("durable-intake-hashes-only", func(t *testing.T) {
		inbox := &fakeInbox{result: app.InboxResult{Created: true, State: "PENDING"}}
		response := serveAsaas(t, validAsaasHandler(inbox), tainted, asaasToken, "application/json", false)
		if response.Code != http.StatusOK || inbox.calls != 1 {
			t.Fatalf("status=%d calls=%d", response.Code, inbox.calls)
		}
		assertPAY18Clean(t, "durable-intake", response)
		assertPAY18InboxHashesOnly(t, "durable-intake", inbox.last, tainted)
	})
}
