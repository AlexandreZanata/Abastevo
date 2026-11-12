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

func TestEmployeeMergeThroughActualCompanyHTTPAndContract(t *testing.T) {
	fx, _ := employeeFixture(t)
	fx.f.GrantMembershipAccessCommands(t)
	fx.f.GrantOffboardingCommands(t)
	fx.f.GrantRehireCommands(t)
	fx.f.GrantEmployeeMergeCommands(t)
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
	duplicate := call("/v1/orgs/"+orgID+"/employees/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic duplicate", "registration_number": "SYN-SOURCE", "work_email": "merge@example.invalid", "work_phone": "", "job_title": "", "home_unit_id": "", "allow_possible_duplicate": false}, 201, "", nil)
	survivor := call(base+"/offboarding/read", map[string]any{}, 200, "EnterpriseOffboardingView", nil)
	employeePath := "/v1/orgs/" + orgID + "/employees/" + duplicate["employee_id"].(string)
	preview := call(employeePath+"/merge/preview", map[string]any{"survivor_id": survivor["employee_id"]}, 200, "EnterpriseEmployeeMergePreview", nil)
	c := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": preview["source"].(map[string]any)["identity_revision"], "survivor_id": survivor["employee_id"], "survivor_revision": preview["survivor"].(map[string]any)["identity_revision"], "review_digest": preview["review_digest"], "reviewed": true, "reason": "  Synthetic reviewed HTTP duplicate  "}
	ownerCookie, ownerCSRF := cookie, csrf
	viewerKey, e := accounts.CreateKeyAccount(t.Context(), "synmergeviewer")
	if e != nil {
		t.Fatal(e)
	}
	viewer, e := accounts.LoginWithKey(t.Context(), viewerKey.Key)
	if e != nil {
		t.Fatal(e)
	}
	teamDriver(t, sourceFx, viewer.Account.ID)
	call("/v1/org-access/session/login", map[string]any{"family_id": viewer.Session.FamilyID, "access_token": viewer.Session.AccessToken}, 200, "", nil)
	call(employeePath+"/merge/preview", map[string]any{"survivor_id": survivor["employee_id"]}, 403, "EnterpriseError", nil)
	call(employeePath+"/merge/confirm", c, 403, "EnterpriseError", nil)
	cookie, csrf = ownerCookie, ownerCSRF

	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		call(employeePath+"/merge/confirm", c, 401, "EnterpriseError", edit)
	}
	for _, key := range []string{"schema_version", "operation_id", "revision", "survivor_id", "survivor_revision", "review_digest", "reviewed", "reason"} {
		original := c[key]
		delete(c, key)
		call(employeePath+"/merge/confirm", c, 400, "EnterpriseError", nil)
		c[key] = nil
		call(employeePath+"/merge/confirm", c, 400, "EnterpriseError", nil)
		c[key] = original
	}
	c["account_id"] = person.Account.ID
	call(employeePath+"/merge/confirm", c, 400, "EnterpriseError", nil)
	delete(c, "account_id")
	c["reviewed"] = false
	call(employeePath+"/merge/confirm", c, 422, "EnterpriseError", nil)
	c["reviewed"] = true
	call("/v1/orgs/"+otherID+"/employees/"+duplicate["employee_id"].(string)+"/merge/preview", map[string]any{"survivor_id": survivor["employee_id"]}, 404, "EnterpriseError", nil)
	// Two real live memberships conflict even though both are visible to the manager.
	call("/v1/orgs/"+orgID+"/employees/"+org["employee_id"].(string)+"/merge/preview", map[string]any{"survivor_id": survivor["employee_id"]}, 409, "EnterpriseError", nil)
	if _, e := fx.f.Admin.Exec(t.Context(), "CREATE FUNCTION syn_merge_http_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic merge late failure';END $$;CREATE CONSTRAINT TRIGGER syn_merge_http_failure AFTER INSERT ON org_employee_merge_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION syn_merge_http_failure()"); e != nil {
		t.Fatal(e)
	}
	call(employeePath+"/merge/confirm", c, 503, "EnterpriseError", nil)
	if v := call(employeePath+"/read", map[string]any{}, 200, "EnterpriseEmployeeView", nil); v["merged_into"] != "" || v["state"] != "ACTIVE" {
		t.Fatal("partial HTTP merge")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER syn_merge_http_failure ON org_employee_merge_commands"); e != nil {
		t.Fatal(e)
	}
	merged := call(employeePath+"/merge/confirm", c, 200, "EnterpriseEmployeeMergeResult", nil)
	if merged["source_id"] != duplicate["employee_id"] || merged["survivor_id"] != survivor["employee_id"] || merged["reason"] != c["reason"] {
		t.Fatal("chosen survivor/intent")
	}
	again := call(employeePath+"/merge/confirm", c, 200, "EnterpriseEmployeeMergeResult", nil)
	if again["replayed"] != true || again["occurred_at"] != merged["occurred_at"] {
		t.Fatal("original HTTP receipt")
	}
	c["reason"] = "Changed intent"
	call(employeePath+"/merge/confirm", c, 409, "EnterpriseError", nil)
	c["reason"] = "  Synthetic reviewed HTTP duplicate  "

	resolved := call(employeePath+"/resolution/read", map[string]any{}, 200, "EnterpriseEmployeeResolution", nil)
	if len(resolved["chain"].([]any)) != 2 || resolved["canonical"].(map[string]any)["id"] != survivor["employee_id"] {
		t.Fatal("historical lookup")
	}
	managerCookie, managerCSRF := cookie, csrf
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	call(base+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	call(otherBase+"/read", map[string]any{}, 200, "EnterpriseMembershipAccessView", nil)
	call("/v1/org-access/session/read", map[string]any{}, 200, "", nil)
	cookie, csrf = managerCookie, managerCSRF
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE identity_rate_windows SET count=200 WHERE subject_digest LIKE 'orgquota:v1:%'`); e != nil {
		t.Fatal(e)
	}
	call(employeePath+"/merge/confirm", c, 429, "EnterpriseError", nil)
	call(employeePath+"/resolution/read", map[string]any{}, 200, "EnterpriseEmployeeResolution", nil)
}
