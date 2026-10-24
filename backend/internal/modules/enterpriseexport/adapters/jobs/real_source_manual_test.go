//go:build integration && realsource

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	officialjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// TestManualRealWeeklyEndToEnd is an operator-run acceptance over one real ANP
// workbook. The file is supplied privately; the test never downloads it.
func TestManualRealWeeklyEndToEnd(t *testing.T) {
	path := os.Getenv("ANPFUEL_MANUAL_XLSX")
	if path == "" {
		t.Skip("ANPFUEL_MANUAL_XLSX is not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pool := freshJobsPool(t)
	ctx := context.Background()
	dirRepo := parentdirectory.NewRepository(pool)
	importer := parentofficial.NewImporter(pool)

	filePath := "/arquivos-lpc/2026/revendas_lpc_2026_10_11-2026_10_17.xlsx"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != filePath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", `"real-week"`)
		_, _ = w.Write(raw)
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	fetcher := source.NewFetcher([]source.Entry{{
		ID: "real-week", Host: u.Hostname(), PathPrefix: "/",
		FileGlob: "*.xlsx", Ports: []int{port},
	}})
	fetcher.Timeout = 60 * time.Second
	fetcher.InsecureTLS = true
	fetcher.IPAllow = func(netip.Addr) bool { return true }
	handler := officialjobs.Import{
		Fetch: &fetcher, Parser: anp.Parser{Limits: anp.DefaultLimits()}, Import: importer,
		Resolve: func(ctx context.Context, cnpj, display string, addr map[string]string) (string, error) {
			st, err := dirRepo.ResolveCNPJ(ctx, cnpj, display, addr)
			if err != nil {
				return "", err
			}
			return st.ID, nil
		},
	}
	payload, _ := json.Marshal(officialjobs.ImportPayload{
		Version: 1, SourceURL: srv.URL + filePath,
		SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17",
	})
	if err := handler.Handle(ctx, platformjobs.Job{Payload: payload}); err != nil {
		t.Fatalf("real import: %v", err)
	}
	var runStatus string
	var rowCounts, errorSummary []byte
	if err := pool.QueryRow(ctx,
		"SELECT status, row_counts, error_summary FROM official_import_runs ORDER BY started_at DESC LIMIT 1").
		Scan(&runStatus, &rowCounts, &errorSummary); err != nil {
		t.Fatal(err)
	}
	var revisionStatus string
	if err := pool.QueryRow(ctx,
		"SELECT status FROM official_revisions ORDER BY survey_start DESC LIMIT 1").
		Scan(&revisionStatus); err != nil {
		t.Fatal(err)
	}
	var stations, prices int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&stations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM official_station_prices").Scan(&prices); err != nil {
		t.Fatal(err)
	}
	t.Logf("run_status=%s revision_status=%s counts=%s errors=%s stations=%d staged_prices=%d",
		runStatus, revisionStatus, rowCounts, errorSummary, stations, prices)
	if revisionStatus != "published" {
		t.Skipf("real edition is %s; source review required before acceptance", revisionStatus)
	}

	root := t.TempDir()
	builder := Build{Pool: pool, Root: root, WindowWeeks: 1, PageSize: 1000, MaxPrices: 500_000}
	if err := builder.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("real build: %v", err)
	}
	state, ok := LoadState(root)
	if !ok || !state.Valid() {
		t.Fatalf("real state = %+v, %v", state, ok)
	}
	snapshot, err := enterpriseexport.ReadSnapshot(state.CandidateDir)
	if err != nil {
		t.Fatal(err)
	}
	analyticsRaw, err := os.ReadFile(state.CandidateDir + "/analytics-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Coverage []struct {
			SourceObservations    int            `json:"source_observations"`
			IncludedObservations  int            `json:"included_observations"`
			StationCount          int            `json:"station_count"`
			UnlocatedObservations int            `json:"unlocated_observations"`
			Excluded              map[string]int `json:"excluded"`
		} `json:"coverage"`
		Summaries []struct {
			Geography struct {
				Level string `json:"level"`
			} `json:"geography"`
		} `json:"summaries"`
		Rankings []struct {
			Geography struct {
				Level string `json:"level"`
			} `json:"geography"`
		} `json:"rankings"`
	}
	if err := json.Unmarshal(analyticsRaw, &projection); err != nil {
		t.Fatal(err)
	}
	t.Logf("export stations=%d revisions=%d prices=%d content=%s",
		len(snapshot.Stations), len(snapshot.Revisions), len(snapshot.Prices), state.ContentSHA256)
	if len(projection.Coverage) != 1 {
		t.Fatalf("coverage entries = %d", len(projection.Coverage))
	}
	t.Logf("coverage source=%d included=%d stations=%d unlocated=%d excluded=%v summaries=%d rankings=%d",
		projection.Coverage[0].SourceObservations,
		projection.Coverage[0].IncludedObservations,
		projection.Coverage[0].StationCount,
		projection.Coverage[0].UnlocatedObservations,
		projection.Coverage[0].Excluded,
		len(projection.Summaries), len(projection.Rankings))
	if len(snapshot.Stations) < 1000 || len(snapshot.Prices) < 1000 || len(snapshot.Revisions) != 1 {
		t.Fatalf("real snapshot too small: %d/%d/%d", len(snapshot.Stations), len(snapshot.Prices), len(snapshot.Revisions))
	}
	if projection.Coverage[0].UnlocatedObservations != projection.Coverage[0].IncludedObservations {
		t.Logf("some stations have geography; national-only assumption changed")
	}
	if len(projection.Rankings) == 0 {
		t.Fatal("no national rankings from the real edition")
	}
	fmt.Fprintln(os.Stderr, "ESP07 real source acceptance PASS")
}
