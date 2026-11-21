package adapters

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
)

func TestWebhookEndpointRequiresCanonicalPublicHTTPSAuthority(t *testing.T) {
	if !validWebhookEndpoint("https://connector.example/v1/fleet") {
		t.Fatal("canonical endpoint rejected")
	}
	for _, endpoint := range []string{
		"http://connector.example/v1/fleet", "https://127.0.0.1/v1/fleet", "https://[::1]/v1/fleet",
		"https://connector.example:443/v1/fleet", "https://user@connector.example/v1/fleet",
		"https://connector.example/v1/fleet?key=synthetic", "https://connector.example/v1/fleet#fragment",
		"https://connector.example/v1/../fleet", "https://connector.example/v1/%2Ffleet",
		"https://connector.example./v1/fleet", "https://CONNECTOR.example/v1/fleet",
	} {
		if validWebhookEndpoint(endpoint) {
			t.Errorf("invalid endpoint accepted: %q", endpoint)
		}
	}
	if conn, err := dialWebhook(t.Context(), "tcp", "127.0.0.1:443"); err == nil {
		conn.Close()
		t.Fatal("unreviewed address was dialed")
	}
}

func TestWebhookHTTPSUsesStableSignatureAndRefusesRedirect(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	const refID = "00000000-0000-4000-8000-000000000002"
	ref, err := domain.NewCredentialRef(refID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	key := bytes.Repeat([]byte{7}, 32)
	if err := os.WriteFile(filepath.Join(dir, refID), key, 0o600); err != nil {
		t.Fatal(err)
	}
	keys, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Close()
	body := []byte(`{"version":1,"delivery_id":"00000000-0000-4000-8000-000000000001"}`)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	wantSignature := "v1=" + hex.EncodeToString(mac.Sum(nil))
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(posted, body) || r.Header.Get("Idempotency-Key") != id ||
			r.Header.Get("Abastevo-Signature") != wantSignature || r.Header.Get("Authorization") != "" {
			t.Error("endpoint did not receive the exact signed request")
		}
		switch r.URL.Path {
		case "/accept":
			w.WriteHeader(http.StatusNoContent)
		case "/retry":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/redirect":
			w.Header().Set("Location", "https://example.com/forbidden")
			w.WriteHeader(http.StatusTemporaryRedirect)
		default:
			t.Error("unapproved endpoint reached")
		}
	}))
	defer fixture.Close()
	sender := NewHTTPSWebhookSender(keys)
	transport := sender.client.Transport.(*http.Transport)
	// Pin the synthetic reviewed hostname to the local TLS fixture in test only.
	transport.DialContext = (&net.Dialer{}).DialContext
	injected := fixture.Client().Transport.(*http.Transport).Clone()
	injected.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return transport.DialContext(ctx, network, fixture.Listener.Addr().String())
	}
	// The fixture certificate covers example.com.
	sender.client.Transport = injected
	for path, want := range map[string]WebhookResult{"/accept": WebhookDelivered, "/retry": WebhookRetry, "/redirect": WebhookFailed} {
		if actual := sender.SendWebhook(t.Context(), "https://example.com"+path, ref, id, body); actual != want {
			t.Errorf("%s outcome=%s want=%s", path, actual, want)
		}
	}
}
