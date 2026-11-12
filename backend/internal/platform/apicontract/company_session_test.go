package apicontract

import (
	"encoding/json"
	web "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"testing"
)

func companyRoutes(t *testing.T) (*openapi3.T, chi.Router) {
	t.Helper()
	document := loadSpec(t)
	router := chi.NewRouter()
	mountEnterprise(t, router)

	return document, router
}
func TestActualCompanyRoutesMatchContractAndRejectGuardDrift(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		name string
		run  func(*openapi3.T, chi.Router)
	}{
		{"creation fresh metadata", func(d *openapi3.T, r chi.Router) {
			d.Paths.Value("/v1/org-access/organizations/create").Post.Extensions["x-abastevo-organization-creation"] = map[string]any{"cookie": true, "csrf": true, "success": 201}
		}},
		{"unguarded creation", func(d *openapi3.T, r chi.Router) {
			r.Post("/v1/org-access/organizations/create", func(http.ResponseWriter, *http.Request) {})
		}},
		{"activation cookie security", func(d *openapi3.T, r chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/organization/activate").Post.Security = nil
		}},
		{"activation csrf", func(d *openapi3.T, r chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/organization/activate").Post.Parameters = nil
		}},
		{"activation metadata", func(d *openapi3.T, r chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/organization/activate").Post.Extensions["x-abastevo-organization"] = map[string]any{"permission": "org.settings.manage", "success": 200}
		}},
		{"missing operation", func(d *openapi3.T, r chi.Router) { d.Paths.Delete(web.PathPrefix + "login") }},
		{"unguarded", func(d *openapi3.T, r chi.Router) {
			r.Post(web.PathPrefix+"read", func(http.ResponseWriter, *http.Request) {})
		}},
		{"missing csrf", func(d *openapi3.T, r chi.Router) { d.Paths.Value(web.PathPrefix + "read").Post.Parameters = nil }},
		{"missing cookie declaration", func(d *openapi3.T, r chi.Router) { d.Paths.Value(web.PathPrefix + "read").Post.Security = nil }},
		{"wrong cookie plane", func(d *openapi3.T, r chi.Router) {
			d.Components.SecuritySchemes["CompanySessionCookie"].Value.Name = "__Host-abv_admin"
		}},
		{"missing fresh metadata", func(d *openapi3.T, r chi.Router) {
			d.Paths.Value(web.PathPrefix + "login").Post.Extensions["x-abastevo-company-session"] = map[string]any{"action": "login", "success": 200}
		}},
		{"missing cache policy", func(d *openapi3.T, r chi.Router) {
			d.Paths.Value(web.PathPrefix + "read").Post.Responses.Value("200").Value.Headers = nil
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			d, r := companyRoutes(t)
			change.run(d, r)
			if err := ValidateOrganizationRoutes(r, d); err == nil {
				t.Fatal("company contract drift accepted")
			}
		})
	}
	for _, schema := range []string{"CompanySessionLoginRequest", "CompanySessionEmptyRequest", "CompanySessionLoginResult", "CompanySessionView", "CompanySessionLogoutResult"} {
		if document.Components.Schemas[schema] == nil {
			t.Fatal("missing company schema")
		}
	}
}

func mountEnterprise(t *testing.T, router chi.Router) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://synthetic@127.0.0.1:1/synthetic?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: "https://company.example.invalid", CursorSecret: make([]byte, 32), Pool: pool}); err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseOnboardingSchemasAndExamples(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseOrganizationCreationRequest", "EnterpriseOrganizationActivationRequest", "EnterpriseOrganizationCreationResult", "EnterpriseOrganizationActivationResult"} {
		schema := document.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing setup schema/example", name)
		}
		raw, err := json.Marshal(schema.Value.Example)
		if err != nil {
			t.Fatal(err)
		}
		var example any
		if err := json.Unmarshal(raw, &example); err != nil {
			t.Fatal(err)
		}
		if err := schema.Value.VisitJSON(example); err != nil {
			t.Fatal("invalid setup example", name, err)
		}
		fields := example.(map[string]any)
		fields["account_id"] = "7b0e2a54-0000-4000-8000-000000000001"
		if err := schema.Value.VisitJSON(fields); err == nil {
			t.Fatal("extra account authority accepted", name)
		}
	}
}
