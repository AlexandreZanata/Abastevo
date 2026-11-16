//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

// handoverLogin is a device member's company session on the composed router.
func handoverLogin(t *testing.T, d syncFixtureState, router chi.Router) matrixPerson {
	t.Helper()
	p := d.person
	raw, err := json.Marshal(map[string]any{"family_id": p.auth.Session.FamilyID, "access_token": p.auth.Session.AccessToken})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+"/v1/org-access/session/login", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var result struct {
		CSRF string `json:"csrf_token"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.CSRF == "" {
		t.Fatal("device member company login", w.Code, w.Body.String())
	}
	person := matrixPerson{account: p.auth.Account.ID, member: p.member, employee: p.employee, csrf: result.CSRF}
	for _, c := range w.Result().Cookies() {
		if c.Value != "" {
			person.cookie = c
		}
	}
	return person
}

// EMP-TRIP-012 through the actual composed company routes: the handover, the
// organization's handover list and the receiver's own open trips, with the
// bounded refusals (FORBIDDEN, SELF_APPROVAL, CONFLICT, VALIDATION_FAILED,
// MALFORMED, OPERATION_ID_REUSED, NOT_FOUND) and no write on refusal.
func TestTripHandoverThroughComposedCompanyHTTP(t *testing.T) {
	x := tripHandoverFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{41}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual handover composition failed", err)
	}
	fx := x.deviceFixtureState
	supervisor, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, supervisor, "SUPERVISOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	d1, d2 := x.assignedDriver(t), x.assignedDriver(t)
	at := x.declaredNow(t)
	start := x.startOp(t, d1, at, absentReading)
	ack, err := x.process(t, d1, start)
	x.eventState.tripState.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	org := x.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/trips"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	body := func(revision any, receiver, reason, op string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": op, "revision": revision, "receiver_membership_id": receiver, "reason": reason}
	}
	handover := base + "/" + trip + "/handover"
	call(auditor, handover, body(1, d2.person.member, "Troca de turno", onboardingID(t)), 403, "FORBIDDEN", "trip.trip.close_for_other")
	call(auditor, base+"/handovers/list", map[string]any{"limit": 10}, 403, "FORBIDDEN", "trip.trip.close_for_other")
	call(auditor, base+"/mine/list", map[string]any{"limit": 10}, 403, "FORBIDDEN", "trip.trip.start")
	call(supervisor, handover, body(1, onboardingID(t), "Troca de turno", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(supervisor, handover, body(1, d2.person.member, " ", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(supervisor, handover, body(2, d2.person.member, "Troca de turno", onboardingID(t)), 409, "CONFLICT", "")
	call(supervisor, handover, body(1, supervisor.member, "Assumo a viagem", onboardingID(t)), 409, "SELF_APPROVAL", "")
	call(supervisor, handover, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "receiver_membership_id": d2.person.member, "reason": "Troca de turno"}, 400, "MALFORMED", "")
	call(supervisor, handover, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "receiver_membership_id": d2.person.member, "reason": "Troca de turno",
		"handed_over_at": time.Now().UTC().Format(time.RFC3339)}, 400, "MALFORMED", "")
	call(supervisor, "/v1/orgs/"+onboardingID(t)+"/trips/"+trip+"/handover", body(1, d2.person.member, "Troca de turno", onboardingID(t)), 404, "NOT_FOUND", "")
	call(supervisor, base+"/"+onboardingID(t)+"/handover", body(1, d2.person.member, "Troca de turno", onboardingID(t)), 404, "NOT_FOUND", "")
	if c := x.handoverCount(t, trip, onboardingID(t)); c.handovers != 0 {
		t.Fatalf("a refused HTTP handover wrote state: %+v", c)
	}
	op := onboardingID(t)
	done := call(supervisor, handover, body(1, d2.person.member, "Troca de turno", op), 200, "", "")
	fact, _ := done["handover"].(map[string]any)
	if done["replayed"] != false || done["revision"] != float64(2) || done["operational_state"] != "IN_PROGRESS" || done["trip_id"] != trip || done["data_mode"] != "SYNTHETIC" ||
		fact["to_membership_id"] != d2.person.member || fact["from_membership_id"] != d1.person.member || fact["recorder_membership_id"] != supervisor.member ||
		fact["reason"] != "Troca de turno" || fact["revision"] != float64(2) || fact["handed_over_at"] == "" {
		t.Fatal("handover answer", done)
	}
	replay := call(supervisor, handover, body(1, d2.person.member, "Troca de turno", op), 200, "", "")
	first, _ := json.Marshal(done["handover"])
	again, _ := json.Marshal(replay["handover"])
	if replay["replayed"] != true || !bytes.Equal(first, again) {
		t.Fatal("replayed handover", replay)
	}
	call(supervisor, handover, body(1, d2.person.member, "Outro motivo", op), 409, "OPERATION_ID_REUSED", "")
	call(supervisor, handover, body(1, d2.person.member, "Troca de turno", onboardingID(t)), 409, "CONFLICT", "")
	items, _ := call(supervisor, base+"/handovers/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["trip_id"] != trip || items[0].(map[string]any)["asset_id"] != x.vehicle.AssetID {
		t.Fatal("handover list", items)
	}
	receiver := handoverLogin(t, d2, router)
	own, _ := call(receiver, base+"/mine/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any)
	if len(own) != 1 || own[0].(map[string]any)["trip_id"] != trip || own[0].(map[string]any)["revision"] != float64(2) || own[0].(map[string]any)["handover"] == nil ||
		own[0].(map[string]any)["operational_state"] != "IN_PROGRESS" {
		t.Fatal("the receiver's own open trips", own)
	}
	giver := handoverLogin(t, d1, router)
	if mine, _ := call(giver, base+"/mine/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any); len(mine) != 0 {
		t.Fatal("the giving driver still lists the trip", mine)
	}
	call(supervisor, base+"/mine/list", map[string]any{"limit": 10, "extra": true}, 400, "MALFORMED", "")
}
