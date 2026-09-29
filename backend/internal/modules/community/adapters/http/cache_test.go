package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

func privateHandler() Handler {
	caller := application.Caller{ContributorID: "c1", Fingerprint: "fp:x", KeyID: "k1", Token: "tok-c1"}
	obs := domain.Observation{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", ContributorRef: "tok-c1",
		StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", Product: "GASOLINE_REGULAR",
		Unit: "L", AmountMilli: 5890, ConditionKind: "STANDARD",
		ReceivedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), PolicyVersion: domain.PolicyV1,
	}
	return Handler{
		Authenticate: func(*http.Request) (application.Caller, error) { return caller, nil },
		Submit: func(context.Context, application.Caller, string, application.SubmitDTO, []byte) (application.SubmitResult, bool, error) {
			return application.SubmitResult{
				ObservationID: obs.ID, State: domain.StateReceived, ReceivedAt: obs.ReceivedAt,
			}, false, nil
		},
		Status: func(context.Context, application.Caller, string) (application.StatusResult, error) {
			return application.StatusResult{Observation: obs, State: domain.StateReceived}, nil
		},
		History: func(context.Context, application.Caller, int, time.Time, string, bool) ([]application.HistoryItem, string, error) {
			return []application.HistoryItem{{Observation: obs, State: domain.StateReceived}}, "", nil
		},
		Confirm: voteHandler().Confirm,
		Dispute: voteHandler().Dispute,
		Secrets: []byte("test-secrets-32-bytes-long-value!"),
	}
}

func servePrivate(h Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", "key-1")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const privateSubmitBody = `{"client_submission_id":"sub-1","station_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26","fuel_product":"GASOLINE_REGULAR","price":{"amount_milli_brl":5890,"currency":"BRL","unit":"L"},"condition":{"kind":"STANDARD"}}`

// TestPrivateWritesAreNeverSharedCached proves every owner write and
// read carries no-store: signed commands, owner status, owner history,
// votes and reports must never sit in shared caches.
func TestPrivateWritesAreNeverSharedCached(t *testing.T) {
	h := privateHandler()
	obsID := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	cases := []struct {
		name   string
		method string
		target string
		body   string
		want   int
	}{
		{"submit", http.MethodPost, "/v1/observations", privateSubmitBody, http.StatusCreated},
		{"status", http.MethodGet, "/v1/observations/" + obsID, "", http.StatusOK},
		{"history", http.MethodGet, "/v1/contributors/me/observations?limit=5", "", http.StatusOK},
		{"confirm", http.MethodPost, "/v1/observations/" + obsID + "/confirmations", `{"client_submission_id":"cfm-1"}`, http.StatusCreated},
		{"dispute", http.MethodPost, "/v1/observations/" + obsID + "/disputes", `{"client_submission_id":"dsp-1","reason":"OTHER"}`, http.StatusCreated},
	}
	for _, tc := range cases {
		w := servePrivate(h, tc.method, tc.target, tc.body, nil)
		if w.Code != tc.want {
			t.Fatalf("%s = %d: %s", tc.name, w.Code, w.Body.String())
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s cache = %q, want no-store", tc.name, cc)
		}
	}
}
