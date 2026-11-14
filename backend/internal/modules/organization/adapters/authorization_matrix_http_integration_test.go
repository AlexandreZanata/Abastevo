//go:build integration

package adapters_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type matrixRoute struct {
	permission, path string
	fresh, write     bool
}

// These expectations are literal policy examples, independent of the catalog
// implementation. The independent v4 PSV supplies every role's cell outcome.
var matrixRepresentatives = []matrixRoute{
	{"org.ownership.transfer", "/ownership-transfer/propose", true, true},
	{"org.ownership.respond", "/ownership-transfer/read", true, false},
	{"auth.role.assign", "/roles/list", true, false},
	{"idn.credential.manage", "/memberships/{membership_id}/devices/list", true, false},
	{"idn.employee.manage", "/employees/matches", false, false},
	{"idn.employee.read", "/employees/list", false, false},
	{"idn.invitation.manage", "/invitations/list", true, false},
	{"idn.membership.manage", "/memberships/{membership_id}/read", true, false},
	{"org.legal_entity.manage", "/legal-entities/list", false, false},
	{"org.settings.manage", "/settings/change", true, true},
	{"org.settings.read", "/settings/read", false, false},
	{"org.structure.manage", "/units/list", false, false},
	{"fleet.asset.manage", "/fleet/assets/create", true, true},
	{"fleet.asset.read", "/fleet/assets/list", false, false},
	{"meter.reading.correct", "/sync/reviews/meter-readings/list", false, false},
}

func matrixOracle(t *testing.T) ([]string, map[string]map[string]string) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("authorization test location unavailable")
	}
	directory := filepath.Join(filepath.Dir(source), "../../../../../contracts/testdata/enterprise")
	file, err := os.Open(filepath.Join(directory, "authorization-catalog-v4.psv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("authorization oracle header absent")
	}
	header := strings.Split(scanner.Text(), "|")
	wantHeader := "permission|OWNER|ADMIN|FLEET_MANAGER|SUPERVISOR|FINANCE|APPROVER|DRIVER|OPERATIONS_STAFF|AUDITOR|STATION_ADMIN|STATION_MANAGER|STATION_ATTENDANT|DATA_MANAGER"
	if strings.Join(header, "|") != wantHeader {
		t.Fatal("independent authorization role vector drift")
	}
	cells := map[string]map[string]string{}
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != len(header) || parts[0] == "" || cells[parts[0]] != nil {
			t.Fatal("invalid independent authorization row")
		}
		cells[parts[0]] = map[string]string{}
		for i, role := range header[1:] {
			cell := parts[i+1]
			if cell != "-" && cell != "O" && cell != "S" && !strings.HasPrefix(cell, "O:") && !strings.HasPrefix(cell, "S:") {
				t.Fatal("unknown independent authorization cell")
			}
			cells[parts[0]][role] = cell
		}
	}
	if scanner.Err() != nil || len(cells) != 85 {
		t.Fatal("authorization oracle must cover all 85 keys")
	}
	var inventory struct {
		Version            int    `json:"schema_version"`
		Roles              string `json:"roles_version"`
		PermissionsVersion string `json:"permissions_version"`
		Permissions        []struct {
			Permission, Status string
			Representative     string `json:"representative_route"`
			OperationType      string `json:"operation_type"`
			OperationSchema    int64  `json:"operation_schema"`
		} `json:"permissions"`
	}
	raw, err := os.ReadFile(filepath.Join(directory, "authorization-routes-v1.json"))
	if err != nil || json.Unmarshal(raw, &inventory) != nil || inventory.Version != 1 || inventory.Roles != "org-roles-v1" || inventory.PermissionsVersion != "org-permissions-v4" || len(inventory.Permissions) != 85 {
		t.Fatal("independent route inventory unavailable or invalid")
	}
	routed := map[string]string{}
	signed := 0
	for _, entry := range inventory.Permissions {
		if entry.Status == "ROUTED" {
			routed[entry.Permission] = entry.Representative
		}
		if entry.Status == "SIGNED_OPERATION" {
			signed++
			if entry.Permission != "meter.reading.record" || entry.OperationType != "meter.reading.record" || entry.OperationSchema != 1 || entry.Representative != "POST /v1/orgs/{org_id}/sync/operations" {
				t.Fatal("signed producer descriptor and independent inventory disagree")
			}
		}
	}
	if len(routed) != len(matrixRepresentatives) || signed != 1 {
		t.Fatal("new routed permission lacks runtime matrix coverage")
	}
	for _, entry := range matrixRepresentatives {
		if routed[entry.permission] != "POST /v1/orgs/{org_id}"+entry.path || cells[entry.permission] == nil {
			t.Fatal("representative request and independent inventory disagree")
		}
	}
	var roles []string
	for _, role := range header[1:] {
		if !strings.HasPrefix(role, "STATION_") {
			roles = append(roles, role)
		}
	}
	if len(roles) != 10 {
		t.Fatal("ordinary current-source role applicability drift")
	}
	return roles, cells
}

type matrixPerson struct {
	account, member, employee, session string
	cookie                             *http.Cookie
	csrf                               string
}

func matrixNewPerson(t *testing.T, fx deviceFixtureState, router chi.Router) matrixPerson {
	t.Helper()
	key, err := fx.accounts.CreateKeyAccount(t.Context(), "synmatrix"+onboardingID(t)[:8])
	if err != nil {
		t.Fatal(err)
	}
	proof, err := fx.accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	employee, member := teamDriver(t, fx.legalFixtureState, proof.Account.ID)
	raw, err := json.Marshal(map[string]any{"family_id": proof.Session.FamilyID, "access_token": proof.Session.AccessToken})
	if err != nil {
		t.Fatal("synthetic login encoding")
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
		t.Fatal("normal source-backed company login failed")
	}
	var session string
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT id::text FROM company_web_sessions WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, proof.Account.ID).Scan(&session); err != nil {
		t.Fatal(err)
	}
	return matrixPerson{proof.Account.ID, member, employee, session, w.Result().Cookies()[0], result.CSRF}
}

func matrixAssign(t *testing.T, fx deviceFixtureState, actor matrixPerson, role, scope, state string) {
	t.Helper()
	org := fx.scope.OrgID().String()
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, actor.member); err != nil {
		t.Fatal(err)
	}
	if state == "NONE" {
		return
	}
	from, to := time.Now().UTC(), any(nil)
	if state == "EXPIRED" {
		// Migration183 refuses fabricated retroactive intervals. Provision a
		// genuine finite period, then let the actual database clock expire it.
		to = from.Add(200 * time.Millisecond)
	}
	target := org
	if scope == "SELF" {
		target = actor.member
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'Synthetic authorization matrix')`, org, onboardingID(t), actor.member, role, scope, target, from, to, fx.member); err != nil {
		t.Fatal(err)
	}
	if state == "EXPIRED" {
		if _, err := fx.f.Runtime.Exec(t.Context(), `SELECT pg_sleep(GREATEST(0,EXTRACT(epoch FROM ($1::timestamptz-clock_timestamp())))+0.001)`, to); err != nil {
			t.Fatal("actual finite authority interval did not expire")
		}
	}
}

func matrixPrivateState(t *testing.T, fx deviceFixtureState) string {
	t.Helper()
	var value string
	// Session last-seen and admission counters are deliberately excluded. This
	// snapshot checks private business/source effects on every refused request.
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,employee_id) FROM org_employee_profiles x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,revision) FROM org_settings_versions x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,transfer_id) FROM org_ownership_transfers x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_asset_categories x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_assets x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,position) FROM org_asset_tanks x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,tank_position,product) FROM org_asset_tank_products x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,position) FROM org_asset_expected x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,started_at) FROM org_asset_unit_allocations x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,asset_id,revision) FROM org_asset_revisions x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_asset_commands x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_meters x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_meter_segments x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_meter_readings x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_meter_commands x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_sync_operations x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_sync_acknowledgements x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_sync_effects x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_sync_review_items x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,review_item_id) FROM org_meter_held_readings x),
	 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_sync_review_resolutions x),
	 (SELECT count(*) FROM org_audit_events), (SELECT count(*) FROM org_settings_commands),
	 (SELECT count(*) FROM org_ownership_commands), (SELECT count(*) FROM org_role_commands))::text`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func matrixCall(t *testing.T, fx deviceFixtureState, router chi.Router, actor matrixPerson, path string, body any, want int, code, permission string, edit func(*http.Request)) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal("synthetic matrix request encoding")
	}
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	if actor.cookie != nil {
		r.AddCookie(actor.cookie)
		r.Header.Set("X-CSRF", actor.csrf)
	}
	if edit != nil {
		edit(r)
	}
	before := ""
	if want >= 400 {
		before = matrixPrivateState(t, fx)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatalf("source authorization %s: status %d, want %d; code %s", path, w.Code, want, code)
	}
	var result map[string]any
	if json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal("invalid matrix response JSON")
	}
	if want >= 400 {
		if before != matrixPrivateState(t, fx) {
			t.Fatal("refused HTTP request changed private source state")
		}
		errorValue, ok := result["error"].(map[string]any)
		if !ok || errorValue["code"] != code {
			t.Fatal("incorrect bounded authorization error code", errorValue["code"], code)
		}
		if len(result) != 1 || len(errorValue) != 4 || errorValue["message"] != "Organization request unavailable or refused" || errorValue["trace_id"] != "" {
			t.Fatal("authorization error is not the exact safe public envelope")
		}
		details, ok := errorValue["details"].([]any)
		if !ok || permission == "" && len(details) != 0 || permission != "" && len(details) != 1 {
			t.Fatal("authorization refusal carries unapproved private details")
		}
		if permission != "" {
			detail, ok := details[0].(map[string]any)
			if !ok || len(detail) != 2 || detail["field"] != "permission" || detail["code"] != permission {
				t.Fatal("permission detail is not the exact registered-key envelope")
			}
		}
		for _, forbidden := range []string{"Synthetic", actor.member, actor.employee, actor.account, "token_", "secret", "source_authority"} {
			if forbidden != "" && strings.Contains(w.Body.String(), forbidden) {
				t.Fatal("refusal disclosed private authority or resource data")
			}
		}
		if permission != "" && !strings.Contains(w.Body.String(), `"code":"`+permission+`"`) {
			t.Fatal("permission refusal omitted the exact registered key")
		}
	}
	return result
}

func TestAuthorizationMatrixActualCompanyHTTP(t *testing.T) {
	roles, oracle := matrixOracle(t)
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = fleetStore(t, fx.legalFixtureState) // Real ordinary Fleet grants; no fake authority or records.
	fx.f.GrantMeterReviews(t)
	roleSQL := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands,org_role_commands TO "+roleSQL+"; GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers,identity_rate_windows TO "+roleSQL); err != nil {
		t.Fatal(err)
	}
	fx.f.GrantMembershipAccessCommands(t)
	var super, bypass, login bool
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper,rolbypassrls,rolcanlogin FROM pg_roles WHERE rolname=current_user`).Scan(&super, &bypass, &login); err != nil || super || bypass || !login {
		t.Fatal("matrix requires an ordinary NOSUPERUSER/NOBYPASSRLS runtime login")
	}
	router := chi.NewRouter()
	// The existing fixture service provisions accounts as setup only. Every
	// dispatch proof, including account/family validation, uses the ordinary pool.
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{53}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal(err)
	}
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	target, stranger := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	org, otherOrg := fx.scope.OrgID().String(), other.scope.OrgID().String()
	base := "/v1/orgs/" + org
	allowed, denied, exercised := 0, 0, 0
	for roleIndex, role := range roles {
		t.Run(role, func(t *testing.T) {
			actor := matrixNewPerson(t, fx, router)
			matrixAssign(t, fx, actor, role, "ORG", "ACTIVE")
			proposalID := ""
			body := func(entry matrixRoute) any {
				switch entry.permission {
				case "org.ownership.transfer":
					return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "target_membership_id": target.member, "step_down_to_admin": true}
				case "org.ownership.respond":
					return map[string]any{"transfer_id": proposalID}
				case "idn.employee.manage":
					return map[string]any{"display_name": "Synthetic matrix candidate"}
				case "org.settings.manage":
					var revision int64
					var values []byte
					if err := fx.f.Admin.QueryRow(t.Context(), `SELECT h.revision,v.settings_values FROM org_settings_heads h JOIN org_settings_versions v ON(v.org_id,v.revision)=(h.org_id,h.revision) WHERE h.org_id=$1`, org).Scan(&revision, &values); err != nil {
						t.Fatal(err)
					}
					return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": revision, "values": json.RawMessage(values), "reason": "Synthetic authorization matrix change"}
				case "fleet.asset.manage":
					return map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "IMPLEMENT", "name": "Synthetic matrix implement", "fleet_number": "MATRIX-" + strconv.Itoa(roleIndex), "usage_basis": "DISTANCE", "category": map[string]any{"new_label": "Synthetic matrix implements"}}
				default:
					return map[string]any{}
				}
			}
			path := func(entry matrixRoute, organization string) string {
				return "/v1/orgs/" + organization + strings.ReplaceAll(entry.path, "{membership_id}", target.member)
			}
			for _, entry := range matrixRepresentatives {
				if entry.permission == "org.ownership.respond" && proposalID == "" {
					v := matrixCall(t, fx, router, owner, base+"/ownership-transfer/propose", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "target_membership_id": actor.member, "step_down_to_admin": true}, 200, "", "", nil)
					proposalID = v["transfer"].(map[string]any)["transfer_id"].(string)
				}
				cell := oracle[entry.permission][role]
				want, code, key := 200, "", ""
				if entry.permission == "fleet.asset.manage" {
					want = 201
				}
				// Fleet's S cells remain refused until the owning assignment source
				// exists. No creator or other record fact substitutes for SELF.
				if cell == "-" || entry.permission == "fleet.asset.read" && strings.HasPrefix(cell, "S") {
					want, code, key = 403, "FORBIDDEN", entry.permission
					denied++
				} else if strings.HasPrefix(cell, "O") {
					allowed++
				} else {
					t.Fatal("new SELF descriptor needs explicit delivered business-resource cases")
				}
				exercised++
				t.Run("in-scope/"+entry.permission, func(t *testing.T) {
					v := matrixCall(t, fx, router, actor, path(entry, org), body(entry), want, code, key, nil)
					if entry.permission == "org.ownership.transfer" && want == 200 {
						proposalID = v["transfer"].(map[string]any)["transfer_id"].(string)
					}
				})
			}
			if role == "ADMIN" {
				t.Run("admin-cannot-assign-owner", func(t *testing.T) {
					matrixCall(t, fx, router, actor, base+"/roles/assign", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "membership_id": target.member, "role": "OWNER", "scope": "ORG", "target_id": org, "reason": "Synthetic forbidden owner promotion"}, 403, "FORBIDDEN", "auth.role.assign", nil)
				})
			}
			for _, variant := range []string{"missing-proof", "expired-proof", "bad-origin", "bad-csrf", "other-organization", "suspended-membership", "ended-assignment", "expired-assignment", "stale-authentication", "capability-off", "suspended-organization", "self-scope"} {
				if variant == "self-scope" && role == "OWNER" {
					continue
				} // OWNER/SELF is not a persistable source state.
				if variant == "suspended-membership" {
					if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, actor.member); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "ended-assignment" {
					matrixAssign(t, fx, actor, role, "ORG", "NONE")
				}
				if variant == "expired-assignment" {
					matrixAssign(t, fx, actor, role, "ORG", "EXPIRED")
				}
				if variant == "self-scope" {
					matrixAssign(t, fx, actor, role, "SELF", "ACTIVE")
				}
				if variant == "stale-authentication" {
					if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at-interval '20 minutes',created_at=created_at-interval '20 minutes',expires_at=expires_at-interval '20 minutes' WHERE id=$1`, actor.session); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "expired-proof" {
					if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at-interval '9 hours',created_at=created_at-interval '9 hours',last_seen_at=last_seen_at-interval '9 hours',expires_at=expires_at-interval '9 hours' WHERE id=$1`, actor.session); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "capability-off" {
					var revision int64
					if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_capabilities WHERE org_id=$1 AND capability='organizations'`, org).Scan(&revision); err != nil {
						t.Fatal(err)
					}
					if _, err := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: owner.account}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: revision, Enabled: false}); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "suspended-organization" {
					if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, org); err != nil {
						t.Fatal(err)
					}
				}
				for _, entry := range matrixRepresentatives {
					t.Run(variant+"/"+entry.permission, func(t *testing.T) {
						want, code, key := 200, "", ""
						if oracle[entry.permission][role] == "-" || entry.permission == "fleet.asset.read" && strings.HasPrefix(oracle[entry.permission][role], "S") {
							want, code, key = 403, "FORBIDDEN", entry.permission
						}
						organization := org
						var edit func(*http.Request)
						// Write representatives run their positive effect once in the
						// in-scope cell. Every variant below must refuse a new write.
						switch variant {
						case "expired-proof":
							want, code, key = 401, "UNAUTHENTICATED", ""
						case "missing-proof":
							want, code, key = 401, "UNAUTHENTICATED", ""
							edit = func(r *http.Request) { r.Header.Del("Cookie") }
						case "bad-origin":
							want, code, key = 401, "UNAUTHENTICATED", ""
							edit = func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }
						case "bad-csrf":
							want, code, key = 401, "UNAUTHENTICATED", ""
							edit = func(r *http.Request) { r.Header.Del("X-CSRF") }
						case "other-organization":
							want, code, key = 404, "NOT_FOUND", ""
							organization = otherOrg
						case "suspended-membership":
							want, code, key = 403, "MEMBERSHIP_SUSPENDED", ""
						case "ended-assignment", "expired-assignment":
							want, code, key = 403, "FORBIDDEN", entry.permission
						case "stale-authentication":
							if want == 200 && entry.fresh {
								want, code, key = 401, "FRESH_AUTH_REQUIRED", ""
							}
						case "capability-off":
							if want == 200 && entry.write {
								want, code, key = 403, "ENTITLEMENT_REQUIRED", ""
							}
						case "suspended-organization":
							if entry.write {
								want, code, key = 403, "ORG_SUSPENDED", ""
							}
						case "self-scope":
							if entry.permission != "org.ownership.respond" {
								want, code, key = 403, "FORBIDDEN", entry.permission
							}
						}
						matrixCall(t, fx, router, actor, path(entry, organization), body(entry), want, code, key, edit)
					})
				}
				if variant == "suspended-membership" {
					if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, actor.member); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "ended-assignment" || variant == "expired-assignment" || variant == "self-scope" {
					matrixAssign(t, fx, actor, role, "ORG", "ACTIVE")
				}
				if variant == "stale-authentication" {
					if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at+interval '20 minutes',created_at=created_at+interval '20 minutes',expires_at=expires_at+interval '20 minutes' WHERE id=$1`, actor.session); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "expired-proof" {
					if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=authenticated_at+interval '9 hours',created_at=created_at+interval '9 hours',last_seen_at=last_seen_at+interval '9 hours',expires_at=expires_at+interval '9 hours' WHERE id=$1`, actor.session); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "capability-off" {
					var revision int64
					if err := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_capabilities WHERE org_id=$1 AND capability='organizations'`, org).Scan(&revision); err != nil {
						t.Fatal(err)
					}
					if _, err := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: owner.account}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: revision, Enabled: true}); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "suspended-organization" {
					if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ACTIVE',suspension_reason='',revision=revision+1 WHERE org_id=$1`, org); err != nil {
						t.Fatal(err)
					}
				}
			}
			// An ordinary SELF participant can inspect its own proposal, while a
			// different SELF member gets exactly the opaque not-found envelope.
			entry := matrixRepresentatives[1]
			matrixCall(t, fx, router, stranger, base+entry.path, map[string]any{"transfer_id": proposalID}, 404, "NOT_FOUND", "", nil)
			// Ended membership is terminal: use the same cookie in all next requests,
			// never fabricate restoration of that period to continue the matrix.
			if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, actor.member); err != nil {
				t.Fatal(err)
			}
			for _, entry := range matrixRepresentatives {
				t.Run("ended-membership/"+entry.permission, func(t *testing.T) {
					matrixCall(t, fx, router, actor, path(entry, org), body(entry), 404, "NOT_FOUND", "", nil)
				})
			}
			t.Run("membership-ended-personal-company-proof-retained", func(t *testing.T) {
				value := matrixCall(t, fx, router, actor, "/v1/org-access/session/read", map[string]any{}, 200, "", "", nil)
				if value["account_id"] != actor.account {
					t.Fatal("organization offboarding changed the independently owned company/account proof")
				}
			})
			// Revocation is terminal. The same previously successful cookie and
			// actual source request fail immediately when its parent family ends.
			if _, err := fx.f.Runtime.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=(SELECT family_id FROM company_web_sessions WHERE id=$1)`, actor.session); err != nil {
				t.Fatal(err)
			}
			t.Run("parent-family-revoked-next-actual-request", func(t *testing.T) {
				matrixCall(t, fx, router, actor, path(matrixRepresentatives[1], org), body(matrixRepresentatives[1]), 401, "UNAUTHENTICATED", "", nil)
			})
			var pending string
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_ownership_transfers WHERE org_id=$1 AND transfer_id=$2`, org, proposalID).Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending == "PENDING" {
				matrixCall(t, fx, router, owner, base+"/ownership-transfer/cancel", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "transfer_id": proposalID}, 200, "", "", nil)
			}
		})
	}
	if exercised != 150 || allowed != 55 || denied != 95 {
		t.Fatal("runtime matrix coverage drift", exercised, allowed, denied)
	}
	t.Logf("actual CompanyOnly HTTP: %d runtime cells (%d allowed/%d denied), 10 ordinary roles; 42 station cells unavailable; 69 permissions not yet routed; one actual signed Meter producer; Fleet S cells remain denied pending actual assignment integration", exercised, allowed, denied)
}
