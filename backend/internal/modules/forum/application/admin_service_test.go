package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
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

func (m *mockAdminStore) AdminListPosts(ctx context.Context, filter forumapp.AdminListFilter) ([]domain.Post, *string, error) {
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
}

func (r *mockRunner) Mutate(ctx context.Context, m adminapp.Mutation, fn func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error) {
	res, events, err := fn(ctx, nil)
	if err != nil {
		return adminapp.MutationResult{}, err
	}
	r.lastEvents = events
	return res, nil
}

func TestAdminService_CreateDraft(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	idCounter := 100

	sourceID := "11111111-1111-4111-8111-111111111111"
	store.sources[sourceID] = &domain.Source{
		ID:           sourceID,
		Code:         "abastevo-editorial",
		Name:         "Redação Abastevo",
		Kind:         domain.SourceKindAbastevo,
		RightsStatus: domain.RightsApproved,
	}

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true

	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
		NewID: func() (string, error) {
			idCounter++
			return "22222222-2222-4222-8222-000000000" + string(rune('0'+idCounter%10)) + string(rune('0'+(idCounter/10)%10)), nil
		},
	}

	mutation := adminapp.Mutation{
		SessionID: "sess-1",
		Action:    "admin.forum.post.create",
		Reason:    "criando rascunho editorial para teste",
		Key:       "33333333-3333-4333-8333-333333333333",
	}

	input := forumapp.CreateDraftInput{
		Kind:         "EDITORIAL",
		Category:     "FUELS",
		Title:        "Novo Panorama de Preços de Combustíveis",
		Excerpt:      "Resumo semanal da movimentação de combustíveis nos postos.",
		Body:         "Texto completo do artigo editorial contendo análises de mercado detalhadas.",
		SourceID:     sourceID,
		CanonicalURL: "https://abastevo.com.br/forum/artigo-1",
	}

	post, err := svc.CreateDraft(context.Background(), mutation, input)
	if err != nil {
		t.Fatalf("unexpected error creating draft: %v", err)
	}

	if post.PublicationState != domain.StateDraft {
		t.Fatalf("expected state DRAFT, got %s", post.PublicationState)
	}
	if post.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", post.Revision)
	}

	// Verify change record in store
	if len(store.changes) != 1 {
		t.Fatalf("expected 1 change record, got %d", len(store.changes))
	}
	if store.changes[0].Action != "CREATE_DRAFT" {
		t.Fatalf("expected action CREATE_DRAFT, got %s", store.changes[0].Action)
	}

	// CRITICAL PRIVACY VERIFICATION: Audit event MUST NOT contain body text or excerpt!
	if len(runner.lastEvents) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(runner.lastEvents))
	}
	auditAfter := string(runner.lastEvents[0].After)
	if strings.Contains(auditAfter, "Texto completo do artigo editorial") {
		t.Fatalf("SECURITY VIOLATION: BodyText found in admin_audit event: %s", auditAfter)
	}
	if strings.Contains(auditAfter, "Resumo semanal") {
		t.Fatalf("SECURITY VIOLATION: Excerpt found in admin_audit event: %s", auditAfter)
	}

	var auditView domain.PostAuditView
	if err := json.Unmarshal(runner.lastEvents[0].After, &auditView); err != nil {
		t.Fatalf("malformed audit JSON: %v", err)
	}
	if auditView.Title != input.Title || auditView.PublicationState != "DRAFT" {
		t.Fatalf("unexpected audit view content: %+v", auditView)
	}
}

func TestAdminService_EditPost(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	postID := "22222222-2222-4222-8222-222222222222"
	sourceID := "11111111-1111-4111-8111-111111111111"
	store.sources[sourceID] = &domain.Source{
		ID:           sourceID,
		RightsStatus: domain.RightsApproved,
	}

	store.posts[postID] = &domain.Post{
		ID:               postID,
		Kind:             domain.KindEditorial,
		Category:         domain.CategoryFuels,
		Title:            "Título Original",
		Excerpt:          "Excerpt Original",
		BodyText:         "Corpo Original do Artigo",
		SourceID:         sourceID,
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
	}

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true

	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
	}

	mutation := adminapp.Mutation{
		SessionID: "sess-1",
		Action:    "admin.forum.post.edit",
		Reason:    "atualizando título e corpo do post",
		Key:       "33333333-3333-4333-8333-333333333333",
	}

	// 1. Revision conflict test (stale revision)
	_, err := svc.EditPost(context.Background(), mutation, forumapp.EditPostInput{
		ID:               postID,
		Title:            "Novo Título",
		Category:         "FUELS",
		ExpectedRevision: 99, // Stale!
	})
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Fatalf("expected ErrRevisionMismatch, got %v", err)
	}

	// 2. Successful edit
	updated, err := svc.EditPost(context.Background(), mutation, forumapp.EditPostInput{
		ID:               postID,
		Title:            "Título Editado com Sucesso",
		Excerpt:          "Excerpt Editado",
		Body:             "Novo Corpo do Artigo Sensível",
		Category:         "FUELS",
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected edit error: %v", err)
	}

	if updated.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", updated.Revision)
	}
	if updated.Title != "Título Editado com Sucesso" {
		t.Fatalf("expected updated title, got %s", updated.Title)
	}

	// PRIVACY: Verify audit Before and After NEVER contain body text!
	auditBefore := string(runner.lastEvents[0].Before)
	auditAfter := string(runner.lastEvents[0].After)
	if strings.Contains(auditBefore, "Corpo Original") || strings.Contains(auditAfter, "Novo Corpo") {
		t.Fatalf("SECURITY VIOLATION: BodyText leaked into audit event: before=%s, after=%s", auditBefore, auditAfter)
	}
}

func TestAdminService_PublishPost_RightsAndTransitions(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	postID := "22222222-2222-4222-8222-222222222222"
	sourceID := "11111111-1111-4111-8111-111111111111"

	// Source with PENDING rights (not approved)
	store.sources[sourceID] = &domain.Source{
		ID:           sourceID,
		RightsStatus: domain.RightsPending,
	}

	store.posts[postID] = &domain.Post{
		ID:               postID,
		Kind:             domain.KindNews,
		Category:         domain.CategoryFuels,
		Title:            "Notícia Externa",
		SourceID:         sourceID,
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
	}

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true

	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
	}

	mutation := adminapp.Mutation{
		SessionID: "sess-1",
		Action:    "admin.forum.post.publish",
		Reason:    "publicando notícia",
		Key:       "33333333-3333-4333-8333-333333333333",
	}

	// 1. Must fail because source rights are not approved
	_, err := svc.PublishPost(context.Background(), mutation, postID)
	if !errors.Is(err, domain.ErrMissingRights) {
		t.Fatalf("expected ErrMissingRights for unapproved source, got %v", err)
	}

	// 2. Approve source rights and publish successfully
	store.sources[sourceID].RightsStatus = domain.RightsApproved

	published, err := svc.PublishPost(context.Background(), mutation, postID)
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	if published.PublicationState != domain.StatePublished {
		t.Fatalf("expected PUBLISHED state, got %s", published.PublicationState)
	}
	if published.FirstPublicationSeq != 1 {
		t.Fatalf("expected sequence 1, got %d", published.FirstPublicationSeq)
	}
	if published.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", published.Revision)
	}
}

func TestAdminService_SetVisibility(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	postID := "22222222-2222-4222-8222-222222222222"
	store.posts[postID] = &domain.Post{
		ID:               postID,
		Kind:             domain.KindEditorial,
		Category:         domain.CategoryFuels,
		Title:            "Editorial Publicado",
		PublicationState: domain.StatePublished,
		Visibility:       domain.VisibilityVisible,
		Revision:         2,
	}

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true

	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
	}

	mutation := adminapp.Mutation{
		SessionID: "sess-1",
		Action:    "admin.forum.post.visibility",
		Reason:    "ocultando post para moderação",
		Key:       "33333333-3333-4333-8333-333333333333",
	}

	hidden, err := svc.SetVisibility(context.Background(), mutation, postID, domain.VisibilityHidden)
	if err != nil {
		t.Fatalf("unexpected set visibility error: %v", err)
	}
	if hidden.Visibility != domain.VisibilityHidden {
		t.Fatalf("expected HIDDEN visibility, got %s", hidden.Visibility)
	}
	if hidden.Revision != 3 {
		t.Fatalf("expected revision 3, got %d", hidden.Revision)
	}

	// Unhide
	unhidden, err := svc.SetVisibility(context.Background(), mutation, postID, domain.VisibilityVisible)
	if err != nil {
		t.Fatalf("unexpected unhide error: %v", err)
	}
	if unhidden.Visibility != domain.VisibilityVisible {
		t.Fatalf("expected VISIBLE visibility, got %s", unhidden.Visibility)
	}
	if unhidden.Revision != 4 {
		t.Fatalf("expected revision 4, got %d", unhidden.Revision)
	}
}

func TestAdminService_DisabledSwitch(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = false // Default-off!

	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
	}

	mutation := adminapp.Mutation{SessionID: "s", Key: "k", Reason: "motivo longo o suficiente"}
	_, err := svc.CreateDraft(context.Background(), mutation, forumapp.CreateDraftInput{})
	if !errors.Is(err, domain.ErrForumDisabled) {
		t.Fatalf("expected ErrForumDisabled when switch is off, got %v", err)
	}
}
