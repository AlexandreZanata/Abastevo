//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Organization creation authenticates the company cookie/Origin/CSRF/fresh
// proof before judging the body's meaning, and every attempt, refused or not,
// spends the creating account's write budget in its own committed admission.
func TestOrganizationCreationAuthenticatesFirstAndSpendsAccountAdmission(t *testing.T) {
	f, _, _, _, _ := onboardingFixture(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	accounts := &account.Service{Clock: onboardingClock{now}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticadmission")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	const origin = "https://company.example.invalid"
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{9}, 32), Pool: f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	csrf := ""
	call := func(path string, body any, edit func(*http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
			request.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(request)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	const create = "/v1/org-access/organizations/create"
	// Semantically invalid (terms not accepted) but syntactically canonical.
	invalid := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic admission company", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main synthetic unit", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": false}}
	if got := call(create, invalid, nil).Code; got != http.StatusUnauthorized {
		t.Fatal("unauthenticated caller learned body validation", got)
	}
	login := call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, nil)
	if login.Code != 200 {
		t.Fatal("login", login.Code, login.Body.String())
	}
	cookie = login.Result().Cookies()[0]
	var session map[string]any
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	csrf = session["csrf_token"].(string)
	if got := call(create, invalid, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }).Code; got != http.StatusForbidden {
		t.Fatal("cross-site caller learned body validation", got)
	}
	spent := func() int {
		t.Helper()
		var n int
		if err := f.Admin.QueryRow(t.Context(), `SELECT COALESCE(sum(count),0)::integer FROM identity_rate_windows WHERE subject_digest LIKE 'orgcreate:v1:%' AND operation='write'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if spent() != 0 {
		t.Fatal("refused authentication spent creation admission")
	}
	if got := call(create, invalid, nil).Code; got != http.StatusUnprocessableEntity {
		t.Fatal("authenticated invalid creation", got)
	}
	valid := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic admission company", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main synthetic unit", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}
	if got := call(create, valid, nil); got.Code != http.StatusCreated {
		t.Fatal("valid creation", got.Code, got.Body.String())
	}
	reused := map[string]any{}
	for k, v := range valid {
		reused[k] = v
	}
	reused["name"] = "Changed synthetic intent"
	if got := call(create, reused, nil).Code; got != http.StatusConflict {
		t.Fatal("changed intent", got)
	}
	// The valid creation and the refused reuse were admitted and committed
	// independently of the creation transaction's outcome.
	if got := spent(); got != 2 {
		t.Fatal("creation admission spent", got)
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE identity_rate_windows SET count=200 WHERE subject_digest LIKE 'orgcreate:v1:%' AND operation='write'`); err != nil {
		t.Fatal(err)
	}
	valid["operation_id"] = onboardingID(t)
	limited := call(create, valid, nil)
	retry, err := strconv.Atoi(limited.Header().Get("Retry-After"))
	if limited.Code != http.StatusTooManyRequests || err != nil || retry < 1 || retry > 3600 {
		t.Fatal("exhausted creation admission", limited.Code, limited.Header().Get("Retry-After"), limited.Body.String())
	}
	var created int
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_onboarding_commands WHERE actor_id=$1 AND kind='CREATED'`, auth.Account.ID).Scan(&created); err != nil || created != 1 {
		t.Fatal("limited creation wrote", created, err)
	}
}
