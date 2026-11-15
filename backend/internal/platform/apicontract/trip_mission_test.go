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
		"EnterpriseMissionAck", "EnterpriseMissionPage", "EnterpriseMissionView", "EnterpriseTripMissionRespondPayload"} {
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
		prefix + "create":              {"permission": "trip.mission.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 201},
		prefix + "list":                read,
		prefix + "{mission_id}/read":   read,
		prefix + "{mission_id}/assign": command,
		prefix + "{mission_id}/cancel": command,
		prefix + "mine/list":           {"permission": "trip.trip.start", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
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
