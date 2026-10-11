package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// SEC02-B trusted-resolver regressions (AUD-03, B-BR-015): behind the
// supplied Caddy edge independent clients bill independently via the
// proxy-observed forwarding address; untrusted headers never select
// the subject; malformed chains fall back to the peer. Formerly
// skipped SEC02-A regressions, now enforced (GREEN).

type proxyProbe struct {
	mu       sync.Mutex
	subjects []string
}

func (p *proxyProbe) handler() Handler {
	proxies, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		panic(err)
	}
	return Handler{
		Registrar:      noRegistrar{},
		Authority:      "test.invalid",
		QuotaSecret:    []byte(strings.Repeat("a", 32)),
		TrustedProxies: proxies,
		CheckQuota: func(_ context.Context, subject, _ string) (time.Duration, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.subjects = append(p.subjects, subject)
			return 0, nil
		},
	}
}

func (p *proxyProbe) challenge(t *testing.T, remoteAddr, forwardedFor, fingerprint string) int {
	t.Helper()
	r := chi.NewRouter()
	p.handler().RegisterRoutes(r)
	body := `{"fingerprint":"` + fingerprint + `","purpose":"REGISTER"}`
	req := httptest.NewRequest("POST", "/v1/identity/challenges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func (p *proxyProbe) ipSubjects() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, s := range p.subjects {
		if strings.HasPrefix(s, "ip:") {
			out = append(out, s)
		}
	}
	return out
}

func TestProxyPeersShareOneBucket(t *testing.T) {
	p := &proxyProbe{}
	fpA := "fp:" + strings.Repeat("a", 64)
	fpB := "fp:" + strings.Repeat("b", 64)
	if code := p.challenge(t, "10.0.0.5:41230", "198.51.100.7", fpA); code == 400 {
		t.Fatalf("trusted proxy request refused: %d", code)
	}
	if code := p.challenge(t, "10.0.0.5:41231", "198.51.100.99", fpB); code == 400 {
		t.Fatalf("trusted proxy request refused: %d", code)
	}
	seen := map[string]bool{}
	for _, s := range p.ipSubjects() {
		seen[s] = true
	}
	if len(seen) < 2 {
		t.Fatalf("two independent clients behind one proxy share %d IP subject(s), want 2", len(seen))
	}
}

func TestSpoofedForwardingHeaderIgnored(t *testing.T) {
	p := &proxyProbe{}
	fp := "fp:" + strings.Repeat("c", 64)
	// Direct (untrusted peer) request forging X-Forwarded-For: the
	// quota subject must derive from the TCP peer, never the header.
	if code := p.challenge(t, "203.0.113.9:50000", "198.51.100.7", fp); code == 400 {
		t.Fatalf("direct request refused: %d", code)
	}
	got := p.ipSubjects()
	if len(got) != 1 {
		t.Fatalf("subjects = %d, want 1", len(got))
	}
	if got[0] == "" || strings.Contains(got[0], "198.51.100.7") {
		t.Fatalf("forged header selected the quota subject: %q", got[0])
	}
}

func TestMalformedForwardingFallsBackToPeer(t *testing.T) {
	p := &proxyProbe{}
	fp := "fp:" + strings.Repeat("d", 64)
	for _, xff := range []string{"not-an-ip", "198.51.100.7, 198.51.100.8", ""} {
		if code := p.challenge(t, "10.0.0.5:41230", xff, fp); code == 400 {
			t.Fatalf("xff %q refused: %d", xff, code)
		}
	}
	if got := len(p.ipSubjects()); got != 3 {
		t.Fatalf("subjects = %d, want 3 (one peer fallback each)", got)
	}
}
