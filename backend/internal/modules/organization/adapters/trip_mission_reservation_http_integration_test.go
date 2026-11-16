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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// B-BR-EMP-414 through the actual composed company entry: request (201 with
// the reservation), replay and reuse, a stale revision, a DRAFT mission, a
// mission without a vehicle, unknown fields, an overlapping confirmed window
// (RESERVATION_CONFLICT with only the window), the fleet confirmation, a
// UNIT-scoped planner on another unit's mission, a driver, an auditor,
// another organization and an unknown mission.
func TestTripMissionReservationRequestThroughComposedCompanyHTTP(t *testing.T) {
	m := missionFixture(t)
	fx := m.deviceFixtureState
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
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
	south := m.secondUnit(t)
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	create := func(unit, asset string, from time.Time) string {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "unit_id": unit, "title": "Entrega sintética",
			"starts_at": from.Format(time.RFC3339Nano), "ends_at": from.Add(2 * time.Hour).Format(time.RFC3339Nano)}
		if asset != "" {
			body["asset_id"] = asset
		}
		id, _ := call(owner, base+"/create", body, 201, "", "")["mission_id"].(string)
		return id
	}
	plan := func(mission string) {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "destination": map[string]any{"kind": "TEXT", "text": "Pátio"},
			"reason": "Entrega", "load": map[string]any{"kind": "NONE"}}
		call(owner, base+"/"+mission+"/plan", body, 200, "", "")
	}
	mission, southern, vehicleless, draft := create(m.unit, m.vehicle.AssetID, start), create(south, m.vehicle.AssetID, start.Add(24*time.Hour)), create(m.unit, "", start), create(m.unit, m.vehicle.AssetID, start)
	for _, id := range []string{mission, southern, vehicleless} {
		plan(id)
	}
	request := func(revision int64) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision, "purpose": "Entrega sintética no cliente"}
	}
	path := base + "/" + mission + "/reservations/request"
	call(owner, base+"/"+draft+"/reservations/request", request(1), 409, "CONFLICT", "")
	call(owner, base+"/"+vehicleless+"/reservations/request", request(2), 422, "VALIDATION_FAILED", "")
	call(owner, path, request(1), 409, "CONFLICT", "")
	blank := request(2)
	blank["purpose"] = ""
	call(owner, path, blank, 422, "VALIDATION_FAILED", "")
	for _, extra := range []string{"asset_id", "employee_id", "starts_at", "confirm"} {
		body := request(2)
		body[extra] = onboardingID(t)
		call(owner, path, body, 400, "MALFORMED", "")
	}
	missing := request(2)
	delete(missing, "revision")
	call(owner, path, missing, 400, "MALFORMED", "")
	call(driver, path, request(2), 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, path, request(2), 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/"+southern+"/reservations/request", request(2), 403, "FORBIDDEN", "trip.mission.manage")
	body := request(2)
	created := call(owner, path, body, 201, "", "")
	reservation, _ := created["reservation_id"].(string)
	if created["state"] != "PLANNED" || created["revision"] != float64(3) || created["replayed"] != false || reservation == "" {
		t.Fatal("reservation request response", created)
	}
	if again := call(owner, path, body, 201, "", ""); again["replayed"] != true || again["reservation_id"] != reservation || again["revision"] != float64(3) {
		t.Fatal("reservation request replay", again)
	}
	reused := request(2)
	reused["operation_id"], reused["purpose"] = body["operation_id"], "Outra"
	call(owner, path, reused, 409, "OPERATION_ID_REUSED", "")
	// The fleet confirmation stays under fleet.reservation.manage.
	confirm := map[string]any{"schema_version": 1, "operation_id": onboardingID(t)}
	confirmed := call(owner, "/v1/orgs/"+org+"/fleet/assets/"+m.vehicle.AssetID+"/reservations/"+reservation+"/confirm", confirm, 200, "", "")
	if confirmed["state"] != "CONFIRMED" {
		t.Fatal("fleet confirmation of the mission's reservation", confirmed)
	}
	// Another mission over the same vehicle and window: only the window.
	overlapping := create(m.unit, m.vehicle.AssetID, start)
	plan(overlapping)
	raw, _ := json.Marshal(request(2))
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+base+"/"+overlapping+"/reservations/request", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	r.AddCookie(owner.cookie)
	r.Header.Set("X-CSRF", owner.csrf)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var conflict map[string]any
	if w.Code != 409 || json.Unmarshal(w.Body.Bytes(), &conflict) != nil {
		t.Fatal("overlapping mission reservation", w.Code)
	}
	failure, _ := conflict["error"].(map[string]any)
	details, _ := failure["details"].([]any)
	if failure["code"] != "RESERVATION_CONFLICT" || len(details) != 2 {
		t.Fatal("conflict refusal", conflict)
	}
	for i, want := range []map[string]any{{"field": "conflict_window", "code": "STARTS_AT", "instant": start.Format(time.RFC3339Nano)},
		{"field": "conflict_window", "code": "ENDS_AT", "instant": start.Add(2 * time.Hour).Format(time.RFC3339Nano)}} {
		got, _ := details[i].(map[string]any)
		if len(got) != 3 || got["field"] != want["field"] || got["code"] != want["code"] || got["instant"] != want["instant"] {
			t.Fatal("conflict reveals more or less than the window", got)
		}
	}
	if strings.Contains(w.Body.String(), reservation) || strings.Contains(w.Body.String(), mission) {
		t.Fatal("conflict revealed the reservation or the mission")
	}
	call(owner, base+"/"+onboardingID(t)+"/reservations/request", request(2), 404, "NOT_FOUND", "")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	call(owner, "/v1/orgs/"+other.scope.OrgID().String()+"/missions/"+mission+"/reservations/request", request(3), 404, "NOT_FOUND", "")
}
