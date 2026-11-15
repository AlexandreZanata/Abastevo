package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-FUEL-002 (EDR-175): the fueling.submit owner and its exact payload.
func TestFuelingSubmitContractIsAnExplicitAtomicOwnerWithExactValues(t *testing.T) {
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
	owner, ok := owners["fueling.submit"]
	if !ok || owner.Schema != 1 || owner.Entity != "fueling" || owner.Payload.Ref != "#/components/schemas/EnterpriseFuelingSubmitPayload" || len(owner.Results) != 2 || owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
		t.Fatal("fueling owner/outcome drift")
	}
	schema := doc.Components.Schemas["EnterpriseFuelingSubmitPayload"]
	if schema == nil || schema.Value == nil || schema.Value.Example == nil {
		t.Fatal("missing fueling payload contract/example")
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
	missing := example()
	missing["reading"], missing["missing_reading_reason"] = nil, "Painel apagado"
	if err := schema.Value.VisitJSON(missing); err != nil {
		t.Fatal("missing-reading form refused", err)
	}
	for _, field := range []string{"author_membership_id", "enrollment_id", "received_at", "review_state", "origin", "operation_id", "occurred_at", "unit_id", "warnings", "expected_centavos"} {
		v := example()
		v[field] = "private"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("payload authority or server field admitted", field)
		}
	}
	for field, bad := range map[string]any{"quantity": "150.000", "paid_total": float64(97000), "unit_price": "0", "discount": "-1", "unit": "GAL", "product": "LPG_P13", "fill": "HALF", "payment_method": "BARTER"} {
		v := example()
		v[field] = bad
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("lossy or unknown value admitted", field, bad)
		}
	}
	for _, field := range []string{"discount", "trip_id", "place"} {
		v := example()
		delete(v, field)
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("optional key may not be omitted", field)
		}
	}
}
