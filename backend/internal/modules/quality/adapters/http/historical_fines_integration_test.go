//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
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

func historyHTTPInput(period string) application.HistoricalFineInput {
	p, _ := application.HistoricalFineProfile(period)
	clock := application.FineHistoryClock{Precision: p.ClockPrecision, Value: "2024-06"}
	if p.ClockPrecision == "DAY" {
		clock.Value = "2020-02-06"
	}
	paid := int64(0)
	a := application.HistoricalFineObservation{Schema: "anp-fine-history-observation-v1", Source: "sanctions", SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", ReferenceLabel: p.ReferenceLabel, ReferenceRef: "SYNTHETIC/1", Status: "Synthetic source status", DueOn: "2019-02-28", AmountText: "R$ 1.234,56", AmountMinor: 123456, PaidText: "R$ 0,00", PaidMinor: &paid, SourcePeriod: period, Clock: clock, InputContract: p.Contract, EvidenceRef: p.SourceFile + ":" + strconv.Itoa(p.HeaderRecord+1)}
	if period == "2023" {
		a.PaidMinor = nil
		a.PaidText = "R$ -"
	}
	if p.Transit {
		date := "2019-02-20"
		a.SourceTransitOn = &date
	}
	a.ObservationKey = a.ObservationIdentity()
	b := a
	b.ReferenceRef = "SYNTHETIC/2"
	b.EvidenceRef = p.SourceFile + ":" + strconv.Itoa(p.HeaderRecord+2)
	b.ObservationKey = b.ObservationIdentity()
	in := application.HistoricalFineInput{Context: application.FineHistoryContext{InputContract: p.Contract, SourcePeriod: period, SourceFile: p.SourceFile, HeaderRecord: p.HeaderRecord, Clock: clock, SourceRecordCount: 2}, Resources: []application.Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/" + p.SourceFile, SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: domain.Counts{Input: 2, Accepted: 2}, ManifestHash: strings.Repeat("b", 64), Observations: []application.HistoricalFineObservation{a, b}}
	if period == "2020" {
		in.Context.SourceRecordCount = 3
		in.Context.MetadataRecords = 1
		in.Counts.Input = 3
		in.Counts.Quarantined = 1
		in.Notes = []application.FineHistoryNote{{Schema: "anp-fine-source-note-v1", Kind: "PUBLIC_PAYMENT_NOTE", Text: "(*) Para verificação do valor total pago dos processos com status parcelamento em andamento é preciso acessar o link: https://app.anp.gov.br/anp-gpc-consulta-web/public/emissao-gru/list.xhtml", EvidenceRef: p.SourceFile + ":" + strconv.Itoa(p.HeaderRecord+3)}}
	}
	return in
}
func TestHistoricalFineActualWireClocksNullsNotesHistoryAndOpenAPI(t *testing.T) {
	for _, period := range []string{"2016-2019", "2020", "2023"} {
		t.Run(period, func(t *testing.T) {
			_, p := freshRouter(t, true)
			ctx := context.Background()
			n := 0
			id := func() (string, error) { n++; return fmt.Sprintf("aaaaaaaa-8888-4888-8888-%012d", n), nil }
			in := historyHTTPInput(period)
			pub := parent.NewPublisher(p)
			first, e := pub.PublishHistoricalFines(ctx, in, id)
			if e != nil {
				t.Fatal(e)
			}
			at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			h := Handler{Read: qualityread.NewReader(p), Secrets: testSecrets, Now: func() time.Time { return at }}
			router := chi.NewRouter()
			h.RegisterRoutes(router)
			_, file, _, _ := runtime.Caller(0)
			spec, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
			if e != nil {
				t.Fatal(e)
			}
			request := func(path, schema, etag string) (*httptest.ResponseRecorder, map[string]any) {
				t.Helper()
				w := httptest.NewRecorder()
				r := httptest.NewRequest("GET", path, nil)
				if etag != "" {
					r.Header.Set("If-None-Match", etag)
				}
				router.ServeHTTP(w, r)
				if w.Code != 200 && w.Code != 304 {
					t.Fatal("historical public read failed", w.Code)
				}
				if w.Code == 304 {
					return w, nil
				}
				var body map[string]any
				if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
					t.Fatal(e)
				}
				if e := spec.Components.Schemas[schema].Value.VisitJSON(body); e != nil {
					t.Fatal("actual historical OpenAPI mismatch", e)
				}
				for _, private := range []string{"11222333000181", "party_cnpj", "source_key"} {
					if strings.Contains(w.Body.String(), private) {
						t.Fatal("public history exposed identity")
					}
				}
				return w, body
			}
			firstPage, page := request("/v1/quality/sanctions?limit=1", "QualitySanctionPage", "")
			lease := page["lease"].(map[string]any)
			item := page["items"].([]any)[0].(map[string]any)
			if lease["source_period"] != period || lease["source_input_contract"] != in.Context.InputContract || lease["source_clock_precision"] != in.Context.Clock.Precision || lease["source_clock_value"] != in.Context.Clock.Value || lease["source_metadata_records"] != float64(in.Context.MetadataRecords) || lease["source_record_count"] != float64(in.Context.SourceRecordCount) || lease["stale"] != true {
				t.Fatal("source lease facts lost")
			}
			if len(in.Notes) > 0 && len(lease["source_notes"].([]any)) != 1 {
				t.Fatal("public source caveat lost")
			}
			if item["source_period"] != period || item["source_clock_value"] != in.Context.Clock.Value || item["reference_label"] != in.Observations[0].ReferenceLabel || item["as_of"] != nil || item["decided_on"] != nil || item["duf_ref"] != nil || item["report_year"] != nil || item["extraction_month"] != nil {
				t.Fatal("historical fields relabelled")
			}
			if period == "2023" && item["paid_minor"] != nil {
				t.Fatal("unknown paid became zero")
			}
			if in.Observations[0].SourceTransitOn != nil && item["source_transit_on"] != *in.Observations[0].SourceTransitOn {
				t.Fatal("source-declared transit lost")
			}
			rowID := item["id"].(string)
			_, detail := request("/v1/quality/sanctions/"+rowID, "QualitySanction", "")
			if detail["reference_ref"] != item["reference_ref"] {
				t.Fatal("page/detail disagree")
			}
			if w, _ := request("/v1/quality/sanctions?limit=1", "QualitySanctionPage", firstPage.Header().Get("ETag")); w.Code != 304 {
				t.Fatal("stable source did not cache")
			}
			cursor := page["next_cursor"].(string)
			_, last := request("/v1/quality/sanctions?limit=1&cursor="+cursor, "QualitySanctionPage", "")
			if last["next_cursor"] != nil || last["items"].([]any)[0].(map[string]any)["id"] == rowID {
				t.Fatal("historical traversal duplicated/lost row")
			}
			layout := "2006-01"
			if in.Context.Clock.Precision == "DAY" {
				layout = "2006-01-02"
			}
			start, _ := time.Parse(layout, in.Context.Clock.Value)
			at = start.Add(180 * 24 * time.Hour)
			fresh, response := request("/v1/quality/sanctions", "QualitySanctionPage", "")
			if response["lease"].(map[string]any)["stale"] != false {
				t.Fatal("source-age boundary changed")
			}
			at = at.Add(time.Second)
			stale, _ := request("/v1/quality/sanctions", "QualitySanctionPage", fresh.Header().Get("ETag"))
			if stale.Code != 200 || stale.Header().Get("ETag") == fresh.Header().Get("ETag") {
				t.Fatal("source age hidden by304")
			}
			oldClock := in.Context.Clock.Value
			in.ManifestHash = strings.Repeat("e", 64)
			if in.Context.Clock.Precision == "DAY" {
				in.Context.Clock.Value = "2020-02-07"
			} else {
				in.Context.Clock.Value = "2024-07"
			}
			for i := range in.Observations {
				in.Observations[i].Clock = in.Context.Clock
				in.Observations[i].ObservationKey = in.Observations[i].ObservationIdentity()
			}
			if _, e = pub.PublishHistoricalFines(ctx, in, id); e != nil {
				t.Fatal(e)
			}
			if code, _, _ := get(t, router, "/v1/quality/sanctions?limit=1&cursor="+cursor); code != 400 {
				t.Fatal("old cursor crossed correction", code)
			}
			if code, _, _ := get(t, router, "/v1/quality/sanctions/"+rowID); code != 404 {
				t.Fatal("old row crossed current revision", code)
			}
			_, old := request("/v1/quality/sanctions/"+rowID+"?revision_id="+first, "QualitySanction", "")
			if old["source_clock_value"] != oldClock || old["lease"].(map[string]any)["revision_status"] != "superseded" {
				t.Fatal("frozen public history changed")
			}
		})
	}
}
