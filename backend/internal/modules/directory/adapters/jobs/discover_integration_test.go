//go:build integration

package jobs

// RST-08 discover-pipeline integration on real disposable PostGIS: full
// dispatched cycles, crash recovery through lease expiry, outage
// dead-lettering and conditional skip without refetch. The loopback
// provider never leaves the test process; no live source is touched.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func discoverTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshDiscoverDB(t *testing.T) (*pgxpool.Pool, *registry.PGStore, *jobs.Queue) {
	t.Helper()
	adminDSN := discoverTestDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("discoverjobs_test_%d", time.Now().UnixNano())
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
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	store := &registry.PGStore{Q: directory.New(pool)}
	return pool, store, jobs.NewQueue(pool)
}

func discoverLoopback(t *testing.T, hits *int, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*hits++
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
}

const discoverItemPage = `{"items": [{"cnpj": "04218406000104", "razaoSocial": "[RST08-TEST] POSTO KILO LTDA", "nomeFantasia": "Kilo", "endereco": {"municipio": "SAO PAULO", "uf": "SP", "codigoIbge": "3550308"}, "situacao": "ATIVA", "location_quality": "unknown", "coordenadas": null}], "page": 1, "pageSize": 100, "nextCursor": null}`

func discoverHandler(store *registry.PGStore, server *httptest.Server) Discover {
	return Discover{Store: store, Config: registry.APIConfig{
		BaseURL: server.URL, AllowedHost: "127.0.0.1", AllowLoopback: true,
		PageSize: 100, MaxPages: 5, MaxRequests: 10,
	}}
}

func enqueueDiscover(t *testing.T, pool *pgxpool.Pool, snapshot string, maxAttempts int32) {
	t.Helper()
	payload, err := EncodeDiscoverPayload(snapshot, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.Enqueue(context.Background(), pool, "registry-discover", payload, DiscoverDedupeKey(snapshot), maxAttempts, time.Time{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
}

func TestDiscoverDispatchedCycleCompletes(t *testing.T) {
	pool, store, queue := freshDiscoverDB(t)
	ctx := context.Background()
	var hits int
	server := discoverLoopback(t, &hits, 0, discoverItemPage)
	defer server.Close()

	enqueueDiscover(t, pool, "api:e2e-1", 3)
	dispatch := &jobs.Dispatcher{
		Queue: queue, Handlers: map[string]jobs.Handler{"registry-discover": discoverHandler(store, server)},
		WorkerID: "test-worker", LeaseTTL: time.Minute, RetryDelay: time.Millisecond,
	}
	found, err := dispatch.RunOnce(ctx)
	if err != nil || !found {
		t.Fatalf("runonce = %v, %v", found, err)
	}
	report, err := store.GetRun(ctx, registry.SourceAPI, "api:e2e-1")
	if err != nil || report.State != "complete" {
		t.Fatalf("run = %+v, err = %v", report, err)
	}
	if hits != 1 {
		t.Fatalf("provider hits = %d, want exactly 1", hits)
	}
	// Re-enqueue converges on the same job; nothing refetches.
	enqueueDiscover(t, pool, "api:e2e-1", 3)
	found, err = dispatch.RunOnce(ctx)
	if err != nil || found {
		t.Fatalf("second runonce = %v, %v (completed work must not reschedule)", found, err)
	}
	if hits != 1 {
		t.Fatalf("provider hits = %d after replay, want still 1", hits)
	}
	queued, dead, _, err := queue.Backlog(ctx)
	if err != nil || queued != 0 || dead != 0 {
		t.Fatalf("backlog = %d/%d, err = %v", queued, dead, err)
	}
}

func TestDiscoverCrashRecoversThroughLeaseExpiry(t *testing.T) {
	pool, store, queue := freshDiscoverDB(t)
	ctx := context.Background()
	var hits int
	server := discoverLoopback(t, &hits, 0, discoverItemPage)
	defer server.Close()

	enqueueDiscover(t, pool, "api:crash-1", 3)
	claimed, err := queue.Claim(ctx, "registry-discover", "crashed-worker", time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Worker dies holding the lease: no Complete, no Fail.
	time.Sleep(1200 * time.Millisecond)
	reclaimed, err := queue.Claim(ctx, "registry-discover", "recovery-worker", time.Minute)
	if err != nil {
		t.Fatalf("reclaim after expiry: %v", err)
	}
	if reclaimed.ID == claimed.ID && reclaimed.LeaseToken == claimed.LeaseToken {
		t.Fatal("expired lease was not re-pooled")
	}
	dispatch := &jobs.Dispatcher{
		Queue: queue, Handlers: map[string]jobs.Handler{"registry-discover": discoverHandler(store, server)},
		WorkerID: "recovery-worker", LeaseTTL: time.Minute, RetryDelay: time.Millisecond,
	}
	// The dispatcher cannot see our manual claim; complete it directly so
	// the lease state stays consistent, then prove the snapshot converges.
	if err := queue.Complete(ctx, reclaimed.ID, reclaimed.LeaseToken); err != nil {
		t.Fatalf("complete reclaimed: %v", err)
	}
	enqueueDiscover(t, pool, "api:crash-1", 3)
	if found, err := dispatch.RunOnce(ctx); err != nil || found {
		t.Fatalf("runonce after recovery = %v, %v", found, err)
	}
}

func TestDiscoverOutageDeadLetters(t *testing.T) {
	pool, store, queue := freshDiscoverDB(t)
	ctx := context.Background()

	// Closed loopback port: instant transport failure, no hanging tests.
	handler := Discover{Store: store, Config: registry.APIConfig{
		BaseURL: "http://127.0.0.1:1", AllowedHost: "127.0.0.1", AllowLoopback: true,
		PageSize: 100, MaxPages: 2, MaxRequests: 4,
	}}
	enqueueDiscover(t, pool, "api:outage-1", 1)
	dispatch := &jobs.Dispatcher{
		Queue: queue, Handlers: map[string]jobs.Handler{"registry-discover": handler},
		WorkerID: "test-worker", LeaseTTL: time.Minute, RetryDelay: time.Millisecond,
	}
	if found, err := dispatch.RunOnce(ctx); err != nil || !found {
		t.Fatalf("runonce = %v, %v", found, err)
	}
	queued, dead, _, err := queue.Backlog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 0 || dead != 1 {
		t.Fatalf("backlog = %d queued/%d dead, want 0/1 (single-attempt circuit)", queued, dead)
	}
	report, err := store.GetRun(ctx, registry.SourceAPI, "api:outage-1")
	if err != nil || report.State != "failed" {
		t.Fatalf("run = %+v, err = %v (failed runs never complete)", report, err)
	}
}

func TestDiscoverProviderPressureBacksOff(t *testing.T) {
	pool, store, queue := freshDiscoverDB(t)
	ctx := context.Background()
	var hits int
	server := discoverLoopback(t, &hits, 429, "")
	defer server.Close()

	enqueueDiscover(t, pool, "api:pressure-1", 3)
	dispatch := &jobs.Dispatcher{
		Queue: queue, Handlers: map[string]jobs.Handler{"registry-discover": discoverHandler(store, server)},
		WorkerID: "test-worker", LeaseTTL: time.Minute, RetryDelay: 50 * time.Millisecond,
	}
	if found, err := dispatch.RunOnce(ctx); err != nil || !found {
		t.Fatalf("runonce = %v, %v", found, err)
	}
	if hits != 3 {
		t.Fatalf("provider hits = %d, want exactly the 3-attempt budget", hits)
	}
	// The job re-queues with backoff instead of completing or dying yet.
	queued, dead, _, err := queue.Backlog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 || dead != 0 {
		t.Fatalf("backlog = %d queued/%d dead, want 1/0 (backoff, not burial)", queued, dead)
	}
}
