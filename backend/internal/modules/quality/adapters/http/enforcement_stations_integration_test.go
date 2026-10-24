//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestEnforcementStationsFacetsCoverageAndOpenAPI(t *testing.T) {
	f := newCompanyHTTPFixture(t)
	router, p := f.r, f.p
	ctx := context.Background()
	n := 0
	next := func() (string, error) { n++; return fmt.Sprintf("dddddddd-3333-4333-8333-%012d", n), nil }
	records := []application.EnforcementSourceRecord{
		facetRecord("11222333000181", "company-shaped", "DF", "BRASILIA", "Revenda de Combustíveis", "2026-10-04", "DOC-1", "Resultado um", 2),
		facetRecord("36754752000195", "company-shaped", "DF", "BRASILIA", "Revenda de GLP", "2026-10-03", "DOC-2", "Resultado dois", 3),
		facetRecord("", "personal-shaped", "SP", "CAMPINAS", "Revenda de Combustíveis", "2019-01-02", "DOC-3", "Resultado três", 4),
	}
	stations := map[string]string{"DF-1": "aaaaaaaa-1111-4111-8111-000000000001", "DF-2": "aaaaaaaa-1111-4111-8111-000000000002", "SP-9": "aaaaaaaa-1111-4111-8111-000000000009"}
	for name, id := range stations {
		var sid pgtype.UUID
		if e := sid.Scan(id); e != nil {
			t.Fatal(e)
		}
		state := "DF"
		if strings.HasPrefix(name, "SP") {
			state = "SP"
		}
		if _, e := p.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,status) VALUES($1,$2,$3,'active')`, sid, "Posto "+name, state); e != nil {
			t.Fatal(e)
		}
	}
	lookup := func(ctx context.Context, cnpjs []string) (application.CompanyIdentitySnapshot, error) {
		return application.CompanyIdentitySnapshot{Evidence: []domain.IdentityEvidence{
			{CNPJ: "11222333000181", StationID: stations["DF-1"], Current: true, StationActive: true},
			{CNPJ: "36754752000195", StationID: stations["DF-1"], Current: true, StationActive: true},
			{CNPJ: "36754752000195", StationID: stations["DF-2"], Current: true, StationActive: true},
		}}, nil
	}
	decisions, e := parent.PlanEnforcementCompanyAssociations(ctx, records[:2], "2026-10-22", lookup)
	if e != nil || len(decisions) != 2 {
		t.Fatal("linkage not planned", e)
	}
	if decisions[0].Kind != domain.ResolutionBound || decisions[0].StationID != stations["DF-1"] {
		t.Fatal("exact binding lost", decisions[0])
	}
	if decisions[1].Kind != domain.ResolutionAmbiguous || decisions[1].StationID != "" || decisions[1].Reason != domain.ReasonAmbiguous {
		t.Fatal("multibranch ambiguity not contained", decisions[1])
	}
	in := application.EnforcementSourceInput{Context: application.EnforcementSourceContext{Profile: application.EnforcementSourceProfile, SourceFile: "synthetic.xlsx", DateSystem: "EXCEL_1900", FirstDocumentDay: "2019-01-02", LastDocumentDay: "2026-10-04"}, Resources: []application.Resource{{URL: "https://www.gov.br/synthetic.xlsx", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Counts: domain.Counts{Input: 3, Accepted: 3}, ManifestHash: strings.Repeat("b", 64), Records: records, CompanyAssociations: &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-22", Decisions: decisions}}
	if _, e := p.Exec(ctx, `DELETE FROM directory_identifiers WHERE normalized_value='11222333000181'; INSERT INTO directory_identifiers(id,station_id,kind,normalized_value) VALUES('aaaaaaaa-1111-4111-8111-000000000099','aaaaaaaa-1111-4111-8111-000000000001','CNPJ','11222333000181')`); e != nil {
		t.Fatal(e)
	}
	pub := parent.NewPublisher(p).WithCompanyGuard(f.guard)
	if _, e := pub.PublishEnforcementSource(ctx, in, next); e != nil {
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
	validateStations := func(path string) map[string]any {
		t.Helper()
		code, body := get(path)
		if code != 200 {
			t.Fatal("station page status", code, path)
		}
		if e := spec.Components.Schemas["QualityEnforcementStationPage"].Value.VisitJSON(body); e != nil {
			t.Fatal("station page outside contract", path, e)
		}
		if strings.Contains(fmt.Sprint(body), "11222333000181") {
			t.Fatal("identity leaked", path)
		}
		return body
	}
	page := validateStations("/v1/quality/enforcement/stations?limit=100")
	items := page["items"].([]any)
	if len(items) != 1 {
		t.Fatal("ambiguous link bound or bound link lost", len(items))
	}
	item := items[0].(map[string]any)
	if item["display_name"] != "Posto DF-1" || item["state"] != "DF" || item["status"] != "active" || item["record_count"] != float64(1) {
		t.Fatal("station mistyped", item)
	}
	// No-price station included (no price rows exist anywhere here).
	df := validateStations("/v1/quality/enforcement/stations?limit=100&state=DF")
	if len(df["items"].([]any)) != 1 {
		t.Fatal("state facet", len(df["items"].([]any)))
	}
	empty := validateStations("/v1/quality/enforcement/stations?limit=100&state=XX")
	if len(empty["items"].([]any)) != 0 || empty["next_cursor"] != nil {
		t.Fatal("unknown state must page empty")
	}
	for _, path := range []string{
		"/v1/quality/enforcement/stations?state=S",
		"/v1/quality/enforcement/stations?from_day=not-a-day",
		"/v1/quality/enforcement/stations?from_day=2026-10-04&to_day=2026-10-03",
		"/v1/quality/enforcement/stations?bogus=1",
		"/v1/quality/enforcement/stations?state=DF&state=SP",
		"/v1/quality/enforcement/stations?state=%GG",
		"/v1/quality/enforcement/stations/coverage?state=DF",
		"/v1/quality/enforcement/stations/coverage?revision_id=x&revision_id=y",
	} {
		if code, _ := get(path); code != 400 {
			t.Fatal("malformed station filter accepted", path, code)
		}
	}
	code, coverage := get("/v1/quality/enforcement/stations/coverage")
	if code != 200 {
		t.Fatal("station coverage status", code)
	}
	if e := spec.Components.Schemas["QualityEnforcementStationCoverage"].Value.VisitJSON(coverage); e != nil {
		t.Fatal("station coverage outside contract", e)
	}
	if coverage["stations"] != float64(1) || coverage["records"] != float64(1) {
		t.Fatal("station/record separation lost", coverage)
	}
	// Live retirement at read: retire the bound station, it must vanish.
	var sid pgtype.UUID
	if e := sid.Scan(stations["DF-1"]); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `UPDATE directory_stations SET status='retired' WHERE id=$1`, sid); e != nil {
		t.Fatal(e)
	}
	retired := validateStations("/v1/quality/enforcement/stations?limit=100")
	if len(retired["items"].([]any)) != 0 {
		t.Fatal("retired station served stale")
	}
}
