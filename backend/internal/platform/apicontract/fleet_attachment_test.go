package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-FLEET-011: attachment shapes are exact and closed; the commands bind
// fresh fleet.asset.manage and the list fleet.asset.read on the actual router.
func TestFleetAttachmentContractExamplesShapesAndGuards(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseFleetAttachRequest", "EnterpriseFleetDetachRequest", "EnterpriseFleetAttachmentAcknowledgement", "EnterpriseFleetAttachment", "EnterpriseFleetAttachmentPage"} {
		schema := document.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing attachment schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
			t.Fatal("invalid attachment example", name)
		}
		for _, field := range []string{"implement_kind", "state_override", "account_id", "attached_by"} {
			injected := map[string]any{}
			for k, v := range example {
				injected[k] = v
			}
			injected[field] = "synthetic"
			if schema.Value.VisitJSON(injected) == nil {
				t.Fatal("caller-selected attachment data accepted", name, field)
			}
		}
	}
	attach := document.Components.Schemas["EnterpriseFleetAttachRequest"].Value
	if attach.VisitJSON(map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000009401"}) == nil {
		t.Fatal("attach without a powered asset accepted")
	}
	cause := document.Components.Schemas["EnterpriseFleetAvailabilityCause"].Value
	if cause.VisitJSON(map[string]any{"availability": "IN_USE", "kind": "ACTIVE_USE", "record_id": "7b0e2a54-0000-4000-8000-000000009601", "powered_asset_id": "7b0e2a54-0000-4000-8000-000000008202", "valid_from": "2026-11-15T11:00:00Z"}) != nil {
		t.Fatal("an implement's use through its powered asset is not representable")
	}
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	prefix := "/v1/orgs/{org_id}/fleet/assets/{asset_id}/attachments/"
	for path, want := range map[string]map[string]any{
		prefix + "attach":                 {"permission": "fleet.asset.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 201},
		prefix + "{attachment_id}/detach": {"permission": "fleet.asset.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 200},
		prefix + "list":                   {"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
	} {
		doc, actual := companyRoutes(t)
		operation := doc.Paths.Value(path).Post
		if operation == nil {
			t.Fatal("missing actual attachment route", path)
		}
		metadata := map[string]any{}
		for k, v := range want {
			metadata[k] = v
		}
		metadata["permission"] = "fleet.asset.assign"
		operation.Extensions["x-abastevo-organization"] = metadata
		if ValidateOrganizationRoutes(actual, doc) == nil {
			t.Fatal("attachment authority/contract drift accepted", path)
		}
	}
}
