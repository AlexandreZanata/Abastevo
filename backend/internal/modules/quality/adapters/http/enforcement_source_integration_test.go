//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/getkin/kin-openapi/openapi3"
)

func TestEnforcementNativeActualHTTPTraversalPrivacyHistoryAndOpenAPI(t *testing.T) {
	router, p := freshRouter(t, true)
	ctx := context.Background()
	n := 0
	next := func() (string, error) { n++; return fmt.Sprintf("bbbbbbbb-1111-4111-8111-%012d", n), nil }
	a := application.EnforcementSourceRecord{SourceCNPJ: "11222333000181", SubjectKind: "company-shaped", UF: "DF", Municipality: "SYNTHETIC", Segment: "synthetic", DocumentDate: "2026-01-02", DocumentNumber: "DOC-1", Procedure: "Inspection", Result: strings.Repeat("Long source result ", 8), EvidenceRef: "synthetic.xlsx#row:2"}
	a.Fingerprint = a.ContentHash()
	a.RecordKey = "enforcement-source:" + a.Fingerprint
	b := a
	b.SourceCNPJ = ""
	b.SubjectKind = "personal-shaped"
	b.Result = "Different result on same document"
	b.EvidenceRef = "synthetic.xlsx#row:3"
	b.Fingerprint = b.ContentHash()
	b.RecordKey = "enforcement-source:" + b.Fingerprint
	in := application.EnforcementSourceInput{Context: application.EnforcementSourceContext{Profile: application.EnforcementSourceProfile, SourceFile: "synthetic.xlsx", DateSystem: "EXCEL_1900", FirstDocumentDay: a.DocumentDate, LastDocumentDay: a.DocumentDate}, Resources: []application.Resource{{URL: "https://www.gov.br/synthetic.xlsx", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Counts: domain.Counts{Input: 2, Accepted: 2}, ManifestHash: strings.Repeat("b", 64), Records: []application.EnforcementSourceRecord{a, b}}
	pub := parent.NewPublisher(p)
	rev, e := pub.PublishEnforcementSource(ctx, in, next)
	if e != nil {
		t.Fatal(e)
	}
	_, file, _, _ := runtime.Caller(0)
	spec, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	validate := func(path, schema string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal("native HTTP status", w.Code)
		}
		var body map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
			t.Fatal(e)
		}
		if e := spec.Components.Schemas[schema].Value.VisitJSON(body); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(w.Body.String(), "11222333000181") || strings.Contains(w.Body.String(), "subject_cnpj") {
			t.Fatal("identity leaked")
		}
		return body
	}
	page := validate("/v1/quality/actions?limit=1", "QualityActionPage")
	lease := page["lease"].(map[string]any)
	if lease["stale"] != true || lease["source_clock_precision"] != "UNAVAILABLE" || lease["source_clock_value"] != nil {
		t.Fatal("invented source clock")
	}
	item := page["items"].([]any)[0].(map[string]any)
	if item["occurred_on"] != nil || item["as_of"] != nil || item["scope"] != nil || item["status"] != nil || item["document_day"] != "2026-01-02" {
		t.Fatal("source semantics relabelled")
	}
	detail := validate("/v1/quality/actions/"+item["id"].(string), "QualityAction")
	if detail["result"] != item["result"] || detail["procedure"] != "Inspection" {
		t.Fatal("detail lost fields")
	}
	cursor := page["next_cursor"].(string)
	tail := validate("/v1/quality/actions?limit=1&cursor="+url.QueryEscape(cursor), "QualityActionPage")
	if len(tail["items"].([]any)) != 1 || tail["next_cursor"] != nil {
		t.Fatal("traversal lost distinct result")
	}
	for _, path := range []string{"/v1/quality/actions?scope=municipal", "/v1/quality/actions?status=final", "/v1/quality/actions?station_id=" + httpStation} {
		if code, _, _ := get(t, router, path); code != 400 {
			t.Fatal("invented native scope", code)
		}
	}
	in.Records[0].Result = "Correction"
	in.Records[0].Fingerprint = in.Records[0].ContentHash()
	in.Records[0].RecordKey = "enforcement-source:" + in.Records[0].Fingerprint
	in.ManifestHash = strings.Repeat("c", 64)
	if _, e := pub.PublishEnforcementSource(ctx, in, next); e != nil {
		t.Fatal(e)
	}
	if code, _, _ := get(t, router, "/v1/quality/actions?limit=1&cursor="+url.QueryEscape(cursor)); code != 400 {
		t.Fatal("stale cursor accepted", code)
	}
	historical := validate("/v1/quality/actions?revision_id="+rev, "QualityActionPage")
	if historical["lease"].(map[string]any)["revision_status"] != "superseded" {
		t.Fatal("history lost")
	}
}
