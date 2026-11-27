package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func storeEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

func withSecretFiles(t *testing.T, files map[string]string) {
	t.Helper()
	previous := readSecretFile
	readSecretFile = func(name string) ([]byte, error) {
		if v, ok := files[name]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("missing")
	}
	t.Cleanup(func() { readSecretFile = previous })
}

func completeStoreEnv() map[string]string {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	other := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	return map[string]string{
		keyStoreEnvironment: "production", keyStoreTokenKey: key, keyStoreBindingKey: other,
		keyPlayServiceAccountFile: "/run/secrets/play.json", keyPlayProducts: "abastevo_premium:monthly, yearly",
		keyPlayPushSubscription: "projects/p/subscriptions/rtdn", keyPlayPushAudience: "https://abastevo.com.br/v1/premium/provider-notifications",
		keyPlayPushServiceAccount: "rtdn@p.iam.gserviceaccount.com",
		keyAppStoreIssuerID:       "57246542-96fe-1a63-e053-0824d011072a", keyAppStoreKeyID: "2X9R4HXF34",
		keyAppStorePrivateKeyFile: "/run/secrets/appstore.p8", keyAppStoreProducts: "com.anpfuel.app.premium.yearly,com.anpfuel.app.premium.monthly",
	}
}

func TestStoreBillingOffByDefault(t *testing.T) {
	cfg := loadStoreBilling(storeEnv(nil))
	if cfg.Play.Configured || cfg.AppStore.Configured || len(cfg.Issues) != 0 || cfg.Play.PackageName != "com.abastevo" || cfg.AppStore.BundleID != "com.anpfuel.app" {
		t.Fatalf("default %+v", cfg)
	}
}

func TestStoreBillingCompleteConfiguration(t *testing.T) {
	withSecretFiles(t, map[string]string{"/run/secrets/play.json": `{"type":"service_account"}`, "/run/secrets/appstore.p8": "synthetic-p8-key"})
	cfg := loadStoreBilling(storeEnv(completeStoreEnv()))
	if !cfg.Play.Configured || !cfg.Play.PushConfigured || !cfg.AppStore.Configured || len(cfg.Issues) != 0 {
		t.Fatalf("complete config %+v issues %v", cfg, cfg.Issues)
	}
	if plans := cfg.Play.Products["abastevo_premium"]; len(plans) != 2 || plans[1] != "yearly" {
		t.Fatalf("plans %v", cfg.Play.Products)
	}
	if strings.Join(cfg.AppStore.Products, ",") != "com.anpfuel.app.premium.monthly,com.anpfuel.app.premium.yearly" || cfg.AppStore.SandboxFallback {
		t.Fatalf("app store %+v", cfg.AppStore)
	}
	logged := cfg.logValue().String()
	for _, secret := range []string{"service_account", "synthetic-p8-key", "2X9R4HXF34"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log leaks %q: %s", secret, logged)
		}
	}
}

func TestPlayLicenseTestAccountFileFailClosed(t *testing.T) {
	const first = "77777777-7777-4777-8777-777777777777"
	const second = "abcdefab-cdef-4abc-8def-abcdefabcdef"
	for _, tc := range []struct {
		name    string
		path    string
		content string
		valid   bool
	}{
		{name: "unset"},
		{name: "unreadable", path: "/run/secrets/missing"},
		{name: "empty", path: "/run/secrets/testers", content: " \r\n"},
		{name: "mixed valid and malformed", path: "/run/secrets/testers", content: first + "\ninvalid\n"},
		{name: "noncanonical UUID", path: "/run/secrets/testers", content: strings.ToUpper(second)},
		{name: "oversized", path: "/run/secrets/testers", content: strings.Repeat("\n", 16<<10) + first},
		{name: "valid", path: "/run/secrets/testers", content: first + "\r\n\n" + second + "\r\n", valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSecretFiles(t, map[string]string{"/run/secrets/play.json": `{}`, "/run/secrets/appstore.p8": "key", "/run/secrets/testers": tc.content})
			values := completeStoreEnv()
			values[keyStoreEnvironment] = "test"
			values[keyPlayTestAccountsFile] = tc.path
			cfg := loadStoreBilling(storeEnv(values))
			if !cfg.Play.Configured || !cfg.Play.PushConfigured || !cfg.AppStore.Configured {
				t.Fatal("test account file affected provider maintenance or App Store")
			}
			if tc.valid {
				_, haveFirst := cfg.Play.TestAccounts[first]
				_, haveSecond := cfg.Play.TestAccounts[second]
				if !haveFirst || !haveSecond || len(cfg.Play.TestAccounts) != 2 || len(cfg.Issues) != 0 {
					t.Fatal("valid test accounts were not configured")
				}
			} else if len(cfg.Play.TestAccounts) != 0 || len(cfg.Issues) != 1 {
				t.Fatal("invalid account file must deny all test offers and record one issue")
			}
			logged := cfg.logValue().String() + strings.Join(cfg.Issues, "\n")
			if strings.Contains(logged, first) || strings.Contains(logged, second) {
				t.Fatal("account IDs appeared in log output")
			}
		})
	}
	withSecretFiles(t, map[string]string{"/run/secrets/play.json": `{}`, "/run/secrets/appstore.p8": "key"})
	values := completeStoreEnv()
	values[keyPlayTestAccountsFile] = "/run/secrets/missing"
	if cfg := loadStoreBilling(storeEnv(values)); !cfg.Play.Configured || len(cfg.Play.TestAccounts) != 0 || len(cfg.Issues) != 0 {
		t.Fatal("production must ignore the test file")
	}
}

func TestStoreBillingPartialConfigurationStaysOff(t *testing.T) {
	withSecretFiles(t, map[string]string{"/run/secrets/play.json": `{}`, "/run/secrets/appstore.p8": "key"})
	cases := map[string]func(map[string]string){
		"environment": func(v map[string]string) { v[keyStoreEnvironment] = "staging" },
		"token key":   func(v map[string]string) { v[keyStoreTokenKey] = "short" },
		"binding key": func(v map[string]string) { delete(v, keyStoreBindingKey) },
		"missing file": func(v map[string]string) {
			v[keyPlayServiceAccountFile] = "/nope"
			v[keyAppStorePrivateKeyFile] = "/nope"
		},
		"no products": func(v map[string]string) { v[keyPlayProducts] = "bad"; v[keyAppStoreProducts] = " , " },
		"no issuer":   func(v map[string]string) { delete(v, keyAppStoreIssuerID); v[keyPlayPackageName] = "bad name" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			values := completeStoreEnv()
			mutate(values)
			cfg := loadStoreBilling(storeEnv(values))
			if len(cfg.Issues) == 0 {
				t.Fatal("issues not recorded")
			}
			if name == "no issuer" {
				if cfg.AppStore.Configured || cfg.Play.Configured {
					t.Fatalf("partial channel enabled %+v", cfg)
				}
				return
			}
			if cfg.Play.Configured && cfg.AppStore.Configured {
				t.Fatalf("partial channels enabled %+v", cfg)
			}
		})
	}
	values := completeStoreEnv()
	delete(values, keyPlayPushAudience)
	cfg := loadStoreBilling(storeEnv(values))
	if cfg.Play.PushConfigured || len(cfg.Issues) == 0 {
		t.Fatal("incomplete push identity accepted")
	}
}
