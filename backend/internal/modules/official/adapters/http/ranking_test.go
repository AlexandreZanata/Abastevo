package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

var rankingTestSecrets = []byte("ranking-test-cursor-secret-32b")

type fakeRankingReader struct {
	mu         sync.Mutex
	revision   application.RankingRevision
	data       application.RankingData
	resolveErr error
	readErr    error
	resolves   int
	reads      int
}

func (f *fakeRankingReader) ResolveRankingRevision(context.Context, string) (application.RankingRevision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolves++
	if f.resolveErr != nil {
		return application.RankingRevision{}, f.resolveErr
	}
	return f.revision, nil
}

func (f *fakeRankingReader) ReadRankingAt(context.Context, application.RankingQuery, application.RankingRevision) (application.RankingData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.readErr != nil {
		return application.RankingData{}, f.readErr
	}
	return f.data, nil
}

func (f *fakeRankingReader) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolves, f.reads
}

func fakeRankingRevision(id string) application.RankingRevision {
	return application.RankingRevision{
		RevisionID: id, SurveyWeekStart: "2025-01-06", SurveyWeekEnd: "2025-01-12",
		PublishedAt: "2025-01-13T12:00:00Z", SourceURL: "https://example.test/anp.xlsx",
		SourceChecksum: "sha256:test",
	}
}

func fakeRankingData() application.RankingData {
	return application.RankingData{
		Revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"),
		Municipalities: []application.MunicipalityAggregate{
			{State: "SP", MunicipalityCode: "3550308", SumMilliBrl: 5999, ObservationCount: 1, MinMilliBrl: 5999, MaxMilliBrl: 5999},
			{State: "SP", MunicipalityCode: "3509502", SumMilliBrl: 6001, ObservationCount: 1, MinMilliBrl: 6001, MaxMilliBrl: 6001},
			{State: "RJ", MunicipalityCode: "3304557", SumMilliBrl: 6100, ObservationCount: 1, MinMilliBrl: 6100, MaxMilliBrl: 6100},
		},
		Stations: []application.StationObservationRow{
			{StationID: "aaaaaaaa-1111-4111-8111-000000000001", AmountMilliBrl: 5999, CollectedOn: "2025-01-08"},
			{StationID: "bbbbbbbb-1111-4111-8111-000000000002", AmountMilliBrl: 6001, CollectedOn: "2025-01-08"},
			{StationID: "cccccccc-1111-4111-8111-000000000003", AmountMilliBrl: 6100, CollectedOn: "2025-01-08"},
		},
		SourceStations: 4, SourceObservations: 4,
	}
}

func rankingTarget(metric, groupBy, sort string, limit int) string {
	return "/v1/prices/ranking?source=ANP_WEEKLY&survey_week_start=2025-01-06" +
		"&fuel=GASOLINE_REGULAR&unit=L&geography=COUNTRY&metric=" + metric +
		"&group_by=" + groupBy + "&sort=" + sort + "&limit=" + strconv.Itoa(limit)
}

func newRankingRouter(reader application.RankingReader, enabled bool, cache *RankingCache, now func() time.Time) chi.Router {
	router := chi.NewRouter()
	RankingHandler{
		Read: reader, Enabled: enabled, Secrets: rankingTestSecrets, Cache: cache, Now: now,
	}.RegisterRoutes(router)
	return router
}

func rankingRequest(t *testing.T, router chi.Router, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeRankingBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func TestMEX29ColdWarmSQLBytesAndETag(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	cache := NewRankingCache(8, 100, time.Minute, func() time.Time { return now })
	router := newRankingRouter(reader, true, cache, func() time.Time { return now })
	target := rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10)

	first := rankingRequest(t, router, target, nil)
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatalf("cold response %d %q", first.Code, first.Header().Get("Cache-Control"))
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	coldBytes := first.Body.Len()
	body := decodeRankingBody(t, first)
	if len(body["areas"].([]any)) != 3 {
		t.Fatalf("areas %v", body["areas"])
	}
	coverage := body["coverage"].(map[string]any)
	if coverage["sampled"] != true || coverage["national_completeness_claimed"] != false {
		t.Fatalf("coverage %v", coverage)
	}

	second := rankingRequest(t, router, target, map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("conditional response %d %q", second.Code, second.Body.String())
	}
	third := rankingRequest(t, router, target, nil)
	if third.Code != http.StatusOK || third.Body.Len() != coldBytes {
		t.Fatalf("warm response %d bytes=%d want=%d", third.Code, third.Body.Len(), coldBytes)
	}
	resolves, reads := reader.counts()
	if resolves != 3 || reads != 1 {
		t.Fatalf("cold/warm SQL counts resolves=%d reads=%d", resolves, reads)
	}
}

func TestMEX29RevisionSwitchInvalidatesCacheAndETag(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	cache := NewRankingCache(8, 100, time.Minute, func() time.Time { return now })
	router := newRankingRouter(reader, true, cache, func() time.Time { return now })
	target := rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10)
	first := rankingRequest(t, router, target, nil)
	oldETag := first.Header().Get("ETag")

	reader.mu.Lock()
	reader.revision = fakeRankingRevision("22222222-2222-4222-8222-222222222222")
	reader.data.Revision = reader.revision
	reader.data.Municipalities[0].SumMilliBrl = 5800
	reader.data.Municipalities[0].MinMilliBrl = 5800
	reader.data.Municipalities[0].MaxMilliBrl = 5800
	reader.mu.Unlock()

	second := rankingRequest(t, router, target, map[string]string{"If-None-Match": oldETag})
	if second.Code != http.StatusOK {
		t.Fatalf("stale ETag answered %d instead of 200", second.Code)
	}
	if second.Header().Get("ETag") == oldETag {
		t.Fatal("ETag did not bind the source revision")
	}
	body := decodeRankingBody(t, second)
	source := body["source"].(map[string]any)
	if source["revision_id"] != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("source revision %v", source)
	}
	areas := body["areas"].([]any)
	firstArea := areas[0].(map[string]any)
	if firstArea["mean"].(map[string]any)["numerator"] != "5800" {
		t.Fatalf("corrected value %v", firstArea)
	}
}

func TestMEX29OutageDoesNotServeStaleCache(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	cache := NewRankingCache(8, 100, time.Minute, func() time.Time { return now })
	router := newRankingRouter(reader, true, cache, func() time.Time { return now })
	target := rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10)
	if first := rankingRequest(t, router, target, nil); first.Code != http.StatusOK {
		t.Fatal(first.Code)
	}
	reader.mu.Lock()
	reader.resolveErr = errors.New("database down")
	reader.mu.Unlock()
	outage := rankingRequest(t, router, target, nil)
	if outage.Code != http.StatusServiceUnavailable || outage.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("outage response %d %q", outage.Code, outage.Header().Get("Cache-Control"))
	}
	if strings.Contains(outage.Body.String(), "5999") {
		t.Fatal("stale cached price leaked during outage")
	}
}

func TestMEX29DisabledAndInvalidInputs(t *testing.T) {
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	disabled := newRankingRouter(reader, false, nil, time.Now)
	if response := rankingRequest(t, disabled, rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10), nil); response.Code != http.StatusNotFound {
		t.Fatalf("disabled answered %d", response.Code)
	}
	router := newRankingRouter(reader, true, NewRankingCache(8, 100, time.Minute, time.Now), time.Now)
	for _, target := range []string{
		rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10) + "&bogus=1",
		rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10) + "&limit=20",
		strings.Replace(rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10), "ANP_WEEKLY", "COMMUNITY", 1),
		rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10) + "&condition=MEMBER",
		rankingTarget("AREA_MEAN", "COUNTRY", "ASC", 10),
		rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 0),
		rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10) + "&cursor=bad",
	} {
		response := rankingRequest(t, router, target, nil)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid target %q answered %d", target, response.Code)
		}
	}
}

func TestMEX29CursorPaginationAndFilterBinding(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	router := newRankingRouter(reader, true, NewRankingCache(8, 100, time.Minute, func() time.Time { return now }), func() time.Time { return now })

	stations := rankingTarget("OBSERVATION_PRICE", "STATION", "ASC", 2)
	first := rankingRequest(t, router, stations, nil)
	body := decodeRankingBody(t, first)
	cursor, ok := body["next_cursor"].(string)
	if !ok || cursor == "" || len(body["stations"].([]any)) != 2 {
		t.Fatalf("first station page %v", body)
	}
	second := rankingRequest(t, router, stations+"&cursor="+url.QueryEscape(cursor), nil)
	secondBody := decodeRankingBody(t, second)
	items := secondBody["stations"].([]any)
	if second.Code != http.StatusOK || len(items) != 1 || secondBody["next_cursor"] != nil {
		t.Fatalf("second station page %d %v", second.Code, secondBody)
	}

	areas := rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 2)
	firstArea := rankingRequest(t, router, areas, nil)
	areaBody := decodeRankingBody(t, firstArea)
	areaCursor, ok := areaBody["next_cursor"].(string)
	if !ok || areaCursor == "" || len(areaBody["areas"].([]any)) != 2 {
		t.Fatalf("first area page %v", areaBody)
	}
	secondArea := rankingRequest(t, router, areas+"&cursor="+url.QueryEscape(areaCursor), nil)
	secondAreaBody := decodeRankingBody(t, secondArea)
	if len(secondAreaBody["areas"].([]any)) != 1 || secondAreaBody["next_cursor"] != nil {
		t.Fatalf("second area page %v", secondAreaBody)
	}
	// A cursor minted for ASC must not open a DESC page.
	mismatched := rankingRequest(t, router, rankingTarget("AREA_MEAN", "MUNICIPALITY", "DESC", 2)+"&cursor="+url.QueryEscape(areaCursor), nil)
	if mismatched.Code != http.StatusBadRequest {
		t.Fatalf("cursor crossed filters: %d", mismatched.Code)
	}
}

func TestMEX29ExpiredCursorRefused(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	router := newRankingRouter(reader, true, NewRankingCache(8, 100, time.Minute, func() time.Time { return now }), func() time.Time { return now })
	target := rankingTarget("OBSERVATION_PRICE", "STATION", "ASC", 2)
	request := httptest.NewRequest(http.MethodGet, target, nil)
	query, err := parseRankingQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	hash := rankingFilterHash(query, reader.revision)
	expired := httpapi.Seal(rankingTestSecrets, hash, "s|5999|aaaaaaaa-1111-4111-8111-000000000001", now.Add(-time.Hour))
	response := rankingRequest(t, router, target+"&cursor="+url.QueryEscape(expired), nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expired cursor answered %d", response.Code)
	}
}

func TestMEX29CancellationWritesNothing(t *testing.T) {
	reader := &fakeRankingReader{revision: fakeRankingRevision("11111111-1111-4111-8111-111111111111"), data: fakeRankingData()}
	router := newRankingRouter(reader, true, NewRankingCache(8, 100, time.Minute, time.Now), time.Now)
	request := httptest.NewRequest(http.MethodGet, rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10), nil)
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Body.Len() != 0 {
		t.Fatalf("canceled request wrote %q", recorder.Body.String())
	}
	resolves, reads := reader.counts()
	if resolves != 0 || reads != 0 {
		t.Fatalf("canceled request touched storage resolves=%d reads=%d", resolves, reads)
	}
}
