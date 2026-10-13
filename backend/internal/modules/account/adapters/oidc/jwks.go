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

// ForcedRefreshInterval bounds unknown-kid recovery and provider failures.
const ForcedRefreshInterval = 30 * time.Second

// CachedKeys keeps independent, bounded issuer entries. The global mutex protects
// only the map; network I/O uses a cancellable single-flight gate per issuer.
type CachedKeys struct {
	Source  KeySource
	TTL     time.Duration
	Now     func() time.Time
	mu      sync.Mutex
	entries map[string]*issuerKeys
}

type issuerKeys struct {
	gate        chan struct{}
	set         []JSONWebKey
	hasSet      bool
	at          time.Time
	lastAttempt time.Time
	loadErr     error
	forced      bool
	forceAt     time.Time
	forceErr    error
}

func (c *CachedKeys) entry(issuer string) (*issuerKeys, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry := c.entries[issuer]; entry != nil {
		return entry, nil
	}
	// Issuers originate in the configured verifier allowlist, not JWT claims.
	// A cap also prevents misuse of this port from growing arbitrary maps.
	if len(c.entries) >= 16 {
		return nil, errors.New("oidc: issuer capacity exhausted")
	}
	if c.entries == nil {
		c.entries = map[string]*issuerKeys{}
	}
	entry := &issuerKeys{gate: make(chan struct{}, 1)}
	c.entries[issuer] = entry
	return entry, nil
}

func (c *CachedKeys) currentTime() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *CachedKeys) load(ctx context.Context, issuer string, force bool) ([]JSONWebKey, error) {
	entry, err := c.entry(issuer)
	if err != nil {
		return nil, err
	}
	select {
	case entry.gate <- struct{}{}:
		defer func() { <-entry.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	now := c.currentTime()
	ttl := c.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	if force {
		if entry.forced && now.Sub(entry.forceAt) < ForcedRefreshInterval {
			if entry.forceErr != nil {
				return nil, entry.forceErr
			}
			return append([]JSONWebKey(nil), entry.set...), nil
		}
		entry.forced = true
		entry.forceAt = now
	} else {
		if entry.hasSet && now.Sub(entry.at) < ttl {
			return append([]JSONWebKey(nil), entry.set...), nil
		}
		if entry.loadErr != nil && now.Sub(entry.lastAttempt) < ForcedRefreshInterval {
			return nil, entry.loadErr
		}
	}
	entry.lastAttempt = now
	if c.Source == nil {
		return nil, errors.New("oidc: key source unavailable")
	}
	set, err := c.Source.Fetch(ctx, issuer)
	entry.loadErr = err
	if force {
		entry.forceErr = err
	}
	if err != nil {
		return nil, err
	}
	entry.set = append([]JSONWebKey(nil), set...)
	entry.hasSet = true
	entry.at = now
	return append([]JSONWebKey(nil), entry.set...), nil
}

// Fetch never serves expired keys after a failed fetch.
func (c *CachedKeys) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	return c.load(ctx, issuer, false)
}

// RefreshingKeySource supports admitted recovery when a cached kid is unknown.
type RefreshingKeySource interface {
	KeySource
	Refresh(ctx context.Context, issuer string) ([]JSONWebKey, error)
}

// Refresh bypasses TTL at most once per issuer per ForcedRefreshInterval.
// No per-kid negative map exists: the admitted recovered set is the bounded
// negative cache for all unknown kids during the interval.
func (c *CachedKeys) Refresh(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	return c.load(ctx, issuer, true)
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
