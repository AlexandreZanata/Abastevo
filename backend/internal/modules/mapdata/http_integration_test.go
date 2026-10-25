//go:build integration

package mapdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerProjectsBoundedAssertionsAndRejectsStaleCursor(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	assertions := fixture(t)
	generation, err := store.Publish(context.Background(), envelopeFor(t, assertions), assertions)
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{Store: store, Secrets: []byte("map-handler-test-secret")}
	target := "/v1/map/assertions?min_lat=-16&min_lon=-48.1&max_lat=-15&max_lon=-47.7&source=ANP&product=GASOLINE_REGULAR&limit=1"
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.assertions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Generation struct {
			ID string `json:"id"`
		} `json:"generation"`
		Items []struct {
			AssertionID string `json:"assertion_id"`
			Prices      []struct {
				AmountMilliBRL int64 `json:"amount_milli_brl"`
			} `json:"prices"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Generation.ID != generation.ID || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("unexpected page: %+v", page)
	}
	if page.Items[0].AssertionID != "11111111-1111-4111-8111-111111111111" ||
		len(page.Items[0].Prices) != 1 || page.Items[0].Prices[0].AmountMilliBRL != 5890 {
		t.Fatalf("missing exact ANP price: %+v", page.Items[0])
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/map/assertions?min_lat=-16&min_lon=-48.1&max_lat=-15&max_lon=-47.7&source=ANP&limit=10", nil)
	rec = httptest.NewRecorder()
	h.assertions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("projected list status %d body %s", rec.Code, rec.Body.String())
	}
	var projected struct {
		Items []struct {
			AssertionID  string `json:"assertion_id"`
			Prices       []any  `json:"prices"`
			Capabilities struct {
				PriceKnown bool `json:"price_known"`
			} `json:"capabilities"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &projected); err != nil {
		t.Fatal(err)
	}
	for _, item := range projected.Items {
		if item.AssertionID == "22222222-2222-4222-8222-222222222222" {
			if len(item.Prices) != 0 || item.Capabilities.PriceKnown {
				t.Fatalf("community projection leaked ANP price flag: %+v", item)
			}
		}
	}
	// A cursor minted for the old generation must fail after a new activation.
	oldCursor := *page.NextCursor
	var doc map[string]any
	_ = json.Unmarshal(assertions, &doc)
	doc["generation"] = "overlay-handler-next"
	nextAssertions, _ := json.Marshal(doc)
	if _, err := store.Publish(context.Background(), envelopeFor(t, nextAssertions), nextAssertions); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, target+"&cursor="+oldCursor, nil)
	rec = httptest.NewRecorder()
	h.assertions(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("stale cursor status %d body %s", rec.Code, rec.Body.String())
	}
}
