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

func TestContactChangeThroughActualCompanyHTTPAndContract(t *testing.T) {
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
	base := "/v1/orgs/" + orgID + "/employees"
	invites := "/v1/orgs/" + orgID + "/invitations"
	const personal = "/v1/org-access/invitations/"
	employee := call(base+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic contact employee", "work_email": "old@example.invalid", "work_phone": "+5511999999999"}, 201, "EnterpriseEmployeeAcknowledgement", nil)["employee_id"].(string)
	path := base + "/" + employee + "/change"
	issue := func() map[string]any {
		t.Helper()
		return call(invites+"/issue", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "DRIVER", "scope": "SELF"}}}, 200, "EnterpriseInvitationResult", nil)
	}
	first := issue()
	oldID := first["invitation"].(map[string]any)["id"].(string)
	oldCode := first["delivery"].(map[string]any)["code"].(string)
	managerCookie, managerCSRF := cookie, csrf
	personKey, e := accounts.CreateKeyAccount(t.Context(), "syncontactperson")
	if e != nil {
		t.Fatal(e)
	}
	person, e := accounts.LoginWithKey(t.Context(), personKey.Key)
	if e != nil {
		t.Fatal(e)
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	personCookie, personCSRF := cookie, csrf
	preview := call(personal+"preview", map[string]any{"code": oldCode}, 200, "EnterpriseInvitationPreview", nil)
	cookie, csrf = managerCookie, managerCSRF
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "display_name": "Synthetic contact employee", "work_email": "changed@example.invalid", "work_phone": "+5511999999999"}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		call(path, change, 401, "EnterpriseError", edit)
	}
	for _, extra := range []map[string]any{{"revision": nil}, {"revision": 0}, {"work_email": nil}, {"account_id": person.Account.ID}} {
		input := make(map[string]any)
		for k, v := range change {
			input[k] = v
		}
		for k, v := range extra {
			input[k] = v
		}
		call(path, input, 400, "EnterpriseError", nil)
	}
	call(path, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic contact employee"}, 400, "EnterpriseError", nil)
	call("/v1/orgs/"+other["organization_id"].(string)+"/employees/"+employee+"/change", change, 404, "EnterpriseError", nil)
	// A deferred receipt failure must also restore the actual pending invitation.
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_contact_http_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic private contact failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_contact_http_failure AFTER INSERT ON org_employee_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_contact_http_failure()`); e != nil {
		t.Fatal(e)
	}
	call(path, change, 503, "EnterpriseError", nil)
	if call(invites+"/"+oldID+"/read", map[string]any{}, 200, "EnterpriseInvitationView", nil)["state"] != "PENDING" {
		t.Fatal("failed HTTP contact revoked invitation")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_contact_http_failure ON org_employee_commands`); e != nil {
		t.Fatal(e)
	}
	ack := call(path, change, 200, "EnterpriseEmployeeAcknowledgement", nil)
	if ack["profile_revision"] != float64(2) || ack["identity_revision"] != float64(1) {
		t.Fatal("contact acknowledgement")
	}
	read := call(invites+"/"+oldID+"/read", map[string]any{}, 200, "EnterpriseInvitationView", nil)
	if read["state"] != "REVOKED" || read["cause"] != "CONTACT_CHANGED" || read["source"].(map[string]any)["revision"] != float64(2) {
		t.Fatal("HTTP contact source provenance")
	}
	cookie, csrf = personCookie, personCSRF
	invalid := call(personal+"accept", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "code": oldCode, "consent": true, "consent_digest": preview["consent_digest"]}, 404, "EnterpriseError", nil)
	if invalid["error"].(map[string]any)["code"] != "TOKEN_INVALID" {
		t.Fatal("old contact token refusal")
	}
	cookie, csrf = managerCookie, managerCSRF
	fresh := issue()
	freshID := fresh["invitation"].(map[string]any)["id"].(string)
	if retry := call(path, change, 200, "EnterpriseEmployeeAcknowledgement", nil); retry["replayed"] != true || retry["profile_revision"] != ack["profile_revision"] {
		t.Fatal("original HTTP contact retry")
	}
	if call(invites+"/"+freshID+"/read", map[string]any{}, 200, "EnterpriseInvitationView", nil)["state"] != "PENDING" {
		t.Fatal("old contact retry revoked newer invitation")
	}
	// A genuine DRIVER/SELF in both companies cannot edit private work contact.
	sourceFx := fx
	sourceFx.scope = scopeFor(t, orgID)
	sourceFx.member = org["membership_id"].(string)
	teamDriver(t, sourceFx, person.Account.ID)
	otherFx := fx
	otherFx.scope = scopeFor(t, other["organization_id"].(string))
	otherFx.member = other["membership_id"].(string)
	teamDriver(t, otherFx, person.Account.ID)
	cookie, csrf = personCookie, personCSRF
	call(path, change, 403, "EnterpriseError", nil)
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	call("/v1/org-access/session/read", map[string]any{}, 200, "", nil)
	var active int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_memberships WHERE account_id=$1 AND org_id IN($2,$3) AND state='ACTIVE'`, person.Account.ID, orgID, other["organization_id"].(string)).Scan(&active); e != nil || active != 2 {
		t.Fatal("contact changed independent memberships", e)
	}
	cookie, csrf = managerCookie, managerCSRF
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE identity_rate_windows SET count=200 WHERE subject_digest LIKE 'orgquota:v1:%'`); e != nil {
		t.Fatal(e)
	}
	call(path, change, 429, "EnterpriseError", nil)
	call(base+"/"+employee+"/read", map[string]any{}, 200, "EnterpriseEmployeeView", nil)
}
