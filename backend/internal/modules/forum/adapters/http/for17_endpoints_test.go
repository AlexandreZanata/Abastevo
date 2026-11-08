package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	"github.com/go-chi/chi/v5"
)

func setupDiscussionTestRouter(handler *DiscussionHandler) chi.Router {
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return r
}

func defaultTestPrincipal() (string, string, string, string, string) {
	return "11111111-1111-4111-8111-000000000001", // accountID
		"22222222-2222-4222-8222-000000000001", // familyID
		"access-token-123", // accessToken
		"33333333-3333-4333-8333-000000000001", // contributorID
		"44444444-4444-4444-8444-000000000001" // keyID
}

func wireMockAuth(h *DiscussionHandler) {
	accID, famID, token, contribID, keyID := defaultTestPrincipal()
	h.Sessions = func(ctx context.Context, familyID, accessToken string) (string, error) {
		if familyID == famID && accessToken == token {
			return accID, nil
		}
		return "", domain.ErrForumDisabled
	}
	h.SignedOwner = func(r *http.Request, accountID string) (string, string, error) {
		if accountID == accID {
			return contribID, keyID, nil
		}
		return "", "", domain.ErrCommentAuthorMismatch
	}
}

// TestFOR17_RollbackAndFeatureGates verifies runtime feature gate switching and rollback behavior.
func TestFOR17_RollbackAndFeatureGates(t *testing.T) {
	store := newMockCommentStore()
	h := &DiscussionHandler{
		Comments: store,
	}
	wireMockAuth(h)

	router := setupDiscussionTestRouter(h)
	postID := "99999999-9999-4999-8999-000000000001"
	_, famID, token, _, _ := defaultTestPrincipal()

	// 1. Fully disabled forum (ReadsEnabled: false, WritesEnabled: false)
	disabledCfg := domain.DefaultConfig()
	disabledCfg.ReadsEnabled = false
	disabledCfg.WritesEnabled = false
	h.Config = &disabledCfg

	t.Run("reads_disabled_returns_503_on_list", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})

	t.Run("reads_disabled_returns_503_on_replies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/comments/"+postID+"/replies", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("writes_disabled_returns_503_on_create", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","body_text":"ola"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	// 2. Rollback mode: ReadsEnabled: true, WritesEnabled: false (public reads allowed, writes blocked)
	rollbackCfg := domain.DefaultConfig()
	rollbackCfg.ReadsEnabled = true
	rollbackCfg.WritesEnabled = false
	h.Config = &rollbackCfg

	t.Run("rollback_mode_allows_public_read", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})

	t.Run("rollback_mode_blocks_create", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","body_text":"ola"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 in rollback mode, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("rollback_mode_blocks_edit", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","expected_revision":1,"body_text":"editado"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/comments/"+postID+"/edit", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 in rollback mode, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("rollback_mode_blocks_remove", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","expected_revision":1}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/comments/"+postID+"/remove", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 in rollback mode, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestFOR17_PayloadSizeAndBadRequests tests body size limits, input bounds, and bad request cases.
func TestFOR17_PayloadSizeAndBadRequests(t *testing.T) {
	store := newMockCommentStore()
	h := &DiscussionHandler{
		Comments: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)
	postID := "99999999-9999-4999-8999-000000000001"
	_, famID, token, _, _ := defaultTestPrincipal()

	t.Run("oversized_payload_exceeding_16kib_returns_400", func(t *testing.T) {
		hugeText := strings.Repeat("A", 20000)
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","body_text":"%s"}`, famID, token, hugeText)
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for oversized body, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "16KiB") {
			t.Errorf("expected error message to mention 16KiB limit, got: %s", w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})

	t.Run("invalid_limit_below_1_returns_400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments?limit=0", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_limit_above_50_returns_400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments?limit=51", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_cursor_returns_400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments?cursor=-1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_uuid_in_path_returns_400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/forum/posts/invalid-uuid/comments", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("body_exceeding_280_chars_returns_400", func(t *testing.T) {
		longText := strings.Repeat("x", 281)
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","body_text":"%s"}`, famID, token, longText)
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestFOR17_RateAndBudgetErrors verifies 429 status and Retry-After headers.
func TestFOR17_RateAndBudgetErrors(t *testing.T) {
	store := newMockCommentStore()
	h := &DiscussionHandler{
		Comments: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)
	postID := "99999999-9999-4999-8999-000000000001"
	_, famID, token, _, _ := defaultTestPrincipal()

	t.Run("quota_error_returns_429_with_retry_after", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error) {
			return application.CommentResult{}, &identityadapters.QuotaError{RetryAfter: 45 * time.Second}
		}
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","body_text":"texto"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
		}
		if retry := w.Header().Get("Retry-After"); retry != "45" {
			t.Errorf("expected Retry-After: 45, got %q", retry)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})

	t.Run("live_budget_exceeded_returns_429_with_retry_after", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateCommentCommand) (application.CommentResult, error) {
			return application.CommentResult{}, domain.ErrCommentLiveQuotaExceeded
		}
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","body_text":"texto"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/posts/"+postID+"/comments", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
		}
		if retry := w.Header().Get("Retry-After"); retry != "3600" {
			t.Errorf("expected Retry-After: 3600, got %q", retry)
		}
	})
}

// TestFOR17_PublicIsolationAndReplyCount verifies public responses include reply_count and exclude private fields.
func TestFOR17_PublicIsolationAndReplyCount(t *testing.T) {
	store := newMockCommentStore()
	h := &DiscussionHandler{
		Comments: store,
	}
	router := setupDiscussionTestRouter(h)
	postID := "99999999-9999-4999-8999-000000000001"
	now := time.Now().UTC()

	store.listFn = func(ctx context.Context, q application.ListPostCommentsQuery) ([]domain.Comment, error) {
		return []domain.Comment{
			{
				ID:                "comment-1",
				PostID:            postID,
				AuthorAccountID:   "secret-account-id-1",
				PublicAlias:       "Motorista123",
				Sequence:          1,
				Revision:          1,
				BodyText:          "Comentario com respostas",
				PublicationState:  domain.StatePublished,
				Visibility:        domain.VisibilityVisible,
				CreatedAt:         now,
				CreateOperationID: "secret-op-id-1",
				BodySHA256:        "secret-sha-1",
				ReplyCount:        3,
			},
		}, nil
	}

	req := httptest.NewRequest("GET", "/v1/forum/posts/"+postID+"/comments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected Cache-Control: no-store")
	}

	var resp CommentsResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}

	item := resp.Items[0]
	if item.ReplyCount != 3 {
		t.Errorf("expected ReplyCount 3, got %d", item.ReplyCount)
	}
	if item.PublicAlias != "Motorista123" {
		t.Errorf("expected PublicAlias Motorista123, got %s", item.PublicAlias)
	}

	rawJSON := w.Body.String()
	if strings.Contains(rawJSON, "secret-account-id") {
		t.Errorf("LEAK: author_account_id found in public response: %s", rawJSON)
	}
	if strings.Contains(rawJSON, "secret-op-id") {
		t.Errorf("LEAK: create_operation_id found in public response: %s", rawJSON)
	}
	if strings.Contains(rawJSON, "secret-sha") {
		t.Errorf("LEAK: body_sha256 found in public response: %s", rawJSON)
	}
}
