package config

import "testing"

func TestEnterprisePortalRefreshDefaultsAndValidation(t *testing.T) {
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
	if cfg.EnterprisePortalRefreshEnabled || cfg.EnterprisePortalRoot != "" || cfg.EnterprisePortalSiteURL != "" || cfg.EnterprisePortalIndexable || cfg.EnterprisePortalOfficialOnly || cfg.EnterprisePortalSearchScript != "" || cfg.EnterprisePortalLandingURL != "" {
		t.Fatalf("defaults = %+v", cfg)
	}
	base := map[string]string{
		"ANPFUEL_ENTERPRISE_EXPORT_ENABLED":         "true",
		"ANPFUEL_ENTERPRISE_EXPORT_ROOT":            "/var/lib/abastevo/export",
		"ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED": "true",
		"ANPFUEL_ENTERPRISE_PORTAL_ROOT":            "/var/lib/abastevo/portal",
		"ANPFUEL_ENTERPRISE_PORTAL_SITE_URL":        "https://portal.example",
		"ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE":       "true",
		"ANPFUEL_ENTERPRISE_PORTAL_SEARCH_JS":       "/opt/abastevo/search.js",
		"ANPFUEL_ENTERPRISE_PORTAL_LANDING_URL":     "https://landing.example",
	}
	cfg, err = loadWith(base)
	if err != nil || !cfg.EnterprisePortalRefreshEnabled || cfg.EnterprisePortalRoot != "/var/lib/abastevo/portal" ||
		cfg.EnterprisePortalSiteURL != "https://portal.example" || !cfg.EnterprisePortalIndexable ||
		cfg.EnterprisePortalSearchScript != "/opt/abastevo/search.js" || cfg.EnterprisePortalLandingURL != "https://landing.example" {
		t.Fatalf("enabled config = %+v, %v", cfg, err)
	}
	without := func(key string) map[string]string {
		values := map[string]string{}
		for k, v := range base {
			if k != key {
				values[k] = v
			}
		}
		return values
	}
	for _, tc := range []struct {
		name   string
		values map[string]string
	}{
		{"without export", map[string]string{
			"ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED": "true",
			"ANPFUEL_ENTERPRISE_PORTAL_ROOT":            "/var/lib/abastevo/portal",
			"ANPFUEL_ENTERPRISE_PORTAL_SITE_URL":        "https://portal.example",
		}},
		{"without root", without("ANPFUEL_ENTERPRISE_PORTAL_ROOT")},
		{"relative root", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_ROOT")
			v["ANPFUEL_ENTERPRISE_PORTAL_ROOT"] = "relative/portal"
			return v
		}()},
		{"same root", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_ROOT")
			v["ANPFUEL_ENTERPRISE_PORTAL_ROOT"] = "/var/lib/abastevo/export"
			return v
		}()},
		{"without site URL", without("ANPFUEL_ENTERPRISE_PORTAL_SITE_URL")},
		{"bad site URL", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_SITE_URL")
			v["ANPFUEL_ENTERPRISE_PORTAL_SITE_URL"] = "ftp://portal.example"
			return v
		}()},
		{"site URL path", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_SITE_URL")
			v["ANPFUEL_ENTERPRISE_PORTAL_SITE_URL"] = "https://portal.example/abastevo"
			return v
		}()},
		{"bad landing URL", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_LANDING_URL")
			v["ANPFUEL_ENTERPRISE_PORTAL_LANDING_URL"] = "javascript:alert(1)"
			return v
		}()},
		{"landing URL path", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_LANDING_URL")
			v["ANPFUEL_ENTERPRISE_PORTAL_LANDING_URL"] = "https://landing.example/home"
			return v
		}()},
		{"relative search JS", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_SEARCH_JS")
			v["ANPFUEL_ENTERPRISE_PORTAL_SEARCH_JS"] = "search.js"
			return v
		}()},
		{"bad enabled", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED")
			v["ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED"] = "yes"
			return v
		}()},
		{"bad indexable", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE")
			v["ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE"] = "yes"
			return v
		}()},
		{"bad official-only", func() map[string]string {
			v := without("ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED")
			v["ANPFUEL_ENTERPRISE_PORTAL_OFFICIAL_ONLY"] = "yes"
			return v
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadWith(tc.values); err == nil {
				t.Fatal("invalid portal refresh config accepted")
			}
		})
	}
	official, err := loadWith(func() map[string]string {
		v := map[string]string{}
		for k, val := range base {
			v[k] = val
		}
		v["ANPFUEL_ENTERPRISE_PORTAL_OFFICIAL_ONLY"] = "true"
		return v
	}())
	if err != nil || !official.EnterprisePortalOfficialOnly {
		t.Fatalf("official-only config = %+v, %v", official, err)
	}
}
