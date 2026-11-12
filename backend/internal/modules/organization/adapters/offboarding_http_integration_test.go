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
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOffboardingThroughActualCompanyHTTPAndContract(t *testing.T) {
	fx, _ := employeeFixture(t)
	fx.f.GrantMembershipAccessCommands(t)
	fx.f.GrantOffboardingCommands(t)
	grantTeamTables(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); e != nil {
		t.Fatal(e)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	key, e := accounts.CreateKeyAccount(t.Context(), "syninvitationmgr")
	if e != nil {
		t.Fatal(e)
	}
	auth, e := accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test location")
	}
	document, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(source), "../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{55}, 32), Pool: fx.f.Runtime, Accounts: accounts}); e != nil {
		t.Fatal(e)
	}
	var cookie *http.Cookie
	csrf := ""
	call := func(path string, body any, want int, schema string, edit func(*http.Request)) map[string]any {
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
			t.Fatalf("invitation HTTP %s: got%d want%d", path, response.Code, want)
		}
		if want == 429 && response.Header().Get("Retry-After") == "" {
			t.Fatal("quota omitted retry delay")
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("private response JSON")
		}
		if schema != "" {
			if e := document.Components.Schemas[schema].Value.VisitJSON(result); e != nil {
				t.Fatal("actual invitation wire violates contract", schema)
			}
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			if len(response.Result().Cookies()) != 1 {
				t.Fatal("cookie")
			}
			cookie = response.Result().Cookies()[0]
			csrf = result["csrf_token"].(string)
		}
		if want >= 400 && (strings.Contains(response.Body.String(), "AIN1.") || strings.Contains(response.Body.String(), "synthetic private") || strings.Contains(response.Body.String(), "SQLSTATE")) {
			t.Fatal("private error disclosure")
		}
		return result
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, "", nil)
	createOrg := func() map[string]any {
		t.Helper()
		org := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic invitation company", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": orgd.SyntheticTermsVersion, "digest": orgd.SyntheticTermsDigest, "accepted": true}}, 201, "", nil)
		if _, e := fx.flags.Set(t.Context(), scopeFor(t, org["organization_id"].(string)), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); e != nil {
			t.Fatal(e)
		}
		return org
	}

	org, other := createOrg(), createOrg()
	orgID := org["organization_id"].(string)
	otherID := other["organization_id"].(string)
	sourceFx := fx
	sourceFx.scope = scopeFor(t, orgID)
	sourceFx.member = org["membership_id"].(string)
	k, e := accounts.CreateKeyAccount(t.Context(), "synmembertarget")
	if e != nil {
		t.Fatal(e)
	}
	person, e := accounts.LoginWithKey(t.Context(), k.Key)
	if e != nil {
		t.Fatal(e)
	}
	_, target := teamDriver(t, sourceFx, person.Account.ID)
	if e := unitSchemaExec(t, sourceFx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp(),$4,'Synthetic membership manager')`, orgID, onboardingID(t), target, sourceFx.member); e != nil {
		t.Fatal(e)
	}
	otherFx := sourceFx
	otherFx.scope = scopeFor(t, otherID)
	otherFx.member = other["membership_id"].(string)
	_, otherMember := teamDriver(t, otherFx, person.Account.ID)
	if e := unitSchemaExec(t, otherFx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp(),$4,'Synthetic independent manager')`, otherID, onboardingID(t), otherMember, otherFx.member); e != nil {
		t.Fatal(e)
	}
	base := "/v1/orgs/" + orgID + "/memberships/" + target
	otherBase := "/v1/orgs/" + otherID + "/memberships/" + otherMember
	view := call(base+"/offboarding/read", map[string]any{}, 200, "EnterpriseOffboardingView", nil)
	command := func(v map[string]any) map[string]any {
		flags := []string{}
		for _, raw := range v["items"].([]any) {
			flags = append(flags, raw.(map[string]any)["key"].(string))
		}
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": v["revision"], "employee_revision": v["employee_revision"], "reason": "Synthetic HTTP ending", "handover_digest": v["handover_digest"], "flags": flags, "terminate_employee": false}
	}
	c := command(view)
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		call(base+"/offboarding/complete", c, 401, "EnterpriseError", edit)
	}
	for _, field := range []string{"employee_revision", "handover_digest", "flags", "terminate_employee"} {
		prior := c[field]
		delete(c, field)
		call(base+"/offboarding/complete", c, 400, "EnterpriseError", nil)
		c[field] = nil
		call(base+"/offboarding/complete", c, 400, "EnterpriseError", nil)
		c[field] = prior
	}
	c["account_id"] = person.Account.ID
	call(base+"/offboarding/complete", c, 400, "EnterpriseError", nil)
	delete(c, "account_id")
	if _, e := fx.f.Admin.Exec(t.Context(), "CREATE FUNCTION syn_offboard_http_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic HTTP late failure';END $$;CREATE CONSTRAINT TRIGGER syn_offboard_http_failure AFTER INSERT ON org_offboarding_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION syn_offboard_http_failure()"); e != nil {
		t.Fatal(e)
	}
	call(base+"/offboarding/complete", c, 503, "EnterpriseError", nil)
	if _, e := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER syn_offboard_http_failure ON org_offboarding_notices"); e != nil {
		t.Fatal(e)
	}
	ended := call(base+"/offboarding/complete", c, 200, "EnterpriseOffboardingResult", nil)
	if ended["state"] != "ENDED" || ended["end_reason"] != "OFFBOARDED" {
		t.Fatal("offboard HTTP")
	}
	again := call(base+"/offboarding/complete", c, 200, "EnterpriseOffboardingResult", nil)
	if again["replayed"] != true || again["occurred_at"] != ended["occurred_at"] {
		t.Fatal("original HTTP outcome")
	}
	managerCookie, managerCSRF := cookie, csrf
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	denied := call(base+"/read", map[string]any{}, 404, "EnterpriseError", nil)
	if denied["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatal("ended membership still exposed")
	}
	independent := call(otherBase+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	cookie, csrf = managerCookie, managerCSRF
	call(otherBase+"/suspend", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": independent["revision"], "reason": "Synthetic suspended leave"}, 200, "EnterpriseMembershipAccessResult", nil)
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	self := call(otherBase+"/leave/read", map[string]any{}, 200, "EnterpriseOffboardingView", nil)
	leave := command(self)
	for i, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		want := 401
		if i > 0 {
			want = 403
		}
		call(otherBase+"/leave", leave, want, "EnterpriseError", edit)
	}

	foreignBase := "/v1/orgs/" + otherID + "/memberships/" + other["membership_id"].(string)
	call(foreignBase+"/leave/read", map[string]any{}, 404, "EnterpriseError", nil)
	unready := onboardingID(t)
	unreadyFx := sourceFx
	unreadyFx.scope = scopeFor(t, unready)
	if e := unitSchemaExec(t, unreadyFx, `INSERT INTO organizations(org_id)VALUES($1)`, unready); e != nil {
		t.Fatal(e)
	}
	call("/v1/orgs/"+unready+"/memberships/"+target+"/leave/read", map[string]any{}, 404, "EnterpriseError", nil)

	leave["terminate_employee"] = true
	call(otherBase+"/leave", leave, 422, "EnterpriseError", nil)
	leave["terminate_employee"] = false
	left := call(otherBase+"/leave", leave, 200, "EnterpriseOffboardingResult", nil)
	if left["end_reason"] != "LEFT" || left["suspension_origin"] != "MANAGER" {
		t.Fatal("suspended self leave lost cause")
	}
	retry := call(otherBase+"/leave", leave, 200, "EnterpriseOffboardingResult", nil)
	if retry["replayed"] != true || retry["occurred_at"] != left["occurred_at"] {
		t.Fatal("ended self retry")
	}
	call("/v1/org-access/session/read", map[string]any{}, 200, "", nil)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, person.Account.ID); e != nil {
		t.Fatal(e)
	}
	call(otherBase+"/leave", leave, 401, "EnterpriseError", nil)
}
