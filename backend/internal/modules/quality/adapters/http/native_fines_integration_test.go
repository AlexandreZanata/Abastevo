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
	"time"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func TestNativeFineActualWirePrecisionHistoryAndOpenAPI(t *testing.T) {
	_, p := freshRouter(t, true)
	ctx := context.Background()
	n := 0
	id := func() (string, error) { n++; return fmt.Sprintf("aaaaaaaa-9999-4999-8999-%012d", n), nil }
	a := application.FineObservation{SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", DUFRef: "SYNTHETIC/1", Department: "SYNTHETIC", Status: "Source only", DueOn: "2025-02-28", AmountText: "150000,00", AmountMinor: 15000000, PaidText: "180000,01", PaidMinor: 18000001, ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: "synthetic.csv:8"}
	a.ObservationKey = a.ObservationIdentity()
	b := a
	b.DUFRef = "SYNTHETIC/2"
	b.EvidenceRef = "synthetic.csv:9"
	b.ObservationKey = b.ObservationIdentity()
	in := application.NativeFineInput{Edition: "2025", Clock: application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}, Resources: []application.Resource{{URL: "https://www.gov.br/anp/fines-2025.csv", SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: domain.Counts{Input: 2, Accepted: 2}, ManifestHash: strings.Repeat("b", 64), Observations: []application.FineObservation{a, b}}
	pub := parent.NewPublisher(p)
	revision, e := pub.PublishNativeFines(ctx, in, id)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	h := Handler{Read: qualityread.NewReader(p), Secrets: testSecrets, Now: func() time.Time { return at }}
	router := chi.NewRouter()
	h.RegisterRoutes(router)
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
			t.Fatal("native public read", w.Code)
		}
		var value map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &value); e != nil {
			t.Fatal(e)
		}
		if e := spec.Components.Schemas[schema].Value.VisitJSON(value); e != nil {
			t.Fatal("native actual OpenAPI response", e)
		}
		if strings.Contains(w.Body.String(), "11222333000181") || strings.Contains(w.Body.String(), "party_cnpj") || strings.Contains(w.Body.String(), "source_key") {
			t.Fatal("public native response leaked party identity")
		}
		return value
	}
	page := validate("/v1/quality/sanctions?limit=1", "QualitySanctionPage")
	lease := page["lease"].(map[string]any)
	if lease["stale"] != true || lease["source_extraction_month"] != "2026-01" || lease["source_clock_precision"] != "MONTH" || !strings.Contains(lease["stale_reason"].(string), "source") {
		t.Fatal("local publication made old source fresh")
	}
	item := page["items"].([]any)[0].(map[string]any)
	rowID := item["id"].(string)
	if item["as_of"] != nil || item["decided_on"] != nil || item["as_of_precision"] != "MONTH" || item["identity_kind"] != "CONTENT_OBSERVATION" || item["extraction_month"] != "2026-01" || item["due_on"] != "2025-02-28" || item["paid_minor"] != float64(18000001) {
		t.Fatal("native wire clock/money relabelled")
	}
	detail := validate("/v1/quality/sanctions/"+rowID, "QualitySanction")
	if detail["due_on"] != item["due_on"] {
		t.Fatal("native page/detail disagree")
	}
	cursor := page["next_cursor"].(string)
	last := validate("/v1/quality/sanctions?limit=1&cursor="+cursor, "QualitySanctionPage")
	if last["next_cursor"] != nil || last["items"].([]any)[0].(map[string]any)["id"] == rowID {
		t.Fatal("native keyset traversal duplicated/lost data")
	}
	// Source-age boundary remains independent of local publication age.
	at = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(180 * 24 * time.Hour)
	before := httptest.NewRecorder()
	router.ServeHTTP(before, httptest.NewRequest("GET", "/v1/quality/sanctions", nil))
	var fresh map[string]any
	_ = json.Unmarshal(before.Body.Bytes(), &fresh)
	if fresh["lease"].(map[string]any)["stale"] != false {
		t.Fatal("month age boundary changed")
	}
	at = at.Add(time.Second)
	request := httptest.NewRequest("GET", "/v1/quality/sanctions", nil)
	request.Header.Set("If-None-Match", before.Header().Get("ETag"))
	after := httptest.NewRecorder()
	router.ServeHTTP(after, request)
	if after.Code != 200 || after.Header().Get("ETag") == before.Header().Get("ETag") {
		t.Fatal("native source staleness hidden by 304")
	}
	in.ManifestHash = strings.Repeat("e", 64)
	in.Clock.ExtractionMonth = "2026-02"
	for i := range in.Observations {
		in.Observations[i].ExtractionMonth = "2026-02"
		in.Observations[i].ObservationKey = in.Observations[i].ObservationIdentity()
	}
	if _, e = pub.PublishNativeFines(ctx, in, id); e != nil {
		t.Fatal(e)
	}
	if code, _, _ := get(t, router, "/v1/quality/sanctions?limit=1&cursor="+cursor); code != 400 {
		t.Fatal("stale native cursor admitted", code)
	}
	if code, _, _ := get(t, router, "/v1/quality/sanctions/"+rowID); code != 404 {
		t.Fatal("old native row crossed current lease", code)
	}
	old := validate("/v1/quality/sanctions/"+rowID+"?revision_id="+revision, "QualitySanction")
	if old["extraction_month"] != "2026-01" || old["lease"].(map[string]any)["revision_status"] != "superseded" {
		t.Fatal("native historical clock mutated")
	}
}
