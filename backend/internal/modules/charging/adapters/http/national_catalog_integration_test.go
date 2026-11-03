//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/read"
	"github.com/go-chi/chi/v5"
)

func TestNationalCatalogReadsActiveAliasesAndRejectsStaleCursor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const publicID = "11111111-1111-4111-8111-111111111111"
	const oldAlias = "22222222-2222-4222-8222-222222222222"
	const second = "33333333-3333-4333-8333-333333333333"
	const stationID = "44444444-4444-4444-8444-444444444444"
	const removedAlias = "55555555-5555-4555-8555-555555555555"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic Fuel')", stationID)
	seed := func(batch, revision, name string) {
		t.Helper()
		exec(`INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
			VALUES($1,'charging-prepared-v1','charging-br-national-v1',$2,$2,2,2)`, batch, revision)
		exec(`INSERT INTO charging_catalog_generations(batch_id,rights_sha256,offer_sha256,uf_counts)
			VALUES($1,repeat('a',64),repeat('b',64),'{"SP":2}')`, batch)
		for i, id := range []string{publicID, second} {
			var place string
			err := pool.QueryRow(ctx, `INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,
				country,latitude,longitude,public_access,reported_points,catalog_details)
				VALUES(gen_random_uuid(),$1,lpad($2,64,'0'),$2,$3,'BR','-23.5','-46.6','PUBLIC',0,
				'{"confidence":"single-source","uf":"SP","sources":[{"source":"OSM","external_id":"node:9","revision":"synthetic","license":"ODbL-1.0","attribution":"Synthetic attribution"}]}')
				RETURNING id::text`, batch, fmt.Sprint(i), name).Scan(&place)
			if err != nil {
				t.Fatal(err)
			}
			exec("INSERT INTO charging_catalog_place_aliases VALUES($1,$2,$3,TRUE)", batch, id, place)
			if i == 0 {
				exec("INSERT INTO charging_catalog_place_aliases VALUES($1,$2,$3,FALSE)", batch, oldAlias, place)
				if name == "Old" {
					exec("INSERT INTO charging_catalog_place_aliases VALUES($1,$2,$3,FALSE)", batch, removedAlias, place)
				}

				exec(`INSERT INTO charging_place_station_links(id,place_id,station_id,state,method,reviewer_ref)
					VALUES(gen_random_uuid(),$1,$2,'REVIEWED','MANUAL','test-owner')`, place, stationID)
			}
		}
		exec("UPDATE charging_catalog_current SET batch_id=$1", batch)
	}
	seed("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", strings.Repeat("1", 64), "Old")
	router := chi.NewRouter()
	Handler{Read: read.NewReader(pool), Secrets: []byte("synthetic-cursor-key")}.RegisterRoutes(router)
	get := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return w.Code, body
	}
	query := "/v1/charging/places?profile=charging-br-national-v1&limit=1"
	code, page := get(query)
	cursor, ok := page["next_cursor"].(string)
	if code != http.StatusOK || !ok || cursor == "" {
		t.Fatalf("page=%+v code=%d", page, code)
	}
	code, nextPage := get(query + "&cursor=" + url.QueryEscape(cursor))
	if code != http.StatusOK || nextPage["next_cursor"] != nil ||
		nextPage["items"].([]any)[0].(map[string]any)["id"] != second {
		t.Fatalf("active second page=%+v code=%d", nextPage, code)
	}
	code, detail := get("/v1/charging/places/" + oldAlias)
	if code != http.StatusOK || detail["id"] != publicID || detail["linked_station_id"] != stationID ||
		detail["confidence"] != "single-source" || detail["sources"] == nil {
		t.Fatalf("detail=%+v code=%d", detail, code)
	}
	seed("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", strings.Repeat("2", 64), "New")
	if code, _ := get(query + "&cursor=" + url.QueryEscape(cursor)); code != http.StatusBadRequest {
		t.Fatalf("stale cursor returned %d", code)
	}
	code, detail = get("/v1/charging/places/" + oldAlias)
	if code != http.StatusOK || detail["name"] != "New" {
		t.Fatalf("alias did not follow active generation: %+v code=%d", detail, code)
	}
	if code, _ := get("/v1/charging/places/" + removedAlias); code != http.StatusNotFound {
		t.Fatalf("inactive alias returned %d", code)
	}
	exec("UPDATE charging_catalog_current SET held=TRUE")
	if code, detail := get("/v1/charging/places/" + oldAlias); code != http.StatusOK || detail["name"] != "New" {
		t.Fatalf("publication hold hid the last good generation: %+v code=%d", detail, code)
	}

}
