package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CachedKeys fronts any KeySource with the frozen JWKS TTL (P13-T01):
// provider keys are fetched once per TTL per process, so verification
// stays local and fast while rotation still propagates within the hour.
// SEC07-AUD-09: expiry is per issuer — one provider's refresh never
// prolongs another provider's TTL.
type CachedKeys struct {
	Source KeySource
	TTL    time.Duration
	Now    func() time.Time
	mu     sync.Mutex
	at     map[string]time.Time
	sets   map[string][]JSONWebKey
}

// Fetch serves the cached set while fresh, refetching past the TTL.
// A failed refetch fails closed (no stale serve); the previous stamp is
// kept so the next call retries instead of serving expired keys.
func (c *CachedKeys) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.Now()
	if set, ok := c.sets[issuer]; ok {
		if at, stamped := c.at[issuer]; stamped && now.Sub(at) < c.TTL {
			return set, nil
		}
	}
	set, err := c.Source.Fetch(ctx, issuer)
	if err != nil {
		return nil, err
	}
	if c.sets == nil {
		c.sets = map[string][]JSONWebKey{}
	}
	if c.at == nil {
		c.at = map[string]time.Time{}
	}
	c.sets[issuer] = set
	c.at[issuer] = now
	return set, nil
}

// RefreshingKeySource is a KeySource that can bypass its TTL once.
// The verifier uses it for a single bounded retry when the cached set
// lacks the token's kid (rotation or removal upstream).
type RefreshingKeySource interface {
	KeySource
	Refresh(ctx context.Context, issuer string) ([]JSONWebKey, error)
}

// Refresh refetches the issuer set unconditionally (one bounded fetch),
// restamping only that issuer's expiry.
func (c *CachedKeys) Refresh(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	set, err := c.Source.Fetch(ctx, issuer)
	if err != nil {
		return nil, err
	}
	if c.sets == nil {
		c.sets = map[string][]JSONWebKey{}
	}
	if c.at == nil {
		c.at = map[string]time.Time{}
	}
	c.sets[issuer] = set
	c.at[issuer] = c.Now()
	return set, nil
}

// HTTPKeys fetches issuer JWKS documents over HTTPS for production wiring.
// Responses are size-capped and JSON-decoded strictly; non-HTTPS issuers
// are refused before any byte is trusted. Redirects are never followed:
// JWKS endpoints are stable, and the default Go client would silently
// follow a 302 to an attacker host (SEC07-AUD-09) — the policy below
// fails the fetch instead, for caller-supplied clients as well.
type HTTPKeys struct {
	Client *http.Client
	URLs   map[string]string
}

// errNoRedirect fails a JWKS fetch on any redirect (SEC07-AUD-09).
var errNoRedirect = errors.New("oidc: JWKS redirects refused")

// Fetch GETs the configured JWKS document for issuer.
func (h HTTPKeys) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	endpoint, ok := h.URLs[issuer]
	if !ok {
		return nil, fmt.Errorf("oidc: no JWKS endpoint for %q", issuer)
	}
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, errors.New("oidc: JWKS endpoint must use https")
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	// Constrain redirects without mutating the caller's client.
	pinned := *client
	pinned.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errNoRedirect
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := pinned.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: JWKS status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Keys []JSONWebKey `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Keys) == 0 {
		return nil, errors.New("oidc: empty JWKS")
	}
	return doc.Keys, nil
}

// ProductionJWKS maps the frozen issuer allowlist to provider documents.
func ProductionJWKS() map[string]string {
	return map[string]string{
		"https://accounts.google.com": "https://www.googleapis.com/oauth2/v3/certs",
		"https://appleid.apple.com":   "https://appleid.apple.com/auth/keys",
	}
}

// countingSource wraps a KeySource for cache tests.
type countingSource struct {
	mu    sync.Mutex
	calls int
	inner KeySource
}

func (c *countingSource) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.Fetch(ctx, issuer)
}

func (c *countingSource) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}
