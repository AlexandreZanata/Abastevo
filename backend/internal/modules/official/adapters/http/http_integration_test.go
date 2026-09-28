//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

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

var testSecrets = []byte("test-cursor-secret-32-bytes-xxxx")

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

// freshRouter seeds one station with two published revisions (old corrected
// by new) and serves the official reads with a wired existence check.
func freshRouter(t *testing.T) (chi.Router, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("prices_test_%d", time.Now().UnixNano())
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
	dirRepo := parentdirectory.NewRepository(pool)
	st, err := dirRepo.ResolveCNPJ(ctx, "04218406000104", "Posto Alfa", nil)
	if err != nil {
		t.Fatalf("station: %v", err)
	}
	im := parentofficial.NewImporter(pool)
	weekStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	weekEnd := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)
	publish := func(checksum string, rows []domain.PriceRow) {
		t.Helper()
		run, err := im.BeginRun(ctx, domain.ImportKey{
			SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: checksum, ParserVersion: "p02-t08",
		}, weekStart, weekEnd)
		if err != nil || run.NoOp {
			t.Fatalf("begin = %+v, %v", run, err)
		}
		var tally domain.QuarantineTally
		if _, err := im.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
			t.Fatalf("stage: %v", err)
		}
		fin, err := im.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
		if err != nil || !fin.Published {
			t.Fatalf("finish = %+v, %v", fin, err)
		}
	}
	row := func(n, milli int64, collected string) domain.PriceRow {
		day, err := time.Parse("2006-01-02", collected)
		if err != nil {
			t.Fatal(err)
		}
		return domain.PriceRow{StationID: st.ID, Product: "GASOLINE_REGULAR", Unit: "L",
			AmountMilli: milli, RawText: "5,999", CollectedOn: day, SourceRow: int(n)}
	}
	publish("sha256:old", []domain.PriceRow{row(8, 5999, "2025-01-08"), row(9, 5899, "2025-01-07")})
	publish("sha256:new", []domain.PriceRow{row(8, 6099, "2025-01-08"), row(9, 5899, "2025-01-07")})
	r := chi.NewRouter()
	prices := officialread.NewReader(pool)
	exists := func(ctx context.Context, id string) (bool, error) {
		_, err := dirRepo.Station(ctx, id)
		return err == nil, nil
	}
	Handler{Prices: prices, StationsExist: exists, Secrets: testSecrets}.RegisterRoutes(r)
	return r, st.ID
}

func get(t *testing.T, r chi.Router, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return body
}

func TestGroupsEnvelope(t *testing.T) {
	r, station := freshRouter(t)
	w := get(t, r, "/v1/stations/"+station+"/prices", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("cache = %q", cc)
	}
	body := decodeBody(t, w)
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("groups = %d, want 1 (current revision only)", len(items))
	}
	g := items[0].(map[string]any)
	if g["fuel_product"] != "GASOLINE_REGULAR" {
		t.Errorf("group = %v", g)
	}
	official, _ := g["official"].(map[string]any)
	if official["amount_milli_brl"] != float64(6099) {
		t.Errorf("current value = %v, want corrected 6099", official)
	}
	if official["source"] != "ANP" || official["revision_id"] == nil {
		t.Errorf("provenance = %v", official)
	}
	if g["community"] != nil {
		t.Errorf("community not null: %v", g["community"])
	}
	if cond, _ := g["condition"].(map[string]any); cond["kind"] != "STANDARD" {
		t.Errorf("condition = %v", cond)
	}
	// Fuel filter with no rows keeps the envelope with empty items.
	w2 := get(t, r, "/v1/stations/"+station+"/prices?fuel_product=CNG", nil)
	b2 := decodeBody(t, w2)
	if len(b2["items"].([]any)) != 0 {
		t.Errorf("filtered groups = %v", b2["items"])
	}
	// Unknown station and bad fuel fail honestly.
	w3 := get(t, r, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/prices", nil)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("unknown = %d", w3.Code)
	}
	w4 := get(t, r, "/v1/stations/"+station+"/prices?fuel_product=JET", nil)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("bad fuel = %d", w4.Code)
	}
}

func TestHistoryWalk(t *testing.T) {
	r, station := freshRouter(t)
	w := get(t, r, "/v1/stations/"+station+"/official-prices?limit=1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag missing")
	}
	w2 := get(t, r, "/v1/stations/"+station+"/official-prices?limit=1", map[string]string{"If-None-Match": etag})
	if w2.Code != http.StatusNotModified {
		t.Fatalf("conditional = %d", w2.Code)
	}
	body := decodeBody(t, w)
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("page one = %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["collected_on"] != "2025-01-08" {
		t.Errorf("newest first = %v", first)
	}
	// Amount and revision stay coherent whichever tie wins the same date.
	amount, _ := first["amount_milli_brl"].(float64)
	checksum := first["source"].(map[string]any)["checksum"]
	if (amount == 6099) != (checksum == "sha256:new") {
		t.Errorf("amount/revision incoherent: %v", first)
	}
	next, _ := body["next_cursor"].(string)
	if next == "" {
		t.Fatal("missing next cursor")
	}
	// Walk to the end across both revisions.
	seen := 1
	cursor := next
	for i := 0; i < 6 && cursor != ""; i++ {
		wi := get(t, r, "/v1/stations/"+station+"/official-prices?limit=1&cursor="+url.QueryEscape(cursor), nil)
		if wi.Code != http.StatusOK {
			t.Fatalf("walk = %d: %s", wi.Code, wi.Body.String())
		}
		bi := decodeBody(t, wi)
		seen += len(bi["items"].([]any))
		cursor, _ = bi["next_cursor"].(string)
	}
	if seen != 4 {
		t.Errorf("walked %d entries, want 4 across two revisions", seen)
	}
	// Tampered cursor fails closed.
	w3 := get(t, r, "/v1/stations/"+station+"/official-prices?limit=1&cursor="+url.QueryEscape(next+"x"), nil)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("tampered = %d", w3.Code)
	}
	// Revision scope isolates each revision deterministically.
	w4 := get(t, r, "/v1/stations/"+station+"/official-prices?limit=10", nil)
	b4 := decodeBody(t, w4)
	byRev := map[string][]float64{}
	for _, it := range b4["items"].([]any) {
		m := it.(map[string]any)
		rev := m["revision_id"].(string)
		byRev[rev] = append(byRev[rev], m["amount_milli_brl"].(float64))
	}
	if len(byRev) != 2 {
		t.Fatalf("revisions = %d, want 2", len(byRev))
	}
	var oldRev string
	for rev, amounts := range byRev {
		for _, a := range amounts {
			if a == 5999 {
				oldRev = rev
			}
		}
	}
	if oldRev == "" {
		t.Fatal("superseded 5999 value missing from history")
	}
	w5 := get(t, r, "/v1/stations/"+station+"/official-prices?revision_id="+oldRev+"&limit=10", nil)
	b5 := decodeBody(t, w5)
	for _, it := range b5["items"].([]any) {
		if it.(map[string]any)["revision_id"] != oldRev {
			t.Errorf("revision scope leaked: %v", it)
		}
	}
	if !strings.Contains(w5.Body.String(), `"amount_milli_brl":5999`) {
		t.Errorf("old value missing: %s", w5.Body.String())
	}
}
