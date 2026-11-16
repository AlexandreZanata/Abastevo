package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-CHECK-002 (EDR-284/285): the check.run.complete owner, its exact
// payload, the run read and release shapes and the CHECKLIST_RUN cause of
// derived availability. No note, photo, author identity or caller-selected
// outcome field is shown or admitted where it does not belong.
func TestChecklistRunOwnerPayloadAndShapesAreExact(t *testing.T) {
	doc := loadSpec(t)
	raw, err := json.Marshal(doc.Paths.Value("/v1/orgs/{org_id}/sync/operations").Post.Extensions["x-abastevo-operation-owners"])
	if err != nil {
		t.Fatal(err)
	}
	var owners map[string]struct {
		Schema  int    `json:"schema"`
		Entity  string `json:"entity_type"`
		Payload struct {
			Ref string `json:"$ref"`
		} `json:"payload"`
		Results   []string          `json:"terminal_results"`
		Followups map[string]string `json:"followup_by_result"`
	}
	if json.Unmarshal(raw, &owners) != nil || len(owners) != 20 {
		t.Fatal("owners unreadable or count drift", len(owners))
	}
	owner, ok := owners["check.run.complete"]
	if !ok || owner.Schema != 1 || owner.Entity != "checklist_run" || owner.Payload.Ref != "#/components/schemas/EnterpriseChecklistRunPayload" || len(owner.Results) != 2 ||
		owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
		t.Fatal("checklist run owner/outcome drift")
	}
	schema := doc.Components.Schemas["EnterpriseChecklistRunPayload"]
	if schema == nil || schema.Value == nil || schema.Value.Example == nil {
		t.Fatal("missing checklist run payload contract/example")
	}
	example := func() map[string]any {
		raw, _ := json.Marshal(schema.Value.Example)
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil {
			t.Fatal("invalid example")
		}
		return v
	}
	if err := schema.Value.VisitJSON(example()); err != nil {
		t.Fatal("invalid example", err)
	}
	answer := func(a string, note any) map[string]any {
		return map[string]any{"answer": a, "note": note, "photo": false}
	}
	for name, edit := range map[string]func(map[string]any){
		"return":   func(v map[string]any) { v["context"] = "RETURN" },
		"warn ack": func(v map[string]any) { v["device_outcome"], v["acknowledged"] = "COMPLETED_WITH_ISSUES", true },
		"all ok":   func(v map[string]any) { v["answers"], v["device_outcome"] = []any{answer("OK", nil)}, "COMPLETED_OK" },
		"not applicab": func(v map[string]any) {
			v["answers"] = []any{answer("NOT_APPLICABLE", nil), answer("NOT_OK", "Freios")}
		},
	} {
		v := example()
		edit(v)
		if err := schema.Value.VisitJSON(v); err != nil {
			t.Fatal("valid run refused", name, err)
		}
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown context": func(v map[string]any) { v["context"] = "PARKING" },
		"zero version":    func(v map[string]any) { v["template_version"] = 0 },
		"zero policy":     func(v map[string]any) { v["policy_revision"] = 0 },
		"no answers":      func(v map[string]any) { v["answers"] = []any{} },
		"unknown answer":  func(v map[string]any) { v["answers"] = []any{answer("MAYBE", nil)} },
		"empty note":      func(v map[string]any) { v["answers"] = []any{answer("NOT_OK", "")} },
		"in progress":     func(v map[string]any) { v["device_outcome"] = "IN_PROGRESS" },
		"ack text":        func(v map[string]any) { v["acknowledged"] = "yes" },
		"nil asset":       func(v map[string]any) { v["asset_id"] = "00000000-0000-0000-0000-000000000000" },
		"missing started": func(v map[string]any) { delete(v, "started_at") },
		"answer extra": func(v map[string]any) {
			v["answers"] = []any{map[string]any{"answer": "OK", "note": nil, "photo": false, "position": 1}}
		},
	} {
		v := example()
		edit(v)
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("invalid run admitted", name)
		}
	}
	for _, field := range []string{"outcome", "recheck_outcome", "server_policy_revision", "template_superseded", "unit_id", "cost_centre_id", "trip_id", "author_membership_id",
		"health", "amount", "document_ids", "content_base64", "received_at", "revision"} {
		v := example()
		v[field] = "private"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("checklist run payload admitted", field)
		}
	}
	for _, name := range []string{"EnterpriseChecklistRunReadRequest", "EnterpriseChecklistRunReleaseRequest", "EnterpriseChecklistRun", "EnterpriseChecklistRunRelease"} {
		t.Run(name, func(t *testing.T) {
			schema := doc.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing run schema or explicit example")
			}
			raw, _ := json.Marshal(schema.Value.Example)
			var v map[string]any
			if json.Unmarshal(raw, &v) != nil || schema.Value.VisitJSON(v) != nil {
				t.Fatal("invalid run example")
			}
			for _, field := range []string{"account_id", "note", "notes", "photo", "author_membership_id", "author_employee_id", "unit_id", "cost_centre_id", "reason"} {
				if name == "EnterpriseChecklistRunReleaseRequest" && field == "reason" {
					continue
				}
				v[field] = "synthetic"
				if schema.Value.VisitJSON(v) == nil {
					t.Fatal("run contract admitted", field)
				}
				delete(v, field)
			}
		})
	}
	release := doc.Components.Schemas["EnterpriseChecklistRunReleaseRequest"].Value
	for name, v := range map[string]map[string]any{
		"no revision":    {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "reason": "Freios verificados"},
		"no reason":      {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "revision": float64(1)},
		"empty reason":   {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "revision": float64(1), "reason": ""},
		"outcome chosen": {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "revision": float64(1), "reason": "x", "outcome": "COMPLETED_OK"},
	} {
		if release.VisitJSON(v) == nil {
			t.Fatal("invalid release admitted", name)
		}
	}
	// EDR-285: availability names the BLOCKING run and its missing source.
	cause := doc.Components.Schemas["EnterpriseFleetAvailabilityCause"].Value
	if cause.VisitJSON(map[string]any{"availability": "BLOCKED", "kind": "CHECKLIST_RUN", "record_id": "7b0e2a54-0000-4000-8000-000000009301", "valid_from": "2026-11-15T12:00:00Z"}) != nil {
		t.Fatal("CHECKLIST_RUN cause refused")
	}
	availability := doc.Components.Schemas["EnterpriseFleetAvailability"].Value
	if availability.VisitJSON(map[string]any{"asset_id": "7b0e2a54-0000-4000-8000-000000008201", "availability": "AVAILABLE", "causes": []any{},
		"missing_sources": []any{"ACTIVE_USE", "CHECKLIST_RUN", "MAINTENANCE_HOLD"}, "evaluated_at": "2026-11-15T12:00:00Z"}) != nil {
		t.Fatal("CHECKLIST_RUN missing source refused")
	}
}

func TestChecklistRunContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	routes := map[string]map[string]any{
		"/v1/orgs/{org_id}/checklists/assets/{asset_id}/runs/{run_id}/read": {"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/checklists/runs/{run_id}/release":                {"permission": "check.incident.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200},
	}
	for path, want := range routes {
		if document.Paths.Value(path) == nil || document.Paths.Value(path).Post == nil {
			t.Fatal("missing actual run route", path)
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
					t.Fatal("run authority/contract drift accepted")
				}
			})
		}
	}
}
