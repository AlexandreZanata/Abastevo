package apicontract

import (
	"encoding/json"
	"strings"
	"testing"
)

// EMP-RPT-001: the export contract carries only references, digests, closed
// states and codes; the capability travels in the body; no storage address,
// requester identity or lease is part of any response.
func TestReportExportContractExamplesAndExactShapes(t *testing.T) {
	doc := loadSpec(t)
	for path, rule := range map[string]string{
		"/v1/orgs/{org_id}/reports/exports/request":             `{"permission":"rpt.export.run","write":true,"fresh":true,"require_revision":false,"company_only":true,"success":202}`,
		"/v1/orgs/{org_id}/reports/exports/list":                `{"permission":"rpt.export.run","write":false,"fresh":true,"require_revision":false,"company_only":true,"success":200}`,
		"/v1/orgs/{org_id}/reports/exports/{export_id}/read":    `{"permission":"rpt.export.run","write":false,"fresh":true,"require_revision":false,"company_only":true,"success":200}`,
		"/v1/orgs/{org_id}/reports/exports/{export_id}/access":  `{"permission":"rpt.export.run","write":false,"fresh":true,"require_revision":false,"company_only":true,"success":200}`,
		"/v1/orgs/{org_id}/reports/exports/{export_id}/content": `{"permission":"rpt.export.run","write":false,"fresh":true,"require_revision":false,"company_only":true,"success":200}`,
	} {
		item := doc.Paths.Value(path)
		if item == nil || item.Post == nil {
			t.Fatal("export operation missing", path)
		}
		raw, err := json.Marshal(item.Post.Extensions["x-abastevo-organization"])
		var got, want map[string]any
		if err != nil || json.Unmarshal(raw, &got) != nil || json.Unmarshal([]byte(rule), &want) != nil || len(got) != len(want) {
			t.Fatal("export operation rule drift", path, string(raw))
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatal("export operation rule drift", path, k)
			}
		}
		if strings.Contains(path, "{export_id}") && item.Post.Responses.Value("410") == nil {
			t.Fatal("expired export response missing", path)
		}
		// EMP-AUTH-011: request and delivery also need the kind's data
		// permission, refused as FORBIDDEN with that key.
		if strings.HasSuffix(path, "/request") || strings.HasSuffix(path, "/access") || strings.HasSuffix(path, "/content") {
			if !strings.Contains(item.Post.Description, "read permission declared by the kind") || item.Post.Responses.Value("403") == nil || item.Post.Responses.Value("422") == nil {
				t.Fatal("export data permission gate undocumented", path)
			}
		}
	}
	names := []string{"EnterpriseExportRequest", "EnterpriseExportListRequest", "EnterpriseExportReadRequest", "EnterpriseExportContentRequest", "EnterpriseExportView", "EnterpriseExportAcknowledgement", "EnterpriseExportPage", "EnterpriseExportCapability", "EnterpriseExportContent"}
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
				t.Fatal("missing export schema or example")
			}
			v := clone(t, name)
			if err := s.Value.VisitJSON(v); err != nil {
				t.Fatal("invalid export example", err)
			}
			for _, field := range []string{"requester_account_id", "storage_key", "url", "lease_token"} {
				v := clone(t, name)
				v[field] = "synthetic"
				if s.Value.VisitJSON(v) == nil {
					t.Fatal("private or caller-selected field accepted", field)
				}
			}
		})
	}
	reject := func(name string, edit func(map[string]any)) {
		t.Helper()
		v := clone(t, name)
		edit(v)
		if doc.Components.Schemas[name].Value.VisitJSON(v) == nil {
			t.Fatal("impossible export document accepted", name)
		}
	}
	reject("EnterpriseExportRequest", func(v map[string]any) { delete(v, "parameters") })
	reject("EnterpriseExportRequest", func(v map[string]any) { v["parameters"] = []any{} })
	reject("EnterpriseExportRequest", func(v map[string]any) { v["kind"] = "Invalid" })
	reject("EnterpriseExportRequest", func(v map[string]any) { v["revision"] = float64(1) })
	reject("EnterpriseExportListRequest", func(v map[string]any) { v["state"] = "DONE" })
	reject("EnterpriseExportListRequest", func(v map[string]any) { v["limit"] = float64(101) })
	reject("EnterpriseExportContentRequest", func(v map[string]any) { delete(v, "capability") })
	reject("EnterpriseExportContentRequest", func(v map[string]any) { v["capability"] = strings.Repeat("a", 2049) })
	reject("EnterpriseExportView", func(v map[string]any) { v["state"] = "CANCELLED" })
	reject("EnterpriseExportView", func(v map[string]any) { v["failure_code"] = "EXPORT_CANCELLED" })
	reject("EnterpriseExportView", func(v map[string]any) { v["sha256"] = "XYZ" })
	reject("EnterpriseExportView", func(v map[string]any) { v["row_count"] = float64(500001) })
	reject("EnterpriseExportView", func(v map[string]any) { v["byte_count"] = float64(16777217) })
	reject("EnterpriseExportView", func(v map[string]any) { v["attempts"] = float64(4) })
	reject("EnterpriseExportContent", func(v map[string]any) { v["content_base64"] = "not base64!" })
	reject("EnterpriseExportCapability", func(v map[string]any) { delete(v, "expires_at") })
	queued := clone(t, "EnterpriseExportView")
	for _, field := range []string{"row_count", "byte_count", "sha256", "media_type", "ready_at", "expires_at", "failure_code", "failed_at", "expired_at"} {
		queued[field] = nil
	}
	queued["state"], queued["attempts"] = "QUEUED", float64(0)
	if doc.Components.Schemas["EnterpriseExportView"].Value.VisitJSON(queued) != nil {
		t.Fatal("queued export without file facts refused")
	}
	errorCodes, err := json.Marshal(doc.Components.Schemas["EnterpriseError"].Value)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"EXPORT_QUEUE_FULL", "EXPORT_NOT_READY", "EXPORT_EXPIRED", "EXPORT_FORMAT_UNAVAILABLE"} {
		if !strings.Contains(string(errorCodes), code) {
			t.Fatal("export error code not registered in the contract", code)
		}
	}
}
