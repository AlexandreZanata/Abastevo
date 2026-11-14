package apicontract

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func TestDeviceContactRequiresExactSignedOwnedBoundary(t *testing.T) {
	doc, router := companyRoutes(t)
	const path = "/v1/orgs/{org_id}/devices/contact"
	if e := ValidateOrganizationRoutes(router, doc); e != nil {
		t.Fatal(e)
	}
	var original http.Handler
	if e := chi.Walk(router, func(method, p string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
		if p == path && method == http.MethodPost {
			original = h
		}
		return nil
	}); e != nil || original == nil {
		t.Fatal("missing contact route", e)
	}
	router.Post(path, func(http.ResponseWriter, *http.Request) {})
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unguarded contact accepted")
	}
	router.Method(http.MethodPost, path, original)
	op := doc.Paths.Value(path).Post
	security := op.Security
	cookie := openapi3.SecurityRequirements{{"CompanySessionCookie": []string{}}}
	op.Security = &cookie
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("cookie contact fallback accepted")
	}
	op.Security = security
	parameters := op.Parameters
	op.Parameters = nil
	for _, p := range parameters {
		if p.Value == nil || p.Value.Name != "Signature" {
			op.Parameters = append(op.Parameters, p)
		}
	}
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unsigned custody observation accepted")
	}
	op.Parameters = parameters
	old := op.Extensions["x-abastevo-device-contact"]
	op.Extensions["x-abastevo-device-contact"] = map[string]any{"account_proof": true, "signed": true, "exact_enrollment": false, "terminal_read": true, "policy_lease": true, "success": 200}
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unbound terminal contact accepted")
	}
	op.Extensions["x-abastevo-device-contact"] = old
	schema := op.RequestBody.Value.Content.Get("application/json").Schema.Value
	for _, field := range []string{"schema_version", "request_id", "membership_id", "enrollment_id", "family_id", "access_token"} {
		old := schema.Required
		schema.Required = nil
		for _, k := range old {
			if k != field {
				schema.Required = append(schema.Required, k)
			}
		}
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("unbound contact selector", field)
		}
		schema.Required = old
	}
	for _, name := range []string{"EnterpriseDeviceContactRequest", "EnterpriseDeviceContactActive", "EnterpriseDeviceContactLock", "EnterpriseDeviceContactErase"} {
		s := doc.Components.Schemas[name].Value
		ex := enterpriseContractObject(t, s.Example)
		if e := s.VisitJSON(ex); e != nil {
			t.Fatal("invalid contact example", name, e)
		}
		for _, key := range s.Required {
			x := enterpriseContractObject(t, s.Example)
			delete(x, key)
			if s.VisitJSON(x) == nil {
				t.Fatal("missing exact contact field", name, key)
			}
		}
	}
	for _, name := range []string{"EnterpriseDeviceContactLock", "EnterpriseDeviceContactErase"} {
		s := doc.Components.Schemas[name].Value
		for _, key := range []string{"access_model", "offline_validity_seconds", "managed_unlock_seconds", "access_token", "key_id", "label"} {
			x := enterpriseContractObject(t, s.Example)
			x[key] = "untrusted"
			if s.VisitJSON(x) == nil {
				t.Fatal("terminal expanded into lease/businessdata", name, key)
			}
		}
	}
}
