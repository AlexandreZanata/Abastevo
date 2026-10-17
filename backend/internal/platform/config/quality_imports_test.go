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
