//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestSetupPanelOrdinaryComposedHTTPRecoveryAndNamedMembers(t *testing.T) {
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()+"; GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	router := chi.NewRouter()
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{61}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); e != nil {
		t.Fatal(e)
	}
	actor := matrixNewPerson(t, fx, router)
	created := matrixCall(t, fx, router, actor, "/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic persisted panel", "kinds": []string{"OTHER"}, "time_zone": "America/Cuiaba", "unit_name": "Synthetic first unit", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201, "", "", nil)
	org := created["organization_id"].(string)
	member := created["membership_id"].(string)
	base := "/v1/orgs/" + org
	directory := matrixCall(t, fx, router, actor, "/v1/org-access/organizations/setups/list", map[string]any{}, 200, "", "", nil)
	if directory["account_id"] != actor.account || len(directory["setups"].([]any)) != 1 || directory["setups"].([]any)[0].(map[string]any)["membership_id"] != member {
		t.Fatal("own created setup could not be rediscovered")
	}
	view := matrixCall(t, fx, router, actor, base+"/settings/read", map[string]any{}, 200, "", "", nil)
	if view["context"].(map[string]any)["membership_id"] != member || view["context"].(map[string]any)["capability_enabled"] != false || view["setup"].(map[string]any)["ready"] != true {
		t.Fatal("persisted capability-off setup misrepresented")
	}
	page := matrixCall(t, fx, router, actor, base+"/settings/responsibility-members/list", map[string]any{}, 200, "", "", nil)
	if page["account_id"] != actor.account || page["organization_id"] != org || page["membership_id"] != member || len(page["members"].([]any)) != 1 {
		t.Fatal("named picker binding/projection changed")
	}
	for _, endpoint := range []string{"/v1/org-access/organizations/setups/list", base + "/settings/responsibility-members/list"} {
		matrixCall(t, fx, router, actor, endpoint, map[string]any{}, 401, "UNAUTHENTICATED", "", func(r *http.Request) { r.Header.Del("Cookie") })
	}
	for _, body := range []map[string]any{{"limit": 0}, {"limit": 101}, {"account_id": actor.account}, {"after_membership_id": "invalid"}} {
		matrixCall(t, fx, router, actor, base+"/settings/responsibility-members/list", body, 400, "MALFORMED", "", nil)
	}
	for _, body := range []map[string]any{{"limit": 0}, {"limit": 101}, {"account_id": actor.account}, {"after_org_id": "invalid"}} {
		matrixCall(t, fx, router, actor, "/v1/org-access/organizations/setups/list", body, 400, "MALFORMED", "", nil)
	}
	foreign := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	matrixCall(t, fx, router, actor, "/v1/orgs/"+foreign.scope.OrgID().String()+"/settings/responsibility-members/list", map[string]any{}, 404, "NOT_FOUND", "", nil)
	other := matrixNewPerson(t, fx, router)
	empty := matrixCall(t, fx, router, other, "/v1/org-access/organizations/setups/list", map[string]any{}, 200, "", "", nil)
	if len(empty["setups"].([]any)) != 0 {
		t.Fatal("another account discovered an owned receipt")
	}
	owned := fx
	owned.scope = scopeFor(t, org)
	owned.member = member
	employee, substitute := teamDriver(t, owned.legalFixtureState, other.account)
	if e := unitSchemaExec(t, owned.legalFixtureState, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name) VALUES($1,$2,'Synthetic named substitute')`, org, employee); e != nil {
		t.Fatal(e)
	}
	first := matrixCall(t, fx, router, actor, base+"/settings/responsibility-members/list", map[string]any{"limit": 1}, 200, "", "", nil)
	cursor := first["next_after_membership_id"].(string)
	if cursor == "" {
		t.Fatal("named page omitted keyset")
	}
	second := matrixCall(t, fx, router, actor, base+"/settings/responsibility-members/list", map[string]any{"limit": 1, "after_membership_id": cursor}, 200, "", "", nil)
	if second["next_after_membership_id"] != "" {
		t.Fatal("named page did not terminate")
	}
	names := map[string]map[string]any{}
	for _, p := range []map[string]any{first, second} {
		for _, entry := range p["members"].([]any) {
			row := entry.(map[string]any)
			if len(row) != 3 {
				t.Fatal("picker leaked other identity/contact fields")
			}
			names[row["membership_id"].(string)] = row
		}
	}
	if len(names) != 2 || names[substitute]["display_name"] != "Synthetic named substitute" || names[member]["profile_pending"] != true || names[member]["display_name"] != "" {
		t.Fatal("picker inferred identity IDs or invented a profile")
	}
	if _, e := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at-interval '20 minutes',created_at=created_at-interval '20 minutes',expires_at=expires_at-interval '20 minutes' WHERE id=$1`, actor.session); e != nil {
		t.Fatal(e)
	}
	stale := matrixCall(t, fx, router, actor, base+"/settings/read", map[string]any{}, 200, "", "", nil)
	if stale["context"].(map[string]any)["fresh"] != false {
		t.Fatal("elapsed freshness kept settings controls fresh")
	}
	matrixCall(t, fx, router, actor, base+"/settings/responsibility-members/list", map[string]any{}, 401, "FRESH_AUTH_REQUIRED", "", nil)
	if _, e := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at+interval '20 minutes',created_at=created_at+interval '20 minutes',expires_at=expires_at+interval '20 minutes' WHERE id=$1`, actor.session); e != nil {
		t.Fatal(e)
	}
	values := setupHTTPClone(t, view["values"])
	values["time_zone"] = "Pacific/Auckland"
	values["policies"].(map[string]any)["require_invitation_confirmation"] = true
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "values": values, "reason": "Synthetic panel settings revision"}
	matrixCall(t, fx, router, actor, base+"/settings/change", change, 403, "ENTITLEMENT_REQUIRED", "", nil)
	if _, e := fx.flags.Set(t.Context(), owned.scope, cap.Actor{AccountID: actor.account}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, owned.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, substitute); e != nil {
		t.Fatal(e)
	}
	available := matrixCall(t, fx, router, actor, base+"/settings/responsibility-members/list", map[string]any{}, 200, "", "", nil)
	if len(available["members"].([]any)) != 1 {
		t.Fatal("suspended member remained eligible")
	}
	invalid := setupHTTPClone(t, change)
	invalid["values"].(map[string]any)["responsibilities"].(map[string]any)["billing"].(map[string]any)["substitute"] = substitute
	matrixCall(t, fx, router, actor, base+"/settings/change", invalid, 422, "VALIDATION_FAILED", "", nil)
	if e := unitSchemaExec(t, owned.legalFixtureState, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, substitute); e != nil {
		t.Fatal(e)
	}
	values["responsibilities"].(map[string]any)["billing"].(map[string]any)["substitute"] = substitute
	ack := matrixCall(t, fx, router, actor, base+"/settings/change", change, 200, "", "", nil)
	if ack["revision"] != float64(2) || ack["root_revision"] != float64(2) {
		t.Fatal("settings/root revisions conflated")
	}
	retry := matrixCall(t, fx, router, actor, base+"/settings/change", change, 200, "", "", nil)
	if retry["replayed"] != true || retry["effective_at"] != ack["effective_at"] {
		t.Fatal("settings retry lost original receipt")
	}
	flipped := setupHTTPClone(t, change)
	flipped["values"].(map[string]any)["policies"].(map[string]any)["require_invitation_confirmation"] = false
	matrixCall(t, fx, router, actor, base+"/settings/change", flipped, 409, "OPERATION_ID_REUSED", "", nil)
	conflict := setupHTTPClone(t, change)
	conflict["operation_id"] = onboardingID(t)
	matrixCall(t, fx, router, actor, base+"/settings/change", conflict, 409, "CONFLICT", "", nil)
	historical := matrixCall(t, fx, router, actor, base+"/settings/revision/read", map[string]any{"revision": 1}, 200, "", "", nil)
	if historical["revision"] != float64(1) || historical["root_revision"] != float64(2) || historical["values"].(map[string]any)["time_zone"] != "America/Cuiaba" || historical["organization"].(map[string]any)["time_zone"] != "Pacific/Auckland" {
		t.Fatal("historical values mislabeled as current root")
	}
	reader := other
	reader.member = substitute
	reader.employee = employee
	matrixAssign(t, owned, reader, "AUDITOR", "ORG", "ACTIVE")
	readOnly := matrixCall(t, fx, router, reader, base+"/settings/read", map[string]any{}, 200, "", "", nil)
	if len(readOnly["context"].(map[string]any)["permissions"].([]any)) != 1 {
		t.Fatal("read-only member received write hints")
	}
	matrixCall(t, fx, router, reader, base+"/settings/responsibility-members/list", map[string]any{}, 403, "FORBIDDEN", "org.settings.manage", nil)
	matrixCall(t, fx, router, reader, base+"/settings/change", conflict, 403, "FORBIDDEN", "org.settings.manage", nil)

}

func setupHTTPClone(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("synthetic HTTP body clone")
	}
	return out
}
