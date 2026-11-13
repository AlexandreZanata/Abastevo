//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	orgd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountContextsThroughActualCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	key, e := accounts.CreateKeyAccount(t.Context(), "syncontexthttp")
	if e != nil {
		t.Fatal(e)
	}
	auth, e := accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{9}, 32), Pool: fx.f.Runtime, Accounts: accounts}); e != nil {
		t.Fatal(e)
	}
	var cookie *http.Cookie
	csrf := ""
	call := func(path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		request := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		if cookie != nil {
			request.AddCookie(cookie)
			request.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(request)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatalf("context HTTP policy/status got%d want%d", response.Code, want)
		}
		if response.Code >= 400 && (strings.Contains(response.Body.String(), "Synthetic") || strings.Contains(response.Body.String(), auth.Account.ID) || strings.Contains(response.Body.String(), "org_memberships")) {
			t.Fatal("private context metadata in error")
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("invalid context JSON")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing context cookie")
			}
			cookie = cookies[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	empty := map[string]any{}
	call("/v1/org-access/contexts/list", empty, 401, nil)
	call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, nil)
	page := call("/v1/org-access/contexts/list", empty, 200, nil)
	if page["account_id"] != auth.Account.ID || len(page["organizations"].([]any)) != 0 {
		t.Fatal("account context inferred unrelated organization")
	}
	created := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic Context HTTP", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": orgd.SyntheticTermsVersion, "digest": orgd.SyntheticTermsDigest, "accepted": true}}, 201, nil)
	org, member := created["organization_id"].(string), created["membership_id"].(string)
	if _, e := fx.flags.Set(t.Context(), scopeFor(t, org), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); e != nil {
		t.Fatal(e)
	}
	page = call("/v1/org-access/contexts/list", map[string]any{"limit": 1}, 200, nil)
	organizations := page["organizations"].([]any)
	if len(organizations) != 1 || organizations[0].(map[string]any)["id"] != org || page["next_after_org_id"] != "" {
		t.Fatal("current own-account context not discovered")
	}
	selected := map[string]any{"organization_id": org, "membership_id": member}
	view := call("/v1/org-access/contexts/read", selected, 200, nil)
	if view["account_id"] != auth.Account.ID || view["organization"].(map[string]any)["membership_id"] != member || len(view["invitation_grants"].([]any)) == 0 || view["invitation_validity_days"] != float64(7) {
		t.Fatal("selected context controls")
	}
	for _, tc := range []struct {
		edit func(*http.Request)
		want int
	}{{func(r *http.Request) { r.Header.Del("Cookie") }, 401}, {func(r *http.Request) { r.Header.Del("X-CSRF") }, 403}, {func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, 403}, {func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, 401}, {func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, 401}, {func(r *http.Request) { r.Header.Set("Signature", "synthetic") }, 401}} {
		call("/v1/org-access/contexts/list", empty, tc.want, tc.edit)
	}
	for _, field := range []string{"account_id", "family_id", "access_token", "roles", "permission", "session_id"} {
		call("/v1/org-access/contexts/list", map[string]any{field: "synthetic"}, 400, nil)
	}
	call("/v1/org-access/contexts/list", map[string]any{"limit": 0}, 400, nil)
	call("/v1/org-access/contexts/list", map[string]any{"after_org_id": "INVALID"}, 400, nil)
	call("/v1/org-access/contexts/list", map[string]any{"after_org_id": strings.Repeat("x", 17*1024)}, 400, nil)
	call("/v1/org-access/contexts/list?limit=1", empty, 400, nil)
	call("/v1/org-access/contexts/read", map[string]any{"organization_id": fx.scope.OrgID().String(), "membership_id": fx.member}, 404, nil)
	call("/v1/org-access/contexts/read", map[string]any{"organization_id": org, "membership_id": onboardingID(t)}, 404, nil)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '16 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, auth.Account.ID); e != nil {
		t.Fatal(e)
	}
	view = call("/v1/org-access/contexts/read", selected, 200, nil)
	if len(view["permissions"].([]any)) != 1 || view["permissions"].([]any)[0] != "idn.employee.read" || len(view["invitation_grants"].([]any)) != 0 {
		t.Fatal("stale proof enabled private management")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, auth.Session.FamilyID); e != nil {
		t.Fatal(e)
	}
	call("/v1/org-access/contexts/list", empty, 401, nil)
}
