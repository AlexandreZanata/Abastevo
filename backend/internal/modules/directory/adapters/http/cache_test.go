package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

type fakeStations struct{}

func (fakeStations) Search(context.Context, application.SearchFilter) ([]application.Station, string, error) {
	return []application.Station{{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", DisplayName: "Alfa",
		LocationQuality: "reviewed",
	}}, "", nil
}

func (fakeStations) Nearby(context.Context, application.NearbyFilter) ([]application.NearbyStation, string, error) {
	return []application.NearbyStation{{
		Station:   application.Station{ID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", DisplayName: "Alfa", LocationQuality: "reviewed"},
		DistanceM: 120,
	}}, "", nil
}

func (fakeStations) Detail(context.Context, string) (application.Station, error) {
	return application.Station{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", DisplayName: "Alfa",
		LocationQuality: "reviewed",
	}, nil
}

func serveDirectory(method, target string, headers map[string]string) *httptest.ResponseRecorder {
	h := Handler{Stations: fakeStations{}, Secrets: []byte("test-secrets-32-bytes-long-value!")}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestPublicReadsAreSharedCacheable pins the edge allowlist: search
// and detail carry public bounded TTLs with ETags and 304 support,
// while nearby stays no-store (request position must never be cached
// or echoed by shared infrastructure).
func TestPublicReadsAreSharedCacheable(t *testing.T) {
	w := serveDirectory(http.MethodGet, "/v1/stations?q=alfa&limit=5", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("search = %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("search cache = %q", cc)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("search misses ETag")
	}

	d := serveDirectory(http.MethodGet, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26", nil)
	if d.Code != http.StatusOK {
		t.Fatalf("detail = %d", d.Code)
	}
	if cc := d.Header().Get("Cache-Control"); cc != "public, max-age=30, s-maxage=60" {
		t.Errorf("detail cache = %q", cc)
	}
	etag := d.Header().Get("ETag")
	if etag == "" {
		t.Fatal("detail misses ETag")
	}
	cached := serveDirectory(http.MethodGet, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26",
		map[string]string{"If-None-Match": etag})
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
		t.Errorf("etag replay = %d bytes %d, want bodiless 304", cached.Code, cached.Body.Len())
	}

	n := serveDirectory(http.MethodGet, "/v1/stations/nearby?lat=-15.8&lon=-47.9&radius_m=3000&limit=5", nil)
	if n.Code != http.StatusOK {
		t.Fatalf("nearby = %d", n.Code)
	}
	if cc := n.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("nearby cache = %q, want no-store", cc)
	}
}

// TestPublicReadsIgnoreIdentity proves the edge may share cached
// public responses: Authorization and Cookie headers change neither
// body nor cacheability, because no public read consults them.
func TestPublicReadsIgnoreIdentity(t *testing.T) {
	identity := map[string]string{
		"Authorization": "Signature sig1=:fictitious:",
		"Cookie":        "session=fictitious",
	}
	for _, target := range []string{
		"/v1/stations?q=alfa&limit=5",
		"/v1/stations/by-cnpj/04218406000104",
		"/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26",
		"/v1/stations/nearby?lat=-15.8&lon=-47.9&radius_m=3000&limit=5",
	} {
		plain := serveDirectory(http.MethodGet, target, nil)
		withAuth := serveDirectory(http.MethodGet, target, identity)
		if plain.Code != withAuth.Code || plain.Body.String() != withAuth.Body.String() {
			t.Errorf("%s varies with identity headers", target)
		}
		if plain.Header().Get("Cache-Control") != withAuth.Header().Get("Cache-Control") {
			t.Errorf("%s cacheability varies with identity headers", target)
		}
		if vary := withAuth.Header().Get("Vary"); strings.Contains(vary, "Authorization") || strings.Contains(vary, "Cookie") {
			t.Errorf("%s varies on identity: %q", target, vary)
		}
	}
}

// TestPublicErrorsAreNotCached proves error envelopes never carry a
// shared-cache directive, even on otherwise cacheable routes.
func TestPublicErrorsAreNotCached(t *testing.T) {
	w := serveDirectory(http.MethodGet, "/v1/stations?q=a&limit=999", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad limit = %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("error cache = %q, want no-store", cc)
	}
}

func (fakeStations) ByCNPJ(_ context.Context, cnpj string) (application.Station, error) {
	if cnpj != "04218406000104" {
		return application.Station{}, application.ErrUnknownStation
	}
	station, _ := (fakeStations{}).Detail(context.Background(), "")
	station.CNPJNormalized = &cnpj
	return station, nil
}

func TestByCNPJReadUsesExactValidatedIdentifier(t *testing.T) {
	for _, tc := range []struct {
		value  string
		status int
	}{
		{"04218406000104", http.StatusOK},
		{"11222333000181", http.StatusNotFound},
		{"04218406000105", http.StatusBadRequest},
		{"123", http.StatusBadRequest},
	} {
		response := serveDirectory(http.MethodGet, "/v1/stations/by-cnpj/"+tc.value, nil)
		if response.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.value, response.Code, tc.status)
		}
		if tc.status != http.StatusOK && response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: negative identifier response must be no-store", tc.value)
		}
	}
}

func TestByCNPJConditionalRead(t *testing.T) {
	target := "/v1/stations/by-cnpj/04218406000104"
	response := serveDirectory(http.MethodGet, target, nil)
	etag := response.Header().Get("ETag")
	if etag == "" || response.Header().Get("Cache-Control") != "public, max-age=30, s-maxage=60" {
		t.Fatal("exact public read lacks bounded cache contract")
	}
	replay := serveDirectory(http.MethodGet, target, map[string]string{"If-None-Match": etag})
	if replay.Code != http.StatusNotModified || replay.Body.Len() != 0 {
		t.Fatal("conditional exact read must be bodiless 304")
	}
}
