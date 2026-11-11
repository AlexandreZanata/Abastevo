package config

import "testing"

// The app's local proof origin is pinned to http://127.0.0.1:18080 and signs
// "@authority" with that port, so development accepts a loopback host:port as
// the canonical host. Staging and production keep bare public hostnames.
func TestCanonicalHostLoopbackPortOnlyInDevelopment(t *testing.T) {
	cases := []struct {
		env, host string
		valid     bool
	}{
		{"development", "127.0.0.1:18080", true},
		{"development", "localhost:18080", true},
		{"development", "teste.abastevo.com.br", true},
		{"development", "10.0.0.5:18080", false},
		{"development", "example.com:443", false},
		{"development", "127.0.0.1:0", false},
		{"development", "127.0.0.1:99999", false},
		{"development", "127.0.0.1:", false},
		{"staging", "127.0.0.1:18080", false},
		{"production", "127.0.0.1:18080", false},
	}
	for _, tc := range cases {
		vars := map[string]string{
			"ANPFUEL_ENV":            tc.env,
			"ANPFUEL_DATABASE_URL":   "postgres://fixture:fixture@localhost/fixture",
			"ANPFUEL_CANONICAL_HOST": tc.host,
			"ANPFUEL_CURSOR_SECRET":  "synthetic-cursor-secret-with-sufficient-length",
		}
		cfg, err := load(envFunc(vars))
		hostErr := err != nil && (err.Error() == keyCanonicalHost+" must be a bare hostname")
		if tc.valid && err != nil {
			t.Fatalf("%s %q rejected: %v", tc.env, tc.host, err)
		}
		if !tc.valid && err == nil {
			t.Fatalf("%s %q accepted", tc.env, tc.host)
		}
		if !tc.valid && !hostErr && tc.env == "development" {
			t.Fatalf("%s %q failed for another reason: %v", tc.env, tc.host, err)
		}
		if tc.valid && cfg.CanonicalHost != tc.host {
			t.Fatalf("canonical host = %q; want %q", cfg.CanonicalHost, tc.host)
		}
	}
}
