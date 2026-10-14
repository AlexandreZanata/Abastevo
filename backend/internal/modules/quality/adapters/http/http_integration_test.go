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
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	parentadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

var testSecrets = []byte("test-cursor-secret-32-bytes-xxxx")

const httpStation = "eeeeeeee-5555-4555-8555-000000000001"

func freshRouter(t *testing.T, enabled bool) (chi.Router, *pgxpool.Pool) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("quality_http_test_%d", time.Now().UnixNano())
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
		t.Fatal(err)
	}
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx,
		"INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto HTTP')", httpStation); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	n := 0
	next := func() (string, error) {
		n++
		return fmt.Sprintf("eeeeeeee-6666-4666-8666-%012d", n), nil
	}
	if _, err := parentadapters.NewPublisher(pool).Publish(ctx, application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pmqc_2026_09.zip",
			SHA256: strings.Repeat("a", 64), Bytes: 10,
		}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat("b", 64),
		Assays: []application.AssayInput{
			{SampleRef: "S-1", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "92,6", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#ASPECTO", Fingerprint: strings.Repeat("c", 64)},
			{SampleRef: "S-1", AssayRef: "DESTILACAO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "181,5", Unit: "°C", Conformity: "CONFORMING", EvidenceRef: "f#S-1#DESTILACAO", Fingerprint: strings.Repeat("d", 64)},
		},
		Links: []application.LinkInput{
			{RecordKey: "pmqc-assay-v1:S-1", StationID: httpStation, Automatic: true, Reason: "exact cnpj", Active: true},
		},
	}, next); err != nil {
		t.Fatalf("seed assays: %v", err)
	}
	if _, err := parentadapters.NewPublisher(pool).PublishEvents(ctx, application.EventsInput{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL: "https://www.gov.br/anp/acoes", SHA256: strings.Repeat("b", 64), Bytes: 9,
		}},
		Counts:       domain.Counts{Input: 1, Accepted: 1},
		ManifestHash: strings.Repeat("f", 64),
		Enforcement: []application.EnforcementInput{
			{NativeKey: "AUTO-1", Scope: "municipal", Status: "autuado", SubjectCNPJ: "11222333000181", OccurredOn: "2026-08-02", AsOf: "2026-09-30", EvidenceRef: "e#AUTO-1"},
		},
	}, next); err != nil {
		t.Fatalf("seed enforcement: %v", err)
	}

	stations := directoryread.NewReader(pool)
	router := chi.NewRouter()
	Handler{
		Read:    qualityread.NewReader(pool),
		Secrets: testSecrets,
		StationsExist: func(ctx context.Context, id string) (bool, error) {
			_, err := stations.Detail(ctx, id)
			if err == nil {
				return true, nil
			}
			return false, nil
		},
		Enabled: func() bool { return enabled },
	}.RegisterRoutes(router)
	return router, pool
}

func get(t *testing.T, router chi.Router, path string) (int, map[string]any, http.Header) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s: invalid JSON %q: %v", path, rec.Body.String(), err)
	}
	return rec.Code, body, rec.Header()
}

func TestSummaryAnonymous(t *testing.T) {
	router, _ := freshRouter(t, true)
	code, body, _ := get(t, router, "/v1/quality/summary?station_id="+httpStation)
	if code != http.StatusOK {
		t.Fatalf("summary = %d %v", code, body)
	}
	cells, _ := body["fuel_cells"].([]any)
	if len(cells) != 1 {
		t.Fatalf("cells = %v", cells)
	}
	lease, _ := body["lease"].(map[string]any)
	if lease["edition"] != "2026-09" || lease["available"] != true || lease["stale"] != false {
		t.Errorf("lease = %v", lease)
	}
	if !strings.Contains(lease["coverage_note"].(string), "2026-09") {
		t.Errorf("coverage note = %v", lease["coverage_note"])
	}
	if strings.Contains(strings.ToLower(recString(body)), "cnpj") {
		t.Errorf("source identifier leaked: %v", body)
	}
}

func recString(body map[string]any) string {
	raw, _ := json.Marshal(body)
	return string(raw)
}

func TestReadCodes(t *testing.T) {
	router, _ := freshRouter(t, true)
	if code, _, _ := get(t, router, "/v1/quality/summary?station_id=ffffffff-6666-4666-8666-000000000002"); code != http.StatusNotFound {
		t.Errorf("unknown station = %d, want 404", code)
	}
	if code, _, _ := get(t, router, "/v1/quality/summary?station_id=not-a-uuid"); code != http.StatusBadRequest {
		t.Errorf("bad id = %d, want 400", code)
	}
	if code, _, _ := get(t, router, "/v1/quality/summary?station_id="+httpStation+"&fuel=HYDROGEN"); code != http.StatusBadRequest {
		t.Errorf("unknown fuel = %d, want 400", code)
	}
	if code, _, _ := get(t, router, "/v1/quality/assays?station_id="+httpStation+"&limit=500"); code != http.StatusBadRequest {
		t.Errorf("bad limit = %d, want 400", code)
	}
	if code, _, _ := get(t, router, "/v1/quality/assays?station_id="+httpStation+"&cursor=tampered"); code != http.StatusBadRequest {
		t.Errorf("tampered cursor = %d, want 400", code)
	}
	if code, _, _ := get(t, router, "/v1/quality/sanctions/aaaaaaaa-1111-4111-8111-000000000099"); code != http.StatusNotFound {
		t.Errorf("unknown sanction = %d, want 404", code)
	}
}

func TestAssayTraversal(t *testing.T) {
	router, _ := freshRouter(t, true)
	code, page1, _ := get(t, router, "/v1/quality/assays?station_id="+httpStation+"&limit=1")
	if code != http.StatusOK {
		t.Fatalf("page1 = %d %v", code, page1)
	}
	items, _ := page1["items"].([]any)
	cursor, _ := page1["next_cursor"].(string)
	if len(items) != 1 || cursor == "" {
		t.Fatalf("page1 = %v", page1)
	}
	code, page2, _ := get(t, router, "/v1/quality/assays?station_id="+httpStation+"&limit=1&cursor="+url.QueryEscape(cursor))
	if code != http.StatusOK {
		t.Fatalf("page2 = %d %v", code, page2)
	}
	items2, _ := page2["items"].([]any)
	if len(items2) != 1 || page2["next_cursor"] != nil {
		t.Fatalf("page2 = %v", page2)
	}
	first, _ := items[0].(map[string]any)
	second, _ := items2[0].(map[string]any)
	if first["id"] == second["id"] {
		t.Error("traversal repeated the row")
	}
}

func TestSampleAndEvents(t *testing.T) {
	router, _ := freshRouter(t, true)
	code, sample, _ := get(t, router, "/v1/quality/samples/S-1")
	if code != http.StatusOK {
		t.Fatalf("sample = %d %v", code, sample)
	}
	if len(sample["assays"].([]any)) != 2 || len(sample["links"].([]any)) != 1 {
		t.Fatalf("sample = %v", sample)
	}
	code, missing, _ := get(t, router, "/v1/quality/samples/S-9")
	if code != http.StatusOK || len(missing["assays"].([]any)) != 0 {
		t.Fatalf("unknown sample = %d %v, want empty 200", code, missing)
	}
	code, actions, _ := get(t, router, "/v1/quality/actions?limit=10")
	if code != http.StatusOK || len(actions["items"].([]any)) != 1 {
		t.Fatalf("actions = %d %v", code, actions)
	}
	first, _ := actions["items"].([]any)[0].(map[string]any)
	code, one, _ := get(t, router, "/v1/quality/actions/"+first["id"].(string))
	if code != http.StatusOK {
		t.Fatalf("action detail = %d %v", code, one)
	}
	if one["native_key"] != "AUTO-1" {
		t.Errorf("action = %v", one)
	}
}

func TestReadsOff(t *testing.T) {
	router, _ := freshRouter(t, false)
	for _, path := range []string{
		"/v1/quality/summary?station_id=" + httpStation,
		"/v1/quality/assays?station_id=" + httpStation,
		"/v1/quality/samples/S-1",
		"/v1/quality/actions",
		"/v1/quality/sanctions",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("disabled %s = %d, want 404", path, rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("disabled %s: invalid JSON: %v", path, err)
		}
		envelope, _ := body["error"].(map[string]any)
		if envelope["code"] != "quality.disabled" {
			t.Errorf("disabled %s code = %v", path, body)
		}
	}
}

func TestCursorBoundToRevision(t *testing.T) {
	router, pool := freshRouter(t, true)
	code, page1, _ := get(t, router, "/v1/quality/assays?station_id="+httpStation+"&limit=1")
	if code != http.StatusOK {
		t.Fatalf("page1 = %d", code)
	}
	cursor, _ := page1["next_cursor"].(string)
	// Concurrent correction publishes a new revision: the old cursor is
	// bound to the previous lease and must fail closed, never wander.
	n := 100
	next := func() (string, error) {
		n++
		return fmt.Sprintf("eeeeeeee-7777-4777-8777-%012d", n), nil
	}
	if _, err := parentadapters.NewPublisher(pool).Publish(context.Background(), application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pmqc_2026_09.zip",
			SHA256: strings.Repeat("a", 64), Bytes: 10,
		}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat("e", 64),
		Assays: []application.AssayInput{
			{SampleRef: "S-1", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "92,7", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#ASPECTO", Fingerprint: strings.Repeat("c", 64)},
			{SampleRef: "S-1", AssayRef: "DESTILACAO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "181,5", Unit: "°C", Conformity: "CONFORMING", EvidenceRef: "f#S-1#DESTILACAO", Fingerprint: strings.Repeat("d", 64)},
		},
		Links: []application.LinkInput{
			{RecordKey: "pmqc-assay-v1:S-1", StationID: httpStation, Automatic: true, Reason: "exact cnpj", Active: true},
		},
	}, next); err != nil {
		t.Fatalf("correction publish: %v", err)
	}
	code, _, _ = get(t, router, "/v1/quality/assays?station_id="+httpStation+"&limit=1&cursor="+url.QueryEscape(cursor))
	if code != http.StatusBadRequest {
		t.Errorf("cross-revision cursor = %d, want 400", code)
	}
}
