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
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type for17Env struct {
	pool         *pgxpool.Pool
	store        *forumadapters.Store
	runner       *identityadapters.Runner
	accountStore *accountadapters.PGStore
	hasher       accountdomain.SHA256Hasher
	router       chi.Router
	handler      *DiscussionHandler
}

func setupFOR17Env(t *testing.T) *for17Env {
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

	name := fmt.Sprintf("forum_for17_%d", time.Now().UnixNano())
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
	runner := identityadapters.NewRunner(pool)
	accountStore := accountadapters.NewPGStore(pool)
	hasher := accountdomain.SHA256Hasher{}

	handler := &DiscussionHandler{
		Comments: store,
		Runner:   runner,
	}

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return &for17Env{
		pool:         pool,
		store:        store,
		runner:       runner,
		accountStore: accountStore,
		hasher:       hasher,
		router:       router,
		handler:      handler,
	}
}

func seedPublishedPostFOR17(t *testing.T, env *for17Env, postID string) {
	t.Helper()
	ctx := context.Background()
	sourceID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	q := `INSERT INTO forum_posts(
		id, kind, category, title, excerpt, body_text, publication_state, visibility,
		first_publication_seq, source_id, comments_enabled, local_created_at, local_updated_at,
		first_published_at, next_comment_sequence
	) VALUES (
		$1, 'EDITORIAL', 'FUELS', 'Post para discussao FOR17', 'Resumo', 'Corpo do post',
		'PUBLISHED', 'VISIBLE', 1, $2, true, $3, $3, $3, 1
	)`
	if _, err := env.pool.Exec(ctx, q, postID, sourceID, now); err != nil {
		t.Fatalf("seed post: %v", err)
	}
}

func seedCommentDirect(t *testing.T, env *for17Env, id, postID, authorID string, parentID *string, depth int16, seq int64, state, visibility, body string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	var pubAt *time.Time
	if state == "PUBLISHED" {
		pubAt = &now
	}
	opID := fmt.Sprintf("66666666-%04d-4666-8666-000000000001", seq)
	q := `INSERT INTO forum_comments (
		id, post_id, author_account_id, public_alias, parent_id, depth, sequence,
		body_text, revision, publication_state, visibility, created_at, updated_at,
		published_at, create_operation_id, body_sha256
	) VALUES (
		$1, $2, $3, 'UsuarioSeq', $4, $5, $6,
		$7, 1, $8, $9, $10, $10,
		$11, $12, 'fixture-sha'
	)`
	if _, err := env.pool.Exec(ctx, q, id, postID, authorID, parentID, depth, seq, body, state, visibility, now, pubAt, opID); err != nil {
		t.Fatalf("seed comment direct: %v", err)
	}
}

func TestDiscussionEndpoints_PostgresIntegration_Gates(t *testing.T) {
	env := setupFOR17Env(t)
	now := time.Now().UTC()

	// Seed author A and author B
	authorA := seedTestAuthor(t, (*for15Env)(env), "aaaa", now)
	authorB := seedTestAuthor(t, (*for15Env)(env), "bbbb", now)

	postID := "99999999-9999-4999-8999-000000000001"
	seedPublishedPostFOR17(t, env, postID)

	// Configure real auth guards
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

	opID1 := "77777777-7777-4777-8777-000000000001"
	bodyText1 := "Comentario inicial assinado."
	var createdCommentID string

	// -------------------------------------------------------------
	// Gate 1: Actual signed API flow & private receipt vs public isolation
	// -------------------------------------------------------------
	t.Run("gate1_signed_create_and_public_pending_isolation", func(t *testing.T) {
		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "%s"
		}`, authorA.FamilyID, authorA.AccessToken, opID1, bodyText1)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}

		var receipt CommentReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal receipt: %v", err)
		}
		if receipt.Replayed {
			t.Errorf("expected first submission replayed=false")
		}
		if receipt.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected state PENDING_REVIEW, got %s", receipt.PublicationState)
		}
		if receipt.BodyText != bodyText1 {
			t.Errorf("expected bodyText %q, got %q", bodyText1, receipt.BodyText)
		}
		createdCommentID = receipt.ID

		// Verify public endpoint returns 0 items (pending text does not leak!)
		pubReq := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		pubW := httptest.NewRecorder()
		env.router.ServeHTTP(pubW, pubReq)

		if pubW.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", pubW.Code, pubW.Body.String())
		}
		var pubResp CommentsResponseDTO
		if err := json.Unmarshal(pubW.Body.Bytes(), &pubResp); err != nil {
			t.Fatalf("unmarshal pubResp: %v", err)
		}
		if len(pubResp.Items) != 0 {
			t.Fatalf("INVARIANT VIOLATION: pending comment leaked to public feed: %+v", pubResp.Items)
		}
	})

	// -------------------------------------------------------------
	// Gate 2: Idempotent replay & session refresh replay
	// -------------------------------------------------------------
	t.Run("gate2_idempotent_replay_and_session_refresh", func(t *testing.T) {
		// A. Identical replay
		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "%s"
		}`, authorA.FamilyID, authorA.AccessToken, opID1, bodyText1)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal receipt: %v", err)
		}
		if !receipt.Replayed {
			t.Errorf("expected replayed=true on identical replay")
		}

		// B. Session refresh replay (different access token, same operation_id)
		newToken := "token-aaaa-refreshed"
		newHash := env.hasher.Hash("salt-aaaa-2", newToken)
		_, err := env.pool.Exec(context.Background(),
			"UPDATE account_session_families SET access_salt = $1, access_hash = $2, access_expires = $3 WHERE id = $4",
			"salt-aaaa-2", newHash, time.Now().Add(2*time.Hour), authorA.FamilyID)
		if err != nil {
			t.Fatalf("update session: %v", err)
		}

		refreshReqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "%s"
		}`, authorA.FamilyID, newToken, opID1, bodyText1)

		refReq := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(refreshReqBody))
		refReq.Header.Set("Content-Type", "application/json")
		refW := httptest.NewRecorder()
		env.router.ServeHTTP(refW, refReq)

		if refW.Code != http.StatusOK {
			t.Fatalf("expected 200 on refreshed session replay, got %d: %s", refW.Code, refW.Body.String())
		}
		var refReceipt CommentReceiptDTO
		if err := json.Unmarshal(refW.Body.Bytes(), &refReceipt); err != nil {
			t.Fatalf("unmarshal refReceipt: %v", err)
		}
		if !refReceipt.Replayed {
			t.Errorf("expected replayed=true across session refresh")
		}
		if refReceipt.ID != createdCommentID {
			t.Errorf("expected same comment ID %s, got %s", createdCommentID, refReceipt.ID)
		}

		// Restore token for authorA
		authorA.AccessToken = newToken
	})

	// -------------------------------------------------------------
	// Gate 3: Conflicting payload on same operation ID
	// -------------------------------------------------------------
	t.Run("gate3_conflicting_payload_same_operation_id", func(t *testing.T) {
		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "Texto alterado com mesmo operation_id"
		}`, authorA.FamilyID, authorA.AccessToken, opID1)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})

	// -------------------------------------------------------------
	// Gate 4: Keyset pagination under concurrent insert and hide
	// -------------------------------------------------------------
	t.Run("gate4_keyset_pagination_and_reply_count", func(t *testing.T) {
		post2ID := "99999999-9999-4999-8999-000000000002"
		seedPublishedPostFOR17(t, env, post2ID)

		// Seed 25 published visible top-level comments on post2
		for seq := int64(1); seq <= 25; seq++ {
			cID := fmt.Sprintf("55555555-5555-4555-8555-%012d", seq)
			seedCommentDirect(t, env, cID, post2ID, authorA.AccountID, nil, 0, seq, "PUBLISHED", "VISIBLE", fmt.Sprintf("Comentario %d", seq))
		}

		// Seed 3 published visible replies under comment #1
		c1ID := fmt.Sprintf("55555555-5555-4555-8555-%012d", 1)
		for rseq := int64(101); rseq <= 103; rseq++ {
			rID := fmt.Sprintf("55555555-5555-4555-8555-%012d", rseq)
			seedCommentDirect(t, env, rID, post2ID, authorB.AccountID, &c1ID, 1, rseq, "PUBLISHED", "VISIBLE", fmt.Sprintf("Resposta %d", rseq))
		}

		// Page 1: limit = 10
		req1 := httptest.NewRequest("GET", "/v1/forum/posts/"+post2ID+"/comments?limit=10", nil)
		w1 := httptest.NewRecorder()
		env.router.ServeHTTP(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("page 1 failed: %d: %s", w1.Code, w1.Body.String())
		}
		var p1 CommentsResponseDTO
		if err := json.Unmarshal(w1.Body.Bytes(), &p1); err != nil {
			t.Fatalf("unmarshal p1: %v", err)
		}
		if len(p1.Items) != 10 {
			t.Fatalf("expected 10 items on page 1, got %d", len(p1.Items))
		}
		if p1.NextCursor == nil || *p1.NextCursor != "10" {
			t.Fatalf("expected next_cursor='10', got %v", p1.NextCursor)
		}
		// Comment #1 should have reply_count = 3
		if p1.Items[0].ReplyCount != 3 {
			t.Errorf("expected reply_count=3 for comment 1, got %d", p1.Items[0].ReplyCount)
		}
		if p1.Items[1].ReplyCount != 0 {
			t.Errorf("expected reply_count=0 for comment 2, got %d", p1.Items[1].ReplyCount)
		}

		// Page 2: cursor = 10, limit = 10
		req2 := httptest.NewRequest("GET", "/v1/forum/posts/"+post2ID+"/comments?limit=10&cursor=10", nil)
		w2 := httptest.NewRecorder()
		env.router.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("page 2 failed: %d: %s", w2.Code, w2.Body.String())
		}
		var p2 CommentsResponseDTO
		if err := json.Unmarshal(w2.Body.Bytes(), &p2); err != nil {
			t.Fatalf("unmarshal p2: %v", err)
		}
		if len(p2.Items) != 10 {
			t.Fatalf("expected 10 items on page 2, got %d", len(p2.Items))
		}
		if p2.NextCursor == nil || *p2.NextCursor != "20" {
			t.Fatalf("expected next_cursor='20', got %v", p2.NextCursor)
		}
		if p2.Items[0].Sequence != 11 || p2.Items[9].Sequence != 20 {
			t.Errorf("unexpected sequence range on page 2: %d..%d", p2.Items[0].Sequence, p2.Items[9].Sequence)
		}

		// Concurrent mutations:
		// 1. Insert comment #26
		c26ID := fmt.Sprintf("55555555-5555-4555-8555-%012d", 26)
		seedCommentDirect(t, env, c26ID, post2ID, authorA.AccountID, nil, 0, 26, "PUBLISHED", "VISIBLE", "Comentario 26")
		// 2. Hide comment #5 (on page 1)
		c5ID := fmt.Sprintf("55555555-5555-4555-8555-%012d", 5)
		if _, err := env.pool.Exec(context.Background(), "UPDATE forum_comments SET visibility = 'HIDDEN' WHERE id = $1", c5ID); err != nil {
			t.Fatalf("hide comment 5: %v", err)
		}

		// Page 3: cursor = 20, limit = 10
		req3 := httptest.NewRequest("GET", "/v1/forum/posts/"+post2ID+"/comments?limit=10&cursor=20", nil)
		w3 := httptest.NewRecorder()
		env.router.ServeHTTP(w3, req3)
		if w3.Code != http.StatusOK {
			t.Fatalf("page 3 failed: %d: %s", w3.Code, w3.Body.String())
		}
		var p3 CommentsResponseDTO
		if err := json.Unmarshal(w3.Body.Bytes(), &p3); err != nil {
			t.Fatalf("unmarshal p3: %v", err)
		}
		// Items 21, 22, 23, 24, 25, 26 -> 6 items
		if len(p3.Items) != 6 {
			t.Fatalf("expected 6 items on page 3, got %d", len(p3.Items))
		}
		if p3.NextCursor != nil {
			t.Errorf("expected next_cursor=nil on final page, got %v", *p3.NextCursor)
		}
		if p3.Items[5].Sequence != 26 {
			t.Errorf("expected last item seq=26, got %d", p3.Items[5].Sequence)
		}
	})

	// -------------------------------------------------------------
	// Gate 5: Reply pagination and parent comment suppression
	// -------------------------------------------------------------
	t.Run("gate5_reply_pagination_and_parent_suppression", func(t *testing.T) {
		c1ID := fmt.Sprintf("55555555-5555-4555-8555-%012d", 1)

		// Page 1 of replies (limit = 2)
		repReq1 := httptest.NewRequest("GET", "/v1/forum/comments/"+c1ID+"/replies?limit=2", nil)
		repW1 := httptest.NewRecorder()
		env.router.ServeHTTP(repW1, repReq1)
		if repW1.Code != http.StatusOK {
			t.Fatalf("replies page 1 failed: %d: %s", repW1.Code, repW1.Body.String())
		}
		var repP1 RepliesResponseDTO
		if err := json.Unmarshal(repW1.Body.Bytes(), &repP1); err != nil {
			t.Fatalf("unmarshal repP1: %v", err)
		}
		if len(repP1.Items) != 2 {
			t.Fatalf("expected 2 replies on page 1, got %d", len(repP1.Items))
		}
		if repP1.NextCursor == nil || *repP1.NextCursor != "102" {
			t.Fatalf("expected next_cursor='102', got %v", repP1.NextCursor)
		}

		// Page 2 of replies
		repReq2 := httptest.NewRequest("GET", "/v1/forum/comments/"+c1ID+"/replies?limit=2&cursor=102", nil)
		repW2 := httptest.NewRecorder()
		env.router.ServeHTTP(repW2, repReq2)
		if repW2.Code != http.StatusOK {
			t.Fatalf("replies page 2 failed: %d: %s", repW2.Code, repW2.Body.String())
		}
		var repP2 RepliesResponseDTO
		if err := json.Unmarshal(repW2.Body.Bytes(), &repP2); err != nil {
			t.Fatalf("unmarshal repP2: %v", err)
		}
		if len(repP2.Items) != 1 {
			t.Fatalf("expected 1 reply on page 2, got %d", len(repP2.Items))
		}
		if repP2.NextCursor != nil {
			t.Errorf("expected next_cursor=nil on final replies page, got %v", *repP2.NextCursor)
		}

		// Parent suppression: hide parent comment c1
		if _, err := env.pool.Exec(context.Background(), "UPDATE forum_comments SET visibility = 'HIDDEN' WHERE id = $1", c1ID); err != nil {
			t.Fatalf("hide c1: %v", err)
		}

		// Query replies again -> MUST return 0 replies (suppression invariant!)
		suppReq := httptest.NewRequest("GET", "/v1/forum/comments/"+c1ID+"/replies", nil)
		suppW := httptest.NewRecorder()
		env.router.ServeHTTP(suppW, suppReq)
		if suppW.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", suppW.Code, suppW.Body.String())
		}
		var suppResp RepliesResponseDTO
		if err := json.Unmarshal(suppW.Body.Bytes(), &suppResp); err != nil {
			t.Fatalf("unmarshal suppResp: %v", err)
		}
		if len(suppResp.Items) != 0 {
			t.Fatalf("INVARIANT VIOLATION: replies not suppressed for hidden parent: %+v", suppResp.Items)
		}
	})

	// -------------------------------------------------------------
	// Gate 6: Owner CAS edit and remove terminal state
	// -------------------------------------------------------------
	t.Run("gate6_owner_cas_edit_and_remove", func(t *testing.T) {
		editOpID := "77777777-7777-4777-8777-000000000002"
		editBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1,
			"body_text": "Texto corrigido pelo autor."
		}`, authorA.FamilyID, authorA.AccessToken, editOpID)

		req := httptest.NewRequest("POST", "/v1/forum/comments/"+createdCommentID+"/edit", strings.NewReader(editBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on edit, got %d: %s", w.Code, w.Body.String())
		}
		var editReceipt CommentReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &editReceipt); err != nil {
			t.Fatalf("unmarshal editReceipt: %v", err)
		}
		if editReceipt.Revision != 2 {
			t.Errorf("expected revision 2, got %d", editReceipt.Revision)
		}
		if editReceipt.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected state return to PENDING_REVIEW, got %s", editReceipt.PublicationState)
		}

		// Stale revision edit -> 409
		staleBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1,
			"body_text": "Tentativa com revisao antiga."
		}`, authorA.FamilyID, authorA.AccessToken, "77777777-7777-4777-8777-000000000003")

		staleReq := httptest.NewRequest("POST", "/v1/forum/comments/"+createdCommentID+"/edit", strings.NewReader(staleBody))
		staleReq.Header.Set("Content-Type", "application/json")
		staleW := httptest.NewRecorder()
		env.router.ServeHTTP(staleW, staleReq)
		if staleW.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict on stale revision, got %d: %s", staleW.Code, staleW.Body.String())
		}

		// Remove with expected_revision = 2
		removeOpID := "77777777-7777-4777-8777-000000000004"
		removeBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 2
		}`, authorA.FamilyID, authorA.AccessToken, removeOpID)

		remReq := httptest.NewRequest("POST", "/v1/forum/comments/"+createdCommentID+"/remove", strings.NewReader(removeBody))
		remReq.Header.Set("Content-Type", "application/json")
		remW := httptest.NewRecorder()
		env.router.ServeHTTP(remW, remReq)

		if remW.Code != http.StatusOK {
			t.Fatalf("expected 200 on remove, got %d: %s", remW.Code, remW.Body.String())
		}
		var remReceipt CommentReceiptDTO
		if err := json.Unmarshal(remW.Body.Bytes(), &remReceipt); err != nil {
			t.Fatalf("unmarshal remReceipt: %v", err)
		}
		if remReceipt.Revision != 3 {
			t.Errorf("expected revision 3, got %d", remReceipt.Revision)
		}
		if remReceipt.PublicationState != "REMOVED" || remReceipt.Visibility != "HIDDEN" {
			t.Errorf("expected REMOVED and HIDDEN, got %s / %s", remReceipt.PublicationState, remReceipt.Visibility)
		}
		if remReceipt.BodyText != "" {
			t.Errorf("expected body wiped on tombstone, got %q", remReceipt.BodyText)
		}

		// Remove again -> 409 Conflict (terminal state)
		termBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 3
		}`, authorA.FamilyID, authorA.AccessToken, "77777777-7777-4777-8777-000000000005")
		termReq := httptest.NewRequest("POST", "/v1/forum/comments/"+createdCommentID+"/remove", strings.NewReader(termBody))
		termReq.Header.Set("Content-Type", "application/json")
		termW := httptest.NewRecorder()
		env.router.ServeHTTP(termW, termReq)
		if termW.Code != http.StatusConflict {
			t.Fatalf("expected 409 on removing terminal comment, got %d: %s", termW.Code, termW.Body.String())
		}
	})

	// -------------------------------------------------------------
	// Gate 7: IDOR defense on all owner commands
	// -------------------------------------------------------------
	t.Run("gate7_idor_defense_returns_indistinguishable_404", func(t *testing.T) {
		// Seed a fresh comment owned by Author A
		cAID := "55555555-5555-4555-8555-000000000999"
		seedCommentDirect(t, env, cAID, postID, authorA.AccountID, nil, 0, 999, "PUBLISHED", "VISIBLE", "Comentario do autor A")

		// Author B attempts Edit
		editBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1,
			"body_text": "Tentativa maliciosa do autor B"
		}`, authorB.FamilyID, authorB.AccessToken, "77777777-7777-4777-8777-000000000006")
		editReq := httptest.NewRequest("POST", "/v1/forum/comments/"+cAID+"/edit", strings.NewReader(editBody))
		editReq.Header.Set("Content-Type", "application/json")
		editW := httptest.NewRecorder()
		env.router.ServeHTTP(editW, editReq)
		if editW.Code != http.StatusNotFound {
			t.Errorf("IDOR VULNERABILITY: expected 404 on foreign edit, got %d: %s", editW.Code, editW.Body.String())
		}

		// Author B attempts Remove
		remBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1
		}`, authorB.FamilyID, authorB.AccessToken, "77777777-7777-4777-8777-000000000007")
		remReq := httptest.NewRequest("POST", "/v1/forum/comments/"+cAID+"/remove", strings.NewReader(remBody))
		remReq.Header.Set("Content-Type", "application/json")
		remW := httptest.NewRecorder()
		env.router.ServeHTTP(remW, remReq)
		if remW.Code != http.StatusNotFound {
			t.Errorf("IDOR VULNERABILITY: expected 404 on foreign remove, got %d: %s", remW.Code, remW.Body.String())
		}

		// Author B attempts OwnerRead
		readBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"comment_id": "%s"
		}`, authorB.FamilyID, authorB.AccessToken, cAID)
		readReq := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(readBody))
		readReq.Header.Set("Content-Type", "application/json")
		readW := httptest.NewRecorder()
		env.router.ServeHTTP(readW, readReq)
		if readW.Code != http.StatusNotFound {
			t.Errorf("IDOR VULNERABILITY: expected 404 on foreign owner read, got %d: %s", readW.Code, readW.Body.String())
		}

		// Author A succeeds on OwnerRead
		authReadBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"comment_id": "%s"
		}`, authorA.FamilyID, authorA.AccessToken, cAID)
		authReadReq := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(authReadBody))
		authReadReq.Header.Set("Content-Type", "application/json")
		authReadW := httptest.NewRecorder()
		env.router.ServeHTTP(authReadW, authReadReq)
		if authReadW.Code != http.StatusOK {
			t.Fatalf("expected 200 for legitimate owner read, got %d: %s", authReadW.Code, authReadW.Body.String())
		}
	})

	// -------------------------------------------------------------
	// Gate 8: Database outage resilience
	// -------------------------------------------------------------
	t.Run("gate8_database_outage_resilience", func(t *testing.T) {
		// Close pool to simulate DB disconnection
		env.pool.Close()

		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 Service Unavailable on DB outage, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})
}
