package config

import "testing"

func TestDiscoveryDefaultAndExplicitLocalDisable(t *testing.T) {
	for _, tc := range []struct {
		raw                 string
		present, want, deny bool
	}{{"", false, true, false}, {"false", true, false, false}, {"true", true, true, false}, {"typo", true, false, true}} {
		cfg, err := load(func(key string) (string, bool) {
			if key == "ANPFUEL_ANP_DISCOVERY_ENABLED" {
				return tc.raw, tc.present
			}
			return "", false
		})
		if (err != nil) != tc.deny {
			t.Fatalf("invalid discovery setting: %v", err)
		}
		if err == nil && cfg.ANPDiscoveryEnabled != tc.want {
			t.Fatal("unexpected discovery activation")
		}
	}
}
