//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestTeamsThroughComposedCompanyHTTP(t *testing.T) {
	fx := teamSchemaFixture(t)
	grantEmployeeTables(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticteamhttp")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{9}, 32), Pool: fx.f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	csrf := ""
	call := func(path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal("private centre request encoding failed")
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
			t.Fatalf("team HTTP policy/status mismatch: got %d want %d", response.Code, want)
		}
		if response.Code >= 400 && (strings.Contains(response.Body.String(), "Synthetic") || strings.Contains(response.Body.String(), "PRIVATE-CODE")) {
			t.Fatal("private centre content entered error")
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private centre response")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing company cookie")
			}
			cookie = cookies[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, nil)
	createOrg := func() map[string]any {
		t.Helper()
		result := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic cost HTTP", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201, nil)
		org := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, org), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	organization, other := createOrg(), createOrg()
	org := organization["organization_id"].(string)
	scope := scopeFor(t, org)
	sourceFx := fx
	sourceFx.scope = scope
	base := "/v1/orgs/" + org + "/teams"
	otherBase := "/v1/orgs/" + other["organization_id"].(string) + "/teams"
	empty := map[string]any{}
	call("/v1/orgs/"+org+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, nil)
	if items := call(base+"/list", empty, 200, nil)["items"].([]any); len(items) != 0 {
		t.Fatal("team incorrectly required at setup")
	}
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic Private Crew", "unit_id": organization["unit_id"]}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }} {
		call(base+"/create", create, 401, edit)
	}
	for _, field := range []string{"account_id", "team_id", "employee_id", "state", "revision", "roles", "permission", "usage", "active_employees"} {
		create[field] = "synthetic"
		call(base+"/create", create, 400, nil)
		delete(create, field)
	}
	team := call(base+"/create", create, 201, nil)
	id := team["team_id"].(string)
	path := base + "/" + id
	if again := call(base+"/create", create, 201, nil); again["team_id"] != id || again["replayed"] != true {
		t.Fatal("team creation replay")
	}
	employee := call("/v1/orgs/"+org+"/employees/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic Crew Employee"}, 201, nil)["employee_id"].(string)
	memberPath := path + "/members/" + employee
	add := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	assigned := call(memberPath+"/add", add, 200, nil)
	if assigned["revision"] != float64(2) || assigned["link_id"] == "" {
		t.Fatal("team link ack")
	}
	call(memberPath+"/add", add, 200, nil)
	call(path+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}, 409, nil)
	remove := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}
	removed := call(memberPath+"/remove", remove, 200, nil)
	if removed["revision"] != float64(3) || removed["link_id"] != assigned["link_id"] {
		t.Fatal("link removal acknowledgement")
	}
	addAgain := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3}
	second := call(memberPath+"/add", addAgain, 200, nil)
	if second["link_id"] == assigned["link_id"] {
		t.Fatal("old link reused")
	}
	page := call(path+"/members/list", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing link cursor")
	}
	call(path+"/members/list", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	call(path+"/members/list", map[string]any{"limit": 1, "state": "ACTIVE", "cursor": cursor}, 400, nil)
	another := call(base+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic Another Crew"}, 201, nil)["team_id"].(string)
	call(base+"/"+another+"/members/list", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	call(otherBase+"/"+id+"/read", empty, 404, nil)
	call(otherBase+"/"+onboardingID(t)+"/read", empty, 404, nil)
	call(path+"/members/"+onboardingID(t)+"/add", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 404, nil)
	call(base+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic Foreign Unit", "unit_id": other["unit_id"]}, 404, nil)
	duplicate := call(base+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "synthetic private crew"}, 422, nil)
	if duplicate["error"].(map[string]any)["code"] != "VALIDATION_FAILED" {
		t.Fatal("duplicate team field")
	}
	call(memberPath+"/remove", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 200, nil)
	archive := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 5}
	call(path+"/archive", archive, 200, nil)
	call(memberPath+"/add", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 6}, 409, nil)
	if call(path+"/read", empty, 200, nil)["display_name"] != "Synthetic Private Crew (arquivada)" {
		t.Fatal("archived display")
	}
	call(path+"/restore", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 6}, 200, nil)
	// Joining a team does not change a genuine DRIVER's current authority.
	k, e := accounts.CreateKeyAccount(t.Context(), "synteamdriver")
	if e != nil {
		t.Fatal(e)
	}
	driver, e := accounts.LoginWithKey(t.Context(), k.Key)
	if e != nil {
		t.Fatal(e)
	}
	sourceFx.member = organization["membership_id"].(string)
	worker, member := teamDriver(t, sourceFx, driver.Account.ID)
	call(path+"/members/"+worker+"/add", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 7}, 200, nil)
	ownerCookie, ownerCSRF := cookie, csrf
	cookie = nil
	csrf = ""
	call("/v1/org-access/session/login", map[string]any{"family_id": driver.Session.FamilyID, "access_token": driver.Session.AccessToken}, 200, nil)
	call(path+"/read", empty, 403, nil)
	call(path+"/members/list", empty, 403, nil)
	call(base+"/create", create, 403, nil)
	cookie, csrf = ownerCookie, ownerCSRF
	if e := unitSchemaExec(t, sourceFx, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, member); e != nil {
		t.Fatal(e)
	}
	current := call(path+"/members/list", map[string]any{"state": "ACTIVE"}, 200, nil)["items"].([]any)
	if len(current) != 0 {
		t.Fatal("suspension left team links")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_team_http_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic team receipt outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_team_http_failure AFTER INSERT ON org_team_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_team_http_failure()`); e != nil {
		t.Fatal(e)
	}
	failed := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 9, "name": "Synthetic Failed HTTP"}
	call(path+"/change", failed, 503, nil)
	if call(path+"/read", empty, 200, nil)["revision"] != float64(9) {
		t.Fatal("failed commit changed team")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_team_http_failure ON org_team_commands`); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	call(path+"/read", empty, 200, nil)
	call(path+"/change", failed, 403, nil)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, auth.Session.FamilyID); e != nil {
		t.Fatal(e)
	}
	call(path+"/read", empty, 401, nil)
	call(base+"/create", create, 401, nil)
}
