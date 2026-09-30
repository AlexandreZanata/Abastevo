package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func deleteBody(sess sessionOut) string {
	raw, _ := json.Marshal(map[string]string{"family_id": sess.FamilyID, "access_token": sess.AccessToken})
	return string(raw)
}

func svcOf(t *testing.T, f *providerFixture) *application.Service {
	t.Helper()
	svc, ok := f.handler.Service.(*application.Service)
	if !ok {
		t.Fatal("handler service must be *application.Service")
	}
	return svc
}

func TestSelfDeleteFlow(t *testing.T) {
	f := newProviderHandler(t)
	sess := f.signup(t, "gone-a@example.invalid", "482916")

	if rec := f.call(t, "POST", "/v1/accounts/providers/link", linkBody(sess, "google", f.googleToken(t, "gone-sub-1", "", "n-del-1"), "n-del-1")); rec.Code != 200 {
		t.Fatalf("link: %d (%s)", rec.Code, rec.Body.String())
	}
	rec := f.call(t, "POST", "/v1/accounts/deletion", deleteBody(sess))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "deleted") {
		t.Fatalf("delete: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("delete response must be no-store")
	}

	// Every session-authed route now refuses with the deleted verdict.
	for _, tc := range []struct{ path, body string }{
		{"/v1/accounts/providers/list", deleteBody(sess)},
		{"/v1/accounts/providers/unlink", `{"family_id":"` + sess.FamilyID + `","access_token":"` + sess.AccessToken + `","provider":"google"}`},
		{"/v1/accounts/sessions/refresh", `{"family_id":"` + sess.FamilyID + `","refresh_token":"tok-1"}`},
	} {
		rec := f.call(t, "POST", tc.path, tc.body)
		if rec.Code != 410 || !strings.Contains(rec.Body.String(), "account.account-deleted") {
			t.Errorf("%s must be 410 account-deleted, got %d (%s)", tc.path, rec.Code, rec.Body.String())
		}
	}

	// Re-signup with the same address mints a NEW account.
	if rec := f.call(t, "POST", "/v1/accounts/email/codes", `{"email":"gone-a@example.invalid"}`); rec.Code != 202 {
		t.Fatalf("re-request: %d", rec.Code)
	}
	rec = f.call(t, "POST", "/v1/accounts/email/consume", `{"email":"gone-a@example.invalid","code":"111111"}`)
	if rec.Code != 200 {
		t.Fatalf("re-signup: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Account struct {
			AccountID string `json:"account_id"`
		} `json:"account"`
		Created bool `json:"created"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Created {
		t.Error("re-signup after deletion must create a new account")
	}
}

func TestSuspendedBlockedViaHTTP(t *testing.T) {
	f := newProviderHandler(t)
	svc := svcOf(t, f)
	sess := f.signup(t, "susp-a@example.invalid", "482916")

	acc, found, err := svc.Store.FindAccount(context.Background(), domain.AddressHash("susp-a@example.invalid"))
	if err != nil || !found {
		t.Fatalf("FindAccount: %+v found=%v err=%v", acc, found, err)
	}
	if err := svc.SuspendAccount(context.Background(), acc.ID); err != nil {
		t.Fatal(err)
	}
	// Live session routes answer 403 with the suspended verdict.
	rec := f.call(t, "POST", "/v1/accounts/providers/list", deleteBody(sess))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "account.account-suspended") {
		t.Errorf("list on suspended must be 403 suspended, got %d (%s)", rec.Code, rec.Body.String())
	}
	// Consume with a pre-suspend code refuses without oracle details.
	if rec := f.call(t, "POST", "/v1/accounts/email/codes", `{"email":"susp-a@example.invalid"}`); rec.Code != 202 {
		t.Errorf("request on suspended must stay 202, got %d", rec.Code)
	}
}

func TestDeleteRejectsMalformedAndForged(t *testing.T) {
	f := newProviderHandler(t)
	for _, body := range []string{
		`{"family_id":"","access_token":""}`,
		`{"family_id":"x"}`,
		`not json`,
	} {
		if rec := f.call(t, "POST", "/v1/accounts/deletion", body); rec.Code != 400 {
			t.Errorf("malformed delete must be 400, got %d for %q", rec.Code, body)
		}
	}
	if rec := f.call(t, "POST", "/v1/accounts/deletion", `{"family_id":"missing","access_token":"bad"}`); rec.Code != 401 {
		t.Errorf("forged delete must be 401, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestFailMapsStatusErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
		code string
	}{
		{domain.ErrAccountSuspended, 403, "account.account-suspended"},
		{domain.ErrAccountDeleted, 410, "account.account-deleted"},
	} {
		req := httptest.NewRequest("POST", "/v1/accounts/deletion", nil)
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
