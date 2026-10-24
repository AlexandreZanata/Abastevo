//go:build integration && ee27

package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/read"
	"github.com/go-chi/chi/v5"
)

// EE-27 real admitted-source chain: Rust prepared batch -> verified Go
// manifest -> PostGIS staging -> public HTTP page traversal. It never
// substitutes synthetic rows: the batch directory must be the actual
// admitted OCM output selected by the caller.
func TestEE27RustToPublicHTTPChain(t *testing.T) {
	dir := os.Getenv("ANPFUEL_TEST_CHARGING_DIR")
	if dir == "" {
		t.Fatal("ANPFUEL_TEST_CHARGING_DIR is required for EE-27 acceptance")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := prepared.VerifyManifest(manifestRaw, "charging-prepared-v1", "ocm-br-20261021-v1")
	if err != nil {
		t.Fatal("real manifest refused", err)
	}
	entities, err := os.ReadFile(filepath.Join(dir, "entities.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := os.ReadFile(filepath.Join(dir, "decisions.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	batch, err := prepared.LoadBatch(context.Background(), manifest, entities, decisions, "ocm")
	if err != nil {
		t.Fatal("real bridge refused", err)
	}
	var input, accepted int64
	for _, counts := range manifest.Counts {
		input += counts.Input
		accepted += counts.Accepted
	}
	sum := sha256.Sum256(manifestRaw)
	manifestSHA := hex.EncodeToString(sum[:])

	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = prepared.StoreBatch(ctx, charging.New(tx), batch, manifestSHA, int(input), int(accepted)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("publication refused", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	Handler{Read: read.NewReader(pool), Secrets: []byte("ee27-acceptance-secret-32-bytes-ok"), Enabled: func() bool { return true }}.RegisterRoutes(router)
	get := func(path string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("GET %s -> %d %s", path, w.Code, w.Body.String())
		}
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatalf("invalid JSON for %s", path)
		}
		return body
	}

	places := 0
	cursor := ""
	firstPage := []byte(nil)
	for {
		path := "/v1/charging/places?profile=ocm-br-20261021-v1&limit=100"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("page -> %d %s", w.Code, w.Body.String())
		}
		if firstPage == nil {
			firstPage = append([]byte(nil), w.Body.Bytes()...)
		}
		var page struct {
			Items      []map[string]any `json:"items"`
			NextCursor *string          `json:"next_cursor"`
		}
		if json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatal("page JSON invalid")
		}
		for _, item := range page.Items {
			for _, forbidden := range []string{"email", "telephone", "phone", "comment", "media", "private"} {
				if _, exists := item[forbidden]; exists {
					t.Fatalf("PII field leaked: %s", forbidden)
				}
			}
			places++
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}
		cursor = *page.NextCursor
	}
	if places != 1298 {
		t.Fatalf("place traversal lost rows: %d", places)
	}
	var groups, vehicles int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM charging_connection_groups`).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM charging_vehicle_references`).Scan(&vehicles); err != nil {
		t.Fatal(err)
	}
	if groups != 2089 || vehicles != 0 {
		t.Fatalf("publication census diverged: groups=%d vehicles=%d", groups, vehicles)
	}
	coverage := get("/v1/charging/coverage?profile=ocm-br-20261021-v1")
	if coverage["places"].(float64) != 1298 || coverage["groups"].(float64) != 2089 {
		t.Fatalf("coverage diverged: %v", coverage)
	}
	if first := firstPage; len(first) > 0 {
		if out := os.Getenv("ANPFUEL_TEST_EE27_PAGE"); out != "" {
			if err := os.WriteFile(out, first, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
