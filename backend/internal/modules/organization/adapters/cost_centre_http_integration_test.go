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

func TestCostCentresThroughComposedCompanyHTTP(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticcosthttp")
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
			t.Fatalf("cost-centre HTTP policy/status mismatch: got %d want %d", response.Code, want)
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
	base := "/v1/orgs/" + org + "/cost-centres"
	otherBase := "/v1/orgs/" + other["organization_id"].(string) + "/cost-centres"
	empty := map[string]any{}
	call("/v1/orgs/"+org+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, nil)
	if items := call(base+"/list", empty, 200, nil)["items"].([]any); len(items) != 0 {
		t.Fatal("cost centre incorrectly required at setup")
	}
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic private centre", "code": "PRIVATE-CODE"}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, func(r *http.Request) { r.Header.Set("Signature", "synthetic") }} {
		call(base+"/create", create, 401, edit)
	}
	for _, field := range []string{"account_id", "cost_centre_id", "state", "usage", "code_key", "permission"} {
		create[field] = "synthetic"
		call(base+"/create", create, 400, nil)
		delete(create, field)
	}
	first := call(base+"/create", create, 201, nil)
	id := first["cost_centre_id"].(string)
	path := base + "/" + id
	if first["state"] != "ACTIVE" || first["replayed"] != false {
		t.Fatal("centre create acknowledgement")
	}
	if repeated := call(base+"/create", create, 201, nil); repeated["replayed"] != true || repeated["cost_centre_id"] != id {
		t.Fatal("centre HTTP creation replay")
	}
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "name": "Synthetic corrected centre", "code": "CORRECTED"}
	if call(path+"/change", change, 200, nil)["revision"] != float64(2) {
		t.Fatal("centre code correction revision")
	}
	if call(path+"/change", change, 200, nil)["replayed"] != true {
		t.Fatal("centre correction replay")
	}
	for _, code := range []string{" private-code ", "corrected"} {
		value := call(base+"/code/read", map[string]any{"code": code}, 200, nil)
		if value["id"] != id || value["code"] != "CORRECTED" {
			t.Fatal("historical HTTP code remapped")
		}
	}
	if call(base+"/create", create, 201, nil)["revision"] != float64(1) {
		t.Fatal("creation retry returned current instead of original acknowledgement")
	}
	change["name"] = "Different intent"
	call(path+"/change", change, 409, nil)
	change["operation_id"] = onboardingID(t)
	call(path+"/change", change, 409, nil)
	duplicate := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic other centre", "code": "private-code"}
	refusal := call(base+"/create", duplicate, 422, nil)["error"].(map[string]any)
	detail := refusal["details"].([]any)[0].(map[string]any)
	if detail["field"] != "code" || detail["code"] != "DUPLICATE" || len(detail) != 2 {
		t.Fatal("historical code refusal leaked content")
	}
	foreign := call(otherBase+"/create", duplicate, 201, nil)
	if foreign["cost_centre_id"] == id {
		t.Fatal("foreign code reused identity")
	}
	call(otherBase+"/"+id+"/read", empty, 404, nil)
	call(otherBase+"/"+onboardingID(t)+"/read", empty, 404, nil)
	call("/v1/orgs/"+fx.scope.OrgID().String()+"/cost-centres/list", empty, 404, nil)
	// Same name is permitted for another stable code.
	duplicate["operation_id"] = onboardingID(t)
	duplicate["name"] = "Synthetic corrected centre"
	duplicate["code"] = "SECOND"
	call(base+"/create", duplicate, 201, nil)
	page := call(base+"/list", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing centre next cursor")
	}
	next := call(base+"/list", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	if len(next["items"].([]any)) != 1 || next["next_cursor"] != "" {
		t.Fatal("centre keyset lost record")
	}
	call(base+"/list", map[string]any{"limit": 2, "cursor": cursor}, 400, nil)
	call(base+"/list", map[string]any{"limit": 1, "state": "ACTIVE", "cursor": cursor}, 400, nil)
	call(otherBase+"/list", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	archive := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}
	if call(path+"/archive", archive, 200, nil)["state"] != "ARCHIVED" || call(path+"/archive", archive, 200, nil)["replayed"] != true {
		t.Fatal("centre archive/retry")
	}
	if view := call(base+"/code/read", map[string]any{"code": "PRIVATE-CODE"}, 200, nil); view["id"] != id || !strings.HasSuffix(view["display_name"].(string), " (arquivada)") {
		t.Fatal("archived alias/display lost stable meaning")
	}
	duplicate["operation_id"] = onboardingID(t)
	duplicate["code"] = "CORRECTED"
	call(base+"/create", duplicate, 422, nil)
	restore := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3}
	if call(path+"/restore", restore, 200, nil)["revision"] != float64(4) {
		t.Fatal("centre restore failed")
	}
	installSyntheticCostCentreSource(t, sourceFx)
	work := onboardingID(t)
	if err := unitSchemaExec(t, sourceFx, `INSERT INTO synthetic_cost_work(org_id,centre_id,id,kind,state) VALUES($1,$2,$3,'BUDGET','ACTIVE')`, org, id, work); err != nil {
		t.Fatal(err)
	}
	inUse := call(path+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 409, nil)["error"].(map[string]any)
	details := inUse["details"].([]any)
	if inUse["code"] != "STRUCTURE_IN_USE" || len(details) != 4 || details[2].(map[string]any)["count"] != "1" {
		t.Fatal("centre current usage counts lost")
	}
	if err := unitSchemaExec(t, sourceFx, `UPDATE synthetic_cost_work SET state='HISTORY' WHERE org_id=$1 AND id=$2`, org, work); err != nil {
		t.Fatal(err)
	}
	call(path+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 200, nil)
	// DRIVER has a genuine proof/session and active membership, without management coverage.
	driverKey, err := accounts.CreateKeyAccount(t.Context(), "syntheticcostdriver")
	if err != nil {
		t.Fatal(err)
	}
	driver, err := accounts.LoginWithKey(t.Context(), driverKey.Key)
	if err != nil {
		t.Fatal(err)
	}
	member, employee := onboardingID(t), onboardingID(t)
	if err := unitSchemaExec(t, sourceFx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employee); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, sourceFx, `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, org, member, employee, driver.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, sourceFx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'DRIVER','SELF',$3,clock_timestamp()-interval '1 minute',$4,'synthetic cost driver')`, org, onboardingID(t), member, organization["membership_id"].(string)); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, sourceFx, `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, member); err != nil {
		t.Fatal(err)
	}
	ownerCookie, ownerCSRF := cookie, csrf
	cookie = nil
	csrf = ""
	call("/v1/org-access/session/login", map[string]any{"family_id": driver.Session.FamilyID, "access_token": driver.Session.AccessToken}, 200, nil)
	call(path+"/read", empty, 403, nil)
	call(base+"/code/read", map[string]any{"code": "PRIVATE-CODE"}, 403, nil)
	call(base+"/create", create, 403, nil)
	cookie, csrf = ownerCookie, ownerCSRF
	// A failed correction must not reserve a new code or publish provisional metadata.
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_http_cost_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic cost receipt outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_http_cost_failure AFTER INSERT ON org_cost_centre_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_http_cost_failure()`); err != nil {
		t.Fatal(err)
	}
	correction := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 5, "name": "Synthetic failed correction", "code": "ROLLBACK"}
	call(path+"/change", correction, 503, nil)
	call(base+"/code/read", map[string]any{"code": "ROLLBACK"}, 404, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_http_cost_failure ON org_cost_centre_commands`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	call(path+"/change", correction, 403, nil)
	call(base+"/code/read", map[string]any{"code": "PRIVATE-CODE"}, 200, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call(path+"/read", empty, 200, nil)
	call(path+"/change", correction, 401, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call(path+"/read", empty, 401, nil)
}
