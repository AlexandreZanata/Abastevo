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
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// The actual composed company-cookie entry with production NoOpenWork wiring.
func TestFleetLifecycleThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	_ = lifecycleStore(t, fx)
	_ = meterStore(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synlifecyclehttp")
	if err != nil {
		t.Fatal("synthetic account creation failed")
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal("synthetic account login failed")
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{7}, 32), Pool: fx.f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal("actual lifecycle composition failed", err)
	}
	var cookie *http.Cookie
	csrf, requests := "", 0
	call := func(label, path string, body any, want int) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
			r.Header.Set("X-CSRF", csrf)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		requests++
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("%s: private lifecycle HTTP got %d want %d: %s", label, w.Code, want, w.Body.String())
		}
		if w.Code >= 400 && (strings.Contains(w.Body.String(), "Synthetic") || strings.Contains(w.Body.String(), "ABC1D23")) {
			t.Fatal("private lifecycle content entered refusal")
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid private lifecycle response")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookie = w.Result().Cookies()[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	code := func(value map[string]any) string {
		t.Helper()
		failure, _ := value["error"].(map[string]any)
		result, _ := failure["code"].(string)
		return result
	}
	login := func(familyID, token string) {
		cookie, csrf = nil, ""
		call("company login", "/v1/org-access/session/login", map[string]any{"family_id": familyID, "access_token": token}, 200)
	}
	login(auth.Session.FamilyID, auth.Session.AccessToken)
	createOrg := func() map[string]any {
		t.Helper()
		result := call("organization creation", "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic lifecycle organization", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Synthetic main", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201)
		id := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, id), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		call("organization activation", "/v1/orgs/"+id+"/organization/activate", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}, 200)
		return result
	}
	organization, other := createOrg(), createOrg()
	org := organization["organization_id"].(string)
	base, otherBase := "/v1/orgs/"+org+"/fleet/assets", "/v1/orgs/"+other["organization_id"].(string)+"/fleet/assets"
	create := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic lifecycle implement", "plate": "ABC1D23", "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic lifecycle category"}, "unit_id": organization["unit_id"]}
	created := call("draft", base+"/create", create, 201)
	path := base + "/" + created["asset_id"].(string)
	command := func(revision int, extra map[string]any) map[string]any {
		out := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	now := lifecycleDBNow(t, fx)
	acquired := now.Add(-72 * time.Hour).Truncate(time.Second).Add(987654321 * time.Nanosecond).Format(time.RFC3339Nano)
	if r := call("acquisition on draft", path+"/acquisition/record", command(1, map[string]any{"acquired_at": acquired}), 200); r["state"] != "DRAFT" || r["revision"] != float64(2) {
		t.Fatal("acquisition changed lifecycle", r)
	}
	if code(call("archive straight from DRAFT", path+"/archive", command(2, nil), 409)) != "CONFLICT" {
		t.Fatal("draft archive code")
	}
	call("activate", path+"/activate", command(2, nil), 200)
	if code(call("archive straight from ACTIVE", path+"/archive", command(3, nil), 409)) != "CONFLICT" {
		t.Fatal("active archive code")
	}
	if code(call("deactivate without reason", path+"/deactivate", command(3, nil), 422)) != "VALIDATION_FAILED" {
		t.Fatal("missing reason code")
	}
	if code(call("deactivate blank reason", path+"/deactivate", command(3, map[string]any{"reason": "   "}), 422)) != "VALIDATION_FAILED" {
		t.Fatal("blank reason code")
	}
	call("state injection", path+"/deactivate", command(3, map[string]any{"reason": "Synthetic idle", "state": "INACTIVE"}), 400)
	call("open work injection", path+"/dispose", command(3, map[string]any{"reason": "Synthetic", "kind": "SALE", "disposed_at": acquired, "open_operations": []any{}}), 400)
	deactivate := command(3, map[string]any{"reason": "Synthetic seasonal idle"})
	if r := call("deactivate", path+"/deactivate", deactivate, 200); r["state"] != "INACTIVE" || r["revision"] != float64(4) || r["replayed"] != false {
		t.Fatal("deactivate result", r)
	}
	if r := call("deactivate replay", path+"/deactivate", deactivate, 200); r["state"] != "INACTIVE" || r["revision"] != float64(4) || r["replayed"] != true {
		t.Fatal("deactivate replay", r)
	}
	changed := command(3, map[string]any{"reason": "Synthetic different intent"})
	changed["operation_id"] = deactivate["operation_id"]
	if code(call("changed replay", path+"/deactivate", changed, 409)) != "OPERATION_ID_REUSED" {
		t.Fatal("changed replay code")
	}
	if code(call("stale revision", path+"/reactivate", command(3, nil), 409)) != "CONFLICT" {
		t.Fatal("stale revision code")
	}
	call("reactivate", path+"/reactivate", command(4, nil), 200)
	future := now.Add(48 * time.Hour).Format(time.RFC3339Nano)
	if code(call("future disposal", path+"/dispose", command(5, map[string]any{"kind": "SALE", "disposed_at": future, "reason": "Synthetic sale"}), 422)) != "VALIDATION_FAILED" {
		t.Fatal("future disposal code")
	}
	if code(call("unknown disposal kind", path+"/dispose", command(5, map[string]any{"kind": "THEFT", "disposed_at": acquired, "reason": "Synthetic"}), 422)) != "VALIDATION_FAILED" {
		t.Fatal("unknown kind code")
	}
	disposedAt := now.Add(-time.Hour).Truncate(time.Second).Add(5 * time.Nanosecond).Format(time.RFC3339Nano)
	call("foreign organization asset hidden", otherBase+"/"+created["asset_id"].(string)+"/dispose", command(5, map[string]any{"kind": "SALE", "disposed_at": disposedAt, "reason": "Synthetic sale"}), 404)
	if r := call("dispose", path+"/dispose", command(5, map[string]any{"kind": "SALE", "disposed_at": disposedAt, "reason": "Synthetic sale"}), 200); r["state"] != "DISPOSED" || r["revision"] != float64(6) {
		t.Fatal("dispose result", r)
	}
	view := call("disposed asset read", path+"/read", map[string]any{}, 200)
	disposal, _ := view["disposal"].(map[string]any)
	if view["state"] != "DISPOSED" || view["acquired_at"] != acquired || disposal["kind"] != "SALE" || disposal["disposed_at"] != disposedAt || disposal["reason"] != "Synthetic sale" || view["plate"] != "ABC1D23" {
		t.Fatal("disposed asset view lost lifecycle data", view)
	}
	duplicate := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic successor", "plate": "abc-1d23", "usage_basis": "HOURS", "category": map[string]any{"new_label": "Synthetic lifecycle category"}}
	if code(call("disposed plate not yet free", base+"/create", duplicate, 409)) != "ASSET_IDENTIFIER_TAKEN" {
		t.Fatal("disposed plate code")
	}
	call("revert", path+"/disposal/revert", command(6, map[string]any{"reason": "Synthetic recorded by mistake"}), 200)
	if view := call("reverted read", path+"/read", map[string]any{}, 200); view["state"] != "INACTIVE" || view["disposal"] != nil {
		t.Fatal("revert kept current disposal", view)
	}
	call("dispose again", path+"/dispose", command(7, map[string]any{"kind": "RETURN_TO_HOLDER", "disposed_at": disposedAt, "reason": "Synthetic lease return"}), 200)
	page := call("lifecycle history page", path+"/lifecycle/list", map[string]any{"limit": 3}, 200)
	items := page["items"].([]any)
	if len(items) != 3 || page["next_cursor"] == "" {
		t.Fatal("lifecycle history page", page)
	}
	first := items[0].(map[string]any)
	if first["command"] != "DEACTIVATE" || first["from_state"] != "ACTIVE" || first["to_state"] != "INACTIVE" || first["reason"] != "Synthetic seasonal idle" {
		t.Fatal("history first row", first)
	}
	rest := call("lifecycle history continuation", path+"/lifecycle/list", map[string]any{"limit": 3, "cursor": page["next_cursor"]}, 200)
	restItems := rest["items"].([]any)
	if len(restItems) != 2 || rest["next_cursor"] != "" || restItems[0].(map[string]any)["command"] != "REVERT_DISPOSAL" || restItems[1].(map[string]any)["disposal_kind"] != "RETURN_TO_HOLDER" {
		t.Fatal("history continuation", rest)
	}
	historyDisposal := items[2].(map[string]any)
	if historyDisposal["command"] != "DISPOSE" || historyDisposal["disposal_kind"] != "SALE" || historyDisposal["disposed_at"] != disposedAt {
		t.Fatal("history lost the reverted disposal", historyDisposal)
	}
	call("cursor bound to asset", base+"/"+onboardingID(t)+"/lifecycle/list", map[string]any{"limit": 3, "cursor": page["next_cursor"]}, 400)
	call("archive", path+"/archive", command(8, nil), 200)
	if code(call("archived is terminal", path+"/acquisition/record", command(9, map[string]any{"acquired_at": acquired}), 409)) != "CONFLICT" {
		t.Fatal("archived terminal code")
	}
	successor := call("archived plate reused", base+"/create", duplicate, 201)
	if successor["asset_id"] == created["asset_id"] {
		t.Fatal("archive renamed instead of freeing")
	}
	if view := call("archived record readable", path+"/read", map[string]any{}, 200); view["state"] != "ARCHIVED" || view["plate"] != "ABC1D23" {
		t.Fatal("archived record unreadable", view)
	}
	// A real FINANCE member reads history but cannot manage the lifecycle.
	financeKey, err := accounts.CreateKeyAccount(t.Context(), "synlifecyclefin")
	if err != nil {
		t.Fatal(err)
	}
	finance, err := accounts.LoginWithKey(t.Context(), financeKey.Key)
	if err != nil {
		t.Fatal(err)
	}
	sourceFx := fx
	sourceFx.scope = scopeFor(t, org)
	member, employee := onboardingID(t), onboardingID(t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{org, employee}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, member, employee, finance.Account.ID}},
		{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'FINANCE','ORG',$1,clock_timestamp()-interval '1 minute',$4,'Synthetic finance source')`, []any{org, onboardingID(t), member, organization["membership_id"]}},
		{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, member}},
	} {
		if err := unitSchemaExec(t, sourceFx, statement.sql, statement.args...); err != nil {
			t.Fatal("synthetic finance membership setup failed", err)
		}
	}
	successorPath := base + "/" + successor["asset_id"].(string)
	login(finance.Session.FamilyID, finance.Session.AccessToken)
	denied := call("finance cannot dispose", successorPath+"/dispose", command(1, map[string]any{"kind": "SALE", "disposed_at": disposedAt, "reason": "Synthetic"}), 403)
	if detail := denied["error"].(map[string]any)["details"].([]any); len(detail) != 1 || detail[0].(map[string]any)["code"] != "fleet.asset.manage" {
		t.Fatal("finance denial did not name the missing permission", denied)
	}
	call("finance cannot deactivate", successorPath+"/deactivate", command(1, map[string]any{"reason": "Synthetic"}), 403)
	call("finance reads lifecycle history", path+"/lifecycle/list", map[string]any{"limit": 10}, 200)
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND asset_id=$2)=6 AND(SELECT state FROM org_assets WHERE org_id=$1 AND id=$3)='DRAFT' AND(SELECT revision FROM org_assets WHERE org_id=$1 AND id=$3)=1`, org, created["asset_id"], successor["asset_id"]).Scan(&exact); err != nil || !exact {
		t.Fatal("HTTP lifecycle effects or denials changed history", err)
	}
	// Rollback switch: with the organizations capability off, lifecycle writes
	// are refused while the retained history stays readable.
	login(auth.Session.FamilyID, auth.Session.AccessToken)
	if _, err := fx.flags.Set(t.Context(), scopeFor(t, org), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	call("capability off refuses lifecycle writes", successorPath+"/acquisition/record", command(1, map[string]any{"acquired_at": acquired}), 403)
	call("capability off keeps history readable", path+"/lifecycle/list", map[string]any{"limit": 10}, 200)
	t.Logf("actual private lifecycle HTTP request assertions: %d", requests)
}
