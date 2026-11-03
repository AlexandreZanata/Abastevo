//go:build integration

package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/read"
	"github.com/go-chi/chi/v5"
)

func TestCatalogOfferServesOnlyPublishedRevision(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const batch = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	revision := strings.Repeat("1", 64)
	router := chi.NewRouter()
	Handler{Read: read.NewReader(pool)}.RegisterRoutes(router)
	request := func(file string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
			"/v1/charging/catalog/"+revision+"/offer/"+file, nil))
		return w
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	data := strings.Repeat("{\"name\":\"Synthetic\",\"id\":1}\n", 10000)
	notice := `{"license":"ODbL-1.0"}`
	hash := func(raw string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) }
	exec(`INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
		VALUES($1,'charging-prepared-v1','charging-br-national-v1',$2,$2,1,1)`, batch, revision)
	exec(`INSERT INTO charging_catalog_generations(batch_id,rights_sha256,offer_sha256,uf_counts)
		VALUES($1,repeat('a',64),$2,'{"SP":1}')`, batch, hash(data))
	for kind, raw := range map[string]string{"data": data, "notice": notice} {
		exec("INSERT INTO charging_catalog_offer_artifacts VALUES($1,$2,$3,$4)", batch, kind, hash(raw), len(raw))
		for position, offset := 0, 0; offset < len(raw); position, offset = position+1, offset+262144 {
			exec("INSERT INTO charging_catalog_offer_chunks VALUES($1,$2,$3,$4)", batch, kind, position, []byte(raw[offset:min(offset+262144, len(raw))]))
		}
	}
	if w := request(offerDataFile); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "charging.offer-not-found") {
		t.Fatalf("unpublished offer response = %d %q", w.Code, w.Body)
	}
	exec("INSERT INTO charging_catalog_activations(batch_id) VALUES($1)", batch)
	for filename, raw := range map[string]string{offerDataFile: data, offerNoticeFile: notice} {
		w := request(filename)
		if w.Code != http.StatusOK || w.Body.String() != raw || w.Header().Get("Content-Length") != strconv.Itoa(len(raw)) ||
			w.Header().Get("ETag") != "\""+hash(raw)+"\"" {
			t.Fatalf("offer %s returned code=%d bytes=%d headers=%v", filename, w.Code, w.Body.Len(), w.Header())
		}
	}
	exec("UPDATE charging_catalog_current SET batch_id=$1", batch)
	coverage := httptest.NewRecorder()
	router.ServeHTTP(coverage, httptest.NewRequest(http.MethodGet, "/v1/charging/coverage?profile=charging-br-national-v1", nil))
	var response struct {
		Offer map[string]string `json:"offer"`
	}
	base := "/v1/charging/catalog/" + revision + "/offer/"
	if err := json.Unmarshal(coverage.Body.Bytes(), &response); err != nil || coverage.Code != http.StatusOK ||
		response.Offer["notice"] != base+offerNoticeFile || response.Offer["data"] != base+offerDataFile {
		t.Fatalf("coverage did not advertise the published offer: %d %+v %v", coverage.Code, response, err)
	}
	cached := httptest.NewRequest(http.MethodGet, base+offerDataFile, nil)
	cached.Header.Set("If-None-Match", "\""+hash(data)+"\"")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, cached)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("conditional request returned %d and %d bytes", w.Code, w.Body.Len())
	}
	if w := request("owner-review-sample.json"); w.Code != http.StatusNotFound {
		t.Fatalf("private file request returned %d", w.Code)
	}
}
