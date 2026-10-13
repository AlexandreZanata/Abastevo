package play

import (
	"context"
	"encoding/json"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func serverClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	base := server.Client().Transport
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "androidpublisher.googleapis.com" {
			t.Error("unexpected provider authority")
		}
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		clone.URL = &u
		return base.RoundTrip(clone)
	})}
}
func response() map[string]any {
	return map[string]any{
		"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE", "acknowledgementState": "ACKNOWLEDGEMENT_STATE_PENDING",
		"testPurchase": map[string]any{}, "externalAccountIdentifiers": map[string]any{"obfuscatedExternalAccountId": "expected-account"},
		"lineItems": []any{map[string]any{"productId": "synthetic.premium", "expiryTime": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), "offerDetails": map[string]any{"basePlanId": "synthetic.monthly"}}},
	}
}
func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	c, err := New(Config{PackageName: "com.anpfuel", Environment: "test", Plans: map[string][]string{"synthetic.premium": {"synthetic.monthly"}}}, serverClient(t, h), func(context.Context) (string, error) { return "synthetic-oauth", nil })
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func request() app.ProviderRequest {
	return app.ProviderRequest{Token: "synthetic-token", ExpectedAccount: "expected-account", PackageName: "com.anpfuel", Environment: "test"}
}
func TestPlayCanonicalVerificationAndBinding(t *testing.T) {
	for _, name := range []string{"valid", "foreign-account", "wrong-product", "wrong-base-plan", "wrong-environment", "unknown-state", "pending", "bad-expiry", "bad-ack"} {
		t.Run(name, func(t *testing.T) {
			c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-oauth" || r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/purchases/subscriptionsv2/tokens/synthetic-token") {
					t.Error("bad canonical request")
				}
				p := response()
				switch name {
				case "foreign-account":
					p["externalAccountIdentifiers"] = map[string]any{"obfuscatedExternalAccountId": "foreign"}
				case "wrong-product":
					p["lineItems"].([]any)[0].(map[string]any)["productId"] = "foreign"
				case "wrong-base-plan":
					p["lineItems"].([]any)[0].(map[string]any)["offerDetails"] = map[string]any{"basePlanId": "foreign"}
				case "wrong-environment":
					delete(p, "testPurchase")
				case "unknown-state":
					p["subscriptionState"] = "FUTURE_STATE"
				case "pending":
					p["subscriptionState"] = "SUBSCRIPTION_STATE_PENDING"
					delete(p["lineItems"].([]any)[0].(map[string]any), "expiryTime")
				case "bad-expiry":
					p["lineItems"].([]any)[0].(map[string]any)["expiryTime"] = "not-a-date"
				case "bad-ack":
					p["acknowledgementState"] = "unknown"
				}
				_ = json.NewEncoder(w).Encode(p)
			})
			got, err := c.Verify(context.Background(), request())
			switch name {
			case "valid":
				if err != nil || got.Snapshot.State != domain.Active {
					t.Fatal(err)
				}
			case "unknown-state":
				if err != nil || got.Snapshot.State != domain.Unknown {
					t.Fatal(err)
				}
			case "pending":
				if err != nil || got.Snapshot.State != domain.Pending || !got.Snapshot.ValidUntil.IsZero() {
					t.Fatal(err)
				}
			default:
				if !errors.Is(err, domain.ErrDenied) {
					t.Fatal("invalid proof granted", err)
				}
			}
		})
	}
}
func TestPlayFaultsRedactionAndCancellation(t *testing.T) {
	for _, name := range []string{"redirect", "404", "429", "500", "oversize", "malformed", "canceled"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch name {
				case "redirect":
					w.Header().Set("Location", "https://attacker.invalid/synthetic-token")
					w.WriteHeader(302)
				case "404":
					w.WriteHeader(404)
				case "429":
					w.WriteHeader(429)
				case "500":
					w.WriteHeader(500)
					_, _ = io.WriteString(w, "provider secret synthetic-token")
				case "oversize":
					_, _ = io.WriteString(w, strings.Repeat("x", 65537))
				case "malformed":
					_, _ = io.WriteString(w, "not-json")
				default:
					_ = json.NewEncoder(w).Encode(response())
				}
			})
			ctx := context.Background()
			if name == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := c.Verify(ctx, request())
			if err == nil || strings.Contains(err.Error(), "synthetic-token") || strings.Contains(err.Error(), "secret") {
				t.Fatal("fault accepted/leaked", err)
			}
			if name == "redirect" && calls != 1 {
				t.Fatal("followed redirect")
			}
		})
	}
}

func TestPlayAcknowledgementIsCanonicalAndBounded(t *testing.T) {
	called := false
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != "POST" || r.URL.Path != "/androidpublisher/v3/applications/com.anpfuel/purchases/subscriptions/synthetic.premium/tokens/synthetic-token:acknowledge" {
			t.Error("wrong acknowledgement")
		}
		w.WriteHeader(204)
	})
	if err := c.Acknowledge(context.Background(), request(), "synthetic.premium"); err != nil || !called {
		t.Fatal(err)
	}
	if err := c.Acknowledge(context.Background(), request(), "foreign"); err == nil {
		t.Fatal("foreign product acknowledged")
	}
}
