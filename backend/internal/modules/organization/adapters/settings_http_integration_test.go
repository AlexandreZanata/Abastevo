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

func TestSettingsThroughComposedCompanyHTTP(t *testing.T) {
	fx := settingsSchemaFixture(t)
	grantEmployeeTables(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synsettingshttp")
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
			t.Fatal("private settings request encoding failed")
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
			t.Fatalf("settings HTTP policy/status mismatch: got %d want %d", response.Code, want)
		}
		if response.Code >= 400 && (strings.Contains(response.Body.String(), "Synthetic") || strings.Contains(response.Body.String(), "PRIVATE-CODE")) {
			t.Fatal("private settings content entered error")
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private settings response")
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
	sourceFx.member = organization["membership_id"].(string)
	base := "/v1/orgs/" + org + "/settings"
	otherBase := "/v1/orgs/" + other["organization_id"].(string) + "/settings"
	empty := map[string]any{}
	original := call(base+"/read", empty, 200, nil)
	if original["source"] != "ONBOARDING" || original["ownership_transfer_validity_days"] != float64(7) {
		t.Fatal("settings initial source/defaults")
	}
	values := original["values"].(map[string]any)
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "values": values, "reason": "Synthetic private settings reason"}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }} {
		call(base+"/change", change, 401, edit)
	}
	for _, field := range []string{"account_id", "source", "author", "effective_at", "roles", "retention_days", "root_revision"} {
		change[field] = "synthetic"
		call(base+"/change", change, 400, nil)
		delete(change, field)
	}
	values["time_zone"] = "GMT"
	call(base+"/change", change, 422, nil)
	values["time_zone"] = "America/Cuiaba"
	values["closure_period_days"] = 6
	invalid := call(base+"/change", change, 422, nil)["error"].(map[string]any)["details"].([]any)[0].(map[string]any)
	if invalid["field"] != "closure_period_days" || invalid["code"] != "RANGE_7_90" {
		t.Fatal("allowed range not exposed safely")
	}
	values["closure_period_days"] = 30
	values["time_zone"] = "Pacific/Auckland"
	values["consumption_display"] = "L_PER_100KM"
	first := call(base+"/change", change, 200, nil)
	if first["revision"] != float64(2) || first["root_revision"] != float64(2) {
		t.Fatal("settings/root revision not published")
	}
	replay := call(base+"/change", change, 200, nil)
	if replay["replayed"] != true || replay["effective_at"] != first["effective_at"] {
		t.Fatal("settings HTTP replay")
	}
	change["reason"] = "Synthetic changed intent"
	call(base+"/change", change, 409, nil)
	change["operation_id"] = onboardingID(t)
	call(base+"/change", change, 409, nil)
	if call(base+"/revision/read", map[string]any{"revision": 1}, 200, nil)["values"].(map[string]any)["time_zone"] != "America/Cuiaba" {
		t.Fatal("old settings rewritten")
	}
	if call("/v1/orgs/"+org+"/units/"+organization["unit_id"].(string)+"/read", empty, 200, nil)["time_zone"] != "America/Cuiaba" {
		t.Fatal("existing unit silently inherited new zone")
	}
	newUnit := call("/v1/orgs/"+org+"/units/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic Settings New Default", "code": "SETTINGS-NEW"}, 201, nil)["unit_id"].(string)
	if call("/v1/orgs/"+org+"/units/"+newUnit+"/read", empty, 200, nil)["time_zone"] != "Pacific/Auckland" {
		t.Fatal("new unit ignored changed root default")
	}
	call(base+"/revision/read", map[string]any{"revision": 0}, 422, nil)
	call(base+"/revision/read", map[string]any{"revision": 999}, 404, nil)
	page := call(base+"/history", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("settings cursor absent")
	}
	call(base+"/history", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	call(base+"/history", map[string]any{"limit": 2, "cursor": cursor}, 400, nil)
	call(otherBase+"/history", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	// A genuine DRIVER may be selected as a contact, with no implicit grant.
	k, e := accounts.CreateKeyAccount(t.Context(), "synsetdriver")
	if e != nil {
		t.Fatal(e)
	}
	driver, e := accounts.LoginWithKey(t.Context(), k.Key)
	if e != nil {
		t.Fatal(e)
	}
	_, member := teamDriver(t, sourceFx, driver.Account.ID)
	values["responsibilities"].(map[string]any)["review_fallback"] = map[string]any{"primary": member, "substitute": sourceFx.member}
	change["revision"] = 2
	change["operation_id"] = onboardingID(t)
	call(base+"/change", change, 200, nil)
	ownerCookie, ownerCSRF := cookie, csrf
	cookie = nil
	csrf = ""
	call("/v1/org-access/session/login", map[string]any{"family_id": driver.Session.FamilyID, "access_token": driver.Session.AccessToken}, 200, nil)
	call(base+"/read", empty, 403, nil)
	call(base+"/history", empty, 403, nil)
	call(base+"/change", change, 403, nil)
	cookie, csrf = ownerCookie, ownerCSRF
	if e := unitSchemaExec(t, sourceFx, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, member); e != nil {
		t.Fatal(e)
	}
	reduced := call(base+"/read", empty, 200, nil)
	if reduced["revision"] != float64(4) || reduced["notice_state"] != "PENDING_NOTIFICATION_CORE" {
		t.Fatal("settings reduction response")
	}
	// Every newly explicit read role can read, while only ADMIN also manages.
	for i, role := range []string{"ADMIN", "FLEET_MANAGER", "SUPERVISOR", "FINANCE", "APPROVER", "AUDITOR"} {
		k, e := accounts.CreateKeyAccount(t.Context(), "synsetreader"+string(rune('a'+i)))
		if e != nil {
			t.Fatal(e)
		}
		reader, e := accounts.LoginWithKey(t.Context(), k.Key)
		if e != nil {
			t.Fatal(e)
		}
		_, member := teamDriver(t, sourceFx, reader.Account.ID)
		if e := unitSchemaExec(t, sourceFx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,'ORG',$1,clock_timestamp()-interval '1 minute',$5,'synthetic settings reader')`, org, onboardingID(t), member, role, sourceFx.member); e != nil {
			t.Fatal(e)
		}
		cookie = nil
		csrf = ""
		call("/v1/org-access/session/login", map[string]any{"family_id": reader.Session.FamilyID, "access_token": reader.Session.AccessToken}, 200, nil)
		call(base+"/read", empty, 200, nil)
		call(base+"/revision/read", map[string]any{"revision": 1}, 200, nil)
		if role != "ADMIN" {
			call(base+"/change", change, 403, nil)
		}
	}
	cookie, csrf = ownerCookie, ownerCSRF
	// Foreign and unavailable references produce bounded refusal without settings.
	values["responsibilities"].(map[string]any)["review_fallback"] = map[string]any{"primary": other["membership_id"], "substitute": ""}
	change["revision"] = 4
	change["operation_id"] = onboardingID(t)
	call(base+"/change", change, 404, nil)
	values["responsibilities"].(map[string]any)["review_fallback"] = map[string]any{"primary": member, "substitute": ""}
	call(base+"/change", change, 422, nil)
	values["responsibilities"].(map[string]any)["review_fallback"] = map[string]any{"primary": "", "substitute": ""}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_settings_http_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic settings receipt outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_settings_http_failure AFTER INSERT ON org_settings_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_settings_http_failure()`); e != nil {
		t.Fatal(e)
	}
	call(base+"/change", change, 503, nil)
	if call(base+"/read", empty, 200, nil)["revision"] != float64(4) {
		t.Fatal("HTTP failure left new settings")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_settings_http_failure ON org_settings_commands`); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	call(base+"/read", empty, 200, nil)
	call(base+"/change", change, 403, nil)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, auth.Session.FamilyID); e != nil {
		t.Fatal(e)
	}
	call(base+"/read", empty, 401, nil)
	call(base+"/history", empty, 401, nil)
}
