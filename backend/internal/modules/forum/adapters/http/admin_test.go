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
	"github.com/jackc/pgx/v5"
)

type mockAdminStore struct {
	posts        map[string]*domain.Post
	sources      map[string]*domain.Source
	changes      []domain.ChangeRecord
	feedRevision int64
}

func newMockAdminStore() *mockAdminStore {
	return &mockAdminStore{
		posts:   make(map[string]*domain.Post),
		sources: make(map[string]*domain.Source),
		changes: make([]domain.ChangeRecord, 0),
	}
}

func (m *mockAdminStore) AdminListPosts(ctx context.Context, filter application.AdminListFilter) ([]domain.Post, *string, error) {
	items := make([]domain.Post, 0, len(m.posts))
	for _, p := range m.posts {
		if filter.PublicationState != nil && string(p.PublicationState) != *filter.PublicationState {
			continue
		}
		items = append(items, *p)
	}
	return items, nil, nil
}

func (m *mockAdminStore) GetPostByID(ctx context.Context, id string) (*domain.Post, error) {
	p, ok := m.posts[id]
	if !ok {
		return nil, domain.ErrPostNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *mockAdminStore) GetSourceByID(ctx context.Context, id string) (*domain.Source, error) {
	s, ok := m.sources[id]
	if !ok {
		return nil, domain.ErrMissingSource
	}
	cp := *s
	return &cp, nil
}

func (m *mockAdminStore) CreateDraftTx(ctx context.Context, tx pgx.Tx, post *domain.Post) error {
	cp := *post
	m.posts[post.ID] = &cp
	return nil
}

func (m *mockAdminStore) GetPostTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Post, error) {
	return m.GetPostByID(ctx, id)
}

func (m *mockAdminStore) GetPostForUpdateTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Post, error) {
	return m.GetPostByID(ctx, id)
}

func (m *mockAdminStore) UpdateEditorialTx(ctx context.Context, tx pgx.Tx, post *domain.Post, expectedRevision int) error {
	current, ok := m.posts[post.ID]
	if !ok {
		return domain.ErrPostNotFound
	}
	if current.Revision != expectedRevision {
		return domain.ErrRevisionMismatch
	}
	cp := *post
	cp.Revision = expectedRevision + 1
	m.posts[post.ID] = &cp
	return nil
}

func (m *mockAdminStore) PublishEditorialTx(ctx context.Context, tx pgx.Tx, id string, publishedAt time.Time, seq int64) error {
	current, ok := m.posts[id]
	if !ok {
		return domain.ErrPostNotFound
	}
	current.PublicationState = domain.StatePublished
	current.FirstPublicationSeq = seq
	current.FirstPublishedAt = &publishedAt
	current.Revision = current.Revision + 1
	return nil
}

func (m *mockAdminStore) ListAllSourcesWithMetrics(ctx context.Context) ([]application.SourceWithMetrics, error) {
	result := make([]application.SourceWithMetrics, 0, len(m.sources))
	for _, s := range m.sources {
		result = append(result, application.SourceWithMetrics{
			ID:                 s.ID,
			Code:               s.Code,
			Name:               s.Name,
			Kind:               s.Kind,
			OriginURL:          s.OriginURL,
			FetchURL:           s.FetchURL,
			FetchFormat:        s.FetchFormat,
			CollectionEnabled:  s.CollectionEnabled,
			PublicationEnabled: s.PublicationEnabled,
			RightsStatus:       s.RightsStatus,
			TitleAllowed:       s.TitleAllowed,
			ExcerptAllowed:     s.ExcerptAllowed,
			ImageAllowed:       s.ImageAllowed,
			TermsURL:           s.TermsURL,
			TermsSHA256:        s.TermsSHA256,
			CheckedAt:          s.CheckedAt,
			ReviewExpiresAt:    s.ReviewExpiresAt,
			Attribution:        s.Attribution,
		})
	}
	return result, nil
}

func (m *mockAdminStore) RejectPostTx(ctx context.Context, tx pgx.Tx, id string, expectedRevision int, now time.Time) error {
	current, ok := m.posts[id]
	if !ok {
		return domain.ErrPostNotFound
	}
	if current.Revision != expectedRevision {
		return domain.ErrRevisionMismatch
	}
	current.PublicationState = domain.StateRejected
	current.LocalUpdatedAt = now
	current.Revision = current.Revision + 1
	return nil
}

func (m *mockAdminStore) SetPostVisibilityTx(ctx context.Context, tx pgx.Tx, id string, visibility domain.Visibility, updatedAt time.Time) error {
	current, ok := m.posts[id]
	if !ok {
		return domain.ErrPostNotFound
	}
	current.Visibility = visibility
	current.LocalUpdatedAt = updatedAt
	current.Revision = current.Revision + 1
	return nil
}

func (m *mockAdminStore) BumpFeedRevisionTx(ctx context.Context, tx pgx.Tx, now time.Time) (int64, error) {
	m.feedRevision++
	return m.feedRevision, nil
}

func (m *mockAdminStore) RecordForumChangeTx(ctx context.Context, tx pgx.Tx, rec domain.ChangeRecord) error {
	m.changes = append(m.changes, rec)
	return nil
}

type mockRunner struct {
	lastEvents []adminapp.AuditEvent
	idempotent map[string]adminapp.MutationResult
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		idempotent: make(map[string]adminapp.MutationResult),
	}
}

func (r *mockRunner) Mutate(ctx context.Context, m adminapp.Mutation, fn func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error) {
	if cached, ok := r.idempotent[m.Key]; ok {
		return cached, nil
	}
	res, events, err := fn(ctx, nil)
	if err != nil {
		return adminapp.MutationResult{}, err
	}
	r.lastEvents = events
	r.idempotent[m.Key] = res
	return res, nil
}

type testHarness struct {
	router     chi.Router
	admin      *adminhttp.Handler
	store      *mockAdminStore
	runner     *mockRunner
	service    *application.AdminService
	handler    *AdminHandler
	session    adminapp.Session
	rawCSRF    string
	origin     string
	idCounter  int
	fixedTime  time.Time
}

type fakeAdminAuthStore struct {
	session adminapp.Session
}

func (f *fakeAdminAuthStore) Login(_ context.Context, _ adminapp.LoginInput) (adminapp.LoginResult, error) {
	return adminapp.LoginResult{}, nil
}
func (f *fakeAdminAuthStore) Authenticate(context.Context, string, time.Time) (adminapp.Session, error) {
	return f.session, nil
}
func (f *fakeAdminAuthStore) Logout(context.Context, adminapp.Session, adminapp.Mutation) (adminapp.MutationResult, error) {
	return adminapp.MutationResult{}, nil
}
func (f *fakeAdminAuthStore) RevokeAll(context.Context, adminapp.Session, adminapp.Mutation) (adminapp.MutationResult, error) {
	return adminapp.MutationResult{}, nil
}
func (f *fakeAdminAuthStore) Mutate(ctx context.Context, m adminapp.Mutation, fn func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error) {
	return adminapp.MutationResult{}, nil
}
func (f *fakeAdminAuthStore) RunInTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return nil
}
func (f *fakeAdminAuthStore) SearchAccounts(context.Context, string) ([]adminapp.Account, error) {
	return nil, nil
}
func (f *fakeAdminAuthStore) GetAccountTx(context.Context, pgx.Tx, string) (adminapp.Account, error) {
	return adminapp.Account{}, nil
}
func (f *fakeAdminAuthStore) PaidSummaryTx(context.Context, pgx.Tx, string) (adminapp.PaidSummary, error) {
	return adminapp.PaidSummary{}, nil
}
func (f *fakeAdminAuthStore) ListAudit(context.Context, adminapp.AuditFilter) ([]adminapp.AuditRecord, error) {
	return nil, nil
}

func setupTestHarness(adminEnabled bool) *testHarness {
	origin := "https://admin.example.com"
	rawCSRF := "test-csrf-token-abc"
	salt := "test-salt"
	hash := admindomain.SaltedHash(salt, rawCSRF)

	session := adminapp.Session{
		ID:             "sess-123",
		KeyFingerprint: "fp-123456",
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
				{Hash: strings.Repeat("a", 64), Fingerprint: "fp-123456"},
			},
		},
	}

	store := newMockAdminStore()
	runner := newMockRunner()
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = adminEnabled

	h := &testHarness{
		admin:     adminHandler,
		store:     store,
		runner:    runner,
		origin:    origin,
		rawCSRF:   rawCSRF,
		session:   session,
		fixedTime: fixedTime,
	}

	svc := &application.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
		NewID: func() (string, error) {
			h.idCounter++
			return "aaaaaaaa-bbbb-4ccc-8ddd-" + strings.Repeat("0", 11) + string(rune('0'+h.idCounter%10)), nil
		},
	}
	h.service = svc
	h.handler = NewAdminHandler(svc, adminHandler)

	r := chi.NewRouter()
	r.Group(func(g chi.Router) {
		g.Use(adminHandler.AdminMiddleware()...)
		if adminEnabled {
			h.handler.RegisterRoutes(g)
		}
	})
	h.router = r

	return h
}

func (h *testHarness) newAuthedRequest(method, path string, body any) *http.Request {
	var bodyReader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Origin", h.origin)
	req.Header.Set("X-Admin-CSRF", h.rawCSRF)
	req.Header.Set("Idempotency-Key", "44444444-4444-4444-8444-444444444444")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.AddCookie(&http.Cookie{
		Name:  "__Host-abv_admin",
		Value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	return req
}

func TestForumAdmin_Routes(t *testing.T) {
	h := setupTestHarness(true)
	methods := map[string][]string{}
	err := chi.Walk(h.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		methods[route] = append(methods[route], method)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string][]string{
		"/v1/admin/forum/sources":            {"GET"},
		"/v1/admin/forum/posts":              {"GET", "POST"},
		"/v1/admin/forum/posts/{id}":         {"GET"},
		"/v1/admin/forum/posts/{id}/edit":    {"POST"},
		"/v1/admin/forum/posts/{id}/publish": {"POST"},
		"/v1/admin/forum/posts/{id}/reject":  {"POST"},
		"/v1/admin/forum/posts/{id}/hide":    {"POST"},
		"/v1/admin/forum/posts/{id}/unhide":  {"POST"},
	}

	for route, expMethods := range expected {
		got, ok := methods[route]
		if !ok {
			t.Fatalf("missing route: %s", route)
		}
		for _, expM := range expMethods {
			found := false
			for _, m := range got {
				if m == expM {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("route %s missing method %s (got %v)", route, expM, got)
			}
		}
	}
}

func TestForumAdmin_AuthAndSecurityDenial(t *testing.T) {
	h := setupTestHarness(true)

	// 1. Missing session cookie -> 401
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/forum/posts", nil)
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", w.Code)
	}

	// 2. Disabled surface -> 404
	hDisabled := setupTestHarness(false)
	reqAuthed := hDisabled.newAuthedRequest(http.MethodGet, "/v1/admin/forum/posts", nil)
	wDisabled := httptest.NewRecorder()
	hDisabled.router.ServeHTTP(wDisabled, reqAuthed)
	if wDisabled.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when forum admin is disabled, got %d", wDisabled.Code)
	}
}

func TestForumAdmin_CreateDraft_Edit_Publish_Hide_Lifecycle(t *testing.T) {
	h := setupTestHarness(true)

	sourceID := "11111111-1111-4111-8111-111111111111"
	h.store.sources[sourceID] = &domain.Source{
		ID:                 sourceID,
		Code:               "abastevo-editorial",
		Name:               "Redação Abastevo",
		Kind:               domain.SourceKindAbastevo,
		RightsStatus:       domain.RightsApproved,
		PublicationEnabled: true,
	}

	// 1. Create Draft
	createBody := map[string]any{
		"kind":          "EDITORIAL",
		"category":      "FUELS",
		"title":         "Análise Semanal de Preços de Gasolina e Etanol",
		"excerpt":       "Resumo das oscilações registradas nos postos esta semana.",
		"body_text":     "Texto detalhado com segredos e rascunho de matérias confidenciais.",
		"source_id":     sourceID,
		"canonical_url": "https://abastevo.com.br/forum/analise-1",
		"reason":        "criação inicial de artigo editorial de teste",
	}

	createReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts", createBody)
	wCreate := httptest.NewRecorder()
	h.router.ServeHTTP(wCreate, createReq)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", wCreate.Code, wCreate.Body.String())
	}

	var createdPost domain.Post
	if err := json.Unmarshal(wCreate.Body.Bytes(), &createdPost); err != nil {
		t.Fatalf("failed to decode created post: %v", err)
	}
	if createdPost.PublicationState != domain.StateDraft {
		t.Fatalf("expected state DRAFT, got %s", createdPost.PublicationState)
	}
	postID := createdPost.ID

	// CRITICAL SECURITY AUDIT TEST: Ensure BodyText NEVER leaked into admin audit events!
	if len(h.runner.lastEvents) == 0 {
		t.Fatal("expected audit event to be recorded")
	}
	auditAfter := string(h.runner.lastEvents[0].After)
	if strings.Contains(auditAfter, "segredos e rascunho de matérias confidenciais") {
		t.Fatalf("CRITICAL SECURITY VIOLATION: BodyText leaked into admin_audit event: %s", auditAfter)
	}

	// 2. Edit Post (with wrong expected revision -> 409 conflict)
	editStaleReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/edit", map[string]any{
		"title":             "Título Novo",
		"category":          "FUELS",
		"expected_revision": 99, // Stale!
		"reason":            "tentando editar com versão obsoleta",
	})
	editStaleReq.Header.Set("Idempotency-Key", "55555555-5555-4555-8555-555555555555")
	wEditStale := httptest.NewRecorder()
	h.router.ServeHTTP(wEditStale, editStaleReq)
	if wEditStale.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", wEditStale.Code)
	}

	// 3. Edit Post (successful -> 200)
	editReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/edit", map[string]any{
		"title":             "Análise Semanal Atualizada",
		"excerpt":           "Resumo revisado.",
		"body_text":         "Corpo do artigo revisado e pronto para publicação.",
		"category":          "FUELS",
		"expected_revision": 1,
		"reason":            "revisão do artigo pela equipe editorial",
	})
	editReq.Header.Set("Idempotency-Key", "66666666-6666-4666-8666-666666666666")
	wEdit := httptest.NewRecorder()
	h.router.ServeHTTP(wEdit, editReq)
	if wEdit.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", wEdit.Code, wEdit.Body.String())
	}

	// 4. Publish Post -> 200
	pubReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/publish", map[string]any{
		"reason": "artigo aprovado e revisado para publicação pública",
	})
	pubReq.Header.Set("Idempotency-Key", "77777777-7777-4777-8777-777777777777")
	wPub := httptest.NewRecorder()
	h.router.ServeHTTP(wPub, pubReq)
	if wPub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", wPub.Code, wPub.Body.String())
	}

	var pubPost domain.Post
	if err := json.Unmarshal(wPub.Body.Bytes(), &pubPost); err != nil {
		t.Fatalf("failed to decode published post: %v", err)
	}
	if pubPost.PublicationState != domain.StatePublished {
		t.Fatalf("expected PUBLISHED state, got %s", pubPost.PublicationState)
	}

	// 5. Hide Post -> 200
	hideReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/hide", map[string]any{
		"reason": "ocultando temporariamente para esclarecimento de dados",
	})
	hideReq.Header.Set("Idempotency-Key", "88888888-8888-4888-8888-888888888888")
	wHide := httptest.NewRecorder()
	h.router.ServeHTTP(wHide, hideReq)
	if wHide.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on hide, got %d", wHide.Code)
	}

	var hiddenPost domain.Post
	if err := json.Unmarshal(wHide.Body.Bytes(), &hiddenPost); err != nil {
		t.Fatalf("failed to decode hidden post: %v", err)
	}
	if hiddenPost.Visibility != domain.VisibilityHidden {
		t.Fatalf("expected HIDDEN visibility, got %s", hiddenPost.Visibility)
	}

	// 6. Unhide Post -> 200
	unhideReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/unhide", map[string]any{
		"reason": "reexibindo artigo após verificação",
	})
	unhideReq.Header.Set("Idempotency-Key", "99999999-9999-4999-8999-999999999999")
	wUnhide := httptest.NewRecorder()
	h.router.ServeHTTP(wUnhide, unhideReq)
	if wUnhide.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on unhide, got %d", wUnhide.Code)
	}

	var unhiddenPost domain.Post
	if err := json.Unmarshal(wUnhide.Body.Bytes(), &unhiddenPost); err != nil {
		t.Fatalf("failed to decode unhidden post: %v", err)
	}
	if unhiddenPost.Visibility != domain.VisibilityVisible {
		t.Fatalf("expected VISIBLE visibility, got %s", unhiddenPost.Visibility)
	}

	// 7. Get Post detail (preview) -> 200
	getReq := h.newAuthedRequest(http.MethodGet, "/v1/admin/forum/posts/"+postID, nil)
	wGet := httptest.NewRecorder()
	h.router.ServeHTTP(wGet, getReq)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on get, got %d", wGet.Code)
	}

	// 8. List Posts -> 200
	listReq := h.newAuthedRequest(http.MethodGet, "/v1/admin/forum/posts?limit=10", nil)
	wList := httptest.NewRecorder()
	h.router.ServeHTTP(wList, listReq)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list, got %d", wList.Code)
	}
	var listResp struct {
		Items []domain.Post `json:"items"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listResp); err != nil || len(listResp.Items) != 1 {
		t.Fatalf("unexpected list response: items=%d err=%v", len(listResp.Items), err)
	}
}

func TestForumAdmin_SourcesAndRejectFlow(t *testing.T) {
	h := setupTestHarness(true)

	// Seed source
	srcID := "00000000-0000-0000-0000-000000000001"
	h.store.sources[srcID] = &domain.Source{
		ID:                 srcID,
		Code:               "EBC",
		Name:               "Agência Brasil",
		Kind:               domain.SourceKindPublisher,
		OriginURL:          "https://agenciabrasil.ebc.com.br",
		CollectionEnabled:  false,
		PublicationEnabled: false,
		RightsStatus:       domain.RightsDenied,
		Attribution:        "Agência Brasil / EBC",
	}

	// 1. List Sources -> 200 OK
	srcReq := h.newAuthedRequest(http.MethodGet, "/v1/admin/forum/sources", nil)
	wSrc := httptest.NewRecorder()
	h.router.ServeHTTP(wSrc, srcReq)
	if wSrc.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list sources, got %d: %s", wSrc.Code, wSrc.Body.String())
	}
	var srcResp struct {
		Items []application.SourceWithMetrics `json:"items"`
	}
	if err := json.Unmarshal(wSrc.Body.Bytes(), &srcResp); err != nil || len(srcResp.Items) != 1 {
		t.Fatalf("unexpected sources response: items=%d err=%v", len(srcResp.Items), err)
	}
	if srcResp.Items[0].Code != "EBC" {
		t.Fatalf("expected code EBC, got %s", srcResp.Items[0].Code)
	}

	// Seed draft candidate post
	postID := "11111111-1111-1111-1111-111111111111"
	h.store.posts[postID] = &domain.Post{
		ID:               postID,
		Kind:             domain.KindNews,
		Category:         domain.CategoryFuels,
		Title:            "Notícia Candidata para Rejeição",
		Excerpt:          "Resumo para rejeição",
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
		SourceID:         srcID,
		LocalCreatedAt:   time.Now().UTC(),
		LocalUpdatedAt:   time.Now().UTC(),
	}

	// 2. Reject Post with revision mismatch -> 409
	badRejectReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/reject", map[string]any{
		"expected_revision": 99,
		"reason":            "rejeitando por motivo de teste válido",
	})
	wBadReject := httptest.NewRecorder()
	h.router.ServeHTTP(wBadReject, badRejectReq)
	if wBadReject.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on revision mismatch, got %d: %s", wBadReject.Code, wBadReject.Body.String())
	}

	// 3. Reject Post with valid expected_revision -> 200 OK
	goodRejectReq := h.newAuthedRequest(http.MethodPost, "/v1/admin/forum/posts/"+postID+"/reject", map[string]any{
		"expected_revision": 1,
		"reason":            "rejeitando por direitos de distribuição negados",
	})
	wGoodReject := httptest.NewRecorder()
	h.router.ServeHTTP(wGoodReject, goodRejectReq)
	if wGoodReject.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on reject, got %d: %s", wGoodReject.Code, wGoodReject.Body.String())
	}
	var rejectResp domain.Post
	if err := json.Unmarshal(wGoodReject.Body.Bytes(), &rejectResp); err != nil {
		t.Fatalf("failed to unmarshal reject response: %v", err)
	}
	if rejectResp.PublicationState != domain.StateRejected {
		t.Fatalf("expected REJECTED state, got %s", rejectResp.PublicationState)
	}
	if rejectResp.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", rejectResp.Revision)
	}
}

func TestForumAdmin_ErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		body string
	}{
		{domain.ErrPostNotFound, http.StatusNotFound, "forum.not-found"},
		{domain.ErrRevisionMismatch, http.StatusConflict, "forum.revision-conflict"},
		{domain.ErrMissingRights, http.StatusBadRequest, "forum.missing-rights"},
		{domain.ErrForumDisabled, http.StatusNotFound, "forum.disabled"},
		{domain.ErrInvalidCategory, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrInvalidPostKind, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrTitleEmpty, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrTitleTooLong, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrExcerptTooLong, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrBodyTooLong, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrInvalidTransition, http.StatusBadRequest, "forum.invalid"},
		{domain.ErrInvalidCursor, http.StatusBadRequest, "forum.invalid"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/v1/admin/forum/posts", nil)
		writeServiceError(w, r, tc.err)
		if w.Code != tc.code {
			t.Fatalf("err=%v expected status %d, got %d", tc.err, tc.code, w.Code)
		}
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || payload.Error.Code != tc.body {
			t.Fatalf("err=%v expected code %s, got %s (body=%s)", tc.err, tc.body, payload.Error.Code, w.Body.String())
		}
	}
}
