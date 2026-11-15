package apicontract

import (
	"encoding/json"
	"slices"
	"testing"
)

// EMP-FLEET-006: the assignment commands and reads are documented with exact,
// closed shapes and no caller-chosen state, instant or authority.
func TestFleetAssignmentContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseFleetAssignmentCreateRequest", "EnterpriseFleetAssignmentEndRequest", "EnterpriseFleetAssignmentAcknowledgement", "EnterpriseFleetAssignmentListRequest", "EnterpriseFleetAssignment", "EnterpriseFleetAssignmentPage"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing assignment schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid assignment object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid assignment example", err)
			}
			for _, field := range []string{"account_id", "membership_id", "at", "unit_id", "recorded_by"} {
				injected := map[string]any{}
				for k, v := range example {
					injected[k] = v
				}
				injected[field] = "synthetic"
				if schema.Value.VisitJSON(injected) == nil {
					t.Fatal("caller-selected assignment authority or data accepted", field)
				}
			}
		})
	}
	create := document.Components.Schemas["EnterpriseFleetAssignmentCreateRequest"].Value
	base := map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000008121", "kind": "ADDITIONAL", "employee_ids": []any{"7b0e2a54-0000-4000-8000-000000008601"}}
	if err := create.VisitJSON(base); err != nil {
		t.Fatal("minimal create refused", err)
	}
	many := make([]any, 101)
	for i := range many {
		many[i] = "7b0e2a54-0000-4000-8000-000000008601"
	}
	for name, edit := range map[string]func(map[string]any){
		"state chosen": func(v map[string]any) { v["state"] = "ACTIVE" },
		"unknown kind": func(v map[string]any) { v["kind"] = "POOL" },
		"no employees": func(v map[string]any) { v["employee_ids"] = []any{} },
		"too many":     func(v map[string]any) { v["employee_ids"] = many },
		"duplicates": func(v map[string]any) {
			v["employee_ids"] = []any{"7b0e2a54-0000-4000-8000-000000008601", "7b0e2a54-0000-4000-8000-000000008601"}
		},
		"empty reason":     func(v map[string]any) { v["reason"] = "" },
		"missing kind":     func(v map[string]any) { delete(v, "kind") },
		"employee by name": func(v map[string]any) { v["employee_ids"] = []any{"Synthetic driver"} },
	} {
		value := map[string]any{}
		for k, v := range base {
			value[k] = v
		}
		edit(value)
		if create.VisitJSON(value) == nil {
			t.Fatal("invalid create accepted by the contract", name)
		}
	}
	item := document.Components.Schemas["EnterpriseFleetAssignment"].Value
	if item.VisitJSON(map[string]any{"assignment_id": "7b0e2a54-0000-4000-8000-000000008701", "asset_id": "7b0e2a54-0000-4000-8000-000000008201", "employee_id": "7b0e2a54-0000-4000-8000-000000008601", "kind": "PRIMARY", "state": "PAUSED", "valid_from": "2026-11-01T03:00:00Z", "reason": "", "recorded_at": "2026-11-01T03:00:00Z"}) == nil {
		t.Fatal("unknown assignment state accepted")
	}
	codes := document.Components.Schemas["EnterpriseError"].Value.AllOf[1].Value.Properties["error"].Value.Properties["code"].Value.Enum
	if !slices.Contains(codes, any("ASSIGNMENT_OVERLAP")) {
		t.Fatal("ASSIGNMENT_OVERLAP is not a registered enterprise error code")
	}
}

func TestFleetAssignmentContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	routes := map[string]map[string]any{
		"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/create":              {"permission": "fleet.asset.assign", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 201},
		"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/{assignment_id}/end": {"permission": "fleet.asset.assign", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/fleet/assets/{asset_id}/assignments/list":                {"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/fleet/assignments/mine/list":                             {"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
	}
	for path, want := range routes {
		if document.Paths.Value(path) == nil || document.Paths.Value(path).Post == nil {
			t.Fatal("missing actual assignment route", path)
		}
		for _, test := range []string{"cookie", "permission", "write"} {
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
					metadata["permission"] = "fleet.asset.manage"
				case "write":
					metadata["write"], metadata["fresh"] = !want["write"].(bool), !want["fresh"].(bool)
				}
				if test != "cookie" {
					operation.Extensions["x-abastevo-organization"] = metadata
				}
				if ValidateOrganizationRoutes(actual, doc) == nil {
					t.Fatal("assignment authority/contract drift accepted")
				}
			})
		}
	}
}
