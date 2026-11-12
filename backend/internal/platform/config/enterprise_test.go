package config

import (
	"strings"
	"testing"
)

func TestEnterpriseSurfaceDefaultsOffAndValidatesSwitch(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil || cfg.EnterpriseEnabled {
		t.Fatal("enterprise enabled by default", err)
	}
	for _, raw := range []string{"true", "false"} {
		cfg, err := load(envFunc(map[string]string{keyEnterpriseEnabled: raw}))
		if err != nil || cfg.EnterpriseEnabled != (raw == "true") {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"", "secret-value", " true "} {
		if _, err := load(envFunc(map[string]string{keyEnterpriseEnabled: raw})); err == nil || !strings.Contains(err.Error(), keyEnterpriseEnabled) || strings.Contains(err.Error(), "secret-value") {
			t.Fatal("unsafe config error", err)
		}
	}
}
