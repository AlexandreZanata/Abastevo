//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// trackingPolicyGrants are the runtime grants of the policy writer (test
// role only; production grants stay retained).
func trackingPolicyGrants(t *testing.T, x trackingState) {
	t.Helper()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_tracking_policies,org_tracking_policy_commands,org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
}

// trackingPolicyRouter is the actual composed enterprise router.
func trackingPolicyRouter(t *testing.T, x trackingState) chi.Router {
	t.Helper()
	trackingPolicyGrants(t, x)
	trackReadGrants(t, x)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{43}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual tracking policy composition failed", err)
	}
	return router
}

// policyState is every stored policy fact of the fixture's organizations.
func (x trackingState) policyState(t *testing.T) string {
	t.Helper()
	var value string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(p) ORDER BY org_id,revision) FROM org_tracking_policies p),
 (SELECT jsonb_agg(to_jsonb(c) ORDER BY org_id,operation_id) FROM org_tracking_policy_commands c),
 (SELECT count(*) FROM org_audit_events WHERE action=$1))::text`, trackingapp.PolicyAuditAction).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// policyCall sends one company request and returns the status and body; a
// refusal must leave every policy fact unchanged.
func policyCall(t *testing.T, x trackingState, router chi.Router, actor matrixPerson, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	before := x.policyState(t)
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	r.AddCookie(actor.cookie)
	r.Header.Set("X-CSRF", actor.csrf)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatal("policy response is cacheable")
	}
	var result map[string]any
	if json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal("invalid policy response JSON")
	}
	if w.Code >= 400 && before != x.policyState(t) {
		t.Fatal("a refused policy request changed the policy", path, w.Code)
	}
	return w.Code, result
}

// refusedField asserts a VALIDATION_FAILED naming one frozen field code.
func refusedField(t *testing.T, status int, body map[string]any, field, code string) {
	t.Helper()
	e, _ := body["error"].(map[string]any)
	details, _ := e["details"].([]any)
	if status != 422 || e["code"] != "VALIDATION_FAILED" || len(details) != 1 {
		t.Fatal("expected a field refusal", status, e)
	}
	if d := details[0].(map[string]any); d["field"] != field || d["code"] != code {
		t.Fatal("wrong field refusal", d)
	}
}

func policyBody(op string, revision any, policy string, version any, at any) map[string]any {
	body := map[string]any{"schema_version": 1, "operation_id": op, "policy": policy}
	if revision != nil {
		body["revision"] = revision
	}
	if version != nil {
		body["notice_version"] = version
	}
	if at != nil {
		body["notice_in_force_at"] = at
	}
	return body
}

// B-BR-EMP-467..469, BUC-EMP-237/238 through the actual composed company
// routes: reads by every settings reader, changes only by a fresh OWNER or
// ADMIN, the notice rule, CAS, replay and reuse, the effect on the real
// tracking intake, and nothing written by any refusal.
func TestTrackingPolicyChangeAndReadThroughComposedCompanyHTTP(t *testing.T) {
	x := trackingFixture(t)
	router := trackingPolicyRouter(t, x)
	fx := x.deviceFixtureState
	d := x.syncFixtureState
	org := x.scope.OrgID().String()
	read, change := "/v1/orgs/"+org+"/tracking/policy/read", "/v1/orgs/"+org+"/tracking/policy/change"
	person := func(role, scope string) matrixPerson {
		p := matrixNewPerson(t, fx, router)
		matrixAssign(t, fx, p, role, scope, "ACTIVE")
		return p
	}
	owner, admin := person("OWNER", "ORG"), person("ADMIN", "ORG")
	readers := []matrixPerson{owner, admin, person("FLEET_MANAGER", "ORG"), person("SUPERVISOR", "ORG"), person("FINANCE", "ORG"), person("APPROVER", "ORG"), person("AUDITOR", "ORG")}
	driver, unitSupervisor := person("DRIVER", "SELF"), person("SUPERVISOR", "UNIT")

	// Nothing recorded: OFF at revision 0, and the real intake refuses tracking.
	for _, reader := range readers {
		status, v := policyCall(t, x, router, reader, read, map[string]any{})
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		if status != 200 || !slices.Equal(keys, []string{"data_mode", "effective_at", "in_force", "notice_in_force_at", "notice_version", "policy", "revision"}) ||
			v["revision"] != float64(0) || v["policy"] != "OFF" || v["notice_version"] != nil || v["notice_in_force_at"] != nil || v["effective_at"] != nil ||
			v["in_force"].(map[string]any)["policy"] != "OFF" || v["in_force"].(map[string]any)["revision"] != float64(0) {
			t.Fatal("empty policy read", status, v)
		}
	}
	matrixCall(t, fx, router, driver, read, map[string]any{}, 403, "FORBIDDEN", "org.settings.read", nil)
	refusedStart := x.driverOp(t, d, trackingapp.StartOperationType, onboardingID(t), x.at(1), "PRECISE", nil)
	ack, err := x.process(t, d, refusedStart)
	x.rejected(t, refusedStart, ack, err, "FORBIDDEN")

	now := lifecycleDBNow(t, x.legalFixtureState)
	inForce := now.Add(-time.Hour).Format(time.RFC3339Nano)
	future := now.Add(time.Hour).Format(time.RFC3339Nano)
	// Every other role, a unit-scoped grant, the auditor and the driver change nothing.
	for _, other := range append(readers[2:], driver, unitSupervisor) {
		matrixCall(t, fx, router, other, change, policyBody(onboardingID(t), nil, "EXPECTED", "notice-2026.11", inForce), 403, "FORBIDDEN", "org.settings.manage", nil)
	}
	// The notice rule and the frozen field codes.
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{policyBody(onboardingID(t), nil, "EXPECTED", nil, inForce), "notice_version", "REQUIRED_WHEN_ENABLED"},
		{policyBody(onboardingID(t), nil, "OPTIONAL", "notice-2026.11", nil), "notice_in_force_at", "REQUIRED_WHEN_ENABLED"},
		{policyBody(onboardingID(t), nil, "OFF", "notice-2026.11", nil), "notice_version", "ABSENT_WHEN_OFF"},
		{policyBody(onboardingID(t), nil, "OFF", nil, inForce), "notice_in_force_at", "ABSENT_WHEN_OFF"},
		{policyBody(onboardingID(t), nil, "EXPECTED", "Aviso 1", inForce), "notice_version", "NOTICE_VERSION_FORMAT"},
		{policyBody(onboardingID(t), nil, "EXPECTED", "notice-2026.11", future), "notice_in_force_at", "NOT_IN_FORCE"},
		{policyBody(onboardingID(t), nil, "ON", nil, nil), "policy", "OFF_OPTIONAL_OR_EXPECTED"},
	} {
		status, body := policyCall(t, x, router, owner, change, c.body)
		refusedField(t, status, body, c.field, c.code)
		if strings.Contains(string(mustPolicyJSON(t, body)), "Aviso") {
			t.Fatal("a refusal echoed the submitted value")
		}
	}
	for name, body := range map[string]map[string]any{
		"notice text": func() map[string]any {
			b := policyBody(onboardingID(t), nil, "OFF", nil, nil)
			b["notice_text"] = "x"
			return b
		}(),
		"zero revision": policyBody(onboardingID(t), 0, "OFF", nil, nil),
		"schema 2": func() map[string]any {
			b := policyBody(onboardingID(t), nil, "OFF", nil, nil)
			b["schema_version"] = 2
			return b
		}(),
		"bad instant":    policyBody(onboardingID(t), nil, "EXPECTED", "notice-2026.11", "yesterday"),
		"numeric policy": policyBody(onboardingID(t), nil, "1", nil, nil),
	} {
		status, v := policyCall(t, x, router, owner, change, body)
		wantMalformed := name != "numeric policy"
		if wantMalformed && (status != 400 || v["error"].(map[string]any)["code"] != "MALFORMED") || !wantMalformed && status != 422 {
			t.Fatal("malformed policy change", name, status, v)
		}
	}
	if status, v := policyCall(t, x, router, owner, change, policyBody(onboardingID(t), 1, "OFF", nil, nil)); status != 409 || v["error"].(map[string]any)["code"] != "CONFLICT" {
		t.Fatal("a premature revision was admitted", status, v)
	}
	matrixCall(t, fx, router, owner, "/v1/orgs/"+onboardingID(t)+"/tracking/policy/change", policyBody(onboardingID(t), nil, "OFF", nil, nil), 404, "NOT_FOUND", "", nil)
	matrixCall(t, fx, router, owner, "/v1/orgs/"+onboardingID(t)+"/tracking/policy/read", map[string]any{}, 404, "NOT_FOUND", "", nil)
	if x.policyState(t) != `[null, null, 0]` {
		t.Fatal("refusals wrote policy state", x.policyState(t))
	}

	// The owner turns tracking on; the exact replay and the reuse.
	op := onboardingID(t)
	body := policyBody(op, nil, "EXPECTED", "notice-2026.11", inForce)
	status, first := policyCall(t, x, router, owner, change, body)
	if status != 200 || first["revision"] != float64(1) || first["policy"] != "EXPECTED" || first["notice_version"] != "notice-2026.11" || first["replayed"] != false || first["effective_at"] == nil {
		t.Fatal("tracking policy change", status, first)
	}
	if at, err := time.Parse(time.RFC3339Nano, first["notice_in_force_at"].(string)); err != nil || !at.Equal(now.Add(-time.Hour)) {
		t.Fatal("stored notice instant", first["notice_in_force_at"])
	}
	status, replay := policyCall(t, x, router, owner, change, body)
	if status != 200 || replay["replayed"] != true || replay["revision"] != float64(1) || replay["effective_at"] != first["effective_at"] {
		t.Fatal("replayed change", status, replay)
	}
	if status, v := policyCall(t, x, router, owner, change, policyBody(op, nil, "OPTIONAL", "notice-2026.11", inForce)); status != 409 || v["error"].(map[string]any)["code"] != "OPERATION_ID_REUSED" {
		t.Fatal("reused operation", status, v)
	}
	matrixCall(t, fx, router, readers[6], change, body, 403, "FORBIDDEN", "org.settings.manage", nil)
	for _, reader := range []matrixPerson{readers[6], readers[2]} {
		if status, v := policyCall(t, x, router, reader, read, map[string]any{}); status != 200 || v["revision"] != float64(1) || v["policy"] != "EXPECTED" ||
			v["effective_at"] != first["effective_at"] || v["in_force"].(map[string]any)["policy"] != "EXPECTED" || v["in_force"].(map[string]any)["revision"] != float64(1) {
			t.Fatal("read after the change", status, v)
		}
	}
	// The real intake now opens a session under revision 1.
	session := onboardingID(t)
	x.startTracking(t, d, session, 2, "PRECISE", 2)
	var policyRevision int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT policy_revision FROM org_tracking_sessions WHERE org_id=$1 AND id=$2`, org, session).Scan(&policyRevision); err != nil || policyRevision != 1 {
		t.Fatal("the session did not open under the written version", err, policyRevision)
	}

	// The administrator turns it off on revision 1; the stale writer conflicts.
	status, off := policyCall(t, x, router, admin, change, policyBody(onboardingID(t), 1, "OFF", nil, nil))
	if status != 200 || off["revision"] != float64(2) || off["policy"] != "OFF" || off["notice_version"] != nil || off["notice_in_force_at"] != nil {
		t.Fatal("turn off", status, off)
	}
	if status, v := policyCall(t, x, router, owner, change, policyBody(onboardingID(t), 1, "OPTIONAL", "notice-2026.11", inForce)); status != 409 || v["error"].(map[string]any)["code"] != "CONFLICT" {
		t.Fatal("stale revision admitted", status, v)
	}
	if status, v := policyCall(t, x, router, owner, read, map[string]any{}); status != 200 || v["revision"] != float64(2) || v["in_force"].(map[string]any)["policy"] != "OFF" {
		t.Fatal("read after off", status, v)
	}
	// The open session keeps its stored data and keeps recording batches.
	batch := x.batchOp(t, d, session, 1, []int{3, 13}, nil)
	ack, err = x.process(t, d, batch)
	x.accepted(t, batch, ack, err, 1, trackingCounts{segments: 1})

	// Exactly two versions, two receipts with their authors, two audit events.
	var receipts, events int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_tracking_policy_commands c JOIN org_audit_events a ON(a.org_id,a.operation_id)=(c.org_id,c.operation_id)
 WHERE c.org_id::text=$1 AND a.actor_id::text=c.actor_account_id::text AND a.target_type='tracking_policy' AND a.target_id::text=$1 AND((c.result_revision=1 AND c.actor_account_id::text=$2)OR(c.result_revision=2 AND c.actor_account_id::text=$3))),
 (SELECT count(*) FROM org_audit_events WHERE org_id::text=$1 AND action='gps.policy.changed')`, org, owner.account, admin.account).Scan(&receipts, &events); err != nil || receipts != 2 || events != 2 {
		t.Fatal("receipts and audit events", err, receipts, events)
	}
}

func mustPolicyJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
