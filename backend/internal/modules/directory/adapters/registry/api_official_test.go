package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const officialSyntheticRecord = `{"cnpj":"04218406000104","razaoSocial":"[ING-AUDIT-TEST] STATION","endereco":"TEST STREET, 10","municipio":"SAO PAULO","uf":"SP","latitude":"-23.561","longitude":"-46.656","srid":"4674","src":"SIRGAS2000","validacao":"Posição Geográfica não validada","estimativaAcuracia":"4","dataObtencao":"20/03/2018","origemInformacao":"Agente de fiscalização da ANP"}`

func officialPage(page, pages, total int, records string) string {
	return fmt.Sprintf(`{"status":200,"succeeded":true,"data":[%s],"searchPageFilter":{"numeroPagina":%d,"tamanhoPagina":5000,"totalRegistro":%d,"totalPagina":%d}}`, records, page, total, pages)
}

func TestOfficialANPContractRetainsUnreviewedCoordinates(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/combustivel" || r.URL.Query().Get("numeropagina") != "1" {
			t.Errorf("wrong official target: %s", r.URL)
		}
		fmt.Fprint(w, officialPage(1, 1, 1, officialSyntheticRecord))
	}))
	defer s.Close()
	store := newFakeStore()
	got, err := Discover(context.Background(), store, "official-contract", testAPIConfig(s))
	if err != nil || got.State != "complete" || got.Accepted != 1 {
		t.Fatalf("official response lost: %+v %v", got, err)
	}
	rows, err := store.ListAssertions(context.Background(), got.RunID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	a := rows[0]
	if !a.HasCoords || a.CRS != "SIRGAS2000" || a.LocationQuality != "unknown" || a.EffectiveDate.Time.Year() != 2018 {
		t.Fatalf("candidate provenance lost or promoted: %+v", a)
	}
}

func TestDiscoverUnknownJSONNeverCompletesEmpty(t *testing.T) {
	for _, body := range []string{`{}`, `{"error":"provider unavailable"}`, `{"status":200,"succeeded":true,"data":[]}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer s.Close()
			got, err := Discover(context.Background(), newFakeStore(), "bad-schema", testAPIConfig(s))
			if err == nil && got.State == "complete" {
				t.Fatalf("schema drift silently complete: %+v", got)
			}
		})
	}
}

func TestOfficialANPExplicitNoResultsIsEmpty(t *testing.T) {
	got, err := decodeAPIPage([]byte(`{"status":200,"title":"Nenhum dado encontrado","succeeded":true,"data":[]}`), 1)
	if err != nil || len(got.Items) != 0 || got.NextCursor != nil {
		t.Fatalf("official empty response: %+v %v", got, err)
	}
}

func TestOfficialANPRejectsIncompleteOrInconsistentPages(t *testing.T) {
	for name, body := range map[string]string{
		"wrong-page":        officialPage(2, 1, 1, officialSyntheticRecord),
		"short-page":        officialPage(1, 1, 2, officialSyntheticRecord),
		"null-data":         `{"status":200,"succeeded":true,"data":null}`,
		"failed-status":     `{"status":500,"succeeded":false,"data":[]}`,
		"invented-contract": `{"items":[],"nextCursor":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAPIPage([]byte(body), 1); err == nil {
				t.Fatal("accepted invalid official snapshot")
			}
		})
	}
}

func TestOfficialANPRetryQuotaCountsEveryHTTPRequest(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(429) }))
	defer s.Close()
	cfg := testAPIConfig(s)
	cfg.MaxRequests = 1
	got, err := Discover(context.Background(), newFakeStore(), "retry-limit", cfg)
	if err != nil || calls != 1 || got.State != "quarantined" || got.ErrorCode != "request_quota" {
		t.Fatalf("calls=%d report=%+v err=%v", calls, got, err)
	}
}

func TestOfficialANPOversizeAndForeignScopeFailClosed(t *testing.T) {
	for _, oversize := range []bool{false, true} {
		t.Run(fmt.Sprint(oversize), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, officialPage(1, 1, 1, officialSyntheticRecord))
			}))
			defer s.Close()
			cfg := testAPIConfig(s)
			cfg.CNPJ = "12ABC34501DE35"
			if oversize {
				cfg.MaxBytes = 8
			}
			store := newFakeStore()
			got, err := Discover(context.Background(), store, "scope", cfg)
			if err == nil && got.State == "complete" {
				t.Fatalf("accepted oversize/foreign scope: %+v", got)
			}
			rows, _ := store.ListAssertions(context.Background(), got.RunID)
			if len(rows) != 0 {
				t.Fatal("failure staged a row")
			}
		})
	}
}

func TestOfficialANPPointMetadataNeverPromotesClaimedReview(t *testing.T) {
	for _, change := range []string{`"validacao":"reviewed"`, `"validacao":"Posição Geográfica validada"`} {
		raw := strings.Replace(officialSyntheticRecord, `"validacao":"Posição Geográfica não validada"`, change, 1)
		a, ok := parseOfficialAPIRecord(json.RawMessage(raw))
		if !ok || a.LocationQuality != "unknown" || !strings.Contains(a.SourceReference, `"accuracy_m_claim":"4"`) {
			t.Fatalf("source claim lost/promoted: %+v", a)
		}
	}
	for _, raw := range []string{
		strings.Replace(officialSyntheticRecord, `"srid":"4674"`, `"srid":"4326"`, 1),
		strings.Replace(officialSyntheticRecord, `"latitude":"-23.561"`, `"latitude":"NaN"`, 1),
		strings.Replace(officialSyntheticRecord, `"longitude":"-46.656"`, `"longitude":""`, 1),
		strings.Replace(officialSyntheticRecord, `"dataObtencao":"20/03/2018"`, `"dataObtencao":"31/02/2026"`, 1),
	} {
		if _, ok := parseOfficialAPIRecord(json.RawMessage(raw)); ok {
			t.Fatal("accepted invalid candidate")
		}
	}
}

func TestOfficialANPParserVersionDoesNotReplayFalseLegacyEmpty(t *testing.T) {
	store := newFakeStore()
	ctx := context.Background()
	_, _, _ = store.CreateRun(ctx, newUUID(), SourceAPI, "legacy-empty", "legacy")
	old, _ := store.GetRun(ctx, SourceAPI, "legacy-empty")
	_ = store.FinishRun(ctx, old.RunID, "complete", 0, 0, 0, "")
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, officialPage(1, 1, 1, officialSyntheticRecord))
	}))
	defer s.Close()
	got, err := Discover(ctx, store, "legacy-empty", testAPIConfig(s))
	if err != nil || got.RunID == old.RunID || got.Accepted != 1 || calls != 1 {
		t.Fatalf("false legacy completion replayed: %+v %v calls=%d", got, err, calls)
	}
}
