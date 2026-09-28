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
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
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

func freshRouter(t *testing.T) (chi.Router, map[string]string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("http_test_%d", time.Now().UnixNano())
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
	repo := parent.NewRepository(pool)
	mk := func(cnpj, display, wkt string) string {
		st, err := repo.ResolveCNPJ(ctx, cnpj, display, map[string]string{"municipio": "3550308"})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if wkt != "" {
			rev, err := repo.RecordLocation(ctx, domain.LocationRevision{
				StationID: st.ID, PointWKT: wkt, Quality: domain.QualityReviewed, Provider: "review",
			})
			if err != nil {
				t.Fatalf("locate: %v", err)
			}
			if _, err := repo.ProjectLocation(ctx, st.ID, rev.ID); err != nil {
				t.Fatalf("project: %v", err)
			}
		}
		return st.ID
	}
	ids := map[string]string{
		"alfa": mk("04218406000104", "Posto Alfa", "POINT(-46.633 -23.550)"),
		"beta": mk("11222333000181", "Posto Beta", "POINT(-46.640 -23.555)"),
		"gama": mk("12ABC34501DE35", "Posto Gama", ""),
	}
	r := chi.NewRouter()
	Handler{Stations: directoryread.NewReader(pool), Secrets: testSecrets}.RegisterRoutes(r)
	return r, ids
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

func TestSearchEnvelope(t *testing.T) {
	r, _ := freshRouter(t)
	w := get(t, r, "/v1/stations?limit=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("cache = %q", cc)
	}
	body := decodeBody(t, w)
	items, _ := body["items"].([]any)
	if len(items) != 3 {
		t.Errorf("items = %d, want 3", len(items))
	}
	if _, ok := body["generated_at"]; !ok {
		t.Error("generated_at missing")
	}
	// Pagination walk through the sealed cursor.
	w2 := get(t, r, "/v1/stations?limit=1", nil)
	b2 := decodeBody(t, w2)
	next, _ := b2["next_cursor"].(string)
	if next == "" {
		t.Fatal("first page has no cursor")
	}
	w3 := get(t, r, "/v1/stations?limit=1&cursor="+url.QueryEscape(next), nil)
	if w3.Code != http.StatusOK {
		t.Fatalf("second page = %d: %s", w3.Code, w3.Body.String())
	}
	b3 := decodeBody(t, w3)
	if len(b3["items"].([]any)) != 1 {
		t.Errorf("second page items = %v", b3["items"])
	}
	// Tampered cursor fails closed with the envelope shape.
	w4 := get(t, r, "/v1/stations?limit=1&cursor="+url.QueryEscape(next+"tampered"), nil)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("tampered cursor = %d", w4.Code)
	}
	b4 := decodeBody(t, w4)
	errObj, _ := b4["error"].(map[string]any)
	if errObj["code"] != "station.bad-cursor" {
		t.Errorf("error = %v", b4)
	}
	// Invalid limit fails with field detail.
	w5 := get(t, r, "/v1/stations?limit=500", nil)
	if w5.Code != http.StatusBadRequest {
		t.Fatalf("bad limit = %d", w5.Code)
	}
	if cc := w5.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("error cache = %q", cc)
	}
}

func TestNearbyHonesty(t *testing.T) {
	r, _ := freshRouter(t)
	lat, lon := "-23.551", "-46.634"
	w := get(t, r, "/v1/stations/nearby?lat="+lat+"&lon="+lon+"&radius_m=3000&limit=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("nearby cache = %q, want no-store", cc)
	}
	raw := w.Body.String()
	if strings.Contains(raw, lat) || strings.Contains(raw, lon) {
		t.Errorf("request point echoed in body: %s", raw)
	}
	body := decodeBody(t, w)
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	for _, it := range items {
		m := it.(map[string]any)
		if _, ok := m["distance_m"]; !ok {
			t.Errorf("distance missing: %v", m)
		}
	}
	w2 := get(t, r, "/v1/stations/nearby?lat=91&lon=0&radius_m=3000", nil)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("out-of-bounds = %d", w2.Code)
	}
	w3 := get(t, r, "/v1/stations/nearby?lat=-23.55&lon=-46.63&radius_m=50", nil)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("small radius = %d", w3.Code)
	}
}

func TestDetailEnvelope(t *testing.T) {
	r, ids := freshRouter(t)
	w := get(t, r, "/v1/stations/"+ids["alfa"], nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag missing")
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=30, s-maxage=60" {
		t.Errorf("cache = %q", cc)
	}
	w2 := get(t, r, "/v1/stations/"+ids["alfa"], map[string]string{"If-None-Match": etag})
	if w2.Code != http.StatusNotModified || w2.Body.Len() != 0 {
		t.Errorf("conditional = %d bytes %d", w2.Code, w2.Body.Len())
	}
	body := decodeBody(t, w)
	if body["cnpj_normalized"] != "04218406000104" {
		t.Errorf("cnpj = %v", body["cnpj_normalized"])
	}
	if coords, _ := body["coordinates"].(map[string]any); coords["lat"] != -23.55 {
		t.Errorf("coordinates = %v", body["coordinates"])
	}
	// Missing location stays honest.
	w3 := get(t, r, "/v1/stations/"+ids["gama"], nil)
	b3 := decodeBody(t, w3)
	if b3["coordinates"] != nil || b3["location_quality"] != "unknown" {
		t.Errorf("missing location dishonest: %v", b3)
	}
	// Unknown and malformed ids.
	w4 := get(t, r, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26", nil)
	if w4.Code != http.StatusNotFound {
		t.Fatalf("unknown = %d", w4.Code)
	}
	b4 := decodeBody(t, w4)
	if b4["error"].(map[string]any)["code"] != "station.not-found" {
		t.Errorf("error = %v", b4)
	}
	w5 := get(t, r, "/v1/stations/not-a-uuid", nil)
	if w5.Code != http.StatusBadRequest {
		t.Fatalf("malformed = %d", w5.Code)
	}
}
