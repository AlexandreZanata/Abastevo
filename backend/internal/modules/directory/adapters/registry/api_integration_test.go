//go:build integration

package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func loopbackConfig(server *httptest.Server) APIConfig {
	return APIConfig{
		BaseURL:       server.URL,
		AllowedHost:   "127.0.0.1",
		AllowLoopback: true,
		PageSize:      100,
		MaxPages:      5,
		MaxRequests:   10,
		MaxBytes:      1 << 20,
	}
}

func TestDiscoverIntegrationStagesAndReplays(t *testing.T) {
	pages := map[string]string{
		"":   `{"items": [{"cnpj": "04218406000104", "razaoSocial": "[P25-TEST] ALFA", "endereco": {"municipio": "SAO PAULO", "uf": "SP", "codigoIbge": "3550308"}, "situacao": "ATIVA", "location_quality": "reviewed", "coordenadas": {"lat": -23.561, "lon": -46.656, "crs": "WGS84"}}], "nextCursor": "p2"}`,
		"p2": `{"items": [{"cnpj": "12ABC34501DE35", "razaoSocial": "[P25-TEST] GAMA", "endereco": {"municipio": "SAO PAULO", "uf": "SP", "codigoIbge": "3550308"}, "situacao": "DESCONHECIDA", "location_quality": "unknown", "coordenadas": null}], "nextCursor": null}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pages[r.URL.Query().Get("cursor")]))
	}))
	defer server.Close()

	store := freshStore(t)
	ctx := context.Background()
	first, err := Discover(ctx, store, "api-int-1", loopbackConfig(server))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if first.State != "complete" || first.Accepted != 2 {
		t.Fatalf("report = %+v", first)
	}
	second, err := Discover(ctx, store, "api-int-1", loopbackConfig(server))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.RunID != first.RunID {
		t.Fatalf("replay diverged: %+v vs %+v", first, second)
	}
	if got := countAssertions(t, store, first.RunID); got != 2 {
		t.Fatalf("assertions = %d, want 2", got)
	}
	n, err := store.Q.CountCompleteRegistryRuns(ctx, SourceAPI)
	if err != nil {
		t.Fatalf("count complete: %v", err)
	}
	if n != 1 {
		t.Fatalf("complete runs = %d, want 1", n)
	}
}

func TestDiscoverIntegrationTargeted404IsHonestEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "cnpj=") {
			t.Errorf("targeted query missing cnpj: %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	store := freshStore(t)
	cfg := loopbackConfig(server)
	cfg.CNPJ = "04218406000104"
	report, err := Discover(context.Background(), store, "api-int-404", cfg)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if report.State != "complete" || report.Accepted != 0 {
		t.Fatalf("report = %+v", report)
	}
}
