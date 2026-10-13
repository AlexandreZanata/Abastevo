package registry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// SourceAPI is the frozen source key for ANP registry API discovery.
const SourceAPI = "registry-api"

// APIParserVersion freezes the discovery parser identity on every run.
const APIParserVersion = "registry-api-v2"

// ProductionAPIHost is the only host P25-T03 may fetch in production.
const ProductionAPIHost = "revendedoresapi.anp.gov.br"

// APIConfig bounds one discovery attempt. Production uses
// ProductionAPIConfig; tests inject loopback overrides explicitly.
type APIConfig struct {
	BaseURL       string
	AllowedHost   string
	AllowLoopback bool
	CNPJ          string
	UF            string
	Municipality  string
	PageSize      int // Retained config field; ANP controls actual page size.
	MaxPages      int
	MaxRequests   int
	MaxBytes      int64
	Timeout       time.Duration
	Sleep         func()
}

// ProductionAPIConfig returns the frozen production discovery bounds.
func ProductionAPIConfig() APIConfig {
	return APIConfig{
		BaseURL:     "https://" + ProductionAPIHost,
		AllowedHost: ProductionAPIHost,
		PageSize:    100,
		MaxPages:    50,
		MaxRequests: 1000,
		MaxBytes:    16 << 20,
		Timeout:     30 * time.Second,
		Sleep:       func() { time.Sleep(200 * time.Millisecond) },
	}
}

func (c APIConfig) withDefaults() APIConfig {
	if c.PageSize <= 0 {
		c.PageSize = 100
	}
	if c.MaxPages <= 0 {
		c.MaxPages = 50
	}
	if c.MaxRequests <= 0 {
		c.MaxRequests = 1000
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = 1 << 20
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Sleep == nil {
		c.Sleep = func() {}
	}
	return c
}

// apiClient builds the guarded HTTP client: exact-host allowlist,
// HTTPS-only (HTTP only for loopback tests), inspected dial IPs (no
// private/loopback/link-local bypass, no rebinding gap), bounded
// redirects re-validated against the allowlist.
func apiClient(cfg APIConfig) (*http.Client, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil || base.Hostname() == "" {
		return nil, fmt.Errorf("registry: bad base url: %w", err)
	}
	if base.Hostname() != cfg.AllowedHost {
		return nil, errors.New("registry: base host not allowlisted")
	}
	if base.Scheme == "http" && !cfg.AllowLoopback {
		return nil, errors.New("registry: plain HTTP outside loopback tests")
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return nil, errors.New("registry: unsupported scheme")
	}
	allowLoopback := cfg.AllowLoopback
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if host != cfg.AllowedHost {
				return nil, errors.New("registry: dial host not allowlisted")
			}
			addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addrs) == 0 {
				return nil, errors.New("registry: no address for allowlisted host")
			}
			for _, addr := range addrs {
				if apiIPAllowed(addr, allowLoopback) {
					dialer := &net.Dialer{Timeout: cfg.Timeout}
					return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
				}
			}
			return nil, errors.New("registry: no allowed address for host")
		},
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout + 10*time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("registry: too many redirects")
			}
			return errors.New("registry: redirects are not an official API contract")
		},
	}, nil
}

// apiIPAllowed mirrors the platform source policy explicitly so the
// registry adapter never depends on another module's adapter: public
// unicast only, loopback solely for tests.
func apiIPAllowed(addr netip.Addr, allowLoopback bool) bool {
	if allowLoopback && addr.IsLoopback() {
		return true
	}
	return addr.IsGlobalUnicast() && !addr.IsPrivate()
}

type apiPage struct {
	Items      []json.RawMessage
	NextCursor *string
	Official   bool
	Total      int
	Pages      int
}

// Discover runs one bounded API discovery into staging: targeted CNPJ
// lookup or scoped page traversal with global request/page quotas.
// Partial traversal never completes: quota exhaustion quarantines the
// run instead of masquerading as a full snapshot. Unsupported
// identifiers fail before any network call.
func Discover(ctx context.Context, store Store, snapshot string, cfg APIConfig) (Report, error) {
	cfg = cfg.withDefaults()
	// The earlier synthetic contract could persist a false empty completion.
	// Version the immutable scope key so that evidence cannot replay under v2.
	snapshot = APIParserVersion + ":" + snapshot
	if cfg.CNPJ != "" {
		normalized, err := ValidCNPJText(cfg.CNPJ)
		if err != nil {
			return Report{}, fmt.Errorf("registry: unsupported CNPJ: %w", err)
		}
		cfg.CNPJ = normalized
	}
	client, err := apiClient(cfg)
	if err != nil {
		return Report{}, err
	}
	runID := newUUID()
	if _, created, err := store.CreateRun(ctx, runID, SourceAPI, snapshot, "api:"+snapshot); err != nil {
		return Report{}, err
	} else if !created {
		return store.GetRun(ctx, SourceAPI, snapshot)
	}
	report := Report{RunID: runID, State: "running"}
	finish := func(state, code string) (Report, error) {
		report.State = state
		report.ErrorCode = code
		if err := store.FinishRun(ctx, runID, state, report.Accepted, report.Duplicates, report.Rejected, code); err != nil {
			return Report{}, err
		}
		return report, nil
	}
	requests := 0
	get := func(target string) ([]byte, int, error) {
		if requests >= cfg.MaxRequests {
			return nil, 0, errQuota
		}
		var body []byte
		var status int
		for attempt := 0; attempt < 3; attempt++ {
			if requests >= cfg.MaxRequests {
				return nil, 0, errQuota
			}
			requests++
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				return nil, 0, err
			}
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				return nil, 0, err
			}
			body, err = io.ReadAll(io.LimitReader(resp.Body, cfg.MaxBytes+1))
			_ = resp.Body.Close()
			if err != nil {
				return nil, 0, err
			}
			if int64(len(body)) > cfg.MaxBytes {
				return nil, 0, errors.New("registry: response body exceeds cap")
			}
			status = resp.StatusCode
			if status == http.StatusTooManyRequests || (status >= 500 && status < 600) {
				cfg.Sleep()
				continue
			}
			return body, status, nil
		}
		return nil, status, errRetryBudget
	}
	cursor := ""
	pages := 0
	seenRows, expectedTotal, expectedPages := 0, -1, -1
	for {
		if pages >= cfg.MaxPages {
			return finish("quarantined", "page_quota")
		}
		target, err := pageURL(cfg, cursor)
		if err != nil {
			return finish("failed", "bad_target")
		}
		body, status, err := get(target)
		if err != nil {
			if errors.Is(err, errQuota) {
				return finish("quarantined", "request_quota")
			}
			if errors.Is(err, errRetryBudget) {
				return finish("failed", "provider_error")
			}
			return finish("failed", "transport")
		}
		if status != http.StatusOK {
			return finish("failed", "provider_error")
		}
		page, err := decodeAPIPage(body, pages+1)
		if err != nil {
			return finish("failed", "provider_schema")
		}
		if page.Official {
			if expectedTotal < 0 {
				expectedTotal, expectedPages = page.Total, page.Pages
			}
			if page.Total != expectedTotal || page.Pages != expectedPages {
				return finish("quarantined", "snapshot_changed")
			}
			seenRows += len(page.Items)
			if seenRows > expectedTotal {
				return finish("quarantined", "row_count")
			}
		}
		for _, item := range page.Items {
			assertion, ok := parseOfficialAPIRecord(item)
			if !ok {
				report.Rejected++
				continue
			}
			if cfg.CNPJ != "" {
				wanted, _ := ValidCNPJText(cfg.CNPJ)
				if assertion.SourceKey != wanted {
					return finish("quarantined", "scope_mismatch")
				}
			}
			if cfg.UF != "" && assertion.State != strings.ToUpper(cfg.UF) {
				return finish("quarantined", "scope_mismatch")
			}
			if cfg.Municipality != "" && !strings.EqualFold(strings.TrimSpace(assertion.Address["municipio"]), strings.TrimSpace(cfg.Municipality)) {
				return finish("quarantined", "scope_mismatch")
			}
			assertion.runID = runID
			accepted, err := store.StageAssertion(ctx, assertion)
			if err != nil {
				return finish("failed", "stage_error")
			}
			if accepted {
				report.Accepted++
			} else {
				report.Duplicates++
			}
		}
		pages++
		if page.NextCursor == nil || *page.NextCursor == "" {
			if page.Official && seenRows != expectedTotal {
				return finish("quarantined", "row_count")
			}
			break
		}
		cursor = *page.NextCursor
	}
	return finish("complete", "")
}

var (
	errQuota       = errors.New("registry: request quota exhausted")
	errRetryBudget = errors.New("registry: retry budget exhausted")
)

// pageURL builds one bounded query: targeted CNPJ lookup or scoped
// traversal. No caller-provided URLs ever reach the fetcher.
func pageURL(cfg APIConfig, cursor string) (string, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	q := base.Query()
	if cfg.CNPJ != "" {
		q.Set("cnpj", cfg.CNPJ)
	} else {
		if cfg.UF != "" {
			q.Set("uf", cfg.UF)
		}
		if cfg.Municipality != "" {
			q.Set("municipio", cfg.Municipality)
		}
	}
	// The official API exposes a fixed route and numbered pages.
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/combustivel"
	page := cursor
	if page == "" {
		page = "1"
	}
	q.Set("numeropagina", page)
	base.RawQuery = q.Encode()
	return base.String(), nil
}
