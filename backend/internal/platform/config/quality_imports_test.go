package config

import "testing"

func TestQualityImportsDefaultOffAndExplicitFlag(t *testing.T) {
	for _, tc := range []struct {
		raw                 string
		present, want, deny bool
	}{{"", false, false, false}, {"false", true, false, false}, {"true", true, true, false}, {"bad", true, false, true}} {
		cfg, e := load(func(key string) (string, bool) {
			if key == "ANPFUEL_QUALITY_IMPORTS_ENABLED" {
				return tc.raw, tc.present
			}
			return "", false
		})
		if (e != nil) != tc.deny {
			t.Fatal("native flag validation", e)
		}
		if e == nil && cfg.QualityImportsEnabled != tc.want {
			t.Fatal("native intake activated implicitly")
		}
	}
}

func TestQualitySourceSchedulesDefaultOffAndIndependent(t *testing.T) {
	keys := []string{"ANPFUEL_QUALITY_DISCOVERY_ENABLED", "ANPFUEL_QUALITY_REVALIDATION_ENABLED"}
	cfg, e := load(func(string) (string, bool) { return "", false })
	if e != nil || cfg.QualityDiscoveryEnabled || cfg.QualityRevalidationEnabled || cfg.QualityImportsEnabled {
		t.Fatal("source schedules activated implicitly", cfg.QualityDiscoveryEnabled, cfg.QualityRevalidationEnabled, e)
	}
	for i, key := range keys {
		cfg, e = load(func(got string) (string, bool) {
			if got == key {
				return "true", true
			}
			return "", false
		})
		if e != nil {
			t.Fatal(e)
		}
		discovery, revalidation := cfg.QualityDiscoveryEnabled, cfg.QualityRevalidationEnabled
		if i == 0 && (!discovery || revalidation) || i == 1 && (discovery || !revalidation) {
			t.Fatal("schedules not independent", i, discovery, revalidation)
		}
		if cfg.QualityImportsEnabled {
			t.Fatal("schedule implicitly activated native imports")
		}
	}
	if _, e = load(func(got string) (string, bool) {
		if got == keys[0] {
			return "maybe", true
		}
		return "", false
	}); e == nil {
		t.Fatal("invalid schedule boolean accepted")
	}
}
