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

// BUC-EMP-192..194, B-BR-EMP-381..385 through the actual composed company
// entry: plan, replay, the plan in read, list and the own list, unknown
// fields (a passenger identity), CPF-shaped text, foreign and archived
// references, a stale revision, an accepted-equivalent refusal, another
// organization, a UNIT-scoped planner, a driver and an auditor.
func TestTripMissionPlansThroughComposedCompanyHTTP(t *testing.T) {
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
	grantMissionTables(t, fx.legalFixtureState)
	grantCostCentreTables(t, fx.legalFixtureState)
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
	south, archivedUnit, centre, archivedCentre := onboardingID(t), onboardingID(t), onboardingID(t), onboardingID(t)
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic South','SOUTH','UTC','ACTIVE')`, []any{org, south}},
		{`INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic West','WEST','UTC','ACTIVE')`, []any{org, archivedUnit}},
		{`UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, archivedUnit}},
		{`INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic client contract','CTR-1','ACTIVE')`, []any{org, centre}},
		{`INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic old contract','CTR-0','ACTIVE')`, []any{org, archivedCentre}},
		{`UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, archivedCentre}},
	} {
		if err := unitSchemaExec(t, fx.legalFixtureState, q.query, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	create := func(unit string) string {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "unit_id": unit, "title": "Entrega sintética",
			"starts_at": start.Format(time.RFC3339Nano), "ends_at": start.Add(2 * time.Hour).Format(time.RFC3339Nano)}
		id, _ := call(owner, base+"/create", body, 201, "", "")["mission_id"].(string)
		return id
	}
	mission, southern := create(north), create(south)
	plan := func(revision int64, change func(map[string]any)) map[string]any {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision,
			"destination": map[string]any{"kind": "TEXT", "text": "Centro de distribuição Norte"}, "reason": "Entrega de peças de reposição", "cost_centre_id": centre,
			"load": map[string]any{"kind": "CARGO", "description": "Peças de reposição"}}
		if change != nil {
			change(body)
		}
		return body
	}
	for name, c := range map[string]struct {
		change func(map[string]any)
		status int
		code   string
	}{
		"passenger-identity-field": {func(b map[string]any) {
			b["load"] = map[string]any{"kind": "PASSENGERS", "count": 2, "names": []any{"Pessoa sintética"}}
		}, 400, "MALFORMED"},
		"customer-person-field": {func(b map[string]any) { b["customer_name"] = "Pessoa sintética" }, 400, "MALFORMED"},
		"cpf-in-destination": {func(b map[string]any) {
			b["destination"] = map[string]any{"kind": "TEXT", "text": "Casa 12345678909"}
		}, 422, "VALIDATION_FAILED"},
		"passenger-description": {func(b map[string]any) {
			b["load"] = map[string]any{"kind": "PASSENGERS", "count": 2, "description": "Equipe de manutenção"}
		}, 422, "VALIDATION_FAILED"},
		"text-and-unit": {func(b map[string]any) {
			b["destination"] = map[string]any{"kind": "TEXT", "text": "Pátio", "unit_id": south}
		}, 422, "VALIDATION_FAILED"},
		"foreign-or-unknown-unit": {func(b map[string]any) {
			b["destination"] = map[string]any{"kind": "UNIT", "unit_id": onboardingID(t)}
		}, 404, "NOT_FOUND"},
		"archived-unit": {func(b map[string]any) {
			b["destination"] = map[string]any{"kind": "UNIT", "unit_id": archivedUnit}
		}, 422, "VALIDATION_FAILED"},
		"unknown-cost-centre":  {func(b map[string]any) { b["cost_centre_id"] = onboardingID(t) }, 404, "NOT_FOUND"},
		"archived-cost-centre": {func(b map[string]any) { b["cost_centre_id"] = archivedCentre }, 422, "VALIDATION_FAILED"},
		"stale-revision":       {func(b map[string]any) { b["revision"] = 2 }, 409, "CONFLICT"},
		"missing-revision":     {func(b map[string]any) { delete(b, "revision") }, 400, "MALFORMED"},
	} {
		t.Run(name, func(t *testing.T) {
			call(owner, base+"/"+mission+"/plan", plan(1, c.change), c.status, c.code, "")
		})
	}
	body := plan(1, nil)
	planned := call(owner, base+"/"+mission+"/plan", body, 200, "", "")
	if planned["state"] != "PLANNED" || planned["revision"] != float64(2) || planned["replayed"] != false || planned["assignee_employee_id"] != nil {
		t.Fatal("plan response", planned)
	}
	if again := call(owner, base+"/"+mission+"/plan", body, 200, "", ""); again["replayed"] != true || again["revision"] != float64(2) {
		t.Fatal("plan replay", again)
	}
	reused := plan(2, nil)
	reused["operation_id"] = body["operation_id"]
	call(owner, base+"/"+mission+"/plan", reused, 409, "OPERATION_ID_REUSED", "")
	read := call(owner, base+"/"+mission+"/read", map[string]any{}, 200, "", "")
	got, _ := read["plan"].(map[string]any)
	if read["state"] != "PLANNED" || got == nil || got["revision"] != float64(2) || got["reason"] != "Entrega de peças de reposição" || got["cost_centre_id"] != centre ||
		got["destination"].(map[string]any)["text"] != "Centro de distribuição Norte" || got["load"].(map[string]any)["kind"] != "CARGO" || got["planned_at"] == nil {
		t.Fatal("the plan in the mission read", read)
	}
	assigned := call(owner, base+"/"+mission+"/assign", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "assignee_employee_id": driver.employee}, 200, "", "")
	if assigned["state"] != "ASSIGNED" || assigned["revision"] != float64(3) {
		t.Fatal("assignment of the planned mission", assigned)
	}
	mine := call(driver, base+"/mine/list", map[string]any{"limit": 10}, 200, "", "")
	if items, _ := mine["items"].([]any); len(items) != 1 || items[0].(map[string]any)["plan"] == nil {
		t.Fatal("the assignee's own list shows the plan", mine)
	}
	if items, _ := call(owner, base+"/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any); len(items) != 2 {
		t.Fatal("organization list", len(items))
	}
	// Re-planning the assigned mission with a unit destination and passengers
	// withdraws the assignment.
	replanned := call(owner, base+"/"+mission+"/plan", plan(3, func(b map[string]any) {
		b["destination"] = map[string]any{"kind": "UNIT", "unit_id": south}
		b["load"] = map[string]any{"kind": "PASSENGERS", "count": 4}
		delete(b, "cost_centre_id")
	}), 200, "", "")
	if replanned["state"] != "PLANNED" || replanned["revision"] != float64(4) {
		t.Fatal("re-plan", replanned)
	}
	if items, _ := call(driver, base+"/mine/list", map[string]any{}, 200, "", "")["items"].([]any); len(items) != 0 {
		t.Fatal("a withdrawn assignment stays in the own list", len(items))
	}

	call(driver, base+"/"+mission+"/plan", plan(4, nil), 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, base+"/"+mission+"/plan", plan(4, nil), 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/"+southern+"/plan", plan(1, nil), 403, "FORBIDDEN", "trip.mission.manage")
	if v := call(manager, base+"/"+mission+"/plan", plan(4, nil), 200, "", ""); v["revision"] != float64(5) {
		t.Fatal("the unit planner plans its unit's mission", v)
	}
	call(owner, base+"/"+onboardingID(t)+"/plan", plan(1, nil), 404, "NOT_FOUND", "")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	call(owner, "/v1/orgs/"+other.scope.OrgID().String()+"/missions/"+mission+"/plan", plan(5, nil), 404, "NOT_FOUND", "")
	cancel := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 5, "reason": "Cliente cancelou a entrega"}
	call(owner, base+"/"+mission+"/cancel", cancel, 200, "", "")
	call(owner, base+"/"+mission+"/plan", plan(6, nil), 409, "CONFLICT", "")
	if v := call(owner, base+"/"+mission+"/read", map[string]any{}, 200, "", ""); v["state"] != "CANCELLED" || v["plan"].(map[string]any)["revision"] != float64(5) {
		t.Fatal("the cancelled mission keeps its last plan", v)
	}
}
