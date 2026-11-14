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
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInvitationsThroughActualCompanyHTTPAndContract(t *testing.T) {
	fx, _ := employeeFixture(t)
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
	base := "/v1/orgs/" + orgID + "/invitations"
	employeeBase := "/v1/orgs/" + orgID + "/employees"
	employee := call(employeeBase+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic recipient", "work_email": "recipient@example.invalid"}, 201, "", nil)["employee_id"].(string)
	input := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}
	foreignEmployee := call("/v1/orgs/"+other["organization_id"].(string)+"/employees/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic other organization recipient"}, 201, "", nil)["employee_id"].(string)
	call(base+"/issue", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": foreignEmployee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}, 404, "EnterpriseError", nil)

	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		want := 401
		call(base+"/issue", input, want, "EnterpriseError", edit)
	}
	// Source request body strictness, including nested grants and explicit nulls.
	for _, invalid := range []map[string]any{
		{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": nil},
		{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"Role": "DRIVER", "scope": "SELF"}}},
		{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF", "account_id": onboardingID(t)}}},
	} {
		call(base+"/issue", invalid, 400, "EnterpriseError", nil)
	}
	result := call(base+"/issue", input, 200, "EnterpriseInvitationResult", nil)
	delivery, ok := result["delivery"].(map[string]any)
	if !ok {
		t.Fatal("first response lost delivery")
	}
	code := delivery["code"].(string)
	u, e := url.Parse(delivery["link"].(string))
	if e != nil || u.RawQuery != "" || !strings.Contains(u.Fragment, code) || delivery["qr_payload"] != delivery["link"] {
		t.Fatal("secret delivery escaped fragment")
	}
	v := result["invitation"].(map[string]any)
	id := v["id"].(string)
	retried := call(base+"/issue", input, 200, "EnterpriseInvitationResult", nil)
	if retried["delivery"] != nil || retried["replayed"] != true || retried["invitation"].(map[string]any)["id"] != id {
		t.Fatal("secret redisplayed on retry")
	}
	read := call(base+"/"+id+"/read", map[string]any{}, 200, "EnterpriseInvitationView", nil)
	if read["state"] != "PENDING" || read["delivery"] != nil {
		t.Fatal("private status")
	}
	call("/v1/orgs/"+other["organization_id"].(string)+"/invitations/"+id+"/read", map[string]any{}, 404, "EnterpriseError", nil)
	call(base+"/list", map[string]any{"limit": 1}, 200, "EnterpriseInvitationPage", nil)
	resend := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	next := call(base+"/"+id+"/resend", resend, 200, "EnterpriseInvitationResult", nil)
	nextID := next["invitation"].(map[string]any)["id"].(string)
	if next["delivery"].(map[string]any)["code"] == code {
		t.Fatal("resend reused secret")
	}
	if retry := call(base+"/"+id+"/resend", resend, 200, "EnterpriseInvitationResult", nil); retry["delivery"] != nil || retry["replayed"] != true {
		t.Fatal("resend retry leaked")
	}
	call(base+"/"+id+"/resend", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 409, "EnterpriseError", nil)
	// Real contact edit consumes the pending invitation and reservation atomically.
	call(employeeBase+"/"+employee+"/change", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "display_name": "Synthetic recipient", "work_email": "changed@example.invalid"}, 200, "", nil)
	reduced := call(base+"/"+nextID+"/read", map[string]any{}, 200, "EnterpriseInvitationView", nil)
	if reduced["state"] != "REVOKED" || reduced["cause"] != "CONTACT_CHANGED" {
		t.Fatal("contact edit did not revoke token")
	}
	freshInput := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}
	fresh := call(base+"/issue", freshInput, 200, "EnterpriseInvitationResult", nil)
	freshID := fresh["invitation"].(map[string]any)["id"].(string)
	revoke := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	call(base+"/"+freshID+"/revoke", revoke, 200, "EnterpriseInvitationResult", nil)
	if retry := call(base+"/"+freshID+"/revoke", revoke, 200, "EnterpriseInvitationResult", nil); retry["delivery"] != nil || retry["replayed"] != true {
		t.Fatal("terminal revoke retry")
	}
	// Late receipt failure exposes no credential, metadata or partial membership.
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_invitation_http_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic private storage failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_invitation_http_failure AFTER INSERT ON org_invitation_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_invitation_http_failure()`); e != nil {
		t.Fatal(e)
	}
	failed := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}
	failure := call(base+"/issue", failed, 503, "EnterpriseError", nil)
	if failure["delivery"] != nil || failure["invitation"] != nil {
		t.Fatal("provisional failed delivery")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_invitation_http_failure ON org_invitation_commands`); e != nil {
		t.Fatal(e)
	}
	call(base+"/issue", failed, 200, "EnterpriseInvitationResult", nil)
	// A genuine personal account with only DRIVER/SELF cannot manage invitations,
	// including reads and retries, even when it knows exact IDs and operations.
	driverKey, e := accounts.CreateKeyAccount(t.Context(), "syninvdriver")
	if e != nil {
		t.Fatal(e)
	}
	driver, e := accounts.LoginWithKey(t.Context(), driverKey.Key)
	if e != nil {
		t.Fatal(e)
	}
	sourceFx := fx
	sourceFx.scope = scopeFor(t, orgID)
	sourceFx.member = org["membership_id"].(string)
	driverEmployee, _ := teamDriver(t, sourceFx, driver.Account.ID)
	refusal := call(base+"/issue", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": driverEmployee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}, 409, "EnterpriseError", nil)
	if refusal["error"].(map[string]any)["code"] != "EMPLOYEE_HAS_ACCESS" {
		t.Fatal("existing access refusal contract")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE identity_rate_windows SET count=200 WHERE subject_digest LIKE 'orgquota:v1:%'`); e != nil {
		t.Fatal(e)
	}
	call(base+"/issue", failed, 429, "EnterpriseError", nil)
	call(base+"/"+freshID+"/revoke", revoke, 429, "EnterpriseError", nil)
	call(base+"/"+id+"/resend", resend, 429, "EnterpriseError", nil)
	call(base+"/"+freshID+"/read", map[string]any{}, 200, "EnterpriseInvitationView", nil)
	call("/v1/org-access/session/login", map[string]any{"family_id": driver.Session.FamilyID, "access_token": driver.Session.AccessToken}, 200, "", nil)
	call(base+"/issue", failed, 403, "EnterpriseError", nil)
	call(base+"/"+id+"/resend", resend, 403, "EnterpriseError", nil)
	call(base+"/"+freshID+"/revoke", revoke, 403, "EnterpriseError", nil)
	call(base+"/"+id+"/read", map[string]any{}, 403, "EnterpriseError", nil)
	call(base+"/list", map[string]any{}, 403, "EnterpriseError", nil)

}
