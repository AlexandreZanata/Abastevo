package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

type fakePrices struct{}

func (fakePrices) Groups(_ context.Context, stationID, fuel string) ([]application.PriceGroup, error) {
	return []application.PriceGroup{{
		StationID: stationID, Product: fuel, Unit: "L",
		Condition: application.Condition{Kind: "STANDARD"},
		Official: &application.OfficialSection{
			Source: "ANP", AmountMilli: 6030, Currency: "BRL",
			CollectedOn: "2026-09-24", SurveyStart: "2026-09-20",
			SurveyEnd: "2026-09-26", RevisionID: "rev-1",
			SourceURL: "https://example.invalid/anp", SourceSha: "aa",
			RawText: "6,030",
		},
	}}, nil
}

func (fakePrices) History(_ context.Context, f application.HistoryFilter) ([]application.HistoryEntry, string, error) {
	return []application.HistoryEntry{{
		RevisionID: "rev-1", WeekStart: "2026-09-20", WeekEnd: "2026-09-26",
		CollectedOn: "2026-09-24", Product: "GASOLINE_REGULAR", Unit: "L",
		AmountMilli: 6030, Currency: "BRL", SourceURL: "https://example.invalid/anp",
		SourceSha: "aa", RawText: "6,030",
	}}, "", nil
}

func serveOfficial(target string, headers map[string]string) *httptest.ResponseRecorder {
	h := Handler{
		Prices:        fakePrices{},
		StationsExist: func(context.Context, string) (bool, error) { return true, nil },
		Community:     nil,
		Secrets:       []byte("test-cursor-secret-32-bytes-xxxx"),
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const priceStation = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

// TestPriceReadsAreSharedCacheable pins the price edge allowlist:
// bounded public TTLs with ETags and 304 support on both group and
// history reads, including the source-separated community section.
func TestPriceReadsAreSharedCacheable(t *testing.T) {
	for _, target := range []string{
		"/v1/stations/" + priceStation + "/prices?fuel_product=GASOLINE_REGULAR",
		"/v1/stations/" + priceStation + "/official-prices?fuel_product=GASOLINE_REGULAR&limit=1",
	} {
		w := serveOfficial(target, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", target, w.Code, w.Body.String())
		}
		if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
			t.Errorf("%s cache = %q", target, cc)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s misses ETag", target)
		}
		cached := serveOfficial(target, map[string]string{"If-None-Match": etag})
		if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
			t.Errorf("%s etag replay = %d bytes %d, want bodiless 304", target, cached.Code, cached.Body.Len())
		}
	}
}

// TestPriceReadsIgnoreIdentity proves shared caching is safe: identity
// headers change neither body nor cacheability on price reads.
func TestPriceReadsIgnoreIdentity(t *testing.T) {
	identity := map[string]string{
		"Authorization": "Signature sig1=:fictitious:",
		"Cookie":        "session=fictitious",
	}
	target := "/v1/stations/" + priceStation + "/prices?fuel_product=GASOLINE_REGULAR"
	plain := serveOfficial(target, nil)
	withAuth := serveOfficial(target, identity)
	if plain.Code != withAuth.Code || plain.Body.String() != withAuth.Body.String() {
		t.Error("price groups vary with identity headers")
	}
	if vary := withAuth.Header().Get("Vary"); strings.Contains(vary, "Authorization") || strings.Contains(vary, "Cookie") {
		t.Errorf("price groups vary on identity: %q", vary)
	}
}
