//go:build integration

package adapters_test

import (
	"context"
	"errors"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type for22Env struct {
	pool  *pgxpool.Pool
	store *forumadapters.Store
}

func setupFOR22Env(t *testing.T) *for22Env {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer conn.Close(ctx)

	name := fmt.Sprintf("forum_for22_%d", time.Now().UnixNano())
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
	return &for22Env{
		pool:  pool,
		store: store,
	}
}

func seedAccount(t *testing.T, pool *pgxpool.Pool, accountID, alias string) {
	t.Helper()
	ctx := context.Background()
	q := `INSERT INTO accounts (id, alias, status) VALUES ($1, $2, 'active') ON CONFLICT (id) DO NOTHING`
	if _, err := pool.Exec(ctx, q, accountID, alias); err != nil {
		t.Fatalf("seed account %s: %v", accountID, err)
	}
}

func publishTopic(ctx context.Context, pool *pgxpool.Pool, topicID string) error {
	now := time.Now().UTC()
	_, err := pool.Exec(ctx, `
		UPDATE forum_posts
		SET publication_state = 'PUBLISHED',
		    first_published_at = COALESCE(first_published_at, $2),
		    first_publication_seq = CASE WHEN first_publication_seq = 0 THEN 1 ELSE first_publication_seq END,
		    local_updated_at = $2
		WHERE id = $1 AND publication_state <> 'PUBLISHED'
	`, topicID, now)
	return err
}

func TestFOR22_TopicPrivacyAndModerationRecovery_Integration(t *testing.T) {
	env := setupFOR22Env(t)
	ctx := context.Background()

	authorA := "11111111-2222-4333-8444-000000000001"
	authorB := "22222222-3333-4444-8555-000000000002"
	authorC := "33333333-4444-4555-8666-000000000003"
	authorD := "44444444-5555-4666-8777-000000000004"

	seedAccount(t, env.pool, authorA, "authorA")
	seedAccount(t, env.pool, authorB, "authorB")
	seedAccount(t, env.pool, authorC, "authorC")
	seedAccount(t, env.pool, authorD, "authorD")

	aliasA := "Alice"
	aliasB := "Bob"
	aliasC := "Charlie"
	aliasD := "David"

	// =========================================================================
	// Gate 1: Edited & Hidden Topic Erasure
	// =========================================================================
	var gate1TopicID string
	t.Run("Gate1_EditedAndHiddenTopicErasure", func(t *testing.T) {
		// 1. Author A creates topic
		res, err := env.store.CreateTopic(ctx, application.CreateTopicCommand{
			AuthorAccountID: authorA,
			Category:        domain.CategoryFuels,
			Title:           "Preços do Etanol no Interior de SP",
			BodyText:        "Texto com detalhes sensíveis e opiniões do autor sobre preços.",
			PublicAlias:     &aliasA,
			OperationID:     "aaaaaaaa-1111-4111-8111-000000000001",
		})
		if err != nil {
			t.Fatalf("create topic failed: %v", err)
		}
		gate1TopicID = res.Topic.ID

		// 2. Operator publishes topic
		if err := publishTopic(ctx, env.pool, gate1TopicID); err != nil {
			t.Fatalf("publish topic failed: %v", err)
		}

		// 3. Author A edits topic (CAS revision 1 -> 2, reverts to PENDING_REVIEW)
		edited, err := env.store.EditTopic(ctx, application.EditTopicCommand{
			TopicID:          gate1TopicID,
			AuthorAccountID:  authorA,
			ExpectedRevision: 1,
			Category:         domain.CategoryFuels,
			Title:            "Preços do Etanol no Interior de SP (Atualizado)",
			BodyText:         "Texto editado com dados atualizados.",
		})
		if err != nil {
			t.Fatalf("edit topic failed: %v", err)
		}
		if edited.Revision != 2 || edited.PublicationState != domain.StatePendingReview {
			t.Fatalf("expected revision 2 and PENDING_REVIEW, got rev %d, state %s", edited.Revision, edited.PublicationState)
		}

		// 4. Operator approves edit
		if err := publishTopic(ctx, env.pool, gate1TopicID); err != nil {
			t.Fatalf("publish edited topic failed: %v", err)
		}

		// 5. Moderation hides topic
		if err := env.store.SetPostVisibility(ctx, gate1TopicID, domain.VisibilityHidden); err != nil {
			t.Fatalf("hide topic failed: %v", err)
		}

		// Verify topic is hidden
		hiddenTopic, err := env.store.GetTopicByID(ctx, gate1TopicID)
		if err != nil {
			t.Fatalf("get hidden topic: %v", err)
		}
		if hiddenTopic.Visibility != domain.VisibilityHidden {
			t.Fatalf("expected HIDDEN visibility, got %s", hiddenTopic.Visibility)
		}

		// 6. Author A requests LGPD account erasure
		done, n, err := env.store.EraseAccountBatch(ctx, authorA, 50)
		if err != nil {
			t.Fatalf("erase account batch: %v", err)
		}
		if !done || n != 1 {
			t.Fatalf("expected done=true, count=1, got done=%v, count=%d", done, n)
		}

		// 7. Verify DB row scrubbed invariants
		var (
			pubState string
			visState string
			title    string
			excerpt  string
			bodyText string
			bodySHA  string
			pubAlias *string
			revision int32
		)
		err = env.pool.QueryRow(ctx, `
			SELECT publication_state, visibility, title, excerpt, body_text, body_sha256, public_alias, revision
			FROM forum_posts WHERE id = $1
		`, gate1TopicID).Scan(&pubState, &visState, &title, &excerpt, &bodyText, &bodySHA, &pubAlias, &revision)
		if err != nil {
			t.Fatalf("query scrubbed topic: %v", err)
		}

		if pubState != string(domain.StateRemoved) {
			t.Errorf("expected publication_state REMOVED, got %s", pubState)
		}
		if visState != string(domain.VisibilityHidden) {
			t.Errorf("expected visibility HIDDEN, got %s", visState)
		}
		if title != "[Tópico removido]" {
			t.Errorf("expected title '[Tópico removido]', got %q", title)
		}
		if excerpt != "" || bodyText != "" || bodySHA != "" {
			t.Errorf("content not scrubbed: excerpt=%q body=%q sha=%q", excerpt, bodyText, bodySHA)
		}
		if pubAlias != nil {
			t.Errorf("expected public_alias NULL, got %v", *pubAlias)
		}

		// 8. Verify ExportAccount for Author A returns scrubbed topic
		envExp, err := application.ExportAccount(ctx, env.store, authorA, time.Now())
		if err != nil {
			t.Fatalf("export account A: %v", err)
		}
		if envExp.Footprint.TotalTopics != 1 {
			t.Fatalf("expected 1 topic in footprint, got %d", envExp.Footprint.TotalTopics)
		}
		expTopic := envExp.Footprint.Topics[0]
		if expTopic.ID != gate1TopicID {
			t.Errorf("expected topic ID %s, got %s", gate1TopicID, expTopic.ID)
		}
		if expTopic.PublicationState != string(domain.StateRemoved) || expTopic.BodyText != "" {
			t.Errorf("expected REMOVED and empty body in export, got state %s, body %q", expTopic.PublicationState, expTopic.BodyText)
		}
	})

	// =========================================================================
	// Gate 2: Child Visibility Under Removed Topic & No Inherited Permissions
	// =========================================================================
	t.Run("Gate2_ChildVisibilityUnderRemovedTopic_NoInheritedPermissions", func(t *testing.T) {
		// 1. Author B creates Topic B
		resTopicB, err := env.store.CreateTopic(ctx, application.CreateTopicCommand{
			AuthorAccountID: authorB,
			Category:        domain.CategoryVehiclesTech,
			Title:           "Discussão Técnica de Motores Flex",
			BodyText:        "Qual a taxa de compressão ideal para rodar 100% no etanol?",
			PublicAlias:     &aliasB,
			OperationID:     "bbbbbbbb-1111-4111-8111-000000000001",
		})
		if err != nil {
			t.Fatalf("create topic B failed: %v", err)
		}
		topicBID := resTopicB.Topic.ID

		// Approve Topic B
		if err := publishTopic(ctx, env.pool, topicBID); err != nil {
			t.Fatalf("publish topic B failed: %v", err)
		}

		// 2. Author C posts root comment C1 under Topic B
		resComC1, err := env.store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          topicBID,
			AuthorAccountID: authorC,
			BodyText:        "Depende do avanço de ignição da central de injeção.",
			PublicAlias:     aliasC,
			OperationID:     "cccccccc-1111-4111-8111-000000000001",
		})
		if err != nil {
			t.Fatalf("create comment C1 failed: %v", err)
		}
		commentC1ID := resComC1.Comment.ID

		// Approve comment C1
		if _, err := env.store.PublishComment(ctx, commentC1ID); err != nil {
			t.Fatalf("publish comment C1 failed: %v", err)
		}

		// 3. Author D replies to comment C1
		resComD1, err := env.store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          topicBID,
			AuthorAccountID: authorD,
			ParentID:        &commentC1ID,
			BodyText:        "Concordo, em motores turbo modernos o etanol brilha.",
			PublicAlias:     aliasD,
			OperationID:     "dddddddd-1111-4111-8111-000000000001",
		})
		if err != nil {
			t.Fatalf("create reply D1 failed: %v", err)
		}
		commentD1ID := resComD1.Comment.ID

		// Approve reply D1
		if _, err := env.store.PublishComment(ctx, commentD1ID); err != nil {
			t.Fatalf("publish reply D1 failed: %v", err)
		}

		// Verify public comments listing returns C1 with reply count = 1
		commentsBefore, err := env.store.ListCommentsByPost(ctx, application.ListPostCommentsQuery{
			PostID: topicBID,
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("list comments before removal: %v", err)
		}
		if len(commentsBefore) != 1 || commentsBefore[0].ID != commentC1ID || commentsBefore[0].ReplyCount != 1 {
			t.Fatalf("expected 1 root comment with 1 reply, got %d comments with replyCount %d", len(commentsBefore), commentsBefore[0].ReplyCount)
		}

		// Verify replies query returns D1
		repliesBefore, err := env.store.ListRepliesByParent(ctx, application.ListCommentRepliesQuery{
			ParentID: commentC1ID,
			Limit:    10,
		})
		if err != nil {
			t.Fatalf("list replies before removal: %v", err)
		}
		if len(repliesBefore) != 1 || repliesBefore[0].ID != commentD1ID {
			t.Fatalf("expected reply D1, got %v", repliesBefore)
		}

		// 4. Topic B is removed by author tombstone
		remTopic, err := env.store.RemoveTopic(ctx, application.RemoveTopicCommand{
			TopicID:          topicBID,
			AuthorAccountID:  authorB,
			ExpectedRevision: 1,
		})
		if err != nil {
			t.Fatalf("remove topic B: %v", err)
		}
		if remTopic.PublicationState != domain.StateRemoved {
			t.Fatalf("expected topic state REMOVED, got %s", remTopic.PublicationState)
		}

		// Invariant A (Child visibility suppression):
		// Public queries for comments under removed topic must return 0 comments!
		commentsAfter, err := env.store.ListCommentsByPost(ctx, application.ListPostCommentsQuery{
			PostID: topicBID,
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("list comments after topic removal failed: %v", err)
		}
		if len(commentsAfter) != 0 {
			t.Errorf("privacy/visibility leak: expected 0 comments for removed topic, got %d", len(commentsAfter))
		}

		// Invariant B (No inherited permissions / Data preservation):
		// Author C's comment C1 was NOT wiped or scrubbed because Topic B was removed
		comC1Raw, err := env.store.GetCommentByID(ctx, commentC1ID)
		if err != nil {
			t.Fatalf("get comment C1: %v", err)
		}
		if comC1Raw.BodyText == "" || comC1Raw.PublicationState != domain.StatePublished {
			t.Errorf("comment C1 unexpectedly mutated: body=%q state=%s", comC1Raw.BodyText, comC1Raw.PublicationState)
		}

		// Invariant C (Account export isolation):
		// Author C's export still contains C1 with full body and correct author
		expC, err := application.ExportAccount(ctx, env.store, authorC, time.Now())
		if err != nil {
			t.Fatalf("export account C: %v", err)
		}
		if expC.Footprint.TotalComments != 1 || expC.Footprint.Comments[0].ID != commentC1ID {
			t.Fatalf("expected comment C1 in author C export, got %+v", expC.Footprint.Comments)
		}
		if expC.Footprint.Comments[0].BodyText != "Depende do avanço de ignição da central de injeção." {
			t.Errorf("expected original body text in author C export, got %q", expC.Footprint.Comments[0].BodyText)
		}

		// Invariant D (Permission boundaries):
		// Author C cannot edit or remove Topic B
		_, err = env.store.EditTopic(ctx, application.EditTopicCommand{
			TopicID:          topicBID,
			AuthorAccountID:  authorC,
			ExpectedRevision: 2,
			Category:         domain.CategoryVehiclesTech,
			Title:            "Tentativa de Hack por Outro Autor",
			BodyText:         "Corpo fraudulento",
		})
		if !errors.Is(err, domain.ErrTopicAuthorMismatch) && !errors.Is(err, domain.ErrTopicAlreadyRemoved) {
			t.Errorf("expected author mismatch or topic already removed, got %v", err)
		}

		// Author B cannot edit Author C's comment
		_, err = env.store.EditComment(ctx, application.EditCommentCommand{
			CommentID:        commentC1ID,
			AuthorAccountID:  authorB,
			ExpectedRevision: 1,
			BodyText:         "Tentativa de alterar comentário alheio",
		})
		if !errors.Is(err, domain.ErrCommentAuthorMismatch) {
			t.Errorf("expected ErrCommentAuthorMismatch, got %v", err)
		}
	})

	// =========================================================================
	// Gate 3: Operator Restore Refusal After Removal
	// =========================================================================
	t.Run("Gate3_OperatorRestoreRefusalAfterRemoval", func(t *testing.T) {
		// Attempt to restore gate1TopicID (which is REMOVED) to VISIBLE
		err := env.store.SetPostVisibility(ctx, gate1TopicID, domain.VisibilityVisible)
		if !errors.Is(err, domain.ErrPostNotFound) {
			t.Fatalf("expected ErrPostNotFound (0 rows updated on terminal REMOVED post), got %v", err)
		}

		// Confirm direct DB state: topic remains REMOVED and HIDDEN
		var state, vis string
		err = env.pool.QueryRow(ctx, "SELECT publication_state, visibility FROM forum_posts WHERE id = $1", gate1TopicID).Scan(&state, &vis)
		if err != nil {
			t.Fatalf("query post: %v", err)
		}
		if state != string(domain.StateRemoved) || vis != string(domain.VisibilityHidden) {
			t.Fatalf("security violation: topic restored from removed terminal state! state=%s vis=%s", state, vis)
		}
	})

	// =========================================================================
	// Gate 4: Metadata-Only Audit Verification
	// =========================================================================
	t.Run("Gate4_MetadataOnlyAuditVerification", func(t *testing.T) {
		rows, err := env.pool.Query(ctx, `
			SELECT target_type, target_id, revision, action, actor_kind, admin_audit_ref, reason_code, prior_state, new_state
			FROM forum_changes
			ORDER BY created_at ASC
		`)
		if err != nil {
			t.Fatalf("query forum_changes: %v", err)
		}
		defer rows.Close()

		foundErasureTopicAudit := false
		changeCount := 0

		for rows.Next() {
			var (
				targetType string
				targetID   string
				revision   int32
				action     string
				actorKind  string
				auditRef   *string
				reasonCode *string
				priorState *string
				newState   *string
			)
			if err := rows.Scan(&targetType, &targetID, &revision, &action, &actorKind, &auditRef, &reasonCode, &priorState, &newState); err != nil {
				t.Fatalf("scan change: %v", err)
			}
			changeCount++

			if targetType != "POST" && targetType != "COMMENT" {
				t.Errorf("unexpected target_type: %s", targetType)
			}
			if targetType == "POST" && action == "REMOVE" && actorKind == "SYSTEM" {
				if auditRef != nil && *auditRef == "account-erasure" && reasonCode != nil && *reasonCode == "erasure-scrub" {
					foundErasureTopicAudit = true
				}
			}
		}

		if changeCount == 0 {
			t.Fatalf("no audit changes recorded in forum_changes")
		}
		if !foundErasureTopicAudit {
			t.Errorf("did not find expected SYSTEM erasure-scrub audit entry for topic removal in forum_changes")
		}

		// Confirm schema of forum_changes has no content columns (title, body, alias, pii)
		columnQuery := `
			SELECT column_name
			FROM information_schema.columns
			WHERE table_name = 'forum_changes'
		`
		colRows, err := env.pool.Query(ctx, columnQuery)
		if err != nil {
			t.Fatalf("query columns: %v", err)
		}
		defer colRows.Close()

		forbiddenCols := map[string]bool{
			"title":        true,
			"body":         true,
			"body_text":    true,
			"excerpt":      true,
			"public_alias": true,
			"email":        true,
			"name":         true,
		}

		for colRows.Next() {
			var col string
			if err := colRows.Scan(&col); err != nil {
				t.Fatalf("scan column: %v", err)
			}
			if forbiddenCols[col] {
				t.Errorf("forbidden PII/content column %q found in forum_changes table!", col)
			}
		}
	})

	// =========================================================================
	// Gate 5: Worker Restart & Batched Erasure Continuation
	// =========================================================================
	authorE := "55555555-6666-4777-8888-000000000005"
	seedAccount(t, env.pool, authorE, "authorE")

	t.Run("Gate5_WorkerRestartAndBatchedErasureContinuation", func(t *testing.T) {
		// Author E creates 2 topics and 2 comments (4 items total)
		tE1, err := env.store.CreateTopic(ctx, application.CreateTopicCommand{
			AuthorAccountID: authorE,
			Category:        domain.CategoryFuels,
			Title:           "Topico E1",
			BodyText:        "Corpo E1",
			OperationID:     "eeeeeeee-0001-4111-8111-000000000001",
		})
		if err != nil {
			t.Fatalf("create E1: %v", err)
		}
		tE2, err := env.store.CreateTopic(ctx, application.CreateTopicCommand{
			AuthorAccountID: authorE,
			Category:        domain.CategoryFuels,
			Title:           "Topico E2",
			BodyText:        "Corpo E2",
			OperationID:     "eeeeeeee-0002-4111-8111-000000000002",
		})
		if err != nil {
			t.Fatalf("create E2: %v", err)
		}

		if err := publishTopic(ctx, env.pool, tE1.Topic.ID); err != nil {
			t.Fatalf("publish tE1: %v", err)
		}

		cE1, err := env.store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          tE1.Topic.ID,
			AuthorAccountID: authorE,
			BodyText:        "Comentario E1",
			OperationID:     "eeeeeeee-0003-4111-8111-000000000003",
		})
		if err != nil {
			t.Fatalf("create comment E1: %v", err)
		}
		cE2, err := env.store.CreateComment(ctx, application.CreateCommentCommand{
			PostID:          tE1.Topic.ID,
			AuthorAccountID: authorE,
			BodyText:        "Comentario E2",
			OperationID:     "eeeeeeee-0004-4111-8111-000000000004",
		})
		if err != nil {
			t.Fatalf("create comment E2: %v", err)
		}

		// Batch 1 (limit 1): erases 1 topic, returns done=false
		done1, n1, err := env.store.EraseAccountBatch(ctx, authorE, 1)
		if err != nil {
			t.Fatalf("batch 1 failed: %v", err)
		}
		if done1 != false || n1 != 1 {
			t.Fatalf("expected done=false, count=1 for batch 1, got done=%v, count=%d", done1, n1)
		}

		// Simulate worker restart (new Store instance on same pool)
		workerStore := forumadapters.NewStore(env.pool)

		// Batch 2 (limit 2): erases remaining 1 topic and 1 comment, returns done=false
		done2, n2, err := workerStore.EraseAccountBatch(ctx, authorE, 2)
		if err != nil {
			t.Fatalf("batch 2 failed: %v", err)
		}
		if done2 != false || n2 != 2 {
			t.Fatalf("expected done=false, count=2 for batch 2, got done=%v, count=%d", done2, n2)
		}

		// Batch 3 (limit 2): erases last comment, returns done=true
		done3, n3, err := workerStore.EraseAccountBatch(ctx, authorE, 2)
		if err != nil {
			t.Fatalf("batch 3 failed: %v", err)
		}
		if done3 != true || n3 != 1 {
			t.Fatalf("expected done=true, count=1 for batch 3, got done=%v, count=%d", done3, n3)
		}

		// Verify all 4 items are in REMOVED state and have empty body
		for _, tid := range []string{tE1.Topic.ID, tE2.Topic.ID} {
			top, err := env.store.GetTopicByID(ctx, tid)
			if err != nil {
				t.Fatalf("get topic %s: %v", tid, err)
			}
			if top.PublicationState != domain.StateRemoved || top.BodyText != "" {
				t.Errorf("topic %s not properly erased: state=%s body=%q", tid, top.PublicationState, top.BodyText)
			}
		}
		for _, cid := range []string{cE1.Comment.ID, cE2.Comment.ID} {
			c, err := env.store.GetCommentByID(ctx, cid)
			if err != nil {
				t.Fatalf("get comment %s: %v", cid, err)
			}
			if c.PublicationState != domain.StateRemoved || c.BodyText != "" {
				t.Errorf("comment %s not properly erased: state=%s body=%q", cid, c.PublicationState, c.BodyText)
			}
		}
	})

	// =========================================================================
	// Gate 6: Restored Deletion Replay & Idempotence
	// =========================================================================
	t.Run("Gate6_RestoredDeletionReplayAndTerminalState", func(t *testing.T) {
		// Replaying EraseAccountBatch on Author E (already erased)
		done, n, err := env.store.EraseAccountBatch(ctx, authorE, 10)
		if err != nil {
			t.Fatalf("idempotent erase replay failed: %v", err)
		}
		if done != true || n != 0 {
			t.Fatalf("expected done=true, count=0 on replay, got done=%v count=%d", done, n)
		}

		// Replaying RemoveTopic on already removed topic returns ErrTopicAlreadyRemoved
		_, err = env.store.RemoveTopic(ctx, application.RemoveTopicCommand{
			TopicID:          gate1TopicID,
			AuthorAccountID:  authorA,
			ExpectedRevision: 2,
		})
		if !errors.Is(err, domain.ErrTopicAlreadyRemoved) {
			t.Fatalf("expected ErrTopicAlreadyRemoved on removed topic, got %v", err)
		}

		// Replaying EditTopic on already removed topic returns ErrTopicAlreadyRemoved
		_, err = env.store.EditTopic(ctx, application.EditTopicCommand{
			TopicID:          gate1TopicID,
			AuthorAccountID:  authorA,
			ExpectedRevision: 2,
			Category:         domain.CategoryFuels,
			Title:            "Edit Tentativa",
			BodyText:         "Novo corpo",
		})
		if !errors.Is(err, domain.ErrTopicAlreadyRemoved) {
			t.Fatalf("expected ErrTopicAlreadyRemoved on edit removed topic, got %v", err)
		}
	})
}
