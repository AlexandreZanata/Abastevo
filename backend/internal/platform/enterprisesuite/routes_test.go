package enterprisesuite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompositionIsDefaultOffAndRequiresCompleteSource(t *testing.T) {
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/org-access/organizations/create", nil))
	if w.Code != 404 {
		t.Fatal("default surface exposed")
	}
	if err := enterprisesuite.Register(chi.NewRouter(), enterprisesuite.Options{Enabled: true, Origin: "https://company.example.invalid", CursorSecret: make([]byte, 32)}); err == nil {
		t.Fatal("missing runtime pool accepted")
	}
	pool, err := pgxpool.New(t.Context(), "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := enterprisesuite.Register(chi.NewRouter(), enterprisesuite.Options{Enabled: true, Origin: "https://company.example.invalid", Pool: pool}); err == nil {
		t.Fatal("missing privacy key accepted")
	}
	if err := enterprisesuite.Register(chi.NewRouter(), enterprisesuite.Options{Enabled: true, Origin: "http://company.example.invalid", CursorSecret: make([]byte, 32), Pool: pool}); err == nil {
		t.Fatal("insecure origin accepted")
	}
}
