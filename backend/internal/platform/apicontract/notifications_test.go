package apicontract

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

// EMP-NOTIF-001 / B-BR-EMP-218: the four own-inbox routes exist in the actual
// composition and the contract, behind the exact SELF_NOTICE boundary.
func TestSelfNoticeRoutesMatchContractAndRejectGuardDrift(t *testing.T) {
	doc, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"/v1/orgs/{org_id}/memberships/{membership_id}/notifications/self/list":              "list",
		"/v1/orgs/{org_id}/memberships/{membership_id}/notifications/self/{notice_id}/read":  "read",
		"/v1/orgs/{org_id}/memberships/{membership_id}/notification-preferences/self/read":   "preferences_read",
		"/v1/orgs/{org_id}/memberships/{membership_id}/notification-preferences/self/update": "preferences_update",
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
	for path, action := range paths {
		if originals[path] == nil {
			t.Fatal("self notice route not composed", path)
		}
		op := doc.Paths.Value(path).Post
		t.Run(action+"/unguarded", func(t *testing.T) {
			router.Post(path, func(http.ResponseWriter, *http.Request) {})
			defer router.Method(http.MethodPost, path, originals[path])
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("bare self notice entry accepted")
			}
		})
		for name, ext := range map[string]map[string]any{
			"fresh":          {"action": action, "cookie": true, "csrf": true, "binding": true, "fresh": true, "success": 200},
			"no binding":     {"action": action, "cookie": true, "csrf": true, "binding": false, "success": 200},
			"other action":   {"action": "list_all", "cookie": true, "csrf": true, "binding": true, "success": 200},
			"no csrf":        {"action": action, "cookie": true, "csrf": false, "binding": true, "success": 200},
			"wrong success":  {"action": action, "cookie": true, "csrf": true, "binding": true, "success": 201},
			"permission tag": {"action": action, "cookie": true, "csrf": true, "binding": true, "success": 200, "permission": "notif.rule.manage"},
		} {
			t.Run(action+"/"+name, func(t *testing.T) {
				old := op.Extensions["x-abastevo-self-notice"]
				defer func() { op.Extensions["x-abastevo-self-notice"] = old }()
				op.Extensions["x-abastevo-self-notice"] = ext
				if ValidateOrganizationRoutes(router, doc) == nil {
					t.Fatal("self notice metadata drift accepted")
				}
			})
		}
		t.Run(action+"/organization permission", func(t *testing.T) {
			defer delete(op.Extensions, "x-abastevo-organization")
			op.Extensions["x-abastevo-organization"] = map[string]any{"permission": "notif.rule.manage", "success": 200}
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("self notice disguised as a permission route")
			}
		})
		t.Run(action+"/csrf", func(t *testing.T) {
			old := op.Parameters
			defer func() { op.Parameters = old }()
			for i, p := range old {
				if p.Value != nil && p.Value.Name == "X-CSRF" {
					op.Parameters = append(append(openapi3.Parameters{}, old[:i]...), old[i+1:]...)
					break
				}
			}
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("CSRF-free self notice accepted")
			}
		})
		t.Run(action+"/membership selector", func(t *testing.T) {
			old := op.Parameters
			defer func() { op.Parameters = old }()
			for i, p := range old {
				if p.Value != nil && p.Value.Name == "membership_id" {
					op.Parameters = append(append(openapi3.Parameters{}, old[:i]...), old[i+1:]...)
					break
				}
			}
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("missing membership selector accepted")
			}
		})
		t.Run(action+"/cache policy", func(t *testing.T) {
			response := op.Responses.Value("200").Value
			old := response.Headers
			defer func() { response.Headers = old }()
			response.Headers = nil
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("cacheable inbox accepted")
			}
		})
	}
	// A self notice descriptor cannot move to another path.
	t.Run("moved route", func(t *testing.T) {
		list := "/v1/orgs/{org_id}/memberships/{membership_id}/notifications/self/list"
		moved := chi.NewRouter()
		for path, h := range originals {
			if path == list {
				moved.Method(http.MethodPost, "/v1/orgs/{org_id}/notifications/self/list", h)
				continue
			}
			moved.Method(http.MethodPost, path, h)
		}
		if ValidateOrganizationRoutes(moved, doc) == nil {
			t.Fatal("self notice route moved off its pinned path")
		}
	})
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal("self notice contract not restored", err)
	}
}

func TestSelfNoticeSchemasCarryOnlyBoundedReferences(t *testing.T) {
	doc := loadSpec(t)
	for _, name := range []string{"EnterpriseNoticeListRequest", "EnterpriseNoticeView", "EnterpriseNoticePage", "EnterpriseNoticeReadRequest", "EnterpriseNoticeReadResult",
		"EnterpriseNoticePreferencesRequest", "EnterpriseNoticePreference", "EnterpriseNoticePreferences", "EnterpriseNoticePreferenceUpdateRequest", "EnterpriseNoticePreferenceResult"} {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil || ref.Value.Example == nil {
			t.Fatal("missing schema or example", name)
		}
		schema := ref.Value
		if err := schema.VisitJSON(enterpriseContractObject(t, schema.Example)); err != nil {
			t.Fatal(name, err)
		}
		for _, field := range schema.Required {
			input := enterpriseContractObject(t, schema.Example)
			delete(input, field)
			if schema.VisitJSON(input) == nil {
				t.Fatal("missing required field accepted", name, field)
			}
		}
		for _, field := range []string{"account_id", "recipients", "deliveries", "body", "title", "attachment", "document", "latitude", "token", "access_token"} {
			input := enterpriseContractObject(t, schema.Example)
			input[field] = "untrusted"
			if schema.VisitJSON(input) == nil {
				t.Fatal("private or free-text field accepted", name, field)
			}
		}
	}
	view := doc.Components.Schemas["EnterpriseNoticeView"].Value
	for name, value := range map[string]any{
		"free text param": map[string]any{"report_id": "Synthetic driver called about the invoice; see attached"},
		"too many params": map[string]any{"a": "1", "b": "1", "c": "1", "d": "1", "e": "1", "f": "1", "g": "1", "h": "1", "i": "1"},
		"nested param":    map[string]any{"report_id": map[string]any{"x": "y"}},
	} {
		input := enterpriseContractObject(t, view.Example)
		input["params"] = value
		if view.VisitJSON(input) == nil {
			t.Fatal("unbounded notice parameter accepted", name)
		}
	}
	for _, link := range []string{"https://example.invalid/reports/x", "/reports/x?token=y", "/reports/../admin", "//example.invalid/x"} {
		input := enterpriseContractObject(t, view.Example)
		input["link_path"] = link
		if view.VisitJSON(input) == nil {
			t.Fatal("capability-like link accepted", link)
		}
	}
}
