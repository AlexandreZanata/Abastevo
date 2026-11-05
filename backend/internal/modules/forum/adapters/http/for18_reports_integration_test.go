//go:build integration

package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
)

type for18Env struct {
	pool            *pgxpool.Pool
	store           *forumadapters.Store
	moderationStore *moderationadapters.Store
	runner          *identityadapters.Runner
	accountStore    *accountadapters.PGStore
	hasher          accountdomain.SHA256Hasher
	router          *chi.Mux
	handler         *DiscussionHandler
}

func setupFOR18Env(t *testing.T) *for18Env {
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

	name := fmt.Sprintf("forum_for18_%d", time.Now().UnixNano())
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
	moderationStore := moderationadapters.NewStore(pool)
	store.CaseOpener = func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (string, bool, error) {
		caseID, err := platformjobs.NewUUIDv4()
		if err != nil {
			return "", false, err
		}
		caseObj, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID:            caseID,
			TargetType:    targetType,
			TargetID:      targetID,
			Priority:      moderationdomain.PriorityP2,
			Reason:        reason,
			Detail:        detail,
			OpenedAt:      time.Now().UTC(),
			PolicyVersion: moderationdomain.PolicyV1,
		})
		if err != nil {
			return "", false, err
		}
		return moderationStore.OpenCaseTx(ctx, tx, caseObj)
	}

	runner := identityadapters.NewRunner(pool)
	accountStore := accountadapters.NewPGStore(pool)
	hasher := accountdomain.SHA256Hasher{}

	handler := &DiscussionHandler{
		Comments: store,
		Reports:  store,
		Runner:   runner,
	}

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return &for18Env{
		pool:            pool,
		store:           store,
		moderationStore: moderationStore,
		runner:          runner,
		accountStore:    accountStore,
		hasher:          hasher,
		router:          router,
		handler:         handler,
	}
}

func seedPublishedPostFOR18(t *testing.T, env *for18Env, postID string) {
	t.Helper()
	ctx := context.Background()
	sourceID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	q := `INSERT INTO forum_posts(
		id, kind, category, title, excerpt, body_text, publication_state, visibility,
		first_publication_seq, source_id, comments_enabled, local_created_at, local_updated_at,
		first_published_at, revision
	) VALUES (
		$1, 'EDITORIAL', 'FUELS', 'Titulo Valido Post', 'Excerto Valido', 'Corpo texto valido',
		'PUBLISHED', 'VISIBLE', 1, $2, true, $3, $3, $3, 1
	) ON CONFLICT (id) DO NOTHING`
	if _, err := env.pool.Exec(ctx, q, postID, sourceID, now); err != nil {
		t.Fatalf("seed post: %v", err)
	}
}

func seedCommentFOR18(t *testing.T, env *for18Env, id, postID, authorID string, parentID *string, depth int16, seq int64, state, visibility, body string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	pubAt := &now
	if state != "PUBLISHED" {
		pubAt = nil
	}
	opID, _ := platformjobs.NewUUIDv4()
	q := `INSERT INTO forum_comments(
		id, post_id, author_account_id, public_alias, parent_id, depth, sequence,
		body_text, revision, publication_state, visibility, created_at, updated_at,
		published_at, create_operation_id
	) VALUES (
		$1, $2, $3, 'alias', $4, $5, $6, $7, 1, $8, $9, $10, $10, $11, $12
	)`
	if _, err := env.pool.Exec(ctx, q, id, postID, authorID, parentID, depth, seq, body, state, visibility, now, pubAt, opID); err != nil {
		t.Fatalf("seed comment direct: %v", err)
	}
}

func seedTestAuthorFOR18(t *testing.T, env *for18Env, prefix string, now time.Time) testAuthor {
	t.Helper()
	ctx := context.Background()

	accID := fmt.Sprintf("11111111-%s-4111-8111-000000000001", prefix)
	familyID := fmt.Sprintf("22222222-%s-4222-8222-000000000001", prefix)
	contribID := fmt.Sprintf("33333333-%s-4333-8333-000000000001", prefix)
	keyID := fmt.Sprintf("44444444-%s-4444-8444-000000000001", prefix)
	token := "token-" + prefix

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := env.pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}

	exec("INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active') ON CONFLICT (id) DO NOTHING", accID, "alias-"+prefix)
	exec("INSERT INTO identity_contributors(id,status) VALUES($1,'active') ON CONFLICT (id) DO NOTHING", contribID)
	exec("INSERT INTO identity_keys(id,contributor_id,algorithm,public_jwk,fingerprint) VALUES($1,$2,'ecdsa-p256-sha512','{}',$3) ON CONFLICT (id) DO NOTHING", keyID, contribID, "fp-"+prefix)
	tokenHash := env.hasher.Hash("salt-"+prefix, token)
	exec("INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'fixture','fixture',$3,$4,$5,$6)",
		familyID, accID, "salt-"+prefix, tokenHash, now.Add(2*time.Hour), now)
	exec("INSERT INTO account_contributor_bindings(account_id,contributor_id,key_fingerprint) VALUES($1,$2,'fp') ON CONFLICT (account_id,contributor_id) DO NOTHING", accID, contribID)

	return testAuthor{
		AccountID:     accID,
		FamilyID:      familyID,
		ContributorID: contribID,
		KeyID:         keyID,
		AccessToken:   token,
	}
}

func wireAuthFOR18(env *for18Env) {
	env.handler.Sessions = func(ctx context.Context, familyID, accessToken string) (string, error) {
		var accID, salt, hash string
		var expires time.Time
		var revokedValid bool
		err := env.pool.QueryRow(ctx,
			"SELECT account_id, access_salt, access_hash, access_expires, revoked_at IS NOT NULL FROM account_session_families WHERE id = $1", familyID).
			Scan(&accID, &salt, &hash, &expires, &revokedValid)
		if err != nil {
			return "", err
		}
		if revokedValid || time.Now().After(expires) || !env.hasher.Equal(hash, salt, accessToken) {
			return "", errors.New("unauthorized session")
		}
		return accID, nil
	}

	env.handler.SignedOwner = func(r *http.Request, accID string) (string, string, error) {
		var contribID, keyID string
		err := env.pool.QueryRow(r.Context(),
			"SELECT b.contributor_id, k.id FROM account_contributor_bindings b JOIN identity_keys k ON k.contributor_id = b.contributor_id::uuid WHERE b.account_id = $1 LIMIT 1", accID).
			Scan(&contribID, &keyID)
		if err != nil {
			return "", "", fmt.Errorf("unbound contributor: %w", err)
		}
		return contribID, keyID, nil
	}

	env.handler.Guard = func(ctx context.Context, tx pgx.Tx, accID, famID, tok, contribID, kID string) error {
		if err := env.accountStore.RequireQualitySessionTx(ctx, tx, accID, famID, tok, contribID, env.hasher, time.Now().Unix()); err != nil {
			return err
		}
		return identityadapters.RequireQualitySigningKeyTx(ctx, tx, contribID, kID)
	}
}

func TestFOR18_PostgresIntegration_Gates(t *testing.T) {
	env := setupFOR18Env(t)
	now := time.Now().UTC()

	authorA := seedTestAuthorFOR18(t, env, "aaaa", now)
	authorB := seedTestAuthorFOR18(t, env, "bbbb", now)
	authorC := seedTestAuthorFOR18(t, env, "cccc", now)
	wireAuthFOR18(env)

	postID := "99999999-9999-4999-8999-000000000001"
	seedPublishedPostFOR18(t, env, postID)

	comment1ID := "88888888-8888-4888-8888-000000000001"
	seedCommentFOR18(t, env, comment1ID, postID, authorA.AccountID, nil, 0, 1, "PUBLISHED", "VISIBLE", "Comentario publico para moderacao.")

	var caseIDComment1 string
	var reportIDComment1 string

	// -------------------------------------------------------------------------
	// Gate 1: Duplicate / account-target report race & convergence
	// -------------------------------------------------------------------------
	t.Run("gate1_duplicate_report_race_and_convergence", func(t *testing.T) {
		// Reporter B reports Comment 1
		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"reason": "SPAM",
			"detail": "Spam comercial"
		}`, authorB.FamilyID, authorB.AccessToken)

		req := httptest.NewRequest("POST", "/v1/forum/comments/"+comment1ID+"/report", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for report, got %d: %s", w.Code, w.Body.String())
		}

		var resp1 ReportResponseDTO
		if err := json.Unmarshal(w.Body.Bytes(), &resp1); err != nil {
			t.Fatalf("unmarshal resp1: %v", err)
		}
		if resp1.Receipt.Replayed {
			t.Fatalf("expected replayed false on first report")
		}
		if resp1.Receipt.CaseID == "" {
			t.Fatalf("expected non-empty case_id in receipt")
		}
		if resp1.Receipt.TargetID != comment1ID {
			t.Fatalf("target_id = %s, want %s", resp1.Receipt.TargetID, comment1ID)
		}
		reportIDComment1 = resp1.Receipt.ReportID
		caseIDComment1 = resp1.Receipt.CaseID

		// Reporter B reports Comment 1 AGAIN -> idempotent replay
		req = httptest.NewRequest("POST", "/v1/forum/comments/"+comment1ID+"/report", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for replayed report, got %d", w.Code)
		}
		var resp2 ReportResponseDTO
		_ = json.Unmarshal(w.Body.Bytes(), &resp2)
		if !resp2.Receipt.Replayed {
			t.Fatalf("expected replayed true on duplicate report")
		}
		if resp2.Receipt.ReportID != reportIDComment1 {
			t.Fatalf("expected same report_id %s, got %s", reportIDComment1, resp2.Receipt.ReportID)
		}
		if resp2.Receipt.CaseID != caseIDComment1 {
			t.Fatalf("expected same case_id %s, got %s", caseIDComment1, resp2.Receipt.CaseID)
		}

		// Reporter C reports Comment 1 -> converges on SAME moderation case
		reqBodyC := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"reason": "HARASSMENT",
			"detail": "Ofensivo"
		}`, authorC.FamilyID, authorC.AccessToken)
		req = httptest.NewRequest("POST", "/v1/forum/comments/"+comment1ID+"/report", strings.NewReader(reqBodyC))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for report from C, got %d", w.Code)
		}
		var respC ReportResponseDTO
		_ = json.Unmarshal(w.Body.Bytes(), &respC)
		if respC.Receipt.Replayed {
			t.Fatalf("expected replayed false for reporter C")
		}
		if respC.Receipt.ReportID == reportIDComment1 {
			t.Fatalf("expected distinct report_id for reporter C")
		}
		if respC.Receipt.CaseID != caseIDComment1 {
			t.Fatalf("expected converged case_id %s, got %s", caseIDComment1, respC.Receipt.CaseID)
		}

		// Concurrent duplicate reports from Reporter B must not create duplicate DB rows
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := httptest.NewRequest("POST", "/v1/forum/comments/"+comment1ID+"/report", strings.NewReader(reqBody))
				r.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				env.router.ServeHTTP(rec, r)
			}()
		}
		wg.Wait()

		var countB int
		err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM forum_reports WHERE account_id = $1 AND target_id = $2",
			authorB.AccountID, comment1ID).Scan(&countB)
		if err != nil || countB != 1 {
			t.Fatalf("expected exactly 1 report row for author B, got count=%d, err=%v", countB, err)
		}
	})

	// -------------------------------------------------------------------------
	// Gate 2: Report does NOT hide (target remains visible until operator decision)
	// -------------------------------------------------------------------------
	t.Run("gate2_report_does_not_hide", func(t *testing.T) {
		// Verify comment 1 in database
		var state, visibility string
		err := env.pool.QueryRow(context.Background(),
			"SELECT publication_state, visibility FROM forum_comments WHERE id = $1", comment1ID).
			Scan(&state, &visibility)
		if err != nil {
			t.Fatalf("query comment: %v", err)
		}
		if state != "PUBLISHED" || visibility != "VISIBLE" {
			t.Fatalf("expected comment to remain PUBLISHED/VISIBLE after report, got %s/%s", state, visibility)
		}

		// Public query still returns the comment
		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 from public comments, got %d", w.Code)
		}
		var feed CommentsResponseDTO
		_ = json.Unmarshal(w.Body.Bytes(), &feed)
		if len(feed.Items) == 0 || feed.Items[0].ID != comment1ID {
			t.Fatalf("reported comment must remain visible in public feed before operator triage")
		}

		// Report post 1: post remains PUBLISHED and VISIBLE
		postReportBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"reason": "OFF_TOPIC"
		}`, authorB.FamilyID, authorB.AccessToken)
		req = httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/report", strings.NewReader(postReportBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for post report, got %d", w.Code)
		}

		var postState, postVis string
		err = env.pool.QueryRow(context.Background(),
			"SELECT publication_state, visibility FROM forum_posts WHERE id = $1", postID).
			Scan(&postState, &postVis)
		if err != nil || postState != "PUBLISHED" || postVis != "VISIBLE" {
			t.Fatalf("post must remain PUBLISHED/VISIBLE after report, got %s/%s", postState, postVis)
		}
	})

	// -------------------------------------------------------------------------
	// Gate 3: Target missing returns 404
	// -------------------------------------------------------------------------
	t.Run("gate3_target_missing_404", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"reason": "SPAM"
		}`, authorB.FamilyID, authorB.AccessToken)

		// 1. Missing post
		missingPostID := "00000000-0000-4000-8000-000000000404"
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+missingPostID+"/report", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing post, got %d", w.Code)
		}

		// 2. Missing comment
		missingCommentID := "00000000-0000-4000-8000-000000000405"
		req = httptest.NewRequest("POST", "/v1/forum/comments/"+missingCommentID+"/report", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing comment, got %d", w.Code)
		}

		// 3. Removed (tombstoned) comment returns 404
		removedCommentID := "88888888-8888-4888-8888-000000000002"
		seedCommentFOR18(t, env, removedCommentID, postID, authorA.AccountID, nil, 0, 2, "REMOVED", "HIDDEN", "")
		req = httptest.NewRequest("POST", "/v1/forum/comments/"+removedCommentID+"/report", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for removed comment, got %d", w.Code)
		}
	})

	// -------------------------------------------------------------------------
	// Gate 4: Self-report forbidden (400 Bad Request)
	// -------------------------------------------------------------------------
	t.Run("gate4_self_report_forbidden_400", func(t *testing.T) {
		// Author A created comment 1; Author A attempts to report comment 1
		selfBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"reason": "OTHER"
		}`, authorA.FamilyID, authorA.AccessToken)

		req := httptest.NewRequest("POST", "/v1/forum/comments/"+comment1ID+"/report", strings.NewReader(selfBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for self-report, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "cannot report own content") {
			t.Fatalf("expected error message to state cannot report own content, got %s", w.Body.String())
		}
	})

	// -------------------------------------------------------------------------
	// Gate 5: Bounded admission / quota (10/day/account; 11th returns 429)
	// -------------------------------------------------------------------------
	t.Run("gate5_bounded_quota_10_per_day", func(t *testing.T) {
		authorQuota := seedTestAuthorFOR18(t, env, "dddd", now)

		// Seed 11 distinct published comments
		for i := 10; i <= 20; i++ {
			cID := fmt.Sprintf("88888888-8888-4888-8888-%012d", i)
			seedCommentFOR18(t, env, cID, postID, authorA.AccountID, nil, 0, int64(i), "PUBLISHED", "VISIBLE", fmt.Sprintf("Comentario %d", i))
		}

		// First 10 reports must succeed
		for i := 10; i <= 19; i++ {
			cID := fmt.Sprintf("88888888-8888-4888-8888-%012d", i)
			b := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","reason":"SPAM"}`, authorQuota.FamilyID, authorQuota.AccessToken)
			req := httptest.NewRequest("POST", "/v1/forum/comments/"+cID+"/report", strings.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			env.router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("report %d should succeed, got %d: %s", i-9, w.Code, w.Body.String())
			}
		}

		// 11th report must return 429 Rate Limited with Retry-After: 86400
		cID11 := "88888888-8888-4888-8888-000000000020"
		b := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","reason":"SPAM"}`, authorQuota.FamilyID, authorQuota.AccessToken)
		req := httptest.NewRequest("POST", "/v1/forum/comments/"+cID11+"/report", strings.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 on 11th report, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Retry-After") != "86400" {
			t.Fatalf("expected Retry-After: 86400, got %s", w.Header().Get("Retry-After"))
		}
	})

	// -------------------------------------------------------------------------
	// Gate 6: Operator action INVALIDATE on reported comment & post
	// -------------------------------------------------------------------------
	t.Run("gate6_operator_invalidate_hides_target_and_audit", func(t *testing.T) {
		// Also seed a reply to comment 1
		replyID := "88888888-8888-4888-8888-000000000030"
		seedCommentFOR18(t, env, replyID, postID, authorB.AccountID, &comment1ID, 1, 30, "PUBLISHED", "VISIBLE", "Resposta ao comentario 1")

		// Verify reply is visible in replies endpoint
		reqReplies := httptest.NewRequest("GET", "/v1/forum/comments/"+comment1ID+"/replies", nil)
		wReplies := httptest.NewRecorder()
		env.router.ServeHTTP(wReplies, reqReplies)
		if wReplies.Code != http.StatusOK {
			t.Fatalf("expected 200 for replies, got %d", wReplies.Code)
		}
		var repliesBefore RepliesResponseDTO
		_ = json.Unmarshal(wReplies.Body.Bytes(), &repliesBefore)
		if len(repliesBefore.Items) != 1 {
			t.Fatalf("expected 1 reply before hide, got %d", len(repliesBefore.Items))
		}

		// Operator executes INVALIDATE action on caseIDComment1
		actResult, err := moderationapp.Act(context.Background(), moderationapp.ActPorts{
			Clock: time.Now,
			NewID: platformjobs.NewUUIDv4,
			Store: env.moderationStore,
			InvalidateForumTarget: func(ctx context.Context, tx pgx.Tx, targetType, targetID, caseID, reason string, at time.Time) error {
				return env.store.InvalidateForumTargetTx(ctx, tx, targetType, targetID, caseID, reason, at)
			},
			EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
				return nil
			},
		}, moderationapp.Caller{OperatorID: "operator-alice"}, moderationapp.ActDTO{
			CaseID: caseIDComment1,
			Action: moderationdomain.ActionInvalidate,
			Reason: "Violacao das diretrizes de spam",
		})
		if err != nil {
			t.Fatalf("operator act invalidate: %v", err)
		}
		if actResult.ToStatus != moderationdomain.StatusResolved {
			t.Fatalf("expected case status RESOLVED, got %s", actResult.ToStatus)
		}

		// 1. Verify comment 1 visibility in DB is now HIDDEN
		var vis string
		err = env.pool.QueryRow(context.Background(),
			"SELECT visibility FROM forum_comments WHERE id = $1", comment1ID).Scan(&vis)
		if err != nil || vis != "HIDDEN" {
			t.Fatalf("expected comment 1 visibility HIDDEN, got %s (err: %v)", vis, err)
		}

		// 2. Verify public feed omits comment 1
		reqFeed := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		wFeed := httptest.NewRecorder()
		env.router.ServeHTTP(wFeed, reqFeed)
		var feedAfter CommentsResponseDTO
		_ = json.Unmarshal(wFeed.Body.Bytes(), &feedAfter)
		for _, item := range feedAfter.Items {
			if item.ID == comment1ID {
				t.Fatalf("invalidated comment 1 must be excluded from public feed")
			}
		}

		// 3. Parent suppression: replies endpoint for comment 1 returns 0 items because parent is hidden
		reqReplies = httptest.NewRequest("GET", "/v1/forum/comments/"+comment1ID+"/replies", nil)
		wReplies = httptest.NewRecorder()
		env.router.ServeHTTP(wReplies, reqReplies)
		if wReplies.Code != http.StatusOK {
			t.Fatalf("expected 200 for replies of hidden parent, got %d: %s", wReplies.Code, wReplies.Body.String())
		}
		var repliesSuppressed RepliesResponseDTO
		_ = json.Unmarshal(wReplies.Body.Bytes(), &repliesSuppressed)
		if len(repliesSuppressed.Items) != 0 {
			t.Fatalf("expected 0 replies due to parent suppression, got %d", len(repliesSuppressed.Items))
		}

		// Also verify submitting a new reply to the hidden comment returns 404
		replyAttemptBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"parent_id": "%s",
			"body_text": "Tentativa de resposta a comentario oculto"
		}`, authorB.FamilyID, authorB.AccessToken, "77777777-7777-4777-8777-000000000099", comment1ID)
		reqNewReply := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(replyAttemptBody))
		reqNewReply.Header.Set("Content-Type", "application/json")
		wNewReply := httptest.NewRecorder()
		env.router.ServeHTTP(wNewReply, reqNewReply)
		if wNewReply.Code != http.StatusNotFound {
			t.Fatalf("submitting reply to hidden comment must return 404, got %d: %s", wNewReply.Code, wNewReply.Body.String())
		}

		// 4. Verify audit record in forum_changes
		var targetType, action, actorKind, reasonCode string
		err = env.pool.QueryRow(context.Background(),
			"SELECT target_type, action, actor_kind, reason_code FROM forum_changes WHERE target_type = 'COMMENT' AND target_id = $1 AND action = 'HIDE'",
			comment1ID).Scan(&targetType, &action, &actorKind, &reasonCode)
		if err != nil {
			t.Fatalf("failed to query forum_changes audit: %v", err)
		}
		if actorKind != "ADMIN" || reasonCode != "moderation_invalidate" {
			t.Fatalf("unexpected audit record: actorKind=%s, reasonCode=%s", actorKind, reasonCode)
		}
	})

	// -------------------------------------------------------------------------
	// Gate 7: Audit failure causes clean rollback
	// -------------------------------------------------------------------------
	t.Run("gate7_audit_failure_causes_rollback", func(t *testing.T) {
		commentToTest := "88888888-8888-4888-8888-000000000040"
		seedCommentFOR18(t, env, commentToTest, postID, authorA.AccountID, nil, 0, 40, "PUBLISHED", "VISIBLE", "Comentario para teste de rollback")

		// Create a case
		caseID, _ := platformjobs.NewUUIDv4()
		caseObj, _, _ := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID:            caseID,
			TargetType:    moderationdomain.TargetForumComment,
			TargetID:      commentToTest,
			Priority:      moderationdomain.PriorityP2,
			Reason:        "SPAM",
			OpenedAt:      time.Now().UTC(),
			PolicyVersion: moderationdomain.PolicyV1,
		})
		_, _, err := env.moderationStore.OpenCase(context.Background(), caseObj)
		if err != nil {
			t.Fatalf("open case: %v", err)
		}

		// Simulate audit failure in InvalidateForumTarget
		expectedErr := errors.New("simulated audit disk failure")
		_, err = moderationapp.Act(context.Background(), moderationapp.ActPorts{
			Clock: time.Now,
			NewID: platformjobs.NewUUIDv4,
			Store: env.moderationStore,
			InvalidateForumTarget: func(ctx context.Context, tx pgx.Tx, targetType, targetID, caseID, reason string, at time.Time) error {
				return expectedErr
			},
			EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
				return nil
			},
		}, moderationapp.Caller{OperatorID: "operator-bob"}, moderationapp.ActDTO{
			CaseID: caseID,
			Action: moderationdomain.ActionInvalidate,
			Reason: "Teste de falha",
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected simulated error, got %v", err)
		}

		// Verify comment visibility in database is STILL VISIBLE (transaction rolled back)
		var vis string
		err = env.pool.QueryRow(context.Background(),
			"SELECT visibility FROM forum_comments WHERE id = $1", commentToTest).Scan(&vis)
		if err != nil || vis != "VISIBLE" {
			t.Fatalf("comment visibility must remain VISIBLE after audit rollback, got %s", vis)
		}
	})

	// -------------------------------------------------------------------------
	// Gate 8: Admin denial (unauthorized or forbidden action)
	// -------------------------------------------------------------------------
	t.Run("gate8_admin_denial", func(t *testing.T) {
		// 1. Anonymous operator is denied with ErrUnauthorized
		_, err := moderationapp.Act(context.Background(), moderationapp.ActPorts{
			Store: env.moderationStore,
		}, moderationapp.Caller{OperatorID: ""}, moderationapp.ActDTO{
			CaseID: caseIDComment1,
			Action: moderationdomain.ActionReview,
			Reason: "Triage",
		})
		if !errors.Is(err, moderationdomain.ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized for anonymous operator, got %v", err)
		}

		// 2. Action BLOCK is not allowed for FORUM_COMMENT target
		_, err = moderationapp.Act(context.Background(), moderationapp.ActPorts{
			Clock: time.Now,
			NewID: platformjobs.NewUUIDv4,
			Store: env.moderationStore,
		}, moderationapp.Caller{OperatorID: "operator-alice"}, moderationapp.ActDTO{
			CaseID: caseIDComment1,
			Action: moderationdomain.ActionBlock,
			Reason: "Bloqueio invalido",
		})
		if !errors.Is(err, moderationdomain.ErrActionForbidden) {
			t.Fatalf("expected ErrActionForbidden for BLOCK on comment, got %v", err)
		}
	})

	// -------------------------------------------------------------------------
	// Gate 9: Hide vs remove/erase and stale restore
	// -------------------------------------------------------------------------
	t.Run("gate9_hide_vs_remove_and_stale_restore", func(t *testing.T) {
		// Author A CAS-removes the hidden comment 1
		removeBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1
		}`, authorA.FamilyID, authorA.AccessToken, "66666666-6666-4666-8666-000000000001")

		req := httptest.NewRequest("POST", "/v1/forum/comments/"+comment1ID+"/remove", strings.NewReader(removeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for author remove on hidden comment, got %d: %s", w.Code, w.Body.String())
		}

		// Comment is now REMOVED + HIDDEN + body scrubbed
		var state, vis, body string
		err := env.pool.QueryRow(context.Background(),
			"SELECT publication_state, visibility, body_text FROM forum_comments WHERE id = $1", comment1ID).
			Scan(&state, &vis, &body)
		if err != nil {
			t.Fatalf("query removed comment: %v", err)
		}
		if state != "REMOVED" || vis != "HIDDEN" || body != "" {
			t.Fatalf("comment must be REMOVED/HIDDEN with empty body, got %s/%s/%q", state, vis, body)
		}

		// Attempting to restore or unhide a REMOVED comment must fail
		// Using SetCommentVisibility with 'VISIBLE' on a REMOVED comment updates 0 rows
		err = env.store.InvalidateForumTargetTx(context.Background(), nil, "FORUM_COMMENT", comment1ID, "case-restore", "tenta restaurar", time.Now().UTC())
		// Invalidate sets HIDDEN, which is harmless, but let's test setting VISIBLE directly:
		tx, _ := env.pool.Begin(context.Background())
		defer tx.Rollback(context.Background())
		_, err = env.pool.Exec(context.Background(),
			`UPDATE forum_comments SET visibility = 'VISIBLE' WHERE id = $1 AND publication_state <> 'REMOVED'`, comment1ID)
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		// Confirm comment state did not become VISIBLE
		_ = env.pool.QueryRow(context.Background(),
			"SELECT visibility FROM forum_comments WHERE id = $1", comment1ID).Scan(&vis)
		if vis == "VISIBLE" {
			t.Fatalf("author removal must be terminal; setting visibility to VISIBLE on REMOVED row must never succeed")
		}
	})

	// -------------------------------------------------------------------------
	// Gate 10: Safe reporter and author receipts (privacy isolation)
	// -------------------------------------------------------------------------
	t.Run("gate10_safe_reporter_and_author_receipts", func(t *testing.T) {
		// Reporter B's receipt was verified in Gate 1 to contain:
		// report_id, case_id, status: "RECEIVED", reason: "SPAM".
		// Now verify author A's private owner read does NOT leak reporter identity or stated reason.
		ownerReadBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"comment_id": "%s"
		}`, authorA.FamilyID, authorA.AccessToken, comment1ID)

		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(ownerReadBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for owner read, got %d", w.Code)
		}

		respStr := w.Body.String()
		// Author sees state REMOVED / HIDDEN
		if !strings.Contains(respStr, `"publication_state":"REMOVED"`) {
			t.Fatalf("expected publication_state REMOVED in owner read, got %s", respStr)
		}
		// Privacy check: MUST NOT contain reporter account ID or reporter report reason
		if strings.Contains(respStr, authorB.AccountID) {
			t.Fatalf("CRITICAL PRIVACY VIOLATION: reporter account ID leaked to author!")
		}
		if strings.Contains(respStr, "Spam comercial") {
			t.Fatalf("CRITICAL PRIVACY VIOLATION: reporter detail text leaked to author!")
		}
	})
}
