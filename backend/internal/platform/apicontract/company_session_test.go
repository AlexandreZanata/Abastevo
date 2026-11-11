package apicontract

import (
	web "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"net/http"
	"testing"
)

func companyRoutes(t *testing.T) (*openapi3.T, chi.Router) {
	t.Helper()
	document := loadSpec(t)
	router := chi.NewRouter()
	handler, err := web.New(&app.Service{Enabled: true}, "https://company.example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	handler.RegisterRoutes(router)
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
