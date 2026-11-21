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

// EMP-GPS-012 (EDR-281): the recorded-track read is an actual company route
// under gps.track.read only, with a closed page request and a closed view
// that carries no live or derived value.
func TestTrackReadContractRouteAndView(t *testing.T) {
	doc := loadSpec(t)
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	path := "/v1/orgs/{org_id}/trips/{trip_id}/track/read"
	want := map[string]any{"permission": "gps.track.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200}
	operation := doc.Paths.Value(path).Post
	if operation == nil {
		t.Fatal("missing actual track route")
	}
	raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
	var got map[string]any
	if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
		t.Fatal("track guard metadata drift", got)
	}
	for k, v := range want {
		if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
			t.Fatal("track guard metadata drift", k)
		}
	}
	for _, permission := range []string{"trip.trip.read", "gps.location.read_live"} {
		document, actual := companyRoutes(t)
		metadata := map[string]any{}
		for k, v := range want {
			metadata[k] = v
		}
		metadata["permission"] = permission
		document.Paths.Value(path).Post.Extensions["x-abastevo-organization"] = metadata
		if ValidateOrganizationRoutes(actual, document) == nil {
			t.Fatal("track authority derived from another permission", permission)
		}
	}
	request := doc.Components.Schemas["EnterpriseTrackReadRequest"].Value
	for _, v := range []map[string]any{{}, {"limit": 20}, {"limit": 1, "cursor": "opaque"}} {
		if err := request.VisitJSON(v); err != nil {
			t.Fatal("a valid track page request is refused", err)
		}
	}
	for name, v := range map[string]map[string]any{"limit 21": {"limit": 21}, "limit 0": {"limit": 0}, "trip driver": {"trip_driver": "x"}, "live": {"live": true}} {
		if request.VisitJSON(v) == nil {
			t.Fatal("an invalid track page request was admitted", name)
		}
	}
	view := doc.Components.Schemas["EnterpriseTrackView"].Value
	example := func() map[string]any {
		raw, _ := json.Marshal(view.Example)
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil {
			t.Fatal("track view example")
		}
		return v
	}
	if err := view.VisitJSON(example()); err != nil {
		t.Fatal("invalid track view example", err)
	}
	untracked := example()
	untracked["session"], untracked["segments"] = nil, []any{}
	if err := view.VisitJSON(untracked); err != nil {
		t.Fatal("an untracked trip page is refused", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"live position": func(v map[string]any) { v["live_position"] = map[string]any{} },
		"distance":      func(v map[string]any) { v["gps_distance_m"] = 1 },
		"freshness":     func(v map[string]any) { v["freshness"] = "CURRENT" },
		"latitude":      func(v map[string]any) { segmentFix(v)["lat_e7"] = 900000001 },
		"fix address":   func(v map[string]any) { segmentFix(v)["address"] = "Rua" },
		"other source":  func(v map[string]any) { v["segments"].([]any)[0].(map[string]any)["source"] = "TELEMATICS" },
		"too many segments": func(v map[string]any) {
			list := v["segments"].([]any)
			for len(list) <= 20 {
				list = append(list, list[0])
			}
			v["segments"] = list
		},
	} {
		v := example()
		mutate(v)
		if view.VisitJSON(v) == nil {
			t.Fatal("an invalid track view was admitted", name)
		}
	}
}

func segmentFix(v map[string]any) map[string]any {
	return v["segments"].([]any)[0].(map[string]any)["fixes"].([]any)[0].(map[string]any)
}

// EDR-302 (B-BR-EMP-467..469): the tracking policy read and change are
// actual company routes under org.settings.read and fresh
// org.settings.manage, with closed request and view schemas that carry no
// notice text, session, member or location field.
func TestTrackingPolicyContractRoutesAndSchemas(t *testing.T) {
	doc := loadSpec(t)
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]map[string]any{
		"/v1/orgs/{org_id}/tracking/policy/read":   {"permission": "org.settings.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/tracking/policy/change": {"permission": "org.settings.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 200},
		// EMP-GPS-002: the member's own read of the policy in force.
		"/v1/orgs/{org_id}/tracking/policy/mine": {"permission": "trip.trip.start", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
	} {
		operation := doc.Paths.Value(path).Post
		if operation == nil {
			t.Fatal("missing actual tracking policy route", path)
		}
		raw, _ := json.Marshal(operation.Extensions["x-abastevo-organization"])
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
			t.Fatal("tracking policy guard metadata drift", path, got)
		}
		for k, v := range want {
			if want, _ := json.Marshal(v); string(want) != string(mustJSON(t, got[k])) {
				t.Fatal("tracking policy guard metadata drift", path, k)
			}
		}
		for _, permission := range []string{"gps.track.read", "trip.trip.start", "org.structure.manage", "org.settings.read"} {
			if permission == want["permission"] {
				continue
			}
			document, actual := companyRoutes(t)
			metadata := map[string]any{}
			for k, v := range want {
				metadata[k] = v
			}
			metadata["permission"] = permission
			document.Paths.Value(path).Post.Extensions["x-abastevo-organization"] = metadata
			if ValidateOrganizationRoutes(actual, document) == nil {
				t.Fatal("tracking policy authority derived from another permission", path, permission)
			}
		}
	}
	schema := func(name string) map[string]any {
		s := doc.Components.Schemas[name]
		if s == nil || s.Value == nil || s.Value.Example == nil {
			t.Fatal("missing tracking policy schema or example", name)
		}
		raw, _ := json.Marshal(s.Value.Example)
		var example map[string]any
		if json.Unmarshal(raw, &example) != nil || s.Value.VisitJSON(example) != nil {
			t.Fatal("invalid tracking policy example", name)
		}
		return example
	}
	visit := func(name string, v map[string]any) error { return doc.Components.Schemas[name].Value.VisitJSON(v) }
	change := "EnterpriseTrackingPolicyChangeRequest"
	base := schema(change)
	with := func(edit func(map[string]any)) map[string]any {
		v := map[string]any{}
		for k, x := range base {
			v[k] = x
		}
		edit(v)
		return v
	}
	for name, v := range map[string]map[string]any{
		"first version": with(func(v map[string]any) { delete(v, "revision") }),
		"off":           with(func(v map[string]any) { v["policy"], v["notice_version"], v["notice_in_force_at"] = "OFF", nil, nil }),
		"optional":      with(func(v map[string]any) { v["policy"] = "OPTIONAL" }),
	} {
		if err := visit(change, v); err != nil {
			t.Fatal("a valid tracking policy change is refused", name, err)
		}
	}
	for name, v := range map[string]map[string]any{
		"zero revision":   with(func(v map[string]any) { v["revision"] = 0 }),
		"unknown policy":  with(func(v map[string]any) { v["policy"] = "ON" }),
		"notice text":     with(func(v map[string]any) { v["notice_text"] = "Aviso" }),
		"upper version":   with(func(v map[string]any) { v["notice_version"] = "Notice-1" }),
		"no operation":    with(func(v map[string]any) { delete(v, "operation_id") }),
		"schema 2":        with(func(v map[string]any) { v["schema_version"] = 2 }),
		"session":         with(func(v map[string]any) { v["session_id"] = "7b0e2a54-0000-4000-8000-000000008702" }),
		"no policy":       with(func(v map[string]any) { delete(v, "policy") }),
		"numeric version": with(func(v map[string]any) { v["notice_version"] = 1 }),
	} {
		if visit(change, v) == nil {
			t.Fatal("an invalid tracking policy change was admitted", name)
		}
	}
	if visit("EnterpriseTrackingPolicyReadRequest", map[string]any{}) != nil || visit("EnterpriseTrackingPolicyReadRequest", map[string]any{"revision": 1}) == nil {
		t.Fatal("the tracking policy read request is not closed")
	}
	ack := schema("EnterpriseTrackingPolicyAcknowledgement")
	view := schema("EnterpriseTrackingPolicyView")
	empty := map[string]any{"revision": 0, "policy": "OFF", "notice_version": nil, "notice_in_force_at": nil, "effective_at": nil, "in_force": map[string]any{"policy": "OFF", "revision": 0}, "data_mode": "SYNTHETIC"}
	if err := visit("EnterpriseTrackingPolicyView", empty); err != nil {
		t.Fatal("the empty policy view is refused", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"session":      func(v map[string]any) { v["sessions"] = []any{} },
		"notice text":  func(v map[string]any) { v["notice_text"] = "x" },
		"other policy": func(v map[string]any) { v["policy"] = "ON" },
	} {
		for label, base := range map[string]map[string]any{"ack": ack, "view": view} {
			v := map[string]any{}
			for k, x := range base {
				v[k] = x
			}
			mutate(v)
			target := "EnterpriseTrackingPolicyAcknowledgement"
			if label == "view" {
				target = "EnterpriseTrackingPolicyView"
			}
			if visit(target, v) == nil {
				t.Fatal("an invalid tracking policy result was admitted", label, name)
			}
		}
	}
	// EMP-GPS-002: the member's own read is the policy in force only, closed,
	// with no member, session, notice text or location.
	mine := "EnterpriseTrackingPolicyInForce"
	inForce := schema(mine)
	if mineOp := doc.Paths.Value("/v1/orgs/{org_id}/tracking/policy/mine").Post; mineOp.RequestBody == nil ||
		mineOp.RequestBody.Value.Content.Get("application/json").Schema.Ref != "#/components/schemas/EnterpriseTrackingPolicyReadRequest" ||
		mineOp.Responses.Value("200").Value.Content.Get("application/json").Schema.Ref != "#/components/schemas/"+mine {
		t.Fatal("the own policy read route does not use its closed request and view")
	}
	for name, v := range map[string]map[string]any{
		"nothing recorded": {"revision": 0, "policy": "OFF", "notice_version": nil, "notice_in_force_at": nil, "data_mode": "SYNTHETIC"},
		"turned off":       {"revision": 3, "policy": "OFF", "notice_version": nil, "notice_in_force_at": nil, "data_mode": "SYNTHETIC"},
		"optional":         {"revision": 2, "policy": "OPTIONAL", "notice_version": "notice-2026.11", "notice_in_force_at": "2026-11-01T00:00:00Z", "data_mode": "SYNTHETIC"},
	} {
		if err := visit(mine, v); err != nil {
			t.Fatal("a valid policy in force is refused", name, err)
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"member":       func(v map[string]any) { v["membership_id"] = "7b0e2a54-0000-4000-8000-000000000601" },
		"session":      func(v map[string]any) { v["sessions"] = []any{} },
		"notice text":  func(v map[string]any) { v["notice_text"] = "x" },
		"position":     func(v map[string]any) { v["lat_e7"] = 1 },
		"other policy": func(v map[string]any) { v["policy"] = "ON" },
		"stored view":  func(v map[string]any) { v["in_force"] = map[string]any{"policy": "OFF", "revision": 0} },
		"no revision":  func(v map[string]any) { delete(v, "revision") },
		"no notice":    func(v map[string]any) { delete(v, "notice_version") },
	} {
		v := map[string]any{}
		for k, x := range inForce {
			v[k] = x
		}
		mutate(v)
		if visit(mine, v) == nil {
			t.Fatal("an invalid policy in force was admitted", name)
		}
	}
}

func TestTrackingF5RoutesAndMetadata(t *testing.T) {
	doc := loadSpec(t)
	_, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	expected := map[string]map[string]any{
		"/v1/orgs/{org_id}/trips/{trip_id}/location/live/read":      {"permission": "gps.location.read_live", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/tracking/live/list":                      {"permission": "gps.location.read_live", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/trips/{trip_id}/track/replay":            {"permission": "gps.track.read", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/tracking/geofences/list":                 {"permission": "gps.geofence.manage", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/tracking/geofences/create":               {"permission": "gps.geofence.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/tracking/geofences/{geofence_id}/change": {"permission": "gps.geofence.manage", "write": true, "fresh": true, "require_revision": true, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/routes/plans/read":                       {"permission": "route.plan.manage", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/routes/plans/save":                       {"permission": "route.plan.manage", "write": true, "fresh": true, "require_revision": false, "company_only": true, "success": 200},
		"/v1/orgs/{org_id}/routes/suggestions/evaluate":             {"permission": "route.plan.manage", "write": false, "fresh": false, "require_revision": false, "company_only": true, "success": 200},
	}
	for path, want := range expected {
		p := doc.Paths.Value(path)
		if p == nil || p.Post == nil {
			t.Fatal("missing F5 route in OpenAPI", path)
		}
		raw, _ := json.Marshal(p.Post.Extensions["x-abastevo-organization"])
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil || len(got) != len(want) {
			t.Fatal("F5 route guard metadata drift", path, got)
		}
		for k, v := range want {
			if wantJSON, _ := json.Marshal(v); string(wantJSON) != string(mustJSON(t, got[k])) {
				t.Fatal("F5 route guard metadata mismatch", path, k, string(wantJSON), string(mustJSON(t, got[k])))
			}
		}
	}
}
