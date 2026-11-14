package apicontract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

// ValidateOrganizationOperators compares actual operator entries in both
// directions; customer AUTH and administrative credentials remain separate.
func ValidateOrganizationOperators(router chi.Routes, doc *openapi3.T) error {
	if router == nil || doc == nil || doc.Paths == nil || doc.Components == nil {
		return fmt.Errorf("operator contract requires complete inputs")
	}
	seen := map[string]bool{}
	e := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(path, "/v1/admin/organizations/") {
			return nil
		}
		descriptor, ok := handler.(orgapi.AdminOrganizationDescriptor)
		if !ok || method != "POST" {
			return fmt.Errorf("unguarded organization operator route %s", path)
		}
		action := ""
		switch path {
		case "/v1/admin/organizations/{org_id}/suspend":
			action = "admin.organization.suspend"
		case "/v1/admin/organizations/{org_id}/reactivate":
			action = "admin.organization.reactivate"
		default:
			return fmt.Errorf("unknown operator route %s", path)
		}
		expected := orgapi.AdminOrganizationRule{Action: action, Cookie: true, CSRF: true, Fresh: true, RequireRevision: true, RequireReason: true, Success: 200}
		item := doc.Paths.Value(path)
		if item == nil || item.Post == nil || descriptor.AdminOrganizationRule() != expected {
			return fmt.Errorf("missing operator descriptor %s", path)
		}
		op := item.Post
		raw, e := json.Marshal(op.Extensions["x-abastevo-organization-operator"])
		var actual orgapi.AdminOrganizationRule
		if e != nil || json.Unmarshal(raw, &actual) != nil || actual != expected {
			return fmt.Errorf("operator metadata drift %s", path)
		}
		scheme := doc.Components.SecuritySchemes["AdminSessionCookie"]
		if scheme == nil || scheme.Value == nil || scheme.Value.Type != "apiKey" || scheme.Value.In != "cookie" || scheme.Value.Name != "__Host-abv_admin" || op.Security == nil || len(*op.Security) != 1 || len((*op.Security)[0]) != 1 {
			return fmt.Errorf("operator credential drift %s", path)
		}
		if _, ok := (*op.Security)[0]["AdminSessionCookie"]; !ok {
			return fmt.Errorf("operator cookie missing %s", path)
		}
		for _, key := range []string{"org_id", "Origin", "X-Admin-CSRF", "Idempotency-Key"} {
			found := false
			for _, p := range op.Parameters {
				if p.Value != nil && p.Value.Name == key && p.Value.Required && (key == "org_id" && p.Value.In == "path" || key != "org_id" && p.Value.In == "header") {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("operator selector/header drift %s", path)
			}
		}
		if op.Responses == nil {
			return fmt.Errorf("operator responses missing %s", path)
		}
		if op.RequestBody == nil || op.RequestBody.Value == nil || !op.RequestBody.Value.Required || op.RequestBody.Value.Content.Get("application/json") == nil {
			return fmt.Errorf("operator body missing %s", path)
		}
		for _, status := range []string{"200", "400", "401", "403", "404", "409", "413", "422", "503"} {
			response := op.Responses.Value(status)
			if response == nil || response.Value == nil || response.Value.Content.Get("application/json") == nil {
				return fmt.Errorf("operator response missing %s", path)
			}
			cache := response.Value.Headers["Cache-Control"]
			if cache == nil || cache.Value == nil || cache.Value.Schema == nil || cache.Value.Schema.Value == nil || len(cache.Value.Schema.Value.Enum) != 1 || cache.Value.Schema.Value.Enum[0] != "no-store" {
				return fmt.Errorf("operator cache drift %s", path)
			}
			for key := range response.Value.Headers {
				if strings.EqualFold(key, "ETag") {
					return fmt.Errorf("operator ETag drift %s", path)
				}
			}
		}
		seen[path] = true
		return nil
	})
	if e != nil {
		return e
	}
	for path, item := range doc.Paths.Map() {
		if strings.HasPrefix(path, "/v1/admin/organizations/") && (len(item.Operations()) != 1 || item.Post == nil || !seen[path]) {
			return fmt.Errorf("unregistered operator contract %s", path)
		}
	}
	return nil
}
