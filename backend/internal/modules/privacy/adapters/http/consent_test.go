package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// CS09C: owner-only training-consent HTTP. Signed writes and the
// owner read share one authenticated-caller shape; anonymous writes
// refuse before any body work. Responses carry consent fields only —
// never contributor IDs, tokens, keys or device internals — under
// no-store.

func ownerCaller() privacyapp.Caller { return privacyapp.Caller{ContributorID: "c1"} }

func unitHandler() Handler {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	return Handler{
		Authenticate: func(r *http.Request) (privacyapp.Caller, error) {
			if r.Header.Get("Authorization") == "" {
				return privacyapp.Caller{}, privacyapp.ErrUnauthorized
			}
			return ownerCaller(), nil
		},
		GrantConsent: func(_ context.Context, caller privacyapp.Caller, dto privacyapp.GrantDTO) (privacyapp.ConsentResult, error) {
			if caller.ContributorID == "" {
				return privacyapp.ConsentResult{}, privacyapp.ErrUnauthorized
			}
			return privacyapp.ConsentResult{
				State: privacydomain.StateGranted, Revision: dto.Revision,
				TermsVersion: dto.TermsVersion, DecidedAt: now,
			}, nil
		},
		RevokeConsent: func(_ context.Context, caller privacyapp.Caller, dto privacyapp.RevokeDTO) (privacyapp.ConsentResult, error) {
			if caller.ContributorID == "" {
				return privacyapp.ConsentResult{}, privacyapp.ErrUnauthorized
			}
			return privacyapp.ConsentResult{
				State: privacydomain.StateRevoked, Revision: dto.Revision,
				DecidedAt: now, DeleteOwnedTrainingAssets: true,
			}, nil
		},
		ReadConsent: func(_ context.Context, caller privacyapp.Caller) (privacyapp.ConsentView, error) {
			if caller.ContributorID == "" {
				return privacyapp.ConsentView{}, privacyapp.ErrUnauthorized
			}
			return privacyapp.ConsentView{
				Granted: true, State: privacydomain.StateGranted,
				TermsVersion: privacydomain.TrainingTermsV1, TermsDigest: "digest-v1",
				Revision: 1, DecidedAt: now,
			}, nil
		},
	}
}

func serveConsent(t *testing.T, h Handler, method, target, body string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if auth {
		req.Header.Set("Authorization", "Bearer test-proof")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return out
}

func TestGrantHappyPath(t *testing.T) {
	w := serveConsent(t, unitHandler(), http.MethodPost, "/v1/privacy/training-consent/grant",
		`{"terms_version":"training-terms-v1","terms_digest":"digest-v1","revision":1}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("grant = %d: %s", w.Code, w.Body.String())
	}
	got := decodeBody(t, w)
	if got["state"] != privacydomain.StateGranted || got["revision"] != 1.0 || got["terms_version"] != "training-terms-v1" {
		t.Errorf("receipt = %v", got)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
	for _, leaked := range []string{"c1", "tok-", "key", "device", "fingerprint"} {
		if strings.Contains(w.Body.String(), leaked) {
			t.Errorf("response leaks %q: %s", leaked, w.Body.String())
		}
	}
}

func TestRevokeHappyPath(t *testing.T) {
	w := serveConsent(t, unitHandler(), http.MethodPost, "/v1/privacy/training-consent/revoke",
		`{"revision":2}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", w.Code, w.Body.String())
	}
	got := decodeBody(t, w)
	if got["state"] != privacydomain.StateRevoked || got["delete_owned_training_assets"] != true {
		t.Errorf("receipt = %v", got)
	}
}

func TestStatusHappyPath(t *testing.T) {
	w := serveConsent(t, unitHandler(), http.MethodGet, "/v1/privacy/training-consent", "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	got := decodeBody(t, w)
	if got["granted"] != true || got["state"] != privacydomain.StateGranted {
		t.Errorf("status = %v", got)
	}
}

func TestAnonymousWritesRefuseBeforeBodyWork(t *testing.T) {
	for _, tc := range []struct{ method, target, body string }{
		{http.MethodPost, "/v1/privacy/training-consent/grant", `{"terms_version":"x","terms_digest":"y","revision":1}`},
		{http.MethodPost, "/v1/privacy/training-consent/revoke", `{"revision":1}`},
		{http.MethodGet, "/v1/privacy/training-consent", ""},
	} {
		w := serveConsent(t, unitHandler(), tc.method, tc.target, tc.body, false)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.target, w.Code)
		}
		var errBody map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		inner, _ := errBody["error"].(map[string]any)
		if inner["code"] != "privacy.auth-required" {
			t.Errorf("%s %s code = %v", tc.method, tc.target, inner["code"])
		}
	}
}

func TestMalformedBodiesRefuse(t *testing.T) {
	cases := map[string]string{
		"unknown field":    `{"terms_version":"training-terms-v1","terms_digest":"d","revision":1,"owner":"evil"}`,
		"trailing garbage": `{"revision":1} trailing`,
		"empty object":     `{}`,
		"missing terms":    `{"revision":1}`,
		"missing digest":   `{"terms_version":"training-terms-v1","revision":1}`,
		"zero revision":    `{"terms_version":"training-terms-v1","terms_digest":"d","revision":0}`,
		"negative rev":     `{"terms_version":"training-terms-v1","terms_digest":"d","revision":-1}`,
		"float revision":   `{"terms_version":"training-terms-v1","terms_digest":"d","revision":1.5}`,
		"string revision":  `{"terms_version":"training-terms-v1","terms_digest":"d","revision":"1"}`,
		"array body":       `[{"revision":1}]`,
		"revoke empty":     ``,
	}
	for name, body := range cases {
		target := "/v1/privacy/training-consent/grant"
		if name == "revoke empty" {
			target = "/v1/privacy/training-consent/revoke"
		}
		w := serveConsent(t, unitHandler(), http.MethodPost, target, body, true)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400 (%s)", name, w.Code, w.Body.String())
		}
	}
	w := serveConsent(t, unitHandler(), http.MethodPost, "/v1/privacy/training-consent/grant",
		"\xff\xfe invalid", true)
	if w.Code != http.StatusBadRequest {
		t.Errorf("non-UTF8 = %d, want 400", w.Code)
	}
	huge := `{"terms_version":"` + strings.Repeat("v", 70<<10) + `","terms_digest":"d","revision":1}`
	w = serveConsent(t, unitHandler(), http.MethodPost, "/v1/privacy/training-consent/grant", huge, true)
	if w.Code != http.StatusBadRequest {
		t.Errorf("oversize = %d, want 400", w.Code)
	}
}

func TestConsentErrorMappings(t *testing.T) {
	mk := func(grantErr, revokeErr error) Handler {
		h := unitHandler()
		h.GrantConsent = func(context.Context, privacyapp.Caller, privacyapp.GrantDTO) (privacyapp.ConsentResult, error) {
			return privacyapp.ConsentResult{}, grantErr
		}
		h.RevokeConsent = func(context.Context, privacyapp.Caller, privacyapp.RevokeDTO) (privacyapp.ConsentResult, error) {
			return privacyapp.ConsentResult{}, revokeErr
		}
		return h
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"outdated terms", privacydomain.ErrTermsOutdated, http.StatusBadRequest, "privacy.terms-outdated"},
		{"stale revision", privacydomain.ErrStaleConsent, http.StatusConflict, "privacy.stale-revision"},
		{"divergent retry", privacydomain.ErrConflict, http.StatusConflict, "privacy.conflict"},
		{"invalid", privacydomain.ErrInvalidRequest, http.StatusBadRequest, "privacy.invalid"},
	} {
		w := serveConsent(t, mk(tc.err, tc.err), http.MethodPost, "/v1/privacy/training-consent/grant",
			`{"terms_version":"training-terms-v1","terms_digest":"d","revision":1}`, true)
		if w.Code != tc.status {
			t.Errorf("%s grant = %d, want %d", tc.name, w.Code, tc.status)
		}
		var errBody map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
			t.Fatal(err)
		}
		if inner, _ := errBody["error"].(map[string]any); inner["code"] != tc.code {
			t.Errorf("%s grant code = %v, want %s", tc.name, inner["code"], tc.code)
		}
		w = serveConsent(t, mk(tc.err, tc.err), http.MethodPost, "/v1/privacy/training-consent/revoke",
			`{"revision":1}`, true)
		if w.Code != tc.status {
			t.Errorf("%s revoke = %d, want %d", tc.name, w.Code, tc.status)
		}
	}
}

func TestUnauthorizedMapsWithoutOracle(t *testing.T) {
	h := unitHandler()
	h.ReadConsent = func(context.Context, privacyapp.Caller) (privacyapp.ConsentView, error) {
		return privacyapp.ConsentView{}, errors.New("boom")
	}
	w := serveConsent(t, h, http.MethodGet, "/v1/privacy/training-consent", "", true)
	if w.Code != http.StatusBadRequest {
		t.Errorf("unexpected failure = %d, want 400", w.Code)
	}
}
