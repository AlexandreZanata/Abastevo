//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// The composed company-cookie entry over the ordinary runtime login, with
// real OWNER, FINANCE and DRIVER members (B-BR-EMP-212/213, BUC-EMP-108).
func TestPeriodsThroughComposedCompanyHTTP(t *testing.T) {
	fx := devicesFixture(t)
	periodGrant(t, fx.f)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{61}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	finance := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "ACTIVE")
	driver := matrixNewPerson(t, fx, router)
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/finance/periods"
	command := func(revision any, extra map[string]any) map[string]any {
		v := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision}
		for key, value := range extra {
			v[key] = value
		}
		return v
	}

	list := matrixCall(t, fx, router, finance, base+"/list", map[string]any{}, 200, "", "", nil)
	if items, ok := list["items"].([]any); !ok || len(items) != 0 || list["next_cursor"] != "" {
		t.Fatal("empty period list", list)
	}
	open := matrixCall(t, fx, router, finance, base+"/2026-10/read", map[string]any{}, 200, "", "", nil)
	if open["period"] != "2026-10" || open["state"] != "OPEN" || open["revision"] != float64(1) || open["period_id"] != nil || open["start_day"] != "2026-10-01" || open["end_day"] != "2026-11-01" || len(open["history"].([]any)) != 0 || open["data_mode"] != "SYNTHETIC" {
		t.Fatal("untouched month read", open)
	}

	closeBody := command(1, nil)
	locked := matrixCall(t, fx, router, finance, base+"/2026-10/close", closeBody, 200, "", "", nil)
	if locked["period"] != "2026-10" || locked["kind"] != "LOCK" || locked["state"] != "LOCKED" || locked["revision"] != float64(2) || locked["replayed"] != false || locked["period_id"] == nil || locked["recorded_at"] == nil {
		t.Fatal("close acknowledgement", locked)
	}
	again := matrixCall(t, fx, router, finance, base+"/2026-10/close", closeBody, 200, "", "", nil)
	if again["replayed"] != true || again["revision"] != float64(2) || again["recorded_at"] != locked["recorded_at"] || again["period_id"] != locked["period_id"] {
		t.Fatal("replayed close changed its acknowledgement", again)
	}
	// Stale revision, double close, operation reuse.
	matrixCall(t, fx, router, finance, base+"/2026-10/close", command(1, nil), 409, "CONFLICT", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/close", command(2, nil), 409, "CONFLICT", "", nil)
	reused := command(2, map[string]any{"reason": "Synthetic reuse"})
	reused["operation_id"] = closeBody["operation_id"]
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", reused, 409, "OPERATION_ID_REUSED", "", nil)
	// Reopen without or with an unusable reason.
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", command(2, nil), 422, "VALIDATION_FAILED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", command(2, map[string]any{"reason": " 　 "}), 422, "VALIDATION_FAILED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", command(2, map[string]any{"reason": "Synthetic\u0007bell"}), 422, "VALIDATION_FAILED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", command(2, map[string]any{"reason": strings.Repeat("a", 281)}), 422, "VALIDATION_FAILED", "", nil)
	// Malformed envelopes.
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "reason": "Synthetic reason"}, 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/close", command(0, nil), 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/close", command(1, map[string]any{"state": "LOCKED"}), 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/close", command(1, map[string]any{"reason": "Synthetic close note"}), 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/reopen", json.RawMessage(`{"schema_version":1,"operation_id":"`+onboardingID(t)+`","revision":2,"reason":"lone \udc00 surrogate"}`), 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, finance, base+"/2026-10/close", command("1", nil), 400, "MALFORMED", "", nil)
	// Unknown or non-canonical months are opaque not-found.
	for _, month := range []string{"2026-13", "2026-1", "202610", "0000-01"} {
		matrixCall(t, fx, router, finance, base+"/"+month+"/close", command(1, nil), 404, "NOT_FOUND", "", nil)
		matrixCall(t, fx, router, finance, base+"/"+month+"/read", map[string]any{}, 404, "NOT_FOUND", "", nil)
	}
	// A role without the permission and another organization.
	matrixCall(t, fx, router, driver, base+"/2026-10/reopen", command(2, map[string]any{"reason": "Synthetic driver reopening"}), 403, "FORBIDDEN", "fin.period.reopen", nil)
	matrixCall(t, fx, router, driver, base+"/2026-10/read", map[string]any{}, 403, "FORBIDDEN", "fin.period.close", nil)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	matrixCall(t, fx, router, finance, "/v1/orgs/"+other.scope.OrgID().String()+"/finance/periods/2026-10/close", command(1, nil), 404, "NOT_FOUND", "", nil)

	reason := "  Synthetic VAL-19 reopening  "
	reopened := matrixCall(t, fx, router, owner, base+"/2026-10/reopen", command(2, map[string]any{"reason": reason}), 200, "", "", nil)
	if reopened["kind"] != "REOPEN" || reopened["state"] != "REOPENED" || reopened["revision"] != float64(3) {
		t.Fatal("reopen", reopened)
	}
	relocked := matrixCall(t, fx, router, finance, base+"/2026-10/close", command(3, nil), 200, "", "", nil)
	if relocked["kind"] != "RELOCK" || relocked["state"] != "LOCKED" || relocked["revision"] != float64(4) {
		t.Fatal("re-lock", relocked)
	}
	view := matrixCall(t, fx, router, owner, base+"/2026-10/read", map[string]any{}, 200, "", "", nil)
	history, _ := view["history"].([]any)
	if view["state"] != "LOCKED" || view["revision"] != float64(4) || len(history) != 3 {
		t.Fatal("history read", view)
	}
	for i, want := range []struct{ kind, from, to, authority, reason, account, member string }{
		{"LOCK", "OPEN", "LOCKED", "fin.period.close", "", finance.account, finance.member},
		{"REOPEN", "LOCKED", "REOPENED", "fin.period.reopen", reason, owner.account, owner.member},
		{"RELOCK", "REOPENED", "LOCKED", "fin.period.close", "", finance.account, finance.member},
	} {
		h := history[i].(map[string]any)
		if h["revision"] != float64(i+2) || h["kind"] != want.kind || h["from_state"] != want.from || h["to_state"] != want.to || h["authority"] != want.authority || h["reason"] != want.reason || h["actor_account_id"] != want.account || h["actor_membership_id"] != want.member || h["operation_id"] == nil || h["recorded_at"] == nil || len(h) != 10 {
			t.Fatal("history entry", i, h)
		}
	}
	// Keyset pages, newest month first, and the state filter.
	for _, month := range []string{"2026-09", "2026-08"} {
		matrixCall(t, fx, router, finance, base+"/"+month+"/close", command(1, nil), 200, "", "", nil)
	}
	matrixCall(t, fx, router, owner, base+"/2026-08/reopen", command(2, map[string]any{"reason": "Synthetic August reopening"}), 200, "", "", nil)
	first := matrixCall(t, fx, router, finance, base+"/list", map[string]any{"limit": 2}, 200, "", "", nil)
	items, _ := first["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["period"] != "2026-10" || items[1].(map[string]any)["period"] != "2026-09" || first["next_cursor"] == "" || items[0].(map[string]any)["history"] != nil {
		t.Fatal("first period page", first)
	}
	second := matrixCall(t, fx, router, finance, base+"/list", map[string]any{"limit": 2, "cursor": first["next_cursor"]}, 200, "", "", nil)
	items, _ = second["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["period"] != "2026-08" || items[0].(map[string]any)["state"] != "REOPENED" || second["next_cursor"] != "" {
		t.Fatal("second period page", second)
	}
	reopenedOnly := matrixCall(t, fx, router, finance, base+"/list", map[string]any{"state": "REOPENED"}, 200, "", "", nil)
	if items, _ := reopenedOnly["items"].([]any); len(items) != 1 || items[0].(map[string]any)["period"] != "2026-08" {
		t.Fatal("state filter", reopenedOnly)
	}
	matrixCall(t, fx, router, finance, base+"/list", map[string]any{"state": "OPEN"}, 400, "MALFORMED", "", nil)
	matrixCall(t, fx, router, finance, base+"/list", map[string]any{"limit": 2, "cursor": first["next_cursor"], "state": "LOCKED"}, 400, "MALFORMED", "", nil)
	var events int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_type='finance_period'`, org).Scan(&events); err != nil || events != 6 {
		t.Fatal("one audit event per accepted HTTP command", events, err)
	}
}
