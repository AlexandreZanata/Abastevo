package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type mockCategoryReader struct {
	items   []domain.CategoryItem
	version int
	err     error
}

func (m *mockCategoryReader) ListCategories(ctx context.Context) ([]domain.CategoryItem, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.items, m.version, nil
}

type mockPostReader struct {
	posts    map[string]*domain.Post
	feedPage domain.FeedPage
	snapshot string
	getErr   error
	feedErr  error
}

func (m *mockPostReader) GetPostByID(ctx context.Context, id string) (*domain.Post, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	p, ok := m.posts[id]
	if !ok {
		return nil, nil
	}
	return p, nil
}

func (m *mockPostReader) ListPublicPosts(ctx context.Context, query application.ListPublicPostsQuery) (domain.FeedPage, error) {
	if m.feedErr != nil {
		return domain.FeedPage{}, m.feedErr
	}
	return m.feedPage, nil
}

func (m *mockPostReader) GetFeedSnapshot(ctx context.Context) (string, error) {
	return m.snapshot, nil
}

type mockClock struct {
	now time.Time
}

func (m mockClock) Now() time.Time {
	return m.now
}

func setupTestRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return r
}

func parseErrorEnvelope(t *testing.T, body []byte) (string, string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string           `json:"code"`
			Message string           `json:"message"`
			Details []httpapi.Detail `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to parse error envelope: %v (raw: %s)", err, string(body))
	}
	return env.Error.Code, env.Error.Message
}

func TestHandler_Categories(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	catReader := &mockCategoryReader{
		items:   domain.DefaultCategories(),
		version: 1,
	}
	postReader := &mockPostReader{posts: make(map[string]*domain.Post)}

	t.Run("anonymous_success", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/categories", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("expected Content-Type: application/json, got %q", ct)
		}

		var resp CategoriesResponseDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal categories response: %v", err)
		}
		if resp.TaxonomyVersion != 1 {
			t.Errorf("expected taxonomy_version 1, got %d", resp.TaxonomyVersion)
		}
		if len(resp.Items) != 7 {
			t.Errorf("expected 7 categories, got %d", len(resp.Items))
		}
		if resp.Items[0].Code != "FUELS" || resp.Items[0].Name != "Combustíveis" {
			t.Errorf("unexpected first category: %+v", resp.Items[0])
		}
	})

	t.Run("disabled_switch_returns_404", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = false
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/categories", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.disabled" {
			t.Errorf("expected code forum.disabled, got %s", code)
		}
	})

	t.Run("database_error_returns_503", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		failingReader := &mockCategoryReader{err: errors.New("db connection failure")}
		svc := application.NewService(cfg, failingReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/categories", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.unavailable" {
			t.Errorf("expected code forum.unavailable, got %s", code)
		}
	})
}

func TestHandler_Feed(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	catReader := &mockCategoryReader{items: domain.DefaultCategories(), version: 1}

	sampleSummary := domain.PostSummary{
		ID:       "00000000-0000-0000-0000-000000000002",
		Kind:     domain.KindEditorial,
		Category: domain.CategoryFuels,
		Title:    "Preços em queda no Brasil",
		Source: &domain.SourceAttribution{
			Name: "Abastevo Editorial",
			URL:  "https://abastevo.com.br",
		},
		FirstPublishedAt: now,
		UpdatedAt:        now,
		Revision:         1,
	}

	t.Run("default_feed_anonymous_success", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		postReader := &mockPostReader{
			feedPage: domain.FeedPage{
				Items:      []domain.PostSummary{sampleSummary},
				NextCursor: nil,
				Snapshot:   "rev-1",
				FetchedAt:  now,
			},
		}
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}

		var resp FeedResponseDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal feed response: %v", err)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Items))
		}
		if resp.Items[0].Title != "Preços em queda no Brasil" {
			t.Errorf("expected title match, got %s", resp.Items[0].Title)
		}
		if resp.NextCursor != nil {
			t.Errorf("expected null next_cursor, got %v", *resp.NextCursor)
		}
		if resp.Snapshot != "rev-1" {
			t.Errorf("expected snapshot rev-1, got %s", resp.Snapshot)
		}
	})

	t.Run("limit_validation", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		postReader := &mockPostReader{
			feedPage: domain.FeedPage{Items: nil, Snapshot: "rev-1", FetchedAt: now},
		}
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		for _, badLimit := range []string{"0", "51", "-1", "abc", "100"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?limit="+badLimit, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("limit %q: expected 400, got %d", badLimit, rec.Code)
			}
			code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
			if code != "forum.bad-limit" {
				t.Errorf("limit %q: expected code forum.bad-limit, got %s", badLimit, code)
			}
		}

		for _, goodLimit := range []string{"1", "20", "50"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?limit="+goodLimit, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("limit %q: expected 200, got %d", goodLimit, rec.Code)
			}
		}
	})

	t.Run("category_and_kind_filter_validation", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		postReader := &mockPostReader{
			feedPage: domain.FeedPage{Items: nil, Snapshot: "rev-1", FetchedAt: now},
		}
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		// Invalid category
		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=INVALID_CAT", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for bad category, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.bad-filter" {
			t.Errorf("expected forum.bad-filter, got %s", code)
		}

		// Invalid kind
		req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts?kind=INVALID_KIND", nil)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for bad kind, got %d", rec.Code)
		}
		code, _ = parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.bad-filter" {
			t.Errorf("expected forum.bad-filter, got %s", code)
		}

		// Valid filters
		req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=ELECTRIC&kind=NEWS", nil)
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for valid filters, got %d", rec.Code)
		}
	})

	t.Run("hmac_cursor_roundtrip_tamper_and_expiry", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		internalCursor := "10:00000000-0000-0000-0000-000000000002"
		postReader := &mockPostReader{
			feedPage: domain.FeedPage{
				Items:      []domain.PostSummary{sampleSummary},
				NextCursor: &internalCursor,
				Snapshot:   "rev-1",
				FetchedAt:  now,
			},
		}
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		// 1. Get first page with category FUELS and limit 10
		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=10", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp FeedResponseDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			t.Fatalf("expected signed next_cursor, got nil")
		}
		mintedCursor := *resp.NextCursor

		// 2. Fetch page 2 using exact same filters and minted cursor -> succeeds
		req2 := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=10&cursor="+mintedCursor, nil)
		rec2 := httptest.NewRecorder()
		r.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200 on page 2, got %d", rec2.Code)
		}

		// 3. Filter tamper: same cursor but category=ELECTRIC -> 400 forum.bad-cursor
		reqTamperCat := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=ELECTRIC&limit=10&cursor="+mintedCursor, nil)
		recTamperCat := httptest.NewRecorder()
		r.ServeHTTP(recTamperCat, reqTamperCat)
		if recTamperCat.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on filter tamper, got %d", recTamperCat.Code)
		}
		code, _ := parseErrorEnvelope(t, recTamperCat.Body.Bytes())
		if code != "forum.bad-cursor" {
			t.Errorf("expected code forum.bad-cursor, got %s", code)
		}

		// 4. Limit tamper: same cursor but limit=20 -> 400 forum.bad-cursor
		reqTamperLimit := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=20&cursor="+mintedCursor, nil)
		recTamperLimit := httptest.NewRecorder()
		r.ServeHTTP(recTamperLimit, reqTamperLimit)
		if recTamperLimit.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on limit tamper, got %d", recTamperLimit.Code)
		}

		// 5. Payload tamper: altered cursor token bytes -> 400 forum.bad-cursor
		corruptedCursor := mintedCursor[:len(mintedCursor)-4] + "AAAA"
		reqCorrupt := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=10&cursor="+corruptedCursor, nil)
		recCorrupt := httptest.NewRecorder()
		r.ServeHTTP(recCorrupt, reqCorrupt)
		if recCorrupt.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on corrupted cursor, got %d", recCorrupt.Code)
		}

		// 6. Oversized cursor (> 512 chars) -> 400 forum.bad-cursor
		oversized := strings.Repeat("A", 513)
		reqOversize := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=10&cursor="+oversized, nil)
		recOversize := httptest.NewRecorder()
		r.ServeHTTP(recOversize, reqOversize)
		if recOversize.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on oversized cursor, got %d", recOversize.Code)
		}

		// 7. Expired cursor: minted with timestamp older than CursorTTL (15m)
		fh := httpapi.FilterHash("forum_feed", "FUELS", "", "10")
		expiredToken := httpapi.Seal(secret, fh, internalCursor, now.Add(-16*time.Minute))
		reqExpired := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS&limit=10&cursor="+expiredToken, nil)
		recExpired := httptest.NewRecorder()
		r.ServeHTTP(recExpired, reqExpired)
		if recExpired.Code != http.StatusBadRequest {
			t.Errorf("expected 400 on expired cursor, got %d", recExpired.Code)
		}
	})

	t.Run("feed_disabled_switch_returns_404", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = false
		postReader := &mockPostReader{feedPage: domain.FeedPage{}}
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.disabled" {
			t.Errorf("expected forum.disabled, got %s", code)
		}
	})

	t.Run("feed_database_failure_returns_503", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		postReader := &mockPostReader{feedErr: errors.New("timeout querying postgres")}
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.unavailable" {
			t.Errorf("expected forum.unavailable, got %s", code)
		}
	})
}

func TestHandler_Detail(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	catReader := &mockCategoryReader{items: domain.DefaultCategories(), version: 1}

	publishedID := "00000000-0000-0000-0000-000000000002"
	draftID := "00000000-0000-0000-0000-000000000003"
	hiddenID := "00000000-0000-0000-0000-000000000004"

	publishedPost := &domain.Post{
		ID:                  publishedID,
		Kind:                domain.KindEditorial,
		Category:            domain.CategoryFuels,
		Title:               "Guia dos Combustíveis",
		Excerpt:             "Resumo introdutório",
		BodyText:            "Corpo completo do artigo editorial",
		SourceID:            "00000000-0000-0000-0000-000000000001",
		Source:              &domain.Source{Name: "Abastevo Editorial", OriginURL: "https://abastevo.com.br"},
		AuthorAccountID:     "00000000-0000-0000-0000-000000000010",
		FirstPublishedAt:    &now,
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 1,
		PublicationState:    domain.StatePublished,
		Visibility:          domain.VisibilityVisible,
		Revision:            1,
		CommentsEnabled:     true,
	}

	draftPost := &domain.Post{
		ID:                  draftID,
		Kind:                domain.KindEditorial,
		Category:            domain.CategoryFuels,
		Title:               "Rascunho não publicado",
		PublicationState:    domain.StateDraft,
		Visibility:          domain.VisibilityVisible,
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 0,
		Revision:            1,
	}

	hiddenPost := &domain.Post{
		ID:                  hiddenID,
		Kind:                domain.KindEditorial,
		Category:            domain.CategoryFuels,
		Title:               "Artigo Ocultado por Moderação",
		PublicationState:    domain.StatePublished,
		Visibility:          domain.VisibilityHidden,
		FirstPublishedAt:    &now,
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 2,
		Revision:            1,
	}

	postReader := &mockPostReader{
		posts: map[string]*domain.Post{
			publishedID: publishedPost,
			draftID:     draftPost,
			hiddenID:    hiddenPost,
		},
	}

	t.Run("published_detail_anonymous_success", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+publishedID, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store, got %q", cc)
		}

		var resp PostDetailResponseDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal post detail response: %v", err)
		}
		if resp.Post.ID != publishedID {
			t.Errorf("expected id %s, got %s", publishedID, resp.Post.ID)
		}
		if resp.Post.Title != "Guia dos Combustíveis" {
			t.Errorf("expected title match, got %s", resp.Post.Title)
		}
		if resp.Post.BodyText == nil || *resp.Post.BodyText != "Corpo completo do artigo editorial" {
			t.Errorf("expected body text match, got %v", resp.Post.BodyText)
		}
		if !resp.Post.CommentsEnabled {
			t.Errorf("expected comments_enabled true")
		}
		if resp.Post.Source == nil || resp.Post.Source.Name != "Abastevo Editorial" {
			t.Errorf("expected source match, got %v", resp.Post.Source)
		}
	})

	t.Run("invalid_uuid_returns_400", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		for _, badID := range []string{"not-a-uuid", "123", "00000000-0000-0000-0000-00000000000Z", "000000000000000000000000000000000000"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+badID, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("id %q: expected 400, got %d", badID, rec.Code)
			}
			code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
			if code != "forum.bad-id" {
				t.Errorf("id %q: expected code forum.bad-id, got %s", badID, code)
			}
		}
	})

	t.Run("non_existent_post_returns_404", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/00000000-0000-0000-0000-000000000999", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.not-found" {
			t.Errorf("expected forum.not-found, got %s", code)
		}
	})

	t.Run("draft_post_returns_404", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+draftID, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for draft post, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.not-found" {
			t.Errorf("expected forum.not-found, got %s", code)
		}
	})

	t.Run("hidden_post_returns_404", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+hiddenID, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for hidden post, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.not-found" {
			t.Errorf("expected forum.not-found, got %s", code)
		}
	})

	t.Run("detail_disabled_switch_returns_404", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = false
		svc := application.NewService(cfg, catReader, postReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+publishedID, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.disabled" {
			t.Errorf("expected forum.disabled, got %s", code)
		}
	})

	t.Run("detail_database_failure_returns_503", func(t *testing.T) {
		cfg := domain.DefaultConfig()
		cfg.ReadsEnabled = true
		failingReader := &mockPostReader{getErr: errors.New("timeout reading post")}
		svc := application.NewService(cfg, catReader, failingReader, mockClock{now: now})
		h := NewHandler(svc, secret)
		r := setupTestRouter(h)

		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+publishedID, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		code, _ := parseErrorEnvelope(t, rec.Body.Bytes())
		if code != "forum.unavailable" {
			t.Errorf("expected forum.unavailable, got %s", code)
		}
	})
}
