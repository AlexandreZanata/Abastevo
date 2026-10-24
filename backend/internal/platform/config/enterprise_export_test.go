package config

import "testing"

func TestEnterpriseExportDefaultsAndValidation(t *testing.T) {
	loadWith := func(values map[string]string) (Config, error) {
		return load(func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		})
	}
	cfg, err := loadWith(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnterpriseExportEnabled || cfg.EnterpriseExportRoot != "" || cfg.EnterpriseExportWindowWeeks != 5 {
		t.Fatalf("defaults = %+v", cfg)
	}

	cfg, err = loadWith(map[string]string{
		"ANPFUEL_ENTERPRISE_EXPORT_ENABLED":      "true",
		"ANPFUEL_ENTERPRISE_EXPORT_ROOT":         "/var/lib/abastevo/export",
		"ANPFUEL_ENTERPRISE_EXPORT_WINDOW_WEEKS": "12",
	})
	if err != nil || !cfg.EnterpriseExportEnabled ||
		cfg.EnterpriseExportRoot != "/var/lib/abastevo/export" ||
		cfg.EnterpriseExportWindowWeeks != 12 {
		t.Fatalf("enabled config = %+v, %v", cfg, err)
	}

	for _, tc := range []struct {
		name   string
		values map[string]string
	}{
		{"enabled without root", map[string]string{"ANPFUEL_ENTERPRISE_EXPORT_ENABLED": "true"}},
		{"relative root", map[string]string{
			"ANPFUEL_ENTERPRISE_EXPORT_ENABLED": "true",
			"ANPFUEL_ENTERPRISE_EXPORT_ROOT":    "relative/export",
		}},
		{"zero weeks", map[string]string{
			"ANPFUEL_ENTERPRISE_EXPORT_ROOT":         "/var/lib/abastevo/export",
			"ANPFUEL_ENTERPRISE_EXPORT_WINDOW_WEEKS": "0",
		}},
		{"too many weeks", map[string]string{
			"ANPFUEL_ENTERPRISE_EXPORT_ROOT":         "/var/lib/abastevo/export",
			"ANPFUEL_ENTERPRISE_EXPORT_WINDOW_WEEKS": "13",
		}},
		{"bad enabled", map[string]string{"ANPFUEL_ENTERPRISE_EXPORT_ENABLED": "yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadWith(tc.values); err == nil {
				t.Fatal("invalid enterprise export config accepted")
			}
		})
	}
}
