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

// EMP-METER-005 "Done when" through the actual composed company routes:
// example D confirmed on the decide route, a reset and a replacement with an
// unknown final value on the change route, with the DRIVER, the supervisor
// of another unit, another organization, a stale segment count and an
// opening above the modulus refused.
func TestMeterBoundariesThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = assignmentStore(t, fx)
	_ = meterStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	// The decision's approved-parent port reads the fueling review state and,
	// since EMP-TRIP-015, the trips that carry the reading.
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+";GRANT SELECT(org_id,reading_id,review_state) ON org_fuelings TO "+role+
		";GRANT SELECT ON org_trips,org_trip_finishes,org_trip_amendments,org_trip_review_entries TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synmeterboundary")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{7}, 32), Pool: fx.f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal("actual meter boundary composition failed", err)
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
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && (strings.Contains(w.Body.String(), "Synthetic") || strings.Contains(w.Body.String(), "Dígitos")) {
			t.Fatal("private reading content entered a refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid response")
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
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic boundary organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic Norte", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
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
	base := "/v1/orgs/" + org + "/fleet/assets"
	created := call("draft", base+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic wrapped implement", "plate": "WRP1A23", "usage_basis": "DISTANCE", "category": map[string]any{"new_label": "Synthetic correction category"}, "unit_id": norte}, 201)
	asset := created["asset_id"].(string)
	opening := time.Now().UTC().Truncate(time.Second).Add(-7 * 24 * time.Hour)
	installed := call("install", base+"/"+asset+"/meters/install", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "opening_reading_id": onboardingID(t), "kind": "ODOMETER",
		"label": "Synthetic six-digit odometer", "modulus": "1000000000", "opening_value": "999000000", "method": "TYPED", "occurred_at": opening.Format(time.RFC3339Nano), "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400}, 201)
	meterID := installed["meter_id"].(string)
	segment := installed["segment_id"].(string)
	call("activate", base+"/"+asset+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}, 200)
	// Field readings as the signed intake classifies them (METER-003), written
	// through the real observation and history guards.
	var session string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM company_web_sessions WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, auth.Account.ID).Scan(&session); err != nil {
		t.Fatal(err)
	}
	seed := func(value int64, at time.Duration, state, flags string) string {
		t.Helper()
		id, op := onboardingID(t), onboardingID(t)
		if err := inScope(org, `INSERT INTO org_meter_readings(org_id,id,meter_id,segment_id,value,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,author_membership_id,capture_kind,web_session_id,operation_id,request_hash,origin,method,state)
VALUES($1,$2,$3,$4,$5,$6,0,'America/Cuiaba',-14400,$7,'WEB_SESSION',$8,$9,repeat('a',64),'FUELING','TYPED',$10)`, org, id, meterID, segment, value, opening.Add(at), organization["membership_id"], session, op, state); err != nil {
			t.Fatal(err)
		}
		if err := inScope(org, `INSERT INTO org_meter_reading_states(org_id,meter_id,reading_id,revision,state,flags,cause,operation_id) VALUES($1,$2,$3,1,$4,COALESCE(string_to_array(NULLIF($5,''),'+'),'{}'),'INTAKE',$6)`, org, meterID, id, state, flags, op); err != nil {
			t.Fatal(err)
		}
		return id
	}
	seed(999_950_000, 24*time.Hour, "VALID", "")
	s2 := seed(120_000, 48*time.Hour, "SUSPECT", "POSSIBLE_ROLLOVER")
	meters := base + "/" + asset + "/meters/" + meterID
	// Members: a DRIVER (SELF), a SUPERVISOR of the Norte unit and one of Sul.
	type person struct{ family, token, member string }
	member := func(roleName, scopeType, target string) person {
		t.Helper()
		key, err := accounts.CreateKeyAccount(t.Context(), "synmbd"+strings.ToLower(roleName[:3])+onboardingID(t)[:6])
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
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()-interval '1 minute',$7,'Synthetic boundary member')`, []any{org, onboardingID(t), membership, roleName, scopeType, target, organization["membership_id"]}},
			{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, membership}},
		} {
			if err := inScope(org, statement.sql, statement.args...); err != nil {
				t.Fatal(roleName, err)
			}
		}
		return person{login.Session.FamilyID, login.Session.AccessToken, membership}
	}
	driver, norteSupervisor, sulSupervisor := member("DRIVER", "SELF", ""), member("SUPERVISOR", "UNIT", norte), member("SUPERVISOR", "UNIT", sul)
	at := func(d time.Duration) string { return opening.Add(d).Format(time.RFC3339Nano) }
	reset := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "change": "RESET", "kind": "ODOMETER", "occurred_at": at(72 * time.Hour),
		"entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "before_value": "150000", "after_value": "0"}
	rollover := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "decision": "CONFIRM_ROLLOVER", "reason": "Painel de seis dígitos"}
	login(driver.family, driver.token)
	if c := code(call("driver confirms a rollover", meters+"/readings/"+s2+"/decide", rollover, 403)); c != "FORBIDDEN" {
		t.Fatal("DRIVER rollover", c)
	}
	if c := code(call("driver resets", meters+"/change", reset, 403)); c != "FORBIDDEN" {
		t.Fatal("DRIVER reset", c)
	}
	login(sulSupervisor.family, sulSupervisor.token)
	if c := code(call("supervisor of another unit resets", meters+"/change", reset, 403)); c != "FORBIDDEN" {
		t.Fatal("other-unit supervisor reset", c)
	}
	login(norteSupervisor.family, norteSupervisor.token)
	call("another organization's path", "/v1/orgs/"+otherOrg+"/fleet/assets/"+asset+"/meters/"+meterID+"/change", reset, 404)
	confirmed := call("supervisor confirms the rollover", meters+"/readings/"+s2+"/decide", rollover, 200)
	boundary := confirmed["boundary"].(map[string]any)
	implied := boundary["implied_usage"].(map[string]any)
	if confirmed["reading"].(map[string]any)["state"] != "VALID" || boundary["change"] != "ROLLOVER" || boundary["closing_value"] != "1000000000" || boundary["opening_value"] != "0" ||
		implied["status"] != "OK" || implied["value"] != "170000" || boundary["opening_reading_id"] != s2 || boundary["segments"] != float64(2) {
		t.Fatal("example D through the decide route", confirmed)
	}
	stale := map[string]any{}
	for k, v := range reset {
		stale[k] = v
	}
	stale["operation_id"], stale["revision"] = onboardingID(t), 1
	if c := code(call("stale segment count", meters+"/change", stale, 409)); c != "CONFLICT" {
		t.Fatal("stale reset", c)
	}
	done := call("supervisor resets", meters+"/change", reset, 200)
	if done["change"] != "RESET" || done["closing_value"] != "150000" || done["implied_usage"].(map[string]any)["value"] != "30000" || done["segments"] != float64(3) || done["replayed"] != false || done["new_meter_id"] != nil {
		t.Fatal("reset through the change route", done)
	}
	if again := call("reset replay", meters+"/change", reset, 200); again["replayed"] != true || again["opened_segment_id"] != done["opened_segment_id"] {
		t.Fatal("reset replay", again)
	}
	replace := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 3, "change": "REPLACE", "kind": "ODOMETER", "occurred_at": at(96 * time.Hour),
		"entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "final_unknown": true, "label": "Synthetic new panel", "modulus": "1000000", "opening_value": "1000000"}
	if c := code(call("opening not below the modulus", meters+"/change", replace, 422)); c != "VALIDATION_FAILED" {
		t.Fatal("opening above the modulus", c)
	}
	replace["operation_id"], replace["opening_value"] = onboardingID(t), "0"
	replaced := call("replacement with an unknown final value", meters+"/change", replace, 200)
	gap := replaced["implied_usage"].(map[string]any)
	if replaced["change"] != "REPLACE" || replaced["closing_value"] != nil || replaced["closing_reading_id"] != nil || gap["status"] != "INSUFFICIENT_DATA" || gap["reason"] != "METER_GAP" || gap["value"] != nil || replaced["new_meter_id"] == nil {
		t.Fatal("replacement with an unknown final value", replaced)
	}
	next := replaced["new_meter_id"].(string)
	latest := call("new meter latest", base+"/"+asset+"/meters/"+next+"/latest", map[string]any{}, 200)["latest"].(map[string]any)
	if latest["id"] != replaced["opening_reading_id"] || latest["value"] != "0" {
		t.Fatal("the new meter's opening is not its current value", latest)
	}
	call("removed meter latest", meters+"/latest", map[string]any{}, 404)
	t.Logf("actual meter boundary HTTP request assertions: %d", requests)
}
