package http

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const providerAudience = "anpfuel-backend"

type providerFixture struct {
	handler Handler
	google  oidc.StubIssuer
	apple   oidc.StubIssuer
}

func newProviderHandler(t *testing.T) *providerFixture {
	t.Helper()
	google, err := oidc.NewStubIssuer("https://accounts.google.com", "RS256")
	if err != nil {
		t.Fatal(err)
	}
	apple, err := oidc.NewStubIssuer("https://appleid.apple.com", "ES256")
	if err != nil {
		t.Fatal(err)
	}
	nonces := oidc.NewMemNonces()
	verifier := &oidc.Verifier{
		Clock:    func() time.Time { return time.Unix(1_800_000_000, 0) },
		Issuers:  map[string]string{"google": "https://accounts.google.com", "apple": "https://appleid.apple.com"},
		Audience: providerAudience,
		Keys: oidc.NewMemKeys(map[string]oidc.StubIssuer{
			"https://accounts.google.com": google,
			"https://appleid.apple.com":   apple,
		}),
		Nonces:   nonces,
		Skew:     120 * time.Second,
		CacheTTL: time.Hour,
	}
	var mu sync.Mutex
	ci, tn, an, gi := 0, 0, 0, 0
	codes := []string{"482916", "111111", "222222", "333333", "444444", "555555", "666666"}
	svc := &application.Service{
		Clock:    &testClock{now: 1_800_000_000},
		Hasher:   domain.SHA256Hasher{},
		Mail:     &testMail{},
		Store:    application.NewMemStore(),
		Verifier: verifier,
		CodeGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			c := codes[ci%len(codes)]
			ci++
			return c, nil
		},
		TokenGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			tn++
			return fmt.Sprintf("tok-%d", tn), nil
		},
		AliasGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			an++
			return fmt.Sprintf("alias-%d", an), nil
		},
		IDGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			gi++
			return fmt.Sprintf("aaaaaaaa-1111-4111-8111-%012d", gi), nil
		},
	}
	return &providerFixture{
		handler: Handler{Service: svc, Audience: providerAudience},
		google:  google,
		apple:   apple,
	}
}

func (f *providerFixture) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	f.handler.RegisterRoutes(r)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func (f *providerFixture) googleToken(t *testing.T, sub, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.google.Token(sub, email, providerAudience, nonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (f *providerFixture) appleToken(t *testing.T, sub, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.apple.Token(sub, email, providerAudience, nonce, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

type sessionOut struct {
	FamilyID     string `json:"family_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (f *providerFixture) signup(t *testing.T, email, code string) sessionOut {
	t.Helper()
	if rec := f.call(t, "POST", "/v1/accounts/email/codes", `{"email":"`+email+`"}`); rec.Code != 202 {
		t.Fatalf("request code: %d (%s)", rec.Code, rec.Body.String())
	}
	rec := f.call(t, "POST", "/v1/accounts/email/consume", `{"email":"`+email+`","code":"`+code+`"}`)
	if rec.Code != 200 {
		t.Fatalf("consume: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Session sessionOut `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return out.Session
}

func linkBody(sess sessionOut, provider, token, nonce string) string {
	raw, _ := json.Marshal(map[string]string{
		"family_id": sess.FamilyID, "access_token": sess.AccessToken,
		"provider": provider, "id_token": token, "nonce": nonce,
	})
	return string(raw)
}

func TestProviderLinkUnlinkListFlow(t *testing.T) {
	f := newProviderHandler(t)
	sess := f.signup(t, "link-a@example.invalid", "482916")

	rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sess, "google", f.googleToken(t, "user-123", "user@example.com", "n-http-1"), "n-http-1"))
	if rec.Code != 200 {
		t.Fatalf("link google: %d (%s)", rec.Code, rec.Body.String())
	}
	var linked struct {
		ProviderLink struct {
			Provider string `json:"provider"`
			Subject  string `json:"subject"`
		} `json:"provider_link"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &linked); err != nil {
		t.Fatal(err)
	}
	if linked.ProviderLink.Provider != "google" || linked.ProviderLink.Subject != "user-123" {
		t.Errorf("link response wrong: %+v", linked)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("link response must be no-store")
	}

	rec = f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sess, "apple", f.appleToken(t, "opaque-apple-001", "relay@privaterelay.appleid.com", "n-http-2"), "n-http-2"))
	if rec.Code != 200 {
		t.Fatalf("link apple: %d (%s)", rec.Code, rec.Body.String())
	}

	listBody, _ := json.Marshal(map[string]string{"family_id": sess.FamilyID, "access_token": sess.AccessToken})
	rec = f.call(t, "POST", "/v1/accounts/providers/list", string(listBody))
	if rec.Code != 200 {
		t.Fatalf("list: %d (%s)", rec.Code, rec.Body.String())
	}
	var listed struct {
		Providers []struct {
			Provider string `json:"provider"`
			Subject  string `json:"subject"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Providers) != 2 {
		t.Errorf("want 2 providers, got %+v", listed)
	}

	unlinkBody, _ := json.Marshal(map[string]string{"family_id": sess.FamilyID, "access_token": sess.AccessToken, "provider": "google"})
	if rec := f.call(t, "POST", "/v1/accounts/providers/unlink", string(unlinkBody)); rec.Code != 200 {
		t.Fatalf("unlink: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = f.call(t, "POST", "/v1/accounts/providers/list", string(listBody))
	var relisted struct {
		Providers []any `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &relisted); err != nil {
		t.Fatal(err)
	}
	if len(relisted.Providers) != 1 {
		t.Errorf("want 1 provider after unlink, got %d", len(relisted.Providers))
	}
	if rec := f.call(t, "POST", "/v1/accounts/providers/unlink", string(unlinkBody)); rec.Code != 404 ||
		!strings.Contains(rec.Body.String(), "account.link-provider-not-linked") {
		t.Errorf("second unlink must be 404 not-linked, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProviderLinkCrossAccountViaHTTP(t *testing.T) {
	f := newProviderHandler(t)
	sessA := f.signup(t, "cross-a@example.invalid", "482916")
	sessB := f.signup(t, "cross-b@example.invalid", "111111")

	if rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sessA, "google", f.googleToken(t, "shared-sub-1", "", "n-http-cross-1"), "n-http-cross-1")); rec.Code != 200 {
		t.Fatalf("link A: %d (%s)", rec.Code, rec.Body.String())
	}
	rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sessB, "google", f.googleToken(t, "shared-sub-1", "", "n-http-cross-2"), "n-http-cross-2"))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "account.link-cross-account-refused") {
		t.Errorf("cross-account must be 403 cross-account-refused, got %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "shared-sub-1") {
		t.Error("refusal must not echo the provider subject")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("refusal must be no-store")
	}
}

func TestProviderLinkReplayAndMismatchViaHTTP(t *testing.T) {
	f := newProviderHandler(t)
	sessA := f.signup(t, "replay-a@example.invalid", "482916")
	sessB := f.signup(t, "replay-b@example.invalid", "111111")

	tok := f.googleToken(t, "replay-sub-1", "", "n-http-replay-1")
	if rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sessA, "google", tok, "n-http-replay-1")); rec.Code != 200 {
		t.Fatalf("first link: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sessB, "google", tok, "n-http-replay-1")); rec.Code != 401 ||
		!strings.Contains(rec.Body.String(), "account.oidc-nonce-reused") {
		t.Errorf("replay must be 401 nonce-reused, got %d (%s)", rec.Code, rec.Body.String())
	}
	tok2 := f.googleToken(t, "replay-sub-1", "", "n-http-other")
	if rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sessB, "google", tok2, "n-http-wrong")); rec.Code != 401 ||
		!strings.Contains(rec.Body.String(), "account.oidc-nonce-mismatch") {
		t.Errorf("mismatch must be 401 nonce-mismatch, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProviderTransportRejectsMalformed(t *testing.T) {
	f := newProviderHandler(t)
	sess := f.signup(t, "bad-a@example.invalid", "482916")
	validTok := f.googleToken(t, "bad-sub-1", "", "n-http-bad-1")

	cases := []struct {
		name, path, body string
	}{
		{"bad provider", "/v1/accounts/providers/link", linkBody(sess, "github", validTok, "n-http-bad-1")},
		{"empty token", "/v1/accounts/providers/link", linkBody(sess, "google", "", "n-http-bad-1")},
		{"empty nonce", "/v1/accounts/providers/link", linkBody(sess, "google", validTok, "")},
		{"empty session", "/v1/accounts/providers/link", linkBody(sessionOut{}, "google", validTok, "n-http-bad-1")},
		{"bad unlink provider", "/v1/accounts/providers/unlink", `{"family_id":"x","access_token":"y","provider":"github"}`},
		{"empty list session", "/v1/accounts/providers/list", `{"family_id":"","access_token":""}`},
		{"not json", "/v1/accounts/providers/link", `not json`},
	}
	for _, tc := range cases {
		if rec := f.call(t, "POST", tc.path, tc.body); rec.Code != 400 {
			t.Errorf("%s must be 400, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestProviderTransportRequiresLiveSession(t *testing.T) {
	f := newProviderHandler(t)
	_ = f.signup(t, "sess-a@example.invalid", "482916")
	tok := f.googleToken(t, "sess-sub-1", "", "n-http-sess-1")
	rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sessionOut{FamilyID: "missing", AccessToken: "bad"}, "google", tok, "n-http-sess-1"))
	if rec.Code != 401 {
		t.Errorf("forged session must be 401, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProviderBodiesNeverLeakSecrets(t *testing.T) {
	f := newProviderHandler(t)
	sess := f.signup(t, "leak-a@example.invalid", "482916")
	tok := f.googleToken(t, "leak-sub-1", "", "n-http-leak-1")
	rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sess, "google", tok, "n-http-leak-1"))
	body := rec.Body.String()
	for _, secret := range []string{tok, sess.AccessToken, "leak-a@example.invalid", "482916"} {
		if secret != "" && strings.Contains(body, secret) {
			t.Errorf("link response must not echo secret %q: %s", secret[:8], body)
		}
	}
	bad := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sess, "google", "bad.token.parts", "n-http-leak-2"))
	for _, secret := range []string{"bad.token.parts", sess.AccessToken} {
		if strings.Contains(bad.Body.String(), secret) {
			t.Errorf("refusal must not echo token: %s", bad.Body.String())
		}
	}
}

func TestFailMapsLinkErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
		code string
	}{
		{domain.ErrOIDCUnknownIssuer, 401, "account.oidc-unknown-issuer"},
		{domain.ErrOIDCWrongAudience, 401, "account.oidc-wrong-audience"},
		{domain.ErrOIDCExpired, 401, "account.oidc-expired"},
		{domain.ErrOIDCNonceReused, 401, "account.oidc-nonce-reused"},
		{domain.ErrOIDCNonceMismatch, 401, "account.oidc-nonce-mismatch"},
		{domain.ErrOIDCUnavailable, 503, "account.oidc-unavailable"},
		{domain.ErrLinkCrossAccount, 403, "account.link-cross-account-refused"},
		{domain.ErrLinkEmailOnly, 403, "account.link-email-only-refused"},
		{domain.ErrLastLoginMethod, 409, "account.link-last-method-refused"},
		{domain.ErrProviderNotLinked, 404, "account.link-provider-not-linked"},
		{domain.ErrAccountNotFound, 401, "account.account-unknown"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", "/v1/accounts/providers/link", nil)
		rec := httptest.NewRecorder()
		fail(rec, req, tc.err)
		if rec.Code != tc.want {
			t.Errorf("fail(%v) status = %d, want %d", tc.err, rec.Code, tc.want)
		}
		if !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("fail(%v) body must carry %q, got %s", tc.err, tc.code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("fail(%v) must be no-store", tc.err)
		}
	}
}
