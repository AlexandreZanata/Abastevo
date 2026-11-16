//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-010 through the actual composed company routes: the planner plans
// stops (refusals, replay, reuse, the plan projections), the member reads the
// plan beside the observed stops and legs of an own trip, and another member,
// another organization, an unknown trip, an auditor and malformed requests
// are refused with no write.
func TestTripStopsThroughComposedCompanyHTTP(t *testing.T) {
	x := tripStopsFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{43}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual stops composition failed", err)
	}
	fx := x.deviceFixtureState
	owner, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	d, d2 := x.assignedDriver(t), x.assignedDriver(t)
	org := x.scope.OrgID().String()
	missions, trips := "/v1/orgs/"+org+"/missions", "/v1/orgs/"+org+"/trips"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	created := call(owner, missions+"/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "unit_id": x.unit, "asset_id": x.vehicle.AssetID,
		"title": "Entrega sintética", "starts_at": start.Format(time.RFC3339Nano), "ends_at": start.Add(4 * time.Hour).Format(time.RFC3339Nano)}, 201, "", "")
	mission, _ := created["mission_id"].(string)
	stop := func(text string) map[string]any { return map[string]any{"kind": "TEXT", "text": text} }
	plan := func(revision int64, stops any) map[string]any {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision,
			"destination": map[string]any{"kind": "TEXT", "text": "Centro de distribuição Norte"}, "reason": "Entrega de peças", "load": map[string]any{"kind": "NONE"}}
		if stops != nil {
			body["stops"] = stops
		}
		return body
	}
	twentyOne := []any{}
	for range 21 {
		twentyOne = append(twentyOne, stop("Parada"))
	}
	for name, c := range map[string]struct {
		stops  any
		status int
		code   string
	}{
		"empty-list":        {[]any{}, 422, "VALIDATION_FAILED"},
		"twenty-one":        {twentyOne, 422, "VALIDATION_FAILED"},
		"unit-stop":         {[]any{map[string]any{"kind": "UNIT", "text": "Pátio"}}, 422, "VALIDATION_FAILED"},
		"cpf-shaped-stop":   {[]any{stop("Contato 123.456.789-09")}, 422, "VALIDATION_FAILED"},
		"blank-stop":        {[]any{stop("  ")}, 422, "VALIDATION_FAILED"},
		"missing-text":      {[]any{map[string]any{"kind": "TEXT"}}, 422, "VALIDATION_FAILED"},
		"coordinates":       {[]any{map[string]any{"kind": "TEXT", "text": "Pátio", "latitude": -15.6}}, 400, "MALFORMED"},
		"planned-time":      {[]any{map[string]any{"kind": "TEXT", "text": "Pátio", "arrive_at": "2026-11-15T13:00:00Z"}}, 400, "MALFORMED"},
		"bare-string-stops": {[]any{"Pátio"}, 400, "MALFORMED"},
	} {
		t.Run(name, func(t *testing.T) {
			call(owner, missions+"/"+mission+"/plan", plan(1, c.stops), c.status, c.code, "")
		})
	}
	if rows := x.planStopRows(t, mission); len(rows) != 0 {
		t.Fatal("a refused plan stored stops", rows)
	}
	body := plan(1, []any{stop(bucStops[0]), stop(bucStops[1]), stop(bucStops[2])})
	planned := call(owner, missions+"/"+mission+"/plan", body, 200, "", "")
	if planned["state"] != "PLANNED" || planned["revision"] != float64(2) || planned["replayed"] != false {
		t.Fatal("plan with stops", planned)
	}
	if again := call(owner, missions+"/"+mission+"/plan", body, 200, "", ""); again["replayed"] != true || again["revision"] != float64(2) {
		t.Fatal("plan replay", again)
	}
	reused := plan(1, []any{stop("Outro lugar")})
	reused["operation_id"] = body["operation_id"]
	call(owner, missions+"/"+mission+"/plan", reused, 409, "OPERATION_ID_REUSED", "")
	read := call(owner, missions+"/"+mission+"/read", map[string]any{}, 200, "", "")
	raw, _ := json.Marshal(read["plan"].(map[string]any)["stops"])
	if want := `[{"kind":"TEXT","sequence":1,"text":"Cliente Jardim Primavera"},{"kind":"TEXT","sequence":2,"text":"Posto de apoio BR-163"},{"kind":"TEXT","sequence":3,"text":"Cliente Vila Rica"}]`; string(raw) != want {
		t.Fatal("the plan projection lost its stops", string(raw))
	}
	call(owner, missions+"/"+mission+"/assign", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 2, "assignee_employee_id": d.person.employee}, 200, "", "")
	if ack, err := x.m.deliver(t, d, x.m.respondOp(t, d, mission, 3, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("acceptance", err)
	}
	// The executing mission's plan cannot change (B-BR-EMP-413).
	startOp := x.m.missionStartOp(t, d, mission, x.declaredNow(t))
	ack, err := x.process(t, d, startOp)
	x.eventState.tripState.accepted(t, startOp, ack, err, 1, 1)
	trip := tripOf(startOp)
	call(owner, missions+"/"+mission+"/plan", plan(5, []any{stop("Parada nova")}), 409, "CONFLICT", "")
	for _, kind := range []string{"ARRIVAL", "DEPARTURE"} {
		op := x.eventOp(t, d, trip, kind, nil, x.declaredNow(t), nil)
		ack, err := x.process(t, d, op)
		x.eventState.accepted(t, op, ack, err)
	}
	finish := x.finishOp(t, d, trip, x.declaredNow(t), nil)
	ack, err = x.process(t, d, finish)
	x.eventState.tripState.accepted(t, finish, ack, err, 2, 1)

	member := handoverLogin(t, d, router)
	path := trips + "/mine/" + trip + "/stops/read"
	view := call(member, path, map[string]any{}, 200, "", "")
	if view["trip_id"] != trip || view["operational_state"] != "COMPLETED" || view["data_mode"] != "SYNTHETIC" || view["end"].(map[string]any)["kind"] != "FINISH" {
		t.Fatal("trip summary", view)
	}
	p, _ := view["planned"].(map[string]any)
	if p == nil || p["mission_id"] != mission || p["revision"] != float64(2) || len(p["stops"].([]any)) != 3 || len(p["legs"].([]any)) != 4 {
		t.Fatal("planned side", p)
	}
	observed := view["observed"].(map[string]any)
	stops, legs := observed["stops"].([]any), observed["legs"].([]any)
	if len(stops) != 1 || len(legs) != 2 || len(observed["after_end"].([]any)) != 0 || len(observed["held"].([]any)) != 0 {
		t.Fatal("observed side", observed)
	}
	first := stops[0].(map[string]any)
	arrival := first["arrival"].(map[string]any)
	if arrival["kind"] != "ARRIVAL" || arrival["entry_zone"] != "America/Cuiaba" || arrival["device_offset_seconds"] != float64(-14400) || arrival["after_receipt"] != false ||
		arrival["received_at"] == "" || first["departure"].(map[string]any)["kind"] != "DEPARTURE" || first["issue"] != nil {
		t.Fatal("observed stop with its recorded times", first)
	}
	if legs[0].(map[string]any)["departure"].(map[string]any)["kind"] != "START" || legs[1].(map[string]any)["arrival"].(map[string]any)["kind"] != "FINISH" {
		t.Fatal("observed legs", legs)
	}
	for _, leg := range append(append([]any{}, legs...), p["legs"].([]any)...) {
		for key := range leg.(map[string]any) {
			if strings.Contains(key, "distance") || strings.Contains(key, "duration") {
				t.Fatal("a leg carries an estimate", key)
			}
		}
	}
	other := handoverLogin(t, d2, router)
	call(other, path, map[string]any{}, 404, "NOT_FOUND", "")
	call(member, trips+"/mine/"+onboardingID(t)+"/stops/read", map[string]any{}, 404, "NOT_FOUND", "")
	call(member, trips+"/mine/not-a-trip/stops/read", map[string]any{}, 404, "NOT_FOUND", "")
	call(member, "/v1/orgs/"+onboardingID(t)+"/trips/mine/"+trip+"/stops/read", map[string]any{}, 404, "NOT_FOUND", "")
	call(member, path, map[string]any{"membership_id": d2.person.member}, 400, "MALFORMED", "")
	call(auditor, path, map[string]any{}, 403, "FORBIDDEN", "trip.trip.start")
	if again := call(member, path, map[string]any{}, 200, "", ""); !jsonEqual(t, again, view) {
		t.Fatal("a refused request changed the member's read")
	}
}

func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}
