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
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestOwnershipTransferThroughComposedCompanyHTTP(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role+"; GRANT INSERT ON accounts TO "+role+"; GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers TO "+role); err != nil {
		t.Fatal(err)
	}

	accounts := &account.Service{
		Clock:    onboardingClock{time.Now().UTC().Truncate(time.Second)},
		Store:    accountpg.NewPGStore(fx.f.Admin),
		Hasher:   accountdomain.SHA256Hasher{},
		IDGen:    jobs.NewUUIDv4,
		TokenGen: accountdomain.GenerateToken,
		KeyGen:   accountdomain.GenerateAccountKey,
	}

	// Owner personal account
	ownerKey, err := accounts.CreateKeyAccount(t.Context(), "synthtransferowner")
	if err != nil {
		t.Fatal(err)
	}
	ownerAuth, err := accounts.LoginWithKey(t.Context(), ownerKey.Key)
	if err != nil {
		t.Fatal(err)
	}

	// Target personal account
	targetKey, err := accounts.CreateKeyAccount(t.Context(), "synthtransfertarget")
	if err != nil {
		t.Fatal(err)
	}
	targetAuth, err := accounts.LoginWithKey(t.Context(), targetKey.Key)
	if err != nil {
		t.Fatal(err)
	}

	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{
		Enabled:      true,
		Origin:       origin,
		CursorSecret: bytes.Repeat([]byte{9}, 32),
		Pool:         fx.f.Runtime,
		Accounts:     accounts,
	}); err != nil {
		t.Fatal(err)
	}

	var activeCookie *http.Cookie
	activeCSRF := ""

	call := func(path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal("request encoding failed")
		}
		request := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		if activeCookie != nil {
			request.AddCookie(activeCookie)
			request.Header.Set("X-CSRF", activeCSRF)
		}
		if edit != nil {
			edit(request)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatalf("HTTP mismatch at %s: got %d want %d, body: %s", path, response.Code, want, response.Body.String())
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("invalid response JSON")
		}
		return result
	}

	// 1. Login as owner
	loginReq := httptest.NewRequest(http.MethodPost, origin+"/v1/org-access/session/login", bytes.NewReader(mustJSON(t, map[string]any{
		"family_id":    ownerAuth.Session.FamilyID,
		"access_token": ownerAuth.Session.AccessToken,
	})))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", origin)
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != 200 {
		t.Fatalf("login failed: %d, body: %s", loginRec.Code, loginRec.Body.String())
	}
	var loginBody map[string]any
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginBody)
	ownerCookie := loginRec.Result().Cookies()[0]
	ownerCSRF := loginBody["csrf_token"].(string)

	// Target login
	loginTargetReq := httptest.NewRequest(http.MethodPost, origin+"/v1/org-access/session/login", bytes.NewReader(mustJSON(t, map[string]any{
		"family_id":    targetAuth.Session.FamilyID,
		"access_token": targetAuth.Session.AccessToken,
	})))
	loginTargetReq.Header.Set("Content-Type", "application/json")
	loginTargetReq.Header.Set("Origin", origin)
	loginTargetRec := httptest.NewRecorder()
	router.ServeHTTP(loginTargetRec, loginTargetReq)
	if loginTargetRec.Code != 200 {
		t.Fatalf("target login failed: %d", loginTargetRec.Code)
	}
	var targetLoginBody map[string]any
	_ = json.Unmarshal(loginTargetRec.Body.Bytes(), &targetLoginBody)
	targetCookie := loginTargetRec.Result().Cookies()[0]
	targetCSRF := targetLoginBody["csrf_token"].(string)

	// Use owner credentials
	activeCookie = ownerCookie
	activeCSRF = ownerCSRF

	// 2. Create and activate organization
	createOrgResult := call("/v1/org-access/organizations/create", map[string]any{
		"schema_version": 1,
		"operation_id":   onboardingID(t),
		"name":           "Transfer Test Org",
		"time_zone":      "America/Cuiaba",
		"kinds":          []string{"OTHER"},
		"unit_name":      "HQ",
		"terms": map[string]any{
			"version":  d.SyntheticTermsVersion,
			"digest":   d.SyntheticTermsDigest,
			"accepted": true,
		},
	}, 201, nil)

	orgID := createOrgResult["organization_id"].(string)
	scope := scopeFor(t, orgID)

	// Enable organizations capability
	if _, err := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: ownerAuth.Account.ID}, cap.Change{
		OperationID: onboardingID(t),
		Capability:  "organizations",
		Enabled:     true,
	}); err != nil {
		t.Fatal(err)
	}

	// Activate org
	call("/v1/orgs/"+orgID+"/organization/activate", map[string]any{
		"schema_version": 1,
		"operation_id":   onboardingID(t),
		"revision":       1,
	}, 200, nil)

	// 3. Register target employee and membership in this org
	fx.scope = scope
	var ownerMemberID string
	if err := fx.f.Admin.QueryRow(t.Context(), "SELECT id FROM org_memberships WHERE org_id = $1 AND account_id = $2", orgID, ownerAuth.Account.ID).Scan(&ownerMemberID); err != nil {
		t.Fatal(err)
	}

	targetEmployeeID := onboardingID(t)
	targetMemberID := onboardingID(t)
	if err := unitSchemaExec(t, fx, "INSERT INTO org_employee_identities(org_id, id, state) VALUES ($1, $2, 'ACTIVE')", orgID, targetEmployeeID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "INSERT INTO org_memberships(org_id, id, employee_id, account_id, access_model, state) VALUES ($1, $2, $3, $4, 'EXISTING_ACCOUNT', 'INVITED')", orgID, targetMemberID, targetEmployeeID, targetAuth.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "INSERT INTO org_role_assignments(org_id, id, membership_id, role, scope_type, scope_target, valid_from, assigned_by, reason, revision) VALUES ($1, $2, $3, 'ADMIN', 'ORG', $1, NOW(), $4, 'Seed admin', 1)", orgID, onboardingID(t), targetMemberID, ownerMemberID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "UPDATE org_memberships SET state = 'ACTIVE', revision = revision + 1 WHERE org_id = $1 AND id = $2", orgID, targetMemberID); err != nil {
		t.Fatal(err)
	}

	// Managed account target
	managedAccountID := onboardingID(t)
	managedEmployeeID := onboardingID(t)
	managedMemberID := onboardingID(t)
	if err := unitSchemaExec(t, fx, "INSERT INTO accounts(id, alias, status, enterprise_managed_org_id) VALUES ($1, 'managed-transfer-test', 'active', $2)", managedAccountID, orgID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "INSERT INTO org_employee_identities(org_id, id, state) VALUES ($1, $2, 'ACTIVE')", orgID, managedEmployeeID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "INSERT INTO org_memberships(org_id, id, employee_id, account_id, access_model, state) VALUES ($1, $2, $3, $4, 'MANAGED', 'INVITED')", orgID, managedMemberID, managedEmployeeID, managedAccountID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "INSERT INTO org_role_assignments(org_id, id, membership_id, role, scope_type, scope_target, valid_from, assigned_by, reason, revision) VALUES ($1, $2, $3, 'ADMIN', 'ORG', $1, NOW(), $4, 'Seed admin', 1)", orgID, onboardingID(t), managedMemberID, ownerMemberID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, "UPDATE org_memberships SET state = 'ACTIVE', revision = revision + 1 WHERE org_id = $1 AND id = $2", orgID, managedMemberID); err != nil {
		t.Fatal(err)
	}

	base := "/v1/orgs/" + orgID + "/ownership-transfer"

	// 4. Test Propose Security / Negative Guards
	proposeBody := map[string]any{
		"schema_version":       1,
		"operation_id":         onboardingID(t),
		"target_membership_id": targetMemberID,
		"step_down_to_admin":   true,
	}

	// Unauthenticated
	call(base+"/propose", proposeBody, 401, func(r *http.Request) { r.Header.Del("Cookie") })
	call(base+"/propose", proposeBody, 401, func(r *http.Request) { r.Header.Del("X-CSRF") })
	call(base+"/propose", proposeBody, 401, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") })

	// Malformed (missing schema_version)
	call(base+"/propose", map[string]any{
		"operation_id":         onboardingID(t),
		"target_membership_id": targetMemberID,
	}, 400, nil)

	// Self-transfer forbidden
	call(base+"/propose", map[string]any{
		"schema_version":       1,
		"operation_id":         onboardingID(t),
		"target_membership_id": ownerMemberID,
		"step_down_to_admin":   true,
	}, 409, nil)

	// Managed account target forbidden (BR-ORG-026)
	call(base+"/propose", map[string]any{
		"schema_version":       1,
		"operation_id":         onboardingID(t),
		"target_membership_id": managedMemberID,
		"step_down_to_admin":   true,
	}, 409, nil)

	// 5. Positive Propose
	opID := onboardingID(t)
	proposeResp := call(base+"/propose", map[string]any{
		"schema_version":       1,
		"operation_id":         opID,
		"target_membership_id": targetMemberID,
		"step_down_to_admin":   true,
	}, 200, nil)

	transferObj := proposeResp["transfer"].(map[string]any)
	transferID := transferObj["transfer_id"].(string)
	if transferObj["state"] != "PENDING" || proposeResp["replayed"] != false {
		t.Fatalf("unexpected proposal response: %+v", proposeResp)
	}

	// Replay propose
	replayResp := call(base+"/propose", map[string]any{
		"schema_version":       1,
		"operation_id":         opID,
		"target_membership_id": targetMemberID,
		"step_down_to_admin":   true,
	}, 200, nil)
	if replayResp["replayed"] != true {
		t.Fatalf("expected replayed propose: %+v", replayResp)
	}

	// Second pending proposal forbidden (TRANSFER_PENDING, 409)
	call(base+"/propose", map[string]any{
		"schema_version":       1,
		"operation_id":         onboardingID(t),
		"target_membership_id": targetMemberID,
		"step_down_to_admin":   true,
	}, 409, nil)

	// 6. Read Pending Transfer
	readResp := call(base+"/read", map[string]any{}, 200, nil)
	if readResp["transfer_id"] != transferID || readResp["state"] != "PENDING" {
		t.Fatalf("unexpected read response: %+v", readResp)
	}

	// 7. Accept Transfer
	acceptOpID := onboardingID(t)
	acceptBody := map[string]any{
		"transfer_id":  transferID,
		"operation_id": acceptOpID,
	}

	// Non-target tries to accept (owner calls accept -> 403 Forbidden)
	call(base+"/accept", acceptBody, 403, nil)

	// Target member calls accept
	activeCookie = targetCookie
	activeCSRF = targetCSRF

	acceptResp := call(base+"/accept", acceptBody, 200, nil)
	acceptTransferObj := acceptResp["transfer"].(map[string]any)
	if acceptTransferObj["state"] != "ACCEPTED" || acceptResp["replayed"] != false {
		t.Fatalf("unexpected accept response: %+v", acceptResp)
	}

	// Replay accept
	replayAcceptResp := call(base+"/accept", acceptBody, 200, nil)
	if replayAcceptResp["replayed"] != true {
		t.Fatalf("expected replayed accept: %+v", replayAcceptResp)
	}

	// Verify target member is now OWNER
	var targetOwnerCount int
	_ = fx.f.Admin.QueryRow(t.Context(), "SELECT COUNT(*) FROM org_role_assignments WHERE org_id = $1 AND membership_id = $2 AND role = 'OWNER' AND ended_at IS NULL", orgID, targetMemberID).Scan(&targetOwnerCount)
	if targetOwnerCount != 1 {
		t.Fatalf("expected target to be active owner, got %d", targetOwnerCount)
	}

	// Verify initiator stepped down to ADMIN
	var initiatorOwnerCount int
	_ = fx.f.Admin.QueryRow(t.Context(), "SELECT COUNT(*) FROM org_role_assignments WHERE org_id = $1 AND membership_id = $2 AND role = 'OWNER' AND ended_at IS NULL", orgID, ownerMemberID).Scan(&initiatorOwnerCount)
	if initiatorOwnerCount != 0 {
		t.Fatalf("expected initiator to step down from owner, got %d", initiatorOwnerCount)
	}
	var initiatorAdminCount int
	_ = fx.f.Admin.QueryRow(t.Context(), "SELECT COUNT(*) FROM org_role_assignments WHERE org_id = $1 AND membership_id = $2 AND role = 'ADMIN' AND ended_at IS NULL", orgID, ownerMemberID).Scan(&initiatorAdminCount)
	if initiatorAdminCount != 1 {
		t.Fatalf("expected initiator to have active admin role, got %d", initiatorAdminCount)
	}

	// 8. Test Cancel
	// Target is now owner and can propose a transfer back to owner
	cancelOpID := onboardingID(t)
	secondTransferResp := call(base+"/propose", map[string]any{
		"schema_version":       1,
		"operation_id":         cancelOpID,
		"target_membership_id": ownerMemberID,
		"step_down_to_admin":   false,
	}, 200, nil)
	secondTransferID := secondTransferResp["transfer"].(map[string]any)["transfer_id"].(string)

	// Target cancels the transfer
	cancelResp := call(base+"/cancel", map[string]any{
		"transfer_id":  secondTransferID,
		"operation_id": onboardingID(t),
	}, 200, nil)
	if cancelResp["transfer"].(map[string]any)["state"] != "CANCELLED" {
		t.Fatalf("expected cancelled state: %+v", cancelResp)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
