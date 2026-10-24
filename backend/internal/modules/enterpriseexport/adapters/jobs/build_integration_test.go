//go:build integration

package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
	parentofficial "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshJobsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("export_jobs_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
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
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedJobsWeek(t *testing.T, pool *pgxpool.Pool, stationID, checksum string, start, end time.Time, amounts ...int64) string {
	t.Helper()
	ctx := context.Background()
	im := parentofficial.NewImporter(pool)
	run, err := im.BeginRun(ctx, domain.ImportKey{
		SourceURL:      "https://www.gov.br/anp/" + checksum + ".xlsx",
		SourceChecksum: checksum, ParserVersion: "esp06-it",
	}, start, end)
	if err != nil || run.NoOp {
		t.Fatalf("begin %s = %+v, %v", checksum, run, err)
	}
	products := []string{"GASOLINE_REGULAR", "ETHANOL"}
	rows := make([]domain.PriceRow, 0, len(amounts))
	for i, amount := range amounts {
		rows = append(rows, domain.PriceRow{
			StationID: stationID, Product: products[i%len(products)], Unit: "L",
			AmountMilli: amount, RawText: "5,999",
			CollectedOn: start.AddDate(0, 0, 2), SourceRow: i + 8,
		})
	}
	var tally domain.QuarantineTally
	if _, err := im.StageBatch(ctx, run.RevisionID, rows, &tally); err != nil {
		t.Fatal(err)
	}
	fin, err := im.FinishRun(ctx, run.RunID, run.RevisionID, &tally)
	if err != nil || !fin.Published {
		t.Fatalf("finish %s = %+v, %v", checksum, fin, err)
	}
	return run.RevisionID
}

func candidateCount(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "candidates"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "export-") {
			count++
		}
	}
	return count
}

func TestBuildCoalescesCorrectionsAndReconciles(t *testing.T) {
	pool := freshJobsPool(t)
	ctx := context.Background()
	root := t.TempDir()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	builder := Build{
		Pool: pool, Root: root, WindowWeeks: 2, PageSize: 2,
		Now: func() time.Time { return now },
	}
	if err := builder.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("empty build: %v", err)
	}
	if _, ok := LoadState(root); ok {
		t.Fatal("state exists without a published revision")
	}

	station, err := parentdirectory.NewRepository(pool).ResolveCNPJ(ctx, "04218406000104", "Posto A", nil)
	if err != nil {
		t.Fatal(err)
	}
	week1Start := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	week1End := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)
	week2Start := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
	week2End := time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC)
	week1 := seedJobsWeek(t, pool, station.ID, "w1a", week1Start, week1End, 5999, 3799)
	seedJobsWeek(t, pool, station.ID, "w2", week2Start, week2End, 6199, 3899)

	if err := builder.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	state, ok := LoadState(root)
	if !ok || !state.Valid() {
		t.Fatalf("first state = %+v, %v", state, ok)
	}
	if candidateCount(t, root) != 1 {
		t.Fatalf("candidates = %d", candidateCount(t, root))
	}
	if len(state.OfficialRevisions) != 2 || state.DirectoryFingerprint == "" || state.AnalyticsSHA256 == "" {
		t.Fatalf("state provenance = %+v", state)
	}
	analyticsRaw, err := os.ReadFile(filepath.Join(state.CandidateDir, "analytics-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(analyticsRaw), `"metrics_version": "abastevo-metrics-v1"`) ||
		!strings.Contains(string(analyticsRaw), state.ContentSHA256) {
		t.Errorf("analytics provenance missing")
	}
	portalDir := filepath.Join(t.TempDir(), "portal")
	portalResult, err := portalgen.Generate(portalgen.Options{
		SnapshotDir: state.CandidateDir, OutputDir: portalDir,
	})
	if err != nil {
		t.Fatalf("portal generation: %v", err)
	}
	if portalResult.Pages == 0 {
		t.Fatal("portal generated no pages")
	}
	stationPage, err := os.ReadFile(filepath.Join(portalDir, "postos", station.ID, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stationPage), "R$ 6,199") ||
		!strings.Contains(string(stationPage), "R$ 5,999") {
		t.Errorf("generated station page missing latest/history prices")
	}
	firstBuiltAt := state.BuiltAt
	if err := builder.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("no-op build: %v", err)
	}
	if candidateCount(t, root) != 1 {
		t.Errorf("unchanged build created a candidate")
	}
	stateAfterNoop, _ := LoadState(root)
	if stateAfterNoop.BuiltAt != firstBuiltAt {
		t.Error("no-op build rewrote the state")
	}

	correction := seedJobsWeek(t, pool, station.ID, "w1b", week1Start, week1End, 5799, 3699)
	if err := builder.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("correction build: %v", err)
	}
	state, _ = LoadState(root)
	if !state.Valid() || candidateCount(t, root) != 2 {
		t.Fatalf("correction state/candidates = %+v/%d", state, candidateCount(t, root))
	}
	foundCorrection := false
	for _, revision := range state.OfficialRevisions {
		if revision.RevisionID == correction {
			foundCorrection = true
		}
	}
	if !foundCorrection || correction == week1 {
		t.Errorf("correction pin missing: %+v", state.OfficialRevisions)
	}

	release, err := Lock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.Handle(ctx, platformjobs.Job{}); !errors.Is(err, ErrBuildBusy) {
		t.Errorf("busy build = %v, want ErrBuildBusy", err)
	}
	release()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := builder.Handle(cancelled, platformjobs.Job{}); err == nil {
		t.Error("cancelled build was accepted")
	}
	if candidateCount(t, root) != 2 {
		t.Error("cancelled build created a candidate")
	}

	var enqueued []BuildPayload
	var dedupes []string
	reconcile := Reconcile{
		Pool: pool, Root: root, WindowWeeks: 2, PageSize: 2,
		Enqueue: func(_ context.Context, payload BuildPayload, dedupe string) (platformjobs.EnqueueResult, error) {
			enqueued = append(enqueued, payload)
			dedupes = append(dedupes, dedupe)
			return platformjobs.EnqueueResult{}, nil
		},
	}
	if err := reconcile.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("current reconcile: %v", err)
	}
	if len(enqueued) != 0 {
		t.Fatalf("current state enqueued: %+v", enqueued)
	}
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", station.ID); err != nil {
		t.Fatal(err)
	}
	if err := reconcile.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("directory reconcile: %v", err)
	}
	if len(enqueued) != 1 || enqueued[0].Trigger != "reconcile" || !strings.Contains(dedupes[0], ":reconcile:") {
		t.Fatalf("directory reconcile = %+v %v", enqueued, dedupes)
	}
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET status='revoked' WHERE id=$1", station.ID); err != nil {
		t.Fatal(err)
	}
	if err := reconcile.Handle(ctx, platformjobs.Job{}); err != nil {
		t.Fatalf("second directory reconcile: %v", err)
	}
	if len(enqueued) != 2 || dedupes[0] == dedupes[1] {
		t.Fatalf("changed fingerprint reused a dedupe key: %v", dedupes)
	}
}
