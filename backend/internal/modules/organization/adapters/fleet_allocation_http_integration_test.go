//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// BUC-EMP-114 through the actual composed company-cookie entry, plus the
// two-organization reference and port isolation on real PostGIS.
func TestFleetAllocationHistoryThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	s := lifecycleStore(t, fx)
	_ = meterStore(t, fx)
	grantCostCentreTables(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+";GRANT UPDATE ON org_asset_unit_periods,org_asset_cost_centre_periods TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synallocationhttp")
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
		t.Fatal("actual allocation composition failed", err)
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
			t.Fatalf("%s: private allocation HTTP got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && strings.Contains(w.Body.String(), "Synthetic") {
			t.Fatal("private allocation content entered refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private allocation response")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookie = w.Result().Cookies()[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	login := func(familyID, token string) {
		cookie, csrf = nil, ""
		call("company login", "/v1/org-access/session/login", map[string]any{"family_id": familyID, "access_token": token}, 200)
	}
	login(auth.Session.FamilyID, auth.Session.AccessToken)
	createOrg := func() map[string]any {
		t.Helper()
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic allocation organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic Norte", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
		id := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, id), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		call("organization activation", "/v1/orgs/"+id+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return result
	}
	organization, other := createOrg(), createOrg()
	org, otherOrg := organization["organization_id"].(string), other["organization_id"].(string)
	scopeA, scopeB := scopeFor(t, org), scopeFor(t, otherOrg)
	base, otherBase := "/v1/orgs/"+org+"/fleet/assets", "/v1/orgs/"+otherOrg+"/fleet/assets"
	asset := func(prefix, unit, plate string) string {
		t.Helper()
		created := call("draft", prefix+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic allocation implement", "plate": plate, "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic allocation category"}, "unit_id": unit}, 201)
		id := created["asset_id"].(string)
		call("activate", prefix+"/"+id+"/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return id
	}
	norte := organization["unit_id"].(string)
	v := asset(base, norte, "ALC1A23")
	foreign := asset(otherBase, other["unit_id"].(string), "ALC1A23")
	units, centres := base+"/"+v+"/unit-allocations/list", base+"/"+v+"/cost-centre-allocations/list"
	page := call("initial unit history", units, map[string]any{}, 200)
	items := page["items"].([]any)
	if len(items) != 1 || page["next_cursor"] != "" {
		t.Fatal("initial unit history", page)
	}
	initial := items[0].(map[string]any)
	if initial["unit_id"] != norte || initial["valid_from"] == nil || initial["recorded_at"] == nil || len(initial) != 3 {
		t.Fatal("initial unit period projection", initial)
	}
	if empty := call("empty cost-centre history", centres, map[string]any{"limit": 5}, 200); len(empty["items"].([]any)) != 0 || empty["next_cursor"] != "" {
		t.Fatal("invented cost-centre history", empty)
	}
	// Test-only change of allocation through the real guards (FLEET-008 owns
	// the public transfer command).
	sulID, centre := onboardingID(t), onboardingID(t)
	inA := func(sql string, args ...any) error {
		return database.WithOrgTx(t.Context(), fx.f.Runtime, scopeA, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
	}
	if err := inA(`INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic Sul','SUL','UTC','ACTIVE')`, org, sulID); err != nil {
		t.Fatal(err)
	}
	if err := inA(`INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic centre','CC-SUL','ACTIVE')`, org, centre); err != nil {
		t.Fatal(err)
	}
	change := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, scopeA, func(tx pgx.Tx) error {
		for _, statement := range []struct {
			sql    string
			target any
		}{
			{`UPDATE org_asset_unit_periods SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2 AND valid_to IS NULL`, nil},
			{`INSERT INTO org_asset_unit_periods(org_id,asset_id,valid_from,unit_id) VALUES($1,$2,$3,$4)`, sulID},
			{`INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,valid_from,cost_centre_id) VALUES($1,$2,$3,$4)`, centre},
		} {
			args := []any{org, v, change}
			if statement.target != nil {
				args = append(args, statement.target)
			}
			if _, err := tx.Exec(t.Context(), statement.sql, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal("test-only reallocation", err)
	}
	first := call("unit history first page", units, map[string]any{"limit": 1}, 200)
	firstItems := first["items"].([]any)
	if len(firstItems) != 1 || first["next_cursor"] == "" || firstItems[0].(map[string]any)["unit_id"] != norte || firstItems[0].(map[string]any)["valid_to"] != change.Format(time.RFC3339Nano) {
		t.Fatal("closed Norte period", first)
	}
	rest := call("unit history continuation", units, map[string]any{"limit": 1, "cursor": first["next_cursor"]}, 200)
	restItems := rest["items"].([]any)
	if len(restItems) != 1 || rest["next_cursor"] != "" || restItems[0].(map[string]any)["unit_id"] != sulID || restItems[0].(map[string]any)["valid_from"] != change.Format(time.RFC3339Nano) || restItems[0].(map[string]any)["valid_to"] != nil {
		t.Fatal("open Sul period", rest)
	}
	if c := call("cost-centre history", centres, map[string]any{}, 200); len(c["items"].([]any)) != 1 || c["items"].([]any)[0].(map[string]any)["cost_centre_id"] != centre {
		t.Fatal("cost-centre period", c)
	}
	call("cursor bound to route", centres, map[string]any{"limit": 1, "cursor": first["next_cursor"]}, 400)
	call("cursor bound to limit", units, map[string]any{"limit": 2, "cursor": first["next_cursor"]}, 400)
	call("cursor bound to asset", base+"/"+onboardingID(t)+"/unit-allocations/list", map[string]any{"limit": 1, "cursor": first["next_cursor"]}, 400)
	call("caller-selected instant", units, map[string]any{"at": change.Format(time.RFC3339Nano)}, 400)
	call("missing asset hidden", base+"/"+onboardingID(t)+"/unit-allocations/list", map[string]any{}, 404)
	call("foreign asset hidden", base+"/"+foreign+"/unit-allocations/list", map[string]any{}, 404)
	call("asset hidden from foreign organization", otherBase+"/"+v+"/cost-centre-allocations/list", map[string]any{}, 404)
	// Same-organization references: a foreign unit or cost centre is refused.
	foreignCentre := onboardingID(t)
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, scopeB, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic foreign centre','CC-B','ACTIVE')`, otherOrg, foreignCentre)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	later := lifecycleDBNow(t, fx).Truncate(time.Microsecond)
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, scopeA, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE org_asset_unit_periods SET valid_to=$3 WHERE org_id=$1 AND asset_id=$2 AND valid_to IS NULL`, org, v, later); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_asset_unit_periods(org_id,asset_id,unit_id,valid_from) VALUES($1,$2,$3,$4)`, org, v, other["unit_id"], later)
		return err
	})
	allocationWant(t, "unit of another organization", err, "23503")
	allocationWant(t, "cost centre of another organization", inA(`INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, org, v, foreignCentre, later.Add(time.Hour)), "23503")
	allocationWant(t, "period of another organization's asset", inA(`INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, org, foreign, centre, later), "23503")
	allocationWant(t, "row of another organization", inA(`INSERT INTO org_asset_cost_centre_periods(org_id,asset_id,cost_centre_id,valid_from) VALUES($1,$2,$3,$4)`, otherOrg, foreign, foreignCentre, later), "42501")
	if got, err := s.UnitAndCostCentreAt(t.Context(), scopeA, v, change); err != nil || got.UnitID != sulID || got.CostCentreID != centre {
		t.Fatal("port in the owning organization", err)
	}
	if _, err := s.UnitAndCostCentreAt(t.Context(), scopeB, v, change); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("port answered across organizations", err)
	}
	if got, err := s.UnitAndCostCentreAt(t.Context(), scopeB, foreign, change); err != nil || got.UnitID != other["unit_id"] || got.CostCentreID != "" {
		t.Fatal("foreign organization's own snapshot", err)
	}
	// A real FINANCE member reads the history; a real DRIVER/SELF does not.
	member := func(label, roleName, scopeType string) (string, string) {
		t.Helper()
		key, err := accounts.CreateKeyAccount(t.Context(), "synallocation"+strings.ToLower(roleName))
		if err != nil {
			t.Fatal(err)
		}
		login, err := accounts.LoginWithKey(t.Context(), key.Key)
		if err != nil {
			t.Fatal(err)
		}
		membership, employee := onboardingID(t), onboardingID(t)
		target := org
		if scopeType == "SELF" {
			target = membership
		}
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{org, employee}},
			{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, membership, employee, login.Account.ID}},
			{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()-interval '1 minute',$7,'Synthetic allocation reader')`, []any{org, onboardingID(t), membership, roleName, scopeType, target, organization["membership_id"]}},
			{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, membership}},
		} {
			if err := inA(statement.sql, statement.args...); err != nil {
				t.Fatal(label, "synthetic membership setup failed", err)
			}
		}
		return login.Session.FamilyID, login.Session.AccessToken
	}
	financeFamily, financeToken := member("finance", "FINANCE", "ORG")
	driverFamily, driverToken := member("driver", "DRIVER", "SELF")
	login(financeFamily, financeToken)
	if f := call("finance reads unit history", units, map[string]any{"limit": 10}, 200); len(f["items"].([]any)) != 2 {
		t.Fatal("finance unit history", f)
	}
	call("finance reads cost-centre history", centres, map[string]any{}, 200)
	login(driverFamily, driverToken)
	denied := call("driver without assignment", units, map[string]any{}, 403)
	if detail := denied["error"].(map[string]any)["details"].([]any); len(detail) != 1 || detail[0].(map[string]any)["code"] != "fleet.asset.read" {
		t.Fatal("driver denial did not name the missing permission", denied)
	}
	t.Logf("actual private allocation HTTP request assertions: %d", requests)
}
