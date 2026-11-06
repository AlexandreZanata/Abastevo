//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

const (
	seedSourceID = "00000000-0000-0000-0000-000000000001"
)

func createTestAccount(t *testing.T, pool *pgxpool.Pool, id, alias string) string {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, alias, status, created_at)
		VALUES ($1, $2, 'active', now())
	`, id, alias)
	if err != nil {
		t.Fatalf("createTestAccount failed: %v", err)
	}
	return id
}

func createTestPost(t *testing.T, pool *pgxpool.Pool, id, title, state, visibility string, commentsEnabled bool) string {
	t.Helper()
	ctx := context.Background()
	firstPubSeq := int64(0)
	var pubAt *time.Time
	if state == string(domain.StatePublished) {
		firstPubSeq = 1
		now := time.Now().UTC()
		pubAt = &now
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO forum_posts (
			id, kind, category, title, excerpt, body_text, source_id,
			publication_state, visibility, first_publication_seq, first_published_at,
			comments_enabled, revision
		) VALUES (
			$1, 'EDITORIAL', 'FUELS', $2, 'Excerpt', 'Body text', $3,
			$4, $5, $6, $7, $8, 1
		)
	`, id, title, seedSourceID, state, visibility, firstPubSeq, pubAt, commentsEnabled)
	if err != nil {
		t.Fatalf("createTestPost failed: %v", err)
	}
	return id
}

func TestComments_CreationAndValidation(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "motorista-1")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Preço dos combustíveis em SP", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	t.Run("valid comment with portuguese accents and emojis", func(t *testing.T) {
		res, err := store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          postID,
			AuthorAccountID: authorID,
			PublicAlias:     "Motorista SP",
			BodyText:        "Excelente notícia! O etanol caiu R$ 0,10 na minha região hoje. ⛽🚗",
			OperationID:     "33333333-3333-4333-8333-000000000001",
		})
		if err != nil {
			t.Fatalf("unexpected error creating comment: %v", err)
		}
		if res.Replayed {
			t.Errorf("expected fresh creation, got replayed")
		}
		c := res.Comment
		if c.Sequence != 1 {
			t.Errorf("expected sequence 1, got %d", c.Sequence)
		}
		if c.PublicationState != domain.StatePendingReview {
			t.Errorf("expected pending_review state, got %s", c.PublicationState)
		}
		if c.Visibility != domain.VisibilityVisible {
			t.Errorf("expected visible visibility, got %s", c.Visibility)
		}
		if c.Depth != 0 || c.ParentID != nil {
			t.Errorf("expected top-level comment (depth 0, parent nil), got depth %d, parent %v", c.Depth, c.ParentID)
		}

		// Verify change was recorded in forum_changes
		var changeAction, targetType string
		err = pool.QueryRow(ctx, `
			SELECT action, target_type FROM forum_changes
			WHERE target_id = $1 AND revision = 1
		`, c.ID).Scan(&changeAction, &targetType)
		if err != nil {
			t.Fatalf("failed to query forum_changes: %v", err)
		}
		if changeAction != "CREATE" || targetType != "COMMENT" {
			t.Errorf("unexpected change record: action=%s, target_type=%s", changeAction, targetType)
		}
	})

	t.Run("second comment gets monotonic sequence 2", func(t *testing.T) {
		res, err := store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          postID,
			AuthorAccountID: authorID,
			PublicAlias:     "Motorista SP",
			BodyText:        "Segundo comentário no mesmo post.",
			OperationID:     "33333333-3333-4333-8333-000000000002",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Comment.Sequence != 2 {
			t.Errorf("expected sequence 2, got %d", res.Comment.Sequence)
		}
	})

	t.Run("boundary 280 code points accepted", func(t *testing.T) {
		res, err := store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          postID,
			AuthorAccountID: authorID,
			PublicAlias:     "Motorista SP",
			BodyText:        strings.Repeat("a", 280),
			OperationID:     "33333333-3333-4333-8333-000000000003",
		})
		if err != nil {
			t.Fatalf("unexpected error on 280 code points: %v", err)
		}
		if res.Comment.Sequence != 3 {
			t.Errorf("expected sequence 3, got %d", res.Comment.Sequence)
		}
	})

	t.Run("exceeding 280 code points rejected by database check", func(t *testing.T) {
		_, err := store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          postID,
			AuthorAccountID: authorID,
			PublicAlias:     "Motorista SP",
			BodyText:        strings.Repeat("a", 281),
			OperationID:     "33333333-3333-4333-8333-000000000004",
		})
		if err == nil {
			t.Fatal("expected error on body exceeding 280 chars, got nil")
		}
	})
}

func TestComments_ReplyDepthAndHierarchy(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	author1 := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	author2 := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000002", "author-2")
	postA := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)
	postB := createTestPost(t, pool, "22222222-2222-4222-8222-000000000002", "Post B", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// 1. Create top-level comment on Post A
	resTop, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postA,
		AuthorAccountID: author1,
		PublicAlias:     "Autor 1",
		BodyText:        "Comentário raiz no Post A",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if err != nil {
		t.Fatalf("failed to create top-level comment: %v", err)
	}
	topID := resTop.Comment.ID

	// 2. Publish top-level comment so it can receive replies
	_, err = store.PublishComment(ctx, topID)
	if err != nil {
		t.Fatalf("failed to publish top comment: %v", err)
	}

	// 3. Valid reply to top-level comment (depth 1)
	resReply, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postA,
		AuthorAccountID: author2,
		PublicAlias:     "Autor 2",
		ParentID:        &topID,
		BodyText:        "Resposta ao comentário raiz",
		OperationID:     "33333333-3333-4333-8333-000000000002",
	})
	if err != nil {
		t.Fatalf("failed to create reply: %v", err)
	}
	if resReply.Comment.Depth != 1 || resReply.Comment.ParentID == nil || *resReply.Comment.ParentID != topID {
		t.Errorf("expected depth 1 reply linked to topID, got depth %d parent %v", resReply.Comment.Depth, resReply.Comment.ParentID)
	}
	replyID := resReply.Comment.ID

	// 4. Publish the reply
	_, err = store.PublishComment(ctx, replyID)
	if err != nil {
		t.Fatalf("failed to publish reply: %v", err)
	}

	// 5. Attempt reply to reply (depth 2) -> must fail with ErrInvalidCommentDepth
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postA,
		AuthorAccountID: author1,
		PublicAlias:     "Autor 1",
		ParentID:        &replyID,
		BodyText:        "Tentativa de resposta a uma resposta (proibido)",
		OperationID:     "33333333-3333-4333-8333-000000000003",
	})
	if !errors.Is(err, domain.ErrInvalidCommentDepth) {
		t.Errorf("expected ErrInvalidCommentDepth, got %v", err)
	}

	// 6. Cross-post parent rejection: comment on Post B referencing parent from Post A
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postB,
		AuthorAccountID: author2,
		PublicAlias:     "Autor 2",
		ParentID:        &topID,
		BodyText:        "Tentativa de cross-post parent",
		OperationID:     "33333333-3333-4333-8333-000000000004",
	})
	if !errors.Is(err, domain.ErrCrossPostParent) {
		t.Errorf("expected ErrCrossPostParent, got %v", err)
	}
}

func TestComments_PostStateAndCommentsEnabled(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	draftPost := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Draft Post", string(domain.StateDraft), string(domain.VisibilityVisible), true)
	hiddenPost := createTestPost(t, pool, "22222222-2222-4222-8222-000000000002", "Hidden Post", string(domain.StatePublished), string(domain.VisibilityHidden), true)
	disabledPost := createTestPost(t, pool, "22222222-2222-4222-8222-000000000003", "Disabled Comments Post", string(domain.StatePublished), string(domain.VisibilityVisible), false)

	// Comment on draft post -> ErrPostNotCommentable
	_, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          draftPost,
		AuthorAccountID: authorID,
		BodyText:        "Comentário em draft",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if !errors.Is(err, domain.ErrPostNotCommentable) {
		t.Errorf("expected ErrPostNotCommentable on draft, got %v", err)
	}

	// Comment on hidden post -> ErrPostNotCommentable
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          hiddenPost,
		AuthorAccountID: authorID,
		BodyText:        "Comentário em post oculto",
		OperationID:     "33333333-3333-4333-8333-000000000002",
	})
	if !errors.Is(err, domain.ErrPostNotCommentable) {
		t.Errorf("expected ErrPostNotCommentable on hidden post, got %v", err)
	}

	// Comment on post with comments disabled -> ErrPostCommentsDisabled
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          disabledPost,
		AuthorAccountID: authorID,
		BodyText:        "Comentário em post desabilitado",
		OperationID:     "33333333-3333-4333-8333-000000000003",
	})
	if !errors.Is(err, domain.ErrPostCommentsDisabled) {
		t.Errorf("expected ErrPostCommentsDisabled, got %v", err)
	}
}

func TestComments_ParentVisibilityAndSuppression(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Published Post", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// Create top comment (starts in PENDING_REVIEW)
	resTop, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário pai ainda pendente de revisão",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if err != nil {
		t.Fatalf("failed to create top comment: %v", err)
	}
	topID := resTop.Comment.ID

	// Reply to pending parent -> ErrParentCommentNotVisible
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		ParentID:        &topID,
		BodyText:        "Tentando responder ao pai pendente",
		OperationID:     "33333333-3333-4333-8333-000000000002",
	})
	if !errors.Is(err, domain.ErrParentCommentNotVisible) {
		t.Errorf("expected ErrParentCommentNotVisible, got %v", err)
	}

	// Publish parent
	_, err = store.PublishComment(ctx, topID)
	if err != nil {
		t.Fatalf("failed to publish parent: %v", err)
	}

	// Now reply succeeds
	resReply, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		ParentID:        &topID,
		BodyText:        "Resposta ao pai publicado",
		OperationID:     "33333333-3333-4333-8333-000000000003",
	})
	if err != nil {
		t.Fatalf("failed to create reply: %v", err)
	}

	// Publish reply
	_, err = store.PublishComment(ctx, resReply.Comment.ID)
	if err != nil {
		t.Fatalf("failed to publish reply: %v", err)
	}

	// Verify reply appears in ListRepliesByParent
	replies, err := store.ListRepliesByParent(ctx, application.ListCommentRepliesQuery{
		ParentID: topID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("failed to list replies: %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}

	// Now remove (tombstone) parent comment
	_, err = store.RemoveComment(ctx, application.RemoveCommentCommand{
		CommentID:        topID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 2, // 1 on create, 2 on publish
	})
	if err != nil {
		t.Fatalf("failed to remove parent comment: %v", err)
	}

	// Per B-BR-FOR-09: a hidden/removed parent suppresses its reply subtree from public reads
	repliesAfterRemove, err := store.ListRepliesByParent(ctx, application.ListCommentRepliesQuery{
		ParentID: topID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("failed to list replies after remove: %v", err)
	}
	if len(repliesAfterRemove) != 0 {
		t.Errorf("expected 0 replies due to parent suppression, got %d", len(repliesAfterRemove))
	}
}

func TestComments_ScopedMonotonicSequenceAndRollback(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	postA := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)
	postB := createTestPost(t, pool, "22222222-2222-4222-8222-000000000002", "Post B", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// Post A: comment 1 -> seq 1
	resA1, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postA,
		AuthorAccountID: authorID,
		BodyText:        "Comentário 1 no Post A",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if err != nil || resA1.Comment.Sequence != 1 {
		t.Fatalf("expected Post A seq 1, got %v (err: %v)", resA1.Comment, err)
	}

	// Post B: comment 1 -> seq 1 (scoped independently!)
	resB1, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postB,
		AuthorAccountID: authorID,
		BodyText:        "Comentário 1 no Post B",
		OperationID:     "33333333-3333-4333-8333-000000000002",
	})
	if err != nil || resB1.Comment.Sequence != 1 {
		t.Fatalf("expected Post B seq 1, got %v (err: %v)", resB1.Comment, err)
	}

	// Post A: comment 2 -> seq 2
	resA2, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postA,
		AuthorAccountID: authorID,
		BodyText:        "Comentário 2 no Post A",
		OperationID:     "33333333-3333-4333-8333-000000000003",
	})
	if err != nil || resA2.Comment.Sequence != 2 {
		t.Fatalf("expected Post A seq 2, got %v (err: %v)", resA2.Comment, err)
	}

	// Concurrent creation test on Post A: two concurrent requests receive distinct sequences
	var wg sync.WaitGroup
	results := make([]application.CommentResult, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			results[idx], errs[idx] = store.CreateComment(ctx, application.CreateCommentCommand{
				PostID:          postA,
				AuthorAccountID: authorID,
				BodyText:        fmt.Sprintf("Comentário concorrente %d no Post A", idx),
				OperationID:     fmt.Sprintf("33333333-3333-4333-8333-00000000000%d", idx+4),
			})
		}()
	}
	wg.Wait()

	for i := 0; i < 2; i++ {
		if errs[i] != nil {
			t.Fatalf("concurrent insert %d failed: %v", i, errs[i])
		}
	}

	seqs := map[int64]bool{
		results[0].Comment.Sequence: true,
		results[1].Comment.Sequence: true,
	}
	if len(seqs) != 2 || (!seqs[3] || !seqs[4]) {
		t.Errorf("expected concurrent sequences {3, 4}, got sequences: %v", seqs)
	}
}

func TestComments_IdempotencyAndReplay(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)
	opID := "33333333-3333-4333-8333-000000000001"

	// 1. Initial creation
	res1, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Mensagem original",
		OperationID:     opID,
	})
	if err != nil {
		t.Fatalf("initial create failed: %v", err)
	}
	if res1.Replayed {
		t.Errorf("expected first call Replayed=false")
	}

	// 2. Replay with identical body -> returns existing comment with Replayed=true
	res2, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Mensagem original",
		OperationID:     opID,
	})
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if !res2.Replayed {
		t.Errorf("expected Replayed=true on replay")
	}
	if res2.Comment.ID != res1.Comment.ID {
		t.Errorf("expected same comment ID %s, got %s", res1.Comment.ID, res2.Comment.ID)
	}
	if res2.Comment.Sequence != res1.Comment.Sequence {
		t.Errorf("expected sequence not to advance on replay: %d vs %d", res1.Comment.Sequence, res2.Comment.Sequence)
	}

	// 3. Replay with DIFFERENT body -> ErrConflictingOperation
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Mensagem alterada com mesma operation id",
		OperationID:     opID,
	})
	if !errors.Is(err, domain.ErrConflictingOperation) {
		t.Errorf("expected ErrConflictingOperation, got %v", err)
	}
}

func TestComments_CASEditAndTombstoneRemove(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	author1 := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	author2 := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000002", "author-2")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// Create comment
	res, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: author1,
		BodyText:        "Texto original",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if err != nil {
		t.Fatalf("create comment failed: %v", err)
	}
	commentID := res.Comment.ID

	// Publish comment
	_, err = store.PublishComment(ctx, commentID)
	if err != nil {
		t.Fatalf("publish comment failed: %v", err)
	}

	// 1. Edit with stale expected_revision (comment is now rev 2 after publish)
	_, err = store.EditComment(ctx, application.EditCommentCommand{
		CommentID:        commentID,
		AuthorAccountID:  author1,
		ExpectedRevision: 1, // Stale!
		BodyText:         "Tentando editar com rev antiga",
	})
	if !errors.Is(err, domain.ErrRevisionMismatch) {
		t.Errorf("expected ErrRevisionMismatch, got %v", err)
	}

	// 2. Edit by non-author -> ErrCommentAuthorMismatch
	_, err = store.EditComment(ctx, application.EditCommentCommand{
		CommentID:        commentID,
		AuthorAccountID:  author2,
		ExpectedRevision: 2,
		BodyText:         "Tentativa de edição por outro autor",
	})
	if !errors.Is(err, domain.ErrCommentAuthorMismatch) {
		t.Errorf("expected ErrCommentAuthorMismatch, got %v", err)
	}

	// 3. Valid edit -> moves state back to PENDING_REVIEW, bumps revision to 3
	edited, err := store.EditComment(ctx, application.EditCommentCommand{
		CommentID:        commentID,
		AuthorAccountID:  author1,
		ExpectedRevision: 2,
		BodyText:         "Texto corrigido e atualizado",
	})
	if err != nil {
		t.Fatalf("valid edit failed: %v", err)
	}
	if edited.Revision != 3 {
		t.Errorf("expected revision 3, got %d", edited.Revision)
	}
	if edited.PublicationState != domain.StatePendingReview {
		t.Errorf("expected state to return to pending_review, got %s", edited.PublicationState)
	}

	// 4. Remove comment (tombstone) with matching expected_revision 3
	removed, err := store.RemoveComment(ctx, application.RemoveCommentCommand{
		CommentID:        commentID,
		AuthorAccountID:  author1,
		ExpectedRevision: 3,
	})
	if err != nil {
		t.Fatalf("remove comment failed: %v", err)
	}
	if removed.Revision != 4 {
		t.Errorf("expected revision 4, got %d", removed.Revision)
	}
	if removed.PublicationState != domain.StateRemoved || removed.Visibility != domain.VisibilityHidden || removed.BodyText != "" {
		t.Errorf("expected clean tombstone (removed, hidden, empty body), got %+v", removed)
	}

	// 5. Subsequent edit on removed comment -> ErrRemovedIsTerminal
	_, err = store.EditComment(ctx, application.EditCommentCommand{
		CommentID:        commentID,
		AuthorAccountID:  author1,
		ExpectedRevision: 4,
		BodyText:         "Tentando ressuscitar comentário removido",
	})
	if !errors.Is(err, domain.ErrRemovedIsTerminal) {
		t.Errorf("expected ErrRemovedIsTerminal, got %v", err)
	}
}

func TestComments_RateQuota(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	// Configure limiter with tiny policy (2 per hour limit)
	policy := identitydomain.QuotaPolicy{
		Version: 1,
		Ops: map[string]identitydomain.OperationQuota{
			identitydomain.OperationWrite: {Limit: 2, Window: time.Hour},
		},
	}
	limiter := identityadapters.NewLimiter(pool, policy)
	store.SetQuotaChecker(limiter)

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// Comment 1 -> OK
	_, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário 1",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if err != nil {
		t.Fatalf("comment 1 failed: %v", err)
	}

	// Comment 2 -> OK
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário 2",
		OperationID:     "33333333-3333-4333-8333-000000000002",
	})
	if err != nil {
		t.Fatalf("comment 2 failed: %v", err)
	}

	// Comment 3 -> Exceeds hourly limit 2 -> denied with QuotaError
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário 3 (excede cota)",
		OperationID:     "33333333-3333-4333-8333-000000000003",
	})
	if err == nil {
		t.Fatal("expected quota error on 3rd comment, got nil")
	}
	var quotaErr *identityadapters.QuotaError
	if !errors.As(err, &quotaErr) {
		t.Errorf("expected *identityadapters.QuotaError, got %v", err)
	} else if quotaErr.RetryAfter <= 0 {
		t.Errorf("expected positive RetryAfter, got %v", quotaErr.RetryAfter)
	}
}

func TestComments_PrivacyInvariant_NoLeakBeforeApproval(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// Create comment -> starts in PENDING_REVIEW
	res, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário submetido aguardando aprovação editorial",
		OperationID:     "33333333-3333-4333-8333-000000000001",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	commentID := res.Comment.ID

	// Public query: ListCommentsByPost must return 0 comments (no leak before approval!)
	list1, err := store.ListCommentsByPost(ctx, application.ListPostCommentsQuery{
		PostID: postID,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(list1) != 0 {
		t.Errorf("expected 0 published comments, but got %d (privacy leak!)", len(list1))
	}

	// Operator approves comment
	published, err := store.PublishComment(ctx, commentID)
	if err != nil {
		t.Fatalf("publish comment failed: %v", err)
	}
	if published.PublicationState != domain.StatePublished {
		t.Errorf("expected published state, got %s", published.PublicationState)
	}

	// Public query now returns the published comment
	list2, err := store.ListCommentsByPost(ctx, application.ListPostCommentsQuery{
		PostID: postID,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(list2) != 1 || list2[0].ID != commentID {
		t.Errorf("expected exactly comment %s in public list, got %v", commentID, list2)
	}
}

func TestComments_DurableLiveQuota(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()

	authorID := createTestAccount(t, pool, "11111111-1111-4111-8111-000000000001", "author-1")
	postID := createTestPost(t, pool, "22222222-2222-4222-8222-000000000001", "Post A", string(domain.StatePublished), string(domain.VisibilityVisible), true)

	// Pre-insert 99 live comments directly for this author
	for i := 1; i <= 99; i++ {
		cid := fmt.Sprintf("55555555-5555-4555-8555-%012d", i)
		opid := fmt.Sprintf("66666666-6666-4666-8666-%012d", i)
		_, err := pool.Exec(ctx, `
			INSERT INTO forum_comments (
				id, post_id, author_account_id, public_alias, depth, sequence,
				body_text, revision, publication_state, visibility, created_at, updated_at,
				published_at, create_operation_id, body_sha256
			) VALUES (
				$1, $2, $3, 'Author', 0, $4,
				'Comment', 1, 'PUBLISHED', 'VISIBLE', now(), now(),
				now(), $5, 'sha'
			)
		`, cid, postID, authorID, i+10, opid)
		if err != nil {
			t.Fatalf("failed to insert live comment %d: %v", i, err)
		}
	}

	// 100th comment -> OK
	res100, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário número 100",
		OperationID:     "77777777-7777-4777-8777-000000000100",
	})
	if err != nil {
		t.Fatalf("comment 100 failed: %v", err)
	}

	// 101st comment -> ErrCommentLiveQuotaExceeded
	_, err = store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário número 101 (deve falhar)",
		OperationID:     "77777777-7777-4777-8777-000000000101",
	})
	if !errors.Is(err, domain.ErrCommentLiveQuotaExceeded) {
		t.Fatalf("expected ErrCommentLiveQuotaExceeded, got %v", err)
	}

	// Remove comment 100 -> releases 1 live slot
	_, err = store.RemoveComment(ctx, application.RemoveCommentCommand{
		CommentID:        res100.Comment.ID,
		AuthorAccountID:  authorID,
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("remove comment 100 failed: %v", err)
	}

	// Now 101st comment succeeds
	res101, err := store.CreateComment(ctx, application.CreateCommentCommand{
		PostID:          postID,
		AuthorAccountID: authorID,
		BodyText:        "Comentário após liberação de cota viva",
		OperationID:     "77777777-7777-4777-8777-000000000101",
	})
	if err != nil {
		t.Fatalf("comment after remove failed: %v", err)
	}
	if res101.Comment == nil {
		t.Errorf("expected created comment after remove")
	}
}
