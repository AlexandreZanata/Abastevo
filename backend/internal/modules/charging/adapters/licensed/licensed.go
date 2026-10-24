package licensed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Closed vocabularies. Unknown provider states never upgrade to AVAILABLE.
const (
	StateAccessPending = "ACCESS_PENDING"
	StateAuthorized    = "AUTHORIZED"

	ReasonNoCredential  = "no-credential"
	ReasonGrantExpired  = "grant-expired"
	ReasonRouteUnpinned = "route-unpinned"

	StatusAvailable   = "AVAILABLE"
	StatusUnavailable = "UNAVAILABLE"
	StatusUnknown     = "UNKNOWN"
)

// Provider fault taxonomy. Every fault refuses; none yields data.
const (
	KindWrongScope     = "wrong-scope"
	KindReplayRefused  = "replay-refused"
	KindStaleTimestamp = "stale-timestamp"
	KindProviderOutage = "provider-outage"
	KindQuotaExceeded  = "quota-exceeded"
	KindRevoked        = "revoked"
	KindInvalidState   = "invalid-state"
)

type Module string

const (
	ModuleStatus  Module = "STATUS"
	ModuleTariffs Module = "TARIFFS"
)

// Config pins one licensed operator. Reads stay disabled until a
// credential exists, the grant is current and a reviewed route is
// attached; pinning alone never dials anyone.
type Config struct {
	ProviderID     string
	Protocol       string
	Modules        []Module
	BaseURL        string
	CredentialEnv  string
	Scope          string
	GrantExpiresAt time.Time
}

// ConfigError names the refused field. It never carries secrets.
type ConfigError struct{ Field string }

func (e *ConfigError) Error() string { return "licensed: invalid " + e.Field }

// AccessError reports a closed gate: no data, only the state/reason.
type AccessError struct {
	State  string
	Reason string
}

func (e *AccessError) Error() string { return "licensed: " + e.State + " (" + e.Reason + ")" }

// ProviderError reports a typed provider fault. It never carries data.
type ProviderError struct{ Kind string }

func (e *ProviderError) Error() string { return "licensed: provider " + e.Kind }

// Access is the resolved gate for one call moment.
type Access struct {
	State  string
	Reason string
}

// StatusResult is one typed location state. UNKNOWN carries its reason
// and is never treated as AVAILABLE.
type StatusResult struct {
	State      string
	ObservedAt time.Time
	Reason     string
}

// TariffResult is one operator tariff reference. Provenance is always
// LICENSED_OPERATOR (set here, never trusted from the wire); pricing
// stays with the EE-11 engine, never with this port.
type TariffResult struct {
	ID         string
	Revision   string
	ValidFrom  time.Time
	ValidUntil *time.Time
	Provenance string
}

// Request is the port call shape. The credential value never enters it:
// only its presence is signalled, so transports cannot leak secrets.
type Request struct {
	Operation     string
	LocationID    string
	Scope         string
	Nonce         string
	Timestamp     time.Time
	HasCredential bool
}

// Response is the transport answer shape. Data and faults are exclusive:
// a faulted response carries no state or tariffs.
type Response struct {
	Status     string
	ObservedAt time.Time
	Reason     string
	Tariffs    []TariffResult
	ErrorKind  string
}

// Transport executes one pinned call. Production wiring needs authorized
// access review; tests and future reviewed bindings supply it.
type Transport interface {
	RoundTrip(ctx context.Context, req Request) (Response, error)
}

// Client is the licensed read port. The zero transport (New) refuses
// every read with route-unpinned, even when authorized.
type Client struct {
	cfg       Config
	now       func() time.Time
	lookup    func(string) (string, bool)
	transport Transport
}

var (
	envPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	providerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

// Validate admits only a pinned read-only operator binding. COMMANDS,
// session or billing modules, non-https endpoints and credential values
// in config are refused here, never at call time.
func Validate(c Config) error {
	if !providerPattern.MatchString(c.ProviderID) {
		return &ConfigError{Field: "provider_id"}
	}
	if c.Protocol != "OCPI_2_2_1" && c.Protocol != "DOCUMENTED_API" {
		return &ConfigError{Field: "protocol"}
	}
	if len(c.Modules) < 1 || len(c.Modules) > 2 {
		return &ConfigError{Field: "modules"}
	}
	seen := map[Module]bool{}
	for _, m := range c.Modules {
		if (m != ModuleStatus && m != ModuleTariffs) || seen[m] {
			return &ConfigError{Field: "modules"}
		}
		seen[m] = true
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Host == "" {
		return &ConfigError{Field: "base_url"}
	}
	if !envPattern.MatchString(c.CredentialEnv) {
		return &ConfigError{Field: "credential_env"}
	}
	if c.Scope == "" || len(c.Scope) > 128 || strings.ContainsAny(c.Scope, " \t\n\r") {
		return &ConfigError{Field: "scope"}
	}
	if c.GrantExpiresAt.IsZero() {
		return &ConfigError{Field: "grant_expires_at"}
	}
	return nil
}

// New validates and returns a routeless client: reads fail closed until
// a reviewed transport is attached.
func New(c Config) (*Client, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	return &Client{cfg: c}, nil
}

// Resolve reports the gate for this moment. Missing credentials and
// expired grants stay ACCESS_PENDING; nothing is dialed to decide.
func (cl *Client) Resolve() Access {
	lookup := cl.lookup
	now := cl.now
	if lookup == nil {
		return Access{State: StateAccessPending, Reason: ReasonNoCredential}
	}
	cred, ok := lookup(cl.cfg.CredentialEnv)
	if !ok || cred == "" {
		return Access{State: StateAccessPending, Reason: ReasonNoCredential}
	}
	current := time.Now()
	if now != nil {
		current = now()
	}
	if !cl.cfg.GrantExpiresAt.After(current) {
		return Access{State: StateAccessPending, Reason: ReasonGrantExpired}
	}
	return Access{State: StateAuthorized}
}

func validLocation(id string) bool {
	return providerPattern.MatchString(id)
}

func newNonce() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func (cl *Client) call(ctx context.Context, operation, locationID string) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if !validLocation(locationID) {
		return Response{}, &ConfigError{Field: "location_id"}
	}
	access := cl.Resolve()
	if access.State != StateAuthorized {
		return Response{}, &AccessError{State: access.State, Reason: access.Reason}
	}
	if cl.transport == nil {
		return Response{}, &AccessError{State: StateAccessPending, Reason: ReasonRouteUnpinned}
	}
	nonce, err := newNonce()
	if err != nil {
		return Response{}, err
	}
	current := time.Now()
	if cl.now != nil {
		current = cl.now()
	}
	resp, err := cl.transport.RoundTrip(ctx, Request{
		Operation: operation, LocationID: locationID, Scope: cl.cfg.Scope,
		Nonce: nonce, Timestamp: current, HasCredential: true,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Response{}, err
		}
		return Response{}, &ProviderError{Kind: KindProviderOutage}
	}
	if resp.ErrorKind != "" {
		switch resp.ErrorKind {
		case KindWrongScope, KindReplayRefused, KindStaleTimestamp, KindProviderOutage, KindQuotaExceeded, KindRevoked:
			return Response{}, &ProviderError{Kind: resp.ErrorKind}
		default:
			return Response{}, &ProviderError{Kind: KindInvalidState}
		}
	}
	return resp, nil
}

// Status reads one typed location state. UNKNOWN is returned as-is,
// never upgraded; faults refuse without data.
func (cl *Client) Status(ctx context.Context, locationID string) (StatusResult, error) {
	resp, err := cl.call(ctx, "STATUS", locationID)
	if err != nil {
		return StatusResult{}, err
	}
	switch resp.Status {
	case StatusAvailable, StatusUnavailable:
		if resp.ObservedAt.IsZero() {
			return StatusResult{}, &ProviderError{Kind: KindInvalidState}
		}
		return StatusResult{State: resp.Status, ObservedAt: resp.ObservedAt}, nil
	case StatusUnknown:
		return StatusResult{State: StatusUnknown, ObservedAt: resp.ObservedAt, Reason: resp.Reason}, nil
	default:
		return StatusResult{}, &ProviderError{Kind: KindInvalidState}
	}
}

// Tariffs reads operator tariff references for one location. Every entry
// is shape-checked and stamped LICENSED_OPERATOR here; an ill-shaped
// entry refuses the whole read.
func (cl *Client) Tariffs(ctx context.Context, locationID string) ([]TariffResult, error) {
	resp, err := cl.call(ctx, "TARIFFS", locationID)
	if err != nil {
		return nil, err
	}
	out := make([]TariffResult, 0, len(resp.Tariffs))
	for _, t := range resp.Tariffs {
		if !uuidPattern.MatchString(t.ID) || t.Revision == "" || len(t.Revision) > 128 || t.ValidFrom.IsZero() {
			return nil, &ProviderError{Kind: KindInvalidState}
		}
		if t.ValidUntil != nil && !t.ValidUntil.After(t.ValidFrom) {
			return nil, &ProviderError{Kind: KindInvalidState}
		}
		t.Provenance = "LICENSED_OPERATOR"
		out = append(out, t)
	}
	return out, nil
}
