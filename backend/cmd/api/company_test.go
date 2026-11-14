package main

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/apicontract"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompanyProductionCompositionIsOptInAndContractChecked(t *testing.T) {
	handler, err := newCompanyHandler(config.Config{}, nil, nil)
	if err != nil || handler != nil {
		t.Fatal("company session mounted by default")
	}
	handler, err = newCompanyHandler(config.Config{EnterpriseEnabled: true, CanonicalHost: "company.example.invalid"}, nil, nil)
	if err != nil || handler == nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	pool, err := pgxpool.New(t.Context(), "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: "https://company.example.invalid", CursorSecret: make([]byte, 32), Pool: pool}); err != nil {
		t.Fatal(err)
	}
	loader := openapi3.NewLoader()
	path, err := filepath.Abs("../../../contracts/openapi/v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	document, err := loader.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := apicontract.ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal("production company registration drift", err)
	}
	r := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/org-access/session/read", nil)
	r.Header.Set("Origin", "https://company.example.invalid")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code == 200 {
		t.Fatal("unwired session yielded authority")
	}
}

func TestProductionOrganizationOperatorContractAndDefaultOff(t *testing.T) {
	pool, e := pgxpool.New(t.Context(), "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	cfg := config.Config{EnterpriseEnabled: true, AdminOrigin: "https://company.example.invalid", AdminKeyHashes: []string{strings.Repeat("a", 64)}}
	admin, ok := newAdminHandler(cfg, pool, nil)
	if !ok {
		t.Fatal("production admin constructor missing")
	}
	router := chi.NewRouter()
	if e := enterprisesuite.RegisterAdmin(router, enterprisesuite.Options{Enabled: cfg.EnterpriseEnabled, Pool: pool}, admin); e != nil {
		t.Fatal(e)
	}
	loader := openapi3.NewLoader()
	path, e := filepath.Abs("../../../contracts/openapi/v1.yaml")
	if e != nil {
		t.Fatal(e)
	}
	doc, e := loader.LoadFromFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e := apicontract.ValidateOrganizationOperators(router, doc); e != nil {
		t.Fatal("production operator drift", e)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "https://company.example.invalid/v1/admin/organizations/7b0e2a54-0000-4000-8000-000000000301/suspend", nil))
	if w.Code != 401 {
		t.Fatal("operator cookie not required")
	}
	off := chi.NewRouter()
	if e := enterprisesuite.RegisterAdmin(off, enterprisesuite.Options{Pool: pool}, admin); e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	off.ServeHTTP(w, httptest.NewRequest("POST", "/v1/admin/organizations/7b0e2a54-0000-4000-8000-000000000301/suspend", nil))
	if w.Code != 404 {
		t.Fatal("default operator plane exposed")
	}
}
