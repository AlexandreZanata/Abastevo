package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

type mockTopicStore struct {
	createFn  func(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error)
	editFn    func(ctx context.Context, cmd application.EditTopicCommand) (*domain.Post, error)
	removeFn  func(ctx context.Context, cmd application.RemoveTopicCommand) (*domain.Post, error)
	getByIDFn func(ctx context.Context, id string) (*domain.Post, error)
}

func (m *mockTopicStore) CreateTopic(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
	if m.createFn != nil {
		return m.createFn(ctx, cmd)
	}
	return application.TopicResult{}, errors.New("not implemented")
}

func (m *mockTopicStore) EditTopic(ctx context.Context, cmd application.EditTopicCommand) (*domain.Post, error) {
	if m.editFn != nil {
		return m.editFn(ctx, cmd)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTopicStore) RemoveTopic(ctx context.Context, cmd application.RemoveTopicCommand) (*domain.Post, error) {
	if m.removeFn != nil {
		return m.removeFn(ctx, cmd)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTopicStore) GetTopicByID(ctx context.Context, id string) (*domain.Post, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrTopicNotFound
}

// TestFOR21_RollbackAndFeatureGates verifies runtime feature gate switching and rollback behavior for topic routes.
func TestFOR21_RollbackAndFeatureGates(t *testing.T) {
	store := &mockTopicStore{}
	h := &DiscussionHandler{
		Topics: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)

	topicID := "99999999-9999-4999-8999-000000000001"
	_, famID, token, _, _ := defaultTestPrincipal()

	disabledCfg := domain.DefaultConfig()
	disabledCfg.ReadsEnabled = false
	disabledCfg.WritesEnabled = false
	h.Config = &disabledCfg

	t.Run("writes_disabled_returns_503_on_create_topic", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Preco em SP","body_text":"Corpo valido"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}
	})

	t.Run("writes_disabled_returns_503_on_edit_topic", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","expected_revision":1,"category":"FUELS","title":"Preco editado","body_text":"Corpo editado"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/edit", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("writes_disabled_returns_503_on_remove_topic", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","expected_revision":1}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics/"+topicID+"/remove", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("reads_disabled_returns_503_on_owner_read_topic", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","topic_id":"%s"}`, famID, token, topicID)
		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestFOR21_InputValidationAndBounds verifies bounds, character constraints, and malformed inputs.
func TestFOR21_InputValidationAndBounds(t *testing.T) {
	store := &mockTopicStore{}
	h := &DiscussionHandler{
		Topics: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)

	topicID := "99999999-9999-4999-8999-000000000001"
	_, famID, token, _, _ := defaultTestPrincipal()

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		wantStatus int
	}{
		{
			name:       "empty_title",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"","body_text":"Corpo"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized_title_gt_120",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"%s","body_text":"Corpo"}`, famID, token, strings.Repeat("A", 121)),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_body",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"   "}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized_body_gt_4000",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"%s"}`, famID, token, strings.Repeat("B", 4001)),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_category",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"NOT_A_CATEGORY","title":"Titulo","body_text":"Corpo"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_uf",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo","region_uf":"XX"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "mismatched_ibge_prefix",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo","region_uf":"SP","region_municipality_ibge":3304557}`, famID, token), // 33 is RJ
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_fuel_product",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo","fuel_product":"KEROSENE"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_station_uuid",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo","station_id":"not-a-uuid"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_operation_uuid",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"not-uuid","category":"FUELS","title":"Titulo","body_text":"Corpo"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "edit_expected_revision_zero",
			method:     "POST",
			url:        "/v1/forum/topics/" + topicID + "/edit",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","expected_revision":0,"category":"FUELS","title":"Titulo","body_text":"Corpo"}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "remove_expected_revision_zero",
			method:     "POST",
			url:        "/v1/forum/topics/" + topicID + "/remove",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","expected_revision":0}`, famID, token),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "payload_exceeding_16kib",
			method:     "POST",
			url:        "/v1/forum/topics",
			body:       fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"%s"}`, famID, token, strings.Repeat("A", 17000)),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("expected Cache-Control: no-store")
			}
		})
	}
}

// TestFOR21_OwnerReadValidationAndIDOR verifies owner read parameter rules and IDOR protections.
func TestFOR21_OwnerReadValidationAndIDOR(t *testing.T) {
	accID, famID, token, _, _ := defaultTestPrincipal()
	topicID := "99999999-9999-4999-8999-000000000001"
	otherAccountID := "00000000-0000-4000-8000-000000000099"

	store := &mockTopicStore{
		getByIDFn: func(ctx context.Context, id string) (*domain.Post, error) {
			if id == topicID {
				now := time.Now().UTC()
				return &domain.Post{
					ID:               topicID,
					Kind:             domain.KindTopic,
					Category:         domain.CategoryFuels,
					Title:            "Topico do Dono",
					BodyText:         "Corpo privado",
					AuthorAccountID:  accID,
					Revision:         1,
					PublicationState: domain.StatePendingReview,
					Visibility:       domain.VisibilityVisible,
					LocalCreatedAt:   now,
					LocalUpdatedAt:   now,
				}, nil
			}
			return nil, domain.ErrTopicNotFound
		},
	}

	h := &DiscussionHandler{
		Topics: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)

	t.Run("neither_comment_nor_topic_id_returns_400", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("both_comment_and_topic_id_returns_400", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","comment_id":"%s","topic_id":"%s"}`, famID, token, topicID, topicID)
		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_topic_uuid_returns_400", func(t *testing.T) {
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","topic_id":"not-a-uuid"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("foreign_owner_idor_returns_404", func(t *testing.T) {
		foreignTopicID := "88888888-8888-4888-8888-000000000001"
		store.getByIDFn = func(ctx context.Context, id string) (*domain.Post, error) {
			if id == foreignTopicID {
				now := time.Now().UTC()
				return &domain.Post{
					ID:               foreignTopicID,
					Kind:             domain.KindTopic,
					Category:         domain.CategoryFuels,
					Title:            "Topico Alheio",
					BodyText:         "Corpo alheio",
					AuthorAccountID:  otherAccountID, // foreign author
					Revision:         1,
					PublicationState: domain.StatePendingReview,
					Visibility:       domain.VisibilityVisible,
					LocalCreatedAt:   now,
					LocalUpdatedAt:   now,
				}, nil
			}
			return nil, domain.ErrTopicNotFound
		}

		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","topic_id":"%s"}`, famID, token, foreignTopicID)
		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for IDOR, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("matching_owner_returns_200_with_topic_receipt", func(t *testing.T) {
		store.getByIDFn = func(ctx context.Context, id string) (*domain.Post, error) {
			if id == topicID {
				now := time.Now().UTC()
				return &domain.Post{
					ID:               topicID,
					Kind:             domain.KindTopic,
					Category:         domain.CategoryFuels,
					Title:            "Topico do Dono",
					BodyText:         "Corpo privado",
					AuthorAccountID:  accID,
					Revision:         1,
					PublicationState: domain.StatePendingReview,
					Visibility:       domain.VisibilityVisible,
					LocalCreatedAt:   now,
					LocalUpdatedAt:   now,
				}, nil
			}
			return nil, domain.ErrTopicNotFound
		}

		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","topic_id":"%s"}`, famID, token, topicID)
		req := httptest.NewRequest("POST", "/v1/forum/owner/read", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp OwnerTopicResponseDTO
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Topic.ID != topicID {
			t.Errorf("expected topic ID %s, got %s", topicID, resp.Topic.ID)
		}
		if resp.Topic.Title != "Topico do Dono" {
			t.Errorf("expected title 'Topico do Dono', got %s", resp.Topic.Title)
		}
		if resp.Topic.PublicationState != "PENDING_REVIEW" {
			t.Errorf("expected PENDING_REVIEW, got %s", resp.Topic.PublicationState)
		}
	})
}

// TestFOR21_QuotaAndRateLimitErrors verifies proper 429 status and Retry-After headers for topic quotas.
func TestFOR21_QuotaAndRateLimitErrors(t *testing.T) {
	store := &mockTopicStore{}
	h := &DiscussionHandler{
		Topics: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)
	_, famID, token, _, _ := defaultTestPrincipal()

	t.Run("live_quota_exceeded_returns_429_retry_after_3600", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
			return application.TopicResult{}, domain.ErrTopicLiveQuotaExceeded
		}
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
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

	t.Run("hourly_quota_exceeded_returns_429_retry_after_3600", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
			return application.TopicResult{}, domain.ErrTopicHourlyQuotaExceeded
		}
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
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

	t.Run("daily_quota_exceeded_returns_429_retry_after_86400", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
			return application.TopicResult{}, domain.ErrTopicDailyQuotaExceeded
		}
		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo","body_text":"Corpo"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
		}
		if retry := w.Header().Get("Retry-After"); retry != "86400" {
			t.Errorf("expected Retry-After: 86400, got %q", retry)
		}
	})
}

// TestFOR21_IdempotentReplayAndCacheControl verifies 201 Created on fresh insert and 200 OK on replay.
func TestFOR21_IdempotentReplayAndCacheControl(t *testing.T) {
	accID, famID, token, _, _ := defaultTestPrincipal()
	topicID := "99999999-9999-4999-8999-000000000001"
	now := time.Now().UTC()

	sampleTopic := &domain.Post{
		ID:               topicID,
		Kind:             domain.KindTopic,
		Category:         domain.CategoryFuels,
		Title:            "Titulo Idempotente",
		BodyText:         "Corpo do topico",
		AuthorAccountID:  accID,
		Revision:         1,
		PublicationState: domain.StatePendingReview,
		Visibility:       domain.VisibilityVisible,
		LocalCreatedAt:   now,
		LocalUpdatedAt:   now,
	}

	store := &mockTopicStore{}
	h := &DiscussionHandler{
		Topics: store,
	}
	wireMockAuth(h)
	router := setupDiscussionTestRouter(h)

	t.Run("fresh_topic_create_returns_201_replayed_false", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
			return application.TopicResult{Topic: sampleTopic, Replayed: false}, nil
		}

		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo Idempotente","body_text":"Corpo do topico"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}

		var receipt TopicReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if receipt.Replayed {
			t.Errorf("expected replayed: false, got true")
		}
	})

	t.Run("replayed_topic_create_returns_200_replayed_true", func(t *testing.T) {
		store.createFn = func(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
			return application.TopicResult{Topic: sampleTopic, Replayed: true}, nil
		}

		body := fmt.Sprintf(`{"family_id":"%s","access_token":"%s","operation_id":"77777777-7777-4777-8777-000000000001","category":"FUELS","title":"Titulo Idempotente","body_text":"Corpo do topico"}`, famID, token)
		req := httptest.NewRequest("POST", "/v1/forum/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on replay, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("expected Cache-Control: no-store")
		}

		var receipt TopicReceiptDTO
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if !receipt.Replayed {
			t.Errorf("expected replayed: true, got false")
		}
	})
}
