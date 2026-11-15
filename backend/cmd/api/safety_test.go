package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	safetyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/adapters"
	safetyhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/adapters/http"
)

// B-BR-SAFE-03: refused credentials inside the transaction are a 401;
// dependency failures stay retryable.
func TestSafetyGuardErrorMapping(t *testing.T) {
	if safetyGuardError(nil) != nil {
		t.Fatal("nil guard error mapped")
	}
	for name, err := range map[string]error{
		"missing binding": pgx.ErrNoRows,
		"inactive":        accountdomain.ErrCodeUnknown,
	} {
		if !errors.Is(safetyGuardError(err), safetyhttp.ErrGuardRefused) {
			t.Fatalf("%s not refused", name)
		}
	}
}

// The five safety routes are mounted and refuse unsigned callers without
// touching the store.
func TestCommunitySafetyRoutesMounted(t *testing.T) {
	r := chi.NewRouter()
	registerCommunitySafety(r, safetyDeps{}, &safetyadapters.Store{})
	var routes []string
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/v1/community/") {
			routes = append(routes, method+" "+route)
		}
		return nil
	})
	sort.Strings(routes)
	want := []string{
		"POST /v1/community/blocks",
		"POST /v1/community/blocks/hidden",
		"POST /v1/community/blocks/list",
		"POST /v1/community/blocks/{block_id}/remove",
		"POST /v1/community/user-reports",
	}
	if strings.Join(routes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("routes = %v", routes)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/community/blocks", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned block = %d", w.Code)
	}
}
