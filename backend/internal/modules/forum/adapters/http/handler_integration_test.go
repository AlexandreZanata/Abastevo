//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

var testCursorSecret = []byte("test-cursor-secret-32-bytes-xxxx")

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
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshForumTestEnv(t *testing.T) (*forumadapters.Store, *pgxpool.Pool, chi.Router) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)

	name := fmt.Sprintf("forum_http_%d", time.Now().UnixNano())
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

	cfg := domain.DefaultConfig()
	cfg.ReadsEnabled = true
	cfg.AdminEnabled = true
	svc := application.NewService(cfg, store, store, mockClock{now: time.Now().UTC()})

	h := NewHandler(svc, testCursorSecret)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	return store, pool, r
}

func TestHandler_Integration_PublicReadLifecycle(t *testing.T) {
	store, pool, router := freshForumTestEnv(t)
	ctx := context.Background()

	// Use official Abastevo source seeded by migration 000157
	const sourceID = "00000000-0000-0000-0000-000000000001"

	// 1. Categories
	req := httptest.NewRequest(http.MethodGet, "/v1/forum/categories", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("categories: expected 200, got %d", rec.Code)
	}
	var catResp CategoriesResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &catResp); err != nil {
		t.Fatalf("unmarshal categories: %v", err)
	}
	if len(catResp.Items) != 7 || catResp.TaxonomyVersion != 1 {
		t.Fatalf("unexpected categories response: %+v", catResp)
	}

	// 2. Initial empty feed
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed: expected 200, got %d", rec.Code)
	}
	var feedResp FeedResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &feedResp); err != nil {
		t.Fatalf("unmarshal feed: %v", err)
	}
	if len(feedResp.Items) != 0 || feedResp.NextCursor != nil {
		t.Fatalf("expected empty feed, got %d items", len(feedResp.Items))
	}

	// 3. Create Draft Post in real DB
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	postID1 := "22222222-2222-4222-8222-222222222221"
	now := time.Now().UTC()
	draft := &domain.Post{
		ID:                  postID1,
		Kind:                domain.KindEditorial,
		Category:            domain.CategoryFuels,
		Title:               "Primeiro Artigo Sobre Etanol",
		Excerpt:             "Resumo do primeiro artigo",
		BodyText:            "Corpo completo do primeiro artigo sobre a paridade de etanol.",
		SourceID:            sourceID,
		CanonicalURL:        "https://abastevo.com.br/artigo-1",
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 0,
		PublicationState:    domain.StateDraft,
		Visibility:          domain.VisibilityVisible,
		Revision:            1,
		CommentsEnabled:     true,
	}
	if err := store.CreateDraftTx(ctx, tx, draft); err != nil {
		t.Fatalf("create draft tx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit draft tx: %v", err)
	}

	// Verify draft is NOT in public feed
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &feedResp); err != nil {
		t.Fatalf("unmarshal feed: %v", err)
	}
	if len(feedResp.Items) != 0 {
		t.Fatalf("expected 0 items in feed while draft, got %d", len(feedResp.Items))
	}

	// Verify detail returns 404 for draft
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+postID1, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("draft detail: expected 404, got %d", rec.Code)
	}

	// 4. Publish the post
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	pubAt := time.Now().UTC()
	pubSeq, err := store.BumpFeedRevisionTx(ctx, tx, pubAt)
	if err != nil {
		t.Fatalf("bump feed: %v", err)
	}
	if err := store.PublishEditorialTx(ctx, tx, postID1, pubAt, pubSeq); err != nil {
		t.Fatalf("publish tx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit pub tx: %v", err)
	}

	// Verify published post IS in public feed
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed: expected 200, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &feedResp); err != nil {
		t.Fatalf("unmarshal feed: %v", err)
	}
	if len(feedResp.Items) != 1 {
		t.Fatalf("expected 1 item in feed, got %d", len(feedResp.Items))
	}
	if feedResp.Items[0].ID != postID1 || feedResp.Items[0].Title != "Primeiro Artigo Sobre Etanol" {
		t.Fatalf("unexpected summary in feed: %+v", feedResp.Items[0])
	}

	// Verify detail returns 200 OK with full body
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+postID1, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: expected 200, got %d", rec.Code)
	}
	var detailResp PostDetailResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if detailResp.Post.ID != postID1 || detailResp.Post.BodyText == nil || *detailResp.Post.BodyText != "Corpo completo do primeiro artigo sobre a paridade de etanol." {
		t.Fatalf("unexpected detail response: %+v", detailResp.Post)
	}

	// 5. Hide the post
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.SetPostVisibilityTx(ctx, tx, postID1, domain.VisibilityHidden, time.Now().UTC()); err != nil {
		t.Fatalf("hide tx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit hide tx: %v", err)
	}

	// Verify feed is empty again
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &feedResp); err != nil {
		t.Fatalf("unmarshal feed: %v", err)
	}
	if len(feedResp.Items) != 0 {
		t.Fatalf("expected 0 items after hide, got %d", len(feedResp.Items))
	}

	// Verify detail returns 404 when hidden
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+postID1, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("hidden detail: expected 404, got %d", rec.Code)
	}

	// 6. Unhide the post
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := store.SetPostVisibilityTx(ctx, tx, postID1, domain.VisibilityVisible, time.Now().UTC()); err != nil {
		t.Fatalf("unhide tx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit unhide tx: %v", err)
	}

	// 7. Keyset Pagination with Multiple Posts
	postID2 := "22222222-2222-4222-8222-222222222222"
	postID3 := "22222222-2222-4222-8222-222222222223"
	for i, id := range []string{postID2, postID3} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		p := &domain.Post{
			ID:                  id,
			Kind:                domain.KindEditorial,
			Category:            domain.CategoryFuels,
			Title:               fmt.Sprintf("Artigo Adicional %d", i+2),
			Excerpt:             "Resumo adicional",
			BodyText:            "Corpo adicional para teste de paginação.",
			SourceID:            sourceID,
			CanonicalURL:        fmt.Sprintf("https://abastevo.com.br/artigo-%d", i+2),
			LocalCreatedAt:      now.Add(time.Duration(i+1) * time.Hour),
			LocalUpdatedAt:      now.Add(time.Duration(i+1) * time.Hour),
			FirstPublicationSeq: 0,
			PublicationState:    domain.StateDraft,
			Visibility:          domain.VisibilityVisible,
			Revision:            1,
		}
		if err := store.CreateDraftTx(ctx, tx, p); err != nil {
			t.Fatalf("create draft %s: %v", id, err)
		}
		pubTime := now.Add(time.Duration(i+1) * time.Hour)
		seq, err := store.BumpFeedRevisionTx(ctx, tx, pubTime)
		if err != nil {
			t.Fatalf("bump seq: %v", err)
		}
		if err := store.PublishEditorialTx(ctx, tx, id, pubTime, seq); err != nil {
			t.Fatalf("publish %s: %v", id, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit %s: %v", id, err)
		}
	}

	// Request page 1 with limit 2
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=2", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 1: expected 200, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &feedResp); err != nil {
		t.Fatalf("unmarshal page 1: %v", err)
	}
	if len(feedResp.Items) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(feedResp.Items))
	}
	if feedResp.NextCursor == nil || *feedResp.NextCursor == "" {
		t.Fatalf("expected next_cursor on page 1")
	}
	page1Cursor := *feedResp.NextCursor

	// Request page 2 with cursor
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=2&cursor="+page1Cursor, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 2: expected 200, got %d", rec.Code)
	}
	var feedResp2 FeedResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &feedResp2); err != nil {
		t.Fatalf("unmarshal page 2: %v", err)
	}
	if len(feedResp2.Items) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(feedResp2.Items))
	}
	if feedResp2.NextCursor != nil {
		t.Fatalf("expected null next_cursor on last page, got %v", *feedResp2.NextCursor)
	}
}

func TestHandler_Integration_DatabaseOutageReturns503(t *testing.T) {
	_, pool, router := freshForumTestEnv(t)
	pool.Close()

	for _, path := range []string{
		"/v1/forum/posts",
		"/v1/forum/posts/00000000-0000-4000-8000-000000000001",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503 during DB outage, got %d (%s)", path, rec.Code, rec.Body.String())
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.unavailable" {
			t.Errorf("%s: expected forum.unavailable, got %s", path, code)
		}
	}
}
