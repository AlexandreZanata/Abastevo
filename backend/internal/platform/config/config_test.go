package config

import (
	"fmt"
	"strings"
	"testing"
)

// envFunc builds a lookup func from a static map for hermetic tests.
func envFunc(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadDevelopmentDefaults(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatalf("load with empty environment: %v", err)
	}
	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDevelopment)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.DatabaseURL == "" {
		t.Error("development DatabaseURL must default, got empty")
	}
	if cfg.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, 1<<20)
	}
}

func TestQualityReportRecoverySwitch(t *testing.T) {
	for _, raw := range []string{"true", "false"} {
		cfg, err := load(envFunc(map[string]string{keyQualityReportsEnabled: raw}))
		if err != nil || cfg.QualityReportsEnabled != (raw == "true") {
			t.Fatal(raw, cfg.QualityReportsEnabled, err)
		}
	}
	if _, err := load(envFunc(map[string]string{keyQualityReportsEnabled: "invalid"})); err == nil {
		t.Fatal("invalid report switch accepted")
	}
}

func TestLoadUnknownEnvironmentFails(t *testing.T) {
	_, err := load(envFunc(map[string]string{keyEnv: "prod-east"}))
	if err == nil {
		t.Fatal("expected error for unknown environment, got nil")
	}
	if !strings.Contains(err.Error(), keyEnv) {
		t.Errorf("error %q must name the variable %q", err, keyEnv)
	}
	if strings.Contains(err.Error(), "prod-east") {
		t.Errorf("error %q must not echo the raw value", err)
	}
}

func TestLoadStagingRequiresExplicitDSN(t *testing.T) {
	_, err := load(envFunc(map[string]string{keyEnv: "staging"}))
	if err == nil {
		t.Fatal("expected error for staging without DSN, got nil")
	}
	if !strings.Contains(err.Error(), keyDatabaseURL) {
		t.Errorf("error %q must name the variable %q", err, keyDatabaseURL)
	}
}

func TestLoadProductionRejectsDevDefaultDSN(t *testing.T) {
	_, err := load(envFunc(map[string]string{
		keyEnv:         "production",
		keyDatabaseURL: devDatabaseURL,
	}))
	if err == nil {
		t.Fatal("expected error for production with dev-default DSN, got nil")
	}
}

func TestLoadMalformedDSNFailsWithoutEcho(t *testing.T) {
	bad := "postgres://user:s3cr3t@db/internal?sslmode=require#frag%%zz"
	_, err := load(envFunc(map[string]string{keyDatabaseURL: bad}))
	if err == nil {
		t.Fatal("expected error for malformed DSN, got nil")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error %q leaks the DSN secret", err)
	}
}

func TestLoadInvalidLimitsFail(t *testing.T) {
	cases := map[string]map[string]string{
		"body-abc":     {keyMaxBodyBytes: "abc"},
		"body-zero":    {keyMaxBodyBytes: "0"},
		"body-neg":     {keyMaxBodyBytes: "-1"},
		"timeout-word": {keyShutdownTimeout: "soon"},
		"timeout-zero": {keyShutdownTimeout: "0s"},
		"loglevel":     {keyLogLevel: "verbose"},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := load(envFunc(vars)); err == nil {
				t.Fatalf("expected error for %s, got nil", name)
			}
		})
	}
}

func TestLoadErrorsNeverContainSecrets(t *testing.T) {
	secret := "postgres://app:P@ssw0rd-9z@db.example:5432/app?sslmode=require"
	_, err := load(envFunc(map[string]string{
		keyDatabaseURL:  secret,
		keyMaxBodyBytes: "bogus",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "P@ssw0rd-9z") {
		t.Errorf("error %q leaks the DSN secret", err)
	}
}

func TestLogValueRedactsSecrets(t *testing.T) {
	secret := "postgres://app:P@ssw0rd-9z@db.example:5432/app?sslmode=require"
	cfg, err := load(envFunc(map[string]string{
		keyEnv:         "staging",
		keyDatabaseURL: secret,
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rendered := fmt.Sprintf("%v", cfg.LogValue())
	if strings.Contains(rendered, "P@ssw0rd-9z") || strings.Contains(rendered, secret) {
		t.Errorf("LogValue %q leaks the DSN secret", rendered)
	}
	if !strings.Contains(rendered, "staging") {
		t.Errorf("LogValue %q must keep the non-secret environment", rendered)
	}
}

func TestLoadValidStagingConfig(t *testing.T) {
	cfg, err := load(envFunc(map[string]string{
		keyEnv:             "staging",
		keyDatabaseURL:     "postgres://app:x@db.internal:5432/app?sslmode=require",
		keyHTTPAddr:        "127.0.0.1:9000",
		keyMaxBodyBytes:    "2097152",
		keyShutdownTimeout: "30s",
		keyLogLevel:        "warn",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Env != EnvStaging || cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Errorf("unexpected mapping: %+v", cfg)
	}
	if cfg.MaxBodyBytes != 2097152 {
		t.Errorf("MaxBodyBytes = %d, want 2097152", cfg.MaxBodyBytes)
	}
}

func TestLoadMetricsAddrDefaultsLoopback(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.MetricsAddr != "127.0.0.1:9090" {
		t.Errorf("MetricsAddr = %q, want loopback default", cfg.MetricsAddr)
	}
}

func TestLoadMetricsAddrValidation(t *testing.T) {
	cfg, err := load(envFunc(map[string]string{keyMetricsAddr: ""}))
	if err != nil || cfg.MetricsAddr != "" {
		t.Errorf("explicit empty must disable, got %q, %v", cfg.MetricsAddr, err)
	}
	if _, err := load(envFunc(map[string]string{keyMetricsAddr: "not-an-addr"})); err == nil {
		t.Error("malformed metrics addr accepted")
	} else if !strings.Contains(err.Error(), keyMetricsAddr) {
		t.Errorf("error %q must name the variable", err)
	}
	if _, err := load(envFunc(map[string]string{keyMetricsAddr: "0.0.0.0:9090"})); err != nil {
		t.Errorf("explicit non-loopback must parse (firewall owns exposure): %v", err)
	}
}

func TestLoadTrainingTermsDigest(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TrainingTermsDigest != defaultTrainingTermsDigest {
		t.Errorf("TrainingTermsDigest = %q, want the published text digest", cfg.TrainingTermsDigest)
	}
	cfg, err = load(envFunc(map[string]string{keyTrainingTermsDigest: defaultTrainingTermsDigest}))
	if err != nil || cfg.TrainingTermsDigest != defaultTrainingTermsDigest {
		t.Errorf("explicit digest must apply, got %q, %v", cfg.TrainingTermsDigest, err)
	}
	for _, bad := range []string{"ab12", strings.Repeat("a", 64), "741dcce562dd29959c5fca09ad5ebd6f986fa6e94c80c2e556fe3f6f80beac48", "UPPERCASE", "not hex!", strings.Repeat("a", 129), "0x12"} {
		if _, err := load(envFunc(map[string]string{keyTrainingTermsDigest: bad})); err == nil {
			t.Errorf("digest %q accepted", bad)
		} else if !strings.Contains(err.Error(), keyTrainingTermsDigest) {
			t.Errorf("error %q must name the variable", err)
		}
	}
}

func TestLoadMailMode(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.MailMode != "off" {
		t.Errorf("MailMode = %q, want off by default", cfg.MailMode)
	}
	cfg, err = load(envFunc(map[string]string{keyMailMode: "memory"}))
	if err != nil || cfg.MailMode != "memory" {
		t.Errorf("explicit memory must apply, got %q, %v", cfg.MailMode, err)
	}
	if _, err := load(envFunc(map[string]string{keyMailMode: "smtp"})); err == nil {
		t.Error("unconfigured smtp accepted")
	} else if !strings.Contains(err.Error(), keyMailMode) {
		t.Errorf("error %q must name the variable", err)
	}
}

// SEC02-C: the deliberate global ceilings default to disabled and
// refuse negative/non-numeric values at startup with the variable
// named.
func TestLoadGlobalQuotas(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.GlobalQuotaChallenge != 0 || cfg.GlobalQuotaRegister != 0 || cfg.GlobalQuotaWrite != 0 {
		t.Errorf("global quotas must default disabled, got %+v", cfg)
	}
	cfg, err = load(envFunc(map[string]string{
		keyGlobalQuotaChallenge: "1000",
		keyGlobalQuotaRegister:  "100",
		keyGlobalQuotaWrite:     "2000",
	}))
	if err != nil || cfg.GlobalQuotaChallenge != 1000 || cfg.GlobalQuotaRegister != 100 || cfg.GlobalQuotaWrite != 2000 {
		t.Errorf("explicit global quotas must apply, got %+v, %v", cfg, err)
	}
	for key, bad := range map[string]string{keyGlobalQuotaChallenge: "-1", keyGlobalQuotaRegister: "many", keyGlobalQuotaWrite: "1.5"} {
		if _, err := load(envFunc(map[string]string{key: bad})); err == nil {
			t.Errorf("global quota %s=%q accepted", key, bad)
		} else if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q must name the variable", err)
		}
	}
}

// SEC02-B: the trusted-proxy allowlist defaults to trusting nothing
// and refuses non-CIDR values at startup with the variable named.
func TestLoadTrustedProxies(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TrustedProxies != "" {
		t.Errorf("TrustedProxies = %q, want empty (trust nothing) by default", cfg.TrustedProxies)
	}
	cfg, err = load(envFunc(map[string]string{keyTrustedProxies: "10.0.0.0/8, 192.168.0.0/16"}))
	if err != nil || cfg.TrustedProxies == "" {
		t.Errorf("explicit CIDRs must apply, got %q, %v", cfg.TrustedProxies, err)
	}
	for _, bad := range []string{"10.0.0.1", "private_ranges", "10.0.0.0/33", "10.0.0.0/8, nope"} {
		if _, err := load(envFunc(map[string]string{keyTrustedProxies: bad})); err == nil {
			t.Errorf("CIDR %q accepted", bad)
		} else if !strings.Contains(err.Error(), keyTrustedProxies) {
			t.Errorf("error %q must name the variable", err)
		}
	}
}

func TestMEX19CompanyReadSwitchIsIndependentAndDefaultsOff(t *testing.T) {
	cfg, e := load(envFunc(nil))
	if e != nil || cfg.QualityCompanyReadsEnabled || !cfg.QualityReadsEnabled {
		t.Fatal("source attribution enabled implicitly", e)
	}
	for _, raw := range []string{"true", "false"} {
		cfg, e = load(envFunc(map[string]string{keyQualityCompanyReadsEnabled: raw}))
		if e != nil || cfg.QualityCompanyReadsEnabled != (raw == "true") || !cfg.QualityReadsEnabled {
			t.Fatal("scoped/global switch coupled", e)
		}
	}
	if _, e = load(envFunc(map[string]string{keyQualityCompanyReadsEnabled: "invalid"})); e == nil {
		t.Fatal("invalid company switch admitted")
	}
}
