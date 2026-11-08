//go:build integration

package adapters_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	qualityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type for16Env struct {
	pool  *pgxpool.Pool
	store *forumadapters.Store
}

func setupFOR16Env(t *testing.T) *for16Env {
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

	name := fmt.Sprintf("forum_for16_%d", time.Now().UnixNano())
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
	return &for16Env{
		pool:  pool,
		store: store,
	}
}

func seedPost(t *testing.T, pool *pgxpool.Pool, postID string) {
	t.Helper()
	sourceID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	q := `INSERT INTO forum_posts(
		id, kind, category, title, excerpt, body_text, publication_state, visibility,
		first_publication_seq, source_id, comments_enabled, local_created_at, local_updated_at,
		first_published_at, next_comment_sequence
	) VALUES (
		$1, 'EDITORIAL', 'FUELS', 'Post para teste de apagamento', 'Resumo', 'Corpo',
		'PUBLISHED', 'VISIBLE', 1, $2, true, $3, $3, $3, 1
	) ON CONFLICT (id) DO NOTHING`
	if _, err := pool.Exec(context.Background(), q, postID, sourceID, now); err != nil {
		t.Fatalf("seed post: %v", err)
	}
}

func seedComment(t *testing.T, pool *pgxpool.Pool, commentID, postID, authorID string, parentID *string, seq int64, state string, body string) {
	t.Helper()
	now := time.Now().UTC()
	opID := fmt.Sprintf("55555555-%s-5555-5555-%s", commentID[:4], commentID[len(commentID)-12:])
	var depth int16
	if parentID != nil {
		depth = 1
	}
	var pubAt *time.Time
	if state == "PUBLISHED" {
		pubAt = &now
	}
	q := `INSERT INTO forum_comments(
		id, post_id, author_account_id, public_alias, parent_id, depth, sequence,
		body_text, revision, publication_state, visibility, created_at, updated_at,
		published_at, create_operation_id, body_sha256
	) VALUES (
		$1, $2, $3, 'Alias', $4, $5, $6,
		$7, 1, $8, 'VISIBLE', $9, $9,
		$10, $11, 'dummy-sha'
	)`
	if _, err := pool.Exec(context.Background(), q, commentID, postID, authorID, parentID, depth, seq, body, state, now, pubAt, opID); err != nil {
		t.Fatalf("seed comment %s: %v", commentID, err)
	}
}

func TestForumErasure_PostgresIntegration_Gates(t *testing.T) {
	env := setupFOR16Env(t)
	ctx := context.Background()

	authorA := "11111111-aaaa-4111-8111-000000000001"
	authorB := "22222222-bbbb-4222-8222-000000000002"

	// Seed author accounts
	if _, err := env.pool.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'aliasA','active'),($2,'aliasB','active')", authorA, authorB); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	post1 := "99999999-0001-4999-8999-000000000001"
	post2 := "99999999-0002-4999-8999-000000000002"
	seedPost(t, env.pool, post1)
	seedPost(t, env.pool, post2)

	// Seed 5 comments for Author A:
	// - commentA1 on post1 (PUBLISHED)
	// - commentA2 reply to commentA1 on post1 (PENDING_REVIEW)
	// - commentA3 on post2 (PUBLISHED)
	// - commentA4 on post2 (PENDING_REVIEW)
	// - commentA5 on post1 (PUBLISHED)
	cA1 := "aaaaaaaa-0001-4aaa-8aaa-000000000001"
	cA2 := "aaaaaaaa-0002-4aaa-8aaa-000000000002"
	cA3 := "aaaaaaaa-0003-4aaa-8aaa-000000000003"
	cA4 := "aaaaaaaa-0004-4aaa-8aaa-000000000004"
	cA5 := "aaaaaaaa-0005-4aaa-8aaa-000000000005"
	seedComment(t, env.pool, cA1, post1, authorA, nil, 1, "PUBLISHED", "Texto confidencial A1")
	seedComment(t, env.pool, cA2, post1, authorA, &cA1, 2, "PENDING_REVIEW", "Texto confidencial A2")
	seedComment(t, env.pool, cA3, post2, authorA, nil, 1, "PUBLISHED", "Texto confidencial A3")
	seedComment(t, env.pool, cA4, post2, authorA, nil, 2, "PENDING_REVIEW", "Texto confidencial A4")
	seedComment(t, env.pool, cA5, post1, authorA, nil, 3, "PUBLISHED", "Texto confidencial A5")

	// Seed 2 comments for Author B (Control / foreign isolation)
	cB1 := "bbbbbbbb-0001-4bbb-8bbb-000000000001"
	cB2 := "bbbbbbbb-0002-4bbb-8bbb-000000000002"
	seedComment(t, env.pool, cB1, post1, authorB, nil, 4, "PUBLISHED", "Texto intocado do autor B")
	seedComment(t, env.pool, cB2, post2, authorB, nil, 3, "PUBLISHED", "Texto intocado 2 do autor B")

	// -------------------------------------------------------------
	// Gate 1: Export before erasure reflects full author footprint
	// -------------------------------------------------------------
	t.Run("gate1_export_before_erasure", func(t *testing.T) {
		envExp, err := application.ExportAccount(ctx, env.store, authorA, time.Now())
		if err != nil {
			t.Fatalf("export account A: %v", err)
		}
		if envExp.Footprint.TotalComments != 5 {
			t.Errorf("expected 5 comments before erasure, got %d", envExp.Footprint.TotalComments)
		}
		// Confirm all 5 comment IDs are present
		ids := make(map[string]bool)
		for _, c := range envExp.Footprint.Comments {
			ids[c.ID] = true
			if c.BodyText == "" {
				t.Errorf("comment %s has unexpectedly empty body text before erasure", c.ID)
			}
		}
		if !ids[cA1] || !ids[cA2] || !ids[cA3] || !ids[cA4] || !ids[cA5] {
			t.Errorf("missing comment IDs in author A export: %+v", ids)
		}
		// Confirm author B comments are NOT present
		if ids[cB1] || ids[cB2] {
			t.Errorf("foreign author B comments found in author A export!")
		}
	})

	// -------------------------------------------------------------
	// Gate 2: Batched erasure convergence (limit = 2 per batch)
	// -------------------------------------------------------------
	t.Run("gate2_batched_erasure_convergence", func(t *testing.T) {
		// Batch 1: limit 2 -> scrubs 2, returns done=false
		done1, n1, err := env.store.EraseAccountBatch(ctx, authorA, 2)
		if err != nil {
			t.Fatalf("batch 1 failed: %v", err)
		}
		if done1 != false || n1 != 2 {
			t.Fatalf("expected done=false, count=2 for batch 1, got done=%v, count=%d", done1, n1)
		}

		// Batch 2: limit 2 -> scrubs 2, returns done=false
		done2, n2, err := env.store.EraseAccountBatch(ctx, authorA, 2)
		if err != nil {
			t.Fatalf("batch 2 failed: %v", err)
		}
		if done2 != false || n2 != 2 {
			t.Fatalf("expected done=false, count=2 for batch 2, got done=%v, count=%d", done2, n2)
		}

		// Batch 3: limit 2 -> scrubs remaining 1, returns done=true
		done3, n3, err := env.store.EraseAccountBatch(ctx, authorA, 2)
		if err != nil {
			t.Fatalf("batch 3 failed: %v", err)
		}
		if done3 != true || n3 != 1 {
			t.Fatalf("expected done=true, count=1 for batch 3, got done=%v, count=%d", done3, n3)
		}

		// Batch 4 (idempotent replay): returns done=true, count=0
		done4, n4, err := env.store.EraseAccountBatch(ctx, authorA, 2)
		if err != nil {
			t.Fatalf("batch 4 failed: %v", err)
		}
		if done4 != true || n4 != 0 {
			t.Fatalf("expected done=true, count=0 for idempotent rerun, got done=%v, count=%d", done4, n4)
		}
	})

	// -------------------------------------------------------------
	// Gate 3: Original text scrub & tombstone invariants
	// -------------------------------------------------------------
	t.Run("gate3_original_text_scrubbed_invariants", func(t *testing.T) {
		comments, err := env.store.ListCommentsByAccount(ctx, authorA)
		if err != nil {
			t.Fatalf("list comments by author A: %v", err)
		}
		if len(comments) != 5 {
			t.Fatalf("expected 5 historical comments, got %d", len(comments))
		}
		for _, c := range comments {
			if c.BodyText != "" {
				t.Errorf("security invariant violated: comment %s retained body text %q after erasure", c.ID, c.BodyText)
			}
			if c.PublicationState != domain.StateRemoved {
				t.Errorf("expected REMOVED state for %s, got %s", c.ID, c.PublicationState)
			}
			if c.Visibility != domain.VisibilityHidden {
				t.Errorf("expected HIDDEN visibility for %s, got %s", c.ID, c.Visibility)
			}
		}

		// Verify change tracking audit was recorded for each scrub
		var changesCount int
		err = env.pool.QueryRow(ctx,
			"SELECT count(*) FROM forum_changes WHERE target_type = 'COMMENT' AND action = 'REMOVE' AND actor_account_id = $1",
			authorA).Scan(&changesCount)
		if err != nil {
			t.Fatalf("query changes: %v", err)
		}
		if changesCount != 5 {
			t.Errorf("expected 5 change records for erased comments, got %d", changesCount)
		}
	})

	// -------------------------------------------------------------
	// Gate 4: Foreign author B completely untouched
	// -------------------------------------------------------------
	t.Run("gate4_foreign_author_untouched", func(t *testing.T) {
		commentsB, err := env.store.ListCommentsByAccount(ctx, authorB)
		if err != nil {
			t.Fatalf("list comments by author B: %v", err)
		}
		if len(commentsB) != 2 {
			t.Fatalf("expected 2 comments for author B, got %d", len(commentsB))
		}
		for _, c := range commentsB {
			if c.BodyText == "" {
				t.Errorf("author B comment %s was incorrectly scrubbed!", c.ID)
			}
			if c.PublicationState != domain.StatePublished || c.Visibility != domain.VisibilityVisible {
				t.Errorf("author B comment %s state corrupted: %s / %s", c.ID, c.PublicationState, c.Visibility)
			}
		}
	})

	// -------------------------------------------------------------
	// Gate 5: Export after erasure reflects empty scrubbed text
	// -------------------------------------------------------------
	t.Run("gate5_export_after_erasure", func(t *testing.T) {
		envExp, err := application.ExportAccount(ctx, env.store, authorA, time.Now())
		if err != nil {
			t.Fatalf("export after erasure: %v", err)
		}
		for _, c := range envExp.Footprint.Comments {
			if c.BodyText != "" {
				t.Errorf("export after erasure leaks body text: %+v", c)
			}
			if c.PublicationState != "REMOVED" {
				t.Errorf("export after erasure state mismatch: %+v", c)
			}
		}
	})

	// -------------------------------------------------------------
	// Gate 6: ErasureRecovery reconciliation coordination
	// -------------------------------------------------------------
	t.Run("gate6_erasure_recovery_coordination", func(t *testing.T) {
		authorC := "33333333-cccc-4333-8333-000000000003"
		if _, err := env.pool.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'aliasC','active')", authorC); err != nil {
			t.Fatalf("seed account C: %v", err)
		}
		cC1 := "cccccccc-0001-4ccc-8ccc-000000000001"
		cC2 := "cccccccc-0002-4ccc-8ccc-000000000002"
		seedComment(t, env.pool, cC1, post1, authorC, nil, 10, "PUBLISHED", "Texto C1")
		seedComment(t, env.pool, cC2, post1, authorC, nil, 11, "PUBLISHED", "Texto C2")

		feedbackStore := feedbackadapters.NewPGStore(env.pool)
		qualityStore := &qualityadapters.ReportStore{Pool: env.pool, Now: time.Now}

		accountStore := accountadapters.NewPGStore(env.pool)

		// Insert account erasure request for Author C
		if _, err := env.pool.Exec(ctx,
			"INSERT INTO account_erasure_requests(account_id, requested_at, next_attempt_at) VALUES($1, now() - interval '10 seconds', now() - interval '10 seconds')", authorC); err != nil {
			t.Fatalf("insert erasure request: %v", err)
		}

		nonceCounter := 0
		newNonce := func() (string, error) {
			nonceCounter++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", nonceCounter), nil
		}

		// Define ErasureRecovery wiring forumStore into EraseFeedback
		var forumCallCount int
		recovery := accountapp.ErasureRecovery{
			Store:    accountStore,
			NewNonce: newNonce,
			Now:      time.Now,
			EraseFeedback: func(ctx context.Context, id string) error {
				qDone, qErr := qualityStore.EraseAccountBatch(ctx, id)
				if qErr != nil {
					return qErr
				}
				if !qDone {
					return accountapp.ErrErasurePending
				}

				fbDone, fbErr := feedbackStore.EraseAccountBatch(ctx, id, time.Now().Unix())
				if fbErr != nil {
					return fbErr
				}
				if !fbDone {
					return accountapp.ErrErasurePending
				}

				forumCallCount++
				// Simulate batching with limit = 1 so first attempt returns ErrErasurePending
				fDone, _, fErr := env.store.EraseAccountBatch(ctx, id, 1)
				if fErr != nil {
					return fErr
				}
				if !fDone {
					return accountapp.ErrErasurePending
				}
				return nil
			},
			EraseContributor: func(ctx context.Context, accountID, contributorID string) error {
				return nil
			},
		}

		// Reconcile pass 1: scrubs cC1 (1 of 2), EraseFeedback returns ErrErasurePending
		err := recovery.Reconcile(ctx)
		if err == nil {
			t.Fatalf("expected ErrErasurePending on first pass, got nil")
		}

		// Reset lease in account_erasure_requests to simulate lease expiry for pass 2
		if _, err := env.pool.Exec(ctx,
			"UPDATE account_erasure_requests SET lease_nonce = NULL, lease_until = NULL, next_attempt_at = now() - interval '10 seconds' WHERE account_id = $1", authorC); err != nil {
			t.Fatalf("reset lease: %v", err)
		}

		// Reconcile pass 2: scrubs cC2 (2 of 2), EraseFeedback completes -> Reconcile finishes
		err = recovery.Reconcile(ctx)
		if err != nil {
			t.Fatalf("expected Reconcile to succeed on second pass, got %v", err)
		}

		// Verify Author C comments are fully scrubbed
		commentsC, err := env.store.ListCommentsByAccount(ctx, authorC)
		if err != nil {
			t.Fatalf("list comments C: %v", err)
		}
		for _, c := range commentsC {
			if c.BodyText != "" || c.PublicationState != domain.StateRemoved {
				t.Errorf("comment %s not scrubbed: %+v", c.ID, c)
			}
		}
	})
}

func TestForumErasure_ScrubsReportDetail(t *testing.T) {
	env := setupFOR16Env(t)
	ctx := context.Background()

	modStore := moderationadapters.NewStore(env.pool)
	caseSeq := 0
	env.store.CaseOpener = func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (string, bool, error) {
		caseSeq++
		c, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID: fmt.Sprintf("cccccccc-0000-4ccc-8ccc-%012d", caseSeq), TargetType: targetType, TargetID: targetID,
			Priority: moderationdomain.PriorityP2, Reason: reason, Detail: detail,
			OpenedAt: time.Now(), PolicyVersion: moderationdomain.PolicyV1,
		})
		if err != nil {
			return "", false, err
		}
		return modStore.OpenCaseTx(ctx, tx, c)
	}

	reporterR := "33333333-cccc-4333-8333-000000000003"
	reporterC := "44444444-dddd-4444-8444-000000000004"
	if _, err := env.pool.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'aliasR','active'),($2,'aliasC','active')", reporterR, reporterC); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
	post := "99999999-0003-4999-8999-000000000003"
	seedPost(t, env.pool, post)

	const detailR = "Texto privado do denunciante R"
	const detailC = "Texto do denunciante C"
	for _, rep := range []struct{ account, detail string }{{reporterR, detailR}, {reporterC, detailC}} {
		if _, err := env.store.Report(ctx, application.ReportCommand{
			ReporterAccountID: rep.account, TargetType: "FORUM_POST", TargetID: post,
			Reason: "SPAM", Detail: rep.detail,
		}); err != nil {
			t.Fatalf("report by %s: %v", rep.account, err)
		}
	}

	// The immutable moderation case never receives reporter free text.
	var caseDetail string
	if err := env.pool.QueryRow(ctx, "SELECT detail FROM moderation_cases WHERE target_type='FORUM_POST' AND target_id=$1", post).Scan(&caseDetail); err != nil {
		t.Fatalf("load case: %v", err)
	}
	if strings.Contains(caseDetail, detailR) || strings.Contains(caseDetail, detailC) {
		t.Fatalf("moderation case copied reporter text: %q", caseDetail)
	}

	done, _, err := env.store.EraseAccountBatch(ctx, reporterR, 100)
	if err != nil || !done {
		t.Fatalf("erase reporter R: done=%v err=%v", done, err)
	}

	details := map[string]string{}
	rows, err := env.pool.Query(ctx, "SELECT account_id::text, detail FROM forum_reports WHERE target_id=$1", post)
	if err != nil {
		t.Fatalf("load reports: %v", err)
	}
	for rows.Next() {
		var account, detail string
		if err := rows.Scan(&account, &detail); err != nil {
			t.Fatalf("scan report: %v", err)
		}
		details[account] = detail
	}
	rows.Close()
	if details[reporterR] != "" {
		t.Fatalf("erased reporter text survived: %q", details[reporterR])
	}
	if details[reporterC] != detailC {
		t.Fatalf("other reporter text must stay, got %q", details[reporterC])
	}

	// Erasing the other reporter leaves no text and stays idempotent.
	if done, _, err := env.store.EraseAccountBatch(ctx, reporterC, 100); err != nil || !done {
		t.Fatalf("erase reporter C: done=%v err=%v", done, err)
	}
	if done, n, err := env.store.EraseAccountBatch(ctx, reporterR, 100); err != nil || !done || n != 0 {
		t.Fatalf("repeat erasure must be a no-op: done=%v n=%d err=%v", done, n, err)
	}
}

func TestForumReports_SettleByCaseIsIdempotent(t *testing.T) {
	env := setupFOR16Env(t)
	ctx := context.Background()

	modStore := moderationadapters.NewStore(env.pool)
	env.store.CaseOpener = func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (string, bool, error) {
		c, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID: "cccccccc-0000-4ccc-8ccc-000000000009", TargetType: targetType, TargetID: targetID,
			Priority: moderationdomain.PriorityP2, Reason: reason, Detail: detail,
			OpenedAt: time.Now(), PolicyVersion: moderationdomain.PolicyV1,
		})
		if err != nil {
			return "", false, err
		}
		return modStore.OpenCaseTx(ctx, tx, c)
	}

	r1 := "55555555-eeee-4555-8555-000000000005"
	r2 := "66666666-ffff-4666-8666-000000000006"
	if _, err := env.pool.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'alias5','active'),($2,'alias6','active')", r1, r2); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
	post := "99999999-0004-4999-8999-000000000004"
	seedPost(t, env.pool, post)
	for _, account := range []string{r1, r2} {
		if _, err := env.store.Report(ctx, application.ReportCommand{
			ReporterAccountID: account, TargetType: "FORUM_POST", TargetID: post, Reason: "SPAM",
		}); err != nil {
			t.Fatalf("report by %s: %v", account, err)
		}
	}

	var caseID string
	if err := env.pool.QueryRow(ctx, "SELECT id::text FROM moderation_cases WHERE target_type='FORUM_POST' AND target_id=$1", post).Scan(&caseID); err != nil {
		t.Fatalf("load case: %v", err)
	}
	if _, err := env.store.SettleReportsByCase(ctx, caseID, "PENDING"); err == nil {
		t.Fatal("expected refusal for a non-final status")
	}
	n, err := env.store.SettleReportsByCase(ctx, caseID, "RESOLVED")
	if err != nil || n != 2 {
		t.Fatalf("settle: n=%d err=%v, want 2 rows", n, err)
	}
	if n, err := env.store.SettleReportsByCase(ctx, caseID, "DISMISSED"); err != nil || n != 0 {
		t.Fatalf("replay must not move settled reports: n=%d err=%v", n, err)
	}
	var resolved int
	if err := env.pool.QueryRow(ctx, "SELECT count(*) FROM forum_reports WHERE target_id=$1 AND status='RESOLVED'", post).Scan(&resolved); err != nil || resolved != 2 {
		t.Fatalf("expected 2 RESOLVED reports, got %d (%v)", resolved, err)
	}
}
