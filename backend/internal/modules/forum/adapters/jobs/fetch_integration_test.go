//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshForumJobsEnv(t *testing.T) (*forumadapters.Store, *pgxpool.Pool, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)

	name := fmt.Sprintf("forum_jobs_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database %s: %v", name, err)
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
			t.Errorf("drop database %s: %v", name, err)
		}
	})

	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()

	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool connect: %v", err)
	}
	t.Cleanup(pool.Close)

	store := forumadapters.NewStore(pool)
	return store, pool, dsn
}

func TestForumJobs_FullLifecycle_RealPostgres(t *testing.T) {
	store, pool, _ := freshForumJobsEnv(t)
	ctx := context.Background()

	const sourceID = "00000000-0000-0000-0000-000000000002"
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	// 1. Seed an approved source in real postgres
	_, err := pool.Exec(ctx, `
		INSERT INTO forum_sources (
			id, code, name, kind, origin_url, fetch_url, fetch_format,
			collection_enabled, publication_enabled, rights_status,
			title_allowed, excerpt_allowed, attribution, created_at, updated_at
		) VALUES (
			$1, 'test-news-source', 'Test Source', 'PUBLISHER',
			'https://example.com', 'https://example.com/rss.xml', 'RSS',
			true, false, 'APPROVED',
			true, true, 'Test Attribution', now(), now()
		)
	`, sourceID)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}

	feedItems := []application.FeedItem{
		{
			GUID:        "ext-guid-1",
			Title:       "Queda no preço do diesel nas refinarias",
			Link:        "https://example.com/news/1",
			Excerpt:     "Média nacional do combustível registra recuo.",
			PublishedAt: now.Add(-1 * time.Hour),
		},
		{
			GUID:        "ext-guid-2",
			Title:       "Novos radares na rodovia federal BR-101",
			Link:        "https://example.com/news/2",
			Excerpt:     "Fiscalização de velocidade ampliada pelo DNIT.",
			PublishedAt: now.Add(-30 * time.Minute),
		},
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
			return application.FetchResponse{
				NotModified: false,
				StatusCode:  200,
				ETag:        `"etag-v1"`,
				Body:        []byte("<rss/>"),
			}, nil
		},
	}

	parser := &mockParser{
		parseFunc: func(raw []byte, now time.Time) ([]application.FeedItem, error) {
			return feedItems, nil
		},
	}

	runner := &SourceFetchRunner{
		Store:   store,
		Fetcher: fetcher,
		Parser:  parser,
		Now:     func() time.Time { return now },
	}

	// 2. First Run: both items ingested as DRAFT
	reports, err := runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: sourceID})
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if len(reports) != 1 || reports[0].ItemsIngested != 2 {
		t.Fatalf("expected 2 items ingested, got %+v", reports)
	}

	// Verify posts in DB
	var postCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM forum_posts WHERE source_id = $1", sourceID).Scan(&postCount)
	if err != nil || postCount != 2 {
		t.Fatalf("expected 2 posts in DB, got %d, err=%v", postCount, err)
	}

	// Verify all candidate posts are DRAFT with 0 publication seq
	rows, err := pool.Query(ctx, "SELECT publication_state, first_publication_seq, first_published_at, category FROM forum_posts WHERE source_id = $1", sourceID)
	if err != nil {
		t.Fatalf("query posts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var seq int64
		var pubAt *time.Time
		var category string
		if err := rows.Scan(&state, &seq, &pubAt, &category); err != nil {
			t.Fatalf("scan post: %v", err)
		}
		if state != string(domain.StateDraft) {
			t.Errorf("expected state DRAFT, got %s", state)
		}
		if seq != 0 || pubAt != nil {
			t.Errorf("expected seq 0 and nil pubAt, got seq=%d, pubAt=%v", seq, pubAt)
		}
	}

	// Verify checkpoint in DB
	cp, err := store.GetSourceCheckpoint(ctx, sourceID)
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if cp.ETag != `"etag-v1"` || cp.LastStatus != "SUCCESS" || cp.ConsecutiveFailures != 0 {
		t.Fatalf("unexpected checkpoint in DB: %+v", cp)
	}

	// 3. Second Run: duplicate items -> deduplication prevents new rows
	reports2, err := runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: sourceID})
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if reports2[0].ItemsIngested != 0 {
		t.Fatalf("expected 0 new items ingested on duplicate run, got %d", reports2[0].ItemsIngested)
	}
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM forum_posts WHERE source_id = $1", sourceID).Scan(&postCount)
	if err != nil || postCount != 2 {
		t.Fatalf("expected post count to remain 2, got %d", postCount)
	}

	// 4. Third Run: conditional GET returns 304 Not Modified
	fetcher.fetchFunc = func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
		if req.ETag != `"etag-v1"` {
			t.Errorf("expected conditional ETag %q, got %q", `"etag-v1"`, req.ETag)
		}
		return application.FetchResponse{
			NotModified: true,
			StatusCode:  304,
			ETag:        `"etag-v1"`,
		}, nil
	}

	reports3, err := runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: sourceID})
	if err != nil {
		t.Fatalf("third run failed: %v", err)
	}
	if reports3[0].Status != "NOT_MODIFIED" {
		t.Fatalf("expected NOT_MODIFIED, got %s", reports3[0].Status)
	}

	// 5. Rights Revocation: change rights_status to 'DENIED' -> collection is BLOCKED
	_, err = pool.Exec(ctx, "UPDATE forum_sources SET rights_status = 'DENIED' WHERE id = $1", sourceID)
	if err != nil {
		t.Fatalf("update rights_status: %v", err)
	}

	fetcherCalled := false
	fetcher.fetchFunc = func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
		fetcherCalled = true
		return application.FetchResponse{}, nil
	}

	reports4, err := runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: sourceID})
	if err != nil {
		t.Fatalf("run after revocation failed: %v", err)
	}
	if fetcherCalled {
		t.Fatalf("fetcher must NOT be called after rights revocation")
	}
	if reports4[0].Status != "BLOCKED" {
		t.Fatalf("expected BLOCKED status after revocation, got %s", reports4[0].Status)
	}
}

func TestForumJobs_PublicFeedExcludesCandidates(t *testing.T) {
	store, pool, _ := freshForumJobsEnv(t)
	ctx := context.Background()

	const sourceID = "00000000-0000-0000-0000-000000000003"
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	// Seed source
	_, err := pool.Exec(ctx, `
		INSERT INTO forum_sources (
			id, code, name, kind, origin_url, fetch_url, fetch_format,
			collection_enabled, publication_enabled, rights_status,
			title_allowed, excerpt_allowed, attribution, created_at, updated_at
		) VALUES (
			$1, 'draft-source', 'Draft Source', 'PUBLISHER',
			'https://example.com', 'https://example.com/rss.xml', 'RSS',
			true, false, 'APPROVED',
			true, true, 'Attribution', now(), now()
		)
	`, sourceID)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}

	runner := &SourceFetchRunner{
		Store: store,
		Fetcher: &mockFetcher{
			fetchFunc: func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
				return application.FetchResponse{
					StatusCode: 200,
					Body:       []byte("<rss/>"),
				}, nil
			},
		},
		Parser: &mockParser{
			parseFunc: func(raw []byte, now time.Time) ([]application.FeedItem, error) {
				return []application.FeedItem{
					{
						GUID:        "candidate-1",
						Title:       "Notícia candidata em rascunho",
						Link:        "https://example.com/draft-1",
						Excerpt:     "Ainda não revisada por editor.",
						PublishedAt: now,
					},
				}, nil
			},
		},
		Now: func() time.Time { return now },
	}

	_, err = runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: sourceID})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// Query public posts
	page, err := store.ListPublicPosts(ctx, application.ListPublicPostsQuery{Limit: 20})
	if err != nil {
		t.Fatalf("list public posts: %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("draft candidate post must NOT appear in public feed, got %d items", len(page.Items))
	}
}

func TestForumJobs_DispatcherQueueAndCoexistence(t *testing.T) {
	store, pool, _ := freshForumJobsEnv(t)
	ctx := context.Background()

	const sourceID = "00000000-0000-0000-0000-000000000004"
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	// Seed source
	_, err := pool.Exec(ctx, `
		INSERT INTO forum_sources (
			id, code, name, kind, origin_url, fetch_url, fetch_format,
			collection_enabled, publication_enabled, rights_status,
			title_allowed, excerpt_allowed, attribution, created_at, updated_at
		) VALUES (
			$1, 'queue-source', 'Queue Source', 'PUBLISHER',
			'https://example.com', 'https://example.com/rss.xml', 'RSS',
			true, false, 'APPROVED',
			true, true, 'Attribution', now(), now()
		)
	`, sourceID)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}

	runner := SourceFetchRunner{
		Store: store,
		Fetcher: &mockFetcher{
			fetchFunc: func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
				return application.FetchResponse{
					StatusCode: 200,
					Body:       []byte("<rss/>"),
				}, nil
			},
		},
		Parser: &mockParser{
			parseFunc: func(raw []byte, now time.Time) ([]application.FeedItem, error) {
				return []application.FeedItem{
					{
						GUID:        "item-queue-1",
						Title:       "Preço dos combustíveis em alta",
						Link:        "https://example.com/queue-1",
						Excerpt:     "Média sobe.",
						PublishedAt: now,
					},
				}, nil
			},
		},
		Now: func() time.Time { return now },
	}

	forumHandler := ForumSourceFetchHandler{Runner: runner}

	// Dummy privacy sweep handler simulating privacy job coexistence
	privacySweepExecuted := false
	dummyPrivacyHandler := dummyHandler{
		kind:    "privacy-training-sweep",
		version: 1,
		handle: func(ctx context.Context, job platformjobs.Job) error {
			privacySweepExecuted = true
			return nil
		},
	}

	handlers := map[string]platformjobs.Handler{
		forumHandler.Kind():        forumHandler,
		dummyPrivacyHandler.Kind(): dummyPrivacyHandler,
	}

	queue := platformjobs.NewQueue(pool)
	dispatcher1 := &platformjobs.Dispatcher{
		Queue:    queue,
		Handlers: handlers,
		WorkerID: "worker-1",
	}
	dispatcher2 := &platformjobs.Dispatcher{
		Queue:    queue,
		Handlers: handlers,
		WorkerID: "worker-2",
	}

	// Enqueue forum fetch job and privacy sweep job
	forumPayload, _ := json.Marshal(FetchJobPayload{Version: 1, SourceID: sourceID})
	_, err = platformjobs.Enqueue(ctx, pool, forumHandler.Kind(), forumPayload, "dedupe-forum-1", 3, time.Time{})
	if err != nil {
		t.Fatalf("enqueue forum job: %v", err)
	}

	privacyPayload, _ := json.Marshal(map[string]any{"version": 1})
	_, err = platformjobs.Enqueue(ctx, pool, dummyPrivacyHandler.Kind(), privacyPayload, "dedupe-privacy-1", 3, time.Time{})
	if err != nil {
		t.Fatalf("enqueue privacy job: %v", err)
	}

	// Dispatcher 1 runs one job
	found1, err := dispatcher1.RunOnce(ctx)
	if err != nil || !found1 {
		t.Fatalf("dispatcher1 run once: found=%v, err=%v", found1, err)
	}

	// Dispatcher 2 runs the other job
	found2, err := dispatcher2.RunOnce(ctx)
	if err != nil || !found2 {
		t.Fatalf("dispatcher2 run once: found=%v, err=%v", found2, err)
	}

	if !privacySweepExecuted {
		t.Errorf("privacy safety job must execute cleanly alongside forum fetch")
	}

	// Verify post was ingested
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM forum_posts WHERE source_id = $1", sourceID).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 ingested post, got %d, err=%v", count, err)
	}
}

type dummyHandler struct {
	kind    string
	version int
	handle  func(ctx context.Context, job platformjobs.Job) error
}

func (d dummyHandler) Kind() string { return d.kind }
func (d dummyHandler) Version() int { return d.version }
func (d dummyHandler) Handle(ctx context.Context, job platformjobs.Job) error {
	return d.handle(ctx, job)
}
