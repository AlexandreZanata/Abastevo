package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/go-chi/chi/v5"
)

// for09UnifiedStore embeds mockAdminStore and implements CategoryReader & PostReader
// to verify the end-to-end editorial flow and rollback safety.
type for09UnifiedStore struct {
	*mockAdminStore
	categories  []domain.CategoryItem
	taxonomyVer int
}

func newFOR09UnifiedStore() *for09UnifiedStore {
	adminStore := newMockAdminStore()
	officialSource := &domain.Source{
		ID:             "00000000-0000-0000-0000-000000000001",
		Code:           "abastevo",
		Name:           "Abastevo",
		Kind:           domain.SourceKindAbastevo,
		OriginURL:      "https://abastevo.com.br",
		RightsStatus:   domain.RightsApproved,
		TitleAllowed:   true,
		ExcerptAllowed: true,
		ImageAllowed:   false,
		Attribution:    "Redação Abastevo",
	}
	adminStore.sources[officialSource.ID] = officialSource

	cats := []domain.CategoryItem{
		{Code: domain.CategoryFuels, Name: "Combustíveis", Description: "Preços e tendências de combustíveis"},
		{Code: domain.CategoryRoadsMobility, Name: "Estradas", Description: "Infraestrutura e mobilidade"},
	}

	return &for09UnifiedStore{
		mockAdminStore: adminStore,
		categories:     cats,
		taxonomyVer:    1,
	}
}

func (s *for09UnifiedStore) ListCategories(ctx context.Context) ([]domain.CategoryItem, int, error) {
	return s.categories, s.taxonomyVer, nil
}

func (s *for09UnifiedStore) ListPublicPosts(ctx context.Context, query application.ListPublicPostsQuery) (domain.FeedPage, error) {
	items := make([]domain.PostSummary, 0)
	for _, p := range s.posts {
		if p.PublicationState == domain.StatePublished && p.Visibility == domain.VisibilityVisible {
			if query.Category != nil && p.Category != *query.Category {
				continue
			}
			if p.Source == nil {
				p.Source = s.sources[p.SourceID]
			}
			sum, err := p.ToSummary()
			if err == nil && sum != nil {
				items = append(items, *sum)
			}
		}
	}
	return domain.FeedPage{
		Items:      items,
		NextCursor: nil,
	}, nil
}

func (s *for09UnifiedStore) GetFeedSnapshot(ctx context.Context) (string, error) {
	return "rev-1", nil
}

// TestFOR09_EditorialChainAndSafeRollback proves:
// 1. Full Admin editorial lifecycle (create draft -> publish -> hide -> unhide).
// 2. Public API privacy invariants (draft and hidden posts return 404 and are omitted from feed).
// 3. Safe source link attribution preservation (HTTPS canonical URL returned in public detail).
// 4. Default-off runtime gates and non-destructive feature rollback (reads/admin can be disabled without data loss).
func TestFOR09_EditorialChainAndSafeRollback(t *testing.T) {
	store := newFOR09UnifiedStore()
	runner := newMockRunner()
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	idCounter := 0

	cfg := domain.DefaultConfig()
	cfg.ReadsEnabled = true
	cfg.AdminEnabled = true

	origin := "https://admin.example.com"
	rawCSRF := "test-csrf-token-abc"
	salt := "test-salt"
	hash := admindomain.SaltedHash(salt, rawCSRF)

	session := adminapp.Session{
		ID:             "sess-for09",
		KeyFingerprint: "fp-123456",
		CSRFSalt:       salt,
		CSRFHash:       hash,
		CreatedAt:      fixedTime,
		ExpiresAt:      fixedTime.Add(time.Hour),
	}
	authStore := &fakeAdminAuthStore{session: session}

	adminHandler := &adminhttp.Handler{
		Origin:       origin,
		ClientIP:     func(r *http.Request) (string, error) { return "127.0.0.1", nil },
		AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		Service: &adminapp.Service{
			Store: authStore,
			Keys: []admindomain.ConfiguredKey{
				{Hash: strings.Repeat("a", 64), Fingerprint: "fp-123456"},
			},
		},
	}

	adminSvc := &application.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
		NewID: func() (string, error) {
			idCounter++
			return "99999999-9999-4999-8999-000000000001", nil
		},
	}

	publicSvc := application.NewService(cfg, store, store, mockClock{now: fixedTime})

	forumAdminH := NewAdminHandler(adminSvc, adminHandler)
	publicH := NewHandler(publicSvc, []byte("for09-test-cursor-secret-32-byte"))

	r := chi.NewRouter()
	r.Group(func(g chi.Router) {
		g.Use(adminHandler.AdminMiddleware()...)
		forumAdminH.RegisterRoutes(g)
	})
	publicH.RegisterRoutes(r)

	newAdminReq := func(method, path string, body any, key string) *http.Request {
		var bodyReader *bytes.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(raw)
		} else {
			bodyReader = bytes.NewReader([]byte{})
		}
		req := httptest.NewRequest(method, path, bodyReader)
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Admin-CSRF", rawCSRF)
		req.Header.Set("Idempotency-Key", key)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.AddCookie(&http.Cookie{
			Name:  "__Host-abv_admin",
			Value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		})
		return req
	}

	// Step 1: Admin creates draft post
	createReq := map[string]any{
		"kind":          "EDITORIAL",
		"category":      "FUELS",
		"title":         "Paridade de Importação e Preços dos Combustíveis",
		"excerpt":       "Análise editorial completa sobre a formação de preços no Brasil.",
		"body_text":     "Texto integral do artigo explicando PPI, margens de distribuição e tributos estaduais.",
		"source_id":     "00000000-0000-0000-0000-000000000001",
		"canonical_url": "https://abastevo.com.br/artigos/paridade-de-importacao",
		"reason":        "Criação de novo artigo editorial sobre mercado de combustíveis",
	}

	req := newAdminReq(http.MethodPost, "/v1/admin/forum/posts", createReq, "11111111-1111-4111-8111-111111111111")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create draft: expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var created domain.Post
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created draft: %v", err)
	}
	postID := created.ID

	// Step 2: Verify draft is NOT visible publicly (Privacy invariant)
	// 2a. Public feed must be empty
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public feed: expected 200, got %d", rec.Code)
	}
	var feed FeedResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &feed)
	if len(feed.Items) != 0 {
		t.Fatalf("draft leaked into public feed: expected 0 items, got %d", len(feed.Items))
	}

	// 2b. Public detail must return 404 Not Found (no leakage of unpublished content)
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+postID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public detail for draft: expected 404, got %d", rec.Code)
	}

	// Step 3: Admin publishes post
	pubReq := map[string]any{
		"reason": "Artigo editorial revisado e aprovado pela equipe técnica",
	}
	req = newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/publish", pubReq, "22222222-2222-4222-8222-222222222222")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin publish: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Step 4: Verify post is now visible in public feed and detail
	// 4a. Feed
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=FUELS", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public feed after publish: expected 200, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &feed)
	if len(feed.Items) != 1 || feed.Items[0].ID != postID {
		t.Fatalf("expected post in feed: %+v", feed.Items)
	}
	if feed.Items[0].Title != createReq["title"] {
		t.Errorf("expected title %q, got %q", createReq["title"], feed.Items[0].Title)
	}

	// 4b. Detail with full body and safe source
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+postID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public detail: expected 200, got %d", rec.Code)
	}
	var detail PostDetailResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if detail.Post.ID != postID || detail.Post.BodyText == nil || *detail.Post.BodyText != createReq["body_text"] {
		t.Fatalf("unexpected detail: %+v", detail.Post)
	}
	if detail.Post.Source == nil || detail.Post.Source.URL != "https://abastevo.com.br" {
		t.Fatalf("unexpected source attribution: %+v", detail.Post.Source)
	}

	// Step 5: Admin hides post (takedown/retraction)
	hideReq := map[string]any{
		"reason": "Necessidade de revisão urgente por errata técnica identificada",
	}
	req = newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/hide", hideReq, "33333333-3333-4333-8333-333333333333")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin hide: expected 200, got %d", rec.Code)
	}

	// Step 6: Verify public feed and detail immediate withdrawal (Privacy & moderation)
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &feed)
	if len(feed.Items) != 0 {
		t.Fatalf("hidden post remained in feed: %+v", feed.Items)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+postID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("hidden detail: expected 404, got %d", rec.Code)
	}

	// Step 7: Admin unhides post
	unhideReq := map[string]any{
		"reason": "Revisão e errata técnica conferida e aprovada pela chefia",
	}
	req = newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/unhide", unhideReq, "44444444-4444-4444-8444-444444444444")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin unhide: expected 200, got %d", rec.Code)
	}

	// Step 8: Safe Rollback Verification (Default-Off switches)
	cfgDisabled := domain.DefaultConfig()
	cfgDisabled.ReadsEnabled = false
	cfgDisabled.AdminEnabled = false

	disabledPublicSvc := application.NewService(cfgDisabled, store, store, mockClock{now: fixedTime})
	disabledAdminSvc := &application.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfgDisabled,
		Clock:  func() time.Time { return fixedTime },
	}

	rDisabled := chi.NewRouter()
	rDisabled.Group(func(g chi.Router) {
		g.Use(adminHandler.AdminMiddleware()...)
		NewAdminHandler(disabledAdminSvc, adminHandler).RegisterRoutes(g)
	})
	NewHandler(disabledPublicSvc, []byte("for09-test-cursor-secret-32-byte")).RegisterRoutes(rDisabled)

	// Public feed returns 404 with code "forum.disabled" when feature is turned off
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
	rec = httptest.NewRecorder()
	rDisabled.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled public feed: expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "forum.disabled") {
		t.Fatalf("disabled public feed: expected forum.disabled, got %s", rec.Body.String())
	}

	// Public categories return 404 with code "forum.disabled"
	req = httptest.NewRequest(http.MethodGet, "/v1/forum/categories", nil)
	rec = httptest.NewRecorder()
	rDisabled.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled public categories: expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "forum.disabled") {
		t.Fatalf("disabled public categories: expected forum.disabled, got %s", rec.Body.String())
	}

	// Admin endpoints return 404 with code "forum.disabled"
	req = newAdminReq(http.MethodGet, "/v1/admin/forum/posts", nil, "55555555-5555-4555-8555-555555555555")
	rec = httptest.NewRecorder()
	rDisabled.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled admin list: expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "forum.disabled") {
		t.Fatalf("disabled admin list: expected forum.disabled, got %s", rec.Body.String())
	}

	// Verify rollback leaves existing store data intact (no data corruption or destructive changes)
	p, err := store.GetPostByID(context.Background(), postID)
	if err != nil || p == nil {
		t.Fatalf("post was lost during rollback: %v", err)
	}
	if p.Title != createReq["title"] {
		t.Errorf("stored post title corrupted: got %q", p.Title)
	}
}
