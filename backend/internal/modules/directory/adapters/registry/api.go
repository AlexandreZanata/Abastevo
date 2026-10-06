package registry

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// SourceAPI is the frozen source key for ANP registry API discovery.
const SourceAPI = "registry-api"

// APIParserVersion freezes the discovery parser identity on every run.
const APIParserVersion = "registry-api-v1"

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
	PageSize      int
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
		MaxBytes:    1 << 20,
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
			if req.URL.Hostname() != cfg.AllowedHost {
				return errors.New("registry: redirect target rejected")
			}
			return nil
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

// apiRecord is one decoded API item before policy validation.
type apiRecord struct {
	CNPJ         string `json:"cnpj"`
	RazaoSocial  string `json:"razaoSocial"`
	NomeFantasia string `json:"nomeFantasia"`
	Situacao     string `json:"situacao"`
	LocationWire string `json:"location_quality"`
	Autorizacao  *struct {
		Ato         string `json:"ato"`
		PublicadoEm string `json:"publicadoEm"`
	} `json:"autorizacao"`
	Endereco *struct {
		Municipio  string `json:"municipio"`
		UF         string `json:"uf"`
		CodigoIbge string `json:"codigoIbge"`
	} `json:"endereco"`
	Coordenadas *struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
		CRS string  `json:"crs"`
	} `json:"coordenadas"`
}

type apiPage struct {
	Items      []json.RawMessage `json:"items"`
	NextCursor *string           `json:"nextCursor"`
}

// parseAPIRecord validates one API item with the frozen T01 policy.
// Unknown statuses stay honest (pending); malformed identity,
// out-of-range coordinates and unsupported CRS reject the row.
func parseAPIRecord(raw json.RawMessage) (Assertion, bool) {
	var rec apiRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Assertion{}, false
	}
	cnpj, err := ValidCNPJText(rec.CNPJ)
	if err != nil {
		return Assertion{}, false
	}
	name := strings.TrimSpace(rec.RazaoSocial)
	if name == "" {
		name = strings.TrimSpace(rec.NomeFantasia)
	}
	if name == "" {
		return Assertion{}, false
	}
	ibge, uf := "", ""
	if rec.Endereco != nil {
		ibge = strings.TrimSpace(rec.Endereco.CodigoIbge)
		uf = strings.ToUpper(strings.TrimSpace(rec.Endereco.UF))
	}
	if ibge == "" || len(uf) != 2 {
		return Assertion{}, false
	}
	auth := MapStatus(rec.Situacao)
	eligibility := DecideEligibility(Record{
		IdentityOK: true, AddressOK: true,
		Authorized: auth == AuthorizationAuthorized,
	})
	quality := "unknown"
	if rec.LocationWire == "reviewed" || rec.LocationWire == "city-centroid" {
		quality = rec.LocationWire
	}
	var lat, lon float64
	var hasCoords bool
	var crs string
	if rec.Coordenadas != nil {
		coords := rec.Coordenadas
		if math.IsNaN(coords.Lat) || math.IsNaN(coords.Lon) ||
			math.IsInf(coords.Lat, 0) || math.IsInf(coords.Lon, 0) ||
			coords.Lat < -90 || coords.Lat > 90 ||
			coords.Lon < -180 || coords.Lon > 180 {
			return Assertion{}, false
		}
		crs = strings.ToUpper(strings.TrimSpace(coords.CRS))
		if crs != "WGS84" && crs != "SIRGAS2000" {
			return Assertion{}, false
		}
		lat, lon, hasCoords = coords.Lat, coords.Lon, true
	} else if quality != "unknown" {
		// A reviewed claim without coordinates is honest unknown, never
		// an inferred point.
		quality = "unknown"
	}
	// Coordinates never upgrade an unknown wire: review owns upgrades.
	sum := sha256.Sum256(raw)
	ato := ""
	var effective pgtype.Date
	if rec.Autorizacao != nil {
		ato = strings.TrimSpace(rec.Autorizacao.Ato)
		if rec.Autorizacao.PublicadoEm != "" {
			if parsed, err := time.Parse("2006-01-02", rec.Autorizacao.PublicadoEm); err == nil {
				effective = pgtype.Date{Time: parsed, Valid: true}
			}
		}
	}
	return Assertion{
		Source:           SourceAPI,
		SourceKey:        cnpj,
		Checksum:         hex.EncodeToString(sum[:]),
		DisplayName:      name,
		Address:          map[string]string{"municipio_ibge": ibge, "uf": uf},
		MunicipalityCode: ibge,
		State:            uf,
		AuthState:        string(auth),
		Eligibility:      string(eligibility),
		LocationQuality:  quality,
		SourceReference:  ato,
		EffectiveDate:    effective,
		Latitude:         lat,
		Longitude:        lon,
		HasCoords:        hasCoords,
		CRS:              crs,
	}, true
}

// Discover runs one bounded API discovery into staging: targeted CNPJ
// lookup or scoped page traversal with global request/page quotas.
// Partial traversal never completes: quota exhaustion quarantines the
// run instead of masquerading as a full snapshot. Unsupported
// identifiers fail before any network call.
func Discover(ctx context.Context, store Store, snapshot string, cfg APIConfig) (Report, error) {
	cfg = cfg.withDefaults()
	if cfg.CNPJ != "" {
		if _, err := ValidCNPJText(cfg.CNPJ); err != nil {
			return Report{}, fmt.Errorf("registry: unsupported CNPJ: %w", err)
		}
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
		requests++
		var body []byte
		var status int
		for attempt := 0; attempt < 3; attempt++ {
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
		if status == http.StatusNotFound && cfg.CNPJ != "" {
			return finish("complete", "")
		}
		if status != http.StatusOK {
			return finish("failed", "provider_error")
		}
		var page apiPage
		if err := json.Unmarshal(body, &page); err != nil {
			return finish("failed", "provider_error")
		}
		for _, item := range page.Items {
			assertion, ok := parseAPIRecord(item)
			if !ok {
				report.Rejected++
				continue
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
		if cfg.CNPJ != "" || page.NextCursor == nil || *page.NextCursor == "" {
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
		q.Set("pageSize", fmt.Sprintf("%d", cfg.PageSize))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
	}
	base.RawQuery = q.Encode()
	return base.String(), nil
}
