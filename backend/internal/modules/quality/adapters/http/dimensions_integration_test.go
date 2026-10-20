//go:build integration

package http

import (
	"context"
	"strings"
	"testing"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestSummarySeparatesSampleDimensionsAndInvalidatesPendingETag(t *testing.T) {
	r, pool := freshRouter(t, true)
	path := "/v1/quality/summary?station_id=" + httpStation + "&fuel=ETHANOL"
	code, before, headers := get(t, r, path)
	state, ok := before["sample_dimensions"].(map[string]any)
	if code != 200 || !ok || state["available"] != false {
		t.Fatal("pending projection fabricated or omitted", code, before)
	}
	var rev string
	if err := pool.QueryRow(context.Background(), "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&rev); err != nil {
		t.Fatal(err)
	}
	if _, err := parent.NewProjectionStore(pool).RecomputeRevision(context.Background(), "pmqc", "pmqc-assay-v1", rev); err != nil {
		t.Fatal(err)
	}
	_, after, nextHeaders := get(t, r, path)
	state = after["sample_dimensions"].(map[string]any)
	if state["available"] != true || headers.Get("ETag") == nextHeaders.Get("ETag") {
		t.Fatal("completed projection did not invalidate pending response", state)
	}
	items := state["items"].([]any)
	if len(items) != 1 {
		t.Fatal("wrong fuel/station dimensions", items)
	}
	cell := items[0].(map[string]any)
	if cell["samples"] != float64(1) || cell["visits"] != float64(1) || cell["conforming"] != float64(1) || cell["score"] != nil || cell["score_state"] != "UNSCORED" {
		t.Fatal("assays counted as samples or score invented", cell)
	}
	_, other, _ := get(t, r, "/v1/quality/summary?station_id="+httpStation+"&fuel=DIESEL_S10")
	if dimension := other["sample_dimensions"].(map[string]any); dimension["available"] != false || len(dimension["items"].([]any)) != 0 {
		t.Fatal("foreign fuel leaked dimensions", dimension)
	}
}

func TestSummaryDimensionsCannotCrossStationOrCorrectionBasis(t *testing.T) {
	r, pool := freshRouter(t, true)
	ctx := context.Background()
	var original string
	if err := pool.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&original); err != nil {
		t.Fatal(err)
	}
	store := parent.NewProjectionStore(pool)
	if _, err := store.RecomputeRevision(ctx, assaySource, assayStream, original); err != nil {
		t.Fatal(err)
	}
	const otherStation = "eeeeeeee-5555-4555-8555-000000000002"
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic other')", otherStation); err != nil {
		t.Fatal(err)
	}
	_, other, _ := get(t, r, "/v1/quality/summary?station_id="+otherStation)
	if other["sample_dimensions"].(map[string]any)["available"] != false {
		t.Fatal("foreign station projection served")
	}
	in := application.Input{Source: assaySource, Stream: assayStream, Edition: "2026-09", Resources: []application.Resource{{URL: "https://www.gov.br/anp/pmqc_2026_09.zip", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("e", 64), Assays: []application.AssayInput{{SampleRef: "corrected", AssayRef: "Aspecto", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "LII", Conformity: "CONFORMING", EvidenceRef: "synthetic", Fingerprint: strings.Repeat("e", 64)}}, Links: []application.LinkInput{{RecordKey: "pmqc-assay-v1:corrected", StationID: httpStation, Automatic: true, Reason: "exact cnpj", Active: true}}}
	revision, err := parent.NewPublisher(pool).Publish(ctx, in, jobs.NewUUIDv4)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/quality/summary?station_id=" + httpStation
	_, pending, _ := get(t, r, path)
	if pending["sample_dimensions"].(map[string]any)["available"] != false {
		t.Fatal("prior basis served after correction")
	}
	if _, err = store.RecomputeRevision(ctx, assaySource, assayStream, revision); err != nil {
		t.Fatal(err)
	}
	_, old, _ := get(t, r, path+"&revision_id="+original)
	if old["sample_dimensions"].(map[string]any)["available"] != false || len(old["fuel_cells"].([]any)) != 1 {
		t.Fatal("historical facts replaced by current dimensions", old)
	}
}
