//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestRedemptionThroughActualCompanyHTTPAndContract(t *testing.T) {
	fx, _ := employeeFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+role+";GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); e != nil {
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

	organization := createOrg()
	orgID := organization["organization_id"].(string)
	base := "/v1/orgs/" + orgID + "/invitations"
	personal := "/v1/org-access/invitations/"
	managerCookie, managerCSRF := cookie, csrf
	issue := func(role string) (string, string) {
		t.Helper()
		employee := call("/v1/orgs/"+orgID+"/employees/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic invitation recipient"}, 201, "", nil)["employee_id"].(string)
		result := call(base+"/issue", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": role, "scope": "SELF"}}}, 200, "EnterpriseInvitationResult", nil)
		return result["invitation"].(map[string]any)["id"].(string), result["delivery"].(map[string]any)["code"].(string)
	}
	id, code := issue("AUDITOR")
	key, e = accounts.CreateKeyAccount(t.Context(), "synrecipient")
	if e != nil {
		t.Fatal(e)
	}
	person, e := accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": person.Session.FamilyID, "access_token": person.Session.AccessToken}, 200, "", nil)
	personCookie, personCSRF := cookie, csrf
	input := map[string]any{"code": code}
	for i, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		want := 403
		if i == 0 {
			want = 401
		}
		call(personal+"preview", input, want, "EnterpriseError", edit)
	}
	for _, bad := range []map[string]any{{"Code": code}, {"code": nil}, {"code": code, "account_id": person.Account.ID}, {"code": code, "access_token": "synthetic"}} {
		call(personal+"preview", bad, 400, "EnterpriseError", nil)
	}
	preview := call(personal+"preview", input, 200, "EnterpriseInvitationPreview", nil)
	if preview["confirmation_required"] != true || preview["organization_id"] != nil || preview["employee_id"] != nil || preview["organization"].(map[string]any)["legal_entity_verified"] != false {
		t.Fatal("private informed preview")
	}
	accept := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "code": code, "consent": true, "consent_digest": preview["consent_digest"]}
	waiting := call(personal+"accept", accept, 200, "EnterpriseInvitationRedemptionResult", nil)
	if waiting["membership_state"] != "INVITED" {
		t.Fatal("privileged access before confirmation")
	}
	if call(personal+"accept", accept, 200, "EnterpriseInvitationRedemptionResult", nil)["replayed"] != true {
		t.Fatal("HTTP personal retry")
	}
	call(personal+"preview", input, 404, "EnterpriseError", nil)
	call(base+"/"+id+"/confirm", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 404, "EnterpriseError", nil)
	cookie, csrf = managerCookie, managerCSRF
	read := call(base+"/"+id+"/acceptance/read", map[string]any{}, 200, "EnterpriseInvitationAcceptanceView", nil)
	if read["state"] != "WAITING" || read["account_alias"] != person.Account.Alias {
		t.Fatal("manager alias confirmation")
	}
	missingRevision := map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}
	call(base+"/"+id+"/confirm", missingRevision, 400, "EnterpriseError", nil)
	confirm := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	if call(base+"/"+id+"/confirm", confirm, 200, "EnterpriseInvitationAcceptanceView", nil)["state"] != "ACTIVE" {
		t.Fatal("HTTP confirmation")
	}
	if call(base+"/"+id+"/confirm", confirm, 200, "EnterpriseInvitationAcceptanceView", nil)["replayed"] != true {
		t.Fatal("HTTP confirmation retry")
	}
	// The optional policy is captured prospectively without rewriting old settings.
	settingsBase := "/v1/orgs/" + orgID + "/settings"
	settings := call(settingsBase+"/read", map[string]any{}, 200, "", nil)
	values := settings["values"].(map[string]any)
	values["policies"].(map[string]any)["require_invitation_confirmation"] = true
	call(settingsBase+"/change", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": settings["revision"], "values": values, "reason": "Synthetic confirmation policy"}, 200, "", nil)
	id2, code2 := issue("DRIVER")
	key, e = accounts.CreateKeyAccount(t.Context(), "synrecipienttwo")
	if e != nil {
		t.Fatal(e)
	}
	two, e := accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": two.Session.FamilyID, "access_token": two.Session.AccessToken}, 200, "", nil)
	p2 := call(personal+"preview", map[string]any{"code": code2}, 200, "EnterpriseInvitationPreview", nil)
	if p2["confirmation_required"] != true {
		t.Fatal("organization confirmation policy ignored")
	}
	call(personal+"accept", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "code": code2, "consent": true, "consent_digest": p2["consent_digest"]}, 200, "EnterpriseInvitationRedemptionResult", nil)
	cookie, csrf = managerCookie, managerCSRF
	if call(base+"/"+id2+"/reject", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, "EnterpriseInvitationAcceptanceView", nil)["state"] != "REJECTED" {
		t.Fatal("HTTP rejection")
	}
	cookie, csrf = personCookie, personCSRF
	for i := 0; i < 11; i++ {
		want := 404
		if i == 10 {
			want = 429
		}
		failure := call(personal+"preview", map[string]any{"code": "AIN1." + onboardingID(t) + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, want, "EnterpriseError", func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113."+fmt.Sprint(i+1)) })
		if i < 10 && failure["error"].(map[string]any)["code"] != "TOKEN_INVALID" {
			t.Fatal("nonuniform unknown token")
		}
	}
	var dump string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)) FROM org_invitation_person_commands x),(SELECT jsonb_agg(to_jsonb(x))FROM org_invitation_acceptances x),(SELECT jsonb_agg(to_jsonb(x))FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x))FROM identity_rate_windows x))::text`).Scan(&dump); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(dump, code) || strings.Contains(dump, code2) || strings.Contains(dump, "192.0.2.1") || strings.Contains(dump, "203.0.113.") {
		t.Fatal("token or raw network persisted")
	}
}
