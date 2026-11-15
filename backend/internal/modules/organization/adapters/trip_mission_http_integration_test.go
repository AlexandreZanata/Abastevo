//go:build integration

package adapters_test

import (
	"bytes"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// BUC-EMP-178/180, B-BR-EMP-357 through the actual composed company-cookie
// entry: create, read, list, assign, cancel and the own list; validation,
// stale revision, unknown records, another organization, a UNIT-scoped
// manager, a driver and an auditor.
func TestTripMissionsThroughComposedCompanyHTTP(t *testing.T) {
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
	grantMissionTables(t, fx.legalFixtureState)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{61}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual mission composition failed", err)
	}
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/missions"
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	driver, auditor, manager := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, driver, "DRIVER", "SELF", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, manager, "FLEET_MANAGER", "UNIT", "ACTIVE")
	var north string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 ORDER BY created_at,id LIMIT 1`, org).Scan(&north); err != nil {
		t.Fatal(err)
	}
	south := onboardingID(t)
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic South','SOUTH','UTC','ACTIVE')`, org, south); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	create := func(unit string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "unit_id": unit, "title": "Entrega sintética no cliente",
			"starts_at": start.Format(time.RFC3339Nano), "ends_at": start.Add(2 * time.Hour).Format(time.RFC3339Nano)}
	}
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	createBody := create(north)
	created := call(owner, base+"/create", createBody, 201, "", "")
	mission, _ := created["mission_id"].(string)
	if mission == "" || created["state"] != "DRAFT" || created["revision"] != float64(1) || created["replayed"] != false {
		t.Fatal("mission creation response", created)
	}
	if again := call(owner, base+"/create", createBody, 201, "", ""); again["mission_id"] != mission || again["replayed"] != true {
		t.Fatal("creation replay", again)
	}
	southern, _ := call(owner, base+"/create", create(south), 201, "", "")["mission_id"].(string)
	invalid := create(north)
	invalid["ends_at"] = invalid["starts_at"]
	call(owner, base+"/create", invalid, 422, "VALIDATION_FAILED", "")
	call(owner, base+"/create", create(onboardingID(t)), 404, "NOT_FOUND", "")

	read := call(owner, base+"/"+mission+"/read", map[string]any{}, 200, "", "")
	if read["unit_id"] != north || read["state"] != "DRAFT" || read["title"] != "Entrega sintética no cliente" || read["asset_id"] != nil || read["assignee_employee_id"] != nil {
		t.Fatal("mission read", read)
	}
	if items, _ := call(owner, base+"/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any); len(items) != 2 {
		t.Fatal("organization list", len(items))
	}
	assign := func(revision int64, employee string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision, "assignee_employee_id": employee}
	}
	call(owner, base+"/"+mission+"/assign", assign(1, onboardingID(t)), 404, "NOT_FOUND", "")
	call(owner, base+"/"+mission+"/assign", assign(1, auditor.employee), 422, "VALIDATION_FAILED", "")
	assigned := call(owner, base+"/"+mission+"/assign", assign(1, driver.employee), 200, "", "")
	if assigned["state"] != "ASSIGNED" || assigned["revision"] != float64(2) || assigned["assignee_employee_id"] != driver.employee || assigned["asset_warning"] != nil {
		t.Fatal("assignment response", assigned)
	}
	call(owner, base+"/"+mission+"/assign", assign(1, owner.employee), 409, "CONFLICT", "")
	call(owner, base+"/"+mission+"/assign", assign(2, driver.employee), 409, "CONFLICT", "")

	mine := call(driver, base+"/mine/list", map[string]any{"limit": 10}, 200, "", "")
	if items, _ := mine["items"].([]any); len(items) != 1 || items[0].(map[string]any)["mission_id"] != mission || items[0].(map[string]any)["state"] != "ASSIGNED" {
		t.Fatal("the driver's own list", mine)
	}
	if items, _ := call(owner, base+"/mine/list", map[string]any{}, 200, "", "")["items"].([]any); len(items) != 0 {
		t.Fatal("the owner is assigned nothing", len(items))
	}
	call(driver, base+"/list", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(driver, base+"/"+mission+"/read", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, base+"/list", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, base+"/"+mission+"/read", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, base+"/mine/list", map[string]any{}, 403, "FORBIDDEN", "trip.trip.start")
	call(auditor, base+"/"+mission+"/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "reason": "Auditor"}, 403, "FORBIDDEN", "trip.mission.manage")

	if v := call(manager, base+"/"+mission+"/read", map[string]any{}, 200, "", ""); v["mission_id"] != mission {
		t.Fatal("unit manager reads its unit's mission", v)
	}
	call(manager, base+"/"+southern+"/read", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/"+southern+"/assign", assign(1, driver.employee), 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/create", create(north), 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/list", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(owner, base+"/"+onboardingID(t)+"/read", map[string]any{}, 404, "NOT_FOUND", "")

	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	call(owner, "/v1/orgs/"+other.scope.OrgID().String()+"/missions/"+mission+"/read", map[string]any{}, 404, "NOT_FOUND", "")

	cancelBody := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "reason": ""}
	call(owner, base+"/"+mission+"/cancel", cancelBody, 422, "VALIDATION_FAILED", "")
	cancelBody["reason"] = "Cliente cancelou a entrega"
	cancelled := call(owner, base+"/"+mission+"/cancel", cancelBody, 200, "", "")
	if cancelled["state"] != "CANCELLED" || cancelled["revision"] != float64(3) || cancelled["replayed"] != false {
		t.Fatal("cancellation", cancelled)
	}
	if again := call(owner, base+"/"+mission+"/cancel", cancelBody, 200, "", ""); again["replayed"] != true || again["revision"] != float64(3) {
		t.Fatal("cancellation replay", again)
	}
	call(owner, base+"/"+mission+"/assign", assign(3, driver.employee), 409, "CONFLICT", "")
	reused := map[string]any{"schema_version": 1, "operation_id": cancelBody["operation_id"], "revision": 2, "reason": "Outro motivo"}
	call(owner, base+"/"+mission+"/cancel", reused, 409, "OPERATION_ID_REUSED", "")
}
