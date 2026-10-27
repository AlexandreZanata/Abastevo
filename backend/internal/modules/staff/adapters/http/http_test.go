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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/domain"
	"github.com/go-chi/chi/v5"
)

// TestCatalog_STF01_NoMutationRoutes proves no endpoint can modify the role
// catalog; only GET /v1/admin/roles exists.
func TestCatalog_STF01_NoMutationRoutes(t *testing.T) {
	router := chi.NewRouter()
	AdminHandler{Service: &application.Service{}}.RegisterRoutes(router)
	methods := map[string][]string{}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		methods[route] = append(methods[route], method)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := methods["/v1/admin/roles"]
	if len(got) != 1 || got[0] != http.MethodGet {
		t.Fatalf("catalog methods=%v", got)
	}
	for route, values := range methods {
		if strings.HasPrefix(route, "/v1/admin/roles") && route != "/v1/admin/roles" {
			t.Fatalf("unexpected catalog route %s %v", route, values)
		}
	}
}

// TestCatalog_STF01_Response returns the code-frozen catalog.
func TestCatalog_STF01_Response(t *testing.T) {
	router := chi.NewRouter()
	AdminHandler{Service: &application.Service{}}.RegisterRoutes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/admin/roles", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var payload struct {
		Version string            `json:"catalog_version"`
		Roles   []domain.RoleInfo `json:"roles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || payload.Version != domain.CatalogVersion || len(payload.Roles) != 4 {
		t.Fatalf("payload=%s err=%v", w.Body.String(), err)
	}
}

// TestRoles_STF04_DoNotAuthorizeAdmin keeps every role outside the admin auth
// path.
func TestRoles_STF04_DoNotAuthorizeAdmin(t *testing.T) {
	admin := &adminhttp.Handler{
		Service: &adminapp.Service{Keys: []admindomain.ConfiguredKey{{Hash: strings.Repeat("a", 64), Fingerprint: strings.Repeat("a", 12)}}},
		Origin:  "https://admin.example",
	}
	router := chi.NewRouter()
	router.Group(func(g chi.Router) {
		g.Use(admin.AdminMiddleware()...)
		g.Get("/v1/admin/roles", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	for _, role := range domain.Catalog() {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/roles", nil)
		req.Header.Set("X-Staff-Role", role.Code)
		req.Header.Set("Authorization", "Bearer account-access-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("role %s authorized admin: %d", role.Code, w.Code)
		}
	}
}

// TestServiceError_STF maps assignment failures to stable codes.
func TestServiceError_STF(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		body string
	}{
		{domain.ErrInvalid, http.StatusBadRequest, "staff.invalid"},
		{domain.ErrIneligible, http.StatusConflict, "staff.account-ineligible"},
		{domain.ErrConflict, http.StatusConflict, "staff.exists"},
		{domain.ErrAlreadyRevoked, http.StatusConflict, "staff.already-revoked"},
		{domain.ErrNotFound, http.StatusNotFound, "staff.not-found"},
		{domain.ErrUnavailable, http.StatusServiceUnavailable, "staff.unavailable"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/admin/accounts/x/roles", nil)
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
