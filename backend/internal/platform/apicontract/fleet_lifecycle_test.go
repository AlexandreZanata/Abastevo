package apicontract

import (
	"encoding/json"
	"testing"
)

func TestFleetLifecycleContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseFleetAssetAcquisitionRequest", "EnterpriseFleetAssetReasonRequest", "EnterpriseFleetAssetDisposeRequest", "EnterpriseFleetAssetDisposal", "EnterpriseFleetAssetLifecycleListRequest", "EnterpriseFleetAssetLifecycleEntry", "EnterpriseFleetAssetLifecyclePage"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing lifecycle schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid lifecycle object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid lifecycle example", err)
			}
			for _, field := range []string{"account_id", "state", "open_operations", "amount_cents"} {
				injected := map[string]any{}
				for k, v := range example {
					injected[k] = v
				}
				injected[field] = "synthetic"
				if schema.Value.VisitJSON(injected) == nil {
					t.Fatal("caller-selected lifecycle authority or data accepted", field)
				}
			}
		})
	}
	dispose := document.Components.Schemas["EnterpriseFleetAssetDisposeRequest"].Value
	base := map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000008111", "revision": float64(5), "kind": "SALE", "disposed_at": "2026-11-10T12:00:00.123456789Z", "reason": "Synthetic sale"}
	if err := dispose.VisitJSON(base); err != nil {
		t.Fatal("nanosecond disposal refused", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"missing reason": func(v map[string]any) { delete(v, "reason") },
		"empty reason":   func(v map[string]any) { v["reason"] = "" },
		"blank reason":   func(v map[string]any) { v["reason"] = "   " },
		"unknown kind":   func(v map[string]any) { v["kind"] = "THEFT" },
		"local instant":  func(v map[string]any) { v["disposed_at"] = "2026-11-10T12:00:00" },
		"ten fractions":  func(v map[string]any) { v["disposed_at"] = "2026-11-10T12:00:00.1234567891Z" },
		"zero revision":  func(v map[string]any) { v["revision"] = float64(0) },
		"missing kind":   func(v map[string]any) { delete(v, "kind") },
	} {
		value := map[string]any{}
		for k, v := range base {
			value[k] = v
		}
		edit(value)
		if dispose.VisitJSON(value) == nil {
			t.Fatal("invalid disposal accepted by the contract", name)
		}
	}
	entry := document.Components.Schemas["EnterpriseFleetAssetLifecycleEntry"].Value
	for _, bad := range []map[string]any{
		{"revision": float64(1), "command": "DEACTIVATE", "from_state": "ACTIVE", "to_state": "INACTIVE", "reason": "R", "recorded_at": "2026-11-14T12:00:00Z"},
		{"revision": float64(3), "command": "RECORD_ACQUISITION", "from_state": "ACTIVE", "to_state": "ACTIVE", "reason": "", "recorded_at": "2026-11-14T12:00:00Z"},
		{"revision": float64(3), "command": "ARCHIVE", "from_state": "DRAFT", "to_state": "ARCHIVED", "reason": "", "recorded_at": "2026-11-14T12:00:00Z"},
	} {
		if entry.VisitJSON(bad) == nil {
			t.Fatal("impossible lifecycle history row accepted")
		}
	}
	view := document.Components.Schemas["EnterpriseFleetAssetView"].Value
	raw, _ := json.Marshal(view.Example)
	var example map[string]any
	if json.Unmarshal(raw, &example) != nil {
		t.Fatal("invalid view example")
	}
	example["state"], example["acquired_at"] = "DISPOSED", "2026-10-01T12:00:00.123456789Z"
	example["disposal"] = map[string]any{"kind": "WRITE_OFF", "disposed_at": "2026-11-10T12:00:00Z", "reason": "Synthetic total loss"}
	if err := view.VisitJSON(example); err != nil {
		t.Fatal("disposed asset view refused", err)
	}
	example["disposal"] = map[string]any{"kind": "WRITE_OFF", "disposed_at": "2026-11-10T12:00:00Z", "reason": "Synthetic", "amount_cents": "1"}
	if view.VisitJSON(example) == nil {
		t.Fatal("disposal view accepted an undeclared amount")
	}
	review := document.Components.Schemas["EnterpriseMeterReviewView"].Value.Properties["code"].Value
	// EMP-OFFL-009 adds the late-intake hold to the same held producer.
	if len(review.Enum) != 3 || review.Enum[0] != "PERMISSION_CHANGED" || review.Enum[1] != "ASSET_UNAVAILABLE" || review.Enum[2] != "ACCESS_ENDED_BEFORE_RECEIPT" {
		t.Fatal("meter review codes drifted from the held producer", review.Enum)
	}
}

func TestFleetLifecycleContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	const base = "/v1/orgs/{org_id}/fleet/assets/{asset_id}"
	manage := []string{"/acquisition/record", "/deactivate", "/reactivate", "/dispose", "/disposal/revert", "/archive"}
	for _, suffix := range append(manage, "/lifecycle/list") {
		if document.Paths.Value(base+suffix) == nil || document.Paths.Value(base+suffix).Post == nil {
			t.Fatal("missing actual lifecycle route", suffix)
		}
	}
	for _, suffix := range manage {
		for _, test := range []string{"cookie", "fresh", "permission", "revision"} {
			t.Run(suffix+"/"+test, func(t *testing.T) {
				doc, actual := companyRoutes(t)
				operation := doc.Paths.Value(base + suffix).Post
				metadata := map[string]any{"permission": "fleet.asset.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200}
				switch test {
				case "cookie":
					operation.Security = nil
				case "fresh":
					metadata["fresh"] = false
				case "permission":
					metadata["permission"] = "fleet.asset.read"
				case "revision":
					metadata["require_revision"] = false
				}
				if test != "cookie" {
					operation.Extensions["x-abastevo-organization"] = metadata
				}
				if ValidateOrganizationRoutes(actual, doc) == nil {
					t.Fatal("lifecycle authority/contract drift accepted")
				}
			})
		}
	}
	doc, actual := companyRoutes(t)
	doc.Paths.Value(base + "/lifecycle/list").Post.Extensions["x-abastevo-organization"] = map[string]any{"permission": "fleet.asset.manage", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	if ValidateOrganizationRoutes(actual, doc) == nil {
		t.Fatal("history read permission drift accepted")
	}
}
