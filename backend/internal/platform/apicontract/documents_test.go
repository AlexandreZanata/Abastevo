package apicontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestDocumentContractExamplesRefuseAuthorityAndUnenabledFormats(t *testing.T) {
	doc := loadSpec(t)
	for _, name := range []string{"EnterpriseDocumentOriginal", "EnterpriseDocumentCreateRequest", "EnterpriseDocumentView", "EnterpriseDocumentContentAccessRequest", "EnterpriseDocumentContentAccess", "EnterpriseDocumentContentReadRequest", "EnterpriseDocumentContent"} {
		t.Run(name, func(t *testing.T) {
			schema := doc.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing document contract/example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid document contract object")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid example", err)
			}
			for _, field := range []string{"account_id", "membership_id", "storage_key", "url", "file_name"} {
				example[field] = "synthetic"
				if schema.Value.VisitJSON(example) == nil {
					t.Fatal("caller-selected authority or storage locator accepted", field)
				}
				delete(example, field)
			}
		})
	}
	create := doc.Components.Schemas["EnterpriseDocumentCreateRequest"].Value
	for name, change := range map[string]func(map[string]any){
		"png":          func(v map[string]any) { v["original"].(map[string]any)["media_type"] = "image/png" },
		"pdf":          func(v map[string]any) { v["original"].(map[string]any)["media_type"] = "application/pdf" },
		"over-budget":  func(v map[string]any) { v["original"].(map[string]any)["bytes"] = 5<<20 + 1 },
		"empty":        func(v map[string]any) { v["original"].(map[string]any)["bytes"] = 0 },
		"upper-digest": func(v map[string]any) { v["original"].(map[string]any)["sha256"] = strings.Repeat("A", 64) },
		"kind":         func(v map[string]any) { v["kind"] = "INVOICE" },
		"schema":       func(v map[string]any) { v["schema_version"] = 2 },
		"nil-document": func(v map[string]any) { v["document_id"] = "00000000-0000-0000-0000-000000000000" },
		"stored-state": func(v map[string]any) { v["storage"] = "STORED" },
	} {
		raw, _ := json.Marshal(create.Example)
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		change(v)
		if create.VisitJSON(v) == nil {
			t.Fatal("document declaration contract admitted", name)
		}
	}
}

func TestDocumentIngressContractIsTheOnlyCapabilityBinaryBoundary(t *testing.T) {
	doc := loadSpec(t)
	ingress := doc.Paths.Value("/v1/orgs/{org_id}/documents/{document_id}/content/upload").Post
	rule := orgapi.DocumentIngressRule{Capability: true, MemberBound: true, CurrentAuthority: true, VerifiedBytes: true, Permission: "doc.document.attach", Success: 200}
	if err := validateDocumentIngress(ingress, "/v1/orgs/{org_id}/documents/{document_id}/content/upload", rule, false); err != nil {
		t.Fatal(err)
	}
	clone := func() *openapi3.Operation {
		raw, err := json.Marshal(ingress)
		if err != nil {
			t.Fatal(err)
		}
		var copy openapi3.Operation
		if err := json.Unmarshal(raw, &copy); err != nil {
			t.Fatal(err)
		}
		return &copy
	}
	for name, mutate := range map[string]func(*openapi3.Operation) (orgapi.DocumentIngressRule, string, bool){
		"other-guard": func(*openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) { return rule, "", true },
		"other-path": func(*openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			return rule, "/v1/orgs/{org_id}/documents/{document_id}/content/read", false
		},
		"content-permission": func(*openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			changed := rule
			changed.Permission = "doc.document.view_content"
			return changed, "", false
		},
		"metadata-drift": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			o.Extensions["x-abastevo-document-ingress"] = map[string]any{"capability": true, "member_bound": false, "current_authority": true, "verified_bytes": true, "permission": "doc.document.attach", "success": 200}
			return rule, "", false
		},
		"organization-metadata": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			o.Extensions["x-abastevo-organization"] = map[string]any{"permission": "doc.document.attach"}
			return rule, "", false
		},
		"cookie-fallback": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			o.Security = &openapi3.SecurityRequirements{{"CompanySessionCookie": {}}}
			return rule, "", false
		},
		"json-body": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			o.RequestBody.Value.Content["application/json"] = o.RequestBody.Value.Content["image/jpeg"]
			return rule, "", false
		},
		"unbounded-body": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			o.RequestBody.Value.Content["image/jpeg"].Schema.Value.MaxLength = nil
			return rule, "", false
		},
		"query-capability": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			o.Parameters = append(o.Parameters, &openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "token", In: "query", Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}}})
			return rule, "", false
		},
		"optional-capability": func(o *openapi3.Operation) (orgapi.DocumentIngressRule, string, bool) {
			for _, p := range o.Parameters {
				if p.Value != nil && p.Value.Name == orgapi.DocumentCapabilityHeader {
					p.Value.Required = false
				}
			}
			return rule, "", false
		},
	} {
		t.Run(name, func(t *testing.T) {
			operation := clone()
			changed, path, other := mutate(operation)
			if path == "" {
				path = "/v1/orgs/{org_id}/documents/{document_id}/content/upload"
			}
			if validateDocumentIngress(operation, path, changed, other) == nil {
				t.Fatal("document ingress drift accepted")
			}
		})
	}
	for _, path := range []string{"/v1/orgs/{org_id}/documents/uploads/create", "/v1/orgs/{org_id}/documents/{document_id}/content/access", "/v1/orgs/{org_id}/documents/{document_id}/content/read"} {
		operation := doc.Paths.Value(path).Post
		if operation.Extensions["x-abastevo-document-ingress"] != nil || operation.RequestBody.Value.Content.Get("image/jpeg") != nil {
			t.Fatal("a guarded document route accepts the ingress body or metadata", path)
		}
	}
}

func TestDocumentLinkContractRefusesAuthorityAmountsContentAndUnregisteredKinds(t *testing.T) {
	doc := loadSpec(t)
	for _, name := range []string{"EnterpriseDocumentLinkTarget", "EnterpriseDocumentLinkCreateRequest", "EnterpriseDocumentLinkListRequest", "EnterpriseDocumentLinkByTargetRequest", "EnterpriseDocumentLink", "EnterpriseDocumentLinked", "EnterpriseDocumentLinkPage"} {
		t.Run(name, func(t *testing.T) {
			schema := doc.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing document link contract/example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid document link contract object")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid example", err)
			}
			// No caller-selected authority, amount, allocation, status or content.
			for _, field := range []string{"account_id", "membership_id", "created_by", "amount", "centavos", "allocation", "content_base64", "capability", "storage_key", "url", "sha256", "statuses"} {
				example[field] = "synthetic"
				if schema.Value.VisitJSON(example) == nil {
					t.Fatal("link contract admitted", field)
				}
				delete(example, field)
			}
		})
	}
	target := doc.Components.Schemas["EnterpriseDocumentLinkTarget"].Value
	for _, kind := range []string{"WORK_ORDER", "TRIP", "asset", "fueling", "expense", "BOGUS"} {
		if target.VisitJSON(map[string]any{"kind": kind, "id": "7b0e2a54-0000-4000-8000-000000020902"}) == nil {
			t.Fatal("unregistered link target kind admitted", kind)
		}
	}
	// EMP-FUEL-011 registered the FUELING owner (EDR-297) and EMP-EXP-004 the EXPENSE owner (EDR-338).
	for _, kind := range []string{"FUELING", "EXPENSE"} {
		if err := target.VisitJSON(map[string]any{"kind": kind, "id": "7b0e2a54-0000-4000-8000-000000020902"}); err != nil {
			t.Fatal("registered link target refused", kind, err)
		}
	}
	if target.VisitJSON(map[string]any{"kind": "ASSET", "id": "00000000-0000-0000-0000-000000000000"}) == nil {
		t.Fatal("nil link target admitted")
	}
	create := doc.Components.Schemas["EnterpriseDocumentLinkCreateRequest"].Value
	for name, change := range map[string]func(map[string]any){
		"schema":       func(v map[string]any) { v["schema_version"] = 2 },
		"no-target":    func(v map[string]any) { delete(v, "target") },
		"no-operation": func(v map[string]any) { delete(v, "operation_id") },
		"document":     func(v map[string]any) { v["document_id"] = "7b0e2a54-0000-4000-8000-000000020542" },
	} {
		raw, _ := json.Marshal(create.Example)
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		change(v)
		if create.VisitJSON(v) == nil {
			t.Fatal("link create contract admitted", name)
		}
	}
	for path, write := range map[string]bool{"/v1/orgs/{org_id}/documents/{document_id}/links/create": true, "/v1/orgs/{org_id}/documents/{document_id}/links/list": false, "/v1/orgs/{org_id}/documents/links/by-target": false} {
		item := doc.Paths.Value(path)
		if item == nil || item.Post == nil {
			t.Fatal("link route undocumented", path)
		}
		raw, err := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
		if err != nil {
			t.Fatal(err)
		}
		var rule orgapi.Rule
		if json.Unmarshal(raw, &rule) != nil || rule.Permission != "doc.document.attach" || rule.Write != write || rule.Fresh || !rule.CompanyOnly {
			t.Fatal("link route is not a non-fresh doc.document.attach company route", path)
		}
		if item.Post.Extensions["x-abastevo-document-ingress"] != nil {
			t.Fatal("link route carries ingress metadata", path)
		}
	}
}
