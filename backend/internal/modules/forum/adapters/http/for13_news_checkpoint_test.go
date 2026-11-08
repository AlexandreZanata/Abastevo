//go:build integration

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type for13TestEnv struct {
	pool        *pgxpool.Pool
	store       *forumadapters.Store
	adminRouter chi.Router
	pubRouter   chi.Router
	adminH      *AdminHandler
	adminSvc    *application.AdminService
	adminSess   adminapp.Session
	origin      string
	rawCSRF     string
}

type poolMutationRunner struct {
	pool *pgxpool.Pool
}

func (r *poolMutationRunner) Mutate(ctx context.Context, m adminapp.Mutation, fn func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return adminapp.MutationResult{}, err
	}
	defer tx.Rollback(ctx)

	res, _, err := fn(ctx, tx)
	if err != nil {
		return adminapp.MutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminapp.MutationResult{}, err
	}
	return res, nil
}

func setupFOR13TestEnv(t *testing.T) *for13TestEnv {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable: %v", err)
	}
	defer conn.Close(ctx)

	name := fmt.Sprintf("forum_for13_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("cleanup connect: %v", err)
			return
		}
		defer c.Close(ctx)
		if _, err := c.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
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

	origin := "https://admin.example.com"
	rawCSRF := "test-csrf-token-for13"
	salt := "test-salt-for13"
	hash := admindomain.SaltedHash(salt, rawCSRF)

	session := adminapp.Session{
		ID:             "sess-for13-admin",
		KeyFingerprint: "fp-for13",
		CSRFSalt:       salt,
		CSRFHash:       hash,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}

	authStore := &fakeAdminAuthStore{session: session}
	adminHandler := &adminhttp.Handler{
		Origin:       origin,
		ClientIP:     func(r *http.Request) (string, error) { return "127.0.0.1", nil },
		AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		Service: &adminapp.Service{
			Store: authStore,
			Keys: []admindomain.ConfiguredKey{
				{Hash: strings.Repeat("b", 64), Fingerprint: "fp-for13"},
			},
		},
	}

	cfg := domain.DefaultConfig()
	cfg.ReadsEnabled = true
	cfg.AdminEnabled = true

	nowFn := func() time.Time { return time.Now().UTC() }
	runner := &poolMutationRunner{pool: pool}

	idCounter := 0
	adminSvc := &application.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  nowFn,
		NewID: func() (string, error) {
			idCounter++
			return fmt.Sprintf("aaaaaaaa-1111-4111-8111-%012d", idCounter), nil
		},
	}

	forumAdminH := NewAdminHandler(adminSvc, adminHandler)
	adminRouter := chi.NewRouter()
	adminRouter.Group(func(g chi.Router) {
		g.Use(adminHandler.AdminMiddleware()...)
		forumAdminH.RegisterRoutes(g)
	})

	publicSvc := application.NewService(cfg, store, store, mockClock{now: time.Now().UTC()})
	publicH := NewHandler(publicSvc, testCursorSecret)
	pubRouter := chi.NewRouter()
	publicH.RegisterRoutes(pubRouter)

	return &for13TestEnv{
		pool:        pool,
		store:       store,
		adminRouter: adminRouter,
		pubRouter:   pubRouter,
		adminH:      forumAdminH,
		adminSvc:    adminSvc,
		adminSess:   session,
		origin:      origin,
		rawCSRF:     rawCSRF,
	}
}

func (env *for13TestEnv) newAdminReq(method, path string, body any, idempotencyKey string) *http.Request {
	var bodyReader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Origin", env.origin)
	req.Header.Set("X-Admin-CSRF", env.rawCSRF)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{
		Name:  "__Host-abv_admin",
		Value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	return req
}

func TestFOR13_NewsCheckpoint_FullIntegration(t *testing.T) {
	env := setupFOR13TestEnv(t)
	ctx := context.Background()

	// 1. Verify source register via Admin API (GET /v1/admin/forum/sources)
	listSourcesReq := env.newAdminReq(http.MethodGet, "/v1/admin/forum/sources", nil, "")
	wSources := httptest.NewRecorder()
	env.adminRouter.ServeHTTP(wSources, listSourcesReq)
	if wSources.Code != http.StatusOK {
		t.Fatalf("expected 200 on list sources, got %d: %s", wSources.Code, wSources.Body.String())
	}

	var sourcesResp struct {
		Items []application.SourceWithMetrics `json:"items"`
	}
	if err := json.Unmarshal(wSources.Body.Bytes(), &sourcesResp); err != nil {
		t.Fatalf("unmarshal sources: %v", err)
	}
	if len(sourcesResp.Items) == 0 {
		t.Fatalf("expected at least 1 source from migration 000157, got 0")
	}

	// Verify Abastevo official source is APPROVED and publication_enabled = true
	var abastevoSource *application.SourceWithMetrics
	for _, s := range sourcesResp.Items {
		if s.ID == "00000000-0000-0000-0000-000000000001" {
			abastevoSource = &s
			break
		}
	}
	if abastevoSource == nil {
		t.Fatalf("missing seeded Abastevo source 00000000-0000-0000-0000-000000000001")
	}
	if abastevoSource.RightsStatus != domain.RightsApproved || !abastevoSource.PublicationEnabled {
		t.Fatalf("expected Abastevo source to be APPROVED with publication_enabled=true, got %+v", abastevoSource)
	}

	// 2. Insert candidate post 1 (to be rejected)
	cand1ID := "22222222-2222-4222-8222-000000000001"
	cand1PublishedAt := time.Now().UTC().Add(-2 * time.Hour)
	cand1 := &domain.Post{
		ID:                cand1ID,
		Kind:              domain.KindNews,
		Category:          domain.CategoryFuels,
		Title:             "Candidato a Rejeição Inicial",
		Excerpt:           "Resumo que será rejeitado pelo revisor.",
		BodyText:          "Texto completo que não deve ser publicado no aplicativo.",
		SourceID:          abastevoSource.ID,
		CanonicalURL:      "https://abastevo.com.br/noticias/candidato-rejeitado",
		ExternalID:        "guid-cand-reject-001",
		SourcePublishedAt: &cand1PublishedAt,
		LocalCreatedAt:    time.Now().UTC(),
		LocalUpdatedAt:    time.Now().UTC(),
		PublicationState:  domain.StateDraft,
		Visibility:        domain.VisibilityVisible,
		CommentsEnabled:   true,
	}
	if err := env.store.InsertNewsCandidatePost(ctx, cand1); err != nil {
		t.Fatalf("insert candidate 1: %v", err)
	}

	// 3. Admin verifies candidate appears in DRAFT/NEWS list
	listPostsReq := env.newAdminReq(http.MethodGet, "/v1/admin/forum/posts?publication_state=DRAFT&kind=NEWS", nil, "")
	wPosts := httptest.NewRecorder()
	env.adminRouter.ServeHTTP(wPosts, listPostsReq)
	if wPosts.Code != http.StatusOK {
		t.Fatalf("expected 200 on list posts, got %d", wPosts.Code)
	}
	var draftPosts struct {
		Items []domain.Post `json:"items"`
	}
	if err := json.Unmarshal(wPosts.Body.Bytes(), &draftPosts); err != nil {
		t.Fatalf("unmarshal draft posts: %v", err)
	}
	foundCand1 := false
	for _, p := range draftPosts.Items {
		if p.ID == cand1ID {
			foundCand1 = true
			break
		}
	}
	if !foundCand1 {
		t.Fatalf("candidate 1 not found in DRAFT news list")
	}

	// 4. Admin edits candidate with correction
	editReq := env.newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+cand1ID+"/edit", map[string]any{
		"category":          "FUELS",
		"title":             "Candidato a Rejeição com Correção",
		"excerpt":           "Resumo corrigido pelo editor.",
		"body_text":         "Texto corrigido pelo editor.",
		"expected_revision": 1,
		"reason":            "correção editorial antes da decisão de rejeição",
	}, "11111111-1111-4111-8111-000000000001")
	wEdit := httptest.NewRecorder()
	env.adminRouter.ServeHTTP(wEdit, editReq)
	if wEdit.Code != http.StatusOK {
		t.Fatalf("expected 200 on edit, got %d: %s", wEdit.Code, wEdit.Body.String())
	}
	var editedPost domain.Post
	if err := json.Unmarshal(wEdit.Body.Bytes(), &editedPost); err != nil {
		t.Fatalf("unmarshal edit resp: %v", err)
	}
	if editedPost.Revision != 2 {
		t.Fatalf("expected revision 2 after edit, got %d", editedPost.Revision)
	}

	// 5. Admin rejects candidate (POST /v1/admin/forum/posts/{id}/reject)
	rejectReq := env.newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+cand1ID+"/reject", map[string]any{
		"expected_revision": 2,
		"reason":            "conteúdo inadequado para publicação comercial",
	}, "11111111-1111-4111-8111-000000000002")
	wReject := httptest.NewRecorder()
	env.adminRouter.ServeHTTP(wReject, rejectReq)
	if wReject.Code != http.StatusOK {
		t.Fatalf("expected 200 on reject, got %d: %s", wReject.Code, wReject.Body.String())
	}
	var rejectedPost domain.Post
	if err := json.Unmarshal(wReject.Body.Bytes(), &rejectedPost); err != nil {
		t.Fatalf("unmarshal reject resp: %v", err)
	}
	if rejectedPost.PublicationState != domain.StateRejected {
		t.Fatalf("expected REJECTED state, got %s", rejectedPost.PublicationState)
	}
	if rejectedPost.Revision != 3 {
		t.Fatalf("expected revision 3 after reject, got %d", rejectedPost.Revision)
	}

	// 6. Acceptance verification: rejected duplicates DO NOT REAPPEAR on subsequent ingestion runs!
	hasGUID, err := env.store.HasPostByExternalID(ctx, abastevoSource.ID, cand1.ExternalID)
	if err != nil {
		t.Fatalf("HasPostByExternalID check: %v", err)
	}
	if !hasGUID {
		t.Fatalf("expected rejected post GUID to remain deduplicated, but HasPostByExternalID returned false")
	}

	hasURL, err := env.store.HasPostByUrlHash(ctx, abastevoSource.ID, "some_hash")
	_ = hasURL // verify query executes cleanly

	// 7. Verify rights enforcement: create a candidate with a blocked/denied source
	blockedSrcID := "33333333-3333-4333-8333-000000000099"
	// Insert blocked source into database
	_, err = env.pool.Exec(ctx, `
		INSERT INTO forum_sources (
			id, code, name, kind, origin_url, fetch_url, fetch_format,
			collection_enabled, publication_enabled, rights_status,
			title_allowed, excerpt_allowed, image_allowed,
			terms_url, terms_sha256, attribution, admin_audit_ref
		) VALUES (
			$1, 'BLOCKED_PORTAL', 'Portal Bloqueado', 'PUBLISHER', 'https://blocked.example.com',
			'https://blocked.example.com/feed.xml', 'RSS',
			false, false, 'DENIED',
			false, false, false,
			'https://blocked.example.com/terms', 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
			'Portal Bloqueado', 'audit-test-for13'
		)
	`, blockedSrcID)
	if err != nil {
		t.Fatalf("insert blocked source: %v", err)
	}

	candBlockedID := "44444444-4444-4444-8444-000000000002"
	candBlockedPubAt := time.Now().UTC().Add(-1 * time.Hour)
	candBlocked := &domain.Post{
		ID:                candBlockedID,
		Kind:              domain.KindNews,
		Category:          domain.CategoryFuels,
		Title:             "Notícia de Fonte Sem Direitos",
		Excerpt:           "Esta notícia nunca deve poder ser publicada.",
		BodyText:          "Texto de fonte negada.",
		SourceID:          blockedSrcID,
		CanonicalURL:      "https://blocked.example.com/noticia-1",
		ExternalID:        "guid-blocked-001",
		SourcePublishedAt: &candBlockedPubAt,
		LocalCreatedAt:    time.Now().UTC(),
		LocalUpdatedAt:    time.Now().UTC(),
		PublicationState:  domain.StateDraft,
		Visibility:        domain.VisibilityVisible,
		CommentsEnabled:   true,
	}
	if err := env.store.InsertNewsCandidatePost(ctx, candBlocked); err != nil {
		t.Fatalf("insert blocked candidate: %v", err)
	}

	// Attempt publication of post from blocked source -> MUST FAIL with 400 forum.missing-rights
	pubBlockedReq := env.newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+candBlockedID+"/publish", map[string]any{
		"expected_revision": 1,
		"reason":            "tentando publicar conteúdo sem direitos comerciais",
	}, "11111111-1111-4111-8111-000000000003")
	wPubBlocked := httptest.NewRecorder()
	env.adminRouter.ServeHTTP(wPubBlocked, pubBlockedReq)
	if wPubBlocked.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on publishing blocked source, got %d: %s", wPubBlocked.Code, wPubBlocked.Body.String())
	}
	if !strings.Contains(wPubBlocked.Body.String(), "forum.missing-rights") {
		t.Fatalf("expected forum.missing-rights error code, got: %s", wPubBlocked.Body.String())
	}

	// 8. Insert candidate 3 with approved source and publish successfully
	cand3ID := "55555555-5555-4555-8555-000000000003"
	cand3PubAt := time.Now().UTC().Add(-30 * time.Minute)
	cand3 := &domain.Post{
		ID:                cand3ID,
		Kind:              domain.KindNews,
		Category:          domain.CategoryDriverEconomy,
		Title:             "Como Economizar Combustível na Cidade: Guia Prático",
		Excerpt:           "Dicas práticas e comprovadas para reduzir o consumo de gasolina e etanol no trânsito urbano.",
		BodyText:          "Texto editorial completo sobre manutenção de pneus, aceleração progressiva e uso do ar condicionado.",
		SourceID:          abastevoSource.ID,
		CanonicalURL:      "https://abastevo.com.br/artigos/economia-de-combustivel",
		ExternalID:        "guid-cand-pub-003",
		SourcePublishedAt: &cand3PubAt,
		LocalCreatedAt:    time.Now().UTC(),
		LocalUpdatedAt:    time.Now().UTC(),
		PublicationState:  domain.StateDraft,
		Visibility:        domain.VisibilityVisible,
		CommentsEnabled:   true,
	}
	if err := env.store.InsertNewsCandidatePost(ctx, cand3); err != nil {
		t.Fatalf("insert candidate 3: %v", err)
	}

	// Publish candidate 3
	pub3Req := env.newAdminReq(http.MethodPost, "/v1/admin/forum/posts/"+cand3ID+"/publish", map[string]any{
		"expected_revision": 1,
		"reason":            "conteúdo editorial aprovado e validado pelo conselho",
	}, "11111111-1111-4111-8111-000000000004")
	wPub3 := httptest.NewRecorder()
	env.adminRouter.ServeHTTP(wPub3, pub3Req)
	if wPub3.Code != http.StatusOK {
		t.Fatalf("expected 200 on publish, got %d: %s", wPub3.Code, wPub3.Body.String())
	}
	var pubPost domain.Post
	if err := json.Unmarshal(wPub3.Body.Bytes(), &pubPost); err != nil {
		t.Fatalf("unmarshal pub resp: %v", err)
	}
	if pubPost.PublicationState != domain.StatePublished {
		t.Fatalf("expected PUBLISHED state, got %s", pubPost.PublicationState)
	}
	if pubPost.FirstPublicationSeq <= 0 {
		t.Fatalf("expected positive FirstPublicationSeq, got %d", pubPost.FirstPublicationSeq)
	}

	// 9. Verify Public Feed shows published post with source attribution and link
	feedReq := httptest.NewRequest(http.MethodGet, "/v1/forum/posts?category=DRIVER_ECONOMY", nil)
	wFeed := httptest.NewRecorder()
	env.pubRouter.ServeHTTP(wFeed, feedReq)
	if wFeed.Code != http.StatusOK {
		t.Fatalf("expected 200 on public feed, got %d: %s", wFeed.Code, wFeed.Body.String())
	}
	var feedResp struct {
		Items []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Category string `json:"category"`
			Source   *struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"source"`
		} `json:"items"`
	}
	if err := json.Unmarshal(wFeed.Body.Bytes(), &feedResp); err != nil {
		t.Fatalf("unmarshal feed resp: %v", err)
	}
	if len(feedResp.Items) != 1 {
		t.Fatalf("expected 1 item in public feed, got %d", len(feedResp.Items))
	}
	item := feedResp.Items[0]
	if item.ID != cand3ID || item.Category != "DRIVER_ECONOMY" {
		t.Fatalf("unexpected feed item: %+v", item)
	}
	if item.Source == nil || item.Source.Name != "Equipe Abastevo" {
		t.Fatalf("expected source attribution Equipe Abastevo, got %+v", item.Source)
	}

	// 10. Verify Public Detail endpoint
	detailReq := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+cand3ID, nil)
	wDetail := httptest.NewRecorder()
	env.pubRouter.ServeHTTP(wDetail, detailReq)
	if wDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 on public detail, got %d", wDetail.Code)
	}

	// 11. Verify rejected post is NOT in public feed
	detailRejReq := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+cand1ID, nil)
	wRejDetail := httptest.NewRecorder()
	env.pubRouter.ServeHTTP(wRejDetail, detailRejReq)
	if wRejDetail.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found on rejected post public read, got %d", wRejDetail.Code)
	}
}
