package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	"github.com/go-chi/chi/v5"
)

// TestCORS_CTB11 allows only the configured origin on the public route.
func TestCORS_CTB11(t *testing.T) {
	handler := PublicHandler{AllowedOrigins: []string{"https://landing.example"}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, tc := range []struct {
		origin string
		allow  bool
	}{
		{"https://landing.example", true},
		{"https://evil.example", false},
		{"", false},
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/contributors", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		handler.cors(next).ServeHTTP(w, req)
		got := w.Header().Get("Access-Control-Allow-Origin")
		if tc.allow && got != tc.origin {
			t.Fatalf("origin %q allow=%q", tc.origin, got)
		}
		if !tc.allow && got != "" {
			t.Fatalf("origin %q unexpectedly allowed", tc.origin)
		}
		if w.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatal("credentials must stay disabled")
		}
	}
}

// TestAdminRoutes_CTB_RequireSession mounts contributor admin routes behind
// the shared admin session middleware.
func TestAdminRoutes_CTB_RequireSession(t *testing.T) {
	admin := &adminhttp.Handler{
		Service: &adminapp.Service{Keys: []admindomain.ConfiguredKey{{Hash: strings.Repeat("a", 64), Fingerprint: strings.Repeat("a", 12)}}},
		Origin:  "https://admin.example",
	}
	router := chi.NewRouter()
	router.Group(func(g chi.Router) {
		g.Use(admin.AdminMiddleware()...)
		g.Get("/v1/admin/github/contributors", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		g.Get("/v1/admin/contributor-claims", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	for _, path := range []string{"/v1/admin/github/contributors", "/v1/admin/contributor-claims"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer account-access-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}
}

// TestUserRoutes_CTB_RequireSession refuses missing account proof.
func TestUserRoutes_CTB_RequireSession(t *testing.T) {
	router := chi.NewRouter()
	UserHandler{}.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/v1/contributor/claims", strings.NewReader(`{"family_id":"11111111-1111-4111-8111-111111111111","access_token":"token","github_login":"octocat","operation_id":"22222222-2222-4222-8222-222222222222"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

// TestServiceError_CTB maps claim/profile failures to stable codes.
func TestServiceError_CTB(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		body string
	}{
		{domain.ErrInvalid, http.StatusBadRequest, "contributor.invalid"},
		{domain.ErrForbidden, http.StatusForbidden, "contributor.forbidden"},
		{domain.ErrNotFound, http.StatusNotFound, "contributor.not-found"},
		{domain.ErrConflict, http.StatusConflict, "contributor.conflict"},
		{domain.ErrQuota, http.StatusTooManyRequests, "contributor.quota"},
		{domain.ErrOverrideRequired, http.StatusConflict, "contributor.override-required"},
		{domain.ErrClaimState, http.StatusConflict, "contributor.claim-state"},
		{domain.ErrUnavailable, http.StatusServiceUnavailable, "contributor.unavailable"},
		// Admin mutation envelope errors keep the admin codes (ADM-12).
		{admindomain.ErrConflict, http.StatusConflict, "admin.conflict"},
		{admindomain.ErrInvalid, http.StatusBadRequest, "admin.invalid"},
		{admindomain.ErrUnauthorized, http.StatusUnauthorized, "admin.denied"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/contributor/claims", nil)
		writeServiceError(w, r, tc.err)
		if w.Code != tc.code {
			t.Fatalf("err=%v status=%d", tc.err, w.Code)
		}
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || payload.Error.Code != tc.body {
			t.Fatalf("err=%v body=%s", tc.err, w.Body.String())
		}
	}
}
