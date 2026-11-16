package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-EXP-002 (EDR-289): the expense.submit owner and its exact payload.
func TestExpenseSubmitContractIsAnExplicitAtomicOwnerWithExactValues(t *testing.T) {
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
	owner, ok := owners["expense.submit"]
	if !ok || owner.Schema != 1 || owner.Entity != "expense" || owner.Payload.Ref != "#/components/schemas/EnterpriseExpenseSubmitPayload" || len(owner.Results) != 2 || owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
		t.Fatal("expense owner/outcome drift")
	}
	schema := doc.Components.Schemas["EnterpriseExpenseSubmitPayload"]
	if schema == nil || schema.Value == nil || schema.Value.Example == nil {
		t.Fatal("missing expense payload contract/example")
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
	for _, field := range []string{"author_membership_id", "enrollment_id", "received_at", "review_state", "origin_kind", "operation_id", "occurred_at", "unit_id", "cost_centre_id", "reporting_day", "fueling_id"} {
		v := example()
		v[field] = "private"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("payload authority or server field admitted", field)
		}
	}
	for field, bad := range map[string]any{"paid_total": "12.50", "company_amount": float64(1250), "category": "toll", "payer": "SUPPLIER", "asset_id": "FR-001"} {
		v := example()
		v[field] = bad
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("lossy or unknown value admitted", field, bad)
		}
	}
	for _, field := range []string{"trip_id", "company_amount", "payer"} {
		v := example()
		delete(v, field)
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("required key may be omitted", field)
		}
	}
}
