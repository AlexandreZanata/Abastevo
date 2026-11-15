//go:build integration

package adapters_test

import (
	"bytes"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-015 through the actual composed company routes: the proposal
// under meter.reading.correct, the decision context with the effective
// revision and each amendment's usage, the approval through the decide
// route, and the bounded refusals (FORBIDDEN, MALFORMED, VALIDATION_FAILED,
// CONFLICT, PERIOD_LOCKED, NOT_FOUND).
func TestTripAmendmentThroughComposedCompanyHTTP(t *testing.T) {
	x := tripAmendFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{37}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual trip amendment composition failed", err)
	}
	fx := x.deviceFixtureState
	owner, manager, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, manager, "FLEET_MANAGER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	d, at := x.assignedDriver(t)
	trip := x.measuredTrip(t, d, at)
	base := "/v1/orgs/" + x.scope.OrgID().String() + "/trips/" + trip
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	decision := func(revision int, kind, reason string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision, "decision": kind, "reason": reason}
	}
	proposal := func(revision int, value, reason string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision, "reason": reason, "end_reading": map[string]any{"kind": "ODOMETER", "value": value}}
	}
	call(owner, base+"/review/decide", decision(1, "APPROVE", ""), 200, "", "")
	amendPath := base + "/amendments/propose"
	call(auditor, amendPath, proposal(1, "184600000", "Leitura final digitada errada"), 403, "FORBIDDEN", "meter.reading.correct")
	call(manager, amendPath, proposal(1, "0184600000", "Leitura final digitada errada"), 400, "MALFORMED", "")
	call(manager, amendPath, proposal(1, "184600000", ""), 422, "VALIDATION_FAILED", "")
	call(manager, amendPath, proposal(1, "184610000", "Mesmo valor"), 422, "VALIDATION_FAILED", "")
	call(manager, amendPath, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "reason": "Outro medidor", "end_reading": map[string]any{"kind": "HOUR_METER", "value": "10"}}, 422, "VALIDATION_FAILED", "")
	call(manager, amendPath, proposal(2, "184600000", "Leitura final digitada errada"), 409, "CONFLICT", "")
	call(manager, "/v1/orgs/"+x.scope.OrgID().String()+"/trips/"+onboardingID(t)+"/amendments/propose", proposal(1, "184600000", "Leitura final digitada errada"), 404, "NOT_FOUND", "")
	month := x.month(t, trip)
	locked := x.lock(t, month, 1)
	call(manager, amendPath, proposal(1, "184600000", "Leitura final digitada errada"), 409, "PERIOD_LOCKED", "")
	x.reopen(t, month, locked.Revision, "Correção de viagens")
	body := proposal(1, "184600000", "Leitura final digitada errada")
	proposed := call(manager, amendPath, body, 200, "", "")
	if proposed["revision"] != float64(2) || proposed["base_revision"] != float64(1) || proposed["intent"] != "AMEND" || proposed["review_state"] != "SUBMITTED" || proposed["replayed"] != false {
		t.Fatal("amendment proposal", proposed)
	}
	if replay := call(manager, amendPath, body, 200, "", ""); replay["replayed"] != true || replay["recorded_at"] != proposed["recorded_at"] {
		t.Fatal("amendment replay", replay)
	}
	read := call(owner, base+"/review/read", map[string]any{}, 200, "", "")
	effective, _ := read["effective_usage"].(map[string]any)
	amendments, _ := read["amendments"].([]any)
	if read["effective_revision"] != float64(1) || effective["distance"] != "290000" || effective["revision"] != float64(1) || len(amendments) != 1 {
		t.Fatal("decision context before the approval", read)
	}
	pending := amendments[0].(map[string]any)
	usage, _ := pending["usage"].(map[string]any)
	end, _ := pending["end_reading"].(map[string]any)
	if pending["review_state"] != "SUBMITTED" || usage["distance"] != "280000" || end["from"] != "184610000" || end["to"] != "184600000" || pending["start_reading"] != nil {
		t.Fatal("pending amendment in the decision context", pending)
	}
	call(manager, base+"/review/decide", decision(2, "APPROVE", ""), 409, "SELF_APPROVAL", "")
	if approved := call(owner, base+"/review/decide", decision(2, "APPROVE", ""), 200, "", ""); approved["review_state"] != "APPROVED" || approved["revision"] != float64(2) {
		t.Fatal("approval of the amendment", approved)
	}
	read = call(owner, base+"/review/read", map[string]any{}, 200, "", "")
	effective, _ = read["effective_usage"].(map[string]any)
	if read["effective_revision"] != float64(2) || effective["distance"] != "280000" || effective["kind"] != "ODOMETER" {
		t.Fatal("decision context after the approval", read)
	}
}
