package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestNativeGoogleAudienceControlsProductionLoginRoute(t *testing.T) {
	for _, tc := range []struct {
		name, audience, verdict string
		status                  int
	}{
		{"unset", "", "account.provider-unavailable", http.StatusServiceUnavailable},
		{"configured", "web-test.apps.example.invalid", "account.bad-body", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appleCompositionEnv(t, map[string]string{
				"ANPFUEL_GOOGLE_NATIVE_AUDIENCE":    tc.audience,
				"ANPFUEL_GOOGLE_AUTHORIZED_PARTIES": "",
			})
			stopped := errors.New("google composition checkpoint")
			err := runWithRouterCheck(func(router chi.Router) error {
				// An invalid local token distinguishes enabled native shape
				// validation from disabled login with zero provider or DB IO.
				const body = `{"provider":"google","id_token":"not a token","nonce":"raw-google-composition-proof"}`
				req := httptest.NewRequest(http.MethodPost, "/v1/accounts/providers/login", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.verdict) {
					t.Fatalf("Google native route returned %d, want %d %s", rec.Code, tc.status, tc.verdict)
				}
				return stopped
			})
			if !errors.Is(err, stopped) {
				t.Fatalf("router checkpoint not reached: %v", err)
			}
		})
	}
}
