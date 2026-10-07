package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func testHandler() Handler {
	return Handler{
		Authenticate: func(*http.Request) (application.Caller, error) {
			return application.Caller{ContributorID: "c1", Token: "tok-c1"}, nil
		},
		Reserve: func(_ context.Context, _ application.Caller, in application.Intent, _ []byte) (application.Result, error) {
			return application.Result{
				SessionID:       "e0000000-0000-4000-8000-000000000001",
				ExpiresAt:       time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC),
				MaxBytes:        domain.MaxUploadBytes,
				URL:             "https://storage.example.invalid/q/k?sig=test",
				URLExpiresAt:    time.Date(2026, 9, 30, 12, 5, 0, 0, time.UTC),
				RequiredHeaders: map[string]string{"Content-Type": "image/jpeg"},
			}, nil
		},
		Complete: func(_ context.Context, _ application.Caller, id string) (application.StatusView, error) {
			return application.StatusView{
				SessionID: id, State: domain.StateVerifying,
				ExpiresAt: time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
			}, nil
		},
		Status: func(_ context.Context, _ application.Caller, id string) (application.StatusView, error) {
			return application.StatusView{
				SessionID: id, State: domain.StateReady,
				ExpiresAt:  time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC),
				UpdatedAt:  time.Date(2026, 9, 30, 12, 1, 0, 0, time.UTC),
				EvidenceID: "e0000000-0000-4000-8000-000000000002",
			}, nil
		},
	}
}

func serve(h Handler, method, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestReserveHappyPath(t *testing.T) {
	w := serve(testHandler(), http.MethodPost, "/v1/uploads",
		`{"client_submission_id":"upl-1","content_type":"image/jpeg","size_bytes":524288,"sha256":"`+strings.Repeat("a", 64)+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("reserve misses no-store")
	}
	for _, want := range []string{`"method":"PUT"`, `"upload_id"`, `"url"`, `"required_headers"`, `"expires_at"`, `"max_bytes"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body lacks %s: %s", want, w.Body.String())
		}
	}
}

func TestReserveRejectsMalformedBodies(t *testing.T) {
	sha := strings.Repeat("a", 64)
	bodies := map[string]string{
		"not-json":      `{`,
		"unknown-field": `{"client_submission_id":"u","content_type":"image/jpeg","size_bytes":10,"sha256":"` + sha + `","bogus":1}`,
		"float-amount":  `{"client_submission_id":"u","content_type":"image/jpeg","size_bytes":5.5,"sha256":"` + sha + `"}`,
		"exp-amount":    `{"client_submission_id":"u","content_type":"image/jpeg","size_bytes":5e3,"sha256":"` + sha + `"}`,
		"missing-hash":  `{"client_submission_id":"u","content_type":"image/jpeg","size_bytes":10}`,
	}
	for name, body := range bodies {
		if w := serve(testHandler(), http.MethodPost, "/v1/uploads", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, w.Code)
		}
	}
}

func TestReserveMapsDomainErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"conflict", application.ErrConflict, http.StatusConflict},
		{"quota", &application.QuotaDeniedError{RetryAfter: 30 * time.Second}, http.StatusTooManyRequests},
		{"storage", application.ErrStorageUnavailable, http.StatusServiceUnavailable},
		{"invalid", domain.ErrUnsupportedMedia, http.StatusBadRequest},
		{"auth", application.ErrUnauthorized, http.StatusUnauthorized},
	}
	for _, c := range cases {
		h := testHandler()
		h.Reserve = func(context.Context, application.Caller, application.Intent, []byte) (application.Result, error) {
			return application.Result{}, c.err
		}
		sha := strings.Repeat("a", 64)
		w := serve(h, http.MethodPost, "/v1/uploads",
			`{"client_submission_id":"u","content_type":"image/jpeg","size_bytes":10,"sha256":"`+sha+`"}`)
		if w.Code != c.code {
			t.Errorf("%s = %d, want %d", c.name, w.Code, c.code)
		}
		if c.name == "quota" && w.Header().Get("Retry-After") != "30" {
			t.Errorf("quota misses Retry-After: %q", w.Header().Get("Retry-After"))
		}
	}
}

func TestCompleteAndStatusShapes(t *testing.T) {
	w := serve(testHandler(), http.MethodPost, "/v1/uploads/e0000000-0000-4000-8000-000000000001/complete", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("complete = %d, want 202", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"state":"VERIFYING"`) {
		t.Errorf("complete body = %s", w.Body.String())
	}
	w = serve(testHandler(), http.MethodGet, "/v1/uploads/e0000000-0000-4000-8000-000000000001", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"state":"READY"`) || !strings.Contains(body, `"evidence_id":"e0000000-0000-4000-8000-000000000002"`) {
		t.Errorf("status body = %s", body)
	}
	// Owner status never leaks keys or URLs: only states and the
	// evidence reference travel to the owner.
	if strings.Contains(body, "q/") || strings.Contains(body, "http") {
		t.Errorf("status leaks storage internals: %s", body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("status misses no-store")
	}
}

func TestCompleteTerminalReportsOK(t *testing.T) {
	h := testHandler()
	h.Complete = func(_ context.Context, _ application.Caller, id string) (application.StatusView, error) {
		return application.StatusView{SessionID: id, State: domain.StateReady, EvidenceID: "e1"}, nil
	}
	w := serve(h, http.MethodPost, "/v1/uploads/e0000000-0000-4000-8000-000000000001/complete", "")
	if w.Code != http.StatusOK {
		t.Errorf("terminal complete = %d, want 200", w.Code)
	}
}

func TestUnknownAndForeignShare404(t *testing.T) {
	h := testHandler()
	h.Status = func(context.Context, application.Caller, string) (application.StatusView, error) {
		return application.StatusView{}, application.ErrSessionNotFound
	}
	h.Complete = func(context.Context, application.Caller, string) (application.StatusView, error) {
		return application.StatusView{}, application.ErrSessionNotFound
	}
	if w := serve(h, http.MethodGet, "/v1/uploads/e0000000-0000-4000-8000-000000000099", ""); w.Code != http.StatusNotFound {
		t.Errorf("missing status = %d, want 404", w.Code)
	}
	if w := serve(h, http.MethodPost, "/v1/uploads/e0000000-0000-4000-8000-000000000099/complete", ""); w.Code != http.StatusNotFound {
		t.Errorf("missing complete = %d, want 404", w.Code)
	}
}

func TestAuthRequired(t *testing.T) {
	h := testHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) {
		return application.Caller{}, errors.New("no proof")
	}
	if w := serve(h, http.MethodPost, "/v1/uploads", `{}`); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous reserve = %d, want 401", w.Code)
	}
}

func TestReserveCaptureEnvelopeIsStrictAndPreservesOriginalTime(t *testing.T) {
	base := `{"client_submission_id":"u","content_type":"image/jpeg","size_bytes":10,"sha256":"` + strings.Repeat("a", 64) + `"`
	fields := `,"photo_capture_id":"c0000000-0000-4000-8000-000000000001","station_id":"d0000000-0000-4000-8000-000000000001","captured_at":"2026-10-07T12:00:00.123Z"`
	got, err := parseReserveBody([]byte(base + fields + `}`))
	if err != nil || got.PhotoCaptureID == "" || got.StationID == "" || got.CapturedAt.Nanosecond() != 123000000 {
		t.Fatalf("envelope=%+v %v", got, err)
	}
	for _, suffix := range []string{`,"photo_capture_id":"c0000000-0000-4000-8000-000000000001"`, `,"station_id":"d0000000-0000-4000-8000-000000000001"`, `,"captured_at":"2026-10-07T12:00:00Z"`, strings.Replace(fields, "c0000000-0000-4000-8000-000000000001", "broken", 1), strings.Replace(fields, "2026-10-07T12:00:00.123Z", "yesterday", 1)} {
		if _, err := parseReserveBody([]byte(base + suffix + `}`)); err == nil {
			t.Fatalf("malformed envelope accepted: %s", suffix)
		}
	}
	h := testHandler()
	h.Reserve = func(context.Context, application.Caller, application.Intent, []byte) (application.Result, error) {
		return application.Result{}, application.ErrPhotoCaptureIneligible
	}
	if got := serve(h, "POST", "/v1/uploads", base+fields+`}`); got.Code != 403 {
		t.Fatalf("capture refusal=%d", got.Code)
	}
}
