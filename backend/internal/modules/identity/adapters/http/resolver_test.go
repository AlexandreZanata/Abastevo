package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func trustedResolver() Resolver {
	proxies, err := ParseTrustedProxies("10.0.0.0/8, 127.0.0.0/8")
	if err != nil {
		panic(err)
	}
	return Resolver{TrustedProxies: proxies}
}

func resolveRequest(t *testing.T, rs Resolver, remoteAddr, xff, realIP string) (string, error) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/identity/challenges", strings.NewReader(`{}`))
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if realIP != "" {
		req.Header.Set("X-Real-IP", realIP)
	}
	return rs.ClientIP(req)
}

func TestParseTrustedProxies(t *testing.T) {
	if got, err := ParseTrustedProxies(""); err != nil || len(got) != 0 {
		t.Fatalf("empty must trust nothing, got %v (%v)", got, err)
	}
	got, err := ParseTrustedProxies("10.0.0.0/8,192.168.1.0/24")
	if err != nil || len(got) != 2 {
		t.Fatalf("valid CSV must parse, got %v (%v)", got, err)
	}
	for _, bad := range []string{"10.0.0.1", "10.0.0.0/33", "private_ranges", "10.0.0.0/8, nope"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("CIDR %q must fail closed", bad)
		}
	}
}

func TestResolverBilling(t *testing.T) {
	rs := trustedResolver()
	for _, tc := range []struct {
		name       string
		remoteAddr string
		xff        string
		realIP     string
		want       string
		wantErr    bool
	}{
		{"direct bills peer", "203.0.113.9:50000", "", "", "203.0.113.9", false},
		{"direct ignores spoofed xff", "203.0.113.9:50000", "198.51.100.7", "", "203.0.113.9", false},
		{"direct ignores spoofed real ip", "203.0.113.9:50000", "", "198.51.100.7", "203.0.113.9", false},
		{"trusted bills single xff", "10.0.0.5:41230", "198.51.100.7", "", "198.51.100.7", false},
		{"trusted bills real ip fallback", "10.0.0.5:41230", "", "198.51.100.7", "198.51.100.7", false},
		{"trusted malformed falls back", "10.0.0.5:41230", "not-an-ip", "", "10.0.0.5", false},
		{"trusted empty falls back", "10.0.0.5:41230", "", "", "10.0.0.5", false},
		{"trusted chain bills last", "10.0.0.5:41230", "198.51.100.7, 198.51.100.8", "", "198.51.100.8", false},
		{"trusted chain skips invalid tail", "10.0.0.5:41230", "198.51.100.7, nope", "", "198.51.100.7", false},
		{"trusted ipv6 client", "10.0.0.5:41230", "2001:db8::7", "", "2001:db8::7", false},
		{"loopback trusted", "127.0.0.1:8080", "198.51.100.7", "", "198.51.100.7", false},
		{"private peer unlisted falls back", "172.16.4.4:9000", "198.51.100.7", "", "172.16.4.4", false},
		{"malformed peer fails closed", "not-a-peer", "198.51.100.7", "", "", true},
	} {
		got, err := resolveRequest(t, rs, tc.remoteAddr, tc.xff, tc.realIP)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: must fail closed", tc.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: client = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDefaultResolverTrustsNothing(t *testing.T) {
	var rs Resolver
	got, err := resolveRequest(t, rs, "10.0.0.5:41230", "198.51.100.7", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.0.0.5" {
		t.Fatalf("empty allowlist must bill the peer, got %q", got)
	}
}
