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

// BUC-EMP-164/165 through the actual composed company-cookie entry:
// attach, overlap, detach rules, lists, both assets' scope and isolation.
func TestFleetAttachmentsThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = attachmentStore(t, fx)
	_ = meterStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+"; GRANT SELECT ON org_work_orders TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synattachmenthttp")
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
		t.Fatal("actual attachment composition failed", err)
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
			t.Fatalf("%s: private attachment HTTP got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && strings.Contains(w.Body.String(), "Synthetic") {
			t.Fatal("private attachment content entered refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private attachment response")
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
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic attachment organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic Norte", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
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
		created := call("draft", prefix+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic attached implement", "plate": plate, "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic attachment category"}, "unit_id": unit}, 201)
		id := created["asset_id"].(string)
		call("activate", prefix+"/"+id+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return id
	}
	harrow, sulHarrow := asset(base, norte, "ATT1A23"), asset(base, sul, "ATT1B23")
	foreign := asset(otherBase, other["unit_id"].(string), "ATT1A23")
	type person struct{ family, token, employee, member string }
	member := func(roleName, scopeType, target string) person {
		t.Helper()
		key, err := accounts.CreateKeyAccount(t.Context(), "synatt"+strings.ToLower(roleName[:3])+onboardingID(t)[:6])
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
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()-interval '1 minute',$7,'Synthetic attachment member')`, []any{org, onboardingID(t), membership, roleName, scopeType, target, organization["membership_id"]}},
			{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, membership}},
		} {
			if err := inScope(org, statement.sql, statement.args...); err != nil {
				t.Fatal(roleName, "synthetic membership setup failed", err)
			}
		}
		return person{login.Session.FamilyID, login.Session.AccessToken, employee, membership}
	}
	powered := func(prefix, unit, plate string) string {
		t.Helper()
		created := call("powered draft", prefix+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "MACHINE", "name": "Synthetic tractor", "plate": plate, "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic tractors"}, "unit_id": unit,
			"tanks": []any{map[string]any{"role": "MAIN", "capacity_milli": "275000", "products": []string{"DIESEL_S10"}}}}, 201)
		id := created["asset_id"].(string)
		installed := call("meter", prefix+"/"+id+"/meters/install", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "opening_reading_id": onboardingID(t), "kind": "HOUR_METER",
			"label": "Synthetic hour meter", "opening_value": "3600", "method": "TYPED", "occurred_at": time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Format(time.RFC3339Nano), "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400}, 201)
		call("powered activation", prefix+"/"+id+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": installed["revision"]}, 200)
		return id
	}
	tractor := powered(base, norte, "ATT1C23")
	sulFleet := member("FLEET_MANAGER", "UNIT", sul)
	attachPath := func(implement string) string { return base + "/" + implement + "/attachments/attach" }
	first := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor}
	created := call("attach", attachPath(harrow), first, 201)
	attachment := created["attachment_id"].(string)
	if created["asset_id"] != harrow || created["replayed"] != false || created["data_mode"] != "SYNTHETIC" {
		t.Fatal("attach acknowledgement", created)
	}
	if again := call("attach replay", attachPath(harrow), first, 201); again["replayed"] != true || again["attachment_id"] != attachment {
		t.Fatal("attach replay", again)
	}
	if c := code(call("AC1 overlapping attachment", attachPath(harrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor}, 409)); c != "CONFLICT" {
		t.Fatal("overlap code", c)
	}
	if c := code(call("attaching to an implement", attachPath(sulHarrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": harrow}, 422)); c != "VALIDATION_FAILED" {
		t.Fatal("kind code", c)
	}
	call("foreign powered asset", attachPath(sulHarrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": foreign}, 404)
	call("foreign implement hidden", attachPath(foreign), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor}, 404)
	call("caller-selected state", attachPath(sulHarrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor, "state": "ACTIVE"}, 400)
	for _, asset := range []string{tractor, harrow} {
		page := call("attachments of "+asset, base+"/"+asset+"/attachments/list", map[string]any{"limit": 10}, 200)
		items := page["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["attachment_id"] != attachment || items[0].(map[string]any)["state"] != "ACTIVE" || items[0].(map[string]any)["powered_asset_id"] != tractor {
			t.Fatal("attachment page", page)
		}
	}
	availability := call("implement availability", base+"/"+harrow+"/availability/read", map[string]any{}, 200)
	// F6 maintenance holds joined active-use trips, so all sources are present.
	if missing := availability["missing_sources"].([]any); availability["availability"] != "AVAILABLE" || len(missing) != 0 {
		t.Fatal("implement availability with live maintenance hold source", availability)
	}
	// AUTH-006: both assets must be covered by fresh fleet.asset.manage.
	login(sulFleet.family, sulFleet.token)
	if c := code(call("Sul fleet manager on a Norte implement", attachPath(harrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor}, 403)); c != "FORBIDDEN" {
		t.Fatal("unit scope on the path asset", c)
	}
	if c := code(call("Sul fleet manager onto a Norte tractor", attachPath(sulHarrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor}, 403)); c != "FORBIDDEN" {
		t.Fatal("unit scope on the other asset", c)
	}
	login(auth.Session.FamilyID, auth.Session.AccessToken)
	detachPath := base + "/" + tractor + "/attachments/" + attachment + "/detach"
	call("detach before the attach instant", detachPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "valid_to": time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second).Format(time.RFC3339Nano)}, 422)
	call("detach through another asset", base+"/"+sulHarrow+"/attachments/"+attachment+"/detach", map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 404)
	if detached := call("detach", detachPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic end of the harrowing"}, 200); detached["attachment_id"] != attachment || detached["asset_id"] != tractor {
		t.Fatal("detach acknowledgement", detached)
	}
	if c := code(call("detach an ended attachment", detachPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 409)); c != "CONFLICT" {
		t.Fatal("ended code", c)
	}
	call("re-attach after the detach", attachPath(harrow), map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "powered_asset_id": tractor}, 201)
	t.Logf("actual private attachment HTTP request assertions: %d", requests)
}
