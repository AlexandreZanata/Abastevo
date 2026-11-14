package apicontract

import (
	"encoding/json"
	"testing"
)

func TestMeterReviewContractExamplesAndImmutableOutcomes(t *testing.T) {
	doc := loadSpec(t)
	for _, path := range []string{"/v1/orgs/{org_id}/sync/reviews/meter-readings/list", "/v1/orgs/{org_id}/sync/reviews/meter-readings/{review_item_id}/read", "/v1/orgs/{org_id}/sync/reviews/meter-readings/{review_item_id}/resolve", "/v1/orgs/{org_id}/sync/review-results"} {
		if doc.Paths.Value(path) == nil || doc.Paths.Value(path).Post == nil {
			t.Fatal("review operation was misplaced outside actual OpenAPI paths", path)
		}
	}
	for _, name := range []string{"EnterpriseReviewResolution", "EnterpriseMeterReviewFact", "EnterpriseMeterReviewView", "EnterpriseMeterReviewListRequest", "EnterpriseMeterReviewPage", "EnterpriseMeterReviewResolveRequest", "EnterpriseMeterReviewResolveResult", "EnterpriseReviewResultsRequest", "EnterpriseReviewResultsResponse"} {
		t.Run(name, func(t *testing.T) {
			s := doc.Components.Schemas[name]
			if s == nil || s.Value == nil || s.Value.Example == nil {
				t.Fatal("missing actual review contract example")
			}
			if e := s.Value.VisitJSON(s.Value.Example); e != nil {
				t.Fatal("invalid review example", e)
			}
		})
	}
	clone := func(name string) map[string]any {
		t.Helper()
		raw, e := json.Marshal(doc.Components.Schemas[name].Value.Example)
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	for name, edit := range map[string]func(map[string]any){"accepted-null": func(v map[string]any) { v["entity_revision"] = nil }, "refused-positive": func(v map[string]any) { v["outcome"] = "REFUSED"; v["reason"] = "Reason" }, "refused-empty": func(v map[string]any) { v["outcome"] = "REFUSED"; v["entity_revision"] = nil }, "extra-authority": func(v map[string]any) { v["reviewer_account_id"] = "synthetic" }} {
		t.Run(name, func(t *testing.T) {
			v := clone("EnterpriseReviewResolution")
			edit(v)
			if doc.Components.Schemas["EnterpriseReviewResolution"].Value.VisitJSON(v) == nil {
				t.Fatal("impossible resolution admitted")
			}
		})
	}
	for name, edit := range map[string]func(map[string]any){"open-with-result": func(v map[string]any) { v["resolution"] = clone("EnterpriseReviewResolution") }, "accepted-without-result": func(v map[string]any) { v["state"] = "RESOLVED_ACCEPTED"; v["revision"] = float64(2) }, "open-terminal-revision": func(v map[string]any) { v["revision"] = float64(2) }} {
		t.Run(name, func(t *testing.T) {
			v := clone("EnterpriseMeterReviewView")
			edit(v)
			if doc.Components.Schemas["EnterpriseMeterReviewView"].Value.VisitJSON(v) == nil {
				t.Fatal("held fact became an unproven accepted result")
			}
		})
	}
	t.Run("refusal-reason", func(t *testing.T) {
		v := clone("EnterpriseMeterReviewResolveRequest")
		v["decision"] = "REFUSE"
		if doc.Components.Schemas["EnterpriseMeterReviewResolveRequest"].Value.VisitJSON(v) == nil {
			t.Fatal("reasonless refusal admitted")
		}
	})
	for _, bad := range []any{float64(123), "1000000000001", "-1", "01"} {
		t.Run("meter-bound", func(t *testing.T) {
			v := clone("EnterpriseMeterReviewFact")
			v["value"] = bad
			if doc.Components.Schemas["EnterpriseMeterReviewFact"].Value.VisitJSON(v) == nil {
				t.Fatal("invalid exact meter value admitted")
			}
		})
	}
}
