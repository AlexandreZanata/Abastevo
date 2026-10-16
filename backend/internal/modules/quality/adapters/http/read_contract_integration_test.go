//go:build integration

package http

import (
	"context"
	"encoding/json"
	parentadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestQualityActualReadResponsesMatchOpenAPI(t *testing.T) {
	r, p := freshRouter(t, true)
	_, file, _, _ := runtime.Caller(0)
	loader := openapi3.NewLoader()
	spec, e := loader.LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	var revision string
	if e := p.QueryRow(context.Background(), "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&revision); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ path, schema string }{
		{"/v1/quality/revisions?stream=" + assayStream, "QualityRevisionPage"},
		{"/v1/quality/revisions/" + revision + "?stream=" + assayStream, "QualityRevisionDetail"},
		{"/v1/quality/summary?station_id=" + httpStation, "QualitySummary"},
		{"/v1/quality/assays?station_id=" + httpStation, "QualityAssayPage"},
		{"/v1/quality/actions?status=absent", "QualityActionPage"},
		{"/v1/quality/sanctions", "QualitySanctionPage"},
	} {
		t.Run(tc.schema, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var value any
			if e := json.Unmarshal(w.Body.Bytes(), &value); e != nil {
				t.Fatal(e)
			}
			if e := spec.Components.Schemas[tc.schema].Value.VisitJSON(value); e != nil {
				t.Fatal(e)
			}
		})
	}
	if _, e := p.Exec(context.Background(), "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", revision); e != nil {
		t.Fatal(e)
	}
	unavailable := httptest.NewRecorder()
	r.ServeHTTP(unavailable, httptest.NewRequest("GET", "/v1/quality/revisions/"+revision+"?stream="+assayStream, nil))
	var unavailableValue any
	if e := json.Unmarshal(unavailable.Body.Bytes(), &unavailableValue); e != nil {
		t.Fatal(e)
	}
	if e := spec.Components.Schemas["QualityRevisionDetail"].Value.VisitJSON(unavailableValue); e != nil {
		t.Fatal("unavailable provenance contract", e)
	}
	if _, e := p.Exec(context.Background(), "DELETE FROM quality_current_revisions"); e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/quality/summary?station_id="+httpStation, nil))
	var value any
	if e := json.Unmarshal(w.Body.Bytes(), &value); e != nil {
		t.Fatal(e)
	}
	if e := spec.Components.Schemas["QualitySummary"].Value.VisitJSON(value); e != nil {
		t.Fatal("no-source lease", e)
	}
}

func TestQualityHTTPFreezeBeforeConcurrentPublication(t *testing.T) {
	_, p := freshRouter(t, true)
	ctx := context.Background()
	var old string
	if e := p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&old); e != nil {
		t.Fatal(e)
	}
	calls := 0
	h := Handler{Read: qualityread.NewReader(p), Secrets: testSecrets, StationsExist: func(context.Context, string) (bool, error) { return true, nil }, Now: func() time.Time {
		calls++
		if calls == 2 {
			tx, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			for _, q := range []string{
				"UPDATE quality_revisions SET status='superseded' WHERE id='" + old + "'",
				"INSERT INTO quality_revisions(id,edition_id,status,published_at) SELECT '99999999-9999-4999-8999-999999999999',edition_id,'published',now() FROM quality_revisions WHERE id='" + old + "'",
				"INSERT INTO quality_revision_provenance(revision_id,provenance_state,input,accepted,duplicates,quarantined,manifest_hash,publication_key) SELECT '99999999-9999-4999-8999-999999999999','VERIFIED',input,accepted,duplicates,quarantined,repeat('f',64),repeat('e',64) FROM quality_revision_provenance WHERE revision_id='" + old + "'",
				"INSERT INTO quality_assays(id,revision_id,sample_ref,assay_ref,source_cnpj,observed_on,fuel,result_text,unit,conformity,evidence_ref,fingerprint) SELECT gen_random_uuid(),'99999999-9999-4999-8999-999999999999',sample_ref,assay_ref,source_cnpj,observed_on,fuel,'later revision',unit,conformity,evidence_ref,fingerprint FROM quality_assays WHERE revision_id='" + old + "'",
				"INSERT INTO quality_links SELECT gen_random_uuid(),'99999999-9999-4999-8999-999999999999',record_key,station_id,automatic,reason,active FROM quality_links WHERE revision_id='" + old + "'",
				"UPDATE quality_current_revisions SET revision_id='99999999-9999-4999-8999-999999999999' WHERE source='pmqc'",
			} {
				if _, e = tx.Exec(ctx, q); e != nil {
					t.Fatal(e)
				}
			}
			if e = tx.Commit(ctx); e != nil {
				t.Fatal(e)
			}
		}
		return time.Now()
	}}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	code, body, _ := get(t, r, "/v1/quality/assays?station_id="+httpStation+"&limit=1")
	if code != 200 {
		t.Fatal(code, body)
	}
	if body["lease"].(map[string]any)["revision_id"] != old || strings.Contains(recString(body), "later revision") {
		t.Fatal("mixed revision", body)
	}
	cursor := body["next_cursor"].(string)
	code, _, _ = get(t, r, "/v1/quality/assays?station_id="+httpStation+"&limit=1&cursor="+cursor)
	if code != 400 {
		t.Fatal("old cursor admitted", code)
	}
}

func TestQualityHTTPCacheIncludesStaleness(t *testing.T) {
	_, p := freshRouter(t, true)
	at := time.Now()
	h := Handler{Read: qualityread.NewReader(p), Secrets: testSecrets, StationsExist: func(context.Context, string) (bool, error) { return true, nil }, Now: func() time.Time { return at }}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	var revision string
	if e := p.QueryRow(context.Background(), "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&revision); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/v1/quality/revisions/" + revision + "?stream=" + assayStream, "/v1/quality/assays?station_id=" + httpStation, "/v1/quality/actions", "/v1/quality/samples/S-1"} {
		at = time.Now()
		code, _, headers := get(t, r, path)
		if code != 200 {
			t.Fatal(path, code)
		}
		at = at.Add(181 * 24 * time.Hour)
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("If-None-Match", headers.Get("ETag"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"stale":true`) {
			t.Fatal("staleness hidden by cache", path, w.Code, w.Body.String())
		}
	}
}

func TestQualityNativeDelimiterCursor(t *testing.T) {
	native, id := splitCursor("native|record|11111111-1111-4111-8111-111111111111")
	if native != "native|record" || id != "11111111-1111-4111-8111-111111111111" {
		t.Fatal(native, id)
	}
}

func TestQualityMemberProvenanceActualWire(t *testing.T) {
	r, p := freshRouter(t, true)
	ctx := context.Background()
	id, e := parentadapters.NewPublisher(p).Publish(ctx, application.Input{Source: assaySource, Stream: assayStream, Edition: "2026-10", Resources: []application.Resource{{URL: "https://www.gov.br/anp/pmqc_2026.zip", SHA256: strings.Repeat("a", 64), Bytes: 30, Member: "2026/PMQC_2026_10.csv", MemberSHA256: strings.Repeat("b", 64), MemberBytes: 40}}, ManifestHash: strings.Repeat("c", 64)}, jobs.NewUUIDv4)
	if e != nil {
		t.Fatal(e)
	}
	code, body, _ := get(t, r, "/v1/quality/revisions/"+id+"?stream="+assayStream)
	if code != 200 {
		t.Fatal(code, body)
	}
	resources := body["resources"].([]any)
	if len(resources) != 1 {
		t.Fatal(body)
	}
	member := resources[0].(map[string]any)
	if member["member_name"] != "2026/PMQC_2026_10.csv" || member["member_sha256"] != strings.Repeat("b", 64) || member["member_bytes"] != float64(40) {
		t.Fatal("member not served", member)
	}
	_, file, _, _ := runtime.Caller(0)
	spec, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	if e = spec.Components.Schemas["QualityRevisionDetail"].Value.VisitJSON(body); e != nil {
		t.Fatal("member contract", e)
	}
}

func TestQualitySourceClockActualWire(t *testing.T) {
	r, p := freshRouter(t, true)
	clock := "2026-10-02 12:34:56"
	in := application.Input{Source: assaySource, Stream: assayStream, Edition: "2026-10", Resources: []application.Resource{{URL: "https://example.invalid/source.zip", SHA256: strings.Repeat("a", 64), Bytes: 30}}, ManifestHash: strings.Repeat("b", 64), Counts: domain.Counts{Input: 1, Accepted: 1}, Assays: []application.AssayInput{{SampleRef: "CLOCK-S", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-10-02", SourceObservedAt: clock, Fuel: "ETHANOL", ResultText: "LII", Conformity: "CONFORMING", EvidenceRef: "source#clock", Fingerprint: strings.Repeat("c", 64)}}, Links: []application.LinkInput{{RecordKey: assayStream + ":CLOCK-S", StationID: httpStation, Automatic: true, Active: true, Reason: "exact cnpj"}}}
	if _, e := parentadapters.NewPublisher(p).Publish(context.Background(), in, jobs.NewUUIDv4); e != nil {
		t.Fatal(e)
	}
	_, file, _, _ := runtime.Caller(0)
	spec, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ path, schema, field string }{{"/v1/quality/assays?station_id=" + httpStation, "QualityAssayPage", "items"}, {"/v1/quality/samples/CLOCK-S", "QualitySample", "assays"}} {
		code, body, _ := get(t, r, tc.path)
		if code != 200 {
			t.Fatal(code, body)
		}
		rows := body[tc.field].([]any)
		if len(rows) != 1 {
			t.Fatal(body)
		}
		row := rows[0].(map[string]any)
		if row["source_observed_at"] != clock || row["observed_on"] != "2026-10-02" {
			t.Fatal("wire clock truncated or zoned", row)
		}
		if e := spec.Components.Schemas[tc.schema].Value.VisitJSON(body); e != nil {
			t.Fatal(tc.path, e)
		}
	}
}
