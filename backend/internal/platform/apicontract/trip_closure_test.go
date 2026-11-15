package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-TRIP-014: the open-trip list and the supervisor closure bind
// trip.trip.close_for_other on the actual router with exact, closed shapes;
// the closure request has no reading, value or distance field and its result
// never carries a final reading or a distance (EDR-259, B-BR-EMP-400).
func TestTripClosureContractShapesAndGuards(t *testing.T) {
	doc := loadSpec(t)
	examples := map[string]map[string]any{}
	for _, name := range []string{"EnterpriseTripOpenListRequest", "EnterpriseTripOpenPage", "EnterpriseTripCloseRequest", "EnterpriseTripClosure", "EnterpriseTripCloseResult"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing closure schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
			t.Fatal("invalid closure example", name)
		}
		examples[name] = example
		injected := map[string]any{"supervisor_override": "synthetic"}
		for k, v := range example {
			injected[k] = v
		}
		if schema.Value.VisitJSON(injected) == nil {
			t.Fatal("open closure shape", name)
		}
	}
	clone := func(name string, edit func(map[string]any)) map[string]any {
		v := map[string]any{}
		for k, x := range examples[name] {
			v[k] = x
		}
		edit(v)
		return v
	}
	request := doc.Components.Schemas["EnterpriseTripCloseRequest"].Value
	for name, bad := range map[string]map[string]any{
		"a final reading":        clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["final_reading"] = map[string]any{"kind": "ODOMETER", "value": "184610000"} }),
		"a meter value":          clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["value"] = "184610000" }),
		"a distance":             clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["distance"] = "290000" }),
		"empty reason":           clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["reason"] = "" }),
		"missing reason omitted": clone("EnterpriseTripCloseRequest", func(v map[string]any) { delete(v, "missing_reading_reason") }),
		"empty missing reason":   clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["missing_reading_reason"] = "" }),
		"revision omitted":       clone("EnterpriseTripCloseRequest", func(v map[string]any) { delete(v, "revision") }),
		"revision zero":          clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["revision"] = float64(0) }),
		"another schema":         clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["schema_version"] = float64(2) }),
		"caller-selected closer": clone("EnterpriseTripCloseRequest", func(v map[string]any) { v["closer_membership_id"] = "7b0e2a54-0000-4000-8000-000000008303" }),
	} {
		if request.VisitJSON(bad) == nil {
			t.Fatal("invalid closure request admitted", name)
		}
	}
	result := doc.Components.Schemas["EnterpriseTripCloseResult"].Value
	for name, bad := range map[string]map[string]any{
		"a final reading": clone("EnterpriseTripCloseResult", func(v map[string]any) { v["final_reading"] = map[string]any{"value": "184610000"} }),
		"a distance": clone("EnterpriseTripCloseResult", func(v map[string]any) {
			v["usage"] = map[string]any{"status": "MEASURED", "reason": nil, "kind": "ODOMETER", "distance": "290000"}
		}),
		"a running trip": clone("EnterpriseTripCloseResult", func(v map[string]any) { v["operational_state"] = "IN_PROGRESS" }),
	} {
		if result.VisitJSON(bad) == nil {
			t.Fatal("invented evidence admitted in the closure result", name)
		}
	}
	view := doc.Components.Schemas["EnterpriseTripReviewView"].Value
	raw, _ := json.Marshal(view.Example)
	var reviewExample map[string]any
	if json.Unmarshal(raw, &reviewExample) != nil {
		t.Fatal("review example unreadable")
	}
	reviewExample["closure"], reviewExample["finished_at"] = examples["EnterpriseTripClosure"], nil
	if view.VisitJSON(reviewExample) != nil {
		t.Fatal("the reviewer read refuses a closed trip's closure")
	}
	reviewExample["closure"] = map[string]any{"closed_at": "2026-11-15T14:00:00Z", "closer_membership_id": "7b0e2a54-0000-4000-8000-000000008303", "reason": "x", "missing_reading_reason": "y", "value": "1"}
	if view.VisitJSON(reviewExample) == nil {
		t.Fatal("the reviewer read admits a closure with a reading value")
	}
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]map[string]any{
		"/v1/orgs/{org_id}/trips/open/list":       {"permission": "trip.trip.close_for_other", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/trips/{trip_id}/close": {"permission": "trip.trip.close_for_other", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200},
	} {
		operation := doc.Paths.Value(path).Post
		if operation == nil {
			t.Fatal("missing actual closure route", path)
		}
		raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
			t.Fatal("closure guard metadata drift", path, got)
		}
		for k, v := range want {
			if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
				t.Fatal("closure guard metadata drift", path, k)
			}
		}
		for _, mutation := range []func(map[string]any){
			func(m map[string]any) { m["permission"] = "trip.trip.review" },
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
				t.Fatal("closure authority/contract drift accepted", path)
			}
		}
	}
}
