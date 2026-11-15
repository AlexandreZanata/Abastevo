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

// BUC-EMP-166..168 through the actual composed company-cookie entry: template
// commands, versions, policy, archive, reads, the applicable read with
// AUTH-006 unit scope and FLEET-006 SELF, role denial and isolation.
func TestChecklistTemplatesThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = reservationStore(t, fx)
	_ = meterStore(t, fx)
	grantChecklistTables(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synchecklisthttp")
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
		t.Fatal("actual checklist composition failed", err)
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
			t.Fatalf("%s: private checklist HTTP got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && strings.Contains(w.Body.String(), "Synthetic") {
			t.Fatal("private checklist content entered a refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private checklist response")
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
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic checklist organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic Norte", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
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
	fleetBase := "/v1/orgs/" + org + "/fleet/assets"
	asset := func(prefix, unit, plate, label string) (string, string) {
		t.Helper()
		created := call("draft", prefix+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic checked implement", "plate": plate, "usage_basis": "HOURS", "category": map[string]any{"new_label": label}, "unit_id": unit}, 201)
		id := created["asset_id"].(string)
		call("activate", prefix+"/"+id+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		read := call("asset read", prefix+"/"+id+"/read", map[string]any{}, 200)
		return id, read["category"].(map[string]any)["id"].(string)
	}
	v, trucks := asset(fleetBase, norte, "CHK1A23", "Synthetic checklist trucks")
	w, vans := asset(fleetBase, sul, "CHK1B23", "Synthetic checklist vans")
	foreign, _ := asset("/v1/orgs/"+otherOrg+"/fleet/assets", other["unit_id"].(string), "CHK1A23", "Synthetic foreign checklist")
	type person struct{ family, token, employee, member string }
	member := func(roleName, scopeType, target string) person {
		t.Helper()
		key, err := accounts.CreateKeyAccount(t.Context(), "synchk"+strings.ToLower(roleName[:3])+onboardingID(t)[:6])
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
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{org, employee}},
			{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, membership, employee, login.Account.ID}},
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()-interval '1 minute',$7,'Synthetic checklist member')`, []any{org, onboardingID(t), membership, roleName, scopeType, target, organization["membership_id"]}},
			{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, membership}},
		} {
			if err := inScope(org, statement.sql, statement.args...); err != nil {
				t.Fatal(roleName, "synthetic membership setup failed", err)
			}
		}
		return person{login.Session.FamilyID, login.Session.AccessToken, employee, membership}
	}
	driver := member("DRIVER", "SELF", "")
	norteSupervisor := member("SUPERVISOR", "UNIT", norte)
	sulSupervisor := member("SUPERVISOR", "UNIT", sul)
	fleetManager := member("FLEET_MANAGER", "ORG", org)
	unitFleetManager := member("FLEET_MANAGER", "UNIT", norte)
	call("assign the driver", fleetBase+"/"+v+"/assignments/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "ADDITIONAL", "employee_ids": []string{driver.employee}}, 201)

	base, otherBase := "/v1/orgs/"+org+"/checklists", "/v1/orgs/"+otherOrg+"/checklists"
	items := []map[string]any{{"label": "Pneus calibrados", "critical": true, "photo_on_not_ok": true}, {"label": "Estepe", "optional": true}, {"label": "Buzina"}}
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "context": "DEPARTURE", "name": "Saída de caminhões", "target_kind": "CATEGORIES", "target_ids": []string{trucks}, "items": items}
	created := call("create", base+"/templates/create", create, 201)
	id := created["template_id"].(string)
	if created["revision"] != float64(1) || created["version"] != float64(1) || created["policy_revision"] != float64(1) || created["state"] != "ACTIVE" || created["replayed"] != false || created["data_mode"] != "SYNTHETIC" {
		t.Fatal("create acknowledgement", created)
	}
	if again := call("create replay", base+"/templates/create", create, 201); again["replayed"] != true || again["template_id"] != id {
		t.Fatal("create replay", again)
	}
	reused := map[string]any{}
	for k, value := range create {
		reused[k] = value
	}
	reused["name"] = "Outro"
	if c := code(call("operation reuse", base+"/templates/create", reused, 409)); c != "OPERATION_ID_REUSED" {
		t.Fatal("reuse", c)
	}
	organizationDefault := call("organization default", base+"/templates/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "context": "DEPARTURE", "name": "Padrão da organização", "target_kind": "ORGANIZATION", "items": []map[string]any{{"label": "Documentos"}}, "policy": map[string]any{"mode": "WARN", "validity_seconds": 3600}}, 201)
	// Negative validation: the answer policy and definition bounds.
	invalid := func(label string, edit func(map[string]any), want int, wantCode string) {
		t.Helper()
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "context": "RETURN", "name": "Inválido", "target_kind": "ORGANIZATION", "items": []map[string]any{{"label": "x"}}}
		edit(body)
		if c := code(call(label, base+"/templates/create", body, want)); c != wantCode {
			t.Fatal(label, c)
		}
	}
	invalid("critical optional item", func(b map[string]any) {
		b["items"] = []map[string]any{{"label": "Freios", "critical": true, "optional": true}}
	}, 422, "VALIDATION_FAILED")
	invalid("41 items", func(b map[string]any) {
		list := []map[string]any{}
		for i := 0; i < 41; i++ {
			list = append(list, map[string]any{"label": "Item"})
		}
		b["items"] = list
	}, 422, "VALIDATION_FAILED")
	invalid("validity below one hour", func(b map[string]any) { b["policy"] = map[string]any{"mode": "BLOCK", "validity_seconds": 3599} }, 422, "VALIDATION_FAILED")
	invalid("unknown policy mode", func(b map[string]any) { b["policy"] = map[string]any{"mode": "ALLOW", "validity_seconds": 7200} }, 422, "VALIDATION_FAILED")
	invalid("blank label", func(b map[string]any) { b["items"] = []map[string]any{{"label": "   "}} }, 422, "VALIDATION_FAILED")
	invalid("caller-selected state", func(b map[string]any) { b["state"] = "ARCHIVED" }, 400, "MALFORMED")
	invalid("foreign asset target", func(b map[string]any) { b["target_kind"], b["target_ids"] = "ASSETS", []string{foreign} }, 404, "NOT_FOUND")
	invalid("target taken", func(b map[string]any) {
		b["context"], b["target_kind"], b["target_ids"] = "DEPARTURE", "CATEGORIES", []string{trucks}
	}, 409, "CONFLICT")
	// Publish, stale revision, older version kept, policy, archive.
	published := call("publish", base+"/templates/"+id+"/publish", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "name": "Saída revisada", "target_kind": "CATEGORIES", "target_ids": []string{trucks, vans}, "items": items[:1]}, 200)
	if published["revision"] != float64(2) || published["version"] != float64(2) {
		t.Fatal("publish", published)
	}
	if c := code(call("stale publish", base+"/templates/"+id+"/publish", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "name": "Stale", "target_kind": "ORGANIZATION", "items": items}, 409)); c != "CONFLICT" {
		t.Fatal("stale", c)
	}
	call("publish without revision", base+"/templates/"+id+"/publish", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Sem revisão", "target_kind": "ORGANIZATION", "items": items}, 400)
	first := call("read version 1", base+"/templates/"+id+"/read", map[string]any{"version": 1}, 200)
	if sv := first["selected_version"].(map[string]any); sv["version"] != float64(1) || sv["name"] != "Saída de caminhões" || len(sv["items"].([]any)) != 3 || first["version"] != float64(2) {
		t.Fatal("version 1 kept after publication", first)
	}
	policy := call("policy", base+"/templates/"+id+"/policy", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "mode": "WARN", "validity_seconds": 7200}, 200)
	if policy["policy_revision"] != float64(2) || policy["revision"] != float64(3) {
		t.Fatal("policy", policy)
	}
	if c := code(call("invalid policy", base+"/templates/"+id+"/policy", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3, "mode": "WARN", "validity_seconds": 259201}, 422)); c != "VALIDATION_FAILED" {
		t.Fatal("invalid policy", c)
	}
	current := call("read current", base+"/templates/"+id+"/read", map[string]any{}, 200)
	if current["policy"].(map[string]any)["mode"] != "WARN" || current["selected_version"].(map[string]any)["version"] != float64(2) || len(current["selected_version"].(map[string]any)["target_ids"].([]any)) != 2 {
		t.Fatal("current read", current)
	}
	list := call("list", base+"/templates/list", map[string]any{"limit": 1}, 200)
	if len(list["items"].([]any)) != 1 || list["next_cursor"] == "" {
		t.Fatal("first page", list)
	}
	rest := call("list rest", base+"/templates/list", map[string]any{"limit": 1, "cursor": list["next_cursor"]}, 200)
	if len(rest["items"].([]any)) != 1 || rest["next_cursor"] != "" {
		t.Fatal("second page", rest)
	}
	// Applicable read: category for the Norte truck, its pinned version and policy.
	applicable := call("applicable", base+"/assets/"+v+"/applicable", map[string]any{"context": "DEPARTURE"}, 200)
	if applicable["match"] != "CATEGORY" || applicable["required"] != true || applicable["template"].(map[string]any)["template_id"] != id || applicable["template"].(map[string]any)["version"] != float64(2) || applicable["policy"].(map[string]any)["revision"] != float64(2) {
		t.Fatal("applicable", applicable)
	}
	if _, leaked := applicable["template"].(map[string]any)["target_ids"]; leaked {
		t.Fatal("the applicable read revealed the template's targets")
	}
	if none := call("applicable none", base+"/assets/"+v+"/applicable", map[string]any{"context": "RETURN"}, 200); none["match"] != "NONE" || none["required"] != false || none["template"] != nil {
		t.Fatal("no return template", none)
	}
	call("applicable unknown context", base+"/assets/"+v+"/applicable", map[string]any{"context": "PARKING"}, 422)
	// Isolation: foreign template and asset are not found.
	call("foreign asset applicable", base+"/assets/"+foreign+"/applicable", map[string]any{"context": "DEPARTURE"}, 404)
	call("template through another organization", otherBase+"/templates/"+id+"/read", map[string]any{}, 404)
	call("archive through another organization", otherBase+"/templates/"+id+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3}, 404)
	call("unknown template", base+"/templates/"+onboardingID(t)+"/read", map[string]any{}, 404)

	// Roles: a supervisor holds no check.template.manage; a UNIT-scoped fleet
	// manager covers no organization-level template; an ORG fleet manager does.
	login(norteSupervisor.family, norteSupervisor.token)
	denied := call("supervisor lists templates", base+"/templates/list", map[string]any{}, 403)
	if detail := denied["error"].(map[string]any)["details"].([]any); code(denied) != "FORBIDDEN" || len(detail) != 1 || detail[0].(map[string]any)["code"] != "check.template.manage" {
		t.Fatal("supervisor denial", denied)
	}
	call("Norte supervisor reads the Norte asset's template", base+"/assets/"+v+"/applicable", map[string]any{"context": "DEPARTURE"}, 200)
	login(sulSupervisor.family, sulSupervisor.token)
	unitDenied := call("Sul supervisor reads a Norte asset", base+"/assets/"+v+"/applicable", map[string]any{"context": "DEPARTURE"}, 403)
	if detail := unitDenied["error"].(map[string]any)["details"].([]any); code(unitDenied) != "FORBIDDEN" || len(detail) != 1 || detail[0].(map[string]any)["code"] != "fleet.asset.read" {
		t.Fatal("unit scope denial", unitDenied)
	}
	if sulRead := call("Sul supervisor reads a Sul asset", base+"/assets/"+w+"/applicable", map[string]any{"context": "DEPARTURE"}, 200); sulRead["match"] != "CATEGORY" {
		t.Fatal("Sul asset", sulRead)
	}
	login(unitFleetManager.family, unitFleetManager.token)
	if c := code(call("UNIT fleet manager creates a template", base+"/templates/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "context": "AD_HOC", "name": "Unidade", "target_kind": "ORGANIZATION", "items": items}, 403)); c != "FORBIDDEN" {
		t.Fatal("unit fleet manager", c)
	}
	login(fleetManager.family, fleetManager.token)
	archived := call("ORG fleet manager archives", base+"/templates/"+organizationDefault["template_id"].(string)+"/archive", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
	if archived["state"] != "ARCHIVED" {
		t.Fatal("archive", archived)
	}
	if c := code(call("policy on an archived template", base+"/templates/"+organizationDefault["template_id"].(string)+"/policy", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "mode": "BLOCK", "validity_seconds": 3600}, 409)); c != "CONFLICT" {
		t.Fatal("archived", c)
	}
	// SELF: the driver reads the applicable template of the assigned asset
	// only, and cannot read or change templates.
	login(driver.family, driver.token)
	call("driver reads the assigned asset's template", base+"/assets/"+v+"/applicable", map[string]any{"context": "DEPARTURE"}, 200)
	call("driver on an unassigned asset", base+"/assets/"+w+"/applicable", map[string]any{"context": "DEPARTURE"}, 403)
	call("driver reads a template", base+"/templates/"+id+"/read", map[string]any{}, 403)
	call("driver publishes", base+"/templates/"+id+"/publish", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3, "name": "Motorista", "target_kind": "ORGANIZATION", "items": items}, 403)
	t.Logf("actual private checklist HTTP request assertions: %d", requests)
}
