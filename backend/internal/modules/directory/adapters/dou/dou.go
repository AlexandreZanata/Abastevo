// Package dou implements the P26 bounded INLABS edition adapter:
// operator-owned credentials (never Git/logs), allowlisted HTTPS fetch,
// entity/DTD-safe edition parsing with byte caps, and checksum
// checkpoints so duplicate editions converge. Live INLABS access is
// unavailable in this environment: absent credentials fail explicitly
// (ErrNoAccess) and no live fetch is claimed. Fixture schemas are
// provisional — reconfirm against the live INLABS format at the owning
// task's opening before trusting new fields.
package dou

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ProductionHost is the only host P26 may fetch in production.
const ProductionHost = "inlabs.in.gov.br"

// MaxEditionBytes bounds one edition document (provisional P25-T01
// budget, flagged [CALIBRATE]).
const MaxEditionBytes = 8 << 20

var (
	ErrNoAccess       = errors.New("dou: source credentials not configured")
	ErrNotAllowlisted = errors.New("dou: host not allowlisted")
	ErrUnsafeXML      = errors.New("dou: unsafe XML construct refused")
	ErrOversize       = errors.New("dou: edition exceeds byte cap")
	ErrMalformed      = errors.New("dou: malformed edition")
)

// Config carries operator-owned access. Credentials travel in memory
// only: no logging, no job payloads, no fixtures.
type Config struct {
	BaseURL       string
	Username      string
	Password      string
	Timeout       time.Duration
	AllowLoopback bool
	HTTPClient    *http.Client
}

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	return c
}

// Act is one parsed edition act with its certified-edition reference.
type Act struct {
	ID    string
	Kind  string
	Title string
	Text  string
}

// Edition is one validated edition with checksum identity.
type Edition struct {
	Date     string
	Number   string
	Acts     []Act
	Checksum string
}

type editionXML struct {
	Date   string   `xml:"data,attr"`
	Number string   `xml:"numero,attr"`
	Acts   []actXML `xml:"ato"`
}

type actXML struct {
	ID    string `xml:"id,attr"`
	Kind  string `xml:"tipo,attr"`
	Title string `xml:"titulo"`
	Text  string `xml:"texto"`
}

// ParseEdition validates and parses one edition document. Entity/DOCTYPE
// constructs are refused before decoding (defense in depth: Go's decoder
// never fetches external entities, but hostile input fails loudly
// here); oversized input fails before allocation.
func ParseEdition(raw []byte, wantDate string) (Edition, error) {
	if len(raw) > MaxEditionBytes {
		return Edition{}, ErrOversize
	}
	upper := strings.ToUpper(string(raw))
	if strings.Contains(upper, "<!DOCTYPE") || strings.Contains(upper, "<!ENTITY") {
		return Edition{}, ErrUnsafeXML
	}
	var doc editionXML
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return Edition{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if strings.TrimSpace(doc.Date) == "" {
		return Edition{}, fmt.Errorf("%w: missing edition date", ErrMalformed)
	}
	if wantDate != "" && doc.Date != wantDate {
		return Edition{}, fmt.Errorf("%w: edition date %q, want %q", ErrMalformed, doc.Date, wantDate)
	}
	edition := Edition{Date: doc.Date, Number: strings.TrimSpace(doc.Number)}
	for _, a := range doc.Acts {
		if strings.TrimSpace(a.ID) == "" {
			return Edition{}, fmt.Errorf("%w: act without id", ErrMalformed)
		}
		edition.Acts = append(edition.Acts, Act{
			ID: strings.TrimSpace(a.ID), Kind: strings.ToLower(strings.TrimSpace(a.Kind)),
			Title: strings.TrimSpace(a.Title), Text: strings.TrimSpace(a.Text),
		})
	}
	sum := sha256.Sum256(raw)
	edition.Checksum = hex.EncodeToString(sum[:])
	return edition, nil
}

// Checkpoints converge duplicate editions by checksum.
type Checkpoints interface {
	Seen(checksum string) (bool, error)
	Mark(checksum string) error
}

// MemoryCheckpoints is an in-process checkpoint store for tests and
// single-shot operator runs; durable checkpoints arrive with the
// scheduled worker wiring.
type MemoryCheckpoints struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewMemoryCheckpoints() *MemoryCheckpoints {
	return &MemoryCheckpoints{seen: map[string]bool{}}
}

func (m *MemoryCheckpoints) Seen(checksum string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seen[checksum], nil
}

func (m *MemoryCheckpoints) Mark(checksum string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[checksum] = true
	return nil
}

// FetchEdition downloads one edition by ISO date over guarded HTTPS.
// Absent credentials fail before any network; the host must be the
// allowlisted production host (loopback solely for tests); dial IPs
// are inspected like the registry transport (no rebinding gap).
func FetchEdition(ctx context.Context, cfg Config, date string) (Edition, error) {
	cfg = cfg.withDefaults()
	if strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "" {
		return Edition{}, ErrNoAccess
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil || base.Hostname() == "" {
		return Edition{}, fmt.Errorf("%w: bad base", ErrNotAllowlisted)
	}
	allowed := base.Hostname() == ProductionHost
	if cfg.AllowLoopback && (base.Hostname() == "127.0.0.1" || base.Hostname() == "localhost") {
		allowed = true
	}
	if !allowed {
		return Edition{}, ErrNotAllowlisted
	}
	if base.Scheme == "http" && !cfg.AllowLoopback {
		return Edition{}, ErrNotAllowlisted
	}
	target := strings.TrimSuffix(cfg.BaseURL, "/") + "/edicao/" + date + ".xml"
	client := cfg.HTTPClient
	if client == nil {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
				if err != nil || len(addrs) == 0 {
					return nil, errors.New("dou: no address for host")
				}
				for _, addr := range addrs {
					if douIPAllowed(addr, cfg.AllowLoopback) {
						dialer := &net.Dialer{Timeout: cfg.Timeout}
						return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
					}
				}
				return nil, errors.New("dou: no allowed address for host")
			},
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}
		client = &http.Client{Transport: transport, Timeout: cfg.Timeout + 10*time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Edition{}, err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Accept", "application/xml")
	resp, err := client.Do(req)
	if err != nil {
		return Edition{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Edition{}, fmt.Errorf("dou: edition fetch HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxEditionBytes+1))
	if err != nil {
		return Edition{}, err
	}
	if int64(len(raw)) > MaxEditionBytes {
		return Edition{}, ErrOversize
	}
	if !bytes.Contains(raw, []byte("<edicao")) {
		return Edition{}, fmt.Errorf("%w: missing edition root", ErrMalformed)
	}
	return ParseEdition(raw, date)
}

func douIPAllowed(addr netip.Addr, allowLoopback bool) bool {
	if allowLoopback && addr.IsLoopback() {
		return true
	}
	return addr.IsGlobalUnicast() && !addr.IsPrivate()
}
