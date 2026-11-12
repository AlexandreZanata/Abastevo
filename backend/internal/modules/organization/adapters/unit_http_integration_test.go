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

func TestOperatingUnitsThroughComposedCompanyHTTP(t *testing.T) {
	fx, _ := operatingUnitFixture(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticunithttp")
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
			t.Fatal("private request encoding failed")
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
			t.Fatalf("unit HTTP policy/status mismatch: got %d want %d", response.Code, want)
		}
		if response.Code >= 400 && (strings.Contains(response.Body.String(), "Synthetic") || strings.Contains(response.Body.String(), "private unit")) {
			t.Fatal("private unit content entered error")
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private response JSON")
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
		result := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic HTTP units", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201, nil)
		id := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, id), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		return result
	}
	organization := createOrg()
	other := createOrg()
	org := organization["organization_id"].(string)
	mainID := organization["unit_id"].(string)
	scope := scopeFor(t, org)
	sourceFx := fx
	sourceFx.scope = scope
	base := "/v1/orgs/" + org + "/units"
	otherBase := "/v1/orgs/" + other["organization_id"].(string) + "/units"
	empty := map[string]any{}
	call("/v1/orgs/"+org+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, nil)
	legal := call("/v1/orgs/"+org+"/legal-entities/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "COMPANY", "legal_name": "Synthetic scoped party", "identifier": "EXEMPL01000170"}, 201, nil)
	establishment := legal["establishment_id"].(string)
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic private unit", "code": "N01", "establishment_id": establishment}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, func(r *http.Request) { r.Header.Set("Signature", "synthetic") }} {
		call(base+"/create", create, 401, edit)
	}
	for _, field := range []string{"account_id", "unit_id", "state", "active_units", "usage", "permission"} {
		create[field] = "synthetic"
		call(base+"/create", create, 400, nil)
		delete(create, field)
	}
	created := call(base+"/create", create, 201, nil)
	id := created["unit_id"].(string)
	path := base + "/" + id
	if created["state"] != "ACTIVE" || created["replayed"] != false {
		t.Fatal("unit creation acknowledgement")
	}
	if replay := call(base+"/create", create, 201, nil); replay["unit_id"] != id || replay["replayed"] != true {
		t.Fatal("HTTP creation replay changed identity")
	}
	view := call(path+"/read", empty, 200, nil)
	if view["time_zone"] != "America/Cuiaba" || view["establishment_id"] != establishment || view["name"] != "Synthetic private unit" {
		t.Fatal("captured organization zone/link lost")
	}
	duplicate := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "synthetic private unit", "code": "OTHER"}
	refusal := call(base+"/create", duplicate, 422, nil)
	detail := refusal["error"].(map[string]any)["details"].([]any)[0].(map[string]any)
	if detail["field"] != "name" || detail["code"] != "DUPLICATE" || len(detail) != 2 {
		t.Fatal("unsafe active name detail")
	}
	duplicate["operation_id"] = onboardingID(t)
	duplicate["name"] = "Synthetic other name"
	duplicate["code"] = "n01"
	if detail := call(base+"/create", duplicate, 422, nil)["error"].(map[string]any)["details"].([]any)[0].(map[string]any); detail["field"] != "code" {
		t.Fatal("active code error mislabelled")
	}
	duplicate["operation_id"] = onboardingID(t)
	duplicate["code"] = "OTHER"
	duplicate["time_zone"] = "GMT"
	call(base+"/create", duplicate, 422, nil)
	duplicate["time_zone"] = "UTC"
	duplicate["establishment_id"] = onboardingID(t)
	call(base+"/create", duplicate, 404, nil)
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "name": "Changed private unit", "code": "N01", "time_zone": "UTC", "establishment_id": establishment}
	if call(path+"/change", change, 200, nil)["revision"] != float64(2) {
		t.Fatal("unit edit revision")
	}
	if call(base+"/create", create, 201, nil)["revision"] != float64(1) {
		t.Fatal("creation retry returned current rather than original result")
	}
	if call(path+"/change", change, 200, nil)["replayed"] != true {
		t.Fatal("unit metadata retry")
	}
	change["name"] = "Different intent"
	call(path+"/change", change, 409, nil)
	change["operation_id"] = onboardingID(t)
	call(path+"/change", change, 409, nil)
	if call(path+"/read", empty, 200, nil)["time_zone"] != "UTC" {
		t.Fatal("new unit zone not captured")
	}
	page := call(base+"/list", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing unit cursor")
	}
	next := call(base+"/list", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	if len(next["items"].([]any)) != 1 || next["next_cursor"] != "" {
		t.Fatal("unit keyset lost a row")
	}
	call(base+"/list", map[string]any{"limit": 2, "cursor": cursor}, 400, nil)
	call(base+"/list", map[string]any{"limit": 1, "state": "ACTIVE", "cursor": cursor}, 400, nil)
	call(otherBase+"/list", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	call(otherBase+"/"+id+"/read", empty, 404, nil)
	call(otherBase+"/"+onboardingID(t)+"/read", empty, 404, nil)
	call("/v1/orgs/"+fx.scope.OrgID().String()+"/units/list", empty, 404, nil)
	archive := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}
	archived := call(path+"/archive", archive, 200, nil)
	if archived["state"] != "ARCHIVED" || archived["revision"] != float64(3) {
		t.Fatal("archive outcome")
	}
	if call(path+"/archive", archive, 200, nil)["replayed"] != true {
		t.Fatal("archive retry")
	}
	if archivedView := call(path+"/read", empty, 200, nil); !strings.HasSuffix(archivedView["display_name"].(string), " (arquivada)") {
		t.Fatal("archive display discarded identity")
	}
	restore := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3}
	if call(path+"/restore", restore, 200, nil)["revision"] != float64(4) || call(path+"/restore", restore, 200, nil)["replayed"] != true {
		t.Fatal("restore outcome/replay")
	}
	mainPath := base + "/" + mainID
	call(mainPath+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, nil)
	last := call(path+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 409, nil)
	if last["error"].(map[string]any)["code"] != "LAST_UNIT" {
		t.Fatal("last-unit protection absent from HTTP")
	}
	call(mainPath+"/restore", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}, 200, nil)
	// This is a disposable owner migration contract, not a production fleet module.
	installSyntheticUnitUsageSource(t, sourceFx)
	for _, kind := range []string{"ASSET", "EMPLOYEE"} {
		if err := unitSchemaExec(t, sourceFx, `INSERT INTO synthetic_unit_work(org_id,unit_id,id,kind,state) VALUES($1,$2,$3,$4,'ACTIVE')`, org, id, onboardingID(t), kind); err != nil {
			t.Fatal(err)
		}
	}
	use := call(path+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 409, nil)["error"].(map[string]any)
	counts := use["details"].([]any)
	if use["code"] != "STRUCTURE_IN_USE" || len(counts) != 4 || counts[0].(map[string]any)["count"] != "1" || counts[1].(map[string]any)["count"] != "1" || counts[2].(map[string]any)["count"] != "0" {
		t.Fatal("HTTP use counts lost or leaked source records")
	}
	if err := unitSchemaExec(t, sourceFx, `UPDATE synthetic_unit_work SET state='HISTORY' WHERE org_id=$1 AND unit_id=$2`, org, id); err != nil {
		t.Fatal(err)
	}
	call(path+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 4}, 200, nil)
	// A live DRIVER cookie must not acquire the owner's management permission.
	driverKey, err := accounts.CreateKeyAccount(t.Context(), "syntheticunitdriver")
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
	if err := unitSchemaExec(t, sourceFx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'DRIVER','SELF',$3,clock_timestamp()-interval '1 minute',$4,'synthetic driver fixture')`, org, onboardingID(t), member, organization["membership_id"].(string)); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, sourceFx, `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, member); err != nil {
		t.Fatal(err)
	}
	ownerCookie, ownerCSRF := cookie, csrf
	cookie = nil
	csrf = ""
	call("/v1/org-access/session/login", map[string]any{"family_id": driver.Session.FamilyID, "access_token": driver.Session.AccessToken}, 200, nil)
	denied := call(path+"/read", empty, 403, nil)["error"].(map[string]any)
	if denied["details"].([]any)[0].(map[string]any)["code"] != "org.structure.manage" {
		t.Fatal("driver permission denial lost")
	}
	call(base+"/create", create, 403, nil)
	cookie, csrf = ownerCookie, ownerCSRF
	// A deferred receipt failure cannot expose the provisional new unit.
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_http_unit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic unit receipt outage';END $$;
 CREATE CONSTRAINT TRIGGER synthetic_http_unit_failure AFTER INSERT ON org_unit_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_http_unit_failure()`); err != nil {
		t.Fatal(err)
	}
	rollback := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic rollback unit", "code": "ROLLBACK"}
	call(base+"/create", rollback, 503, nil)
	var rows int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_units WHERE org_id=$1 AND code='ROLLBACK'`, org).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("failed HTTP commit retained unit", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_http_unit_failure ON org_unit_commands`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	call(base+"/create", rollback, 403, nil)
	call(path+"/read", empty, 200, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call(path+"/read", empty, 200, nil)
	call(base+"/create", rollback, 401, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call(path+"/read", empty, 401, nil)
}
