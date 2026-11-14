//go:build integration

package adapters_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	web "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/go-chi/chi/v5"
)

// Native cookie storage is deliberately memory-only and has no browser jar.
// A retired cookie may be presented only to the existing fresh-proof login:
// it replaces that proven account's old company session, never personal proof.
func TestNativeCompanyCookieRotationKeepsAdmissionBoundedAndAccountsIndependent(t *testing.T) {
	f := setup(t)
	router := chi.NewRouter()
	server := httptest.NewTLSServer(router)
	defer server.Close()
	handler, e := web.New(f.service, server.URL)
	if e != nil {
		t.Fatal(e)
	}
	handler.RegisterRoutes(router)
	type channel struct {
		cookie *http.Cookie
		csrf   string
	}
	login := func(family, access string, previous *http.Cookie) channel {
		t.Helper()
		var cookies []*http.Cookie
		if previous != nil {
			cookies = []*http.Cookie{previous}
		}
		response, raw := post(t, server, web.PathPrefix+"login", map[string]string{"family_id": family, "access_token": access}, cookies, "", nil)
		if response.StatusCode != 200 || len(response.Cookies()) != 1 || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
			t.Fatalf("native company login policy/status mismatch: %d", response.StatusCode)
		}
		var view struct {
			AccountID string `json:"account_id"`
			CSRF      string `json:"csrf_token"`
		}
		if json.Unmarshal(raw, &view) != nil || view.AccountID == "" || view.CSRF == "" {
			t.Fatal("native company login metadata malformed")
		}
		return channel{response.Cookies()[0], view.CSRF}
	}
	read := func(ch channel, want int) {
		t.Helper()
		response, _ := post(t, server, web.PathPrefix+"read", map[string]any{}, []*http.Cookie{ch.cookie}, ch.csrf, nil)
		if response.StatusCode != want || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
			t.Fatalf("native company read policy/status got%d want%d", response.StatusCode, want)
		}
	}
	count := func(account string, want int) {
		t.Helper()
		var total, live int
		if e := f.db.Admin.QueryRow(t.Context(), `SELECT count(*),count(*) FILTER(WHERE revoked_at IS NULL) FROM company_web_sessions WHERE account_id=$1`, account).Scan(&total, &live); e != nil || total != want || live != want {
			t.Fatal("company rotation consumed additional admission", e)
		}
	}
	personal := f.auth.Session
	browser := login(personal.FamilyID, personal.AccessToken, nil)
	native := login(personal.FamilyID, personal.AccessToken, nil)
	count(f.auth.Account.ID, 2)
	// Exceed MaxLiveSessions through ordinary repeated ceremonies. The old
	// cookie is not authority for login; fresh source account proof remains mandatory.
	for range 12 {
		old := native
		native = login(personal.FamilyID, personal.AccessToken, old.cookie)
		if native.cookie.Value == old.cookie.Value || native.csrf == old.csrf {
			t.Fatal("native company cookie was not rotated")
		}
		count(f.auth.Account.ID, 2)
		read(old, 401)
		read(native, 200)
		read(browser, 200)
		if _, e := f.accounts.ValidateAccess(t.Context(), personal.FamilyID, personal.AccessToken); e != nil {
			t.Fatal("company rotation revoked independent personal proof", e)
		}
	}
	// Reauthentication may produce a new personal family. Rotation is still
	// bound to the proven account and must retain the independent old family.
	renewed, e := f.accounts.LoginWithKey(t.Context(), f.key)
	if e != nil {
		t.Fatal(e)
	}
	if renewed.Session.FamilyID == personal.FamilyID {
		t.Fatal("fixture did not issue a new personal family")
	}
	old := native
	native = login(renewed.Session.FamilyID, renewed.Session.AccessToken, old.cookie)
	count(f.auth.Account.ID, 2)
	read(old, 401)
	read(native, 200)
	read(browser, 200)
	for _, proof := range [][2]string{{personal.FamilyID, personal.AccessToken}, {renewed.Session.FamilyID, renewed.Session.AccessToken}} {
		if _, e := f.accounts.ValidateAccess(t.Context(), proof[0], proof[1]); e != nil {
			t.Fatal("native company replacement changed a personal family", e)
		}
	}
	// An old cookie can never retire another account's company session.
	key, e := f.accounts.CreateKeyAccount(t.Context(), "nativeforeign")
	if e != nil {
		t.Fatal(e)
	}
	foreign, e := f.accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	foreignChannel := login(foreign.Session.FamilyID, foreign.Session.AccessToken, native.cookie)
	count(f.auth.Account.ID, 2)
	count(foreign.Account.ID, 1)
	read(native, 200)
	read(browser, 200)
	read(foreignChannel, 200)
	if _, e := f.accounts.ValidateAccess(t.Context(), foreign.Session.FamilyID, foreign.Session.AccessToken); e != nil {
		t.Fatal("foreign company login changed its personal proof", e)
	}
}
