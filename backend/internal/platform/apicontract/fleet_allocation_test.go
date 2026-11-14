package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-FLEET-005: both history reads are documented with exact, closed shapes.
func TestFleetAllocationContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseFleetAssetAllocationListRequest", "EnterpriseFleetAssetUnitAllocation", "EnterpriseFleetAssetUnitAllocationPage", "EnterpriseFleetAssetCostCentreAllocation", "EnterpriseFleetAssetCostCentreAllocationPage"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing allocation schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid allocation object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid allocation example", err)
			}
			for _, field := range []string{"account_id", "state", "at", "team_id", "amount_cents"} {
				injected := map[string]any{}
				for k, v := range example {
					injected[k] = v
				}
				injected[field] = "synthetic"
				if schema.Value.VisitJSON(injected) == nil {
					t.Fatal("caller-selected allocation authority or data accepted", field)
				}
			}
		})
	}
	for _, name := range []string{"EnterpriseFleetAssetUnitAllocation", "EnterpriseFleetAssetCostCentreAllocation", "EnterpriseFleetAssetAllocationListRequest"} {
		if document.Components.Schemas[name] == nil || document.Components.Schemas[name].Value == nil {
			t.Fatal("missing allocation schema", name)
		}
	}
	unit := document.Components.Schemas["EnterpriseFleetAssetUnitAllocation"].Value
	open := map[string]any{"unit_id": "7b0e2a54-0000-4000-8000-000000008301", "valid_from": "2026-04-01T03:00:00.123456Z", "recorded_at": "2026-04-01T03:00:00.123456Z"}
	if err := unit.VisitJSON(open); err != nil {
		t.Fatal("open unit period refused", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"missing unit":     func(v map[string]any) { delete(v, "unit_id") },
		"missing start":    func(v map[string]any) { delete(v, "valid_from") },
		"missing recorded": func(v map[string]any) { delete(v, "recorded_at") },
		"cost centre here": func(v map[string]any) { v["cost_centre_id"] = "7b0e2a54-0000-4000-8000-000000008401" },
		"null end":         func(v map[string]any) { v["valid_to"] = nil },
		"local start":      func(v map[string]any) { v["valid_from"] = "2026-04-01T03:00:00" },
		"unit name":        func(v map[string]any) { v["unit_id"] = "Norte" },
	} {
		value := map[string]any{}
		for k, v := range open {
			value[k] = v
		}
		edit(value)
		if unit.VisitJSON(value) == nil {
			t.Fatal("invalid unit period accepted by the contract", name)
		}
	}
	centre := document.Components.Schemas["EnterpriseFleetAssetCostCentreAllocation"].Value
	if centre.VisitJSON(open) == nil {
		t.Fatal("unit period accepted as a cost-centre period")
	}
	request := document.Components.Schemas["EnterpriseFleetAssetAllocationListRequest"].Value
	for _, bad := range []map[string]any{{"limit": float64(0)}, {"limit": float64(101)}, {"unit_id": "x"}} {
		if request.VisitJSON(bad) == nil {
			t.Fatal("invalid allocation page request accepted", bad)
		}
	}
}

func TestFleetAllocationContractBindsActualCurrentCompanyGuards(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	const base = "/v1/orgs/{org_id}/fleet/assets/{asset_id}"
	for _, suffix := range []string{"/unit-allocations/list", "/cost-centre-allocations/list"} {
		if document.Paths.Value(base+suffix) == nil || document.Paths.Value(base+suffix).Post == nil {
			t.Fatal("missing actual allocation route", suffix)
		}
		for _, test := range []string{"cookie", "permission", "write"} {
			t.Run(suffix+"/"+test, func(t *testing.T) {
				doc, actual := companyRoutes(t)
				operation := doc.Paths.Value(base + suffix).Post
				metadata := map[string]any{"permission": "fleet.asset.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
				switch test {
				case "cookie":
					operation.Security = nil
				case "permission":
					metadata["permission"] = "fleet.asset.manage"
				case "write":
					metadata["write"], metadata["fresh"] = true, true
				}
				if test != "cookie" {
					operation.Extensions["x-abastevo-organization"] = metadata
				}
				if ValidateOrganizationRoutes(actual, doc) == nil {
					t.Fatal("allocation authority/contract drift accepted")
				}
			})
		}
	}
}
