package source

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"time"
)

// Spec-scale defaults (ANP_INGESTION import stages 1-3).
const (
	DefaultMaxBytes     = 30 << 20
	DefaultMaxRedirects = 5
	DefaultTimeout      = 60 * time.Second
)

var (
	ErrPrivateTarget  = errors.New("source: target address not allowed")
	ErrNoAddress      = errors.New("source: no allowed address for host")
	ErrTooLarge       = errors.New("source: response exceeds size limit")
	ErrBadStatus      = errors.New("source: unexpected status")
	ErrRedirectLimit  = errors.New("source: too many redirects")
	ErrRedirectTarget = errors.New("source: redirect target rejected")
)

// deniedPrefixes is explicit instead of netip.IsPrivate so behavior never
// shifts silently under a toolchain upgrade.
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

// PublicUnicastOnly refuses everything that is not a routable public
// unicast address.
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

// Fetcher downloads allowlisted URLs with bounded resources. The zero value
// is invalid; use NewFetcher. InsecureTLS disables certificate verification
// and exists only for loopback httptest servers; production paths never set
// it, and the dial IP policy still applies.
type Fetcher struct {
	Allow        []Entry
	MaxBytes     int64
	MaxRedirects int
	Timeout      time.Duration
	TempDir      string
	IPAllow      IPPolicy
	InsecureTLS  bool
}

// NewFetcher applies spec defaults to every unset knob.
func NewFetcher(allow []Entry) Fetcher {
	return Fetcher{
		Allow:        allow,
		MaxBytes:     DefaultMaxBytes,
		MaxRedirects: DefaultMaxRedirects,
		Timeout:      DefaultTimeout,
		IPAllow:      PublicUnicastOnly,
	}
}

func (f Fetcher) policy() IPPolicy {
	if f.IPAllow != nil {
		return f.IPAllow
	}
	return PublicUnicastOnly
}

// dial validates every resolved address and dials the first allowed one,
// so the connection always goes to an inspected IP (no rebinding gap).
func (f Fetcher) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	resolver := &net.Resolver{}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("%w for %s: %v", ErrNoAddress, host, err)
	}
	allowed := false
	for _, a := range addrs {
		if f.policy()(a) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("%w: %s", ErrPrivateTarget, host)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	for _, a := range addrs {
		if !f.policy()(a) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(a.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("%w for %s", ErrNoAddress, host)
}

// tlsConfig enforces modern TLS; verification is skipped only under the
// explicit test-only flag.
func tlsConfig(insecure bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: insecure,
	}
}

func (f Fetcher) client() *http.Client {
	transport := &http.Transport{
		DialContext:         f.dial,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfig(f.InsecureTLS),
	}
	check := func(req *http.Request, via []*http.Request) error {
		if len(via) >= f.maxRedirects() {
			return ErrRedirectLimit
		}
		if _, err := Match(f.Allow, req.URL.String()); err != nil {
			return fmt.Errorf("%w: %v", ErrRedirectTarget, err)
		}
		return nil
	}
	return &http.Client{
		Timeout:       f.timeout(),
		Transport:     transport,
		CheckRedirect: check,
	}
}

func (f Fetcher) maxRedirects() int {
	if f.MaxRedirects > 0 {
		return f.MaxRedirects
	}
	return DefaultMaxRedirects
}

func (f Fetcher) timeout() time.Duration {
	if f.Timeout > 0 {
		return f.Timeout
	}
	return DefaultTimeout
}

func (f Fetcher) maxBytes() int64 {
	if f.MaxBytes > 0 {
		return f.MaxBytes
	}
	return DefaultMaxBytes
}

// Result is one stored download. Close removes the temporary file and is
// safe to call twice; every error path in Fetch also removes it.
type Result struct {
	Path         string
	SHA256       string
	Bytes        int64
	ETag         string
	LastModified string
	NotModified  bool
	closed       bool
}

// Close deletes the temporary file.
func (r *Result) Close() error {
	if r == nil || r.closed || r.Path == "" {
		return nil
	}
	r.closed = true
	return os.Remove(r.Path)
}

// Fetch downloads rawURL with optional conditional headers. A 304 returns
// NotModified with no file. Anything else than 200 fails with no file left
// behind. Cancellation aborts the stream and cleans up.
func (f Fetcher) Fetch(ctx context.Context, rawURL, etag, lastModified string) (Result, error) {
	if _, err := Match(f.Allow, rawURL); err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return Result{NotModified: true, ETag: etag, LastModified: lastModified}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("%w: %s", ErrBadStatus, resp.Status)
	}
	if resp.ContentLength > f.maxBytes() {
		return Result{}, fmt.Errorf("%w: declared %d bytes", ErrTooLarge, resp.ContentLength)
	}
	tmp, err := os.CreateTemp(f.TempDir, "anp-*.xlsx")
	if err != nil {
		return Result{}, err
	}
	tmpName := tmp.Name()
	failed := true
	defer func() {
		_ = tmp.Close()
		if failed {
			_ = os.Remove(tmpName)
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(tmp, io.TeeReader(io.LimitReader(resp.Body, f.maxBytes()+1), hash))
	if err != nil {
		return Result{}, err
	}
	if n > f.maxBytes() {
		return Result{}, fmt.Errorf("%w: streamed content", ErrTooLarge)
	}
	if err := tmp.Close(); err != nil {
		return Result{}, err
	}
	failed = false
	return Result{
		Path:         tmpName,
		SHA256:       "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		Bytes:        n,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// Probe reports whether a source changed since the known ETag,
// downloading nothing. Any allowlisted URL probes; the allowlist itself is
// the guard. Schedulers retain the last known revision on any error instead
// of treating failure as change.
func (f Fetcher) Probe(ctx context.Context, rawURL, knownETag string) (changed bool, etag, lastModified string, err error) {
	if _, err := Match(f.Allow, rawURL); err != nil {
		return false, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, "", "", err
	}
	if knownETag != "" {
		req.Header.Set("If-None-Match", knownETag)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return false, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return false, knownETag, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", "", fmt.Errorf("%w: %s", ErrBadStatus, resp.Status)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, f.maxBytes()+1)); err != nil {
		return false, "", "", err
	}
	return true, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), nil
}
