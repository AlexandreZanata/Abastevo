package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/go-chi/chi/v5"
)

// TestAdminRoutes_ADM01_UnconfiguredReturns404 is S1.
func TestAdminRoutes_ADM01_UnconfiguredReturns404(t *testing.T) {
	router := chi.NewRouter()
	if registerAdminRoutes(router, config.Config{}, nil, nil) {
		t.Fatal("admin routes mounted without a key hash")
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/admin/session"},
		{http.MethodGet, "/v1/admin/session"},
		{http.MethodDelete, "/v1/admin/session"},
		{http.MethodPost, "/v1/admin/sessions/revoke-all"},
		{http.MethodGet, "/v1/admin/accounts"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d", tc.method, tc.path, w.Code)
		}
	}
}
