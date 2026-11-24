package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func TestAccountAuthorizationInventoryCoversProductionRouter(t *testing.T) {
	appleCompositionEnv(t, map[string]string{
		"ANPFUEL_GOOGLE_NATIVE_AUDIENCE":    "",
		"ANPFUEL_GOOGLE_AUTHORIZED_PARTIES": "",
	})
	spec, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "contracts", "openapi", "v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	type boundary string
	const (
		anonymous boundary = "anonymous"
		retired   boundary = "retired"
		session   boundary = "session"
		fresh     boundary = "fresh"
	)
	routes := map[string]boundary{
		"email/codes": anonymous, "email/consume": retired, "keys": anonymous,
		"keys/login": anonymous, "keys/signup": anonymous, "recovery": anonymous,
		"sessions/refresh": anonymous, "sessions/revoke": session,
		"profile": session, "profile/name": session, "recovery-email": fresh,
		"providers/login": anonymous, "providers/link": session, "providers/link-native": fresh,
		"providers/unlink": session, "providers/list": session, "deletion": session,
		"bindings/bind": session, "bindings/unbind": session, "bindings/list": session,
	}
	stopped := errors.New("account route composition checkpoint")
	err = runWithRouterCheck(func(router chi.Router) error {
		seen := make(map[string]bool, len(routes))
		if err := chi.Walk(router, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			route, account := strings.CutPrefix(path, "/v1/accounts/")
			if !account {
				return nil
			}
			kind, ok := routes[route]
			if !ok || seen[route] || method != http.MethodPost {
				t.Errorf("unexpected account route: %s %s", method, path)
				return nil
			}
			seen[route] = true
			item := spec.Paths.Find(path)
			if item == nil || item.Post == nil {
				t.Errorf("OpenAPI missing POST %s", path)
				return nil
			}
			if kind == retired || kind == session || kind == fresh {
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				want := http.StatusBadRequest
				if kind == retired {
					want = http.StatusGone
					if !strings.Contains(rec.Body.String(), "account.email-login-retired") {
						t.Errorf("retired endpoint returned %s", rec.Body.String())
					}
				}
				if rec.Code != want || item.Post.Responses.Status(want) == nil {
					t.Errorf("%s without proof returned %d, want documented %d", path, rec.Code, want)
				}
			}
			return nil
		}); err != nil {
			return err
		}
		for route := range routes {
			if !seen[route] {
				t.Errorf("missing production account route %s", route)
			}
		}
		return stopped
	})
	if !errors.Is(err, stopped) {
		t.Fatalf("check the production composition: %v", err)
	}
}
