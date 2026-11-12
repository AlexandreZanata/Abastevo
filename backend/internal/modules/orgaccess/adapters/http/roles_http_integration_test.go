//go:build integration

package http_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func randomUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type fixedClock struct{ t time.Time }

func (c fixedClock) NowUnix() int64 { return c.t.Unix() }

func TestRoles_ComposedHTTP_Lifecycle(t *testing.T) {
	f := orgtest.New(t)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)

	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	grantSQL := "GRANT USAGE ON SCHEMA public TO " + r + ";" +
		" GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments,org_audit_events,org_units,org_onboarding_commands,org_cost_centres,org_legal_entities,org_establishments,org_capabilities,identity_rate_windows TO " + r + ";" +
		" GRANT SELECT,UPDATE ON accounts,account_session_families TO " + r + ";" +
		" GRANT SELECT,INSERT ON payment_billing_exits,org_settings_commands,org_unit_commands,org_cost_centre_commands,org_legal_commands,org_employee_commands TO " + r + ";" +
		" GRANT SELECT,INSERT,UPDATE,DELETE ON company_web_sessions TO " + r + ";"
	if _, err := f.Admin.Exec(t.Context(), grantSQL); err != nil {
		t.Fatal(err)
	}

	clock := fixedClock{t: time.Now().UTC().Truncate(time.Second)}
	accounts := &account.Service{
		Clock:    clock,
		Store:    accountpg.NewPGStore(f.Admin),
		Hasher:   accountdomain.SHA256Hasher{},
		IDGen:    jobs.NewUUIDv4,
		TokenGen: accountdomain.GenerateToken,
		KeyGen:   accountdomain.GenerateAccountKey,
	}

	// Create owner personal key account and login
	keyAccount, err := accounts.CreateKeyAccount(t.Context(), "roleownerhttp")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), keyAccount.Key)
	if err != nil {
		t.Fatal(err)
	}

	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	cursorSecret := bytes.Repeat([]byte{7}, 32)
	opts := enterprisesuite.Options{
		Enabled:      true,
		Origin:       origin,
		CursorSecret: cursorSecret,
		Pool:         f.Runtime,
		Accounts:     accounts,
	}
	if err := enterprisesuite.Register(router, opts); err != nil {
		t.Fatal(err)
	}

	var cookie *http.Cookie
	csrf := ""

	call := func(path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal("json marshal error")
		}
		request := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		if cookie != nil {
			request.AddCookie(cookie)
			request.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(request)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, request)
		if rec.Code != want {
			t.Fatalf("HTTP status mismatch on %s: got %d want %d; body: %s", path, rec.Code, want, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("ETag") != "" {
			t.Fatalf("Cache-Control/ETag header violation on %s", path)
		}
		var result map[string]any
		if len(rec.Body.Bytes()) > 0 {
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatalf("response unmarshal error: %v", err)
			}
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing company cookie")
			}
			cookie = cookies[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}

	// 1. Login to get company session cookie and CSRF
	call("/v1/org-access/session/login", map[string]any{
		"family_id":    auth.Session.FamilyID,
		"access_token": auth.Session.AccessToken,
	}, 200, nil)

	// 2. Create organization
	orgOpID := randomUUID(t)
	createOrgRes := call("/v1/org-access/organizations/create", map[string]any{
		"schema_version": 1,
		"operation_id":   orgOpID,
		"name":           "Role Management Test Org",
		"time_zone":      "America/Sao_Paulo",
		"kinds":          []string{"OTHER"},
		"unit_name":      "Main Office",
		"terms": map[string]any{
			"version":  orgdomain.SyntheticTermsVersion,
			"digest":   orgdomain.SyntheticTermsDigest,
			"accepted": true,
		},
	}, 201, nil)

	orgID := createOrgRes["organization_id"].(string)
	ownerMemberID := createOrgRes["membership_id"].(string)
	ownerAssignID := createOrgRes["assignment_id"].(string)
	parsedOrgID, err := kernel.ParseOrgID(orgID)
	if err != nil {
		t.Fatal(err)
	}
	orgScope, err := kernel.NewOrgScope(parsedOrgID)
	if err != nil {
		t.Fatal(err)
	}

	// Enable "organizations" capability flag
	capCatalog, err := cap.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	capAuditCatalog, err := audit.NewCatalog(cappg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	capAuditStore := auditpg.NewStore(f.Runtime, capAuditCatalog)
	flagsStore := cappg.NewStore(f.Runtime, capCatalog, capAuditStore)
	if _, err := flagsStore.Set(t.Context(), orgScope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{
		OperationID: randomUUID(t),
		Capability:  "organizations",
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}

	// 3. Create a second account and membership to receive roles
	memberAccountID := randomUUID(t)
	memberEmployeeID := randomUUID(t)
	memberID := randomUUID(t)
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, memberAccountID, "target-member-acc"); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, orgScope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, orgID, memberEmployeeID); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, orgID, memberID, memberEmployeeID, memberAccountID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	rolesBase := "/v1/orgs/" + orgID + "/roles"

	// 4. Test missing CSRF / bad Origin on POST /assign -> 401
	assignOpID := randomUUID(t)
	assignBody := map[string]any{
		"schema_version": 1,
		"operation_id":   assignOpID,
		"membership_id":  memberID,
		"role":           "ADMIN",
		"scope":          "ORG",
		"target_id":      orgID,
		"reason":         "Assigning administrator via HTTP",
	}
	call(rolesBase+"/assign", assignBody, 401, func(r *http.Request) {
		r.Header.Del("X-CSRF")
	})
	call(rolesBase+"/assign", assignBody, 401, func(r *http.Request) {
		r.Header.Set("Origin", "https://malicious.example.invalid")
	})

	// 5. Positive assign: Owner assigns ADMIN to member
	assignRes := call(rolesBase+"/assign", assignBody, 201, nil)
	assignID := assignRes["id"].(string)
	if assignRes["role"] != "ADMIN" || assignRes["membership_id"] != memberID || assignRes["replayed"] != false {
		t.Fatalf("unexpected assign response: %+v", assignRes)
	}

	// 6. Replay assign: same operation_id -> 201 with replayed = true
	replayRes := call(rolesBase+"/assign", assignBody, 201, nil)
	if replayRes["id"] != assignID || replayRes["replayed"] != true {
		t.Fatalf("unexpected replay response: %+v", replayRes)
	}

	// 7. Reused operation_id with different payload -> 409
	reusedBody := map[string]any{
		"schema_version": 1,
		"operation_id":   assignOpID,
		"membership_id":  memberID,
		"role":           "DRIVER",
		"scope":          "SELF",
		"target_id":      memberID,
		"reason":         "Reused operation ID",
	}
	call(rolesBase+"/assign", reusedBody, 409, nil)

	// 8. Self-assignment refusal: Owner tries to assign role to self -> 403 Forbidden
	selfAssignBody := map[string]any{
		"schema_version": 1,
		"operation_id":   randomUUID(t),
		"membership_id":  ownerMemberID,
		"role":           "DRIVER",
		"scope":          "SELF",
		"target_id":      ownerMemberID,
		"reason":         "Self assignment attempt",
	}
	call(rolesBase+"/assign", selfAssignBody, 403, nil)

	// 9. Read role assignment via HTTP
	readRes := call(rolesBase+"/"+assignID+"/read", map[string]any{}, 200, nil)
	if readRes["id"] != assignID || readRes["role"] != "ADMIN" {
		t.Fatalf("unexpected read response: %+v", readRes)
	}

	// 10. List role assignments via HTTP
	listRes := call(rolesBase+"/list", map[string]any{
		"page":  map[string]any{"limit": 10},
		"state": "ACTIVE",
	}, 200, nil)
	items := listRes["items"].([]any)
	if len(items) < 2 { // Owner + Admin
		t.Fatalf("expected at least 2 active assignments, got %d", len(items))
	}

	// 11. Change role atomically: change member from ADMIN to FINANCE
	changeOpID := randomUUID(t)
	changeBody := map[string]any{
		"schema_version": 1,
		"operation_id":   changeOpID,
		"revision":       1,
		"role":           "FINANCE",
		"scope":          "ORG",
		"target_id":      orgID,
		"reason":         "Changing administrator to finance role",
	}
	changeRes := call(rolesBase+"/"+assignID+"/change", changeBody, 200, nil)
	endedObj := changeRes["ended_assignment"].(map[string]any)
	newObj := changeRes["new_assignment"].(map[string]any)
	if endedObj["id"] != assignID || endedObj["ended_at"] == nil {
		t.Fatalf("unexpected ended assignment in change: %+v", endedObj)
	}
	newAssignID := newObj["id"].(string)
	if newObj["role"] != "FINANCE" || newObj["revision"] != float64(1) {
		t.Fatalf("unexpected new assignment in change: %+v", newObj)
	}

	// 12. End role: try to end sole owner -> 409 LAST_OWNER
	endSoleOwnerBody := map[string]any{
		"schema_version": 1,
		"operation_id":   randomUUID(t),
		"revision":       1,
		"reason":         "Attempt to remove only owner",
	}
	call(rolesBase+"/"+ownerAssignID+"/end", endSoleOwnerBody, 409, nil)

	// 13. End the newly changed finance role -> 200 OK
	endFinanceBody := map[string]any{
		"schema_version": 1,
		"operation_id":   randomUUID(t),
		"revision":       1,
		"reason":         "Terminating finance role",
	}
	endRes := call(rolesBase+"/"+newAssignID+"/end", endFinanceBody, 200, nil)
	if endRes["id"] != newAssignID || endRes["ended_at"] == nil {
		t.Fatalf("unexpected end response: %+v", endRes)
	}
}
