//go:build integration

package http

import (
	"context"
	"encoding/json"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	jobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func historyCorrection(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	id, e := parent.NewPublisher(p).Publish(context.Background(), application.Input{Source: assaySource, Stream: assayStream, Edition: "2026-09", Resources: []application.Resource{{URL: "https://www.gov.br/anp/pmqc_2026_09.zip", SHA256: strings.Repeat("c", 64), Bytes: 20}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("d", 64), Assays: []application.AssayInput{{SampleRef: "S-1", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "Corrected synthetic result", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "corrected#S-1", Fingerprint: strings.Repeat("e", 64)}}, Links: []application.LinkInput{{RecordKey: assayStream + ":S-1", StationID: httpStation, Automatic: true, Reason: "exact cnpj", Active: true}}}, jobs.NewUUIDv4)
	if e != nil {
		t.Fatal(e)
	}
	return id
}

func TestHistoricalHTTPPagesDetailsCatalogAndIsolation(t *testing.T) {
	router, p := freshRouter(t, true)
	ctx := context.Background()
	var old string
	if e := p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&old); e != nil {
		t.Fatal(e)
	}
	path := "/v1/quality/assays?station_id=" + httpStation + "&limit=1&revision_id=" + old
	code, first, _ := get(t, router, path)
	if code != 200 {
		t.Fatal(first)
	}
	cursor := first["next_cursor"].(string)
	current := historyCorrection(t, p)
	code, second, _ := get(t, router, path+"&cursor="+url.QueryEscape(cursor))
	if code != 200 || len(second["items"].([]any)) != 1 || second["lease"].(map[string]any)["revision_status"] != "superseded" {
		t.Fatal("old traversal lost", code, second)
	}
	for _, bad := range []string{strings.Replace(path, old, current, 1) + "&cursor=" + url.QueryEscape(cursor), "/v1/quality/assays?station_id=" + httpStation + "&limit=1&cursor=" + url.QueryEscape(cursor)} {
		if code, _, _ := get(t, router, bad); code != 400 {
			t.Fatal("revision cursor reused", code)
		}
	}
	for _, endpoint := range []string{"/v1/quality/summary?station_id=" + httpStation + "&revision_id=" + old, "/v1/quality/samples/S-1?revision_id=" + old, "/v1/quality/revisions/" + old + "?stream=" + assayStream} {
		code, body, _ := get(t, router, endpoint)
		if code != 200 {
			t.Fatal(endpoint, code, body)
		}
		lease := body["lease"].(map[string]any)
		if lease["revision_id"] != old || lease["revision_status"] != "superseded" || lease["is_current_stream"] != false {
			t.Fatal("history status missing", body)
		}
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), "11222333000181") {
			t.Fatal("source identifier disclosed")
		}
	}
	code, detail, _ := get(t, router, "/v1/quality/revisions/"+old+"?stream="+assayStream)
	if code != 200 || detail["counts"].(map[string]any)["accepted"] != float64(2) || detail["manifest_hash"] != strings.Repeat("b", 64) || detail["resources"].([]any)[0].(map[string]any)["sha256"] != strings.Repeat("a", 64) {
		t.Fatal("provenance drift", detail)
	}
	seen := map[string]bool{}
	next := ""
	for pages := 0; pages < 3; pages++ {
		code, body, _ := get(t, router, "/v1/quality/revisions?stream="+assayStream+"&limit=1&cursor="+url.QueryEscape(next))
		if code != 200 || body["catalog_live"] != true {
			t.Fatal(body)
		}
		for _, item := range body["items"].([]any) {
			id := item.(map[string]any)["revision_id"].(string)
			if seen[id] {
				t.Fatal("catalog repeat")
			}
			seen[id] = true
		}
		if body["next_cursor"] == nil {
			break
		}
		next = body["next_cursor"].(string)
	}
	for _, endpoint := range []string{
		"/v1/quality/revisions?stream=" + actionStream + "&limit=1&cursor=" + url.QueryEscape(next),
		"/v1/quality/revisions?stream=" + assayStream + "&limit=2&cursor=" + url.QueryEscape(next),
		"/v1/quality/revisions?stream=" + assayStream + "&limit=1&cursor=" + url.QueryEscape(next+"tampered"),
	} {
		if code, _, _ := get(t, router, endpoint); code != 400 {
			t.Fatal("catalog cursor scope", endpoint, code)
		}
	}
	if len(seen) != 2 || !seen[old] || !seen[current] {
		t.Fatal("catalog incomplete", seen)
	}
	for _, endpoint := range []string{"/v1/quality/actions?revision_id=" + old, "/v1/quality/revisions/" + old + "?stream=" + actionStream, "/v1/quality/summary?station_id=" + httpStation + "&revision_id=11111111-1111-4111-8111-111111111111"} {
		if code, _, _ := get(t, router, endpoint); code != 404 {
			t.Fatal("cross-stream/unknown selector", endpoint, code)
		}
	}
	for _, endpoint := range []string{"/v1/quality/assays?station_id=" + httpStation + "&revision_id=bad", "/v1/quality/revisions?stream=bad", "/v1/quality/revisions?stream=" + assayStream + "&limit=101"} {
		if code, _, _ := get(t, router, endpoint); code != 400 {
			t.Fatal("bad selector admitted", endpoint, code)
		}
	}
}

func TestHistoricalHTTPUnavailableMetadataAndKillSwitch(t *testing.T) {
	router, p := freshRouter(t, true)
	ctx := context.Background()
	var old string
	_ = p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&old)
	if _, e := p.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", old); e != nil {
		t.Fatal(e)
	}
	code, body, _ := get(t, router, "/v1/quality/revisions/"+old+"?stream="+assayStream)
	if code != 200 || body["counts"] != nil || body["manifest_hash"] != nil || len(body["resources"].([]any)) != 0 || body["lease"].(map[string]any)["available"] != false {
		t.Fatal("withdrawn provenance invented", code, body)
	}
	code, body, _ = get(t, router, "/v1/quality/assays?station_id="+httpStation+"&revision_id="+old)
	if code != 200 || len(body["items"].([]any)) != 0 {
		t.Fatal("withdrawn facts served", code, body)
	}
	if _, e := p.Exec(ctx, "UPDATE quality_revision_provenance SET provenance_state='LEGACY_UNVERIFIED',input=NULL,accepted=NULL,duplicates=NULL,quarantined=NULL,manifest_hash=NULL,publication_key=NULL WHERE revision_id=$1", old); e != nil {
		t.Fatal(e)
	}
	code, body, _ = get(t, router, "/v1/quality/revisions/"+old+"?stream="+assayStream)
	if code != 200 || body["counts"] != nil || body["lease"].(map[string]any)["provenance_state"] != "LEGACY_UNVERIFIED" || body["lease"].(map[string]any)["published_at"] == nil {
		t.Fatal("legacy metadata fabricated/lost", body)
	}
	h := Handler{Read: qualityread.NewReader(p), Enabled: func() bool { return false }}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	for _, path := range []string{"/v1/quality/revisions?stream=" + assayStream, "/v1/quality/revisions/" + old + "?stream=" + assayStream} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("kill switch", w.Code)
		}
	}
}

func TestHistoricalHTTPConcurrentSelectedAndCurrentReaders(t *testing.T) {
	_, p := freshRouter(t, true)
	ctx := context.Background()
	var old string
	_ = p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&old)
	calls := 0
	h := Handler{Read: qualityread.NewReader(p), Secrets: testSecrets, StationsExist: func(context.Context, string) (bool, error) { return true, nil }, Now: func() time.Time {
		calls++
		if calls == 2 {
			historyCorrection(t, p)
		}
		return time.Now()
	}}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	code, body, _ := get(t, r, "/v1/quality/assays?station_id="+httpStation+"&revision_id="+old)
	if code != 200 || body["lease"].(map[string]any)["revision_id"] != old {
		t.Fatal("mixed capture", body)
	}
	code, body, _ = get(t, r, "/v1/quality/assays?station_id="+httpStation)
	if code != 200 || body["lease"].(map[string]any)["revision_id"] == old || len(body["items"].([]any)) != 1 {
		t.Fatal("request-local selector leaked", body)
	}
}
