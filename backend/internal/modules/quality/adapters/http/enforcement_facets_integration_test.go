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

func facetRecord(cnpj, kind, uf, city, segment, day, doc, result string, row int) application.EnforcementSourceRecord {
	r := application.EnforcementSourceRecord{SourceCNPJ: cnpj, SubjectKind: kind, UF: uf, Municipality: city, Segment: segment, DocumentDate: day, DocumentNumber: doc, Procedure: "Procedimento sintético", Result: result, EvidenceRef: fmt.Sprintf("synthetic.xlsx#row:%d", row)}
	r.Fingerprint = r.ContentHash()
	r.RecordKey = "enforcement-source:" + r.Fingerprint
	return r
}

func TestEnforcementFacetsCoverageAndOpenAPI(t *testing.T) {
	router, p := freshRouter(t, true)
	ctx := context.Background()
	n := 0
	next := func() (string, error) { n++; return fmt.Sprintf("cccccccc-2222-4222-8222-%012d", n), nil }
	records := []application.EnforcementSourceRecord{
		facetRecord("11222333000181", "company-shaped", "DF", "BRASILIA", "Revenda de Combustíveis", "2026-10-04", "DOC-1", "Resultado um", 2),
		facetRecord("11222333000181", "company-shaped", "DF", "BRASILIA", "Revenda de GLP", "2026-10-03", "DOC-2", "Resultado dois", 3),
		facetRecord("", "personal-shaped", "SP", "CAMPINAS", "Revenda de Combustíveis", "2019-01-02", "DOC-3", "Resultado três", 4),
		facetRecord("", "other", "SP", "CAMPINAS", "", "2019-01-02", "DOC-4", "Resultado quatro", 5),
	}
	in := application.EnforcementSourceInput{Context: application.EnforcementSourceContext{Profile: application.EnforcementSourceProfile, SourceFile: "synthetic.xlsx", DateSystem: "EXCEL_1900", FirstDocumentDay: "2019-01-02", LastDocumentDay: "2026-10-04"}, Resources: []application.Resource{{URL: "https://www.gov.br/synthetic.xlsx", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Counts: domain.Counts{Input: 4, Accepted: 4}, ManifestHash: strings.Repeat("b", 64), Records: records}
	if _, e := parent.NewPublisher(p).PublishEnforcementSource(ctx, in, next); e != nil {
		t.Fatal(e)
	}
	_, file, _, _ := runtime.Caller(0)
	spec, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	get := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	validatePage := func(path string) map[string]any {
		t.Helper()
		code, body := get(path)
		if code != 200 {
			t.Fatal("facet page status", code, path)
		}
		if e := spec.Components.Schemas["QualityActionPage"].Value.VisitJSON(body); e != nil {
			t.Fatal("page outside contract", path, e)
		}
		if strings.Contains(fmt.Sprint(body), "11222333000181") {
			t.Fatal("identity leaked", path)
		}
		return body
	}
	all := validatePage("/v1/quality/actions?limit=100")
	if len(all["items"].([]any)) != 4 {
		t.Fatal("unfiltered page lost records")
	}
	for _, item := range all["items"].([]any) {
		if item.(map[string]any)["date_basis"] != "document_day" {
			t.Fatal("date basis undeclared", item)
		}
	}
	df := validatePage("/v1/quality/actions?limit=100&uf=DF")
	if len(df["items"].([]any)) != 2 {
		t.Fatal("uf facet", len(df["items"].([]any)))
	}
	for _, item := range df["items"].([]any) {
		if item.(map[string]any)["uf"] != "DF" {
			t.Fatal("uf facet leaked", item)
		}
	}
	glp := validatePage("/v1/quality/actions?limit=100&uf=DF&segment=" + url.QueryEscape("Revenda de GLP"))
	if len(glp["items"].([]any)) != 1 {
		t.Fatal("combined facets", len(glp["items"].([]any)))
	}
	personal := validatePage("/v1/quality/actions?limit=100&subject_kind=personal-shaped")
	if len(personal["items"].([]any)) != 1 {
		t.Fatal("kind facet lost unmatched records")
	}
	period := validatePage("/v1/quality/actions?limit=100&from_day=2026-10-03&to_day=2026-10-04")
	if len(period["items"].([]any)) != 2 {
		t.Fatal("period facet", len(period["items"].([]any)))
	}
	unknown := validatePage("/v1/quality/actions?limit=100&uf=XX")
	if len(unknown["items"].([]any)) != 0 || unknown["next_cursor"] != nil {
		t.Fatal("unknown facet must page empty")
	}
	for _, path := range []string{
		"/v1/quality/actions?uf=S",
		"/v1/quality/actions?uf=SPO",
		"/v1/quality/actions?subject_kind=bogus",
		"/v1/quality/actions?from_day=not-a-day",
		"/v1/quality/actions?from_day=2026-10-04&to_day=2026-10-03",
	} {
		if code, _ := get(path); code != 400 {
			t.Fatal("malformed facet accepted", path, code)
		}
	}
	code, coverage := get("/v1/quality/actions/coverage")
	if code != 200 {
		t.Fatal("coverage status", code)
	}
	if e := spec.Components.Schemas["QualityActionCoverage"].Value.VisitJSON(coverage); e != nil {
		t.Fatal("coverage outside contract", e)
	}
	if coverage["observations"] != float64(4) || coverage["cases"] != float64(4) ||
		coverage["first_day"] != "2019-01-02" || coverage["last_day"] != "2026-10-04" ||
		coverage["date_basis"] != "document_day" {
		t.Fatal("coverage miscounted", coverage)
	}
	cursor := df["next_cursor"]
	if cursor == nil {
		// Two DF rows with limit 100 fit one page; force keyset traversal.
		df = validatePage("/v1/quality/actions?limit=1&uf=DF")
		cursor = df["next_cursor"]
		if cursor == nil {
			t.Fatal("facet traversal missing cursor")
		}
		tail := validatePage("/v1/quality/actions?limit=1&uf=DF&cursor=" + url.QueryEscape(cursor.(string)))
		if len(tail["items"].([]any)) != 1 || tail["next_cursor"] != nil {
			t.Fatal("facet traversal omitted or duplicated")
		}
	}
}
