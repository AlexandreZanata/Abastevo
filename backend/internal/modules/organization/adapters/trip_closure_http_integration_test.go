//go:build integration

package adapters_test

import (
	"bytes"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-014 through the actual composed company routes: the open-trip
// list and one closure, with the bounded refusals of B-BR-EMP-399..403
// (FORBIDDEN, SELF_APPROVAL, CONFLICT, VALIDATION_FAILED, MALFORMED for a
// meter value, OPERATION_ID_REUSED, NOT_FOUND) and an unchanged private state
// on refusal; the review read then shows the closure.
func TestTripClosureThroughComposedCompanyHTTP(t *testing.T) {
	x := tripReviewFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{29}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual trip closure composition failed", err)
	}
	fx := x.deviceFixtureState
	supervisor, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, supervisor, "SUPERVISOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	d, at := x.assignedDriver(t)
	start := x.startOp(t, d, at, absentReading)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 0)
	trip := tripOf(start)
	org := x.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/trips"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	items, _ := call(supervisor, base+"/open/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["trip_id"] != trip || items[0].(map[string]any)["operational_state"] != "IN_PROGRESS" ||
		items[0].(map[string]any)["revision"] != float64(1) || items[0].(map[string]any)["author_membership_id"] != d.source.MembershipID {
		t.Fatal("open-trip list", items)
	}
	for _, private := range []string{"reading", "missing_reading_reason", "run_clock", "unit_id", "cost_centre_id"} {
		if _, ok := items[0].(map[string]any)[private]; ok {
			t.Fatal("the open-trip list exposed a private trip field", private)
		}
	}
	closure := func(revision any, reason, missing, op string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": op, "revision": revision, "reason": reason, "missing_reading_reason": missing}
	}
	close := base + "/" + trip + "/close"
	call(auditor, base+"/open/list", map[string]any{}, 403, "FORBIDDEN", "trip.trip.close_for_other")
	call(auditor, close, closure(1, closureReason, closureMissing, onboardingID(t)), 403, "FORBIDDEN", "trip.trip.close_for_other")
	call(supervisor, close, closure(1, "", closureMissing, onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(supervisor, close, closure(1, closureReason, " 　 ", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(supervisor, close, closure(2, closureReason, closureMissing, onboardingID(t)), 409, "CONFLICT", "")
	// B-BR-EMP-400: a meter value, reading or distance cannot be sent.
	for _, field := range []string{"final_reading", "value", "distance"} {
		body := closure(1, closureReason, closureMissing, onboardingID(t))
		body[field] = "184610000"
		call(supervisor, close, body, 400, "MALFORMED", "")
	}
	call(supervisor, close, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": closureReason, "missing_reading_reason": closureMissing}, 400, "MALFORMED", "")
	call(supervisor, "/v1/orgs/"+onboardingID(t)+"/trips/"+trip+"/close", closure(1, closureReason, closureMissing, onboardingID(t)), 404, "NOT_FOUND", "")
	call(supervisor, base+"/"+onboardingID(t)+"/close", closure(1, closureReason, closureMissing, onboardingID(t)), 404, "NOT_FOUND", "")
	op := onboardingID(t)
	closed := call(supervisor, close, closure(1, closureReason, closureMissing, op), 200, "", "")
	usage, _ := closed["usage"].(map[string]any)
	facts, _ := closed["closure"].(map[string]any)
	if closed["operational_state"] != "COMPLETED" || closed["revision"] != float64(2) || closed["replayed"] != false || closed["final_reading"] != nil ||
		usage["status"] != "NOT_MEASURED" || usage["reason"] != "READING_ABSENT" || usage["distance"] != nil || usage["kind"] != nil ||
		facts["closer_membership_id"] != supervisor.member || facts["reason"] != closureReason || facts["missing_reading_reason"] != closureMissing {
		t.Fatal("closure result", closed)
	}
	replay := call(supervisor, close, closure(1, closureReason, closureMissing, op), 200, "", "")
	if replay["replayed"] != true || replay["closure"].(map[string]any)["closed_at"] != facts["closed_at"] {
		t.Fatal("replayed closure", replay)
	}
	call(supervisor, close, closure(1, "Outro motivo", closureMissing, op), 409, "OPERATION_ID_REUSED", "")
	call(supervisor, close, closure(1, closureReason, closureMissing, onboardingID(t)), 409, "CONFLICT", "")
	if items, _ := call(supervisor, base+"/open/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any); len(items) != 0 {
		t.Fatal("a closed trip is still open", items)
	}
	read := call(supervisor, base+"/"+trip+"/review/read", map[string]any{}, 200, "", "")
	if read["operational_state"] != "COMPLETED" || read["finished_at"] != nil || read["closure"] == nil || read["closure"].(map[string]any)["missing_reading_reason"] != closureMissing {
		t.Fatal("decision context of a closed trip", read)
	}
	// The ADMIN author holds trip.trip.close_for_other and still cannot close its own trip.
	own := x.startOp(t, x.syncFixtureState, at.Add(2*time.Hour), absentReading)
	ack, err = x.process(t, x.syncFixtureState, own)
	x.accepted(t, own, ack, err, 1, 0)
	admin := tripReviewLogin(t, x, router)
	call(admin, base+"/"+tripOf(own)+"/close", closure(1, closureReason, closureMissing, onboardingID(t)), 409, "SELF_APPROVAL", "")
	if closed := call(supervisor, base+"/"+tripOf(own)+"/close", closure(1, closureReason, closureMissing, onboardingID(t)), 200, "", ""); closed["operational_state"] != "COMPLETED" {
		t.Fatal("independent closure", closed)
	}
}
