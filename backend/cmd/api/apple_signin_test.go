package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func appleCompositionEnv(t *testing.T, extra map[string]string) {
	t.Helper()
	for key, value := range map[string]string{
		"ANPFUEL_ENV": "development", "ANPFUEL_DATABASE_URL": "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable",
		"ANPFUEL_CANONICAL_HOST": "api.example.invalid", "ANPFUEL_METRICS_ADDR": "", "ANPFUEL_MAIL_MODE": "off",
		"ANPFUEL_MAP_DIST_DIR": "", "ANPFUEL_MAP_DIST_PUBLIC_KEYS": "", "ANPFUEL_ENTERPRISE_ENABLED": "false",
		"ANPFUEL_APPLE_NATIVE_AUDIENCE": "", "ANPFUEL_APPLE_TEAM_ID": "", "ANPFUEL_APPLE_KEY_ID": "",
		"ANPFUEL_APPLE_PRIVATE_KEY_FILE": "", "ANPFUEL_APPLE_GRANT_KEY_FILE": "",
	} {
		t.Setenv(key, value)
	}
	for key, value := range extra {
		t.Setenv(key, value)
	}
}

func appleKeyFiles(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p8 := filepath.Join(dir, "AuthKey.p8")
	if err := os.WriteFile(p8, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	grant := filepath.Join(dir, "grant.key")
	if err := os.WriteFile(grant, bytes.Repeat([]byte{5}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	return p8, grant
}

func TestNativeAppleSignInFailsClosedWhenUnconfigured(t *testing.T) {
	appleCompositionEnv(t, nil)
	stopped := errors.New("apple composition checkpoint")
	err := runWithRouterCheck(func(router chi.Router) error {
		body := `{"provider":"apple","id_token":"eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln","nonce":"raw-composition-nonce-1"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/accounts/providers/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "account.provider-unavailable") {
			t.Fatalf("unconfigured native sign-in: %d %s", rec.Code, rec.Body.String())
		}
		return stopped
	})
	if !errors.Is(err, stopped) {
		t.Fatalf("checkpoint not reached: %v", err)
	}
}

func TestAppleRevocationCompositionLoadsOrRefusesStartup(t *testing.T) {
	p8, grant := appleKeyFiles(t)
	configured := map[string]string{
		"ANPFUEL_APPLE_NATIVE_AUDIENCE": "com.anpfuel.app", "ANPFUEL_APPLE_TEAM_ID": "TEAM123456",
		"ANPFUEL_APPLE_KEY_ID": "KEY1234567", "ANPFUEL_APPLE_PRIVATE_KEY_FILE": p8, "ANPFUEL_APPLE_GRANT_KEY_FILE": grant,
	}
	appleCompositionEnv(t, configured)
	stopped := errors.New("apple composition checkpoint")
	err := runWithRouterCheck(func(router chi.Router) error {
		found := false
		_ = chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			found = found || (method == http.MethodPost && route == "/v1/accounts/providers/login")
			return nil
		})
		if !found {
			t.Fatal("native sign-in route not mounted")
		}
		return stopped
	})
	if !errors.Is(err, stopped) {
		t.Fatalf("configured composition must start: %v", err)
	}

	configured["ANPFUEL_APPLE_GRANT_KEY_FILE"] = filepath.Join(filepath.Dir(grant), "absent.key")
	appleCompositionEnv(t, configured)
	reached := false
	err = runWithRouterCheck(func(chi.Router) error { reached = true; return stopped })
	if reached || err == nil || !strings.Contains(err.Error(), "Sign in with Apple credentials unavailable") || strings.Contains(err.Error(), filepath.Dir(grant)) {
		t.Fatalf("unreadable key material must refuse startup without echoing paths: reached=%v err=%v", reached, err)
	}
}
