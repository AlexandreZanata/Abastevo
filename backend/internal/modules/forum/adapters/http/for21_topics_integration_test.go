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
	"strconv"
	"strings"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type for21Env struct {
	pool          *pgxpool.Pool
	store         *forumadapters.Store
	runner        *identityadapters.Runner
	accountStore  *accountadapters.PGStore
	hasher        accountdomain.SHA256Hasher
	router        *chi.Mux
	handler       *DiscussionHandler
	publicHandler *Handler
}

func setupFOR21Env(t *testing.T) *for21Env {
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

	name := fmt.Sprintf("forum_for21_%d", time.Now().UnixNano())
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

	// Configure limiter with 3 writes per hour (B-BR-FOR-11 hourly topic quota)
	policy := identitydomain.QuotaPolicy{
		Version: 1,
		Ops: map[string]identitydomain.OperationQuota{
			identitydomain.OperationWrite: {Limit: 3, Window: time.Hour},
		},
	}
	limiter := identityadapters.NewLimiter(pool, policy)
	store.SetQuotaChecker(limiter)

	forumCfg := domain.DefaultConfig()
	forumCfg.ReadsEnabled = true
	forumCfg.WritesEnabled = true
	forumCfg.AdminEnabled = true

	handler := &DiscussionHandler{
		Comments: store,
		Topics:   store,
		Reports:  store,
		Runner:   runner,
		Config:   &forumCfg,
	}

	forumService := application.NewService(forumCfg, store, store, application.SystemClock{})
	publicHandler := NewHandler(forumService, []byte("cursor-secret-123"))

	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	publicHandler.RegisterRoutes(router)

	return &for21Env{
		pool:          pool,
		store:         store,
		runner:        runner,
		accountStore:  accountStore,
		hasher:        hasher,
		router:        router,
		handler:       handler,
		publicHandler: publicHandler,
	}
}

func seedTestAuthorFOR21(t *testing.T, env *for21Env, prefix string, now time.Time) testAuthor {
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

// TestTopicEndpoints_PostgresIntegration_Gates verifies end-to-end integration for topic endpoints against real PostgreSQL.
func TestTopicEndpoints_PostgresIntegration_Gates(t *testing.T) {
	env := setupFOR21Env(t)
	now := time.Now().UTC()

	authorA := seedTestAuthorFOR21(t, env, "aaaa", now)
	authorB := seedTestAuthorFOR21(t, env, "bbbb", now)

	// Configure real auth wiring matching cmd/api/main.go
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

	env.handler.SignedOwner = func(r *http.Request, accountID string) (string, string, error) {
		var contribID, keyID string
		err := env.pool.QueryRow(r.Context(),
			"SELECT b.contributor_id, k.id FROM account_contributor_bindings b JOIN identity_keys k ON k.contributor_id = b.contributor_id::uuid WHERE b.account_id = $1 LIMIT 1", accountID).
			Scan(&contribID, &keyID)
		if err != nil {
			return "", "", fmt.Errorf("unbound contributor: %w", err)
		}
		return contribID, keyID, nil
	}

	env.handler.Guard = func(ctx context.Context, tx pgx.Tx, accountID, familyID, accessToken, contributorID, keyID string) error {
		if err := env.accountStore.RequireQualitySessionTx(ctx, tx, accountID, familyID, accessToken, contributorID, env.hasher, time.Now().Unix()); err != nil {
			return err
		}
		return identityadapters.RequireQualitySigningKeyTx(ctx, tx, contributorID, keyID)
	}

	var topicID string
	createOpID := "77777777-1111-4777-8777-000000000001"

	// Gate 1: Signed Topic Creation (201 Created) and Premoderation Default (never optimistic publication)
	t.Run("Gate1_SignedTopicCreate_PremoderationDefault", func(t *testing.T) {
		body := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"category": "FUELS",
			"title": "Variacao do preco de gasolina em Campinas",
			"body_text": "Observamos grande variacao entre postos da regiao central e rodovias.",
			"public_alias": "MotoristaCampinas",
			"region_uf": "SP",
			"region_municipality_ibge": 3509502,
			"fuel_product": "GASOLINE_REGULAR"
		}`, authorA.FamilyID, authorA.AccessToken, createOpID)

		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store, got %q", w.Header().Get("Cache-Control"))
		}

		var receipt TopicReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if receipt.ID == "" {
			t.Fatal("expected non-empty topic id")
		}
		topicID = receipt.ID
		if receipt.Kind != "TOPIC" {
			t.Errorf("expected kind TOPIC, got %s", receipt.Kind)
		}
		if receipt.Category != "FUELS" {
			t.Errorf("expected category FUELS, got %s", receipt.Category)
		}
		if receipt.Revision != 1 {
			t.Errorf("expected revision 1, got %d", receipt.Revision)
		}
		if receipt.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected publication_state PENDING_REVIEW, got %s", receipt.PublicationState)
		}
		if receipt.Visibility != "VISIBLE" {
			t.Errorf("expected visibility VISIBLE, got %s", receipt.Visibility)
		}
		if receipt.Replayed {
			t.Errorf("expected replayed: false, got true")
		}
		if receipt.Region == nil || receipt.Region.UF != "SP" || receipt.Region.MunicipalityIBGE == nil || *receipt.Region.MunicipalityIBGE != 3509502 {
			t.Errorf("unexpected region: %+v", receipt.Region)
		}
		if receipt.FuelProduct == nil || *receipt.FuelProduct != "GASOLINE_REGULAR" {
			t.Errorf("unexpected fuel product: %v", receipt.FuelProduct)
		}

		// Public feed and detail isolation: PENDING_REVIEW topics MUST NOT appear in public projection
		feedReq := httptest.NewRequest("GET", "/v1/forum/posts", nil)
		feedW := httptest.NewRecorder()
		env.router.ServeHTTP(feedW, feedReq)
		if feedW.Code != http.StatusOK {
			t.Fatalf("expected 200 on public feed, got %d: %s", feedW.Code, feedW.Body.String())
		}
		if strings.Contains(feedW.Body.String(), topicID) {
			t.Fatalf("LEAK: pending topic %s found in public feed: %s", topicID, feedW.Body.String())
		}

		detailReq := httptest.NewRequest("GET", "/v1/forum/posts/"+topicID, nil)
		detailW := httptest.NewRecorder()
		env.router.ServeHTTP(detailW, detailReq)
		if detailW.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unapproved topic in public detail, got %d: %s", detailW.Code, detailW.Body.String())
		}
	})

	// Gate 2: Idempotent Replay (200 OK) and Conflicting Intent Defense (409 Conflict)
	t.Run("Gate2_IdempotentReplayAndConflictDefense", func(t *testing.T) {
		// Exact replay -> 200 OK with replayed: true
		body := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"category": "FUELS",
			"title": "Variacao do preco de gasolina em Campinas",
			"body_text": "Observamos grande variacao entre postos da regiao central e rodovias.",
			"public_alias": "MotoristaCampinas",
			"region_uf": "SP",
			"region_municipality_ibge": 3509502,
			"fuel_product": "GASOLINE_REGULAR"
		}`, authorA.FamilyID, authorA.AccessToken, createOpID)

		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on replay, got %d: %s", w.Code, w.Body.String())
		}
		var receipt TopicReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if receipt.ID != topicID {
			t.Errorf("expected same topic ID %s, got %s", topicID, receipt.ID)
		}
		if !receipt.Replayed {
			t.Errorf("expected replayed: true on replay, got false")
		}

		// Conflicting intent on same operation_id -> 409 Conflict
		conflictBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"category": "ELECTRIC",
			"title": "Conflito intencional",
			"body_text": "Outro corpo completamente diferente."
		}`, authorA.FamilyID, authorA.AccessToken, createOpID)

		conflictReq := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(conflictBody))
		conflictReq.Header.Set("Content-Type", "application/json")
		conflictW := httptest.NewRecorder()
		env.router.ServeHTTP(conflictW, conflictReq)

		if conflictW.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict on altered payload, got %d: %s", conflictW.Code, conflictW.Body.String())
		}
	})

	// Gate 3: Owner Read and Foreign Owner IDOR Defense
	t.Run("Gate3_OwnerReadAndIDORDefense", func(t *testing.T) {
		// Author A reads own topic -> 200 OK with TopicReceipt
		readBody := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","topic_id":"%s"}`, authorA.FamilyID, authorA.AccessToken, topicID)
		readReq := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(readBody))
		readReq.Header.Set("Content-Type", "application/json")
		readW := httptest.NewRecorder()
		env.router.ServeHTTP(readW, readReq)

		if readW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on owner read, got %d: %s", readW.Code, readW.Body.String())
		}
		var resp OwnerTopicResponseDTO
		if err := json.Unmarshal(readW.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Topic.ID != topicID {
			t.Errorf("expected topic ID %s, got %s", topicID, resp.Topic.ID)
		}
		if resp.Topic.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected PENDING_REVIEW, got %s", resp.Topic.PublicationState)
		}

		// Author B (foreign account) attempts to read Author A's topic -> 404 (IDOR defense)
		idorBody := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","topic_id":"%s"}`, authorB.FamilyID, authorB.AccessToken, topicID)
		idorReq := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(idorBody))
		idorReq.Header.Set("Content-Type", "application/json")
		idorW := httptest.NewRecorder()
		env.router.ServeHTTP(idorW, idorReq)

		if idorW.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for IDOR owner read, got %d: %s", idorW.Code, idorW.Body.String())
		}
	})

	// Gate 4: CAS Edit of Authored Topic and Foreign Edit IDOR Defense
	t.Run("Gate4_CASEditAndForeignDefense", func(t *testing.T) {
		editOpID := "77777777-2222-4777-8777-000000000001"

		// Author B attempts to edit Author A's topic -> 404
		foreignEditBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1,
			"category": "FUELS",
			"title": "Tentativa de edicao invasiva",
			"body_text": "Texto invasivo"
		}`, authorB.FamilyID, authorB.AccessToken, editOpID)

		foreignEditReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/edit", strings.NewReader(foreignEditBody))
		foreignEditReq.Header.Set("Content-Type", "application/json")
		foreignEditW := httptest.NewRecorder()
		env.router.ServeHTTP(foreignEditW, foreignEditReq)

		if foreignEditW.Code != http.StatusNotFound {
			t.Fatalf("expected 404 on foreign edit, got %d: %s", foreignEditW.Code, foreignEditW.Body.String())
		}

		// Author A performs legitimate CAS edit with expected_revision: 1
		validEditBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1,
			"category": "FUELS",
			"title": "Variacao de precos em Campinas atualizada",
			"body_text": "Observamos nova variacao nos postos da regiao central apos reajuste.",
			"region_uf": "SP",
			"region_municipality_ibge": 3509502,
			"fuel_product": "GASOLINE_REGULAR"
		}`, authorA.FamilyID, authorA.AccessToken, editOpID)

		validEditReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/edit", strings.NewReader(validEditBody))
		validEditReq.Header.Set("Content-Type", "application/json")
		validEditW := httptest.NewRecorder()
		env.router.ServeHTTP(validEditW, validEditReq)

		if validEditW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on valid CAS edit, got %d: %s", validEditW.Code, validEditW.Body.String())
		}
		var receipt TopicReceiptDTO
		if err := json.Unmarshal(validEditW.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if receipt.Revision != 2 {
			t.Errorf("expected revision 2, got %d", receipt.Revision)
		}
		if receipt.Title != "Variacao de precos em Campinas atualizada" {
			t.Errorf("unexpected updated title: %s", receipt.Title)
		}
		if receipt.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected edited topic to stay in PENDING_REVIEW, got %s", receipt.PublicationState)
		}

		// Stale CAS edit with expected_revision: 1 -> 409 Conflict
		staleOpID := "77777777-3333-4777-8777-000000000001"
		staleEditBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 1,
			"category": "FUELS",
			"title": "Edicao concorrente desatualizada",
			"body_text": "Texto que deve falhar."
		}`, authorA.FamilyID, authorA.AccessToken, staleOpID)

		staleReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/edit", strings.NewReader(staleEditBody))
		staleReq.Header.Set("Content-Type", "application/json")
		staleW := httptest.NewRecorder()
		env.router.ServeHTTP(staleW, staleReq)

		if staleW.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict on stale revision, got %d: %s", staleW.Code, staleW.Body.String())
		}
	})

	// Gate 5: Tombstone Removal and Terminal Removal State
	t.Run("Gate5_TombstoneRemovalAndTerminalState", func(t *testing.T) {
		removeOpID := "77777777-4444-4777-8777-000000000001"

		// Author B attempts to remove Author A's topic -> 404
		foreignRemoveBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 2
		}`, authorB.FamilyID, authorB.AccessToken, removeOpID)

		foreignRemoveReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/remove", strings.NewReader(foreignRemoveBody))
		foreignRemoveReq.Header.Set("Content-Type", "application/json")
		foreignRemoveW := httptest.NewRecorder()
		env.router.ServeHTTP(foreignRemoveW, foreignRemoveReq)

		if foreignRemoveW.Code != http.StatusNotFound {
			t.Fatalf("expected 404 on foreign remove, got %d: %s", foreignRemoveW.Code, foreignRemoveW.Body.String())
		}

		// Author A removes topic with expected_revision: 2 -> 200 OK
		validRemoveBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 2
		}`, authorA.FamilyID, authorA.AccessToken, removeOpID)

		validRemoveReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/remove", strings.NewReader(validRemoveBody))
		validRemoveReq.Header.Set("Content-Type", "application/json")
		validRemoveW := httptest.NewRecorder()
		env.router.ServeHTTP(validRemoveW, validRemoveReq)

		if validRemoveW.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on remove, got %d: %s", validRemoveW.Code, validRemoveW.Body.String())
		}
		var receipt TopicReceiptDTO
		if err := json.Unmarshal(validRemoveW.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if receipt.PublicationState != "REMOVED" {
			t.Errorf("expected publication_state REMOVED, got %s", receipt.PublicationState)
		}
		if receipt.Visibility != "HIDDEN" {
			t.Errorf("expected visibility HIDDEN, got %s", receipt.Visibility)
		}
		if receipt.BodyText != "" {
			t.Errorf("expected empty body_text on tombstoned topic, got %q", receipt.BodyText)
		}

		// Subsequent edit on removed topic -> 400 Bad Request (ErrTopicAlreadyRemoved)
		postRemoveEditOp := "77777777-5555-4777-8777-000000000001"
		postRemoveEditBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 3,
			"category": "FUELS",
			"title": "Tentando editar topico removido",
			"body_text": "Texto posterior a remocao."
		}`, authorA.FamilyID, authorA.AccessToken, postRemoveEditOp)

		postRemoveEditReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/edit", strings.NewReader(postRemoveEditBody))
		postRemoveEditReq.Header.Set("Content-Type", "application/json")
		postRemoveEditW := httptest.NewRecorder()
		env.router.ServeHTTP(postRemoveEditW, postRemoveEditReq)

		if postRemoveEditW.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 on edit of removed topic, got %d: %s", postRemoveEditW.Code, postRemoveEditW.Body.String())
		}

		// Subsequent remove on removed topic -> 400 Bad Request (ErrTopicAlreadyRemoved)
		postRemoveRemoveOp := "77777777-6666-4777-8777-000000000001"
		postRemoveRemoveBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"expected_revision": 3
		}`, authorA.FamilyID, authorA.AccessToken, postRemoveRemoveOp)

		postRemoveRemoveReq := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/remove", strings.NewReader(postRemoveRemoveBody))
		postRemoveRemoveReq.Header.Set("Content-Type", "application/json")
		postRemoveRemoveW := httptest.NewRecorder()
		env.router.ServeHTTP(postRemoveRemoveW, postRemoveRemoveReq)

		if postRemoveRemoveW.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 on remove of removed topic, got %d: %s", postRemoveRemoveW.Code, postRemoveRemoveW.Body.String())
		}
	})

	// Gate 6: Quota Enforcement via HTTP API (3 topics/hour ceiling -> 429)
	t.Run("Gate6_QuotaEnforcement_429", func(t *testing.T) {
		// Author B creates up to hourly quota (3 topics per hour)
		for i := 1; i <= 3; i++ {
			quotaOpID := fmt.Sprintf("88888888-000%d-4888-8888-000000000001", i)
			body := fmt.Sprintf(`{
				"family_id": "%s",
				"access_token": "%s",
				"operation_id": "%s",
				"category": "ELECTRIC",
				"title": "Topico de cota %d",
				"body_text": "Corpo valido para teste de cota."
			}`, authorB.FamilyID, authorB.AccessToken, quotaOpID, i)

			req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			env.router.ServeHTTP(w, req)

			if w.Code != http.StatusCreated {
				t.Fatalf("topic %d creation expected 201 Created, got %d: %s", i, w.Code, w.Body.String())
			}
		}

		// 4th creation attempt by Author B exceeds hourly quota -> 429 Too Many Requests
		exceededOpID := "88888888-0004-4888-8888-000000000001"
		exceededBody := fmt.Sprintf(`{
			"family_id": "%s",
			"access_token": "%s",
			"operation_id": "%s",
			"category": "ELECTRIC",
			"title": "Topico excedente de cota",
			"body_text": "Este topico deve ser rejeitado por cota horaria."
		}`, authorB.FamilyID, authorB.AccessToken, exceededOpID)

		exceededReq := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(exceededBody))
		exceededReq.Header.Set("Content-Type", "application/json")
		exceededW := httptest.NewRecorder()
		env.router.ServeHTTP(exceededW, exceededReq)

		if exceededW.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 Too Many Requests on 4th topic, got %d: %s", exceededW.Code, exceededW.Body.String())
		}
		retry := exceededW.Header().Get("Retry-After")
		if n, err := strconv.Atoi(retry); err != nil || n <= 0 || n > 3600 {
			t.Errorf("expected positive Retry-After <= 3600, got %q", retry)
		}
		if exceededW.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})
}
