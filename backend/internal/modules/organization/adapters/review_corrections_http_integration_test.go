//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	orgd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// BUC-EMP-89/90: the actual company HTTP wire for an erased acceptance and for
// a suspended organization's contexts matches the additive OpenAPI shapes.
func TestReviewCorrectionsActualCompanyHTTPMatchesContract(t *testing.T) {
	fx, _ := employeeFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+role+";GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); e != nil {
		t.Fatal(e)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	login := func(alias string) account.AuthResult {
		t.Helper()
		key, e := accounts.CreateKeyAccount(t.Context(), alias)
		if e != nil {
			t.Fatal(e)
		}
		auth, e := accounts.LoginWithKey(t.Context(), key.Key)
		if e != nil {
			t.Fatal(e)
		}
		return auth
	}
	manager, person := login("synreviewmanager"), login("synreviewperson")
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
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{57}, 32), Pool: fx.f.Runtime, Accounts: accounts}); e != nil {
		t.Fatal(e)
	}
	var cookie *http.Cookie
	csrf := ""
	call := func(path string, body any, want int, schema string) map[string]any {
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
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("review correction HTTP %s: got%d want%d", path, response.Code, want)
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("private response JSON")
		}
		if schema != "" {
			if e := document.Components.Schemas[schema].Value.VisitJSON(result); e != nil {
				t.Fatal("actual wire violates contract", schema, e)
			}
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookie, csrf = response.Result().Cookies()[0], result["csrf_token"].(string)
		}
		return result
	}
	session := func(auth account.AuthResult) {
		t.Helper()
		call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, "")
	}
	session(manager)
	created := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic review company", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": orgd.SyntheticTermsVersion, "digest": orgd.SyntheticTermsDigest, "accepted": true}}, 201, "")
	org, member := created["organization_id"].(string), created["membership_id"].(string)
	scope := scopeFor(t, org)
	if _, e := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: manager.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); e != nil {
		t.Fatal(e)
	}
	base := "/v1/orgs/" + org + "/invitations"
	employee := call("/v1/orgs/"+org+"/employees/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic review recipient"}, 201, "")["employee_id"].(string)
	issued := call(base+"/issue", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "employee_id": employee, "grants": []any{map[string]any{"role": "AUDITOR", "scope": "SELF"}}}, 200, "EnterpriseInvitationResult")
	id, code := issued["invitation"].(map[string]any)["id"].(string), issued["delivery"].(map[string]any)["code"].(string)
	managerCookie, managerCSRF := cookie, csrf
	session(person)
	preview := call("/v1/org-access/invitations/preview", map[string]any{"code": code}, 200, "EnterpriseInvitationPreview")
	call("/v1/org-access/invitations/accept", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "code": code, "consent": true, "consent_digest": preview["consent_digest"]}, 200, "EnterpriseInvitationRedemptionResult")
	cookie, csrf = managerCookie, managerCSRF
	read := call(base+"/"+id+"/acceptance/read", map[string]any{}, 200, "EnterpriseInvitationAcceptanceView")
	if read["account_alias"] != person.Account.Alias || read["account_erased"] != false {
		t.Fatal("live acceptance wire")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+role); e != nil {
		t.Fatal(e)
	}
	if e := invitationDeleteAccount(t, fx.f, person.Account.ID); e != nil {
		t.Fatal("person account deletion", e)
	}
	read = call(base+"/"+id+"/acceptance/read", map[string]any{}, 200, "EnterpriseInvitationAcceptanceView")
	if read["account_alias"] != "" || read["account_erased"] != true || read["state"] != "ENDED" {
		t.Fatal("erased acceptance wire discloses the alias")
	}
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, scope, func(tx pgx.Tx) error {
		_, e := tx.Exec(t.Context(), `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, org)
		if e == nil {
			_, e = tx.Exec(t.Context(), `UPDATE organizations SET state='SUSPENDED',suspension_reason='BILLING',revision=revision+1 WHERE org_id=$1`, org)
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	page := call("/v1/org-access/contexts/list", map[string]any{}, 200, "EnterpriseAccountContextListResult")
	organizations := page["organizations"].([]any)
	if len(organizations) != 1 || organizations[0].(map[string]any)["state"] != "SUSPENDED" || organizations[0].(map[string]any)["suspension_reason"] != "BILLING" {
		t.Fatal("suspended organization context wire")
	}
	view := call("/v1/org-access/contexts/read", map[string]any{"organization_id": org, "membership_id": member}, 200, "EnterpriseAccountContextView")
	if view["organization"].(map[string]any)["state"] != "SUSPENDED" || len(view["invitation_grants"].([]any)) != 0 {
		t.Fatal("suspended organization read wire")
	}
	for _, p := range view["permissions"].([]any) {
		if p != "idn.employee.read" {
			t.Fatal("suspended organization advertised a write control", p)
		}
	}
}
