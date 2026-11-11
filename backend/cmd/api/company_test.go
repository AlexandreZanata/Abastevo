package main

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/apicontract"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	handler.RegisterRoutes(router)
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
