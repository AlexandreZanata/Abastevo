//go:build load && integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type forumLoadTracer struct{ queries atomic.Int64 }

func (q *forumLoadTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.queries.Add(1)
	return ctx
}

func (q *forumLoadTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type forumLoadEnv struct {
	pool    *pgxpool.Pool
	server  *httptest.Server
	router  chi.Router
	tracer  *forumLoadTracer
	postIDs []string
}

func freshForumLoadEnv(t *testing.T, posts int) *forumLoadEnv {
	t.Helper()
	adminDSN := testDSN(t)
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("forum_load_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Logf("admin reconnect cleanup: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Logf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolCfg.MaxConns = int32(envInt("FORUM_LOAD_POOL", 10))
	tracer := &forumLoadTracer{}
	poolCfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := seedForumLoad(ctx, pool, posts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tracer.queries.Store(0)
	store := forumadapters.NewStore(pool)
	cfg := domain.DefaultConfig()
	cfg.ReadsEnabled = true
	svc := application.NewService(cfg, store, store, mockClock{now: time.Now().UTC()})
	h := NewHandler(svc, testCursorSecret)
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	rows, err := pool.Query(ctx, `SELECT id::text FROM forum_posts WHERE publication_state = 'PUBLISHED' ORDER BY first_publication_seq DESC LIMIT 1000`)
	if err != nil {
		t.Fatalf("select detail ids: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatal("no published load fixtures")
	}
	return &forumLoadEnv{pool: pool, server: server, router: router, tracer: tracer, postIDs: ids}
}

func seedForumLoad(ctx context.Context, pool *pgxpool.Pool, posts int) error {
	if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, alias, status) VALUES ('00000000-0000-4000-8000-000000000123', 'forum-load-reader', 'active') ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO forum_posts (
			id, kind, category, title, excerpt, body_text, source_id,
			local_created_at, local_updated_at, first_published_at,
			first_publication_seq, publication_state, visibility, revision,
			comments_enabled, region_uf, fuel_product, rights_snapshot
		)
		SELECT md5('forum-load-post-' || g::text)::uuid,
			'EDITORIAL',
			(ARRAY['FUELS','ELECTRIC','TRAFFIC_RULES','VEHICLES_TECH','ROADS_MOBILITY','DRIVER_ECONOMY','ABASTEVO_DATA'])[1 + g % 7],
			'Boletim sintético ' || g::text,
			'Resumo sintético de carga.',
			'Corpo sintético de carga para leitura pública.',
			'00000000-0000-0000-0000-000000000001',
			now() - (g::text || ' seconds')::interval, now(),
			CASE WHEN g % 10 = 0 THEN NULL ELSE now() - (g::text || ' seconds')::interval END,
			CASE WHEN g % 10 = 0 THEN 0 ELSE g END,
			CASE WHEN g % 10 = 0 THEN 'PENDING_REVIEW' ELSE 'PUBLISHED' END,
			'VISIBLE', 1, true,
			CASE WHEN g % 27 = 0 THEN 'SP' ELSE NULL END,
			CASE WHEN g % 7 = 0 THEN 'GASOLINE_REGULAR' ELSE NULL END,
			NULL
		FROM generate_series(1, $1) AS g
	`, posts); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO forum_comments (
			id, post_id, author_account_id, public_alias, parent_id, depth, sequence,
			body_text, publication_state, visibility, published_at, create_operation_id
		)
		SELECT md5('forum-load-comment-' || p.rn::text || '-' || k::text)::uuid,
			p.id, '00000000-0000-4000-8000-000000000123', 'Leitor', NULL, 0, k,
			'Comentário sintético.', 'PUBLISHED', 'VISIBLE', now(),
			md5('forum-load-op-' || p.rn::text || '-' || k::text)::uuid
		FROM (
			SELECT id, row_number() OVER (ORDER BY first_publication_seq) AS rn
			FROM forum_posts
			WHERE publication_state = 'PUBLISHED'
			LIMIT 10000
		) AS p
		CROSS JOIN generate_series(1, 2) AS k
	`); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `ANALYZE forum_posts; ANALYZE forum_comments;`)
	return err
}

type forumLoadMetrics struct {
	Phase        string  `json:"phase"`
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	Drops        int64   `json:"drops"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	P99Ms        float64 `json:"p99_ms"`
	MaxFeedKiB   float64 `json:"max_feed_kib"`
	MaxDetailKiB float64 `json:"max_detail_kib"`
}

func runForumLoadPhase(ctx context.Context, client *http.Client, baseURL string, env *forumLoadEnv, rps float64, duration time.Duration, maxInflight int) forumLoadMetrics {
	metrics := forumLoadMetrics{Phase: "phase"}
	if rps <= 0 || duration <= 0 {
		return metrics
	}
	interval := time.Duration(float64(time.Second) / rps)
	if interval < time.Microsecond {
		interval = time.Microsecond
	}
	sem := make(chan struct{}, maxInflight)
	var wg sync.WaitGroup
	var mu sync.Mutex
	durations := make([]time.Duration, 0, int(rps*float64(duration/time.Second))+16)
	var mix atomic.Int64
	var detailIndex atomic.Int64
	maxFeed := int64(0)
	maxDetail := int64(0)
	start := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for time.Since(start) < duration {
		<-ticker.C
		select {
		case sem <- struct{}{}:
		default:
			mu.Lock()
			metrics.Drops++
			mu.Unlock()
			continue
		}
		requestKind := "feed"
		path := "/v1/forum/posts?limit=20"
		n := mix.Add(1)
		if n%5 == 0 {
			requestKind = "detail"
			path = "/v1/forum/posts/" + env.postIDs[int(detailIndex.Add(1))%len(env.postIDs)]
		} else if n%10 == 0 {
			path = "/v1/forum/posts?uf=SP&limit=20"
		} else if n%7 == 0 {
			path = "/v1/forum/posts?category=FUELS&limit=20"
		}
		wg.Add(1)
		go func(kind, reqPath string) {
			defer wg.Done()
			defer func() { <-sem }()
			begin := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+reqPath, nil)
			if err != nil {
				mu.Lock()
				metrics.Errors++
				mu.Unlock()
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				mu.Lock()
				metrics.Errors++
				mu.Unlock()
				return
			}
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			elapsed := time.Since(begin)
			mu.Lock()
			metrics.Requests++
			durations = append(durations, elapsed)
			if readErr != nil || resp.StatusCode != http.StatusOK {
				metrics.Errors++
			}
			if kind == "detail" {
				if int64(len(body)) > maxDetail {
					maxDetail = int64(len(body))
				}
			} else if int64(len(body)) > maxFeed {
				maxFeed = int64(len(body))
			}
			mu.Unlock()
		}(requestKind, path)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		percentile := func(p float64) float64 {
			idx := int(float64(len(durations)-1) * p)
			return float64(durations[idx].Microseconds()) / 1000
		}
		metrics.P50Ms = percentile(0.50)
		metrics.P95Ms = percentile(0.95)
		metrics.P99Ms = percentile(0.99)
	}
	metrics.MaxFeedKiB = float64(maxFeed) / 1024
	metrics.MaxDetailKiB = float64(maxDetail) / 1024
	return metrics
}

func envInt(key string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if value, err := strconv.ParseFloat(raw, 64); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if value, err := time.ParseDuration(raw); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func TestForumLoad_BoundedScenario(t *testing.T) {
	posts := envInt("FORUM_LOAD_POSTS", 10000)
	rps := envFloat("FORUM_LOAD_RPS", 1.39)
	burstRPS := envFloat("FORUM_LOAD_BURST_RPS", rps*10)
	maxInflight := envInt("FORUM_LOAD_MAX_INFLIGHT", 20)
	warmup := envDuration("FORUM_LOAD_WARMUP", 30*time.Second)
	steady := envDuration("FORUM_LOAD_STEADY", 2*time.Minute)
	burst := envDuration("FORUM_LOAD_BURST", 20*time.Second)
	if os.Getenv("FORUM_LOAD_FULL") == "1" {
		warmup, steady, burst = 5*time.Minute, 10*time.Minute, 60*time.Second
	}

	env := freshForumLoadEnv(t, posts)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
	}}
	poolBefore := env.pool.Stat()
	rssBefore := rssKiB()
	warm := runForumLoadPhase(context.Background(), client, env.server.URL, env, rps, warmup, maxInflight)
	steadyMetrics := runForumLoadPhase(context.Background(), client, env.server.URL, env, rps, steady, maxInflight)
	burstMetrics := runForumLoadPhase(context.Background(), client, env.server.URL, env, burstRPS, burst, maxInflight)
	poolAfter := env.pool.Stat()
	summary := map[string]any{
		"posts": posts, "rps": rps, "burst_rps": burstRPS, "max_inflight": maxInflight,
		"warmup": warmup.String(), "steady": steady.String(), "burst": burst.String(),
		"warmup_result": warm, "steady_result": steadyMetrics, "burst_result": burstMetrics,
		"queries":        env.tracer.queries.Load(),
		"pool_before":    map[string]any{"acquire_count": poolBefore.AcquireCount(), "acquire_duration": poolBefore.AcquireDuration().String()},
		"pool_after":     map[string]any{"acquire_count": poolAfter.AcquireCount(), "acquire_duration": poolAfter.AcquireDuration().String(), "empty_acquire_count": poolAfter.EmptyAcquireCount(), "canceled_acquire_count": poolAfter.CanceledAcquireCount(), "new_conns_count": poolAfter.NewConnsCount()},
		"rss_before_kib": rssBefore, "rss_after_kib": rssKiB(),
	}
	encoded, _ := json.Marshal(summary)
	t.Logf("FORUM_LOAD_SUMMARY %s", encoded)
	if steadyMetrics.Errors > 0 || steadyMetrics.Drops > 0 || burstMetrics.Errors > 0 || burstMetrics.Drops > 0 {
		t.Fatalf("load errors/drops steady=%+v burst=%+v", steadyMetrics, burstMetrics)
	}
	if steadyMetrics.P95Ms > 300 {
		t.Fatalf("steady p95 %.1fms exceeds 300ms", steadyMetrics.P95Ms)
	}
	if steadyMetrics.MaxFeedKiB > 64 || steadyMetrics.MaxDetailKiB > 32 {
		t.Fatalf("response size gate feed=%.1fKiB detail=%.1fKiB", steadyMetrics.MaxFeedKiB, steadyMetrics.MaxDetailKiB)
	}
}

func rssKiB() int64 {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, _ := strconv.ParseInt(fields[1], 10, 64)
				return value
			}
		}
	}
	return 0
}
