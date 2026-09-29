package config

import (
	"strings"
	"testing"
)

func r2Vars(vars map[string]string) map[string]string {
	return vars
}

func TestR2UnsetDisablesStorage(t *testing.T) {
	cfg, err := load(envFunc(r2Vars(map[string]string{})))
	if err != nil {
		t.Fatalf("load = %v", err)
	}
	if cfg.R2 != nil {
		t.Errorf("R2 = %+v, want disabled", cfg.R2)
	}
}

func TestR2FullConfigParses(t *testing.T) {
	cfg, err := load(envFunc(r2Vars(map[string]string{
		keyR2Endpoint: "https://xxx.r2.cloudflarestorage.com",
		keyR2Bucket:   "anpfuel-quarantine",
		keyR2Access:   "access-id",
		keyR2Secret:   "secret-value-32-bytes-long-placeholder",
	})))
	if err != nil {
		t.Fatalf("load = %v", err)
	}
	if cfg.R2 == nil {
		t.Fatal("R2 missing")
	}
	if cfg.R2.Region != "auto" || cfg.R2.Bucket != "anpfuel-quarantine" {
		t.Errorf("R2 = %+v", cfg.R2)
	}
	if cfg.R2.SecretAccessKey != "secret-value-32-bytes-long-placeholder" {
		t.Error("secret not retained")
	}
}

func TestR2LoopbackHTTPAllowed(t *testing.T) {
	_, err := load(envFunc(r2Vars(map[string]string{
		keyR2Endpoint: "http://127.0.0.1:9000",
		keyR2Bucket:   "anpfuel-quarantine",
		keyR2Access:   "minioadmin",
		keyR2Secret:   "minioadmin-secret-value-long-enough",
	})))
	if err != nil {
		t.Errorf("loopback endpoint rejected: %v", err)
	}
}

func TestR2PartialAndInsecureRefuse(t *testing.T) {
	secret := "super-secret-value-never-logged"
	cases := map[string]map[string]string{
		"endpoint-only": {
			keyR2Endpoint: "https://xxx.r2.cloudflarestorage.com",
		},
		"missing-secret": {
			keyR2Endpoint: "https://xxx.r2.cloudflarestorage.com",
			keyR2Bucket:   "anpfuel-quarantine",
			keyR2Access:   "access-id",
		},
		"plain-http": {
			keyR2Endpoint: "http://objects.example.com",
			keyR2Bucket:   "anpfuel-quarantine",
			keyR2Access:   "access-id",
			keyR2Secret:   secret,
		},
		"credentials-in-url": {
			keyR2Endpoint: "https://user:pass@xxx.r2.cloudflarestorage.com",
			keyR2Bucket:   "anpfuel-quarantine",
			keyR2Access:   "access-id",
			keyR2Secret:   secret,
		},
	}
	for name, vars := range cases {
		if name == "plain-http" || name == "credentials-in-url" {
			vars[keyR2Secret] = secret
		}
		_, err := load(envFunc(vars))
		if err == nil {
			t.Errorf("%s accepted", name)
			continue
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("%s error leaks secret: %v", name, err)
		}
	}
}

func TestR2LogValueHidesSecret(t *testing.T) {
	cfg, err := load(envFunc(r2Vars(map[string]string{
		keyR2Endpoint: "https://xxx.r2.cloudflarestorage.com",
		keyR2Bucket:   "anpfuel-quarantine",
		keyR2Access:   "access-id",
		keyR2Secret:   "super-secret-value-never-logged",
	})))
	if err != nil {
		t.Fatal(err)
	}
	rendered := cfg.LogValue().String()
	if strings.Contains(rendered, "super-secret-value-never-logged") {
		t.Errorf("LogValue leaks secret: %s", rendered)
	}
}
