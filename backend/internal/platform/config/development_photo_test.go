package config

import (
	"testing"
	"time"
)

func TestDevelopmentPhotoPreviewConfiguration(t *testing.T) {
	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	for _, tc := range []struct {
		env, host, deadline string
		allowed             bool
	}{
		{"development", "localhost", until, true},
		{"staging", "teste.abastevo.com.br", until, true},
		{"production", "teste.abastevo.com.br", until, false},
		{"staging", "api.abastevo.com.br", until, false},
		{"development", "localhost", "invalid", false},
		{"development", "localhost", time.Now().Add(8 * 24 * time.Hour).Format(time.RFC3339), false},
	} {
		cfg, err := load(envFunc(map[string]string{keyEnv: tc.env, keyCanonicalHost: tc.host,
			keyDatabaseURL:                    "postgres://test:test@localhost:5434/test?sslmode=disable",
			"ANPFUEL_DEV_PHOTO_PREVIEW_UNTIL": tc.deadline}))
		if (err == nil) != tc.allowed {
			t.Fatalf("env=%s host=%s allowed=%v err=%v", tc.env, tc.host, tc.allowed, err)
		}
		if tc.allowed && cfg.DevelopmentPhotoPreviewUntil.IsZero() {
			t.Fatal("missing deadline")
		}
	}
	cfg, err := load(envFunc(nil))
	if err != nil || !cfg.DevelopmentPhotoPreviewUntil.IsZero() {
		t.Fatal("must default off")
	}
}
