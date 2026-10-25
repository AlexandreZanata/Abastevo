//go:build integration && realsource

package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// TestManualRealWeeklyRecovery proves replay convergence, source-outage
// failure preservation and backup/restore equality on the registered real
// edition. It runs on a disposable database and never publishes: the real
// edition stays needs-review until the monetary-precision policy is decided.
func TestManualRealWeeklyRecovery(t *testing.T) {
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
	newHandler := func() officialjobs.Import {
		fetcher := source.NewFetcher([]source.Entry{{
			ID: "real-week", Host: u.Hostname(), PathPrefix: "/",
			FileGlob: "*.xlsx", Ports: []int{port},
		}})
		fetcher.Timeout = 60 * time.Second
		fetcher.InsecureTLS = true
		fetcher.IPAllow = func(netip.Addr) bool { return true }
		return officialjobs.Import{
			Fetch: &fetcher, Parser: anp.Parser{Limits: anp.DefaultLimits()}, Import: importer,
			Resolve: func(ctx context.Context, cnpj, display string, addr map[string]string) (string, error) {
				st, err := dirRepo.ResolveCNPJ(ctx, cnpj, display, addr)
				if err != nil {
					return "", err
				}
				return st.ID, nil
			},
		}
	}
	payload, _ := json.Marshal(officialjobs.ImportPayload{
		Version: 1, SourceURL: srv.URL + filePath,
		SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17",
	})
	counts := func() (stations, prices, runs int64, revisions string) {
		t.Helper()
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&stations); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM official_station_prices").Scan(&prices); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM official_import_runs").Scan(&runs); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT coalesce(string_agg(status, ',' ORDER BY survey_start), '') FROM official_revisions").Scan(&revisions); err != nil {
			t.Fatal(err)
		}
		return stations, prices, runs, revisions
	}
	// First import: the registered edition fails closed at the review gate.
	if err := newHandler().Handle(ctx, platformjobs.Job{Payload: payload}); err != nil {
		t.Fatalf("first real import: %v", err)
	}
	stations, prices, runs, revisions := counts()
	if stations != 7708 || prices != 19609 || revisions != "needs-review" {
		t.Fatalf("first import = %d stations %d prices revisions %q", stations, prices, revisions)
	}
	// Replay of the identical payload converges: no duplicate stations or
	// prices, no second accepted revision.
	if err := newHandler().Handle(ctx, platformjobs.Job{Payload: payload}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	stations2, prices2, runs2, revisions2 := counts()
	if stations2 != stations || prices2 != prices || revisions2 != revisions {
		t.Fatalf("replay diverged: %d/%d/%q -> %d/%d/%q", stations, prices, revisions, stations2, prices2, revisions2)
	}
	if runs2 < runs {
		t.Fatalf("replay lost run history: %d -> %d", runs, runs2)
	}
	// Source outage fails visibly and preserves the last accepted state:
	// no new revision, counts unchanged. The outage server is closed
	// before the request, so the failure is a refused connection, not a
	// policy refusal.
	outageSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	outageURL, _ := url.Parse(outageSrv.URL)
	outagePort, _ := strconv.Atoi(outageURL.Port())
	outageSrv.Close()
	outage, _ := json.Marshal(officialjobs.ImportPayload{
		Version: 1, SourceURL: outageSrv.URL + filePath,
		SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17",
	})
	outageFetcher := source.NewFetcher([]source.Entry{{
		ID: "real-week-outage", Host: outageURL.Hostname(), PathPrefix: "/",
		FileGlob: "*.xlsx", Ports: []int{outagePort},
	}})
	outageFetcher.Timeout = 5 * time.Second
	outageFetcher.InsecureTLS = true
	outageFetcher.IPAllow = func(netip.Addr) bool { return true }
	outageHandler := newHandler()
	outageHandler.Fetch = &outageFetcher
	if err := outageHandler.Handle(ctx, platformjobs.Job{Payload: outage}); err == nil {
		t.Fatal("unreachable source accepted")
	} else {
		t.Logf("outage refused visibly: %v", err)
	}
	stations3, prices3, _, revisions3 := counts()
	if stations3 != stations || prices3 != prices || revisions3 != revisions {
		t.Fatalf("outage mutated accepted state: %d/%d/%q", stations3, prices3, revisions3)
	}
	// Backup the disposable database and restore it elsewhere; canonical
	// content digests (rows, prices with raw text, runs, revisions with
	// status, provenance and pointers) must match exactly — not just counts.
	restored := backupRestoreJobsDB(t, pool)
	for _, table := range []struct{ name, order string }{
		{"directory_stations", "id"},
		{"official_station_prices", "id"},
		{"official_import_runs", "started_at, id"},
		{"official_revisions", "id"},
		{"official_current_revisions", "survey_start, survey_end"},
	} {
		wantCount, wantDigest := canonicalTable(t, pool, table.name, table.order)
		gotCount, gotDigest := canonicalTable(t, restored, table.name, table.order)
		if wantCount != gotCount || wantDigest != gotDigest {
			t.Fatalf("backup/restore mismatch %s: count %d/%d digest %s/%s",
				table.name, wantCount, gotCount, wantDigest, gotDigest)
		}
	}
	// With no published revision the export builder must leave no
	// candidate behind instead of publishing a partial one.
	builderRoot := filepath.Join(t.TempDir(), "export")
	builder := Build{Pool: pool, Root: builderRoot, WindowWeeks: 1, PageSize: 1000, MaxPrices: 500_000}
	if err := builder.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("export no-op: %v", err)
	}
	if _, ok := LoadState(builderRoot); ok {
		t.Fatal("export wrote state for an unpublished official stream")
	}
}

// canonicalTable hashes the full ordered content of one acceptance table:
// row JSON (prices with raw text, runs, revisions with status, provenance
// and pointers) so backup/restore proves identical content, not just equal
// counts. Table and order are fixed call-site constants, never input.
func canonicalTable(t *testing.T, pool *pgxpool.Pool, table, order string) (int64, string) {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT row_to_json(t) FROM (SELECT * FROM "+table+" ORDER BY "+order+") t")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	hasher := sha256.New()
	var count int64
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		hasher.Write(raw)
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return count, hex.EncodeToString(hasher.Sum(nil))
}

// backupRestoreJobsDB dumps the given disposable database with pg_dump and
// restores it into a second disposable database, returning a pool on the
// restored copy. Both databases are dropped by cleanup. The dump runs inside
// the disposable PostGIS container (ANPFUEL_TEST_PG_CONTAINER, default
// anpfuel-dev-db-1) so no host client package is required.
func backupRestoreJobsDB(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	container := os.Getenv("ANPFUEL_TEST_PG_CONTAINER")
	if container == "" {
		container = "anpfuel-dev-db-1"
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable")
	}
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	var sourceDB string
	if err := pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&sourceDB); err != nil {
		t.Fatal(err)
	}
	execDocker := func(tool string, args ...string) {
		t.Helper()
		cmdArgs := append([]string{"exec", "-e", "PGPASSWORD=" + password, container, tool,
			"-h", "127.0.0.1", "-p", "5432", "-U", u.User.Username()}, args...)
		cmd := exec.Command("docker", cmdArgs...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", tool, err, out)
		}
	}
	// The dump file is private and exclusive per execution inside the
	// container, so simultaneous tests/agents cannot overwrite backups.
	dumpName := fmt.Sprintf("/tmp/jobs-manual-%d.dump", time.Now().UnixNano())
	execDocker("pg_dump", "-Fc", "-f", dumpName, sourceDB)
	defer func() {
		_ = exec.Command("docker", "exec", container, "rm", "-f", dumpName).Run()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("export_jobs_restore_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create restore database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	execDocker("pg_restore", "-d", name, "--no-owner", dumpName)
	ru, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	ru.Path = "/" + name
	restored, err := pgxpool.New(ctx, ru.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Close)
	return restored
}
