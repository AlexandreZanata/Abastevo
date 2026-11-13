package apicontract

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func TestDeviceRoutesRequireActualProofPlaneAndBoundSelectors(t *testing.T) {
	doc, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"/v1/orgs/{org_id}/devices/enroll",
		"/v1/orgs/{org_id}/devices/check",
		"/v1/orgs/{org_id}/memberships/{membership_id}/devices/list",
		"/v1/orgs/{org_id}/memberships/{membership_id}/devices/{device_id}/revoke",
		"/v1/orgs/{org_id}/memberships/{membership_id}/devices/self/list",
		"/v1/orgs/{org_id}/memberships/{membership_id}/devices/self/{device_id}/revoke",
	}
	originals := map[string]http.Handler{}
	if err := chi.Walk(router, func(method, path string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost {
			originals[path] = h
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(path+"/unguarded", func(t *testing.T) {
			router.Post(path, func(http.ResponseWriter, *http.Request) {})
			defer router.Method(http.MethodPost, path, originals[path])
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("bare device entry accepted")
			}
		})
	}
	for _, name := range []string{"EnterpriseDeviceView", "EnterpriseDeviceResult", "EnterpriseDevicePage", "EnterpriseDeviceEnrollRequest", "EnterpriseDeviceCheckRequest", "EnterpriseDeviceRevokeRequest", "EnterpriseDeviceListRequest"} {
		schema := doc.Components.Schemas[name].Value
		if err := schema.VisitJSON(enterpriseContractObject(t, schema.Example)); err != nil {
			t.Fatal(name, err)
		}
		for _, field := range schema.Required {
			input := enterpriseContractObject(t, schema.Example)
			delete(input, field)
			if schema.VisitJSON(input) == nil {
				t.Fatal("missing device intent accepted", name, field)
			}
		}
	}
	for _, name := range []string{"EnterpriseDeviceView", "EnterpriseDeviceResult", "EnterpriseDevicePage"} {
		schema := doc.Components.Schemas[name].Value
		for _, field := range []string{"account_id", "session_id", "family_id", "access_token", "contributor_id", "key_id", "fingerprint"} {
			input := enterpriseContractObject(t, schema.Example)
			input[field] = "untrusted"
			if schema.VisitJSON(input) == nil {
				t.Fatal("personal authority exposed", name, field)
			}
		}
	}
	for _, path := range paths[:2] {
		op := doc.Paths.Value(path).Post
		t.Run(path+"/signature", func(t *testing.T) {
			old := op.Parameters
			defer func() { op.Parameters = old }()
			for i, p := range old {
				if p.Value != nil && p.Value.Name == "Signature" {
					op.Parameters = append(append(openapi3.Parameters{}, old[:i]...), old[i+1:]...)
					break
				}
			}
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("signature-free device entry accepted")
			}
		})
		t.Run(path+"/cookie_fallback", func(t *testing.T) {
			old := op.Security
			defer func() { op.Security = old }()
			cookie := openapi3.SecurityRequirements{{"CompanySessionCookie": []string{}}}
			op.Security = &cookie
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("cookie fallback to signed device accepted")
			}
		})
		t.Run(path+"/account_proof", func(t *testing.T) {
			schema := op.RequestBody.Value.Content.Get("application/json").Schema.Value
			old := schema.Required
			defer func() { schema.Required = old }()
			schema.Required = []string{"access_token"}
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("missing body family proof accepted")
			}
		})
	}
	check := doc.Paths.Value(paths[1]).Post
	old := check.Extensions["x-abastevo-signed-device"]
	check.Extensions["x-abastevo-signed-device"] = map[string]any{"action": "check", "account_proof": true, "signed": true, "fresh": false, "enrollment": false, "bootstrap": true, "success": 200}
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("check disguised as an enrollment bootstrap")
	}
	check.Extensions["x-abastevo-signed-device"] = old
	for _, path := range paths[2:] {
		op := doc.Paths.Value(path).Post
		old := op.Parameters
		for i, p := range old {
			if p.Value != nil && p.Value.Name == "membership_id" {
				op.Parameters = append(append(openapi3.Parameters{}, old[:i]...), old[i+1:]...)
				break
			}
		}
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("missing membership selector accepted", path)
		}
		op.Parameters = old
	}
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal("device contract not restored after adversarial checks", err)
	}
}
