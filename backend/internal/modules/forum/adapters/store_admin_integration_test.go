//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestForumAdmin_Integration_StoreLifecycle(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	// Seed source
	sourceID := "11111111-1111-4111-8111-111111111111"
	_, err := pool.Exec(ctx, `
		INSERT INTO forum_sources (id, code, name, kind, origin_url, rights_status, publication_enabled)
		VALUES ($1, 'test-editorial', 'Redação Abastevo', 'ABASTEVO', 'https://abastevo.com.br', 'APPROVED', true)
	`, sourceID)
	if err != nil {
		t.Fatalf("insert source: %v", err)
	}

	src, err := store.GetSourceByID(ctx, sourceID)
	if err != nil {
		t.Fatalf("get source by id: %v", err)
	}
	if src.Code != "test-editorial" {
		t.Fatalf("unexpected source code: %s", src.Code)
	}

	// 1. Transactional Draft Creation
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	postID := "22222222-2222-4222-8222-222222222222"
	now := time.Now().UTC()
	draft := &domain.Post{
		ID:                  postID,
		Kind:                domain.KindEditorial,
		Category:            domain.CategoryFuels,
		Title:               "Rascunho Inicial de Teste",
		Excerpt:             "Resumo de rascunho",
		BodyText:            "Corpo completo de rascunho com conteúdo confidencial.",
		SourceID:            sourceID,
		CanonicalURL:        "https://abastevo.com.br/forum/artigo-admin",
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

	refKey := "test-audit-ref-1"
	changeRec := domain.ChangeRecord{
		TargetType:    "POST",
		TargetID:      draft.ID,
		Revision:      1,
		Action:        "CREATE_DRAFT",
		ActorKind:     "ADMIN",
		AdminAuditRef: &refKey,
		ReasonCode:    "test_create_draft",
		NewState:      "DRAFT",
		CreatedAt:     now,
	}
	if err := store.RecordForumChangeTx(ctx, tx, changeRec); err != nil {
		t.Fatalf("record forum change tx: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit create draft: %v", err)
	}

	// 2. Read back post
	loaded, err := store.GetPostByID(ctx, postID)
	if err != nil {
		t.Fatalf("get post by id: %v", err)
	}
	if loaded.Title != "Rascunho Inicial de Teste" || loaded.Revision != 1 || loaded.PublicationState != domain.StateDraft {
		t.Fatalf("unexpected loaded post: %+v", loaded)
	}

	// 3. Edit post with optimistic concurrency check
	txEdit, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin edit tx: %v", err)
	}
	defer txEdit.Rollback(ctx)

	forUpdate, err := store.GetPostForUpdateTx(ctx, txEdit, postID)
	if err != nil {
		t.Fatalf("get post for update tx: %v", err)
	}

	// Fail with stale revision
	if err := store.UpdateEditorialTx(ctx, txEdit, forUpdate, 99); !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Fatalf("expected ErrRevisionMismatch, got %v", err)
	}

	// Succeed with revision 1
	forUpdate.Title = "Título Atualizado em Transação"
	forUpdate.Excerpt = "Resumo atualizado"
	if err := store.UpdateEditorialTx(ctx, txEdit, forUpdate, 1); err != nil {
		t.Fatalf("update editorial tx: %v", err)
	}

	priorStateStr := "rev-1"
	editChangeRec := domain.ChangeRecord{
		TargetType:    "POST",
		TargetID:      draft.ID,
		Revision:      2,
		Action:        "EDIT",
		ActorKind:     "ADMIN",
		AdminAuditRef: &refKey,
		ReasonCode:    "test_edit",
		PriorState:    &priorStateStr,
		NewState:      "rev-2",
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.RecordForumChangeTx(ctx, txEdit, editChangeRec); err != nil {
		t.Fatalf("record edit change tx: %v", err)
	}

	if err := txEdit.Commit(ctx); err != nil {
		t.Fatalf("commit edit: %v", err)
	}

	// 4. Publish post with monotonic sequence
	txPub, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin pub tx: %v", err)
	}
	defer txPub.Rollback(ctx)

	pubTime := time.Now().UTC()
	seq, err := store.BumpFeedRevisionTx(ctx, txPub, pubTime)
	if err != nil {
		t.Fatalf("bump feed revision tx: %v", err)
	}
	if seq < 1 {
		t.Fatalf("expected seq >= 1, got %d", seq)
	}

	if err := store.PublishEditorialTx(ctx, txPub, postID, pubTime, seq); err != nil {
		t.Fatalf("publish editorial tx: %v", err)
	}

	pubChangeRec := domain.ChangeRecord{
		TargetType:    "POST",
		TargetID:      draft.ID,
		Revision:      3,
		Action:        "PUBLISH",
		ActorKind:     "ADMIN",
		AdminAuditRef: &refKey,
		ReasonCode:    "test_publish",
		PriorState:    &priorStateStr,
		NewState:      "PUBLISHED",
		CreatedAt:     pubTime,
	}
	if err := store.RecordForumChangeTx(ctx, txPub, pubChangeRec); err != nil {
		t.Fatalf("record pub change tx: %v", err)
	}

	if err := txPub.Commit(ctx); err != nil {
		t.Fatalf("commit pub: %v", err)
	}

	published, err := store.GetPostByID(ctx, postID)
	if err != nil {
		t.Fatalf("get published post: %v", err)
	}
	if published.PublicationState != domain.StatePublished || published.FirstPublicationSeq != seq {
		t.Fatalf("unexpected published post: %+v", published)
	}

	// 5. Hide post
	txHide, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin hide tx: %v", err)
	}
	defer txHide.Rollback(ctx)

	if err := store.SetPostVisibilityTx(ctx, txHide, postID, domain.VisibilityHidden, time.Now().UTC()); err != nil {
		t.Fatalf("set post visibility tx: %v", err)
	}
	if err := txHide.Commit(ctx); err != nil {
		t.Fatalf("commit hide: %v", err)
	}

	hidden, err := store.GetPostByID(ctx, postID)
	if err != nil {
		t.Fatalf("get hidden post: %v", err)
	}
	if hidden.Visibility != domain.VisibilityHidden {
		t.Fatalf("expected hidden visibility, got %s", hidden.Visibility)
	}

	// 6. AdminListPosts with filters and pagination
	listFilter := application.AdminListFilter{
		PublicationState: nil,
		Limit:            10,
	}
	posts, nextCursor, err := store.AdminListPosts(ctx, listFilter)
	if err != nil {
		t.Fatalf("admin list posts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("expected 1 post in admin list, got %d", len(posts))
	}
	if nextCursor != nil {
		t.Fatalf("expected nil nextCursor when results < limit, got %s", *nextCursor)
	}

	// Verify that admin list does not load body text
	if posts[0].BodyText != "" {
		t.Fatalf("expected empty BodyText on list projection, got: %s", posts[0].BodyText)
	}

	// Test cursor roundtrip
	token := EncodeAdminCursor(posts[0].LocalCreatedAt, posts[0].ID)
	parsedT, parsedID, err := DecodeAdminCursor(token)
	if err != nil {
		t.Fatalf("decode admin cursor: %v", err)
	}
	if parsedID != posts[0].ID {
		t.Fatalf("expected id %s, got %s", posts[0].ID, parsedID)
	}
	if parsedT.Unix() != posts[0].LocalCreatedAt.Unix() {
		t.Fatalf("expected time %v, got %v", posts[0].LocalCreatedAt, parsedT)
	}
}
