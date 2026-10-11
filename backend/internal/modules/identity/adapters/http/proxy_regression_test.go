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

// SEC02-A proxy-fairness regressions (AUD-03): behind the supplied
// Caddy edge every client shares the TCP peer, so per-IP quotas
// collapse onto one bucket; untrusted forwarding headers must never
// select the quota subject. Each test asserts the future behavior
// and skips until SEC02-B implements the trusted resolver —
// committed skipped (not blessed, not breaking CI), mirroring the
// audit characterization without permanently asserting the flaw.

type proxyProbe struct {
	mu       sync.Mutex
	subjects []string
}

func (p *proxyProbe) handler() Handler {
	return Handler{
		Registrar:   noRegistrar{},
		Authority:   "test.invalid",
		QuotaSecret: []byte(strings.Repeat("a", 32)),
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
	t.Skip("SEC02-B: trusted proxy resolver pending")
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
	t.Skip("SEC02-B: trusted proxy resolver pending")
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
	t.Skip("SEC02-B: trusted proxy resolver pending")
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
