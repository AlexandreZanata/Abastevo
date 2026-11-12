//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestEmployeeRegisterThroughComposedCompanyHTTP(t *testing.T) {
	fx, _ := employeeFixture(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synemployeehttp")
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
			t.Fatalf("employee HTTP policy/status mismatch: got %d want %d", response.Code, want)
		}
		if response.Code >= 400 && (strings.Contains(response.Body.String(), "Synthetic") || strings.Contains(response.Body.String(), "SYN-PRIVATE") || strings.Contains(response.Body.String(), "worker@example.invalid")) {
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
		result := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic employee HTTP", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201, nil)
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
	base := "/v1/orgs/" + org + "/employees"
	otherBase := "/v1/orgs/" + other["organization_id"].(string) + "/employees"
	empty := map[string]any{}
	call("/v1/orgs/"+org+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, nil)
	if items := call(base+"/list", empty, 200, nil)["items"].([]any); len(items) != 1 {
		t.Fatal("pending creator profile missing at setup")
	}
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic Private Worker", "registration_number": "SYN-PRIVATE", "work_email": "worker@example.invalid"}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, func(r *http.Request) { r.Header.Set("Signature", "synthetic") }} {
		call(base+"/create", create, 401, edit)
	}
	for _, field := range []string{"account_id", "employee_id", "state", "identity_revision", "profile_revision", "role", "permission", "cpf", "licence_number", "teams"} {
		create[field] = "synthetic"
		call(base+"/create", create, 400, nil)
		delete(create, field)
	}
	first := call(base+"/create", create, 201, nil)
	id := first["employee_id"].(string)
	path := base + "/" + id
	if first["identity_revision"] != float64(1) || first["profile_revision"] != float64(1) || first["state"] != "ACTIVE" {
		t.Fatal("initial register acknowledgement")
	}
	if retry := call(base+"/create", create, 201, nil); retry["replayed"] != true || retry["employee_id"] != id {
		t.Fatal("register retry")
	}
	record := call(path+"/read", empty, 200, nil)
	if record["display_name"] != create["display_name"] || record["work_email"] != create["work_email"] || record["profile_pending"] != false {
		t.Fatal("private profile view")
	}
	duplicate := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic Another", "registration_number": "syn-private", "allow_possible_duplicate": true}
	refusal := call(base+"/create", duplicate, 422, nil)
	if refusal["error"].(map[string]any)["code"] != "VALIDATION_FAILED" {
		t.Fatal("duplicate registration code")
	}
	possible := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "synthetic  private worker", "work_email": "WORKER@example.invalid"}
	review := call(base+"/create", possible, 409, nil)
	if review["error"].(map[string]any)["code"] != "DUPLICATE_REVIEW_REQUIRED" {
		t.Fatal("missing duplicate warning")
	}
	matches := call(base+"/matches", map[string]any{"display_name": possible["display_name"], "work_email": possible["work_email"], "limit": 1}, 200, nil)
	if len(matches["items"].([]any)) != 1 || matches["items"].([]any)[0].(map[string]any)["id"] != id {
		t.Fatal("scoped review candidate")
	}
	possible["allow_possible_duplicate"] = true
	second := call(base+"/create", possible, 201, nil)
	if second["employee_id"] == id {
		t.Fatal("automatic merge")
	}
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "display_name": "Synthetic Corrected Worker", "job_title": "Synthetic job"}
	changed := call(path+"/change", change, 200, nil)
	if changed["profile_revision"] != float64(2) || changed["identity_revision"] != float64(1) {
		t.Fatal("profile changed access identity")
	}
	call(path+"/change", change, 200, nil)
	change["operation_id"] = onboardingID(t)
	call(path+"/change", change, 409, nil)
	if old := call(base+"/create", create, 201, nil); old["profile_revision"] != float64(1) {
		t.Fatal("old acknowledgement drift")
	}
	creator := organization["employee_id"].(string)
	creatorPath := base + "/" + creator
	if call(creatorPath+"/read", empty, 200, nil)["profile_pending"] != true {
		t.Fatal("creator profile invented")
	}
	complete := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "display_name": "Synthetic Creator"}
	call(creatorPath+"/complete", complete, 200, nil)
	call(creatorPath+"/complete", complete, 200, nil)
	call(otherBase+"/"+id+"/read", empty, 404, nil)
	call(otherBase+"/"+onboardingID(t)+"/read", empty, 404, nil)
	foreignUnit := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic Foreign Home", "home_unit_id": other["unit_id"]}
	call(base+"/create", foreignUnit, 404, nil)
	page := call(base+"/list", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing bounded cursor")
	}
	call(base+"/list", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	call(otherBase+"/list", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	call(base+"/list", map[string]any{"limit": 2, "cursor": cursor}, 400, nil)
	// Every currently persistable fixed role uses genuine proof and membership.
	// STATION roles still require their later authority-scoped admission task.
	ownerCookie, ownerCSRF := cookie, csrf
	roles := []struct {
		name         string
		read, manage bool
	}{{"ADMIN", true, true}, {"FLEET_MANAGER", true, false}, {"SUPERVISOR", true, false}, {"FINANCE", true, false}, {"APPROVER", true, false}, {"AUDITOR", true, false}, {"DRIVER", false, false}, {"OPERATIONS_STAFF", false, false}, {"DATA_MANAGER", false, false}}
	for i, role := range roles {
		k, e := accounts.CreateKeyAccount(t.Context(), "employeerole"+fmt.Sprint(i))
		if e != nil {
			t.Fatal(e)
		}
		a, e := accounts.LoginWithKey(t.Context(), k.Key)
		if e != nil {
			t.Fatal(e)
		}
		member, employeeID := onboardingID(t), onboardingID(t)
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, []any{org, employeeID}},
			{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, member, employeeID, a.Account.ID}},
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,'ORG',$1,clock_timestamp()-interval '1 minute',$5,'synthetic employee matrix')`, []any{org, onboardingID(t), member, role.name, organization["membership_id"].(string)}},
			{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, member}},
		} {
			if e := unitSchemaExec(t, sourceFx, q.sql, q.args...); e != nil {
				t.Fatal("matrix fixture", role.name, e)
			}
		}
		cookie = nil
		csrf = ""
		call("/v1/org-access/session/login", map[string]any{"family_id": a.Session.FamilyID, "access_token": a.Session.AccessToken}, 200, nil)
		readStatus, manageStatus := 403, 403
		if role.read {
			readStatus = 200
		}
		if role.manage {
			manageStatus = 201
		}
		call(path+"/read", empty, readStatus, nil)
		call(base+"/list", empty, readStatus, nil)
		command := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic role " + role.name}
		call(base+"/create", command, manageStatus, nil)
		matchStatus := 403
		if role.manage {
			matchStatus = 200
		}
		call(base+"/matches", map[string]any{"display_name": "Synthetic Private Worker", "work_email": "worker@example.invalid"}, matchStatus, nil)
	}
	cookie, csrf = ownerCookie, ownerCSRF
	if _, err := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	call(path+"/read", empty, 200, nil)
	call(base+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "display_name": "Synthetic Disabled"}, 403, nil)
	if _, err := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true, ExpectedRevision: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_employee_http_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic receipt failure';END $$;CREATE CONSTRAINT TRIGGER synthetic_employee_http_failure AFTER INSERT ON org_employee_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_employee_http_failure()`); err != nil {
		t.Fatal(err)
	}
	failed := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "display_name": "Synthetic Failed HTTP"}
	call(path+"/change", failed, 503, nil)
	if call(path+"/read", empty, 200, nil)["profile_revision"] != float64(2) {
		t.Fatal("failed HTTP commit mutated profile")
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_employee_http_failure ON org_employee_commands`); err != nil {
		t.Fatal(err)
	}
	// Revoking the actual source family denies subsequent reads and old retries.
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, auth.Session.FamilyID); err != nil {
		t.Fatal(err)
	}
	call(path+"/read", empty, 401, nil)
	call(base+"/create", create, 401, nil)
}
