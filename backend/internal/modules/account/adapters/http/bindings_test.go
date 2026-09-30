package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

type stubBinder struct {
	mu     sync.Mutex
	known  map[string]bool
	denied bool
}

func (s *stubBinder) VerifyKeyProof(_ context.Context, _, contributorID, proof string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.denied || !s.known[contributorID] || proof != "proof-"+contributorID {
		return "", domain.ErrKeyProofDenied
	}
	return "fp:" + contributorID, nil
}

func bindCall(t *testing.T, f *providerFixture, sess sessionOut, contributor, proof string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{
		"family_id": sess.FamilyID, "access_token": sess.AccessToken,
		"contributor_id": contributor, "proof": proof,
	})
	return string(raw)
}

func TestBindingFlowViaHTTP(t *testing.T) {
	f := newProviderHandler(t)
	svcOf(t, f).KeyProver = &stubBinder{known: map[string]bool{"contrib-1": true}}
	sess := f.signup(t, "bind-a@example.invalid", "482916")

	rec := f.call(t, "POST", "/v1/accounts/bindings/bind", bindCall(t, f, sess, "contrib-1", "proof-contrib-1"))
	if rec.Code != 200 {
		t.Fatalf("bind: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Binding struct {
			ContributorID  string `json:"contributor_id"`
			KeyFingerprint string `json:"key_fingerprint"`
		} `json:"binding"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Binding.ContributorID != "contrib-1" || out.Binding.KeyFingerprint != "fp:contrib-1" {
		t.Errorf("bind response wrong: %+v", out)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("bind response must be no-store")
	}

	listBody, _ := json.Marshal(map[string]string{"family_id": sess.FamilyID, "access_token": sess.AccessToken})
	if rec := f.call(t, "POST", "/v1/accounts/bindings/list", string(listBody)); rec.Code != 200 ||
		!strings.Contains(rec.Body.String(), "contrib-1") {
		t.Fatalf("list: %d (%s)", rec.Code, rec.Body.String())
	}
	unbindBody, _ := json.Marshal(map[string]string{"family_id": sess.FamilyID, "access_token": sess.AccessToken, "contributor_id": "contrib-1"})
	if rec := f.call(t, "POST", "/v1/accounts/bindings/unbind", string(unbindBody)); rec.Code != 200 {
		t.Fatalf("unbind: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := f.call(t, "POST", "/v1/accounts/bindings/unbind", string(unbindBody)); rec.Code != 404 ||
		!strings.Contains(rec.Body.String(), "account.binding-not-found") {
		t.Errorf("second unbind must be 404 not-found, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestBindingStolenViaHTTP(t *testing.T) {
	f := newProviderHandler(t)
	svcOf(t, f).KeyProver = &stubBinder{known: map[string]bool{"shared-1": true}}
	sessA := f.signup(t, "bind-a@example.invalid", "482916")
	sessB := f.signup(t, "bind-b@example.invalid", "111111")

	if rec := f.call(t, "POST", "/v1/accounts/bindings/bind", bindCall(t, f, sessA, "shared-1", "proof-shared-1")); rec.Code != 200 {
		t.Fatalf("bind A: %d (%s)", rec.Code, rec.Body.String())
	}
	rec := f.call(t, "POST", "/v1/accounts/bindings/bind", bindCall(t, f, sessB, "shared-1", "proof-shared-1"))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "account.binding-cross-account-refused") {
		t.Errorf("stolen bind must be 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "shared-1-proof") {
		t.Error("refusal must not echo proof material")
	}
}

func TestBindingProofDeniedViaHTTP(t *testing.T) {
	f := newProviderHandler(t)
	svcOf(t, f).KeyProver = &stubBinder{known: map[string]bool{}, denied: true}
	sess := f.signup(t, "bind-c@example.invalid", "482916")

	rec := f.call(t, "POST", "/v1/accounts/bindings/bind", bindCall(t, f, sess, "contrib-9", "proof-contrib-9"))
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "account.key-proof-denied") {
		t.Errorf("denied proof must be 401 key-proof-denied, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestBindingRejectsMalformed(t *testing.T) {
	f := newProviderHandler(t)
	svcOf(t, f).KeyProver = &stubBinder{known: map[string]bool{"contrib-1": true}}
	sess := f.signup(t, "bind-d@example.invalid", "482916")

	cases := []struct{ name, path, body string }{
		{"empty contributor", "/v1/accounts/bindings/bind", bindCall(t, f, sess, "", "proof-x")},
		{"empty proof", "/v1/accounts/bindings/bind", bindCall(t, f, sess, "contrib-1", "")},
		{"empty session", "/v1/accounts/bindings/bind", bindCall(t, f, sessionOut{}, "contrib-1", "proof-contrib-1")},
		{"empty unbind", "/v1/accounts/bindings/unbind", `{"family_id":"x","access_token":"y","contributor_id":""}`},
		{"empty list", "/v1/accounts/bindings/list", `{"family_id":"","access_token":""}`},
		{"not json", "/v1/accounts/bindings/bind", `not json`},
	}
	for _, tc := range cases {
		if rec := f.call(t, "POST", tc.path, tc.body); rec.Code != 400 {
			t.Errorf("%s must be 400, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestFailMapsBindingErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
		code string
	}{
		{domain.ErrBindingCrossAccount, 403, "account.binding-cross-account-refused"},
		{domain.ErrBindingNotFound, 404, "account.binding-not-found"},
		{domain.ErrBindingInvalid, 400, "account.binding-invalid"},
		{domain.ErrKeyUnavailable, 503, "account.key-unavailable"},
		{domain.ErrKeyProofDenied, 401, "account.key-proof-denied"},
		{errors.New("account: key proof denied by stub"), 401, "account.code-unknown"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", "/v1/accounts/bindings/bind", nil)
		rec := httptest.NewRecorder()
		fail(rec, req, tc.err)
		if rec.Code != tc.want {
			t.Errorf("fail(%v) status = %d, want %d", tc.err, rec.Code, tc.want)
		}
		if !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("fail(%v) body must carry %q, got %s", tc.err, tc.code, rec.Body.String())
		}
	}
}
