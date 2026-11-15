package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-TRIP-004: trip.submit is an explicit owner with the exact empty payload
// and no held path; the reviewer read and decision bind trip.trip.review on
// the actual router with exact, closed shapes (EDR-220, EDR-222).
func TestTripReviewContractOwnerShapesAndGuards(t *testing.T) {
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
	owner, ok := owners["trip.submit"]
	if !ok || owner.Schema != 1 || owner.Entity != "trip_review" || owner.Payload.Ref != "#/components/schemas/EnterpriseTripSubmitPayload" || len(owner.Results) != 2 ||
		owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
		t.Fatal("trip.submit owner/outcome drift")
	}
	payload := doc.Components.Schemas["EnterpriseTripSubmitPayload"]
	if payload == nil || payload.Value == nil || payload.Value.VisitJSON(map[string]any{}) != nil {
		t.Fatal("the empty submission payload is refused")
	}
	for _, field := range []string{"trip_id", "revision", "review_state", "author_membership_id", "reason"} {
		if payload.Value.VisitJSON(map[string]any{field: "private"}) == nil {
			t.Fatal("a submission payload key was admitted", field)
		}
	}
	examples := map[string]map[string]any{}
	for _, name := range []string{"EnterpriseTripReviewReadRequest", "EnterpriseTripReviewDecisionRequest", "EnterpriseTripReviewDecision", "EnterpriseTripReviewView"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing review schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
			t.Fatal("invalid review example", name)
		}
		examples[name] = example
		injected := map[string]any{"reviewer_override": "synthetic"}
		for k, v := range example {
			injected[k] = v
		}
		if schema.Value.VisitJSON(injected) == nil {
			t.Fatal("open review shape", name)
		}
	}
	request := doc.Components.Schemas["EnterpriseTripReviewDecisionRequest"].Value
	clone := func(edit func(map[string]any)) map[string]any {
		v := map[string]any{}
		for k, x := range examples["EnterpriseTripReviewDecisionRequest"] {
			v[k] = x
		}
		edit(v)
		return v
	}
	for name, bad := range map[string]map[string]any{
		"open is not a decision":   clone(func(v map[string]any) { v["decision"] = "OPEN" }),
		"submit is not decided":    clone(func(v map[string]any) { v["decision"] = "SUBMIT" }),
		"cancel is not offered":    clone(func(v map[string]any) { v["decision"] = "CANCEL" }),
		"revision zero":            clone(func(v map[string]any) { v["revision"] = float64(0) }),
		"reason key omitted":       clone(func(v map[string]any) { delete(v, "reason") }),
		"revision key omitted":     clone(func(v map[string]any) { delete(v, "revision") }),
		"another schema version":   clone(func(v map[string]any) { v["schema_version"] = float64(2) }),
		"caller-selected reviewer": clone(func(v map[string]any) { v["reviewer_membership_id"] = "7b0e2a54-0000-4000-8000-000000008302" }),
	} {
		if request.VisitJSON(bad) == nil {
			t.Fatal("invalid decision request admitted", name)
		}
	}
	if request.VisitJSON(clone(func(v map[string]any) { v["decision"], v["reason"] = "APPROVE", "" })) != nil {
		t.Fatal("an approval without a reason is refused by the shape")
	}
	view := doc.Components.Schemas["EnterpriseTripReviewView"].Value
	withValues := func(key string, value any) map[string]any {
		v := map[string]any{}
		for k, x := range examples["EnterpriseTripReviewView"] {
			v[k] = x
		}
		v[key] = value
		return v
	}
	if view.VisitJSON(withValues("review_state", "COMPLETED")) == nil || view.VisitJSON(withValues("operational_state", "APPROVED")) == nil {
		t.Fatal("operational and review vocabularies are interchangeable")
	}
	for _, private := range []string{"reading", "run_clock", "coordinates", "unit_id"} {
		if view.VisitJSON(withValues(private, "private")) == nil {
			t.Fatal("private trip data admitted in the reviewer read", private)
		}
	}
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	prefix := "/v1/orgs/{org_id}/trips/{trip_id}/review/"
	for path, want := range map[string]map[string]any{
		"/v1/orgs/{org_id}/trips/review/list": {"permission": "trip.trip.review", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		prefix + "read":                       {"permission": "trip.trip.review", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		prefix + "decide":                     {"permission": "trip.trip.review", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200},
	} {
		operation := doc.Paths.Value(path).Post
		if operation == nil {
			t.Fatal("missing actual review route", path)
		}
		raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
			t.Fatal("review guard metadata drift", path, got)
		}
		for k, v := range want {
			if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
				t.Fatal("review guard metadata drift", path, k)
			}
		}
		for _, mutation := range []func(map[string]any){
			func(m map[string]any) { m["permission"] = "trip.trip.read" },
			func(m map[string]any) { m["fresh"] = !m["fresh"].(bool) },
		} {
			document, actual := companyRoutes(t)
			metadata := map[string]any{}
			for k, v := range want {
				metadata[k] = v
			}
			mutation(metadata)
			document.Paths.Value(path).Post.Extensions["x-abastevo-organization"] = metadata
			if ValidateOrganizationRoutes(actual, document) == nil {
				t.Fatal("review authority/contract drift accepted", path)
			}
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
