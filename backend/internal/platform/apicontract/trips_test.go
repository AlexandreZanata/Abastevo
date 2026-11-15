package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
)

// EMP-TRIP-002 (EDR-202): the trip.start and trip.finish owners and payloads.
func TestTripOwnersAreExplicitAtomicReviewOwnersWithExactPayloads(t *testing.T) {
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
	for kind, schemaName := range map[string]string{"trip.start": "EnterpriseTripStartPayload", "trip.finish": "EnterpriseTripFinishPayload"} {
		owner, ok := owners[kind]
		if !ok || owner.Schema != 1 || owner.Entity != "trip" || owner.Payload.Ref != "#/components/schemas/"+schemaName || len(owner.Results) != 3 ||
			owner.Results[0] != "ACCEPTED" || owner.Results[1] != "ACCEPTED_FOR_REVIEW" || owner.Results[2] != "REJECTED" || len(owner.Followups) != 3 ||
			owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["ACCEPTED_FOR_REVIEW"] != "ATOMIC_REVIEW" {
			t.Fatal("trip owner/outcome drift", kind)
		}
		schema := doc.Components.Schemas[schemaName]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing trip payload contract/example", kind)
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
			t.Fatal("invalid example", kind, err)
		}
		absent := example()
		absent["reading"], absent["missing_reading_reason"] = nil, "Painel apagado"
		if err := schema.Value.VisitJSON(absent); err != nil {
			t.Fatal("missing-reading form refused", kind, err)
		}
		for _, field := range []string{"author_membership_id", "enrollment_id", "received_at", "review_state", "state", "operation_id", "occurred_at", "unit_id", "reporting_day", "trip_id"} {
			v := example()
			v[field] = "private"
			if schema.Value.VisitJSON(v) == nil {
				t.Fatal("payload authority or server field admitted", kind, field)
			}
		}
		for _, field := range []string{"reading", "missing_reading_reason", "run_clock"} {
			v := example()
			delete(v, field)
			if schema.Value.VisitJSON(v) == nil {
				t.Fatal("nullable key may not be omitted", kind, field)
			}
		}
		for _, bad := range []map[string]any{{"boot_id": "boot 1", "elapsed_millis": "1"}, {"boot_id": "boot-a1", "elapsed_millis": "-1"}, {"boot_id": "boot-a1", "elapsed_millis": float64(1)}, {"boot_id": "boot-a1"}} {
			v := example()
			v["run_clock"] = bad
			if schema.Value.VisitJSON(v) == nil {
				t.Fatal("malformed run clock admitted", kind, bad)
			}
		}
	}
}

// EMP-TRIP-003 (EDR-207): the trip.event.record owner and its exact payload.
func TestTripEventOwnerIsAnAtomicReviewOwnerWithAnExactPayload(t *testing.T) {
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
	owner, ok := owners["trip.event.record"]
	if !ok || owner.Schema != 1 || owner.Entity != "trip_event" || owner.Payload.Ref != "#/components/schemas/EnterpriseTripEventPayload" || len(owner.Results) != 3 ||
		owner.Results[0] != "ACCEPTED" || owner.Results[1] != "ACCEPTED_FOR_REVIEW" || owner.Results[2] != "REJECTED" || len(owner.Followups) != 3 ||
		owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["ACCEPTED_FOR_REVIEW"] != "ATOMIC_REVIEW" {
		t.Fatal("trip event owner/outcome drift")
	}
	schema := doc.Components.Schemas["EnterpriseTripEventPayload"]
	if schema == nil || schema.Value == nil || schema.Value.Example == nil {
		t.Fatal("missing trip event payload contract/example")
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
	vocabulary := []string{"NOTE", "STOP", "ARRIVAL", "DEPARTURE", "FUELING", "TOLL", "PARKING", "INCIDENT", "DAMAGE", "HANDOVER", "PAUSE", "RESUME", "CANCEL"}
	if enum := schema.Value.Properties["type"].Value.Enum; len(enum) != len(vocabulary) {
		t.Fatal("logbook vocabulary drift", enum)
	}
	for _, kind := range vocabulary {
		v := example()
		v["type"], v["text"] = kind, nil
		if err := schema.Value.VisitJSON(v); err != nil {
			t.Fatal("closed logbook type refused", kind, err)
		}
	}
	for _, kind := range []string{"note", "START", "END", "FINISH", " NOTE", ""} {
		v := example()
		v["type"] = kind
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("unknown logbook type admitted", kind)
		}
	}
	for _, field := range []string{"asset_id", "author_membership_id", "enrollment_id", "received_at", "state", "operation_id", "occurred_at", "event_id", "reporting_day"} {
		v := example()
		v[field] = "private"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("payload authority or server field admitted", field)
		}
	}
	for _, field := range []string{"trip_id", "type", "text"} {
		v := example()
		delete(v, field)
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("payload key may not be omitted", field)
		}
	}
	for _, bad := range []any{"", strings.Repeat("a", 281), float64(1)} {
		v := example()
		v["text"] = bad
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("malformed text admitted", bad)
		}
	}
}

// EMP-TRIP-011 (EDR-253): the three lifecycle owners on the trip entity and
// their exact payload; a cancellation's reason is enforced at receipt.
func TestTripLifecycleOwnersAreAtomicReviewOwnersWithAnExactPayload(t *testing.T) {
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
	for _, kind := range []string{"trip.pause", "trip.resume", "trip.cancel"} {
		owner, ok := owners[kind]
		if !ok || owner.Schema != 1 || owner.Entity != "trip" || owner.Payload.Ref != "#/components/schemas/EnterpriseTripTransitionPayload" || len(owner.Results) != 3 ||
			owner.Results[0] != "ACCEPTED" || owner.Results[1] != "ACCEPTED_FOR_REVIEW" || owner.Results[2] != "REJECTED" || len(owner.Followups) != 3 ||
			owner.Followups["ACCEPTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["REJECTED"] != "NO_ASYNC_FOLLOWUP" || owner.Followups["ACCEPTED_FOR_REVIEW"] != "ATOMIC_REVIEW" {
			t.Fatal("trip lifecycle owner/outcome drift", kind)
		}
	}
	schema := doc.Components.Schemas["EnterpriseTripTransitionPayload"]
	if schema == nil || schema.Value == nil || schema.Value.Example == nil {
		t.Fatal("missing trip lifecycle payload contract/example")
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
	none := example()
	none["reason"] = nil
	if err := schema.Value.VisitJSON(none); err != nil {
		t.Fatal("a pause without reason refused", err)
	}
	if schema.Value.VisitJSON(map[string]any{}) == nil {
		t.Fatal("the nullable reason key may be omitted")
	}
	for _, field := range []string{"state", "revision", "expected_revision", "trip_id", "author_membership_id", "occurred_at", "received_at"} {
		v := example()
		v[field] = "private"
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("payload authority or server field admitted", field)
		}
	}
	for _, bad := range []any{"", strings.Repeat("a", 281), float64(1)} {
		v := example()
		v["reason"] = bad
		if schema.Value.VisitJSON(v) == nil {
			t.Fatal("malformed reason admitted", bad)
		}
	}
}
