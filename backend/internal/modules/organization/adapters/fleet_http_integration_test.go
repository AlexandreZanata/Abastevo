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

// This is the actual composed company-cookie entry over an ordinary runtime
// login. Fixture/operator writes are explicit and never supply request authority.
func TestFleetAssetsThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = fleetStore(t, fx) // Exact installed fleet privileges; no permissive grant.
	_ = meterStore(t, fx) // Installed real meter owner, not fixture presence facts.
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticfleethttp")
	if err != nil {
		t.Fatal("synthetic account creation failed")
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal("synthetic account login failed")
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{9}, 32), Pool: fx.f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal("actual fleet composition failed", err)
	}
	var cookie *http.Cookie
	csrf := ""
	requests := 0
	call := func(label, path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal("synthetic request encoding failed")
		}
		r := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
			r.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(r)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		requests++
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("%s: private fleet HTTP got%d want%d", label, w.Code, want)
		}
		if w.Code >= 400 && (strings.Contains(w.Body.String(), "Synthetic") || strings.Contains(w.Body.String(), "ABC1D23") || strings.Contains(w.Body.String(), "syntheticfleet")) {
			t.Fatal("private fleet source content entered refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private fleet response")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing company cookie")
			}
			cookie = cookies[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	clone := func(value map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(value)
		var out map[string]any
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("invalid immutable fixture copy")
		}
		return out
	}
	code := func(value map[string]any) string {
		t.Helper()
		failure, ok := value["error"].(map[string]any)
		if !ok {
			t.Fatal("registered fleet refusal missing")
		}
		result, ok := failure["code"].(string)
		if !ok {
			t.Fatal("registered fleet refusal code missing")
		}
		return result
	}
	call("company login", "/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, nil)
	createOrg := func() map[string]any {
		t.Helper()
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic fleet organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic main", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201, nil)
		id := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, id), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal("explicit fixture capability enable failed", err)
		}
		call("organization activation", "/v1/orgs/"+id+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200, nil)
		return result
	}
	organization, other := createOrg(), createOrg()
	org, otherOrg := organization["organization_id"].(string), other["organization_id"].(string)
	base, otherBase := "/v1/orgs/"+org+"/fleet/assets", "/v1/orgs/"+otherOrg+"/fleet/assets"
	sourceFx := fx
	sourceFx.scope = scopeFor(t, org)
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic private implement", "plate": "ABC1D23", "serial_number": "SYNTHETIC-SERIAL", "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic declared category"}, "unit_id": organization["unit_id"]}
	for label, edit := range map[string]func(*http.Request){"no cookie": func(r *http.Request) { r.Header.Del("Cookie") }, "no csrf": func(r *http.Request) { r.Header.Del("X-CSRF") }, "foreign origin": func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, "wrong cookie plane": func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, "header proof": func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, "signed instead cookie": func(r *http.Request) { r.Header.Set("Signature", "synthetic") }} {
		call(label, base+"/create", create, 401, edit)
	}
	for _, field := range []string{"account_id", "family_id", "state", "meters", "accepted_operations", "permission"} {
		injected := clone(create)
		injected[field] = "synthetic"
		call("authority injection", base+"/create", injected, 400, nil)
	}
	created := call("actual draft create", base+"/create", create, 201, nil)
	id := created["asset_id"].(string)
	path := base + "/" + id
	if created["state"] != "DRAFT" || created["revision"] != float64(1) || created["discarded"] != false {
		t.Fatal("new asset lifecycle was not source selected")
	}
	if replay := call("original create replay", base+"/create", create, 201, nil); replay["asset_id"] != id || replay["replayed"] != true {
		t.Fatal("create replay changed original identity")
	}
	view := call("source read", path+"/read", map[string]any{}, 200, nil)
	category := view["category"].(map[string]any)["id"].(string)
	if view["plate"] != "ABC1D23" || view["unit_id"] != organization["unit_id"] {
		t.Fatal("stored source fields changed")
	}
	noUnit := clone(create)
	noUnit["operation_id"] = onboardingID(t)
	noUnit["plate"] = ""
	noUnit["serial_number"] = "SYNTHETIC-NO-UNIT"
	noUnit["unit_id"] = ""
	noUnit["category"] = map[string]any{"existing_id": category}
	noUnitAck := call("draft may omit unit", base+"/create", noUnit, 201, nil)
	incomplete := call("known missing activation unit", base+"/"+noUnitAck["asset_id"].(string)+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 422, nil)
	if code(incomplete) != "VALIDATION_FAILED" {
		t.Fatal("known missing unit lost validation code")
	}
	missing := incomplete["error"].(map[string]any)["details"].([]any)
	if len(missing) != 1 || missing[0].(map[string]any)["field"] != "unit_id" || missing[0].(map[string]any)["code"] != "REQUIRED" {
		t.Fatal("known missing unit lost exact checklist")
	}
	duplicate := clone(create)
	duplicate["operation_id"] = onboardingID(t)
	duplicate["plate"] = "abc-1d23"
	if code(call("normalized identifier conflict", base+"/create", duplicate, 409, nil)) != "ASSET_IDENTIFIER_TAKEN" {
		t.Fatal("identifier conflict lost registered code")
	}
	foreignCreate := clone(duplicate)
	foreignCreate["unit_id"] = other["unit_id"]
	foreignCreate["operation_id"] = onboardingID(t)
	foreign := call("other organization independent key", otherBase+"/create", foreignCreate, 201, nil)
	call("foreign resource hidden", otherBase+"/"+id+"/read", map[string]any{}, 404, nil)
	foreignCategory := clone(create)
	foreignCategory["operation_id"] = onboardingID(t)
	foreignCategory["plate"] = "DEF2G34"
	foreignCategory["category"] = map[string]any{"existing_id": category}
	foreignCategory["unit_id"] = other["unit_id"]
	call("foreign category hidden", otherBase+"/create", foreignCategory, 404, nil)
	change := clone(create)
	change["operation_id"] = onboardingID(t)
	change["revision"] = 1
	change["name"] = "Synthetic changed implement"
	change["category"] = map[string]any{"existing_id": category}
	if result := call("profile CAS", path+"/change", change, 200, nil); result["revision"] != float64(2) {
		t.Fatal("profile CAS revision missing")
	}
	stale := clone(change)
	stale["operation_id"] = onboardingID(t)
	if code(call("stale profile no effect", path+"/change", stale, 409, nil)) != "CONFLICT" {
		t.Fatal("stale profile code changed")
	}
	// Powered activation now consumes actual persisted meter facts. A real
	// empty source has a typed missing-meter checklist; installation satisfies it.
	powered := clone(create)
	powered["operation_id"] = onboardingID(t)
	powered["kind"] = "VEHICLE"
	powered["name"] = "Synthetic powered asset"
	powered["plate"] = "GHI3J45"
	powered["usage_basis"] = "DISTANCE"
	powered["category"] = map[string]any{"existing_id": category}
	powered["tanks"] = []any{map[string]any{"role": "MAIN", "capacity_milli": "275000", "products": []string{"DIESEL_S10"}}}
	powered["expected_consumption"] = []any{map[string]any{"product": "DIESEL_S10", "basis": "DISTANCE", "value": "9007199254740993", "source": "Synthetic manual", "date": "2026-11-14"}}
	poweredAck := call("powered draft exact profile", base+"/create", powered, 201, nil)
	poweredPath := base + "/" + poweredAck["asset_id"].(string)
	poweredView := call("powered source exact read", poweredPath+"/read", map[string]any{}, 200, nil)
	if poweredView["tanks"].([]any)[0].(map[string]any)["capacity_milli"] != "275000" || poweredView["expected_consumption"].([]any)[0].(map[string]any)["value"] != "9007199254740993" {
		t.Fatal("HTTP rounded exact fleet fields")
	}
	missingMeter := call("actual source proves missing meter", poweredPath+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 422, nil)
	if code(missingMeter) != "VALIDATION_FAILED" {
		t.Fatal("missing real meter code")
	}
	details := missingMeter["error"].(map[string]any)["details"].([]any)
	if len(details) != 1 || details[0].(map[string]any)["field"] != "meters" || details[0].(map[string]any)["code"] != "REQUIRED" {
		t.Fatal("missing real meter checklist")
	}
	observed := time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond).Format(time.RFC3339Nano)
	install := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "opening_reading_id": onboardingID(t), "kind": "ODOMETER", "label": "Synthetic observed display", "opening_value": "1000000000000", "modulus": "1000000000001", "occurred_at": observed, "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "method": "TYPED"}
	installed := call("real observed meter installation", poweredPath+"/meters/install", install, 201, nil)
	meterID := installed["meter_id"].(string)
	if installed["revision"] != float64(2) || installed["opening_reading_id"] != install["opening_reading_id"] {
		t.Fatal("meter installation metadata")
	}
	original := call("meter original acknowledgement", poweredPath+"/meters/install", install, 201, nil)
	if original["replayed"] != true || original["meter_id"] != meterID || original["revision"] != installed["revision"] {
		t.Fatal("meter replay changed actual result")
	}
	changedInstall := clone(install)
	changedInstall["label"] = "Synthetic changed display"
	if code(call("changed meter operation body", poweredPath+"/meters/install", changedInstall, 409, nil)) != "OPERATION_ID_REUSED" {
		t.Fatal("meter intent reuse")
	}
	duplicateMeter := clone(install)
	duplicateMeter["operation_id"] = onboardingID(t)
	duplicateMeter["opening_reading_id"] = onboardingID(t)
	duplicateMeter["revision"] = 2
	if code(call("second in-service meter kind", poweredPath+"/meters/install", duplicateMeter, 409, nil)) != "METER_ALREADY_IN_SERVICE" {
		t.Fatal("meter in-service uniqueness")
	}
	meterView := call("actual latest observed reading", poweredPath+"/meters/"+meterID+"/latest", map[string]any{}, 200, nil)
	latest := meterView["latest"].(map[string]any)
	if latest["value"] != "1000000000000" || latest["occurred_at"] != observed || latest["capture"].(map[string]any)["kind"] != "WEB_SESSION" {
		t.Fatal("private exact meter evidence lost")
	}
	if active := call("powered actual source activation", poweredPath+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}, 200, nil); active["state"] != "ACTIVE" || active["revision"] != float64(3) {
		t.Fatal("powered activation did not consume real meter")
	}
	hour := clone(install)
	hour["operation_id"] = onboardingID(t)
	hour["opening_reading_id"] = onboardingID(t)
	hour["revision"] = 3
	hour["kind"] = "HOUR_METER"
	hour["opening_value"] = "3600"
	delete(hour, "modulus")
	call("second independent meter kind", poweredPath+"/meters/install", hour, 201, nil)
	meterPage := call("bounded actual meter page", poweredPath+"/meters/list", map[string]any{"limit": 1}, 200, nil)
	meterCursor := meterPage["next_cursor"].(string)
	if len(meterPage["items"].([]any)) != 1 || meterCursor == "" {
		t.Fatal("meter keyset pagination missing")
	}
	call("actual next meter page", poweredPath+"/meters/list", map[string]any{"limit": 1, "cursor": meterCursor}, 200, nil)
	call("meter cursor bound to asset", path+"/meters/list", map[string]any{"limit": 1, "cursor": meterCursor}, 400, nil)
	call("foreign actual meter hidden", otherBase+"/"+foreign["asset_id"].(string)+"/meters/"+meterID+"/latest", map[string]any{}, 404, nil)

	activation := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	foreignPath := otherBase + "/" + foreign["asset_id"].(string)
	if active := call("implement actual activation", foreignPath+"/activate", activation, 200, nil); active["state"] != "ACTIVE" || active["revision"] != float64(2) {
		t.Fatal("actual implement activation failed")
	}
	activeView := call("actual initial allocation", foreignPath+"/read", map[string]any{}, 200, nil)
	if activeView["initial_allocation"].(map[string]any)["unit_id"] != other["unit_id"] {
		t.Fatal("initial unit allocation missing")
	}
	// A bound keyset cannot be reused for another current source or filter.
	page := call("bounded first keyset page", base+"/list", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if len(page["items"].([]any)) != 1 || cursor == "" {
		t.Fatal("stable bounded keyset page missing")
	}
	call("filter bound cursor", base+"/list", map[string]any{"limit": 1, "state": "ACTIVE", "cursor": cursor}, 400, nil)
	call("organization bound cursor", otherBase+"/list", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	call("actual keyset continuation", base+"/list", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	// Physically discarded draft receipts survive and never recreate its row.
	discard := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2}
	removed := call("unused draft discard", path+"/discard", discard, 200, nil)
	if removed["discarded"] != true || removed["revision"] != float64(3) {
		t.Fatal("discard immutable metadata missing")
	}
	if _, ok := removed["state"]; ok {
		t.Fatal("discard invented ARCHIVED state")
	}
	call("discarded draft hidden", path+"/read", map[string]any{}, 404, nil)
	if call("discard original replay after deletion", path+"/discard", discard, 200, nil)["replayed"] != true {
		t.Fatal("discard replay required live asset")
	}
	if result := call("create original replay after deletion", base+"/create", create, 201, nil); result["asset_id"] != id || result["revision"] != float64(1) || result["state"] != "DRAFT" || result["replayed"] != true {
		t.Fatal("original create receipt changed after deletion")
	}
	changedIntent := clone(create)
	changedIntent["name"] = "Synthetic reused changed intent"
	if code(call("changed operation reuse", base+"/create", changedIntent, 409, nil)) != "OPERATION_ID_REUSED" {
		t.Fatal("changed operation reused original receipt")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_assets WHERE org_id=$1 AND id=$2) AND(SELECT count(*) FROM org_asset_commands WHERE org_id=$1 AND asset_id=$2)=3 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2)=3`, org, id).Scan(&exact); err != nil || !exact {
		t.Fatal("HTTP retries duplicated effects or lost history", err)
	}
	// A real DRIVER's SELF scope has no actual assignment source yet.
	driverKey, err := accounts.CreateKeyAccount(t.Context(), "syntheticfleetdriver")
	if err != nil {
		t.Fatal("synthetic driver account failed")
	}
	driver, err := accounts.LoginWithKey(t.Context(), driverKey.Key)
	if err != nil {
		t.Fatal("synthetic driver login failed")
	}
	member, employee := onboardingID(t), onboardingID(t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{org, employee}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, member, employee, driver.Account.ID}},
		{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'DRIVER','SELF',$3,clock_timestamp()-interval '1 minute',$4,'Synthetic driver source')`, []any{org, onboardingID(t), member, organization["membership_id"]}},
		{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, member}},
	} {
		if err := unitSchemaExec(t, sourceFx, statement.sql, statement.args...); err != nil {
			t.Fatal("synthetic source membership setup failed", err)
		}
	}
	ownerCookie, ownerCSRF := cookie, csrf
	cookie = nil
	csrf = ""
	call("driver company login", "/v1/org-access/session/login", map[string]any{"family_id": driver.Session.FamilyID, "access_token": driver.Session.AccessToken}, 200, nil)
	call("driver cannot manage", base+"/create", create, 403, nil)
	call("SELF has no invented assignment", poweredPath+"/read", map[string]any{}, 403, nil)
	cookie, csrf = ownerCookie, ownerCSRF
	// A failed final COMMIT withholds its provisional ACK and all data/audit.
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_http_fleet_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic fleet receipt outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_http_fleet_failure AFTER INSERT ON org_asset_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_http_fleet_failure()`); err != nil {
		t.Fatal(err)
	}
	rollback := clone(create)
	rollback["operation_id"] = onboardingID(t)
	rollback["plate"] = "KLM4N56"
	rollback["name"] = "Synthetic rollback asset"
	rollback["category"] = map[string]any{"new_label": "Synthetic rollback category"}
	call("failed commit withholds ACK", base+"/create", rollback, 503, nil)
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_assets WHERE org_id=$1 AND plate_key='KLM4N56')AND NOT EXISTS(SELECT 1 FROM org_asset_categories WHERE org_id=$1 AND label='Synthetic rollback category')AND NOT EXISTS(SELECT 1 FROM org_asset_commands WHERE org_id=$1 AND operation_id=$2)`, org, rollback["operation_id"]).Scan(&exact); err != nil || !exact {
		t.Fatal("failed HTTP commit retained provisional effect", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_http_fleet_failure ON org_asset_commands`); err != nil {
		t.Fatal(err)
	}
	// Suspending the sole owner is correctly protected by the real database.
	// Install a second real existing-account owner in this disposable source,
	// after proving that the same membership's earlier DRIVER/SELF was denied.
	if err := unitSchemaExec(t, sourceFx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp()-interval '1 minute',$4,'Synthetic second owner for current suspension')`, org, onboardingID(t), member, organization["membership_id"]); err != nil {
		t.Fatal("second owner source fixture failed", err)
	}
	if err := unitSchemaExec(t, sourceFx, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, organization["membership_id"]); err != nil {
		t.Fatal(err)
	}
	call("current suspended source", poweredPath+"/read", map[string]any{}, 403, nil)
	if err := unitSchemaExec(t, sourceFx, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, organization["membership_id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.flags.Set(t.Context(), scopeFor(t, org), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	call("current capability refused", base+"/create", rollback, 403, nil)
	call("current reads retained with capability off", poweredPath+"/read", map[string]any{}, 200, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call("stale proof retained read", poweredPath+"/read", map[string]any{}, 200, nil)
	call("fresh configuration proof required", base+"/create", rollback, 401, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp()WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call("revoked company source", poweredPath+"/read", map[string]any{}, 401, nil)
	t.Logf("actual private fleet/meter HTTP request assertions: %d; powered activation uses real persisted evidence", requests)
}
