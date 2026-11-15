package apicontract

import (
	"encoding/json"
	"slices"
	"testing"
)

// EMP-METER-004: the history, suspect list and decision are documented with
// exact, closed shapes; the caller never chooses state, author, capture,
// flags or the replacement identity.
func TestMeterCorrectionContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseMeterHistoryRequest", "EnterpriseMeterHistoryPage", "EnterpriseMeterDecisionRequest", "EnterpriseMeterDecision"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing meter correction schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid object example")
			}
			if err := schema.Value.VisitJSON(example); err != nil {
				t.Fatal("invalid example", err)
			}
			for _, field := range []string{"author_membership_id", "capture", "state_override", "replacement_id", "received_at_override"} {
				injected := map[string]any{}
				for k, v := range example {
					injected[k] = v
				}
				injected[field] = "synthetic"
				if schema.Value.VisitJSON(injected) == nil {
					t.Fatal("caller-selected field accepted", field)
				}
			}
		})
	}
	request := document.Components.Schemas["EnterpriseMeterDecisionRequest"].Value
	base := map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000008501", "revision": float64(1), "decision": "SUPERSEDE", "reason": "Dígitos trocados", "kind": "ODOMETER", "value": "184190000"}
	if err := request.VisitJSON(base); err != nil {
		t.Fatal("minimal supersession refused", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown decision":  func(v map[string]any) { v["decision"] = "REOPEN" },
		"empty reason":      func(v map[string]any) { v["reason"] = "" },
		"missing reason":    func(v map[string]any) { delete(v, "reason") },
		"missing revision":  func(v map[string]any) { delete(v, "revision") },
		"negative revision": func(v map[string]any) { v["revision"] = float64(-1) },
		"decimal value":     func(v map[string]any) { v["value"] = "184190.5" },
		"unknown kind":      func(v map[string]any) { v["kind"] = "FUEL" },
		"chosen state":      func(v map[string]any) { v["state"] = "VALID" },
		"chosen flags":      func(v map[string]any) { v["flags"] = []any{} },
	} {
		value := map[string]any{}
		for k, v := range base {
			value[k] = v
		}
		edit(value)
		if request.VisitJSON(value) == nil {
			t.Fatal("invalid decision accepted by the contract", name)
		}
	}
	item := document.Components.Schemas["EnterpriseMeterHistoryItem"].Value
	example := document.Components.Schemas["EnterpriseMeterHistoryPage"].Value.Example.(map[string]any)["items"].([]any)[0].(map[string]any)
	bad := map[string]any{}
	for k, v := range example {
		bad[k] = v
	}
	bad["flags"] = []any{"POSSIBLE_ROLLOVER"}
	if item.VisitJSON(bad) == nil {
		t.Fatal("unknown flag accepted")
	}
	bad["flags"], bad["state"] = []any{}, "HELD"
	if item.VisitJSON(bad) == nil {
		t.Fatal("unknown reading state accepted")
	}
	codes := document.Components.Schemas["EnterpriseError"].Value.AllOf[1].Value.Properties["error"].Value.Properties["code"].Value.Enum
	for _, code := range []string{"READING_ALREADY_SUPERSEDED", "AMENDMENT_REQUIRED"} {
		if !slices.Contains(codes, any(code)) {
			t.Fatal(code, "is not a registered enterprise error code")
		}
	}
	for path, permission := range map[string]string{
		"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/list":                "fleet.asset.read",
		"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/suspect/list":        "meter.reading.correct",
		"/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/readings/{reading_id}/decide": "meter.reading.correct",
	} {
		item := document.Paths.Value(path)
		if item == nil || item.Post == nil {
			t.Fatal("undocumented meter correction route", path)
		}
		raw, _ := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
		var rule struct {
			Permission string `json:"permission"`
		}
		if json.Unmarshal(raw, &rule) != nil || rule.Permission != permission {
			t.Fatal("route rule", path, rule.Permission)
		}
	}
}
