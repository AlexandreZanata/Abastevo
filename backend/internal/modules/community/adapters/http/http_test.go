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

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

func voteHandler() Handler {
	return Handler{
		Authenticate: func(*http.Request) (application.Caller, error) {
			return application.Caller{ContributorID: "c2", Fingerprint: "fp:y", KeyID: "k2", Token: "tok-c2"}, nil
		},
		Submit: func(ctx context.Context, caller application.Caller, key string, dto application.SubmitDTO, body []byte) (application.SubmitResult, bool, error) {
			return application.SubmitResult{}, false, errors.New("unused")
		},
		Status: func(ctx context.Context, caller application.Caller, id string) (application.StatusResult, error) {
			return application.StatusResult{}, errors.New("unused")
		},
		History: func(ctx context.Context, caller application.Caller, limit int, after time.Time, afterID string, hasCursor bool) ([]application.HistoryItem, string, error) {
			return nil, "", errors.New("unused")
		},
		Confirm: func(_ context.Context, _ application.Caller, observationID string, _ application.ConfirmDTO, _ []byte) (application.ConfirmResult, error) {
			return application.ConfirmResult{
				ConfirmationID: "c6c74c23-63db-4c24-a2e5-408cb23bad26",
				ReceivedAt:     time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
			}, nil
		},
		Dispute: func(_ context.Context, _ application.Caller, observationID string, _ application.DisputeDTO, _ []byte) (application.DisputeResult, error) {
			return application.DisputeResult{
				DisputeID:  "e6c74c23-63db-4c24-a2e5-408cb23bad26",
				State:      domain.DisputeOpen,
				ReceivedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
			}, nil
		},
		Secrets: []byte("test-secrets-32-bytes-long-value!"),
	}
}

func serveVotes(h Handler, method, target, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const voteTarget = "/v1/observations/d6c74c23-63db-4c24-a2e5-408cb23bad27"

func TestConfirmHappyPath(t *testing.T) {
	w := serveVotes(voteHandler(), http.MethodPost, voteTarget+"/confirmations", `{"client_submission_id":"cfm-1"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("confirm misses no-store")
	}
	for _, want := range []string{`"confirmation_id"`, `"observation_id"`, `"received_at"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body lacks %s: %s", want, w.Body.String())
		}
	}
}

func TestDisputeHappyPath(t *testing.T) {
	w := serveVotes(voteHandler(), http.MethodPost, voteTarget+"/disputes",
		`{"client_submission_id":"dsp-1","reason":"WRONG_PRODUCT","detail":" etanol label ","replacement_observation_id":null}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	for _, want := range []string{`"dispute_id"`, `"state":"OPEN"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body lacks %s: %s", want, w.Body.String())
		}
	}
}

func TestVotesRejectMalformedBodies(t *testing.T) {
	bodies := map[string]string{
		"not-json":      `{`,
		"unknown-field": `{"client_submission_id":"c","bogus":1}`,
		"empty-id":      `{"client_submission_id":""}`,
	}
	for name, body := range bodies {
		if w := serveVotes(voteHandler(), http.MethodPost, voteTarget+"/confirmations", body); w.Code != http.StatusBadRequest {
			t.Errorf("confirm %s = %d, want 400", name, w.Code)
		}
	}
	disputeBodies := map[string]string{
		"not-json":      `{`,
		"unknown-field": `{"client_submission_id":"c","reason":"OTHER","bogus":1}`,
		"empty-reason":  `{"client_submission_id":"c","reason":""}`,
	}
	for name, body := range disputeBodies {
		if w := serveVotes(voteHandler(), http.MethodPost, voteTarget+"/disputes", body); w.Code != http.StatusBadRequest {
			t.Errorf("dispute %s = %d, want 400", name, w.Code)
		}
	}
}

func TestVotesMapDomainErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"self", domain.ErrSelfConfirmation, http.StatusForbidden},
		{"ineligible", application.ErrIneligibleTarget, http.StatusConflict},
		{"missing", application.ErrTargetNotFound, http.StatusNotFound},
		{"conflict", application.ErrConflict, http.StatusConflict},
		{"already", domain.ErrAlreadyConfirmed, http.StatusConflict},
		{"reason", domain.ErrUnknownReason, http.StatusBadRequest},
		{"quota", &application.QuotaDeniedError{RetryAfter: 30 * time.Second}, http.StatusTooManyRequests},
	}
	for _, c := range cases {
		h := voteHandler()
		h.Confirm = func(context.Context, application.Caller, string, application.ConfirmDTO, []byte) (application.ConfirmResult, error) {
			return application.ConfirmResult{}, c.err
		}
		h.Dispute = func(context.Context, application.Caller, string, application.DisputeDTO, []byte) (application.DisputeResult, error) {
			return application.DisputeResult{}, c.err
		}
		if w := serveVotes(h, http.MethodPost, voteTarget+"/confirmations", `{"client_submission_id":"c"}`); w.Code != c.code {
			t.Errorf("confirm %s = %d, want %d", c.name, w.Code, c.code)
		}
		body := `{"client_submission_id":"c","reason":"OTHER"}`
		if w := serveVotes(h, http.MethodPost, voteTarget+"/disputes", body); w.Code != c.code {
			t.Errorf("dispute %s = %d, want %d", c.name, w.Code, c.code)
		}
		if c.name == "quota" {
			w := serveVotes(h, http.MethodPost, voteTarget+"/confirmations", `{"client_submission_id":"c"}`)
			if w.Header().Get("Retry-After") != "30" {
				t.Errorf("quota misses Retry-After")
			}
		}
	}
}

func TestVotesRequireAuth(t *testing.T) {
	h := voteHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) {
		return application.Caller{}, errors.New("no proof")
	}
	if w := serveVotes(h, http.MethodPost, voteTarget+"/confirmations", `{"client_submission_id":"c"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous confirm = %d, want 401", w.Code)
	}
}
