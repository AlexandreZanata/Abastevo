package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// SEC07-B (AUD-09): per-issuer JWKS expiry, one bounded refresh on
// unknown/removed kids, fail-closed outage, refused redirects. Local
// httptest servers only; no external network.

// Alternating providers past the TTL: google's refresh must not prolong
// apple's TTL, and apple's fetch must not prolong google's.
func TestPerIssuerExpiryIndependent(t *testing.T) {
	google, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	apple, err := NewStubIssuer("https://appleid.apple.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	src := &countingSource{inner: NewMemKeys(map[string]StubIssuer{
		"https://accounts.google.com": google,
		"https://appleid.apple.com":   apple,
	})}
	cached := &CachedKeys{Source: src, TTL: time.Hour, Now: func() time.Time { return now }}
	ctx := context.Background()

	if _, err := cached.Fetch(ctx, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(50 * time.Minute)
	cached.Now = func() time.Time { return now }
	if _, err := cached.Fetch(ctx, "https://appleid.apple.com"); err != nil {
		t.Fatal(err)
	}
	// Google is now 70m old (expired) while apple is 20m old (fresh):
	// only google may refetch.
	now = now.Add(20 * time.Minute)
	cached.Now = func() time.Time { return now }
	if _, err := cached.Fetch(ctx, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.Fetch(ctx, "https://appleid.apple.com"); err != nil {
		t.Fatal(err)
	}
	if got := src.count(); got != 3 {
		t.Fatalf("per-issuer expiry must refetch only google, fetched %d, want 3", got)
	}
}

// Rotation propagates through exactly one bounded refresh; a removed key
// still refuses after that refresh.
func TestUnknownKidRefreshThenRemovalRefuses(t *testing.T) {
	first, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	inner := NewMemKeys(map[string]StubIssuer{"https://accounts.google.com": first})
	src := &countingSource{inner: inner}
	cached := &CachedKeys{Source: src, TTL: time.Hour, Now: func() time.Time { return now }}
	v := &Verifier{
		Clock:    func() time.Time { return now },
		Issuers:  map[string]string{"google": "https://accounts.google.com"},
		Audience: testAudience,
		Keys:     cached,
		Nonces:   NewMemNonces(),
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	ctx := context.Background()

	rotated, err := second.Token("user-123", "", testAudience, "sec07b-rot", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Upstream still serves the old key: one refresh, then refuse.
	if _, err := v.Verify(ctx, "google", rotated, testAudience, "sec07b-rot"); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Fatalf("pre-rotation kid must refuse, got %v", err)
	}
	if got := src.count(); got != 2 {
		t.Fatalf("unknown kid must trigger exactly one refresh, fetched %d, want 2", got)
	}
	// Upstream rotates: recovery resumes after the bounded refresh interval.
	now = now.Add(ForcedRefreshInterval + time.Second)
	inner.mu.Lock()
	inner.sets["https://accounts.google.com"] = []JSONWebKey{second.PublicJWK()}
	inner.mu.Unlock()
	rotatedFresh, err := second.Token("user-123", "", testAudience, "sec07b-rot2", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, "google", rotatedFresh, testAudience, "sec07b-rot2"); err != nil {
		t.Fatalf("rotated-in key must verify after refresh, got %v", err)
	}
	// Upstream removes the key again: past the TTL the cache refetches,
	// the kid is unknown, one bounded refresh still misses, then refuse.
	removed, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	inner.mu.Lock()
	inner.sets["https://accounts.google.com"] = []JSONWebKey{removed.PublicJWK()}
	inner.mu.Unlock()
	now = now.Add(2 * time.Hour)
	gone, err := second.Token("user-123", "", testAudience, "sec07b-rot3", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, "google", gone, testAudience, "sec07b-rot3"); !errors.Is(err, domain.ErrOIDCUnknownIssuer) {
		t.Fatalf("removed key must refuse after refresh, got %v", err)
	}
}

// A failed refresh fails closed as unavailable, never as verified.
func TestRefreshOutageFailsClosed(t *testing.T) {
	issuer, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	cached := &CachedKeys{
		Source: &flakySource{keys: NewMemKeys(map[string]StubIssuer{"https://accounts.google.com": issuer})},
		TTL:    time.Hour,
		Now:    func() time.Time { return now },
	}
	v := &Verifier{
		Clock:    func() time.Time { return now },
		Issuers:  map[string]string{"google": "https://accounts.google.com"},
		Audience: testAudience,
		Keys:     cached,
		Nonces:   NewMemNonces(),
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	ctx := context.Background()
	other, err := NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := other.Token("user-123", "", testAudience, "sec07b-out", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, "google", tok, testAudience, "sec07b-out"); !errors.Is(err, domain.ErrOIDCUnavailable) {
		t.Fatalf("failed refresh must be unavailable, got %v", err)
	}
}

type flakySource struct {
	calls int
	keys  KeySource
}

func (f *flakySource) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	f.calls++
	if f.calls > 1 {
		return nil, domain.ErrOIDCUnavailable
	}
	return f.keys.Fetch(ctx, issuer)
}

// A redirecting JWKS endpoint is refused without following the target:
// the attacker server must see zero requests.
func TestJWKSRedirectRefused(t *testing.T) {
	var attackerHits atomic.Int64
	attacker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attackerHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(attacker.Close)
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	h := HTTPKeys{Client: redirector.Client(), URLs: map[string]string{"https://accounts.google.com": redirector.URL}}
	_, err := h.Fetch(context.Background(), "https://accounts.google.com")
	if err == nil {
		t.Fatal("redirecting JWKS must be refused")
	}
	if !strings.Contains(err.Error(), "redirects refused") {
		t.Fatalf("refusal must name the redirect policy, got %v", err)
	}
	if got := attackerHits.Load(); got != 0 {
		t.Fatalf("redirect target must see zero requests, saw %d", got)
	}
}
