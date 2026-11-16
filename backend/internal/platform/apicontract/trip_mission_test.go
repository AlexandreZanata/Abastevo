package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-TRIP-008: trip.mission.respond is an explicit owner with an exact,
// closed payload and no held path; the six mission routes bind
// trip.mission.manage (and the own list trip.trip.start) on the actual router
// with exact, closed shapes (EDR-229, EDR-231).
func TestTripMissionContractOwnerShapesAndGuards(t *testing.T) {
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
	owner, ok := owners["trip.mission.respond"]
	if !ok || owner.Schema != 1 || owner.Entity != "mission" || owner.Payload.Ref != "#/components/schemas/EnterpriseTripMissionRespondPayload" || len(owner.Results) != 2 ||
		owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
		t.Fatal("trip.mission.respond owner/outcome drift")
	}
	payload := doc.Components.Schemas["EnterpriseTripMissionRespondPayload"].Value
	for name, v := range map[string]map[string]any{"accept": {"response": "ACCEPT", "reason": nil}, "decline": {"response": "DECLINE", "reason": "Veiculo em manutencao"}} {
		if payload.VisitJSON(v) != nil {
			t.Fatal("a valid response payload is refused", name)
		}
	}
	for name, v := range map[string]map[string]any{
		"unknown response":    {"response": "ACCEPTED", "reason": nil},
		"reason key omitted":  {"response": "ACCEPT"},
		"caller-chosen asset": {"response": "ACCEPT", "reason": nil, "asset_id": "7b0e2a54-0000-4000-8000-000000008201"},
		"caller-chosen trip":  {"response": "ACCEPT", "reason": nil, "trip_id": "7b0e2a54-0000-4000-8000-000000008701"},
		"empty reason":        {"response": "DECLINE", "reason": ""},
	} {
		if payload.VisitJSON(v) == nil {
			t.Fatal("an invalid response payload was admitted", name)
		}
	}
	for _, name := range []string{"EnterpriseMissionCreateRequest", "EnterpriseMissionAssignRequest", "EnterpriseMissionCancelRequest", "EnterpriseMissionListRequest", "EnterpriseMissionReadRequest",
		"EnterpriseMissionAck", "EnterpriseMissionPage", "EnterpriseMissionView", "EnterpriseTripMissionRespondPayload", "EnterpriseMissionPlanRequest", "EnterpriseMissionPlan", "EnterpriseMissionVehicleRequest",
		"EnterpriseMissionReservationRequest"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing mission schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
			t.Fatal("invalid mission example", name)
		}
		injected := map[string]any{"caller_authority": "synthetic"}
		for k, v := range example {
			injected[k] = v
		}
		if schema.Value.VisitJSON(injected) == nil {
			t.Fatal("open mission shape", name)
		}
	}
	view := doc.Components.Schemas["EnterpriseMissionView"].Value
	for _, state := range []string{"APPROVED", "PAUSED", "SUBMITTED", "SAVED_ON_DEVICE"} {
		example := map[string]any{}
		raw, _ := json.Marshal(view.Example)
		_ = json.Unmarshal(raw, &example)
		example["state"] = state
		if view.VisitJSON(example) == nil {
			t.Fatal("a review, trip or sync state was admitted as a mission state", state)
		}
	}
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	prefix := "/v1/orgs/{org_id}/missions/"
	read := map[string]any{"permission": "trip.mission.manage", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	command := map[string]any{"permission": "trip.mission.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200}
	for path, want := range map[string]map[string]any{
		prefix + "create":                      {"permission": "trip.mission.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 201},
		prefix + "list":                        read,
		prefix + "{mission_id}/read":           read,
		prefix + "{mission_id}/assign":         command,
		prefix + "{mission_id}/cancel":         command,
		prefix + "{mission_id}/plan":           command,
		prefix + "{mission_id}/vehicle/change": command,
		// EMP-TRIP-023 (EDR-266): the mission's reservation request.
		prefix + "{mission_id}/reservations/request": {"permission": "trip.mission.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 201},
		prefix + "mine/list":                         {"permission": "trip.trip.start", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
	} {
		operation := doc.Paths.Value(path).Post
		if operation == nil {
			t.Fatal("missing actual mission route", path)
		}
		raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
			t.Fatal("mission guard metadata drift", path, got)
		}
		for k, v := range want {
			if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
				t.Fatal("mission guard metadata drift", path, k)
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
				t.Fatal("mission authority/contract drift accepted", path)
			}
		}
	}
}

// EMP-TRIP-009 (B-BR-EMP-382, EDR-249): the plan body is closed — a
// destination is exactly a text or a unit, passengers carry only a count and
// no identity field fits anywhere; the read projections carry the plan.
func TestTripMissionPlanContractShapes(t *testing.T) {
	doc := loadSpec(t)
	request := doc.Components.Schemas["EnterpriseMissionPlanRequest"].Value
	base := func(destination, load map[string]any) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000008804", "revision": 1, "destination": destination, "reason": "Entrega", "load": load}
	}
	visit := func(v map[string]any) error {
		raw, _ := json.Marshal(v)
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		return request.VisitJSON(decoded)
	}
	text := map[string]any{"kind": "TEXT", "text": "Centro de distribuição Norte"}
	unit := map[string]any{"kind": "UNIT", "unit_id": "7b0e2a54-0000-4000-8000-000000008102"}
	for name, v := range map[string]map[string]any{
		"text-cargo":      base(text, map[string]any{"kind": "CARGO", "description": "Peças"}),
		"unit-passengers": base(unit, map[string]any{"kind": "PASSENGERS", "count": 999}),
		"none":            base(text, map[string]any{"kind": "NONE"}),
	} {
		if err := visit(v); err != nil {
			t.Fatal("a valid plan request is refused", name, err)
		}
	}
	for name, v := range map[string]map[string]any{
		"passenger-names":       base(text, map[string]any{"kind": "PASSENGERS", "count": 2, "names": []any{"Pessoa"}}),
		"passenger-description": base(text, map[string]any{"kind": "PASSENGERS", "count": 2, "description": "Equipe"}),
		"passenger-count-zero":  base(text, map[string]any{"kind": "PASSENGERS", "count": 0}),
		"passenger-count-1000":  base(text, map[string]any{"kind": "PASSENGERS", "count": 1000}),
		"cargo-with-count":      base(text, map[string]any{"kind": "CARGO", "description": "Peças", "count": 2}),
		"cargo-without-text":    base(text, map[string]any{"kind": "CARGO"}),
		"unknown-load":          base(text, map[string]any{"kind": "BOTH"}),
		"both-destinations":     base(map[string]any{"kind": "TEXT", "text": "X", "unit_id": "7b0e2a54-0000-4000-8000-000000008102"}, map[string]any{"kind": "NONE"}),
		"coordinates":           base(map[string]any{"kind": "TEXT", "text": "X", "latitude": -15.6}, map[string]any{"kind": "NONE"}),
		"customer-person": func() map[string]any {
			v := base(text, map[string]any{"kind": "NONE"})
			v["customer_name"] = "Pessoa"
			return v
		}(),
		"missing-reason": func() map[string]any { v := base(text, map[string]any{"kind": "NONE"}); delete(v, "reason"); return v }(),
		"missing-revision": func() map[string]any {
			v := base(text, map[string]any{"kind": "NONE"})
			delete(v, "revision")
			return v
		}(),
	} {
		if visit(v) == nil {
			t.Fatal("an invalid plan request was admitted", name)
		}
	}
	for _, name := range []string{"EnterpriseMissionView", "EnterpriseMissionItem"} {
		plan := doc.Components.Schemas[name].Value.Properties["plan"]
		if plan == nil || plan.Ref != "#/components/schemas/EnterpriseMissionPlan" {
			t.Fatal("mission projection without the plan", name)
		}
	}
}

// EMP-TRIP-023 (B-BR-EMP-414): the reservation request body is closed — no
// asset, window, employee or confirmation is chosen by the caller (they come
// from the mission and the fleet confirmation route); the ack names the
// reservation.
func TestTripMissionReservationContractShapes(t *testing.T) {
	doc := loadSpec(t)
	schema := doc.Components.Schemas["EnterpriseMissionReservationRequest"].Value
	request := struct{ VisitJSON func(map[string]any) error }{func(v map[string]any) error {
		raw, _ := json.Marshal(v)
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		return schema.VisitJSON(decoded)
	}}
	base := map[string]any{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000008806", "revision": 3, "purpose": "Entrega"}
	if request.VisitJSON(base) != nil {
		t.Fatal("a valid reservation request is refused")
	}
	for name, extra := range map[string]map[string]any{
		"caller-chosen asset":    {"asset_id": "7b0e2a54-0000-4000-8000-000000008202"},
		"caller-chosen window":   {"starts_at": "2026-11-20T11:00:00Z"},
		"caller-chosen employee": {"employee_id": "7b0e2a54-0000-4000-8000-000000008301"},
		"self confirmation":      {"confirm": true},
	} {
		v := map[string]any{}
		for k, x := range base {
			v[k] = x
		}
		for k, x := range extra {
			v[k] = x
		}
		if request.VisitJSON(v) == nil {
			t.Fatal("an open reservation request was admitted", name)
		}
	}
	for _, missing := range []string{"revision", "purpose"} {
		v := map[string]any{}
		for k, x := range base {
			if k != missing {
				v[k] = x
			}
		}
		if request.VisitJSON(v) == nil {
			t.Fatal("a reservation request without its field was admitted", missing)
		}
	}
	ack := doc.Components.Schemas["EnterpriseMissionAck"].Value
	raw, _ := json.Marshal(map[string]any{"mission_id": "7b0e2a54-0000-4000-8000-000000008901", "revision": 4, "state": "ACCEPTED", "reservation_id": "7b0e2a54-0000-4000-8000-000000008951", "replayed": false, "data_mode": "SYNTHETIC"})
	var example any
	if json.Unmarshal(raw, &example) != nil || ack.VisitJSON(example) != nil {
		t.Fatal("the reservation ack is refused")
	}
}
