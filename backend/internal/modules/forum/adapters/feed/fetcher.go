package feed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

const (
	DefaultConnectTimeout = 5 * time.Second
	DefaultTLSFetchTimeout = 10 * time.Second
	DefaultTotalTimeout   = 30 * time.Second
	DefaultMaxRedirects   = 2
	UserAgent             = "AbastevoBot/1.0 (+https://abastevo.com.br)"
)

// deniedPrefixes blocks loopback, private, carrier-grade NAT, link-local, multicast,
// documentation, cloud metadata (169.254.169.254), IPv4-mapped IPv6, and ULA ranges.
var deniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("::ffff:0:0/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// IPPolicy decides whether a resolved address may be dialed.
type IPPolicy func(netip.Addr) bool

// PublicUnicastOnly refuses any address that is not a routable public unicast address.
func PublicUnicastOnly(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, p := range deniedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// AllowedTarget specifies an authorized origin and optional path prefix for feed egress.
type AllowedTarget struct {
	Host       string
	Port       int // Optional port; defaults to 443 if 0
	PathPrefix string
}

// SafeFeedFetcher implements application.SourceFetcher with strict SSRF and resource bounds.
type SafeFeedFetcher struct {
	AllowedTargets []AllowedTarget
	MaxBytes       int64
	MaxRedirects   int
	ConnectTimeout time.Duration
	FetchTimeout   time.Duration
	TotalTimeout   time.Duration
	IPAllow        IPPolicy
	InsecureTLS    bool // Test-only flag for loopback TLS servers
}

// NewSafeFeedFetcher constructs a SafeFeedFetcher with spec-mandated security defaults.
func NewSafeFeedFetcher(targets []AllowedTarget) *SafeFeedFetcher {
	return &SafeFeedFetcher{
		AllowedTargets: targets,
		MaxBytes:       MaxFeedBodyBytes,
		MaxRedirects:   DefaultMaxRedirects,
		ConnectTimeout: DefaultConnectTimeout,
		FetchTimeout:   DefaultTLSFetchTimeout,
		TotalTimeout:   DefaultTotalTimeout,
		IPAllow:        PublicUnicastOnly,
	}
}

func (f *SafeFeedFetcher) policy() IPPolicy {
	if f.IPAllow != nil {
		return f.IPAllow
	}
	return PublicUnicastOnly
}

func (f *SafeFeedFetcher) validateTarget(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse url: %v", domain.ErrEgressForbidden, err)
	}

	// 1. Strict HTTPS-only
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q not allowed (https only)", domain.ErrEgressForbidden, u.Scheme)
	}

	// 2. Reject credentials in URL
	if u.User != nil {
		return nil, fmt.Errorf("%w: user credentials in url forbidden", domain.ErrEgressForbidden)
	}

	// 3. Port parsing: defaults to 443
	actualPort := 443
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil || p <= 0 || p > 65535 {
			return nil, fmt.Errorf("%w: invalid port %q", domain.ErrEgressForbidden, u.Port())
		}
		actualPort = p
	}

	// 4. Normalize path (strip dot segments)
	cleanPath := path.Clean(u.Path)
	if cleanPath == "." {
		cleanPath = "/"
	}

	// 5. Match against Allowlist
	host := strings.ToLower(u.Hostname())
	matched := false
	for _, target := range f.AllowedTargets {
		expectedPort := target.Port
		if expectedPort == 0 {
			expectedPort = 443
		}
		if actualPort != expectedPort {
			continue
		}
		if strings.EqualFold(target.Host, host) {
			if target.PathPrefix == "" || strings.HasPrefix(cleanPath, target.PathPrefix) {
				matched = true
				break
			}
		}
	}
	if !matched {
		return nil, fmt.Errorf("%w: host %q port %d path %q not in allowlist", domain.ErrEgressForbidden, host, actualPort, cleanPath)
	}

	return u, nil
}

func (f *SafeFeedFetcher) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: split host port: %v", domain.ErrEgressForbidden, err)
	}

	// Resolve IP addresses
	resolver := &net.Resolver{}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("%w: resolve %s: %v", domain.ErrEgressForbidden, host, err)
	}

	// Validate against IP policy and select the first allowed IP
	var selectedIP netip.Addr
	policy := f.policy()
	for _, a := range addrs {
		if policy(a) {
			selectedIP = a
			break
		}
	}

	if !selectedIP.IsValid() {
		return nil, fmt.Errorf("%w: resolved addresses for %s rejected by egress policy", domain.ErrEgressForbidden, host)
	}

	// Pin the connection to the validated IP to prevent DNS rebinding TOCTOU
	connectTimeout := f.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = DefaultConnectTimeout
	}
	dialer := &net.Dialer{Timeout: connectTimeout}

	targetAddr := net.JoinHostPort(selectedIP.String(), port)
	conn, err := dialer.DialContext(ctx, network, targetAddr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Fetch sends a conditional GET request for the authorized source feed.
func (f *SafeFeedFetcher) Fetch(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
	targetURL, err := f.validateTarget(req.URL)
	if err != nil {
		return application.FetchResponse{}, err
	}

	totalTimeout := f.TotalTimeout
	if totalTimeout <= 0 {
		totalTimeout = DefaultTotalTimeout
	}
	fetchCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	transport := &http.Transport{
		DialContext:         f.dialContext,
		TLSHandshakeTimeout: f.ConnectTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ServerName:         targetURL.Hostname(),
			InsecureSkipVerify: f.InsecureTLS,
		},
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(redirectReq *http.Request, via []*http.Request) error {
			if len(via) >= f.MaxRedirects {
				return domain.ErrRedirectDisallowed
			}
			// Verify redirect target adheres to exact scheme, port and allowlist
			if _, err := f.validateTarget(redirectReq.URL.String()); err != nil {
				return fmt.Errorf("%w: redirect target refused: %v", domain.ErrRedirectDisallowed, err)
			}
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return application.FetchResponse{}, fmt.Errorf("create http request: %w", err)
	}

	httpReq.Header.Set("User-Agent", UserAgent)
	httpReq.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")
	if req.ETag != "" {
		httpReq.Header.Set("If-None-Match", req.ETag)
	}
	if req.LastModified != "" {
		httpReq.Header.Set("If-Modified-Since", req.LastModified)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(fetchCtx.Err(), context.Canceled) {
			return application.FetchResponse{}, context.Canceled
		}
		if errors.Is(fetchCtx.Err(), context.DeadlineExceeded) {
			return application.FetchResponse{}, context.DeadlineExceeded
		}
		return application.FetchResponse{}, err
	}
	defer resp.Body.Close()

	etag := resp.Header.Get("ETag")
	lastMod := resp.Header.Get("Last-Modified")

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		return application.FetchResponse{
			NotModified:  true,
			StatusCode:   http.StatusNotModified,
			ETag:         etag,
			LastModified: lastMod,
			Body:         nil,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return application.FetchResponse{
			NotModified:  false,
			StatusCode:   resp.StatusCode,
			ETag:         etag,
			LastModified: lastMod,
			Body:         nil,
		}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	// Bounded body reader: refuse oversized payload (> MaxBytes)
	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxFeedBodyBytes
	}
	limitReader := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limitReader)
	if err != nil {
		return application.FetchResponse{}, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return application.FetchResponse{}, domain.ErrFeedOversized
	}

	return application.FetchResponse{
		NotModified:  false,
		StatusCode:   http.StatusOK,
		ETag:         etag,
		LastModified: lastMod,
		Body:         body,
	}, nil
}
