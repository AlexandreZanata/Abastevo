package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const (
	testAudience = "anpfuel-backend"
	testNonce    = "n-07"
)

func testSetup(t *testing.T, alg string) (*Verifier, *StubIssuer, *MemNonces) {
	t.Helper()
	issuer, err := NewStubIssuer("https://accounts.google.com", alg)
	if err != nil {
		t.Fatal(err)
	}
	nonces := NewMemNonces()
	v := &Verifier{
		Clock:    func() time.Time { return time.Unix(1_800_000_000, 0) },
		Issuers:  map[string]string{"google": "https://accounts.google.com", "apple": "https://appleid.apple.com"},
		Audience: testAudience,
		Keys:     NewMemKeys(map[string]StubIssuer{"https://accounts.google.com": issuer}),
		Nonces:   nonces,
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	return v, &issuer, nonces
}

func TestValidGoogleAndApple(t *testing.T) {
	for _, alg := range []string{"RS256", "ES256"} {
		v, issuer, _ := testSetup(t, alg)
		tok, err := issuer.Token("user-123", "user@example.com", testAudience, testNonce, time.Unix(1_800_000_000, 0), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		got, err := v.Verify(context.Background(), "google", tok, testAudience, testNonce)
		if err != nil {
			t.Errorf("%s valid token rejected: %v", alg, err)
		}
		if got.Issuer != "https://accounts.google.com" || got.Subject != "user-123" {
			t.Errorf("%s subject binding wrong: %+v", alg, got)
		}
		if got.Email != "user@example.com" {
			t.Errorf("%s must carry the relay/display email, got %q", alg, got.Email)
		}
	}
}

func TestWrongIssuerAudienceExpiry(t *testing.T) {
	v, issuer, _ := testSetup(t, "RS256")
	now := time.Unix(1_800_000_000, 0)
	mint := func(iss, aud string, exp time.Time, nonce string) string {
		tok, err := issuer.TokenWith("user-123", "", aud, nonce, now, exp, iss)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	if _, err := v.Verify(context.Background(), "google", mint("https://evil.example.invalid", testAudience, now.Add(time.Hour), testNonce), testAudience, testNonce); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Errorf("evil issuer must fail unknown-issuer, got %v", err)
	}
	if _, err := v.Verify(context.Background(), "google", mint("https://accounts.google.com", "other-app", now.Add(time.Hour), testNonce), testAudience, testNonce); !errors.Is(err, domain.ErrOIDCWrongAudience) {
		t.Errorf("wrong audience must fail, got %v", err)
	}
	if _, err := v.Verify(context.Background(), "google", mint("https://accounts.google.com", testAudience, now.Add(-time.Hour), testNonce), testAudience, testNonce); !errors.Is(err, domain.ErrOIDCExpired) {
		t.Errorf("expired token must fail, got %v", err)
	}
	if _, err := v.Verify(context.Background(), "google", mint("https://accounts.google.com", testAudience, now.Add(-119*time.Second), "skew-ok"), testAudience, "skew-ok"); err != nil {
		t.Errorf("token inside 120s skew must pass, got %v", err)
	}
}

func TestNonceSingleUseAndMismatch(t *testing.T) {
	v, issuer, _ := testSetup(t, "RS256")
	now := time.Unix(1_800_000_000, 0)
	tok, err := issuer.Token("user-123", "", testAudience, testNonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), "google", tok, testAudience, testNonce); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if _, err := v.Verify(context.Background(), "google", tok, testAudience, testNonce); !errors.Is(err, domain.ErrOIDCNonceReused) {
		t.Errorf("replayed nonce must be nonce-reused, got %v", err)
	}
	tok2, err := issuer.Token("user-123", "", testAudience, "other-nonce", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), "google", tok2, testAudience, testNonce); !errors.Is(err, domain.ErrOIDCNonceMismatch) {
		t.Errorf("nonce mismatch must fail closed, got %v", err)
	}
}

func TestUnknownKidRotationAndTamper(t *testing.T) {
	v, issuer, _ := testSetup(t, "RS256")
	now := time.Unix(1_800_000_000, 0)
	other, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := other.Token("user-123", "", testAudience, "rot-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), "google", rotated, testAudience, "rot-1"); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Errorf("rotated-away key must fail (unknown kid), got %v", err)
	}
	_ = issuer
	valid, err := issuer.Token("user-123", "", testAudience, "tamper-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := splitToken(valid)
	parts[1] = parts[1][:len(parts[1])-2] + "AA"
	if _, err := v.Verify(context.Background(), "google", joinToken(parts), testAudience, "tamper-1"); err == nil {
		t.Error("tampered payload must fail")
	}
	if _, err := v.Verify(context.Background(), "google", "not.a.token", testAudience, testNonce); err == nil {
		t.Error("malformed token must fail")
	}
}

func TestProviderOutageFailsClosed(t *testing.T) {
	v, issuer, _ := testSetup(t, "RS256")
	now := time.Unix(1_800_000_000, 0)
	tok, err := issuer.Token("user-123", "", testAudience, "outage-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	v.Keys = FailingKeys{}
	if _, err := v.Verify(context.Background(), "google", tok, testAudience, "outage-1"); !errors.Is(err, domain.ErrOIDCUnavailable) {
		t.Errorf("outage must be unavailable, got %v", err)
	}
	if _, err := v.Verify(context.Background(), "unknown-provider", "x.y.z", testAudience, testNonce); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Errorf("unknown provider must fail unknown-issuer, got %v", err)
	}
}

func TestAppleRelayBindsOpaqueSubject(t *testing.T) {
	v, _, _ := testSetup(t, "ES256")
	apple, err := NewStubIssuer("https://appleid.apple.com", "ES256")
	if err != nil {
		t.Fatal(err)
	}
	v.Keys = NewMemKeys(map[string]StubIssuer{"https://appleid.apple.com": apple})
	now := time.Unix(1_800_000_000, 0)
	tok, err := apple.Token("opaque-apple-subject.001", "relay@privaterelay.appleid.com", testAudience, "apple-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Verify(context.Background(), "apple", tok, testAudience, "apple-1")
	if err != nil {
		t.Fatalf("apple relay token rejected: %v", err)
	}
	if got.Subject != "opaque-apple-subject.001" {
		t.Errorf("binding must use the opaque subject, got %+v", got)
	}
}

func splitToken(raw string) []string {
	parts := []string{"", "", ""}
	segs := splitSegs(raw)
	copy(parts, segs)
	return parts
}

func splitSegs(raw string) []string {
	var out []string
	cur := ""
	for _, c := range raw {
		if c == '.' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	return append(out, cur)
}

func joinToken(parts []string) string {
	return parts[0] + "." + parts[1] + "." + parts[2]
}

func TestJWKSCacheHonorsFrozenTTL(t *testing.T) {
	issuer, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	src := &countingSource{inner: NewMemKeys(map[string]StubIssuer{"https://accounts.google.com": issuer})}
	cached := &CachedKeys{Source: src, TTL: time.Hour, Now: func() time.Time { return now }}
	ctx := context.Background()
	if _, err := cached.Fetch(ctx, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.Fetch(ctx, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}
	if got := src.count(); got != 1 {
		t.Errorf("cache must fetch once, fetched %d", got)
	}
	now = now.Add(2 * time.Hour)
	cached.Now = func() time.Time { return now }
	if _, err := cached.Fetch(ctx, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}
	if got := src.count(); got != 2 {
		t.Errorf("expired cache must refetch, fetched %d", got)
	}
}

func TestHTTPKeysFetchAndRefuse(t *testing.T) {
	issuer, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	server := testHTTPServer(t, issuer.ServeJSON())
	h := HTTPKeys{Client: server.Client(), URLs: map[string]string{"https://accounts.google.com": server.URL}}
	keys, err := h.Fetch(context.Background(), "https://accounts.google.com")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(keys) != 1 || keys[0].Kid != issuer.Kid {
		t.Errorf("wrong key set: %+v", keys)
	}
	if _, err := h.Fetch(context.Background(), "https://unknown.example.invalid"); err == nil {
		t.Error("unconfigured issuer must fail")
	}
	plain := HTTPKeys{URLs: map[string]string{"https://accounts.google.com": "http://plain.example.invalid/keys"}}
	if _, err := plain.Fetch(context.Background(), "https://accounts.google.com"); err == nil {
		t.Error("plain http JWKS must be refused")
	}
}

func testHTTPServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}
