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

// BUC-EMP-195/196, B-BR-EMP-387 through the actual composed company entry:
// change, replay and reuse, the new vehicle in the read, unknown and foreign
// vehicles, a draft vehicle, the current vehicle again, a stale revision,
// unknown fields, another organization, a UNIT-scoped planner on another
// unit's mission, a driver and an auditor.
func TestTripMissionVehicleChangeThroughComposedCompanyHTTP(t *testing.T) {
	m := missionFixture(t)
	fx := m.deviceFixtureState
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	// The composed FLEET-007 assessment of the vehicle change reads open
	// work-order holds (EMP-MAINT-005), like the production runtime role.
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_work_orders TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
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
	south := m.secondUnit(t)
	first, second, draft := m.vehicle.AssetID, m.otherVehicle(t, "").AssetID, m.draftAsset(t)
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	create := func(unit, asset string) string {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "unit_id": unit, "title": "Entrega sintética", "asset_id": asset,
			"starts_at": start.Format(time.RFC3339Nano), "ends_at": start.Add(2 * time.Hour).Format(time.RFC3339Nano)}
		id, _ := call(owner, base+"/create", body, 201, "", "")["mission_id"].(string)
		return id
	}
	mission, southern := create(m.unit, first), create(south, first)
	change := func(revision int64, asset string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision, "asset_id": asset, "reason": "Veículo em manutenção"}
	}
	path := base + "/" + mission + "/vehicle/change"
	call(owner, path, change(1, second), 409, "CONFLICT", "")
	plan := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "destination": map[string]any{"kind": "TEXT", "text": "Pátio"},
		"reason": "Entrega", "load": map[string]any{"kind": "NONE"}}
	if v := call(owner, base+"/"+mission+"/plan", plan, 200, "", ""); v["revision"] != float64(2) {
		t.Fatal("plan before the vehicle change", v)
	}
	plan["operation_id"] = onboardingID(t)
	call(owner, base+"/"+southern+"/plan", plan, 200, "", "")
	call(owner, path, change(2, onboardingID(t)), 404, "NOT_FOUND", "")
	call(owner, path, change(2, draft), 422, "VALIDATION_FAILED", "")
	call(owner, path, change(2, m.installed.AssetID), 422, "VALIDATION_FAILED", "")
	call(owner, path, change(2, first), 409, "CONFLICT", "")
	call(owner, path, change(3, second), 409, "CONFLICT", "")
	blank := change(2, second)
	blank["reason"] = ""
	call(owner, path, blank, 422, "VALIDATION_FAILED", "")
	extra := change(2, second)
	extra["meter_id"] = onboardingID(t)
	call(owner, path, extra, 400, "MALFORMED", "")
	missing := change(2, second)
	delete(missing, "revision")
	call(owner, path, missing, 400, "MALFORMED", "")
	body := change(2, second)
	changed := call(owner, path, body, 200, "", "")
	if changed["state"] != "PLANNED" || changed["revision"] != float64(3) || changed["replayed"] != false {
		t.Fatal("vehicle change response", changed)
	}
	if again := call(owner, path, body, 200, "", ""); again["replayed"] != true || again["revision"] != float64(3) {
		t.Fatal("change replay", again)
	}
	reused := change(2, first)
	reused["operation_id"] = body["operation_id"]
	call(owner, path, reused, 409, "OPERATION_ID_REUSED", "")
	if read := call(owner, base+"/"+mission+"/read", map[string]any{}, 200, "", ""); read["asset_id"] != second || read["revision"] != float64(3) {
		t.Fatal("the mission reads its new vehicle", read)
	}
	call(driver, path, change(3, first), 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, path, change(3, first), 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/"+southern+"/vehicle/change", change(2, second), 403, "FORBIDDEN", "trip.mission.manage")
	if v := call(manager, path, change(3, first), 200, "", ""); v["revision"] != float64(4) {
		t.Fatal("the unit planner changes the vehicle of its unit's mission", v)
	}
	call(owner, base+"/"+onboardingID(t)+"/vehicle/change", change(2, first), 404, "NOT_FOUND", "")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	call(owner, "/v1/orgs/"+other.scope.OrgID().String()+"/missions/"+mission+"/vehicle/change", change(4, second), 404, "NOT_FOUND", "")
}
