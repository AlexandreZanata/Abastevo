package apicontract

import (
	"encoding/json"
	"testing"
)

// EMP-TRIP-007 (EDR-263): the three conflict routes bind trip.trip.review
// with exact closed shapes; the view admits only held trip owners and codes.
func TestTripConflictContractShapesAndGuards(t *testing.T) {
	doc := loadSpec(t)
	for path, want := range map[string]struct {
		write           bool
		request, result string
	}{
		"/v1/orgs/{org_id}/sync/reviews/trips/list":                     {false, "EnterpriseMeterReviewListRequest", "EnterpriseTripConflictPage"},
		"/v1/orgs/{org_id}/sync/reviews/trips/{review_item_id}/read":    {false, "EnterpriseMeterReadRequest", "EnterpriseTripConflictView"},
		"/v1/orgs/{org_id}/sync/reviews/trips/{review_item_id}/resolve": {true, "EnterpriseMeterReviewResolveRequest", "EnterpriseMeterReviewResolveResult"},
	} {
		item := doc.Paths.Value(path)
		if item == nil || item.Post == nil {
			t.Fatal("conflict route missing", path)
		}
		raw, _ := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
		var rule struct {
			Permission      string `json:"permission"`
			Write           bool   `json:"write"`
			Fresh           bool   `json:"fresh"`
			RequireRevision bool   `json:"require_revision"`
			CompanyOnly     bool   `json:"company_only"`
			Success         int    `json:"success"`
		}
		if json.Unmarshal(raw, &rule) != nil || rule.Permission != "trip.trip.review" || rule.Write != want.write || rule.Fresh != want.write || rule.RequireRevision != want.write || !rule.CompanyOnly || rule.Success != 200 {
			t.Fatal("conflict route guard drift", path, string(raw))
		}
		request := item.Post.RequestBody.Value.Content.Get("application/json")
		response := item.Post.Responses.Value("200").Value.Content.Get("application/json")
		if request.Schema.Ref != "#/components/schemas/"+want.request || response.Schema.Ref != "#/components/schemas/"+want.result {
			t.Fatal("conflict route schema drift", path)
		}
		for name, c := range map[string]struct {
			schema  string
			example any
		}{"request": {want.request, request.Example}, "response": {want.result, response.Example}} {
			raw, _ := json.Marshal(c.example)
			var value map[string]any
			if json.Unmarshal(raw, &value) != nil || doc.Components.Schemas[c.schema].Value.VisitJSON(value) != nil {
				t.Fatal("invalid conflict example", path, name)
			}
			value["reviewer_override"] = "synthetic"
			if doc.Components.Schemas[c.schema].Value.VisitJSON(value) == nil {
				t.Fatal("open conflict shape", path, name)
			}
		}
	}
	read := doc.Paths.Value("/v1/orgs/{org_id}/sync/reviews/trips/{review_item_id}/read").Post.Responses.Value("200").Value.Content.Get("application/json")
	raw, _ := json.Marshal(read.Example)
	view := doc.Components.Schemas["EnterpriseTripConflictView"].Value
	edit := func(change func(map[string]any)) map[string]any {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		change(v)
		return v
	}
	for name, bad := range map[string]map[string]any{
		"meter owner":          edit(func(v map[string]any) { v["operation_type"] = "meter.reading.record" }),
		"meter code":           edit(func(v map[string]any) { v["code"] = "ACCESS_ENDED_BEFORE_RECEIPT" }),
		"third revision":       edit(func(v map[string]any) { v["revision"] = float64(3) }),
		"no conflicting trips": edit(func(v map[string]any) { delete(v, "conflicting_trips") }),
		"fact without reading": edit(func(v map[string]any) { delete(v["fact"].(map[string]any), "reading") }),
		"open fact":            edit(func(v map[string]any) { v["fact"].(map[string]any)["preview"] = "x" }),
		"unknown trip state": edit(func(v map[string]any) {
			v["conflicting_trips"].([]any)[0].(map[string]any)["operational_state"] = "PLANNED"
		}),
	} {
		if view.VisitJSON(bad) == nil {
			t.Fatal("conflict view admitted", name)
		}
	}
}
