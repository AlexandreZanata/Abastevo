package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-METER-005: the change route and the rollover decision are documented
// with exact, closed shapes; the caller never chooses identifiers, the
// closure, the implied usage or the state.
func TestMeterBoundaryContractExamplesAndExactShapes(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{"EnterpriseMeterChangeRequest", "EnterpriseMeterChange"} {
		t.Run(name, func(t *testing.T) {
			schema := document.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing meter boundary schema or explicit example")
			}
			raw, err := json.Marshal(schema.Value.Example)
			if err != nil {
				t.Fatal(err)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
				t.Fatal("invalid example")
			}
			for _, field := range []string{"new_meter_id_override", "segment_id", "closing_kind", "implied_override", "state"} {
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
	request := document.Components.Schemas["EnterpriseMeterChangeRequest"].Value
	base := map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000008511", "revision": float64(1), "change": "RESET", "kind": "HOUR_METER",
		"occurred_at": "2026-11-11T08:00:00Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": float64(-14400), "before_value": "15200000", "after_value": "0"}
	if err := request.VisitJSON(base); err != nil {
		t.Fatal("minimal reset refused", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown change":    func(v map[string]any) { v["change"] = "ROLLOVER" },
		"zero revision":     func(v map[string]any) { v["revision"] = float64(0) },
		"missing revision":  func(v map[string]any) { delete(v, "revision") },
		"missing instant":   func(v map[string]any) { delete(v, "occurred_at") },
		"decimal value":     func(v map[string]any) { v["before_value"] = "15200000.5" },
		"negative value":    func(v map[string]any) { v["after_value"] = "-1" },
		"unknown kind":      func(v map[string]any) { v["kind"] = "FUEL" },
		"chosen segment":    func(v map[string]any) { v["segment_id"] = "7b0e2a54-0000-4000-8000-000000008514" },
		"chosen usage":      func(v map[string]any) { v["implied_usage"] = "0" },
		"non-boolean final": func(v map[string]any) { v["final_unknown"] = "yes" },
	} {
		value := map[string]any{}
		for k, v := range base {
			value[k] = v
		}
		edit(value)
		if request.VisitJSON(value) == nil {
			t.Fatal("invalid change accepted by the contract", name)
		}
	}
	decision := document.Components.Schemas["EnterpriseMeterDecisionRequest"].Value
	if err := decision.VisitJSON(map[string]any{"schema_version": float64(1), "operation_id": "7b0e2a54-0000-4000-8000-000000008501", "revision": float64(1), "decision": "CONFIRM_ROLLOVER", "reason": "Painel de seis dígitos"}); err != nil {
		t.Fatal("rollover decision refused", err)
	}
	response := document.Components.Schemas["EnterpriseMeterDecision"].Value
	raw, _ := json.Marshal(document.Components.Schemas["EnterpriseMeterChange"].Value.Example)
	var boundary map[string]any
	if err := json.Unmarshal(raw, &boundary); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"asset_id", "replayed", "data_mode"} {
		delete(boundary, k)
	}
	boundary["change"], boundary["closing_reading_id"], boundary["new_meter_id"] = "ROLLOVER", nil, nil
	decided := map[string]any{"asset_id": "7b0e2a54-0000-4000-8000-000000008201", "meter_id": "7b0e2a54-0000-4000-8000-000000008401", "decision": "CONFIRM_ROLLOVER",
		"reading": map[string]any{"reading_id": "7b0e2a54-0000-4000-8000-000000008404", "state": "VALID", "flags": []any{}, "revision": float64(2)}, "replacement": nil,
		"reevaluated": []any{}, "boundary": boundary, "replayed": false, "data_mode": "SYNTHETIC"}
	if err := response.VisitJSON(decided); err != nil {
		t.Fatal("rollover decision response refused", err)
	}
	boundary["implied_usage"] = map[string]any{"status": "INSUFFICIENT_DATA", "reason": "NO_READING_BEFORE", "value": nil, "from_reading_id": "7b0e2a54-0000-4000-8000-000000008405"}
	if response.VisitJSON(decided) == nil {
		t.Fatal("a reason other than METER_GAP accepted for a boundary")
	}
	item := document.Paths.Value("/v1/orgs/{org_id}/fleet/assets/{asset_id}/meters/{meter_id}/change")
	if item == nil || item.Post == nil {
		t.Fatal("undocumented meter change route")
	}
	rawRule, _ := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
	var rule struct {
		Permission      string `json:"permission"`
		Write           bool   `json:"write"`
		Fresh           bool   `json:"fresh"`
		RequireRevision bool   `json:"require_revision"`
	}
	if json.Unmarshal(rawRule, &rule) != nil || rule.Permission != "meter.reading.correct" || !rule.Write || !rule.Fresh || !rule.RequireRevision {
		t.Fatal("meter change route rule", rule)
	}
}
