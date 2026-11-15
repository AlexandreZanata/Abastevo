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

// tripReviewLogin opens a company session for an existing device person
// through the composed login route.
func tripReviewLogin(t *testing.T, x tripReviewState, router chi.Router) matrixPerson {
	t.Helper()
	p := x.person
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
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.CSRF == "" || len(w.Result().Cookies()) != 1 {
		t.Fatal("company login of the device person failed", w.Code)
	}
	return matrixPerson{account: p.auth.Account.ID, member: p.member, employee: p.employee, cookie: w.Result().Cookies()[0], csrf: result.CSRF}
}

// EMP-TRIP-004 through the actual composed company routes: the queue, the
// decision context and one decision, with the bounded refusals of
// B-BR-EMP-345 (FORBIDDEN, SELF_APPROVAL, CONFLICT, VALIDATION_FAILED,
// OPERATION_ID_REUSED, NOT_FOUND) and an unchanged private state on refusal.
func TestTripReviewThroughComposedCompanyHTTP(t *testing.T) {
	x := tripReviewFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{29}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual trip review composition failed", err)
	}
	fx := x.deviceFixtureState
	owner, auditor := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	d, at := x.assignedDriver(t)
	trip, _ := x.completed(t, d, at)
	x.submitted(t, d, trip, at.Add(90*time.Minute))
	org := x.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/trips"
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	items, _ := call(owner, base+"/review/list", map[string]any{"limit": 10}, 200, "", "")["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["trip_id"] != trip || items[0].(map[string]any)["review_state"] != "SUBMITTED" {
		t.Fatal("review queue", items)
	}
	read := call(owner, base+"/"+trip+"/review/read", map[string]any{}, 200, "", "")
	history, _ := read["history"].([]any)
	if read["review_state"] != "SUBMITTED" || read["operational_state"] != "COMPLETED" || read["revision"] != float64(1) || len(history) != 1 || read["finished_at"] == nil || read["data_mode"] != "SYNTHETIC" {
		t.Fatal("decision context", read)
	}
	for _, private := range []string{"reading", "missing_reading_reason", "run_clock", "unit_id", "cost_centre_id"} {
		if _, ok := read[private]; ok {
			t.Fatal("the decision context exposed a private trip field", private)
		}
	}
	decision := func(revision any, kind, reason, op string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": op, "revision": revision, "decision": kind, "reason": reason}
	}
	decide := base + "/" + trip + "/review/decide"
	for _, path := range []string{base + "/review/list", base + "/" + trip + "/review/read"} {
		call(auditor, path, map[string]any{}, 403, "FORBIDDEN", "trip.trip.review")
	}
	call(auditor, decide, decision(1, "APPROVE", "", onboardingID(t)), 403, "FORBIDDEN", "trip.trip.review")
	call(owner, decide, decision(1, "RETURN", "", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, decide, decision(1, "RETURN", " 　 ", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, decide, decision(1, "OPEN", "", onboardingID(t)), 422, "VALIDATION_FAILED", "")
	call(owner, decide, decision(2, "APPROVE", "", onboardingID(t)), 409, "CONFLICT", "")
	call(owner, decide, map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "decision": "APPROVE", "reason": ""}, 400, "MALFORMED", "")
	call(owner, "/v1/orgs/"+onboardingID(t)+"/trips/"+trip+"/review/decide", decision(1, "APPROVE", "", onboardingID(t)), 404, "NOT_FOUND", "")
	call(owner, base+"/"+onboardingID(t)+"/review/read", map[string]any{}, 404, "NOT_FOUND", "")
	op := onboardingID(t)
	returned := call(owner, decide, decision(1, "RETURN", "Informe o motivo da parada", op), 200, "", "")
	if returned["review_state"] != "RETURNED" || returned["replayed"] != false || returned["revision"] != float64(1) || returned["reason"] != "Informe o motivo da parada" {
		t.Fatal("returned decision", returned)
	}
	replay := call(owner, decide, decision(1, "RETURN", "Informe o motivo da parada", op), 200, "", "")
	if replay["replayed"] != true || replay["recorded_at"] != returned["recorded_at"] || replay["review_state"] != "RETURNED" {
		t.Fatal("replayed decision", replay)
	}
	call(owner, decide, decision(1, "RETURN", "Synthetic other reason", op), 409, "OPERATION_ID_REUSED", "")
	call(owner, decide, decision(1, "APPROVE", "", onboardingID(t)), 409, "CONFLICT", "")
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,RETURN:RETURNED:1" || audits != 1 {
		t.Fatal("HTTP decision history", entries, audits)
	}
	// The ADMIN author holds trip.trip.review and still cannot decide its own trip.
	own, _ := x.completed(t, x.syncFixtureState, at.Add(2*time.Hour))
	x.submitted(t, x.syncFixtureState, own, at.Add(3*time.Hour+30*time.Minute))
	admin := tripReviewLogin(t, x, router)
	call(admin, base+"/"+own+"/review/decide", decision(1, "APPROVE", "", onboardingID(t)), 409, "SELF_APPROVAL", "")
	if read := call(admin, base+"/"+own+"/review/read", map[string]any{}, 200, "", ""); read["review_state"] != "SUBMITTED" {
		t.Fatal("the author's read after a refused self-approval", read["review_state"])
	}
	if approved := call(owner, base+"/"+own+"/review/decide", decision(1, "APPROVE", "", onboardingID(t)), 200, "", ""); approved["review_state"] != "APPROVED" {
		t.Fatal("independent approval", approved)
	}
}
