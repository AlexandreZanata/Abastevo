package http

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Resolver selects the quota-billing client address for one request
// (SEC02-B, AUD-03). Only the immediate TCP peer inside the explicit
// TrustedProxies allowlist may contribute forwarding headers; every
// other peer bills itself and untrusted headers are ignored, so a
// direct client can never spoof another client's quota subject. An
// empty allowlist trusts nothing and preserves the historical
// peer-only behavior (shared bucket, safe but unfair). Raw IPs never
// persist: the caller hashes the returned address before storage,
// and logs carry no addresses (see platform telemetry redaction).
type Resolver struct {
	TrustedProxies []netip.Prefix
}

// ParseTrustedProxies parses a comma-separated CIDR allowlist
// (ANPFUEL_TRUSTED_PROXIES). Empty input trusts nothing. Each entry
// must be a valid CIDR; anything else fails closed with the variable
// named and no value echoed.
func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("ANPFUEL_TRUSTED_PROXIES must be comma-separated CIDRs")
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

// ClientIP returns the billable client address for r: the
// proxy-observed forwarding address when the immediate peer is
// trusted, otherwise the peer itself. Malformed or absent forwarding
// data falls back to the peer; a malformed peer fails closed like
// the historical path (callers answer 400).
func (rs Resolver) ClientIP(r *http.Request) (string, error) {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", err
	}
	peer = strings.TrimSpace(peer)
	addr, err := netip.ParseAddr(peer)
	if err != nil {
		return "", err
	}
	if addr.Zone() != "" {
		return "", fmt.Errorf("http: scoped peer address refused")
	}
	if !rs.trusted(addr) {
		return peer, nil
	}
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff == "" {
		if xff = strings.TrimSpace(r.Header.Get("X-Real-IP")); xff == "" {
			return peer, nil
		}
	}
	// Header-chain ownership: only the entry observed by the trusted
	// immediate proxy is billable, i.e. the last valid address. Older
	// entries are client-asserted and never selected; with the
	// supplied Caddy edge (header_up replaces the chain with the
	// observed host) the chain holds exactly one entry.
	var last string
	for _, part := range strings.Split(xff, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		candidate, err := netip.ParseAddr(part)
		if err != nil || candidate.Zone() != "" {
			continue
		}
		last = part
	}
	if last == "" {
		return peer, nil
	}
	return last, nil
}

func (rs Resolver) trusted(addr netip.Addr) bool {
	unmapped := addr.Unmap()
	for _, prefix := range rs.TrustedProxies {
		if prefix.Contains(unmapped) {
			return true
		}
	}
	return false
}
