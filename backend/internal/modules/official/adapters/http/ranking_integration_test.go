//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type rankingHTTPFixture struct {
	router    chi.Router
	pool      *pgxpool.Pool
	stations  map[string]string
	weekStart time.Time
	weekEnd   time.Time
}

func freshRankingHTTPFixture(t *testing.T) *rankingHTTPFixture {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("ranking_http_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	repo := parentdirectory.NewRepository(pool)
	stations := map[string]string{}
	for _, station := range []struct{ key, cnpj, name, uf, ibge string }{
		{"a", "11222333000181", "Synthetic HTTP ranking A", "SP", "3550308"},
		{"b", "04218406000104", "Synthetic HTTP ranking B", "SP", "3509502"},
		{"c", "99888777000166", "Synthetic HTTP ranking C", "RJ", "3304557"},
	} {
		created, err := repo.ResolveCNPJ(ctx, station.cnpj, station.name, nil)
		if err != nil {
			t.Fatalf("station %s: %v", station.key, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE directory_stations SET state=$2, municipality_code=$3 WHERE id=$1", created.ID, station.uf, station.ibge); err != nil {
			t.Fatalf("geography %s: %v", station.key, err)
		}
		stations[station.key] = created.ID
	}
	reader := officialread.NewReader(pool)
	router := chi.NewRouter()
	RankingHandler{
		Read: reader, Enabled: true, Secrets: testSecrets,
		Cache: NewRankingCache(8, 100, time.Minute, time.Now),
	}.RegisterRoutes(router)
	return &rankingHTTPFixture{
		router: router, pool: pool, stations: stations,
		weekStart: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		weekEnd:   time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC),
	}
}

func (f *rankingHTTPFixture) publish(t *testing.T, checksum string, rows []domain.PriceRow) string {
	t.Helper()
	ctx := context.Background()
	im := parentofficial.NewImporter(f.pool)
	run, err := im.BeginRun(ctx, domain.ImportKey{
		SourceURL: "https://www.gov.br/anp/ranking.xlsx", SourceChecksum: checksum, ParserVersion: "mex29-test",
	}, f.weekStart, f.weekEnd)
	if err != nil || run.NoOp {
		t.Fatalf("begin = %+v, %v", run, err)
	}
	var tally domain.QuarantineTally
	if _, err := im.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
		t.Fatalf("stage: %v", err)
	}
	finished, err := im.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
	if err != nil || !finished.Published {
		t.Fatalf("finish = %+v, %v", finished, err)
	}
	return run.RevisionID
}

func (f *rankingHTTPFixture) row(t *testing.T, station string, amount int64, collected string, sourceRow int) domain.PriceRow {
	t.Helper()
	day, err := time.Parse("2006-01-02", collected)
	if err != nil {
		t.Fatal(err)
	}
	return domain.PriceRow{
		StationID: f.stations[station], Product: "GASOLINE_REGULAR", Unit: "L",
		AmountMilli: amount, RawText: "synthetic", CollectedOn: day, SourceRow: sourceRow,
	}
}

func rankingHTTPGet(t *testing.T, router chi.Router, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func rankingSpec(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	specPath := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..", "contracts", "openapi", "v1.yaml")
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("spec validation: %v", err)
	}
	return doc
}

func TestMEX29HTTPRankingOpenAPIRevisionAndConcurrency(t *testing.T) {
	fixture := freshRankingHTTPFixture(t)
	fixture.publish(t, "sha256:mex29-v1", []domain.PriceRow{
		fixture.row(t, "a", 5900, "2025-01-07", 1),
		fixture.row(t, "a", 5999, "2025-01-08", 2),
		fixture.row(t, "b", 6001, "2025-01-08", 3),
		fixture.row(t, "c", 6100, "2025-01-08", 4),
	})
	target := rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 10)
	first := rankingHTTPGet(t, fixture.router, target, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("ranking %d %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" || first.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatalf("headers etag=%q cache=%q", etag, first.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	schema := rankingSpec(t).Components.Schemas["PriceRankingResponse"].Value
	if err := schema.VisitJSON(body); err != nil {
		t.Fatalf("response does not match OpenAPI: %v", err)
	}
	areas := body["areas"].([]any)
	if len(areas) != 3 || areas[0].(map[string]any)["ibge_code"] != "3550308" ||
		areas[0].(map[string]any)["mean"].(map[string]any)["numerator"] != "5999" {
		t.Fatalf("areas %v", areas)
	}
	coverage := body["coverage"].(map[string]any)
	if coverage["source_observations"] != float64(4) || coverage["stations_with_price"] != float64(3) ||
		coverage["source_stations"] != float64(3) {
		t.Fatalf("coverage %v", coverage)
	}

	conditional := rankingHTTPGet(t, fixture.router, target, map[string]string{"If-None-Match": etag})
	if conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional %d", conditional.Code)
	}

	// Concurrent fills share the bounded cache and never disagree on source.
	var wait sync.WaitGroup
	statuses := make(chan int, 8)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			statuses <- rankingHTTPGet(t, fixture.router, target, nil).Code
		}()
	}
	wait.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent status %d", status)
		}
	}

	// Capture a page cursor from the current revision before it is replaced.
	pageTarget := rankingTarget("AREA_MEAN", "MUNICIPALITY", "ASC", 2)
	page := rankingHTTPGet(t, fixture.router, pageTarget, nil)
	var pageBody map[string]any
	if err := json.Unmarshal(page.Body.Bytes(), &pageBody); err != nil {
		t.Fatal(err)
	}
	cursor, _ := pageBody["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing page cursor")
	}

	// Revision switch invalidates the old ETag and any old cursor.
	fixture.publish(t, "sha256:mex29-v2", []domain.PriceRow{
		fixture.row(t, "a", 5900, "2025-01-07", 10),
		fixture.row(t, "a", 5800, "2025-01-08", 11),
		fixture.row(t, "b", 6001, "2025-01-08", 12),
		fixture.row(t, "c", 6100, "2025-01-08", 13),
	})
	switched := rankingHTTPGet(t, fixture.router, target, map[string]string{"If-None-Match": etag})
	if switched.Code != http.StatusOK || switched.Header().Get("ETag") == etag {
		t.Fatalf("stale ETag admitted: %d", switched.Code)
	}
	var switchedBody map[string]any
	if err := json.Unmarshal(switched.Body.Bytes(), &switchedBody); err != nil {
		t.Fatal(err)
	}
	if switchedBody["areas"].([]any)[0].(map[string]any)["mean"].(map[string]any)["numerator"] != "5800" {
		t.Fatalf("correction not served: %v", switchedBody["areas"])
	}
	if staleCursor := rankingHTTPGet(t, fixture.router, pageTarget+"&cursor="+url.QueryEscape(cursor), nil); staleCursor.Code != http.StatusBadRequest {
		t.Fatalf("cursor survived revision switch: %d", staleCursor.Code)
	}
}
