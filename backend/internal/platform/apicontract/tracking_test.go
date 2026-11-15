package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-GPS-004 (EDR-277/278): the six gps.session.* owners and gps.track.batch
// are explicit owners on the tracking_session entity with exact, closed
// payloads, final ACCEPTED/REJECTED results and no held path or follow-up.
func TestTrackingIntakeContractOwnersAndPayloads(t *testing.T) {
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
	payloads := map[string]string{
		"gps.session.start": "EnterpriseTrackingSessionDriverPayload", "gps.session.resume": "EnterpriseTrackingSessionDriverPayload",
		"gps.session.suspend": "EnterpriseTrackingSessionEventPayload", "gps.session.interrupt": "EnterpriseTrackingSessionEventPayload",
		"gps.session.reduce": "EnterpriseTrackingSessionEventPayload", "gps.session.end": "EnterpriseTrackingSessionEventPayload",
		"gps.track.batch": "EnterpriseTrackingBatchPayload",
	}
	for kind, schema := range payloads {
		owner, ok := owners[kind]
		if !ok || owner.Schema != 1 || owner.Entity != "tracking_session" || owner.Payload.Ref != "#/components/schemas/"+schema || len(owner.Results) != 2 ||
			owner.Results[0] != "ACCEPTED" || owner.Results[1] != "REJECTED" || len(owner.Followups) != 2 || owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" {
			t.Fatal("tracking owner/outcome drift", kind)
		}
	}
	trip := "7b0e2a54-0000-4000-8000-000000008701"
	check := func(name string, valid []map[string]any, invalid map[string]map[string]any) {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing tracking schema or explicit example", name)
		}
		raw, _ := json.Marshal(schema.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || schema.Value.VisitJSON(example) != nil {
			t.Fatal("invalid tracking example", name)
		}
		for _, v := range valid {
			if err := schema.Value.VisitJSON(v); err != nil {
				t.Fatal("a valid tracking payload is refused", name, err)
			}
		}
		for label, v := range invalid {
			if schema.Value.VisitJSON(v) == nil {
				t.Fatal("an invalid tracking payload was admitted", name, label)
			}
		}
	}
	check("EnterpriseTrackingSessionDriverPayload",
		[]map[string]any{{"trip_id": trip, "origin": "TRIP_SCREEN", "permission": "NONE", "notification_visible": false}},
		map[string]map[string]any{
			"absent permission":  {"trip_id": trip, "origin": "TRIP_SCREEN", "permission": nil, "notification_visible": true},
			"unknown permission": {"trip_id": trip, "origin": "TRIP_SCREEN", "permission": "FINE", "notification_visible": true},
			"unknown origin":     {"trip_id": trip, "origin": "SCREEN", "permission": "PRECISE", "notification_visible": true},
			"position":           {"trip_id": trip, "origin": "TRIP_SCREEN", "permission": "PRECISE", "notification_visible": true, "lat_e7": 1},
			"missing notice":     {"trip_id": trip, "origin": "TRIP_SCREEN", "permission": "PRECISE"},
		})
	check("EnterpriseTrackingSessionEventPayload",
		[]map[string]any{{"trip_id": trip, "cause": "TRIP_COMPLETED"}, {"trip_id": trip, "cause": "PRECISION_REDUCED"}},
		map[string]map[string]any{
			"fix gap reason": {"trip_id": trip, "cause": "SIGNAL_LOST"},
			"opening cause":  {"trip_id": trip, "cause": "NOT_STARTED"},
			"no cause":       {"trip_id": trip},
			"extra key":      {"trip_id": trip, "cause": "PROCESS_STOPPED", "permission": "PRECISE"},
		})
	fixes := func(n int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = []any{i * 10000, 0, -12670, 8, 1389, 270, 0}
		}
		return out
	}
	batch := func(edit func(map[string]any)) map[string]any {
		v := map[string]any{"trip_id": trip, "seq": 42, "source": "PHONE", "boot_id": "boot-a1", "base_elapsed_millis": 9063000,
			"base": map[string]any{"lat_e7": -100000000, "lon_e7": -500000000}, "fixes": fixes(2), "dropped": map[string]any{"accuracy": 1, "stale": 0, "jump": 0, "mock": 0}}
		if edit != nil {
			edit(v)
		}
		return v
	}
	check("EnterpriseTrackingBatchPayload",
		[]map[string]any{batch(func(v map[string]any) { v["fixes"] = fixes(200) }), batch(func(v map[string]any) {
			v["boot_id"], v["base_elapsed_millis"] = nil, nil
			v["fixes"] = []any{[]any{0, 0, 0, 0, nil, nil, 3}}
		})},
		map[string]map[string]any{
			"no fix":            batch(func(v map[string]any) { v["fixes"] = []any{} }),
			"oversized":         batch(func(v map[string]any) { v["fixes"] = fixes(201) }),
			"short fix":         batch(func(v map[string]any) { v["fixes"] = []any{[]any{0, 0, 0, 8, 0, 0}} }),
			"fractional value":  batch(func(v map[string]any) { v["fixes"] = []any{[]any{0, 0.5, 0, 8, 0, 0, 0}} }),
			"unknown flag":      batch(func(v map[string]any) { v["fixes"] = []any{[]any{0, 0, 0, 8, 0, 0, 4}} }),
			"bearing 360":       batch(func(v map[string]any) { v["fixes"] = []any{[]any{0, 0, 0, 8, 0, 360, 0}} }),
			"zero sequence":     batch(func(v map[string]any) { v["seq"] = 0 }),
			"other source":      batch(func(v map[string]any) { v["source"] = "TELEMATICS" }),
			"base out of range": batch(func(v map[string]any) { v["base"] = map[string]any{"lat_e7": 900000001, "lon_e7": 0} }),
			"open dropped":      batch(func(v map[string]any) { v["dropped"].(map[string]any)["extra"] = 1 }),
			"caller precision":  batch(func(v map[string]any) { v["precision"] = "FINE" }),
		})
}
