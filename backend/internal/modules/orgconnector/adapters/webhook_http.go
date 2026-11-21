package adapters

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/netpolicy"
)

var webhookHost = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var webhookPath = regexp.MustCompile(`^/(?:[A-Za-z0-9_.~-]+/)*(?:[A-Za-z0-9_.~-]+)?$`)
var webhookIPv6 = netip.MustParsePrefix("2000::/3")
var webhook6to4 = netip.MustParsePrefix("2002::/16")

func validWebhookEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > 1024 || u.Scheme != "https" || u.User != nil || len(u.Host) > 253 || !webhookHost.MatchString(u.Host) ||
		u.RawPath != "" || !webhookPath.MatchString(u.Path) || endpoint != "https://"+u.Host+u.Path {
		return false
	}
	for segment := range strings.SplitSeq(u.Path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

var errWebhookDestination = errors.New("fleet webhook: destination unavailable")

// Pin each TLS connection to a vetted address from the reviewed hostname.
// The HTTP client keeps the reviewed DNS name for SNI and verification.
func dialWebhook(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" || !webhookHost.MatchString(host) {
		return nil, errWebhookDestination
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host+".")
	if err != nil {
		return nil, errWebhookDestination
	}
	dialer := net.Dialer{}
	for _, addr := range addresses {
		if !addr.IsGlobalUnicast() || !netpolicy.PublicUnicastOnly(addr) ||
			addr.Is6() && (!webhookIPv6.Contains(addr) || webhook6to4.Contains(addr)) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, errWebhookDestination
}

type HTTPSWebhookSender struct {
	keys   *os.Root
	client *http.Client
	now    func() time.Time
}

func NewHTTPSWebhookSender(keys *os.Root) HTTPSWebhookSender {
	return HTTPSWebhookSender{keys: keys, now: time.Now, client: &http.Client{
		Transport: &http.Transport{DialContext: dialWebhook, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10,
			DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       4 * time.Second,
	}}
}

func (s HTTPSWebhookSender) SendWebhook(ctx context.Context, endpoint string, ref domain.CredentialRef, id string, body []byte) WebhookResult {
	if s.keys == nil || s.client == nil || !validWebhookEndpoint(endpoint) || ref.Reference() == "" {
		return WebhookSuppressed
	}
	key, err := readConnectorKey(s.keys, ref.Reference())
	if err != nil {
		return WebhookRetry
	}
	defer clear(key[:])
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return WebhookSuppressed
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", id)
	req.Header.Set("Abastevo-Timestamp", timestamp)
	req.Header.Set("Abastevo-Signature", "v2="+hex.EncodeToString(mac.Sum(nil)))
	response, err := s.client.Do(req)
	if err != nil {
		return WebhookRetry
	}
	defer response.Body.Close()
	switch status := response.StatusCode; {
	case status >= 200 && status <= 299:
		return WebhookDelivered
	case status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500:
		return WebhookRetry
	default:
		return WebhookFailed
	}
}

func readConnectorKey(root *os.Root, name string) ([32]byte, error) {
	var key [32]byte
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(key)) {
		return key, errWebhookDestination
	}
	file, err := root.Open(name)
	if err != nil {
		return key, errWebhookDestination
	}
	defer file.Close()
	if _, err := io.ReadFull(file, key[:]); err != nil {
		clear(key[:])
		return key, errWebhookDestination
	}
	return key, nil
}
