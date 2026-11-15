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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// BUC-EMP-124/125 through the actual composed company-cookie entry: commands,
// reads, the SELF scope on assigned assets and AUTH-006 unit scope.
func TestFleetAssignmentsThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = assignmentStore(t, fx)
	_ = meterStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synassignmenthttp")
	if err != nil {
		t.Fatal("synthetic account creation failed")
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal("synthetic account login failed")
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{7}, 32), Pool: fx.f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal("actual assignment composition failed", err)
	}
	var cookie *http.Cookie
	csrf, requests := "", 0
	call := func(label, path string, body any, want int) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
			r.Header.Set("X-CSRF", csrf)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		requests++
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("%s: private assignment HTTP got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && strings.Contains(w.Body.String(), "Synthetic") {
			t.Fatal("private assignment content entered refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private assignment response")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookie = w.Result().Cookies()[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	code := func(v map[string]any) string { return v["error"].(map[string]any)["code"].(string) }
	login := func(familyID, token string) {
		cookie, csrf = nil, ""
		call("company login", "/v1/org-access/session/login", map[string]any{"family_id": familyID, "access_token": token}, 200)
	}
	login(auth.Session.FamilyID, auth.Session.AccessToken)
	createOrg := func() map[string]any {
		t.Helper()
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic assignment organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic Norte", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
		id := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, id), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		call("organization activation", "/v1/orgs/"+id+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return result
	}
	organization, other := createOrg(), createOrg()
	org, otherOrg := organization["organization_id"].(string), other["organization_id"].(string)
	inScope := func(scope string, sql string, args ...any) error {
		return database.WithOrgTx(t.Context(), fx.f.Runtime, scopeFor(t, scope), func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
	}
	norte, sul := organization["unit_id"].(string), onboardingID(t)
	if err := inScope(org, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic Sul','SUL','UTC','ACTIVE')`, org, sul); err != nil {
		t.Fatal(err)
	}
	base, otherBase := "/v1/orgs/"+org+"/fleet/assets", "/v1/orgs/"+otherOrg+"/fleet/assets"
	asset := func(prefix, unit, plate string) string {
		t.Helper()
		created := call("draft", prefix+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic assigned implement", "plate": plate, "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic assignment category"}, "unit_id": unit}, 201)
		id := created["asset_id"].(string)
		call("activate", prefix+"/"+id+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return id
	}
	v, w, v2 := asset(base, norte, "ASG1A23"), asset(base, sul, "ASG1B23"), asset(base, norte, "ASG1C23")
	foreign := asset(otherBase, other["unit_id"].(string), "ASG1A23")
	// Real members: an account, an ACTIVE employee record, a membership and
	// one role assignment at the requested scope.
	type person struct{ family, token, employee, member string }
	member := func(scope string, roleName, scopeType, target string) person {
		t.Helper()
		key, err := accounts.CreateKeyAccount(t.Context(), "synasg"+strings.ToLower(roleName[:3])+onboardingID(t)[:6])
		if err != nil {
			t.Fatal(err)
		}
		login, err := accounts.LoginWithKey(t.Context(), key.Key)
		if err != nil {
			t.Fatal(err)
		}
		membership, employee := onboardingID(t), onboardingID(t)
		if scopeType == "SELF" {
			target = membership
		}
		owner := organization["membership_id"]
		if scope == otherOrg {
			owner = other["membership_id"]
		}
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{scope, employee}},
			{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{scope, membership, employee, login.Account.ID}},
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()-interval '1 minute',$7,'Synthetic assignment member')`, []any{scope, onboardingID(t), membership, roleName, scopeType, target, owner}},
			{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{scope, membership}},
		} {
			if err := inScope(scope, statement.sql, statement.args...); err != nil {
				t.Fatal(roleName, "synthetic membership setup failed", err)
			}
		}
		return person{login.Session.FamilyID, login.Session.AccessToken, employee, membership}
	}
	driver := member(org, "DRIVER", "SELF", "")
	norteSupervisor := member(org, "SUPERVISOR", "UNIT", norte)
	sulSupervisor := member(org, "SUPERVISOR", "UNIT", sul)
	foreignDriver := member(otherOrg, "DRIVER", "SELF", "")

	assign := func(label, assetID string, body map[string]any, want int) map[string]any {
		t.Helper()
		body["schema_version"] = 1
		if body["operation_id"] == nil {
			body["operation_id"] = onboardingID(t)
		}
		return call(label, base+"/"+assetID+"/assignments/create", body, want)
	}
	// Owner (ORG) assigns the driver as primary of V; replay; overlap; AC3.
	primary := map[string]any{"operation_id": onboardingID(t), "kind": "PRIMARY", "employee_ids": []string{driver.employee}, "reason": "Synthetic primary driver"}
	created := assign("primary", v, primary, 201)
	ids := created["assignment_ids"].([]any)
	if len(ids) != 1 || created["replayed"] != false || created["asset_id"] != v || created["data_mode"] != "SYNTHETIC" || created["replaced_assignment_id"] != nil {
		t.Fatal("primary acknowledgement", created)
	}
	if again := assign("primary replay", v, primary, 201); again["replayed"] != true || again["assignment_ids"].([]any)[0] != ids[0] {
		t.Fatal("primary replay", again)
	}
	if c := code(assign("overlapping primary", v, map[string]any{"kind": "PRIMARY", "employee_ids": []string{norteSupervisor.employee}}, 409)); c != "ASSIGNMENT_OVERLAP" {
		t.Fatal("overlap code", c)
	}
	if c := code(assign("temporary without end", v, map[string]any{"kind": "TEMPORARY", "employee_ids": []string{norteSupervisor.employee}}, 422)); c != "VALIDATION_FAILED" {
		t.Fatal("temporary code", c)
	}
	assign("employee of another organization", v, map[string]any{"kind": "ADDITIONAL", "employee_ids": []string{foreignDriver.employee}}, 404)
	assign("caller-selected state", v, map[string]any{"kind": "ADDITIONAL", "employee_ids": []string{norteSupervisor.employee}, "state": "ACTIVE"}, 400)
	call("foreign asset hidden", base+"/"+foreign+"/assignments/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "ADDITIONAL", "employee_ids": []string{driver.employee}}, 404)
	call("asset hidden from another organization", otherBase+"/"+v+"/assignments/list", map[string]any{}, 404)
	page := call("asset assignments", base+"/"+v+"/assignments/list", map[string]any{"limit": 10}, 200)
	items := page["items"].([]any)
	if len(items) != 1 || page["next_cursor"] != "" || page["evaluated_at"] == nil {
		t.Fatal("asset assignment page", page)
	}
	item := items[0].(map[string]any)
	if item["assignment_id"] != ids[0] || item["employee_id"] != driver.employee || item["kind"] != "PRIMARY" || item["state"] != "ACTIVE" || item["valid_to"] != nil || item["reason"] != "Synthetic primary driver" {
		t.Fatal("assignment projection", item)
	}

	// The DRIVER's SELF grant covers exactly the asset assigned to it.
	login(driver.family, driver.token)
	call("driver reads the assigned asset", base+"/"+v+"/read", map[string]any{}, 200)
	call("driver reads its assignment history", base+"/"+v+"/assignments/list", map[string]any{}, 200)
	call("driver reads the assigned lifecycle", base+"/"+v+"/lifecycle/list", map[string]any{}, 200)
	call("driver reads the assigned meters", base+"/"+v+"/meters/list", map[string]any{}, 200)
	denied := call("driver on an unassigned asset", base+"/"+w+"/read", map[string]any{}, 403)
	if detail := denied["error"].(map[string]any)["details"].([]any); len(detail) != 1 || detail[0].(map[string]any)["code"] != "fleet.asset.read" {
		t.Fatal("driver denial did not name the missing permission", denied)
	}
	call("driver on the organization list", base+"/list", map[string]any{}, 403)
	call("driver cannot assign", base+"/"+v+"/assignments/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "ADDITIONAL", "employee_ids": []string{driver.employee}}, 403)
	mine := call("driver's own assignments", "/v1/orgs/"+org+"/fleet/assignments/mine/list", map[string]any{}, 200)
	if own := mine["items"].([]any); len(own) != 1 || own[0].(map[string]any)["asset_id"] != v || own[0].(map[string]any)["state"] != "ACTIVE" {
		t.Fatal("driver's own assignments", mine)
	}

	// AUTH-006: a supervisor scoped to another unit is denied; the asset's
	// unit supervisor may assign.
	login(sulSupervisor.family, sulSupervisor.token)
	if c := code(call("Sul supervisor on a Norte asset", base+"/"+v+"/assignments/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "ADDITIONAL", "employee_ids": []string{sulSupervisor.employee}}, 403)); c != "FORBIDDEN" {
		t.Fatal("unit scope", c)
	}
	call("Sul supervisor reads Norte assignments", base+"/"+v+"/assignments/list", map[string]any{}, 403)
	assign("Sul supervisor on a Sul asset", w, map[string]any{"kind": "ADDITIONAL", "employee_ids": []string{sulSupervisor.employee}}, 201)
	call("a UNIT-only grant never covers the own list", "/v1/orgs/"+org+"/fleet/assignments/mine/list", map[string]any{}, 403)
	login(norteSupervisor.family, norteSupervisor.token)
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	end := start.Add(24 * time.Hour)
	substitute := assign("Norte supervisor adds a substitute", v, map[string]any{"kind": "TEMPORARY", "employee_ids": []string{norteSupervisor.employee}, "valid_from": start.Format(time.RFC3339Nano), "valid_to": end.Format(time.RFC3339Nano)}, 201)
	if listed := call("scheduled substitute", base+"/"+v+"/assignments/list", map[string]any{"limit": 1}, 200); len(listed["items"].([]any)) != 1 || listed["next_cursor"] == "" {
		t.Fatal("first assignment page", listed)
	} else {
		rest := call("assignment continuation", base+"/"+v+"/assignments/list", map[string]any{"limit": 1, "cursor": listed["next_cursor"]}, 200)
		next := rest["items"].([]any)
		if len(next) != 1 || next[0].(map[string]any)["assignment_id"] != substitute["assignment_ids"].([]any)[0] || next[0].(map[string]any)["state"] != "SCHEDULED" {
			t.Fatal("scheduled substitute page", rest)
		}
		call("cursor bound to asset", base+"/"+v2+"/assignments/list", map[string]any{"limit": 1, "cursor": listed["next_cursor"]}, 400)
		call("cursor bound to limit", base+"/"+v+"/assignments/list", map[string]any{"limit": 2, "cursor": listed["next_cursor"]}, 400)
	}

	// The owner ends the driver's assignment: the SELF coverage ends with it.
	login(auth.Session.FamilyID, auth.Session.AccessToken)
	endPath := base + "/" + v + "/assignments/" + ids[0].(string) + "/end"
	ending := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic return"}
	if ended := call("end", endPath, ending, 200); ended["assignment_ids"].([]any)[0] != ids[0] || ended["replayed"] != false {
		t.Fatal("end acknowledgement", ended)
	}
	if again := call("end replay", endPath, ending, 200); again["replayed"] != true {
		t.Fatal("end replay", again)
	}
	if c := code(call("end an ended assignment", endPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 409)); c != "CONFLICT" {
		t.Fatal("ended code", c)
	}
	call("end through another asset", base+"/"+w+"/assignments/"+ids[0].(string)+"/end", map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 404)
	login(driver.family, driver.token)
	call("driver after the end", base+"/"+v+"/read", map[string]any{}, 403)
	if mine := call("driver's own assignments after the end", "/v1/orgs/"+org+"/fleet/assignments/mine/list", map[string]any{}, 200); len(mine["items"].([]any)) != 0 {
		t.Fatal("ended assignment still listed", mine)
	}
	t.Logf("actual private assignment HTTP request assertions: %d", requests)
}
