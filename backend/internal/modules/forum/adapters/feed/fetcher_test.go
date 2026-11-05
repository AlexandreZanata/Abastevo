package feed

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestSafeFeedFetcher_ValidationRules(t *testing.T) {
	fetcher := NewSafeFeedFetcher([]AllowedTarget{
		{Host: "agenciabrasil.ebc.com.br", PathPrefix: "/rss"},
		{Host: "transportes.gov.br", PathPrefix: "/"},
	})

	ctx := context.Background()

	// 1. Non-HTTPS rejected
	_, err := fetcher.Fetch(ctx, application.FetchRequest{
		URL: "http://agenciabrasil.ebc.com.br/rss/feed.xml",
	})
	if !errors.Is(err, domain.ErrEgressForbidden) {
		t.Fatalf("expected ErrEgressForbidden for http://, got %v", err)
	}

	// 2. User credentials rejected
	_, err = fetcher.Fetch(ctx, application.FetchRequest{
		URL: "https://user:pass@agenciabrasil.ebc.com.br/rss/feed.xml",
	})
	if !errors.Is(err, domain.ErrEgressForbidden) {
		t.Fatalf("expected ErrEgressForbidden for userinfo, got %v", err)
	}

	// 3. Custom port rejected
	_, err = fetcher.Fetch(ctx, application.FetchRequest{
		URL: "https://agenciabrasil.ebc.com.br:8443/rss/feed.xml",
	})
	if !errors.Is(err, domain.ErrEgressForbidden) {
		t.Fatalf("expected ErrEgressForbidden for port 8443, got %v", err)
	}

	// 4. Host not in allowlist rejected
	_, err = fetcher.Fetch(ctx, application.FetchRequest{
		URL: "https://evil.example.com/rss/feed.xml",
	})
	if !errors.Is(err, domain.ErrEgressForbidden) {
		t.Fatalf("expected ErrEgressForbidden for evil host, got %v", err)
	}

	// 5. Path outside authorized prefix rejected
	_, err = fetcher.Fetch(ctx, application.FetchRequest{
		URL: "https://agenciabrasil.ebc.com.br/admin/secrets.json",
	})
	if !errors.Is(err, domain.ErrEgressForbidden) {
		t.Fatalf("expected ErrEgressForbidden for unauthorized path, got %v", err)
	}
}

func TestSafeFeedFetcher_IPPolicy_RefusesPrivateAndMetadata(t *testing.T) {
	tests := []struct {
		ip      string
		allowed bool
	}{
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.5", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false}, // AWS/GCP metadata
		{"::1", false},
		{"fe80::1", false},
		{"::ffff:127.0.0.1", false}, // IPv4-mapped
		{"8.8.8.8", true},          // Public DNS
		{"1.1.1.1", true},          // Public DNS
		{"200.160.2.3", true},      // Public BR IP
	}

	for _, tc := range tests {
		addr := netip.MustParseAddr(tc.ip)
		got := PublicUnicastOnly(addr)
		if got != tc.allowed {
			t.Errorf("PublicUnicastOnly(%s) = %v, expected %v", tc.ip, got, tc.allowed)
		}
	}
}

func TestSafeFeedFetcher_ConditionalGETAndNotModified(t *testing.T) {
	etag := `"sample-etag-123"`
	lastMod := "Thu, 05 Nov 2026 12:00:00 GMT"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lastMod)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<rss><channel><title>Feed</title></channel></rss>"))
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	serverPort, _ := strconv.Atoi(u.Port())

	fetcher := &SafeFeedFetcher{
		AllowedTargets: []AllowedTarget{{Host: u.Hostname(), Port: serverPort, PathPrefix: "/"}},
		MaxBytes:       MaxFeedBodyBytes,
		MaxRedirects:   DefaultMaxRedirects,
		ConnectTimeout: 2 * time.Second,
		FetchTimeout:   2 * time.Second,
		TotalTimeout:   5 * time.Second,
		InsecureTLS:    true,
		IPAllow:        func(addr netip.Addr) bool { return true }, // loopback allowed for test server
	}

	// First fetch: 200 OK
	ctx := context.Background()
	req := application.FetchRequest{
		SourceID: "src-1",
		URL:      server.URL,
	}
	res1, err := fetcher.Fetch(ctx, req)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if res1.NotModified {
		t.Errorf("expected NotModified false on first fetch")
	}
	if res1.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", res1.StatusCode)
	}
	if res1.ETag != etag {
		t.Errorf("expected etag %s, got %s", etag, res1.ETag)
	}
	if !strings.Contains(string(res1.Body), "<rss>") {
		t.Errorf("expected rss body, got %s", string(res1.Body))
	}

	// Second fetch: conditional GET sends ETag -> receives 304 Not Modified
	req2 := application.FetchRequest{
		SourceID: "src-1",
		URL:      server.URL,
		ETag:     res1.ETag,
	}
	res2, err := fetcher.Fetch(ctx, req2)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if !res2.NotModified {
		t.Errorf("expected NotModified true on 304 response")
	}
	if res2.StatusCode != http.StatusNotModified {
		t.Errorf("expected 304, got %d", res2.StatusCode)
	}
	if len(res2.Body) != 0 {
		t.Errorf("expected empty body on 304, got %d bytes", len(res2.Body))
	}
}

func TestSafeFeedFetcher_RefusesOversizedResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write 100 KiB while max is set to 10 KiB
		_, _ = w.Write(bytes.Repeat([]byte("A"), 100*1024))
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	serverPort, _ := strconv.Atoi(u.Port())

	fetcher := &SafeFeedFetcher{
		AllowedTargets: []AllowedTarget{{Host: u.Hostname(), Port: serverPort, PathPrefix: "/"}},
		MaxBytes:       10 * 1024, // 10 KiB limit
		MaxRedirects:   DefaultMaxRedirects,
		ConnectTimeout: 2 * time.Second,
		FetchTimeout:   2 * time.Second,
		TotalTimeout:   5 * time.Second,
		InsecureTLS:    true,
		IPAllow:        func(addr netip.Addr) bool { return true },
	}

	_, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: server.URL,
	})
	if !errors.Is(err, domain.ErrFeedOversized) {
		t.Fatalf("expected ErrFeedOversized for payload > MaxBytes, got %v", err)
	}
}

func TestSafeFeedFetcher_RefusesExcessiveRedirects(t *testing.T) {
	var server *httptest.Server
	redirectCount := 0

	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		// Infinite redirect loop
		http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	serverPort, _ := strconv.Atoi(u.Port())

	fetcher := &SafeFeedFetcher{
		AllowedTargets: []AllowedTarget{{Host: u.Hostname(), Port: serverPort, PathPrefix: "/"}},
		MaxBytes:       MaxFeedBodyBytes,
		MaxRedirects:   2,
		ConnectTimeout: 2 * time.Second,
		FetchTimeout:   2 * time.Second,
		TotalTimeout:   5 * time.Second,
		InsecureTLS:    true,
		IPAllow:        func(addr netip.Addr) bool { return true },
	}

	_, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: server.URL,
	})
	if !errors.Is(err, domain.ErrRedirectDisallowed) {
		t.Fatalf("expected ErrRedirectDisallowed after 2 redirects, got %v", err)
	}
}

func TestSafeFeedFetcher_ContextCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	serverPort, _ := strconv.Atoi(u.Port())

	fetcher := &SafeFeedFetcher{
		AllowedTargets: []AllowedTarget{{Host: u.Hostname(), Port: serverPort, PathPrefix: "/"}},
		MaxBytes:       MaxFeedBodyBytes,
		MaxRedirects:   2,
		ConnectTimeout: 2 * time.Second,
		FetchTimeout:   2 * time.Second,
		TotalTimeout:   5 * time.Second,
		InsecureTLS:    true,
		IPAllow:        func(addr netip.Addr) bool { return true },
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := fetcher.Fetch(ctx, application.FetchRequest{
		URL: server.URL,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSafeFeedFetcher_SSRF_BlockedByIPPolicy(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<rss/>"))
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	serverPort, _ := strconv.Atoi(u.Port())

	// Default fetcher has PublicUnicastOnly which refuses loopback (127.0.0.1)
	fetcher := NewSafeFeedFetcher([]AllowedTarget{
		{Host: u.Hostname(), Port: serverPort, PathPrefix: "/"},
	})
	fetcher.InsecureTLS = true

	_, err := fetcher.Fetch(context.Background(), application.FetchRequest{
		URL: server.URL,
	})
	if !errors.Is(err, domain.ErrEgressForbidden) {
		t.Fatalf("expected ErrEgressForbidden for loopback address under default policy, got %v", err)
	}
}
