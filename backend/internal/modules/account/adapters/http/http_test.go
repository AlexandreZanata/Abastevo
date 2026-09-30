package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

type testClock struct {
	mu  sync.Mutex
	now int64
}

func (c *testClock) NowUnix() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

type testMail struct {
	mu   sync.Mutex
	sent []string
}

func (m *testMail) SendCode(_ context.Context, address, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, address+"\x00"+code)
	return nil
}

func testHandler() (Handler, *testMail) {
	mail := &testMail{}
	codes := []string{"482916", "111111"}
	var mu sync.Mutex
	ci, tn := 0, 0
	h := Handler{Service: &application.Service{
		Clock:   &testClock{now: 1_700_000_000},
		Hasher:  domain.SHA256Hasher{},
		Mail:    mail,
		Store:   application.NewMemStore(),
		CodeGen: func() (string, error) { mu.Lock(); defer mu.Unlock(); c := codes[ci%len(codes)]; ci++; return c, nil },
		TokenGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			tn++
			if tn > 64 {
				tn = 1
			}
			return strings.Repeat("t", tn) + strings.Repeat("k", 64-tn), nil
		},
		AliasGen: func() (string, error) { return "alias-test", nil },
		IDGen:    func() (string, error) { return "aaaaaaaa-1111-4111-8111-111111111111", nil },
	}}
	return h, mail
}

func call(t *testing.T, h Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRequestCodeAlways202(t *testing.T) {
	h, _ := testHandler()
	for _, body := range []string{
		`{"email":"case-01@example.invalid"}`,
		`{"email":"nobody@example.invalid"}`,
	} {
		rec := call(t, h, "POST", "/v1/accounts/email/codes", body)
		if rec.Code != 202 {
			t.Errorf("request must answer 202, got %d (%s)", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("account response must be no-store")
		}
	}
}

func TestRequestCodeRejectsMalformed(t *testing.T) {
	h, _ := testHandler()
	for _, body := range []string{
		`{"email":""}`,
		`{"email":"not-an-address"}`,
		`{"email":123}`,
		`not json`,
	} {
		rec := call(t, h, "POST", "/v1/accounts/email/codes", body)
		if rec.Code != 400 {
			t.Errorf("malformed request must be 400, got %d for %q", rec.Code, body)
		}
	}
}

func TestConsumeLoginFlow(t *testing.T) {
	h, _ := testHandler()
	if rec := call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"case-02@example.invalid"}`); rec.Code != 202 {
		t.Fatalf("request: %d", rec.Code)
	}
	rec := call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"case-02@example.invalid","code":"482916"}`)
	if rec.Code != 200 {
		t.Fatalf("consume: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Account struct {
			AccountID string `json:"account_id"`
			Alias     string `json:"alias"`
			Status    string `json:"status"`
		} `json:"account"`
		Session struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			FamilyID     string `json:"family_id"`
		} `json:"session"`
		Created bool `json:"created"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Created || out.Account.Status != "active" || out.Session.AccessToken == "" {
		t.Errorf("login must create an active sessioned account: %+v", out)
	}

	bad := call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"case-02@example.invalid","code":"000000"}`)
	if bad.Code != 401 {
		t.Errorf("wrong code must be 401, got %d", bad.Code)
	}
	if !strings.Contains(bad.Body.String(), "account.code-unknown") {
		t.Errorf("wrong code must carry code-unknown, got %s", bad.Body.String())
	}
}

func TestRefreshAndRevokeFlow(t *testing.T) {
	h, _ := testHandler()
	call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"case-03@example.invalid"}`)
	login := call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"case-03@example.invalid","code":"482916"}`)
	var auth struct {
		Session struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			FamilyID     string `json:"family_id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil {
		t.Fatal(err)
	}
	refreshBody := `{"family_id":"` + auth.Session.FamilyID + `","refresh_token":"` + auth.Session.RefreshToken + `"}`
	rotated := call(t, h, "POST", "/v1/accounts/sessions/refresh", refreshBody)
	if rotated.Code != 200 {
		t.Fatalf("refresh: %d (%s)", rotated.Code, rotated.Body.String())
	}
	if rec := call(t, h, "POST", "/v1/accounts/sessions/refresh", refreshBody); rec.Code != 401 ||
		!strings.Contains(rec.Body.String(), "account.session-reuse-revoked") {
		t.Errorf("superseded refresh must be 401 reuse-revoked, got %d (%s)", rec.Code, rec.Body.String())
	}

	call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"case-03@example.invalid"}`)
	login2 := call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"case-03@example.invalid","code":"111111"}`)
	var auth2 struct {
		Session struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			FamilyID     string `json:"family_id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(login2.Body.Bytes(), &auth2); err != nil {
		t.Fatal(err)
	}
	revokeBody := `{"family_id":"` + auth2.Session.FamilyID + `","access_token":"` + auth2.Session.AccessToken + `"}`
	if rec := call(t, h, "POST", "/v1/accounts/sessions/revoke", revokeBody); rec.Code != 200 {
		t.Fatalf("revoke: %d (%s)", rec.Code, rec.Body.String())
	}
	refresh2 := `{"family_id":"` + auth2.Session.FamilyID + `","refresh_token":"` + auth2.Session.RefreshToken + `"}`
	if rec := call(t, h, "POST", "/v1/accounts/sessions/refresh", refresh2); rec.Code != 401 ||
		!strings.Contains(rec.Body.String(), "account.session-revoked") {
		t.Errorf("revoked family must be 401 session-revoked, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestBodiesNeverLeakSecrets(t *testing.T) {
	h, _ := testHandler()
	call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"case-04@example.invalid"}`)
	bodies := []*httptest.ResponseRecorder{
		call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"case-04@example.invalid","code":"000000"}`),
		call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"","code":""}`),
		call(t, h, "POST", "/v1/accounts/sessions/refresh", `{"family_id":"x","refresh_token":"tok-test"}`),
	}
	for _, rec := range bodies {
		body := rec.Body.String()
		if strings.Contains(body, "482916") || strings.Contains(body, "case-04@example.invalid") {
			t.Errorf("response must not echo code or address: %s", body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("account error must be no-store")
		}
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	h, _ := testHandler()
	big := `{"email":"` + strings.Repeat("a", 9000) + `@example.invalid"}`
	if rec := call(t, h, "POST", "/v1/accounts/email/codes", big); rec.Code != 400 {
		t.Errorf("oversized body must be 400, got %d", rec.Code)
	}
}
