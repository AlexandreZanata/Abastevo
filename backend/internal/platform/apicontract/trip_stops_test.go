package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-TRIP-010 (B-BR-EMP-475..478, EDR-307/309): planned stops are closed
// free-text places on PLAN and on the plan projection; the own-trip read binds
// trip.trip.start on the actual router with exact, closed shapes and carries
// no distance, duration or pairing.
func TestTripStopsContractShapesAndGuards(t *testing.T) {
	doc := loadSpec(t)
	visit := func(name string, v any) error {
		raw, _ := json.Marshal(v)
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		return doc.Components.Schemas[name].Value.VisitJSON(decoded)
	}
	examples := map[string]map[string]any{}
	for _, name := range []string{"EnterpriseMissionPlannedStopRequest", "EnterpriseMissionPlannedStop", "EnterpriseTripStopsReadRequest", "EnterpriseTripStopsView"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing stops schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || visit(name, example) != nil {
			t.Fatal("invalid stops example", name)
		}
		examples[name] = example
		injected := map[string]any{"distance_m": 1200}
		for k, v := range example {
			injected[k] = v
		}
		if visit(name, injected) == nil {
			t.Fatal("open stops shape", name)
		}
	}
	plan := func(stops any) map[string]any {
		v := map[string]any{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000008804", "revision": 1,
			"destination": map[string]any{"kind": "TEXT", "text": "Centro de distribuição Norte"}, "reason": "Entrega", "load": map[string]any{"kind": "NONE"}}
		if stops != nil {
			v["stops"] = stops
		}
		return v
	}
	stop := func(text string) map[string]any { return map[string]any{"kind": "TEXT", "text": text} }
	twenty, twentyOne := []any{}, []any{}
	for range 20 {
		twenty = append(twenty, stop("Parada"))
	}
	twentyOne = append(append(twentyOne, twenty...), stop("Parada"))
	for name, v := range map[string]map[string]any{"no-stops": plan(nil), "two-stops": plan([]any{stop("Cliente Jardim Primavera"), stop("Posto de apoio BR-163")}), "twenty": plan(twenty)} {
		if err := visit("EnterpriseMissionPlanRequest", v); err != nil {
			t.Fatal("a valid plan request with stops is refused", name, err)
		}
	}
	for name, v := range map[string]map[string]any{
		"empty-list":      plan([]any{}),
		"twenty-one":      plan(twentyOne),
		"unit-stop":       plan([]any{map[string]any{"kind": "UNIT", "unit_id": "7b0e2a54-0000-4000-8000-000000008102"}}),
		"coordinates":     plan([]any{map[string]any{"kind": "TEXT", "text": "X", "latitude": -15.6}}),
		"planned-time":    plan([]any{map[string]any{"kind": "TEXT", "text": "X", "arrive_at": "2026-11-15T13:00:00Z"}}),
		"contact":         plan([]any{map[string]any{"kind": "TEXT", "text": "X", "contact": "Pessoa"}}),
		"empty-text":      plan([]any{stop("")}),
		"bare-string":     plan([]any{"Cliente"}),
		"missing-kind":    plan([]any{map[string]any{"text": "X"}}),
		"sequence-chosen": plan([]any{map[string]any{"kind": "TEXT", "text": "X", "sequence": 1}}),
	} {
		if visit("EnterpriseMissionPlanRequest", v) == nil {
			t.Fatal("an invalid planned stop was admitted", name)
		}
	}
	projected := map[string]any{"revision": 2, "destination": map[string]any{"kind": "TEXT", "text": "X"}, "reason": "Entrega", "load": map[string]any{"kind": "NONE"},
		"planned_at": "2026-11-15T10:10:00Z", "stops": []any{map[string]any{"sequence": 1, "kind": "TEXT", "text": "Cliente"}}}
	if err := visit("EnterpriseMissionPlan", projected); err != nil {
		t.Fatal("a plan projection with stops is refused", err)
	}
	projected["stops"] = []any{}
	if visit("EnterpriseMissionPlan", projected) == nil {
		t.Fatal("a plan projection with an empty stop list was admitted")
	}
	clone := func(edit func(map[string]any)) map[string]any {
		raw, _ := json.Marshal(examples["EnterpriseTripStopsView"])
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		edit(v)
		return v
	}
	observed := func(v map[string]any) map[string]any { return v["observed"].(map[string]any) }
	for name, v := range map[string]map[string]any{
		"no planned side": clone(func(v map[string]any) { v["planned"] = nil }),
		"no end":          clone(func(v map[string]any) { v["end"] = nil; v["operational_state"] = "IN_PROGRESS" }),
		"an observation after the end": clone(func(v map[string]any) {
			observed(v)["after_end"] = []any{observed(v)["stops"].([]any)[0].(map[string]any)["arrival"]}
		}),
	} {
		if err := visit("EnterpriseTripStopsView", v); err != nil {
			t.Fatal("a valid stops view is refused", name, err)
		}
	}
	for name, v := range map[string]map[string]any{
		"a planned leg distance": clone(func(v map[string]any) {
			v["planned"].(map[string]any)["legs"].([]any)[0].(map[string]any)["distance_m"] = 1000
		}),
		"an observed leg duration": clone(func(v map[string]any) { observed(v)["legs"].([]any)[0].(map[string]any)["duration_s"] = 60 }),
		"a pairing with a planned stop": clone(func(v map[string]any) {
			observed(v)["stops"].([]any)[0].(map[string]any)["planned_sequence"] = 1
		}),
		"an invented issue":       clone(func(v map[string]any) { observed(v)["stops"].([]any)[0].(map[string]any)["issue"] = "LATE" }),
		"a boundary without zone": clone(func(v map[string]any) { delete(v["start"].(map[string]any), "entry_zone") }),
		"a boundary without flag": clone(func(v map[string]any) { delete(v["start"].(map[string]any), "after_receipt") }),
		"a held accepted code":    clone(func(v map[string]any) { observed(v)["held"].([]any)[0].(map[string]any)["code"] = "ACCEPTED" }),
		"a held note":             clone(func(v map[string]any) { observed(v)["held"].([]any)[0].(map[string]any)["type"] = "NOTE" }),
		"no observed legs":        clone(func(v map[string]any) { observed(v)["legs"] = []any{} }),
		"no after-end list":       clone(func(v map[string]any) { delete(observed(v), "after_end") }),
		"a held fact after the end": clone(func(v map[string]any) {
			observed(v)["after_end"] = []any{observed(v)["held"].([]any)[0]}
		}),
		"a coordinate on the stop": clone(func(v map[string]any) { observed(v)["stops"].([]any)[0].(map[string]any)["latitude"] = -15.6 }),
	} {
		if visit("EnterpriseTripStopsView", v) == nil {
			t.Fatal("an invalid stops view was admitted", name)
		}
	}
	if visit("EnterpriseTripStopsReadRequest", map[string]any{"membership_id": "7b0e2a54-0000-4000-8000-000000009301"}) == nil {
		t.Fatal("the stops read accepts a caller-selected member")
	}
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	path := "/v1/orgs/{org_id}/trips/mine/{trip_id}/stops/read"
	want := map[string]any{"permission": "trip.trip.start", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	operation := doc.Paths.Value(path).Post
	if operation == nil {
		t.Fatal("missing actual stops route", path)
	}
	raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
	var got map[string]any
	if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
		t.Fatal("stops guard metadata drift", got)
	}
	for k, v := range want {
		if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
			t.Fatal("stops guard metadata drift", k)
		}
	}
	for _, mutation := range []func(map[string]any){
		func(m map[string]any) { m["permission"] = "trip.trip.read" },
		func(m map[string]any) { m["permission"] = "trip.mission.manage" },
		func(m map[string]any) { m["fresh"] = true },
	} {
		document, actual := companyRoutes(t)
		metadata := map[string]any{}
		for k, v := range want {
			metadata[k] = v
		}
		mutation(metadata)
		document.Paths.Value(path).Post.Extensions["x-abastevo-organization"] = metadata
		if ValidateOrganizationRoutes(actual, document) == nil {
			t.Fatal("stops authority/contract drift accepted")
		}
	}
}
