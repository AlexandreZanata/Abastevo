package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-CHECK-001: checklist template shapes are exact and closed; no state,
// revision result, version, authority or target list is chosen by the caller
// or revealed by the applicable read.
func TestChecklistTemplateContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseChecklistItemInput", "EnterpriseChecklistPolicyInput", "EnterpriseChecklistTemplateCreateRequest", "EnterpriseChecklistTemplatePublishRequest", "EnterpriseChecklistTemplatePolicyRequest",
		"EnterpriseChecklistTemplateArchiveRequest", "EnterpriseChecklistTemplateListRequest", "EnterpriseChecklistTemplateReadRequest", "EnterpriseChecklistApplicableRequest", "EnterpriseChecklistTemplateAcknowledgement",
		"EnterpriseChecklistItem", "EnterpriseChecklistPolicy", "EnterpriseChecklistTemplateSummary", "EnterpriseChecklistTemplatePage", "EnterpriseChecklistTemplateVersion", "EnterpriseChecklistTemplate",
		"EnterpriseChecklistApplicableTemplate", "EnterpriseChecklistApplicable"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing checklist schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid checklist object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid checklist example", err)
			}
			for _, field := range []string{"account_id", "membership_id", "created_by", "org_id", "unit_id", "at", "target_count"} {
				injected := map[string]any{}
				for k, v := range example {
					injected[k] = v
				}
				injected[field] = "synthetic"
				if schema.Value.VisitJSON(injected) == nil {
					t.Fatal("caller-selected checklist authority or data accepted", field)
				}
			}
		})
	}
	create := document.Components.Schemas["EnterpriseChecklistTemplateCreateRequest"].Value
	item := map[string]any{"label": "Pneus"}
	base := func() map[string]any {
		return map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009101", "context": "DEPARTURE", "name": "Saída", "target_kind": "ORGANIZATION", "items": []any{item}}
	}
	if err := create.VisitJSON(base()); err != nil {
		t.Fatal("minimal create refused", err)
	}
	forty := []any{}
	for i := 0; i < 41; i++ {
		forty = append(forty, item)
	}
	for name, edit := range map[string]func(map[string]any){
		"state chosen":        func(v map[string]any) { v["state"] = "ACTIVE" },
		"revision chosen":     func(v map[string]any) { v["revision"] = float64(1) },
		"version chosen":      func(v map[string]any) { v["version"] = float64(2) },
		"no items":            func(v map[string]any) { v["items"] = []any{} },
		"41 items":            func(v map[string]any) { v["items"] = forty },
		"unknown context":     func(v map[string]any) { v["context"] = "PARKING" },
		"unknown target kind": func(v map[string]any) { v["target_kind"] = "UNITS" },
		"empty name":          func(v map[string]any) { v["name"] = "" },
		"duplicate targets": func(v map[string]any) {
			v["target_ids"] = []any{"7b0e2a54-0000-4000-8000-000000008501", "7b0e2a54-0000-4000-8000-000000008501"}
		},
		"unknown item flag": func(v map[string]any) { v["items"] = []any{map[string]any{"label": "x", "blocking": true}} },
		"policy too short": func(v map[string]any) {
			v["policy"] = map[string]any{"mode": "BLOCK", "validity_seconds": float64(3599)}
		},
		"policy too long": func(v map[string]any) {
			v["policy"] = map[string]any{"mode": "BLOCK", "validity_seconds": float64(259201)}
		},
		"unknown policy mode": func(v map[string]any) {
			v["policy"] = map[string]any{"mode": "ALLOW", "validity_seconds": float64(3600)}
		},
	} {
		value := base()
		edit(value)
		if create.VisitJSON(value) == nil {
			t.Fatal("invalid create accepted by the contract", name)
		}
	}
	for name, schema := range map[string]map[string]any{
		"EnterpriseChecklistTemplatePublishRequest": {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009102", "name": "x", "target_kind": "ORGANIZATION", "items": []any{item}},
		"EnterpriseChecklistTemplatePolicyRequest":  {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009103", "mode": "WARN", "validity_seconds": float64(3600)},
		"EnterpriseChecklistTemplateArchiveRequest": {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009104"},
	} {
		if document.Components.Schemas[name].Value.VisitJSON(schema) == nil {
			t.Fatal("command without the template revision accepted", name)
		}
	}
	applicable := document.Components.Schemas["EnterpriseChecklistApplicableTemplate"].Value
	if applicable.VisitJSON(map[string]any{"template_id": "7b0e2a54-0000-4000-8000-000000009301", "name": "x", "version": float64(1), "published_at": "2026-11-15T12:00:00Z", "items": []any{map[string]any{"position": float64(1), "label": "x", "critical": false, "optional": false, "photo_on_not_ok": false}}, "target_ids": []any{}}) == nil {
		t.Fatal("the applicable template may not carry the template's targets")
	}
	match := document.Components.Schemas["EnterpriseChecklistApplicable"].Value
	if match.VisitJSON(map[string]any{"asset_id": "7b0e2a54-0000-4000-8000-000000008201", "context": "DEPARTURE", "required": false, "match": "UNIT", "evaluated_at": "2026-11-15T12:30:00Z", "data_mode": "SYNTHETIC"}) == nil {
		t.Fatal("unknown match accepted")
	}
}

func TestChecklistTemplateContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	write := func(success int, revision bool) map[string]any {
		return map[string]any{"permission": "check.template.manage", "write": true, "fresh": true, "require_revision": revision, "company_only": true, "success": success}
	}
	templateRead := map[string]any{"permission": "check.template.manage", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	assetRead := map[string]any{"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	prefix := "/v1/orgs/{org_id}/checklists/"
	routes := map[string]map[string]any{
		prefix + "templates/create":                write(201, false),
		prefix + "templates/{template_id}/publish": write(200, true),
		prefix + "templates/{template_id}/policy":  write(200, true),
		prefix + "templates/{template_id}/archive": write(200, true),
		prefix + "templates/list":                  templateRead,
		prefix + "templates/{template_id}/read":    templateRead,
		prefix + "assets/{asset_id}/applicable":    assetRead,
	}
	for path, want := range routes {
		if document.Paths.Value(path) == nil || document.Paths.Value(path).Post == nil {
			t.Fatal("missing actual checklist route", path)
		}
		for _, test := range []string{"cookie", "permission", "write", "revision"} {
			t.Run(path+"/"+test, func(t *testing.T) {
				doc, actual := companyRoutes(t)
				operation := doc.Paths.Value(path).Post
				metadata := map[string]any{}
				for k, v := range want {
					metadata[k] = v
				}
				switch test {
				case "cookie":
					operation.Security = nil
				case "permission":
					metadata["permission"] = "check.checklist.execute"
				case "write":
					metadata["write"], metadata["fresh"] = !want["write"].(bool), !want["fresh"].(bool)
				case "revision":
					metadata["require_revision"] = !want["require_revision"].(bool)
				}
				if test != "cookie" {
					operation.Extensions["x-abastevo-organization"] = metadata
				}
				if ValidateOrganizationRoutes(actual, doc) == nil {
					t.Fatal("checklist authority/contract drift accepted")
				}
			})
		}
	}
}
