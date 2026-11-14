package protocol

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// ValidateOrigin is a strict common Go/JVM capture profile, not a general URL normalizer.
func ValidateOrigin(raw string) error {
	if !strings.HasPrefix(raw, "https://") || strings.ContainsAny(raw, "?#%\\") {
		return ErrInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.Opaque != "" || parsed.ForceQuery {
		return ErrInvalid
	}
	authority := parsed.Host
	host := parsed.Hostname()
	port := parsed.Port()
	if authority != strings.ToLower(authority) || host == "" {
		return ErrInvalid
	}
	expected := host
	if strings.HasPrefix(authority, "[") {
		expected = "[" + host + "]"
		ip, err := netip.ParseAddr(host)
		if err != nil || !ip.Is6() || ip.Zone() != "" {
			return ErrInvalid
		}
	} else {
		if strings.Contains(host, ":") {
			return ErrInvalid
		}
		ip, err := netip.ParseAddr(host)
		if err == nil {
			if !ip.Is4() || ip.String() != host {
				return ErrInvalid
			}
		} else if !dnsHost(host) {
			return ErrInvalid
		}
	}
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 || value == 443 || strconv.Itoa(value) != port {
			return ErrInvalid
		}
		expected += ":" + port
	}
	if authority != expected {
		return ErrInvalid
	}
	return nil
}
func dnsHost(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	if strings.Trim(host, "0123456789") == "" {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i, c := range []byte(label) {
			if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
				continue
			}
			if c == '-' && i > 0 && i < len(label)-1 {
				continue
			}
			return false
		}
	}
	if len(labels) > 1 {
		last := labels[len(labels)-1]
		if last[0] < 'a' || last[0] > 'z' {
			return false
		}
	}
	return true
}
