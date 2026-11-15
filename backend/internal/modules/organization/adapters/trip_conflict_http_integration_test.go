//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

// EMP-TRIP-007 through the actual composed company routes: the queue, the
// read and one resolution with the bounded refusals of B-BR-EMP-408..410
// (FORBIDDEN, SELF_APPROVAL, CONFLICT, VALIDATION_FAILED, MALFORMED,
// OPERATION_ID_REUSED, NOT_FOUND) and an unchanged private state on refusal.
func TestTripConflictsThroughComposedCompanyHTTP(t *testing.T) {
	x := conflictFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{37}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual conflict composition failed", err)
	}
	fx := x.deviceFixtureState
	owner, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	bruno, _ := x.assignedDriver(t)
	diego := x.syncFixtureState
	at := x.declaredNow(t)
	diegoStart := x.startOp(t, diego, at.Add(5*time.Minute), nil)
	x.deliver(t, diego, diegoStart, protocol.Accepted, "")
	brunoStart := x.startOp(t, bruno, at, nil)
	item := x.deliver(t, bruno, brunoStart, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
	org := x.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/sync/reviews/trips"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	items, _ := call(owner, base+"/list", map[string]any{"limit": 10, "state": "OPEN"}, 200, "", "")["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["review_item_id"] != item || items[0].(map[string]any)["operation_type"] != "trip.start" || items[0].(map[string]any)["code"] != "ASSET_BUSY" {
		t.Fatal("conflict queue", items)
	}
	read := call(owner, base+"/"+item+"/read", map[string]any{}, 200, "", "")
	fact, _ := read["fact"].(map[string]any)
	trips, _ := read["conflicting_trips"].([]any)
	reading, _ := fact["reading"].(map[string]any)
	if read["state"] != "OPEN" || read["revision"] != float64(1) || read["resolution"] != nil || read["data_mode"] != "SYNTHETIC" || fact["asset_id"] != x.vehicle.AssetID ||
		fact["driver_employee_id"] != bruno.person.employee || reading["value"] != "184320000" || reading["kind"] != "ODOMETER" || fact["event"] != nil || fact["submitted_revision"] != nil ||
		len(trips) != 1 || trips[0].(map[string]any)["trip_id"] != tripOf(diegoStart) || trips[0].(map[string]any)["operational_state"] != "IN_PROGRESS" || trips[0].(map[string]any)["finished_at"] != nil {
		t.Fatal("conflict read", read)
	}
	resolve := base + "/" + item + "/resolve"
	body := func(revision any, kind, reason, op string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": op, "revision": revision, "decision": kind, "reason": reason}
	}
	for _, path := range []string{base + "/list", base + "/" + item + "/read"} {
		call(auditor, path, map[string]any{}, 403, "FORBIDDEN", "trip.trip.review")
	}
	call(auditor, resolve, body(1, "REFUSE", "Synthetic", onboardingID(t)), 403, "FORBIDDEN", "trip.trip.review")
	call(owner, resolve, body(1, "REFUSE", "", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, resolve, body(1, "REFUSE", " 　 ", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, resolve, body(1, "ATTACH", "Synthetic", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, resolve, body(1, "ACCEPT", "", onboardingID(t)), 409, "CONFLICT", "")
	call(owner, resolve, body(2, "REFUSE", "Synthetic", onboardingID(t)), 409, "CONFLICT", "")
	call(owner, resolve, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "decision": "REFUSE", "reason": "Synthetic"}, 400, "MALFORMED", "")
	call(owner, "/v1/orgs/"+onboardingID(t)+"/sync/reviews/trips/"+item+"/resolve", body(1, "REFUSE", "Synthetic", onboardingID(t)), 404, "NOT_FOUND", "")
	call(owner, base+"/"+onboardingID(t)+"/read", map[string]any{}, 404, "NOT_FOUND", "")
	if c := x.conflictCount(t, brunoStart); c != (conflictCounts{}) {
		t.Fatalf("a refused HTTP resolution wrote state: %+v", c)
	}
	op := onboardingID(t)
	refused := call(owner, resolve, body(1, "REFUSE", "Veículo em uso por outra viagem", op), 200, "", "")
	resolution, _ := refused["resolution"].(map[string]any)
	if refused["replayed"] != false || resolution["outcome"] != "REFUSED" || resolution["reason"] != "Veículo em uso por outra viagem" || resolution["review_item_id"] != item ||
		resolution["resolution_id"] != op || resolution["entity_revision"] != nil {
		t.Fatal("refused resolution", refused)
	}
	replay := call(owner, resolve, body(1, "REFUSE", "Veículo em uso por outra viagem", op), 200, "", "")
	first, _ := json.Marshal(refused["resolution"])
	again, _ := json.Marshal(replay["resolution"])
	if replay["replayed"] != true || !bytes.Equal(first, again) {
		t.Fatal("replayed resolution", replay)
	}
	call(owner, resolve, body(1, "REFUSE", "Synthetic other reason", op), 409, "OPERATION_ID_REUSED", "")
	call(owner, resolve, body(1, "REFUSE", "Synthetic", onboardingID(t)), 409, "CONFLICT", "")
	if read := call(owner, base+"/"+item+"/read", map[string]any{}, 200, "", ""); read["state"] != "RESOLVED_REFUSED" || read["revision"] != float64(2) {
		t.Fatal("refused read", read["state"])
	}
	if items, _ := call(owner, base+"/list", map[string]any{"limit": 10, "state": "RESOLVED_REFUSED"}, 200, "", "")["items"].([]any); len(items) != 1 {
		t.Fatal("refused queue", items)
	}
	// The ADMIN author holds trip.trip.review and still cannot resolve its own held start.
	x.deliver(t, diego, x.finishOp(t, diego, tripOf(diegoStart), at.Add(6*time.Hour), nil), protocol.Accepted, "")
	other := x.startOp(t, bruno, at.Add(7*time.Hour), absentReading)
	x.deliver(t, bruno, other, protocol.Accepted, "")
	own := x.startOp(t, diego, at.Add(8*time.Hour), absentReading)
	ownItem := x.deliver(t, diego, own, protocol.AcceptedForReview, tripapp.AssetBusy).Snapshot().ReviewItemID
	admin := tripReviewLogin(t, x.tripReviewState, router)
	call(admin, base+"/"+ownItem+"/resolve", body(1, "REFUSE", "Meu próprio registro", onboardingID(t)), 409, "SELF_APPROVAL", "")
	if read := call(admin, base+"/"+ownItem+"/read", map[string]any{}, 200, "", ""); read["state"] != "OPEN" {
		t.Fatal("the author's read after a refused self review", read["state"])
	}
}
