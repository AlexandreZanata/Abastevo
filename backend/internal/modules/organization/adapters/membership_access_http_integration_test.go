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

func TestMembershipAccessThroughActualCompanyHTTPAndContract(t *testing.T) {
	fx, _ := employeeFixture(t)
	fx.f.GrantMembershipAccessCommands(t)
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
	read := call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	if read["state"] != "ACTIVE" || read["account_id"] != nil {
		t.Fatal("private membership view")
	}
	pause := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": read["revision"], "reason": "Synthetic HTTP pause"}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		call(base+"/suspend", pause, 401, "EnterpriseError", edit)
	}
	for _, field := range []string{"account_id", "roles", "state", "session_id", "suspension_origin"} {
		pause[field] = "synthetic"
		call(base+"/suspend", pause, 400, "EnterpriseError", nil)
		delete(pause, field)
	}
	pause["reason"] = nil
	call(base+"/suspend", pause, 400, "EnterpriseError", nil)
	pause["reason"] = " "
	call(base+"/suspend", pause, 422, "EnterpriseError", nil)
	pause["reason"] = "Synthetic HTTP pause"
	managerCookie, managerCSRF := cookie, csrf
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	targetCookie, targetCSRF := cookie, csrf
	call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	refusal := call(base+"/suspend", pause, 403, "EnterpriseError", nil)
	if refusal["error"].(map[string]any)["code"] != "FORBIDDEN" {
		t.Fatal("self suspension")
	}
	cookie, csrf = managerCookie, managerCSRF
	first := call(base+"/suspend", pause, 200, "EnterpriseMembershipAccessResult", nil)
	if first["state"] != "SUSPENDED" || first["suspension_origin"] != "MANAGER" || first["replayed"] != false {
		t.Fatal("suspension result")
	}
	replay := call(base+"/suspend", pause, 200, "EnterpriseMembershipAccessResult", nil)
	if replay["replayed"] != true || replay["occurred_at"] != first["occurred_at"] {
		t.Fatal("original HTTP outcome")
	}
	cookie, csrf = targetCookie, targetCSRF
	denied := call(base+"/read", map[string]any{}, 403, "EnterpriseError", nil)
	if denied["error"].(map[string]any)["code"] != "MEMBERSHIP_SUSPENDED" {
		t.Fatal("next request retained access")
	}
	call("/v1/org-access/session/read", map[string]any{}, 200, "", nil)
	call(otherBase+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	cookie, csrf = managerCookie, managerCSRF
	resume := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": first["revision"], "reason": "Synthetic HTTP resume"}
	active := call(base+"/reactivate", resume, 200, "EnterpriseMembershipAccessResult", nil)
	if active["state"] != "ACTIVE" || active["previous_suspension_origin"] != "MANAGER" {
		t.Fatal("reactivation result")
	}
	original := call(base+"/suspend", pause, 200, "EnterpriseMembershipAccessResult", nil)
	if original["state"] != "SUSPENDED" || original["occurred_at"] != first["occurred_at"] {
		t.Fatal("historical acknowledgement")
	}
	if call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)["state"] != "ACTIVE" {
		t.Fatal("old replay changed current state")
	}
	cookie, csrf = targetCookie, targetCSRF
	call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	call("/v1/orgs/"+otherID+"/memberships/"+target+"/read", map[string]any{}, 404, "EnterpriseError", nil)
	cookie, csrf = managerCookie, managerCSRF
	pause["operation_id"] = onboardingID(t)
	pause["revision"] = active["revision"]
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_member_http_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic private failure';END $$;CREATE CONSTRAINT TRIGGER synthetic_member_http_failure AFTER INSERT ON org_membership_access_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_member_http_failure()`); e != nil {
		t.Fatal(e)
	}
	call(base+"/suspend", pause, 503, "EnterpriseError", nil)
	if call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)["state"] != "ACTIVE" {
		t.Fatal("HTTP provisional suspension")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_member_http_failure ON org_membership_access_notices`); e != nil {
		t.Fatal(e)
	}
	call(base+"/suspend", pause, 200, "EnterpriseMembershipAccessResult", nil)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE identity_rate_windows SET count=200 WHERE subject_digest LIKE 'orgquota:v1:%'`); e != nil {
		t.Fatal(e)
	}
	call(base+"/suspend", pause, 429, "EnterpriseError", nil)
	call(base+"/reactivate", resume, 429, "EnterpriseError", nil)
	call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
}
