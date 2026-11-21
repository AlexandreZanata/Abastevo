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

// BUC-EMP-148..151 through the actual composed company-cookie entry:
// commands, conflict details, block removal, reads, AUTH-006 unit scope,
// SELF reads on an assigned asset and isolation.
func TestFleetReservationsThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = reservationStore(t, fx)
	_ = meterStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+"; GRANT SELECT ON org_work_orders TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synreservationhttp")
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
		t.Fatal("actual reservation composition failed", err)
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
			t.Fatalf("%s: private reservation HTTP got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && strings.Contains(w.Body.String(), "Synthetic") {
			t.Fatal("private reservation content entered refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private reservation response")
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
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic reservation organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic Norte", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
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
		created := call("draft", prefix+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic reserved implement", "plate": plate, "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic reservation category"}, "unit_id": unit}, 201)
		id := created["asset_id"].(string)
		call("activate", prefix+"/"+id+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return id
	}
	v, w := asset(base, norte, "RSV1A23"), asset(base, sul, "RSV1B23")
	foreign := asset(otherBase, other["unit_id"].(string), "RSV1A23")
	type person struct{ family, token, employee, member string }
	member := func(roleName, scopeType, target string) person {
		t.Helper()
		key, err := accounts.CreateKeyAccount(t.Context(), "synrsv"+strings.ToLower(roleName[:3])+onboardingID(t)[:6])
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
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()-interval '1 minute',$7,'Synthetic reservation member')`, []any{org, onboardingID(t), membership, roleName, scopeType, target, organization["membership_id"]}},
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
	call("assign the driver", base+"/"+v+"/assignments/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "ADDITIONAL", "employee_ids": []string{driver.employee}}, 201)

	now := time.Now().UTC().Truncate(time.Second)
	instant := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339Nano) }
	request := func(label, assetID string, body map[string]any, want int) map[string]any {
		t.Helper()
		body["schema_version"] = 1
		if body["operation_id"] == nil {
			body["operation_id"] = onboardingID(t)
		}
		if body["purpose"] == nil {
			body["purpose"] = "Synthetic delivery round"
		}
		return call(label, base+"/"+assetID+"/reservations/request", body, want)
	}
	// Owner (ORG) requests and confirms in one command; replay; conflict.
	first := map[string]any{"operation_id": onboardingID(t), "starts_at": instant(2 * time.Hour), "ends_at": instant(4 * time.Hour), "employee_id": driver.employee, "confirm": true}
	created := request("confirmed request", v, first, 201)
	id := created["reservation_id"].(string)
	if created["state"] != "CONFIRMED" || created["replayed"] != false || created["asset_id"] != v || created["data_mode"] != "SYNTHETIC" {
		t.Fatal("request acknowledgement", created)
	}
	if again := request("request replay", v, first, 201); again["replayed"] != true || again["reservation_id"] != id {
		t.Fatal("request replay", again)
	}
	conflict := request("overlapping confirmation", v, map[string]any{"starts_at": instant(3 * time.Hour), "ends_at": instant(5 * time.Hour), "confirm": true}, 409)
	details := conflict["error"].(map[string]any)["details"].([]any)
	if code(conflict) != "RESERVATION_CONFLICT" || len(details) != 2 {
		t.Fatal("conflict refusal", conflict)
	}
	for i, want := range []map[string]any{{"field": "conflict_window", "code": "STARTS_AT", "instant": instant(2 * time.Hour)}, {"field": "conflict_window", "code": "ENDS_AT", "instant": instant(4 * time.Hour)}} {
		got := details[i].(map[string]any)
		if len(got) != 3 || got["field"] != want["field"] || got["code"] != want["code"] || got["instant"] != want["instant"] {
			t.Fatal("conflict reveals more or less than the window", got)
		}
	}
	if raw, _ := json.Marshal(conflict); strings.Contains(string(raw), id) || strings.Contains(string(raw), driver.employee) {
		t.Fatal("conflict revealed the reservation or the employee")
	}
	request("caller-selected state", v, map[string]any{"starts_at": instant(6 * time.Hour), "ends_at": instant(7 * time.Hour), "state": "CONFIRMED"}, 400)
	request("window in the past", v, map[string]any{"starts_at": instant(-time.Hour), "ends_at": instant(time.Hour)}, 422)
	call("foreign asset hidden", base+"/"+foreign+"/reservations/request", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "starts_at": instant(time.Hour), "ends_at": instant(2 * time.Hour), "purpose": "Synthetic"}, 404)
	call("asset hidden from another organization", otherBase+"/"+v+"/reservations/list", map[string]any{}, 404)
	call("reservation through another asset", base+"/"+w+"/reservations/"+id+"/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic"}, 404)
	// Blocks: a removal without a reason is refused; ended removals conflict.
	block := call("block", base+"/"+v+"/blocks/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason_kind": "LENT_OUT", "note": "Synthetic loan", "valid_from": instant(24 * time.Hour)}, 201)
	blockID := block["block_id"].(string)
	removePath := base + "/" + v + "/blocks/" + blockID + "/remove"
	if c := code(call("block removed without a reason", removePath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 422)); c != "VALIDATION_FAILED" {
		t.Fatal("removal without reason", c)
	}
	call("block removed with an empty reason", removePath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": ""}, 422)
	call("block reason kind unknown", base+"/"+v+"/blocks/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason_kind": "MAINTENANCE"}, 422)
	blocks := call("blocks", base+"/"+v+"/blocks/list", map[string]any{"limit": 10}, 200)
	if items := blocks["items"].([]any); len(items) != 1 || items[0].(map[string]any)["reason_kind"] != "LENT_OUT" || items[0].(map[string]any)["removed_at"] != nil {
		t.Fatal("block list", blocks)
	}
	call("remove the block", removePath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic plans changed"}, 200)
	if c := code(call("remove a removed block", removePath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic again"}, 409)); c != "CONFLICT" {
		t.Fatal("removed block", c)
	}
	page := call("reservations", base+"/"+v+"/reservations/list", map[string]any{"limit": 10}, 200)
	if items := page["items"].([]any); len(items) != 1 || items[0].(map[string]any)["state"] != "CONFIRMED" || items[0].(map[string]any)["employee_id"] != driver.employee || items[0].(map[string]any)["no_show_tolerance_minutes"] != float64(60) || page["evaluated_at"] == nil {
		t.Fatal("reservation page", page)
	}
	available := call("availability", base+"/"+v+"/availability/read", map[string]any{}, 200)
	if available["availability"] != "AVAILABLE" || len(available["causes"].([]any)) != 0 || len(available["missing_sources"].([]any)) != 0 {
		t.Fatal("availability before the window", available)
	}

	// AUTH-006: a supervisor scoped to another unit is denied with the key.
	pending := request("pending request", v, map[string]any{"starts_at": instant(8 * time.Hour), "ends_at": instant(9 * time.Hour)}, 201)
	pendingID := pending["reservation_id"].(string)
	login(sulSupervisor.family, sulSupervisor.token)
	denied := call("Sul supervisor confirms a Norte reservation", base+"/"+v+"/reservations/"+pendingID+"/confirm", map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 403)
	if detail := denied["error"].(map[string]any)["details"].([]any); code(denied) != "FORBIDDEN" || len(detail) != 1 || detail[0].(map[string]any)["code"] != "fleet.reservation.manage" {
		t.Fatal("unit scope denial", denied)
	}
	call("Sul supervisor blocks a Sul asset", base+"/"+w+"/blocks/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason_kind": "SAFETY"}, 201)
	login(norteSupervisor.family, norteSupervisor.token)
	if confirmed := call("Norte supervisor confirms", base+"/"+v+"/reservations/"+pendingID+"/confirm", map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 200); confirmed["state"] != "CONFIRMED" {
		t.Fatal("confirm", confirmed)
	}
	cancelPath := base + "/" + v + "/reservations/" + pendingID + "/cancel"
	call("cancel by someone other than the requester without a reason", cancelPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 422)
	if cancelled := call("cancel with a reason", cancelPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic plan change"}, 200); cancelled["state"] != "CANCELLED" {
		t.Fatal("cancel", cancelled)
	}
	call("confirm a cancelled reservation", base+"/"+v+"/reservations/"+pendingID+"/confirm", map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 409)
	own := request("supervisor's own request", v, map[string]any{"starts_at": instant(10 * time.Hour), "ends_at": instant(11 * time.Hour)}, 201)
	call("the requester cancels without a reason", base+"/"+v+"/reservations/"+own["reservation_id"].(string)+"/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}, 200)

	// SELF: the driver reads availability and reservations of the assigned
	// asset, cannot reserve it and cannot read another asset.
	login(driver.family, driver.token)
	call("driver reads availability", base+"/"+v+"/availability/read", map[string]any{}, 200)
	call("driver reads reservations", base+"/"+v+"/reservations/list", map[string]any{}, 200)
	call("driver reads blocks", base+"/"+v+"/blocks/list", map[string]any{}, 200)
	call("driver on an unassigned asset", base+"/"+w+"/availability/read", map[string]any{}, 403)
	if c := code(request("driver cannot reserve", v, map[string]any{"starts_at": instant(12 * time.Hour), "ends_at": instant(13 * time.Hour)}, 403)); c != "FORBIDDEN" {
		t.Fatal("driver reservation", c)
	}
	t.Logf("actual private reservation HTTP request assertions: %d", requests)
}
