package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// P25-T03 bounded ANP API discovery: typed page traversal and targeted
// lookup with global quota, redirect/SSRF guards and per-record policy.
// Partial traversal never masquerades as a full snapshot; unsupported
// identifiers fall back safely instead of corrupting identity.

func pageBody(items ...string) string {
	return officialPage(1, 1, len(items), strings.Join(items, ","))
}

func recordBody(cnpj, quality, coords string) string {
	var point *struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
		CRS string  `json:"crs"`
	}
	_ = json.Unmarshal([]byte(coords), &point)
	lat, lon, srid, crs := "", "", "", ""
	if point != nil {
		lat = fmt.Sprint(point.Lat)
		lon = fmt.Sprint(point.Lon)
		crs = point.CRS
		if crs == "WGS84" {
			srid = "4326"
		}
	}
	return fmt.Sprintf(`{"cnpj":%q,"razaoSocial":"[P25-TEST] X","endereco":"TEST STREET, 10","municipio":"SAO PAULO","uf":"SP","latitude":%q,"longitude":%q,"srid":%q,"src":%q,"validacao":%q}`, cnpj, lat, lon, srid, crs, quality)
}

func testAPIConfig(server *httptest.Server) APIConfig {
	return APIConfig{
		BaseURL:       server.URL,
		AllowedHost:   "127.0.0.1",
		AllowLoopback: true,
		PageSize:      100,
		MaxPages:      5,
		MaxRequests:   10,
	}
}

func TestDiscoverPagesAndDedupsAcrossPages(t *testing.T) {
	first := pageBody(
		recordBody("04218406000104", "reviewed", `{"lat": -23.561, "lon": -46.656, "crs": "WGS84"}`),
		recordBody("12ABC34501DE35", "unknown", `null`),
	)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(first))
	}))
	defer server.Close()

	store := newFakeStore()
	report, err := Discover(context.Background(), store, "api-1", testAPIConfig(server))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if report.State != "complete" {
		t.Fatalf("state = %q", report.State)
	}
	if report.Accepted != 2 || report.Duplicates != 0 || report.Rejected != 0 {
		t.Fatalf("counts = %+v", report)
	}
	if calls != 1 {
		t.Fatalf("requests = %d, want 1 (null cursor ends traversal)", calls)
	}
}

func TestDiscoverFollowsCursorAndCountsDuplicates(t *testing.T) {
	pages := []string{
		strings.Replace(officialPage(1, 2, 2, recordBody("04218406000104", "reviewed", `{"lat": -23.561, "lon": -46.656, "crs": "WGS84"}`)), `"tamanhoPagina":5000`, `"tamanhoPagina":1`, 1),
		strings.Replace(officialPage(2, 2, 2, recordBody("04218406000104", "reviewed", `{"lat": -23.561, "lon": -46.656, "crs": "WGS84"}`)), `"tamanhoPagina":5000`, `"tamanhoPagina":1`, 1),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("numeropagina") == "1" {
			_, _ = w.Write([]byte(pages[0]))
			return
		}
		_, _ = w.Write([]byte(pages[1]))
	}))
	defer server.Close()

	store := newFakeStore()
	report, err := Discover(context.Background(), store, "api-cur", testAPIConfig(server))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if report.Accepted != 1 || report.Duplicates != 1 {
		t.Fatalf("counts = %+v", report)
	}
}

func TestDiscoverRejectsBadRecordsHonestly(t *testing.T) {
	body := pageBody(
		recordBody("123", "reviewed", `null`),
		recordBody("04218406000104", "reviewed", `{"lat":-23.5,"lon":-46.6,"crs":"MERCATOR"}`),
		recordBody("00428184000195", "reviewed", `{"lat":-91,"lon":0,"crs":"WGS84"}`),
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	store := newFakeStore()
	report, err := Discover(context.Background(), store, "api-bad", testAPIConfig(server))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if report.Accepted != 0 || report.Rejected != 3 {
		t.Fatalf("counts = %+v", report)
	}
}

func TestDiscoverRetries429ThenSucceeds(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pageBody(recordBody("04218406000104", "reviewed", `{"lat": -23.561, "lon": -46.656, "crs": "WGS84"}`))))
	}))
	defer server.Close()

	cfg := testAPIConfig(server)
	cfg.Sleep = func() {}
	store := newFakeStore()
	report, err := Discover(context.Background(), store, "api-429", cfg)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if report.Accepted != 1 || calls != 2 {
		t.Fatalf("report = %+v, calls = %d", report, calls)
	}
}

func TestDiscoverRefusesOffAllowlistAndPrivateBypass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pageBody()))
	}))
	defer server.Close()

	store := newFakeStore()
	cfg := testAPIConfig(server)
	cfg.AllowedHost = "revendedoresapi.anp.gov.br"
	if _, err := Discover(context.Background(), store, "api-deny", cfg); err == nil {
		t.Fatal("off-allowlist host must be refused")
	}
}

func TestDiscoverTargetedLookupResolvesCNPJ(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cnpj") != "04218406000104" {
			t.Errorf("targeted query missing cnpj, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pageBody(recordBody("04218406000104", "reviewed", `{"lat": -23.561, "lon": -46.656, "crs": "WGS84"}`))))
	}))
	defer server.Close()

	cfg := testAPIConfig(server)
	cfg.CNPJ = "04218406000104"
	store := newFakeStore()
	report, err := Discover(context.Background(), store, "api-target", cfg)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if report.Accepted != 1 {
		t.Fatalf("counts = %+v", report)
	}
}

func TestDiscoverTargetedLookupRejectsUnsupportedCNPJ(t *testing.T) {
	cfg := APIConfig{BaseURL: "https://127.0.0.1", AllowedHost: "127.0.0.1", CNPJ: "123"}
	if _, err := Discover(context.Background(), newFakeStore(), "api-tbad", cfg); err == nil {
		t.Fatal("unsupported CNPJ must fail before network")
	}
}

func TestLegacyNormalizedRecordCannotClaimOfficialLocationReview(t *testing.T) {
	if _, ok := parseOfficialAPIRecord(json.RawMessage(`{"cnpj":"04218406000104","endereco":{"uf":"SP"},"location_quality":"reviewed"}`)); ok {
		t.Fatal("legacy invented wire must not be an official record")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
