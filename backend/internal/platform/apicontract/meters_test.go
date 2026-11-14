package apicontract

import (
	"encoding/json"
	"testing"
)

func TestMeterContractExamplesAndExactEvidence(t *testing.T) {
	doc := loadSpec(t)
	for _, name := range []string{"EnterpriseMeterInstallRequest", "EnterpriseMeterReadRequest", "EnterpriseMeterListRequest", "EnterpriseMeterAcknowledgement", "EnterpriseMeterReading", "EnterpriseMeterView", "EnterpriseMeterPage", "EnterpriseStandaloneMeterReadingPayload"} {
		t.Run(name, func(t *testing.T) {
			schema := doc.Components.Schemas[name]
			if schema == nil || schema.Value == nil || schema.Value.Example == nil {
				t.Fatal("missing meter contract/example")
			}
			raw, e := json.Marshal(schema.Value.Example)
			if e != nil {
				t.Fatal(e)
			}
			var example map[string]any
			if json.Unmarshal(raw, &example) != nil {
				t.Fatal("invalid contract object")
			}
			if e := schema.Value.VisitJSON(example); e != nil {
				t.Fatal("invalid example", e)
			}
			example["account_id"] = "synthetic"
			if schema.Value.VisitJSON(example) == nil {
				t.Fatal("caller-selected authority accepted")
			}
		})
	}
}

func TestMeterStandaloneContractHasOneExplicitAtomicOwner(t *testing.T) {
	doc := loadSpec(t)
	raw, e := json.Marshal(doc.Paths.Value("/v1/orgs/{org_id}/sync/operations").Post.Extensions["x-abastevo-operation-owners"])
	if e != nil {
		t.Fatal(e)
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
	if json.Unmarshal(raw, &owners) != nil || len(owners) != 1 {
		t.Fatal("missing explicit source-owned operation declaration")
	}
	owner := owners["meter.reading.record"]
	if owner.Schema != 1 || owner.Entity != "meter_reading" || owner.Payload.Ref != "#/components/schemas/EnterpriseStandaloneMeterReadingPayload" || len(owner.Followups) != 3 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["ACCEPTED_FOR_REVIEW"] != "ATOMIC_REVIEW" || len(owner.Results) != 3 || owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || owner.Results[2] != "ACCEPTED_FOR_REVIEW" {
		t.Fatal("standalone owner/outcome drift")
	}
	schema := doc.Components.Schemas["EnterpriseStandaloneMeterReadingPayload"].Value
	for _, field := range []string{"author_membership_id", "device_id", "received_at", "state", "origin", "operation_id", "occurred_at"} {
		raw, _ := json.Marshal(schema.Example)
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		v[field] = "private"
		if schema.VisitJSON(v) == nil {
			t.Fatal("payload authority or duplicate envelope metadata admitted", field)
		}
	}
}

func TestMeterContractRejectsLossyValuesAndInconsistentUnits(t *testing.T) {
	doc := loadSpec(t)
	install := doc.Components.Schemas["EnterpriseMeterInstallRequest"].Value
	for _, bad := range []any{float64(1), "-1", "01", "1.0", "1000000000001"} {
		raw, _ := json.Marshal(install.Example)
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		v["opening_value"] = bad
		if install.VisitJSON(v) == nil {
			t.Fatal("out-of-bound/inexact opening admitted", bad)
		}
	}
	raw, _ := json.Marshal(install.Example)
	var hour map[string]any
	_ = json.Unmarshal(raw, &hour)
	hour["kind"] = "HOUR_METER"
	hour["opening_value"] = "3600000000"
	if e := install.VisitJSON(hour); e != nil {
		t.Fatal("exact hour bound refused", e)
	}
	hour["opening_value"] = "3600000001"
	if install.VisitJSON(hour) == nil {
		t.Fatal("hour bound omitted")
	}
	view := doc.Components.Schemas["EnterpriseMeterView"].Value
	raw, _ = json.Marshal(view.Example)
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	v["unit"] = "s"
	if view.VisitJSON(v) == nil {
		t.Fatal("odometer reinterpreted as seconds")
	}
	_ = json.Unmarshal(raw, &v)
	v["latest"].(map[string]any)["state"] = "SUSPECT"
	if view.VisitJSON(v) == nil {
		t.Fatal("latest-valid contract admits suspect truth")
	}
}
