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
type CachedKeys struct {
	Source KeySource
	TTL    time.Duration
	Now    func() time.Time
	mu     sync.Mutex
	at     time.Time
	sets   map[string][]JSONWebKey
}

// Fetch serves the cached set while fresh, refetching past the TTL.
func (c *CachedKeys) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.Now()
	if c.sets != nil {
		if set, ok := c.sets[issuer]; ok && now.Sub(c.at) < c.TTL {
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
	c.sets[issuer] = set
	c.at = now
	return set, nil
}

// HTTPKeys fetches issuer JWKS documents over HTTPS for production wiring.
// Responses are size-capped and JSON-decoded strictly; redirects, file URLs
// and non-HTTPS issuers are refused before any byte is trusted.
type HTTPKeys struct {
	Client *http.Client
	URLs   map[string]string
}

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
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
