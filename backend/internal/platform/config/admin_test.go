package config

import (
	"strings"
	"testing"
)

func adminHash(fill string) string { return strings.Repeat(fill, 64) }

// TestAdminConfig_ADM01_DisabledByDefault is the no-hash configuration.
func TestAdminConfig_ADM01_DisabledByDefault(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AdminKeyHashes) != 0 || cfg.AdminOrigin != "" || len(cfg.AdminAllowedCIDRs) != 0 {
		t.Fatalf("admin unexpectedly configured: %+v", cfg.AdminKeyHashes)
	}
}

// TestAdminConfig_ADM01_MalformedHashFails is S2.
func TestAdminConfig_ADM01_MalformedHashFails(t *testing.T) {
	for _, raw := range []string{
		"not-a-hash",
		strings.Repeat("A", 64),
		strings.Repeat("g", 64),
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
	} {
		_, err := load(envFunc(map[string]string{keyAdminKeySHA256: raw}))
		if err == nil || !strings.Contains(err.Error(), keyAdminKeySHA256) {
			t.Fatalf("hash %q err=%v", raw, err)
		}
		if strings.Contains(err.Error(), raw) {
			t.Fatal("configuration error echoed the value")
		}
	}
}

// TestAdminConfig_ADM06_Origin covers required and normalized origins.
func TestAdminConfig_ADM06_Origin(t *testing.T) {
	hash := adminHash("a")
	if _, err := load(envFunc(map[string]string{keyAdminKeySHA256: hash})); err == nil || !strings.Contains(err.Error(), keyAdminOrigin) {
		t.Fatalf("missing origin err=%v", err)
	}
	cfg, err := load(envFunc(map[string]string{
		keyAdminKeySHA256: hash,
		keyAdminOrigin:    "https://admin.example.org/",
	}))
	if err != nil || cfg.AdminOrigin != "https://admin.example.org" {
		t.Fatalf("origin=%q err=%v", cfg.AdminOrigin, err)
	}
	for _, raw := range []string{"admin.example.org", "https://admin.example.org/path", "https://admin.example.org?q=1", "https://@admin.example.org"} {
		if _, err := load(envFunc(map[string]string{keyAdminKeySHA256: hash, keyAdminOrigin: raw})); err == nil {
			t.Fatalf("origin %q accepted", raw)
		}
	}
}

// TestAdminConfig_ADM07_NextHashAccepted covers rotation configuration.
func TestAdminConfig_ADM07_NextHashAccepted(t *testing.T) {
	first, second := adminHash("a"), adminHash("b")
	cfg, err := load(envFunc(map[string]string{
		keyAdminKeySHA256:     first,
		keyAdminKeySHA256Next: second,
		keyAdminOrigin:        "https://admin.example.org",
	}))
	if err != nil || len(cfg.AdminKeyHashes) != 2 {
		t.Fatalf("hashes=%v err=%v", cfg.AdminKeyHashes, err)
	}
	duplicate, err := load(envFunc(map[string]string{
		keyAdminKeySHA256:     first,
		keyAdminKeySHA256Next: first,
		keyAdminOrigin:        "https://admin.example.org",
	}))
	if err != nil || len(duplicate.AdminKeyHashes) != 1 {
		t.Fatalf("duplicate hashes=%v err=%v", duplicate.AdminKeyHashes, err)
	}
}

// TestAdminConfig_ADM08_CIDRFence covers the optional network fence.
func TestAdminConfig_ADM08_CIDRFence(t *testing.T) {
	cfg, err := load(envFunc(map[string]string{
		keyAdminKeySHA256:    adminHash("a"),
		keyAdminOrigin:       "https://admin.example.org",
		keyAdminAllowedCIDRs: "10.0.0.0/8, 127.0.0.0/8",
	}))
	if err != nil || len(cfg.AdminAllowedCIDRs) != 2 {
		t.Fatalf("cidrs=%v err=%v", cfg.AdminAllowedCIDRs, err)
	}
	if _, err := load(envFunc(map[string]string{
		keyAdminKeySHA256:    adminHash("a"),
		keyAdminOrigin:       "https://admin.example.org",
		keyAdminAllowedCIDRs: "10.0.0.0/33",
	})); err == nil || !strings.Contains(err.Error(), keyAdminAllowedCIDRs) {
		t.Fatalf("malformed CIDR err=%v", err)
	}
}
