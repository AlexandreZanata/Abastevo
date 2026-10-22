package jobs

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Allowlisted discovery origins. Nothing else is fetched, ever: URLs
// outside these hosts/paths refuse before any dial (no SSRF surface).
var allowedDiscovery = []struct{ host, prefix string }{
	{"raw.githubusercontent.com", "/openchargemap/ocm-export/"},
	{"api.github.com", "/repos/openchargemap/ocm-export/"},
	{"www.gov.br", "/inmetro/"},
}

// DiscoveryResult reports whether a newer source revision exists. It
// never downloads snapshots, keys or bulk data; fetching is a separate
// reviewed step with its own approval.
type DiscoveryResult struct {
	Available   bool
	Revision    string
	Description string
}

// Discover checks one allowlisted source index for a newer revision with
// a dead budget. Schedules stay off; callers trigger explicitly and
// retain the last accepted state on any failure.
func Discover(ctx context.Context, rawURL string) (DiscoveryResult, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" {
		return DiscoveryResult{}, errors.New("charging discovery: URL refused")
	}
	allowed := false
	for _, scope := range allowedDiscovery {
		if parsed.Host == scope.host && strings.HasPrefix(parsed.EscapedPath(), scope.prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return DiscoveryResult{}, errors.New("charging discovery: origin refused")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("charging discovery: redirect refused")
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return DiscoveryResult{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "abastevo-discovery")
	response, err := client.Do(request)
	if err != nil {
		return DiscoveryResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return DiscoveryResult{}, errors.New("charging discovery: source unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || len(body) == 0 {
		return DiscoveryResult{}, errors.New("charging discovery: body refused")
	}
	return DiscoveryResult{Available: true, Revision: parsed.String(), Description: "index reachable"}, nil
}
