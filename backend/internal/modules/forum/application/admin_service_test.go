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

func (m *mockAdminStore) ListAllSourcesWithMetrics(ctx context.Context) ([]forumapp.SourceWithMetrics, error) {
	result := make([]forumapp.SourceWithMetrics, 0, len(m.sources))
	for _, s := range m.sources {
		result = append(result, forumapp.SourceWithMetrics{
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
	store.sources[sourceID].PublicationEnabled = true

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

func TestAdminService_PublishBulletinHidesSuperseded(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	sourceID := "11111111-1111-4111-8111-111111111111"
	store.sources[sourceID] = &domain.Source{
		ID: sourceID, Code: "abastevo-editorial", RightsStatus: domain.RightsApproved, PublicationEnabled: true,
	}
	oldID := "22222222-2222-4222-8222-222222222222"
	newID := "33333333-3333-4333-8333-333333333333"
	oldKey := "forum-bulletin-v1:old"
	oldGroup := "forum-bulletin-v1:group"
	oldProv := &domain.BulletinProvenance{MethodologyVersion: "forum-official-mean-v1", TemplateVersion: "forum-bulletin-v1"}
	store.posts[oldID] = &domain.Post{
		ID: oldID, Kind: domain.KindBulletin, Category: domain.CategoryAbastevoData,
		Title: "Boletim antigo", SourceID: sourceID,
		PublicationState: domain.StatePublished, Visibility: domain.VisibilityVisible, Revision: 2,
		BulletinKey: &oldKey, BulletinGroupKey: &oldGroup, BulletinProvenance: oldProv,
	}
	newKey := "forum-bulletin-v1:new"
	store.posts[newID] = &domain.Post{
		ID: newID, Kind: domain.KindBulletin, Category: domain.CategoryAbastevoData,
		Title: "Boletim corrigido", SourceID: sourceID,
		PublicationState: domain.StatePendingReview, Visibility: domain.VisibilityVisible, Revision: 1,
		BulletinKey: &newKey, BulletinGroupKey: &oldGroup, BulletinProvenance: oldProv,
		SupersedesPostID: &oldID,
	}
	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true
	svc := &forumapp.AdminService{Store: store, Runner: runner, Config: cfg, Clock: func() time.Time { return fixedTime }}
	mutation := adminapp.Mutation{SessionID: "sess-1", Action: "admin.forum.post.publish", Reason: "publicando correção", Key: "44444444-4444-4444-8444-444444444444"}

	published, err := svc.PublishPost(context.Background(), mutation, newID)
	if err != nil {
		t.Fatalf("publish bulletin: %v", err)
	}
	if published.PublicationState != domain.StatePublished {
		t.Fatalf("new bulletin state %s", published.PublicationState)
	}
	if store.posts[oldID].Visibility != domain.VisibilityHidden {
		t.Fatalf("superseded bulletin visibility %s", store.posts[oldID].Visibility)
	}
	if store.feedRevision != 1 {
		t.Fatalf("feed revision %d", store.feedRevision)
	}
	if len(runner.lastEvents) != 2 || runner.lastEvents[1].Action != "admin.forum.post.supersede" ||
		runner.lastEvents[1].TargetID != oldID {
		t.Fatalf("supersede audit %+v", runner.lastEvents)
	}
}

func TestAdminService_EditBulletinRefused(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	postID := "22222222-2222-4222-8222-222222222222"
	key := "forum-bulletin-v1:key"
	group := "forum-bulletin-v1:group"
	prov := &domain.BulletinProvenance{MethodologyVersion: "forum-official-mean-v1", TemplateVersion: "forum-bulletin-v1"}
	store.posts[postID] = &domain.Post{
		ID: postID, Kind: domain.KindBulletin, Category: domain.CategoryAbastevoData,
		Title: "Boletim", PublicationState: domain.StatePendingReview, Visibility: domain.VisibilityVisible,
		Revision: 1, BulletinKey: &key, BulletinGroupKey: &group, BulletinProvenance: prov,
	}
	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true
	svc := &forumapp.AdminService{Store: store, Runner: runner, Config: cfg, Clock: func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }}
	_, err := svc.EditPost(context.Background(), adminapp.Mutation{SessionID: "s", Action: "admin.forum.post.edit", Reason: "tentativa", Key: "k"}, forumapp.EditPostInput{ID: postID, Title: "tentativa", Category: "FUELS", ExpectedRevision: 1})
	if !errors.Is(err, domain.ErrBulletinImmutable) {
		t.Fatalf("expected ErrBulletinImmutable, got %v", err)
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

func TestAdminService_ListSources(t *testing.T) {
	store := newMockAdminStore()
	store.sources["s1"] = &domain.Source{
		ID:                 "s1",
		Code:               "EBC",
		Name:               "Agência Brasil",
		Kind:               domain.SourceKindPublisher,
		OriginURL:          "https://agenciabrasil.ebc.com.br",
		CollectionEnabled:  false,
		PublicationEnabled: false,
		RightsStatus:       domain.RightsDenied,
		Attribution:        "Agência Brasil / EBC",
	}

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true
	svc := &forumapp.AdminService{
		Store:  store,
		Runner: &mockRunner{},
		Config: cfg,
	}

	sources, err := svc.ListSources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Code != "EBC" || sources[0].RightsStatus != domain.RightsDenied {
		t.Fatalf("unexpected source: %+v", sources[0])
	}
}

func TestAdminService_RejectPost(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true
	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
	}

	postID := "11111111-1111-1111-1111-111111111111"
	store.posts[postID] = &domain.Post{
		ID:               postID,
		Kind:             domain.KindNews,
		Category:         domain.CategoryFuels,
		Title:            "Notícia Candidata",
		Excerpt:          "Resumo da notícia",
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
		SourceID:         "00000000-0000-0000-0000-000000000001",
		LocalCreatedAt:   fixedTime,
		LocalUpdatedAt:   fixedTime,
	}

	mutation := adminapp.Mutation{
		SessionID: "session-1",
		Reason:    "rejeitando notícia candidata inadequada",
		Key:       "33333333-3333-4333-8333-333333333333",
	}

	// 1. Revision mismatch fails
	_, err := svc.RejectPost(context.Background(), mutation, postID, 99)
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Fatalf("expected ErrRevisionMismatch, got %v", err)
	}

	// 2. Reject succeeds with expected revision
	rejected, err := svc.RejectPost(context.Background(), mutation, postID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.PublicationState != domain.StateRejected {
		t.Fatalf("expected REJECTED state, got %s", rejected.PublicationState)
	}
	if rejected.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", rejected.Revision)
	}

	// 3. Verify audit and change record
	if len(runner.lastEvents) != 1 || runner.lastEvents[0].Action != "admin.forum.post.reject" {
		t.Fatalf("expected audit event admin.forum.post.reject, got %+v", runner.lastEvents)
	}
	if len(store.changes) != 1 || store.changes[0].Action != "REJECT" {
		t.Fatalf("expected change record REJECT, got %+v", store.changes)
	}

	// 4. Double reject fails transition
	_, err = svc.RejectPost(context.Background(), mutation, postID, 2)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition on already rejected post, got %v", err)
	}
}

func TestAdminService_PublishPost_RightsEnforcement(t *testing.T) {
	store := newMockAdminStore()
	runner := &mockRunner{}
	fixedTime := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	cfg := domain.DefaultConfig()
	cfg.AdminEnabled = true
	svc := &forumapp.AdminService{
		Store:  store,
		Runner: runner,
		Config: cfg,
		Clock:  func() time.Time { return fixedTime },
	}

	// Source 1: Rights Approved but PublicationEnabled = false
	disabledSrcID := "00000000-0000-0000-0000-000000000001"
	store.sources[disabledSrcID] = &domain.Source{
		ID:                 disabledSrcID,
		RightsStatus:       domain.RightsApproved,
		PublicationEnabled: false,
	}

	post1ID := "11111111-1111-1111-1111-111111111111"
	store.posts[post1ID] = &domain.Post{
		ID:               post1ID,
		Kind:             domain.KindNews,
		Category:         domain.CategoryFuels,
		Title:            "Notícia de Fonte Desabilitada",
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
		SourceID:         disabledSrcID,
		LocalCreatedAt:   fixedTime,
		LocalUpdatedAt:   fixedTime,
	}

	mutation := adminapp.Mutation{SessionID: "s", Key: "k", Reason: "tentando publicar notícia"}

	_, err := svc.PublishPost(context.Background(), mutation, post1ID)
	if !errors.Is(err, domain.ErrMissingRights) {
		t.Fatalf("expected ErrMissingRights when PublicationEnabled is false, got %v", err)
	}

	// Source 2: Rights Approved and PublicationEnabled, but review expired
	expiredTime := fixedTime.Add(-24 * time.Hour)
	expiredSrcID := "00000000-0000-0000-0000-000000000002"
	store.sources[expiredSrcID] = &domain.Source{
		ID:                 expiredSrcID,
		RightsStatus:       domain.RightsApproved,
		PublicationEnabled: true,
		ReviewExpiresAt:    &expiredTime,
	}

	post2ID := "22222222-2222-2222-2222-222222222222"
	store.posts[post2ID] = &domain.Post{
		ID:               post2ID,
		Kind:             domain.KindNews,
		Category:         domain.CategoryFuels,
		Title:            "Notícia de Fonte Expirada",
		PublicationState: domain.StateDraft,
		Visibility:       domain.VisibilityVisible,
		Revision:         1,
		SourceID:         expiredSrcID,
		LocalCreatedAt:   fixedTime,
		LocalUpdatedAt:   fixedTime,
	}

	_, err = svc.PublishPost(context.Background(), mutation, post2ID)
	if !errors.Is(err, domain.ErrMissingRights) {
		t.Fatalf("expected ErrMissingRights when ReviewExpiresAt has passed, got %v", err)
	}
}
