//go:build integration

package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		"1": strings.Replace(officialPage(1, 2, 2, recordBody("04218406000104", "unknown", `{"lat":-23.561,"lon":-46.656,"crs":"WGS84"}`)), `"tamanhoPagina":5000`, `"tamanhoPagina":1`, 1),
		"2": strings.Replace(officialPage(2, 2, 2, recordBody("12ABC34501DE35", "unknown", `null`)), `"tamanhoPagina":5000`, `"tamanhoPagina":1`, 1),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pages[r.URL.Query().Get("numeropagina")]))
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

func TestDiscoverIntegrationTargetedHTTP404IsProviderFailure(t *testing.T) {
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
	if report.State != "failed" || report.ErrorCode != "provider_error" || report.Accepted != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestOfficialANPIntegrationCandidatesReplayAndConcurrentIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, officialPage(1, 1, 1, officialSyntheticRecord))
	}))
	defer server.Close()
	store := freshStore(t)
	ctx := context.Background()
	cfg := loopbackConfig(server)
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := Discover(ctx, store, "official-concurrent", cfg); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	report, err := Discover(ctx, store, "official-concurrent", cfg)
	if err != nil || report.State != "complete" || report.Accepted != 1 {
		t.Fatalf("replay report=%+v err=%v", report, err)
	}
	rows, err := store.ListAssertions(ctx, report.RunID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	a := rows[0]
	if !a.HasCoords || a.LocationQuality != "unknown" || a.CRS != "SIRGAS2000" {
		t.Fatalf("candidate was lost/promoted: %+v", a)
	}
}
