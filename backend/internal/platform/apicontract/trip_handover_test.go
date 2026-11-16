package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-TRIP-012: the handover, the organization's handover list and the
// member's own open trips bind their permissions on the actual router with
// exact, closed shapes; the handover request has no time or actor field and
// its result keeps the trip open (EDR-255, B-BR-EMP-394/398).
func TestTripHandoverContractShapesAndGuards(t *testing.T) {
	doc := loadSpec(t)
	examples := map[string]map[string]any{}
	for _, name := range []string{"EnterpriseTripHandoverRequest", "EnterpriseTripHandover", "EnterpriseTripHandoverResult", "EnterpriseTripHandoverPage", "EnterpriseTripOwnPage"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing handover schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
			t.Fatal("invalid handover example", name)
		}
		examples[name] = example
		injected := map[string]any{"supervisor_override": "synthetic"}
		for k, v := range example {
			injected[k] = v
		}
		if schema.Value.VisitJSON(injected) == nil {
			t.Fatal("open handover shape", name)
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
	request := doc.Components.Schemas["EnterpriseTripHandoverRequest"].Value
	for name, bad := range map[string]map[string]any{
		"a device instant":           clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["handed_over_at"] = "2026-11-15T13:00:00Z" }),
		"a caller-selected giver":    clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["from_membership_id"] = "7b0e2a54-0000-4000-8000-000000009301" }),
		"a caller-selected recorder": clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["recorder_membership_id"] = "7b0e2a54-0000-4000-8000-000000009303" }),
		"empty reason":               clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["reason"] = "" }),
		"receiver omitted":           clone("EnterpriseTripHandoverRequest", func(v map[string]any) { delete(v, "receiver_membership_id") }),
		"receiver not an identity":   clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["receiver_membership_id"] = "driver-2" }),
		"revision omitted":           clone("EnterpriseTripHandoverRequest", func(v map[string]any) { delete(v, "revision") }),
		"revision zero":              clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["revision"] = float64(0) }),
		"another schema":             clone("EnterpriseTripHandoverRequest", func(v map[string]any) { v["schema_version"] = float64(2) }),
	} {
		if request.VisitJSON(bad) == nil {
			t.Fatal("invalid handover request admitted", name)
		}
	}
	result := doc.Components.Schemas["EnterpriseTripHandoverResult"].Value
	for name, bad := range map[string]map[string]any{
		"a closed trip":       clone("EnterpriseTripHandoverResult", func(v map[string]any) { v["operational_state"] = "COMPLETED" }),
		"the start revision":  clone("EnterpriseTripHandoverResult", func(v map[string]any) { v["revision"] = float64(1) }),
		"no handover fact":    clone("EnterpriseTripHandoverResult", func(v map[string]any) { delete(v, "handover") }),
		"an unflagged replay": clone("EnterpriseTripHandoverResult", func(v map[string]any) { delete(v, "replayed") }),
	} {
		if result.VisitJSON(bad) == nil {
			t.Fatal("invalid handover result admitted", name)
		}
	}
	own := doc.Components.Schemas["EnterpriseTripOwnPage"].Value
	page := clone("EnterpriseTripOwnPage", func(map[string]any) {})
	entry := page["items"].([]any)[0].(map[string]any)
	entry["handover"] = nil
	if own.VisitJSON(page) != nil {
		t.Fatal("an own-start open trip without a handover was refused")
	}
	entry["operational_state"] = "COMPLETED"
	if own.VisitJSON(page) == nil {
		t.Fatal("a closed trip was admitted as an own open trip")
	}
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]map[string]any{
		"/v1/orgs/{org_id}/trips/{trip_id}/handover": {"permission": "trip.trip.close_for_other", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/trips/handovers/list":     {"permission": "trip.trip.close_for_other", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/trips/mine/list":          {"permission": "trip.trip.start", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
	} {
		operation := doc.Paths.Value(path).Post
		if operation == nil {
			t.Fatal("missing actual handover route", path)
		}
		raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
			t.Fatal("handover guard metadata drift", path, got)
		}
		for k, v := range want {
			if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
				t.Fatal("handover guard metadata drift", path, k)
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
				t.Fatal("handover authority/contract drift accepted", path)
			}
		}
	}
}
