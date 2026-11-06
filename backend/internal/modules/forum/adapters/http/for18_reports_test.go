package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
)

func testUUID() string {
	id, _ := platformjobs.NewUUIDv4()
	return id
}

type mockReportStore struct {
	reportFn func(ctx context.Context, cmd application.ReportCommand) (*domain.ReportReceipt, error)
}

func (m *mockReportStore) Report(ctx context.Context, cmd application.ReportCommand) (*domain.ReportReceipt, error) {
	if m.reportFn != nil {
		return m.reportFn(ctx, cmd)
	}
	return nil, nil
}

func setupReportTestRouter(handler *DiscussionHandler) *chi.Mux {
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return r
}

func TestFOR18_RollbackAndFeatureGates(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.WritesEnabled = false // Rollback mode: writes disabled
	cfg.ReadsEnabled = true

	store := &mockReportStore{}
	handler := &DiscussionHandler{
		Reports: store,
		Config:  &cfg,
	}
	r := setupReportTestRouter(handler)

	postID := testUUID()
	commentID := testUUID()

	body, _ := json.Marshal(ReportRequest{
		SessionEnvelope: SessionEnvelope{
			FamilyID:    testUUID(),
			AccessToken: "tok",
		},
		Reason: "SPAM",
	})

	// 1. Post report endpoint with writes disabled
	req := httptest.NewRequest(http.MethodPost, "/v1/forum/posts/"+postID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for writes disabled, got %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %s", w.Header().Get("Cache-Control"))
	}

	// 2. Comment report endpoint with writes disabled
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/comments/"+commentID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for writes disabled, got %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %s", w.Header().Get("Cache-Control"))
	}
}

func TestFOR18_ReasonAndBoundsValidation(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.WritesEnabled = true
	cfg.ReadsEnabled = true

	store := &mockReportStore{}
	accountID := testUUID()
	contribID := testUUID()
	keyID := testUUID()

	handler := &DiscussionHandler{
		Reports: store,
		Config:  &cfg,
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			return accountID, nil
		},
		SignedOwner: func(r *http.Request, owner string) (string, string, error) {
			return contribID, keyID, nil
		},
	}
	r := setupReportTestRouter(handler)
	postID := testUUID()

	// 1. Invalid reason code
	body, _ := json.Marshal(ReportRequest{
		SessionEnvelope: SessionEnvelope{
			FamilyID:    testUUID(),
			AccessToken: "tok",
		},
		Reason: "INVENTED_REASON",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/forum/posts/"+postID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid reason, got %d", w.Code)
	}

	// 2. Empty reason
	body, _ = json.Marshal(ReportRequest{
		SessionEnvelope: SessionEnvelope{
			FamilyID:    testUUID(),
			AccessToken: "tok",
		},
		Reason: "   ",
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/posts/"+postID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty reason, got %d", w.Code)
	}

	// 3. Detail too long (>500 chars)
	body, _ = json.Marshal(ReportRequest{
		SessionEnvelope: SessionEnvelope{
			FamilyID:    testUUID(),
			AccessToken: "tok",
		},
		Reason: "SPAM",
		Detail: strings.Repeat("A", 501),
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/posts/"+postID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for detail too long, got %d", w.Code)
	}

	// 4. Invalid target ID in URL
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/posts/invalid-uuid/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-UUID target, got %d", w.Code)
	}
}

func TestFOR18_ErrorStatusMappings(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.WritesEnabled = true
	cfg.ReadsEnabled = true

	var returnErr error
	store := &mockReportStore{
		reportFn: func(ctx context.Context, cmd application.ReportCommand) (*domain.ReportReceipt, error) {
			return nil, returnErr
		},
	}

	accountID := testUUID()
	contribID := testUUID()
	keyID := testUUID()

	handler := &DiscussionHandler{
		Reports: store,
		Config:  &cfg,
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			return accountID, nil
		},
		SignedOwner: func(r *http.Request, owner string) (string, string, error) {
			return contribID, keyID, nil
		},
	}
	r := setupReportTestRouter(handler)
	commentID := testUUID()

	body, _ := json.Marshal(ReportRequest{
		SessionEnvelope: SessionEnvelope{
			FamilyID:    testUUID(),
			AccessToken: "tok",
		},
		Reason: "SPAM",
	})

	// 1. Quota exceeded -> 429 with Retry-After: 86400
	returnErr = domain.ErrReportQuotaExceeded
	req := httptest.NewRequest(http.MethodPost, "/v1/forum/comments/"+commentID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for quota, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "86400" {
		t.Fatalf("expected Retry-After: 86400, got %s", w.Header().Get("Retry-After"))
	}

	// 2. Self report forbidden -> 400
	returnErr = domain.ErrSelfReportForbidden
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/comments/"+commentID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for self-report, got %d", w.Code)
	}

	// 3. Target not found -> 404
	returnErr = domain.ErrCommentNotFound
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/comments/"+commentID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for comment not found, got %d", w.Code)
	}

	// 4. Post not found -> 404
	returnErr = domain.ErrPostNotFound
	postID := testUUID()
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/posts/"+postID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for post not found, got %d", w.Code)
	}

	// 5. Database failure -> 503
	returnErr = errors.New("connection failed")
	req = httptest.NewRequest(http.MethodPost, "/v1/forum/comments/"+commentID+"/report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for internal db error, got %d", w.Code)
	}
}
