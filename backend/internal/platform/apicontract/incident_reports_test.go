package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
)

// EMP-CHECK-003 (EDR-238): the check.incident.report owner, its exact payload
// and the closed handling shapes. No cost, document content, health or
// caller-selected authority field exists.
func TestIncidentReportOwnerAndPayloadAreExact(t *testing.T) {
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
	if json.Unmarshal(raw, &owners) != nil {
		t.Fatal("owners unreadable")
	}
	owner, ok := owners["check.incident.report"]
	if !ok || owner.Schema != 1 || owner.Entity != "incident_report" || owner.Payload.Ref != "#/components/schemas/EnterpriseIncidentReportPayload" || len(owner.Results) != 2 ||
		owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
		t.Fatal("incident report owner/outcome drift")
	}
	schema := doc.Components.Schemas["EnterpriseIncidentReportPayload"]
	if schema == nil || schema.Value == nil || schema.Value.Example == nil {
		t.Fatal("missing incident report payload contract/example")
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
	for name, edit := range map[string]func(map[string]any){
		"incident": func(v map[string]any) { v["report_type"], v["incident_kind"] = "INCIDENT", "THEFT" },
		"no trip":  func(v map[string]any) { v["trip_id"] = nil },
		"no place": func(v map[string]any) { v["location"] = nil },
		"blocking": func(v map[string]any) { v["blocks_use"] = true },
	} {
		v := example()
		edit(v)
		if err := schema.Value.VisitJSON(v); err != nil {
			t.Fatal("valid report refused", name, err)
		}
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown type":      func(v map[string]any) { v["report_type"] = "FIRE" },
		"unknown kind":      func(v map[string]any) { v["incident_kind"] = "FLOOD" },
		"empty description": func(v map[string]any) { v["description"] = "" },
		"empty location":    func(v map[string]any) { v["location"] = "" },
		"blocks text":       func(v map[string]any) { v["blocks_use"] = "no" },
		"nil asset":         func(v map[string]any) { v["asset_id"] = "00000000-0000-0000-0000-000000000000" },
		"nil trip":          func(v map[string]any) { v["trip_id"] = "00000000-0000-0000-0000-000000000000" },
		"missing trip":      func(v map[string]any) { delete(v, "trip_id") },
		"long description":  func(v map[string]any) { v["description"] = strings.Repeat("a", 4001) },
	} {
		v := example()
		edit(v)
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("invalid report admitted", name)
		}
	}
	// BR-CHECK-015/018: no health, person, cost, document or authority field.
	for _, field := range []string{"injuries", "health", "victim", "third_party", "amount", "cost_centavos", "expense_id", "document_ids", "photo", "content_base64", "police_report",
		"state", "revision", "reporter_membership_id", "received_at", "unit_id", "cost_centre_id", "occurred_at"} {
		v := example()
		v[field] = "private"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("incident report payload admitted", field)
		}
	}
	for _, name := range []string{"EnterpriseIncidentReportListRequest", "EnterpriseIncidentReportReadRequest", "EnterpriseIncidentReportPage", "EnterpriseIncidentOwnReportPage",
		"EnterpriseIncidentReportView", "EnterpriseIncidentReportDecisionRequest", "EnterpriseIncidentReportDecision"} {
		t.Run(name, func(t *testing.T) {
			schema := doc.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing incident schema or explicit example")
			}
			raw, _ := json.Marshal(schema.Value.Example)
			var v map[string]any
			if json.Unmarshal(raw, &v) != nil || schema.Value.VisitJSON(v) != nil {
				t.Fatal("invalid incident example")
			}
			for _, field := range []string{"account_id", "content_base64", "amount", "storage_key", "sha256", "capability", "unit_id", "cost_centre_id"} {
				v[field] = "synthetic"
				if schema.Value.VisitJSON(v) == nil {
					t.Fatal("incident contract admitted", field)
				}
				delete(v, field)
			}
		})
	}
	attachment := doc.Components.Schemas["EnterpriseIncidentAttachment"].Value
	for _, state := range []string{"UPLOADING", "RECEIVED", "QUARANTINED", "FAILED", "STORED"} {
		if attachment.VisitJSON(map[string]any{"document_id": "7b0e2a54-0000-4000-8000-000000009701", "storage_state": state, "linked_at": "2026-11-15T10:07:00Z"}) != nil {
			t.Fatal("a document's own storage state refused", state)
		}
	}
	if attachment.VisitJSON(map[string]any{"document_id": "7b0e2a54-0000-4000-8000-000000009701", "storage_state": "READY", "linked_at": "2026-11-15T10:07:00Z"}) == nil {
		t.Fatal("an invented ready state admitted")
	}
	decide := doc.Components.Schemas["EnterpriseIncidentReportDecisionRequest"].Value
	for name, v := range map[string]map[string]any{
		"no revision":    {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "action": "ACKNOWLEDGE"},
		"unknown action": {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "revision": float64(1), "action": "OPEN"},
		"null note":      {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "revision": float64(1), "action": "RESOLVE", "note": nil},
		"state chosen":   {"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009601", "revision": float64(1), "action": "ACKNOWLEDGE", "state": "ACKNOWLEDGED"},
	} {
		if decide.VisitJSON(v) == nil {
			t.Fatal("invalid handling command admitted", name)
		}
	}
	// EDR-239: the document link target admits the INCIDENT kind.
	target := doc.Components.Schemas["EnterpriseDocumentLinkTarget"].Value
	if target.VisitJSON(map[string]any{"kind": "INCIDENT", "id": "7b0e2a54-0000-4000-8000-000000009101"}) != nil {
		t.Fatal("INCIDENT link target refused")
	}
}

func TestIncidentReportContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	read := func(permission string) map[string]any {
		return map[string]any{"permission": permission, "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	}
	prefix := "/v1/orgs/{org_id}/incidents/reports/"
	routes := map[string]map[string]any{
		prefix + "list":               read("check.incident.manage"),
		prefix + "{report_id}/read":   read("check.incident.manage"),
		prefix + "mine":               read("check.incident.report"),
		prefix + "{report_id}/decide": {"permission": "check.incident.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200},
	}
	for path, want := range routes {
		if document.Paths.Value(path) == nil || document.Paths.Value(path).Post == nil {
			t.Fatal("missing actual incident route", path)
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
					t.Fatal("incident authority/contract drift accepted")
				}
			})
		}
	}
}
