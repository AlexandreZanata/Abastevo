package apicontract

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestEnterpriseConventionSchemaVectors(t *testing.T) {
	document := loadSpec(t)
	if document.Extensions["x-abastevo-organization-api"] == nil {
		t.Fatal("missing organization API section")
	}
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "testdata/enterprise/api-conventions-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Vectors []struct {
			Name, Schema string
			Valid        bool
			Value        any
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	for _, vector := range data.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			schema := document.Components.Schemas[vector.Schema]
			if schema == nil || schema.Value == nil {
				t.Fatal("missing enterprise schema")
			}
			err := schema.Value.VisitJSON(vector.Value)
			if vector.Valid != (err == nil) {
				t.Fatalf("schema validity mismatch: %v", err)
			}
		})
	}
}

func conventionRoute(t *testing.T) (*openapi3.T, chi.Router) {
	t.Helper()
	document := loadSpec(t)
	path := "/v1/orgs/{org_id}/example/read"
	rule := orgapi.Rule{Permission: "org.settings.read", Success: 200}
	responses := openapi3.NewResponses()
	for _, code := range []string{"200", "400", "401", "403", "404", "409", "422", "429", "503"} {
		responses.Set(code, document.Components.Responses["EnterprisePrivateError"])
	}
	operation := &openapi3.Operation{
		OperationID: "testOrganizationRead", Extensions: map[string]any{"x-abastevo-organization": rule},
		Parameters: openapi3.Parameters{document.Components.Parameters["EnterpriseOrgID"]},
		RequestBody: &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{Required: true, Content: openapi3.Content{
			"application/json": &openapi3.MediaType{Schema: document.Components.Schemas["EnterpriseReadRequest"]},
		}}},
		Responses: responses,
	}
	document.Paths.Set(path, &openapi3.PathItem{Post: operation})
	router := chi.NewRouter()
	mountEnterprise(t, router)
	err := orgapi.Register(router, path, orgapi.Gateway{
		Enabled: true,
		Origin:  "https://api.example.invalid", CursorSecret: make([]byte, 32),
		WebSession:        func(*http.Request) (string, error) { return "", orgapi.Unauthenticated },
		WebSessionConfirm: func(*http.Request, bool) (string, error) { return "", orgapi.Unauthenticated },
		Resolve: func(context.Context, orgapi.Actor, kernel.OrgScope, orgapi.Rule) (orgapi.Membership, error) {
			return orgapi.Membership{}, orgapi.NotFound
		},
	}, rule, func(context.Context, orgapi.Principal, orgapi.Session) (any, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	return document, router
}

func TestOrganizationRouteGateRejectsBothKindsOfDrift(t *testing.T) {
	document, router := conventionRoute(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		name  string
		apply func(*openapi3.T, chi.Router)
	}{
		{"undocumented", func(d *openapi3.T, _ chi.Router) { d.Paths.Delete("/v1/orgs/{org_id}/example/read") }},
		{"unregistered", func(d *openapi3.T, _ chi.Router) {
			d.Paths.Set("/v1/orgs/{org_id}/other", d.Paths.Value("/v1/orgs/{org_id}/example/read"))
		}},
		{"permission mismatch", func(d *openapi3.T, _ chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/example/read").Post.Extensions["x-abastevo-organization"] = orgapi.Rule{Permission: "org.settings.manage", Success: 200}
		}},
		{"missing scope", func(d *openapi3.T, _ chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/example/read").Post.Parameters = nil
		}},
		{"missing body", func(d *openapi3.T, _ chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/example/read").Post.RequestBody = nil
		}},
		{"missing error", func(d *openapi3.T, _ chi.Router) {
			d.Paths.Value("/v1/orgs/{org_id}/example/read").Post.Responses.Delete("401")
		}},
		{"bare handler", func(_ *openapi3.T, r chi.Router) {
			r.Post("/v1/orgs/{org_id}/unguarded", func(http.ResponseWriter, *http.Request) {})
		}},
		{"etag", func(d *openapi3.T, _ chi.Router) {
			d.Components.Responses["EnterprisePrivateError"].Value.Headers["ETag"] = &openapi3.HeaderRef{Value: &openapi3.Header{}}
		}},
		{"cacheable response", func(d *openapi3.T, _ chi.Router) {
			d.Components.Headers["EnterpriseNoStore"].Value.Schema.Value.Enum = []any{"no-store", "public"}
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			d, r := conventionRoute(t)
			change.apply(d, r)
			if err := ValidateOrganizationRoutes(r, d); err == nil {
				t.Fatal("organization route drift passed")
			}
		})
	}
	// The company lifecycle is now implemented; no organization business
	// operation is published. All three real lifecycle registrations are checked.
	canonical, actual := companyRoutes(t)
	if err := ValidateOrganizationRoutes(actual, canonical); err != nil {
		t.Fatal(err)
	}
}
