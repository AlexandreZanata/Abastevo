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

type for15Env struct {
	pool         *pgxpool.Pool
	store        *forumadapters.Store
	runner       *identityadapters.Runner
	accountStore *accountadapters.PGStore
	hasher       accountdomain.SHA256Hasher
	router       chi.Router
	handler      *DiscussionHandler
}

func setupFOR15Env(t *testing.T) *for15Env {
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

	name := fmt.Sprintf("forum_for15_%d", time.Now().UnixNano())
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

	return &for15Env{
		pool:         pool,
		store:        store,
		runner:       runner,
		accountStore: accountStore,
		hasher:       hasher,
		router:       router,
		handler:      handler,
	}
}

type testAuthor struct {
	AccountID     string
	FamilyID      string
	ContributorID string
	KeyID         string
	AccessToken   string
}

func seedTestAuthor(t *testing.T, env *for15Env, prefix string, now time.Time) testAuthor {
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

func seedPublishedPost(t *testing.T, env *for15Env, postID string) {
	t.Helper()
	ctx := context.Background()
	sourceID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()
	q := `INSERT INTO forum_posts(
		id, kind, category, title, excerpt, body_text, publication_state, visibility,
		first_publication_seq, source_id, comments_enabled, local_created_at, local_updated_at,
		first_published_at, next_comment_sequence
	) VALUES (
		$1, 'EDITORIAL', 'FUELS', 'Post para discussao', 'Resumo', 'Corpo do post',
		'PUBLISHED', 'VISIBLE', 1, $2, true, $3, $3, $3, 1
	)`
	if _, err := env.pool.Exec(ctx, q, postID, sourceID, now); err != nil {
		t.Fatalf("seed post: %v", err)
	}
}

func TestDiscussionAdmission_PostgresIntegration_Gates(t *testing.T) {
	env := setupFOR15Env(t)
	now := time.Now().UTC()

	authorA := seedTestAuthor(t, env, "aaaa", now)
	authorB := seedTestAuthor(t, env, "bbbb", now)

	postID := "99999999-9999-4999-8999-000000000001"
	seedPublishedPost(t, env, postID)

	// Configure admission ports using real database guards
	env.handler.Sessions = func(ctx context.Context, familyID, accessToken string) (string, error) {
		// Look up account from account_session_families and verify hash
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
		// Mock verifier that validates contributor is bound to account
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
		if err := identityadapters.RequireQualitySigningKeyTx(ctx, tx, contribID, kID); err != nil {
			return err
		}
		return nil
	}

	opID1 := "77777777-7777-4777-8777-000000000001"
	bodyText1 := "Primeiro comentario real sobre o post."

	// -------------------------------------------------------------
	// Gate 1: Successful CreateComment under signed author
	// -------------------------------------------------------------
	t.Run("gate1_create_comment_success", func(t *testing.T) {
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
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal receipt: %v", err)
		}
		if receipt.PostID != postID || receipt.BodyText != bodyText1 || receipt.Replayed != false {
			t.Errorf("unexpected receipt values: %+v", receipt)
		}
		if receipt.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected PENDING_REVIEW initial state, got %s", receipt.PublicationState)
		}
	})

	// -------------------------------------------------------------
	// Gate 2: Session refresh replay (identical business intent with refreshed credentials)
	// -------------------------------------------------------------
	t.Run("gate2_session_refresh_replay", func(t *testing.T) {
		// Author A refreshes session: mint new family & access token for author A
		newFamilyID := "22222222-aaaa-4222-8222-000000000002"
		newToken := "token-aaaa-refreshed"
		tokenHash := env.hasher.Hash("salt-refreshed", newToken)
		_, err := env.pool.Exec(context.Background(),
			"INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'fixture','fixture',$3,$4,$5,$6)",
			newFamilyID, authorA.AccountID, "salt-refreshed", tokenHash, now.Add(2*time.Hour), now)
		if err != nil {
			t.Fatalf("insert refreshed family: %v", err)
		}

		// Re-attempt same operation ID with refreshed session credentials and identical body
		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "%s"
		}`, newFamilyID, newToken, opID1, bodyText1)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for refreshed session replay, got %d: %s", w.Code, w.Body.String())
		}
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal replay receipt: %v", err)
		}
		if receipt.Replayed != true {
			t.Errorf("expected replayed: true on token refresh replay, got false")
		}
		if receipt.BodyText != bodyText1 {
			t.Errorf("expected body text %q, got %q", bodyText1, receipt.BodyText)
		}
	})

	// -------------------------------------------------------------
	// Gate 3: Changed body conflict for same operation ID -> 409 Conflict
	// -------------------------------------------------------------
	t.Run("gate3_changed_body_conflict", func(t *testing.T) {
		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "Texto alterado com o mesmo operation id"
		}`, authorA.FamilyID, authorA.AccessToken, opID1)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for changed body, got %d: %s", w.Code, w.Body.String())
		}
	})

	// -------------------------------------------------------------
	// Gate 4: Cross-key replay (same account, different bound key/device)
	// -------------------------------------------------------------
	t.Run("gate4_cross_key_replay", func(t *testing.T) {
		// Register a second device / contributor key bound to Author A
		contrib2 := "33333333-aaaa-4333-8333-000000000002"
		key2 := "44444444-aaaa-4444-8444-000000000002"
		_, err := env.pool.Exec(context.Background(),
			"INSERT INTO identity_contributors(id,status) VALUES($1,'active')", contrib2)
		if err != nil {
			t.Fatalf("insert contrib2: %v", err)
		}
		_, err = env.pool.Exec(context.Background(),
			"INSERT INTO identity_keys(id,contributor_id,algorithm,public_jwk,fingerprint) VALUES($1,$2,'ecdsa-p256-sha512','{}',$3)",
			key2, contrib2, "fp-key2")
		if err != nil {
			t.Fatalf("insert key2: %v", err)
		}
		_, err = env.pool.Exec(context.Background(),
			"INSERT INTO account_contributor_bindings(account_id,contributor_id,key_fingerprint) VALUES($1,$2,'fp2')",
			authorA.AccountID, contrib2)
		if err != nil {
			t.Fatalf("insert binding2: %v", err)
		}

		// Configure SignedOwner temporarily to return the second key
		origSigned := env.handler.SignedOwner
		env.handler.SignedOwner = func(r *http.Request, accID string) (string, string, error) {
			return contrib2, key2, nil
		}
		defer func() { env.handler.SignedOwner = origSigned }()

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
			t.Fatalf("expected 200 OK for cross-key replay, got %d: %s", w.Code, w.Body.String())
		}
		var receipt CommentReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal cross-key receipt: %v", err)
		}
		if receipt.Replayed != true {
			t.Errorf("expected replayed: true for cross-key replay, got false")
		}
	})

	// -------------------------------------------------------------
	// Gate 5: Suspension during commit (Guard fails in tx -> fails closed)
	// -------------------------------------------------------------
	t.Run("gate5_suspension_during_commit", func(t *testing.T) {
		opID2 := "77777777-7777-4777-8777-000000000002"
		// Suspend author A in database
		_, err := env.pool.Exec(context.Background(), "UPDATE accounts SET status = 'suspended' WHERE id = $1", authorA.AccountID)
		if err != nil {
			t.Fatalf("suspend author: %v", err)
		}
		defer env.pool.Exec(context.Background(), "UPDATE accounts SET status = 'active' WHERE id = $1", authorA.AccountID)

		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "Tentativa enquanto suspenso"
		}`, authorA.FamilyID, authorA.AccessToken, opID2)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		// Must fail closed (401 or 503, never 200)
		if w.Code == http.StatusOK {
			t.Fatalf("security violation: suspended account was permitted to comment! code: %d", w.Code)
		}

		// Invariant: no row created in forum_comments
		var count int
		_ = env.pool.QueryRow(context.Background(), "SELECT count(*) FROM forum_comments WHERE create_operation_id = $1", opID2).Scan(&count)
		if count != 0 {
			t.Errorf("security violation: comment row was persisted for suspended author! count: %d", count)
		}
	})

	// -------------------------------------------------------------
	// Gate 6: Revoked session family during commit (Guard fails in tx)
	// -------------------------------------------------------------
	t.Run("gate6_revoked_session_during_commit", func(t *testing.T) {
		opID3 := "77777777-7777-4777-8777-000000000003"
		// Revoke session family in database
		_, err := env.pool.Exec(context.Background(), "UPDATE account_session_families SET revoked_at = now() WHERE id = $1", authorA.FamilyID)
		if err != nil {
			t.Fatalf("revoke family: %v", err)
		}
		defer env.pool.Exec(context.Background(), "UPDATE account_session_families SET revoked_at = NULL WHERE id = $1", authorA.FamilyID)

		reqBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"body_text": "Tentativa com sessao revogada"
		}`, authorA.FamilyID, authorA.AccessToken, opID3)

		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Fatalf("security violation: revoked session was permitted to comment! code: %d", w.Code)
		}
		var count int
		_ = env.pool.QueryRow(context.Background(), "SELECT count(*) FROM forum_comments WHERE create_operation_id = $1", opID3).Scan(&count)
		if count != 0 {
			t.Errorf("security violation: comment row was persisted for revoked session! count: %d", count)
		}
	})

	// -------------------------------------------------------------
	// Gate 7: Other-owner IDOR on EditComment -> 404 Not Found
	// -------------------------------------------------------------
	t.Run("gate7_other_owner_idor_edit_404", func(t *testing.T) {
		// Retrieve comment ID of Author A's comment
		var commentIDA string
		err := env.pool.QueryRow(context.Background(), "SELECT id FROM forum_comments WHERE create_operation_id = $1", opID1).Scan(&commentIDA)
		if err != nil {
			t.Fatalf("find author A comment: %v", err)
		}

		// Author B attempts to edit Author A's comment
		editBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "77777777-7777-4777-8777-000000000004",
			"expected_revision": 1,
			"body_text": "Tentativa IDOR de edicao por autor B"
		}`, authorB.FamilyID, authorB.AccessToken)

		req := httptest.NewRequest("POST", "/v1/forum/comments/"+commentIDA+"/edit", strings.NewReader(editBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("IDOR protection failed: expected 404 for foreign-owner edit, got %d: %s", w.Code, w.Body.String())
		}
	})

	// -------------------------------------------------------------
	// Gate 8: Other-owner IDOR on RemoveComment -> 404 Not Found
	// -------------------------------------------------------------
	t.Run("gate8_other_owner_idor_remove_404", func(t *testing.T) {
		var commentIDA string
		err := env.pool.QueryRow(context.Background(), "SELECT id FROM forum_comments WHERE create_operation_id = $1", opID1).Scan(&commentIDA)
		if err != nil {
			t.Fatalf("find author A comment: %v", err)
		}

		// Author B attempts to remove Author A's comment
		remBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "77777777-7777-4777-8777-000000000005",
			"expected_revision": 1
		}`, authorB.FamilyID, authorB.AccessToken)

		req := httptest.NewRequest("POST", "/v1/forum/comments/"+commentIDA+"/remove", strings.NewReader(remBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("IDOR protection failed: expected 404 for foreign-owner remove, got %d: %s", w.Code, w.Body.String())
		}
	})

	// -------------------------------------------------------------
	// Gate 9: Other-owner IDOR on OwnerRead -> 404 Not Found
	// -------------------------------------------------------------
	t.Run("gate9_other_owner_idor_read_404", func(t *testing.T) {
		var commentIDA string
		err := env.pool.QueryRow(context.Background(), "SELECT id FROM forum_comments WHERE create_operation_id = $1", opID1).Scan(&commentIDA)
		if err != nil {
			t.Fatalf("find author A comment: %v", err)
		}

		// Author B attempts to private-read Author A's comment
		readBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"comment_id": "%s"
		}`, authorB.FamilyID, authorB.AccessToken, commentIDA)

		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(readBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("IDOR protection failed: expected 404 for foreign-owner read, got %d: %s", w.Code, w.Body.String())
		}
	})

	// -------------------------------------------------------------
	// Gate 10: Private owner read allows author A to read pending comment, while public feed hides it
	// -------------------------------------------------------------
	t.Run("gate10_private_owner_vs_public_feed", func(t *testing.T) {
		var commentIDA string
		err := env.pool.QueryRow(context.Background(), "SELECT id FROM forum_comments WHERE create_operation_id = $1", opID1).Scan(&commentIDA)
		if err != nil {
			t.Fatalf("find author A comment: %v", err)
		}

		// Author A reads private pending comment via OwnerRead -> 200 OK with body text
		readBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"comment_id": "%s"
		}`, authorA.FamilyID, authorA.AccessToken, commentIDA)

		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(readBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for author A read, got %d: %s", w.Code, w.Body.String())
		}
		var ownerResp OwnerCommentResponseDTO
		if err := json.Unmarshal(w.Body.Bytes(), &ownerResp); err != nil {
			t.Fatalf("unmarshal owner response: %v", err)
		}
		if ownerResp.Comment.ID != commentIDA || ownerResp.Comment.BodyText != bodyText1 {
			t.Errorf("unexpected private comment receipt: %+v", ownerResp.Comment)
		}

		// Public anonymous request to GET /v1/forum/posts/{post_id}/comments -> returns empty items
		pubReq := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		wPub := httptest.NewRecorder()
		env.router.ServeHTTP(wPub, pubReq)

		if wPub.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for public comments, got %d", wPub.Code)
		}
		var pubResp CommentsResponseDTO
		if err := json.Unmarshal(wPub.Body.Bytes(), &pubResp); err != nil {
			t.Fatalf("unmarshal public response: %v", err)
		}
		if len(pubResp.Items) != 0 {
			t.Errorf("privacy invariant violated: pending comment was returned in public list: %+v", pubResp.Items)
		}
	})

	// -------------------------------------------------------------
	// Gate 11: 7-day attempt expiry and purge
	// -------------------------------------------------------------
	t.Run("gate11_seven_day_attempt_expiry", func(t *testing.T) {
		// Update expires_at of an attempt row in identity_idempotency to 8 days in past
		_, err := env.pool.Exec(context.Background(),
			"UPDATE identity_idempotency SET expires_at = now() - interval '8 days' WHERE key = $1", opID1)
		if err != nil {
			t.Fatalf("expire attempt: %v", err)
		}

		purged, _, err := env.runner.PurgeExpiredAttempts(context.Background(), time.Now().UTC(), 100)
		if err != nil {
			t.Fatalf("purge expired attempts: %v", err)
		}
		if purged < 1 {
			t.Errorf("expected at least 1 purged expired attempt, got %d", purged)
		}

		// Verify row is deleted from identity_idempotency
		var count int
		_ = env.pool.QueryRow(context.Background(), "SELECT count(*) FROM identity_idempotency WHERE key = $1", opID1).Scan(&count)
		if count != 0 {
			t.Errorf("expected 0 attempts remaining for purged key, got %d", count)
		}
	})
}
