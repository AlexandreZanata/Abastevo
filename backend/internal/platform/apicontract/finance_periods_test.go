package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
)

// EMP-APPR-006: the period contract admits only canonical months, exact
// revisions and the B-BR-EMP-212 reason profile, and never a caller-selected
// state, authority or reporting day.
func TestFinancePeriodContractExamplesAndExactShapes(t *testing.T) {
	doc := loadSpec(t)
	for path, permission := range map[string]string{
		"/v1/orgs/{org_id}/finance/periods/list":            "fin.period.close",
		"/v1/orgs/{org_id}/finance/periods/{period}/read":   "fin.period.close",
		"/v1/orgs/{org_id}/finance/periods/{period}/close":  "fin.period.close",
		"/v1/orgs/{org_id}/finance/periods/{period}/reopen": "fin.period.reopen",
	} {
		item := doc.Paths.Value(path)
		if item == nil || item.Post == nil {
			t.Fatal("period operation missing", path)
		}
		raw, err := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
		if err != nil || !strings.Contains(string(raw), `"permission":"`+permission+`"`) {
			t.Fatal("period operation permission drift", path)
		}
	}
	names := []string{"EnterpriseFinancePeriodListRequest", "EnterpriseFinancePeriodReadRequest", "EnterpriseFinancePeriodCloseRequest", "EnterpriseFinancePeriodReopenRequest", "EnterpriseFinancePeriodAcknowledgement", "EnterpriseFinancePeriodTransition", "EnterpriseFinancePeriodView", "EnterpriseFinancePeriodPage"}
	clone := func(t *testing.T, name string) map[string]any {
		t.Helper()
		raw, err := json.Marshal(doc.Components.Schemas[name].Value.Example)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			s := doc.Components.Schemas[name]
			if s == nil || s.Value == nil || s.Value.Example == nil {
				t.Fatal("missing period schema or example")
			}
			v := clone(t, name)
			if err := s.Value.VisitJSON(v); err != nil {
				t.Fatal("invalid period example", err)
			}
			v["account_id"] = "7b0e2a54-0000-4000-8000-000000009001"
			if s.Value.VisitJSON(v) == nil {
				t.Fatal("caller-selected authority field accepted")
			}
		})
	}
	reject := func(name string, edit func(map[string]any)) {
		t.Helper()
		v := clone(t, name)
		edit(v)
		if doc.Components.Schemas[name].Value.VisitJSON(v) == nil {
			t.Fatal("impossible period document accepted", name)
		}
	}
	for _, bad := range []any{float64(0), float64(-1), "2", 1.5} {
		reject("EnterpriseFinancePeriodCloseRequest", func(v map[string]any) { v["revision"] = bad })
	}
	reject("EnterpriseFinancePeriodCloseRequest", func(v map[string]any) { delete(v, "revision") })
	reject("EnterpriseFinancePeriodCloseRequest", func(v map[string]any) { v["reason"] = "Synthetic" })
	reject("EnterpriseFinancePeriodCloseRequest", func(v map[string]any) { v["state"] = "LOCKED" })
	reject("EnterpriseFinancePeriodReopenRequest", func(v map[string]any) { delete(v, "reason") })
	reject("EnterpriseFinancePeriodReopenRequest", func(v map[string]any) { v["reason"] = "" })
	reject("EnterpriseFinancePeriodReopenRequest", func(v map[string]any) { v["reason"] = strings.Repeat("a", 281) })
	reject("EnterpriseFinancePeriodListRequest", func(v map[string]any) { v["state"] = "OPEN" })
	reject("EnterpriseFinancePeriodListRequest", func(v map[string]any) { v["limit"] = float64(101) })
	for _, bad := range []string{"2026-13", "2026-1", "0000-01", "202610"} {
		reject("EnterpriseFinancePeriodView", func(v map[string]any) { v["period"] = bad })
		reject("EnterpriseFinancePeriodAcknowledgement", func(v map[string]any) { v["period"] = bad })
	}
	reject("EnterpriseFinancePeriodView", func(v map[string]any) { v["state"] = "VOIDED" })
	reject("EnterpriseFinancePeriodView", func(v map[string]any) { v["revision"] = float64(0) })
	reject("EnterpriseFinancePeriodAcknowledgement", func(v map[string]any) { v["state"] = "OPEN" })
	reject("EnterpriseFinancePeriodAcknowledgement", func(v map[string]any) { v["revision"] = float64(1) })
	reject("EnterpriseFinancePeriodAcknowledgement", func(v map[string]any) { v["kind"] = "UNLOCK" })
	reject("EnterpriseFinancePeriodTransition", func(v map[string]any) { v["authority"] = "fin.period.bypass" })
	reject("EnterpriseFinancePeriodTransition", func(v map[string]any) { delete(v, "reason") })
	reject("EnterpriseFinancePeriodTransition", func(v map[string]any) { v["to_state"] = "OPEN" })
	view := clone(t, "EnterpriseFinancePeriodView")
	view["period_id"] = nil
	view["state"], view["revision"], view["history"] = "OPEN", float64(1), []any{}
	if doc.Components.Schemas["EnterpriseFinancePeriodView"].Value.VisitJSON(view) != nil {
		t.Fatal("implicit OPEN period at revision 1 refused")
	}
}
