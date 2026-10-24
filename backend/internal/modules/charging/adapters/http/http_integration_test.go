//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal("integration database unavailable", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("charging_read_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err == nil {
			admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
			admin.Close(ctx)
		}
	})
	u, _ := url.Parse(adminDSN)
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatal("migrate", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedBatch(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	q := charging.New(pool)
	batchID := "aaaaaaaa-1111-4111-8111-000000000001"
	if _, err := pool.Exec(ctx, `INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count) VALUES($1,'charging-prepared-v1','ocm-br-20261021-v1','rev1',repeat('a',64),2,2)`, batchID); err != nil {
		t.Fatal(err)
	}
	places := []struct{ id, key, ext, name string }{
		{"bbbbbbbb-1111-4111-8111-000000000001", "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899", "1", "Posto A"},
		{"bbbbbbbb-1111-4111-8111-000000000002", "bbccddeeff00112233445566778899aabbccddeeff00112233445566778899aa", "2", "Posto B"},
	}
	groupKeys := map[string]string{
		"bbbbbbbb-1111-4111-8111-000000000001": "ccddeeff00112233445566778899aabbccddeeff00112233445566778899ccdd",
		"bbbbbbbb-1111-4111-8111-000000000002": "ddeeff00112233445566778899aabbccddeeff00112233445566778899aabbcc",
	}
	for _, p := range places {
		if _, err := pool.Exec(ctx, `INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points) VALUES($1,$2,$3,$4,$5,'BR','-23.5','-46.6','PUBLIC',2)`, p.id, batchID, p.key, p.ext, p.name); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO charging_connection_groups(id,batch_id,place_id,entity_key,external_id,connector_code,raw_connector_code,current,power_w,reported_quantity) VALUES(md5($1)::uuid,$2,(SELECT id FROM charging_places WHERE entity_key=$3),$4,$5,'TYPE2','25','AC',5000,1)`, p.id, batchID, p.key, groupKeys[p.id], p.ext); err != nil {
			t.Fatal(err)
		}
	}
	_ = q
}

func TestChargingReadsTraversalCoverageAndOpenAPI(t *testing.T) {
	pool := testPool(t)
	seedBatch(t, pool)
	router := chi.NewRouter()
	Handler{Read: read.NewReader(pool), Secrets: []byte("test-secret-32-bytes-long-xxxx"), Enabled: func() bool { return true }}.RegisterRoutes(router)
	_, file, _, _ := runtime.Caller(0)
	spec, err := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	code, page := get("/v1/charging/places?profile=ocm-br-20261021-v1&limit=100")
	if code != 200 {
		t.Fatal("places status", code, page)
	}
	if err := spec.Components.Schemas["ChargingPlacePage"].Value.VisitJSON(page); err != nil {
		t.Fatal("page outside contract", err)
	}
	items := page["items"].([]any)
	if len(items) != 2 {
		t.Fatal("places lost", len(items))
	}
	for _, item := range items {
		if err := spec.Components.Schemas["ChargingPlace"].Value.VisitJSON(item); err != nil {
			t.Fatal("place outside contract", err)
		}
	}
	id := items[0].(map[string]any)["id"].(string)
	code, detail := get("/v1/charging/places/" + id)
	if code != 200 {
		t.Fatal("detail status", code)
	}
	if groups, ok := detail["groups"].([]any); !ok || len(groups) != 1 {
		t.Fatal("groups lost", detail)
	}
	code, coverage := get("/v1/charging/coverage?profile=ocm-br-20261021-v1")
	if code != 200 {
		t.Fatal("coverage status", code)
	}
	if err := spec.Components.Schemas["ChargingCoverage"].Value.VisitJSON(coverage); err != nil {
		t.Fatal("coverage outside contract", err)
	}
	if coverage["places"] != float64(2) || coverage["groups"] != float64(2) {
		t.Fatal("coverage miscounted", coverage)
	}
	// Unknown profile refuses; bad limit refuses; unknown param refuses.
	for _, path := range []string{
		"/v1/charging/places?profile=bogus-v1",
		"/v1/charging/places?profile=ocm-br-20261021-v1&limit=101",
		"/v1/charging/places?profile=ocm-br-20261021-v1&evil=1",
		"/v1/charging/coverage",
	} {
		if code, _ := get(path); code != 400 {
			t.Fatal("malformed request accepted", path, code)
		}
	}
	// Traversal: cursor through all places with limit 1, no omissions.
	seen := map[string]bool{}
	cursor := ""
	for {
		path := "/v1/charging/places?profile=ocm-br-20261021-v1&limit=1"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		code, page := get(path)
		if code != 200 {
			t.Fatal("traversal status", code)
		}
		items := page["items"].([]any)
		if len(items) != 1 {
			t.Fatal("traversal page size", len(items))
		}
		id := items[0].(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatal("traversal duplicated", id)
		}
		seen[id] = true
		next, _ := page["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 2 {
		t.Fatal("traversal omitted", len(seen))
	}
}
