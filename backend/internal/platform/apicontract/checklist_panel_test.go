package apicontract

import (
	"encoding/json"
	"slices"
	"testing"
)

// EMP-CHECK-004 (B-BR-EMP-506/507, EDR-328/329): the run queue page carries
// exactly the run read's item and a closed request; the context hints admit
// the document-content read hint and nothing broader.
func TestChecklistRunQueueAndContentHintContractsAreExact(t *testing.T) {
	doc := loadSpec(t)
	path := doc.Paths.Value("/v1/orgs/{org_id}/checklists/runs/list")
	if path == nil || path.Post == nil || path.Post.RequestBody == nil {
		t.Fatal("missing run queue route")
	}
	if ref := path.Post.RequestBody.Value.Content.Get("application/json").Schema.Ref; ref != "#/components/schemas/EnterpriseChecklistRunListRequest" {
		t.Fatal("run queue request schema", ref)
	}
	if ref := path.Post.Responses.Value("200").Value.Content.Get("application/json").Schema.Ref; ref != "#/components/schemas/EnterpriseChecklistRunPage" {
		t.Fatal("run queue response schema", ref)
	}
	request := doc.Components.Schemas["EnterpriseChecklistRunListRequest"].Value
	for name, v := range map[string]map[string]any{"empty": {}, "limit": {"limit": float64(100)}, "cursor": {"limit": float64(1), "cursor": "sealed"}} {
		if err := request.VisitJSON(v); err != nil {
			t.Fatal("valid queue request refused", name, err)
		}
	}
	for name, v := range map[string]map[string]any{"zero": {"limit": float64(0)}, "large": {"limit": float64(101)}, "empty cursor": {"cursor": ""},
		"outcome filter": {"outcome": "BLOCKING"}, "asset filter": {"asset_id": "7b0e2a54-0000-4000-8000-000000009721"}, "org": {"org_id": "x"}} {
		if request.VisitJSON(v) == nil {
			t.Fatal("invalid queue request admitted", name)
		}
	}
	page := doc.Components.Schemas["EnterpriseChecklistRunPage"]
	if page == nil || page.Value == nil || page.Value.Example == nil {
		t.Fatal("missing run queue page or example")
	}
	if ref := page.Value.Properties["items"].Value.Items.Ref; ref != "#/components/schemas/EnterpriseChecklistRun" || page.Value.Properties["items"].Value.MaxItems == nil || *page.Value.Properties["items"].Value.MaxItems != 100 {
		t.Fatal("run queue items are not the run read's item", ref)
	}
	raw, _ := json.Marshal(page.Value.Example)
	var example map[string]any
	if json.Unmarshal(raw, &example) != nil || page.Value.VisitJSON(example) != nil {
		t.Fatal("invalid run queue example")
	}
	for _, field := range []string{"note", "reason", "author_membership_id", "photo", "unit_id"} {
		v := map[string]any{}
		_ = json.Unmarshal(raw, &v)
		v["items"].([]any)[0].(map[string]any)[field] = "synthetic"
		if page.Value.VisitJSON(v) == nil {
			t.Fatal("run queue item admitted", field)
		}
		v = map[string]any{}
		_ = json.Unmarshal(raw, &v)
		v[field] = "synthetic"
		if page.Value.VisitJSON(v) == nil {
			t.Fatal("run queue page admitted", field)
		}
	}
	hints := doc.Components.Schemas["EnterpriseAccountContextView"].Value.Properties["permissions"].Value
	var enum []string
	for _, v := range hints.Items.Value.Enum {
		enum = append(enum, v.(string))
	}
	slices.Sort(enum)
	if !slices.Equal(enum, []string{"doc.document.view_content", "idn.employee.manage", "idn.employee.read", "idn.invitation.manage"}) || hints.MaxItems == nil || *hints.MaxItems != 4 {
		t.Fatal("context hint vocabulary", enum)
	}
}
