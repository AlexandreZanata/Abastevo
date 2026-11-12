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

func TestLegalDeclarationsThroughComposedCompanyHTTP(t *testing.T) {
	fx := legalFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticlegalhttp")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://company.example.invalid"
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{9}, 32), Pool: fx.f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	csrf := ""
	sensitive := syntheticLegalCPF(73)
	call := func(path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal("request encoding failed")
		}
		request := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
			request.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(request)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		// Never dump a potentially sensitive response, even on a failed assertion.
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatalf("private HTTP status/policy mismatch: got %d want %d", response.Code, want)
		}
		if (response.Code != 200 || !strings.HasSuffix(path, "/identifier/read")) && strings.Contains(response.Body.String(), sensitive) {
			t.Fatal("default/error response leaked identifier")
		}
		var result map[string]any
		if json.Unmarshal(response.Body.Bytes(), &result) != nil {
			t.Fatal("invalid response JSON")
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing company cookie")
			}
			cookie = cookies[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, nil)
	createOrg := func() string {
		t.Helper()
		result := call("/v1/org-access/organizations/create", map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "name": "Synthetic legal HTTP", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}, 201, nil)
		id := result["organization_id"].(string)
		if _, err := fx.flags.Set(t.Context(), scopeFor(t, id), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	orgID := createOrg()
	otherID := createOrg()
	base := "/v1/orgs/" + orgID + "/legal-entities"
	other := "/v1/orgs/" + otherID + "/legal-entities"
	natural := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "NATURAL_PERSON", "legal_name": "Synthetic private party", "identifier": sensitive, "purpose": "VERIFICATION_AND_AGREEMENTS"}
	for _, test := range []struct {
		edit   func(*http.Request)
		status int
	}{
		{func(r *http.Request) { r.Header.Del("Cookie") }, 401},
		{func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, 401},
		{func(r *http.Request) { r.Header.Del("X-CSRF") }, 401},
		{func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, 401},
		{func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, 401},
	} {
		call(base+"/create", natural, test.status, test.edit)
	}
	for _, field := range []string{"account_id", "state", "verified", "reveal_identifier"} {
		natural[field] = "VERIFIED"
		call(base+"/create", natural, 400, nil)
		delete(natural, field)
	}
	natural["establishment"] = map[string]any{}
	call(base+"/create", natural, 422, nil)
	delete(natural, "establishment")
	declared := call(base+"/create", natural, 201, nil)
	id := declared["legal_entity_id"].(string)
	if declared["state"] != "DECLARED" || declared["replayed"] != false {
		t.Fatal("declaration implied verification or replay")
	}
	repeated := call(base+"/create", natural, 201, nil)
	if repeated["legal_entity_id"] != id || repeated["replayed"] != true {
		t.Fatal("declaration HTTP retry changed identity")
	}
	natural["legal_name"] = "Changed intent"
	call(base+"/create", natural, 409, nil)
	natural["legal_name"] = "Synthetic private party"
	natural["operation_id"] = onboardingID(t)
	dup := call(base+"/create", natural, 422, nil)
	details := dup["error"].(map[string]any)["details"].([]any)
	if len(details) != 1 || details[0].(map[string]any)["field"] != "identifier" || details[0].(map[string]any)["code"] != "DUPLICATE" {
		t.Fatal("duplicate error lost bounded field detail")
	}
	call(other+"/create", natural, 201, nil)
	empty := map[string]any{}
	masked := call(base+"/"+id+"/read", empty, 200, nil)
	if masked["identifier"] != "***.***.***-"+sensitive[9:] || masked["status_label"] != "Declarado pela empresa" {
		t.Fatal("default view not explicitly declared/masked")
	}
	reveal := call(base+"/"+id+"/identifier/read", empty, 200, nil)
	if reveal["identifier"] != sensitive || reveal["audit_operation_id"] == "" {
		t.Fatal("fresh management read not audited")
	}

	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_http_reveal_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit outage'; END $$;
 CREATE CONSTRAINT TRIGGER synthetic_http_reveal_failure AFTER INSERT ON org_audit_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.action='org.legal_entity.identifier_read') EXECUTE FUNCTION synthetic_http_reveal_failure()`); err != nil {
		t.Fatal(err)
	}
	call(base+"/"+id+"/identifier/read", empty, 503, nil)
	if _, err := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_http_reveal_failure ON org_audit_events`); err != nil {
		t.Fatal(err)
	}
	foreign := call(other+"/"+id+"/read", empty, 404, nil)
	absent := call(other+"/"+onboardingID(t)+"/read", empty, 404, nil)
	if foreign["error"].(map[string]any)["code"] != absent["error"].(map[string]any)["code"] {
		t.Fatal("foreign declaration oracle")
	}
	company := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "kind": "COMPANY", "legal_name": "Synthetic company", "identifier": "EXEMPL01000170", "establishment": map[string]any{"declared_role": "NOT_INFORMED"}}
	companyResult := call(base+"/create", company, 201, nil)
	companyID := companyResult["legal_entity_id"].(string)
	parent := base + "/" + companyID
	first := companyResult["establishment_id"].(string)
	list := call(parent+"/establishments/list", map[string]any{"limit": 1}, 200, nil)
	if list["items"].([]any)[0].(map[string]any)["declared_role"] != "NOT_INFORMED" {
		t.Fatal("identifier order inferred head office")
	}
	add := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "cnpj": syntheticLegalCNPJ("EXEMPL010002"), "declared_role": "HEAD_OFFICE"}
	second := call(parent+"/establishments/create", add, 201, nil)
	if call(parent+"/establishments/create", add, 201, nil)["establishment_id"] != second["establishment_id"] {
		t.Fatal("establishment replay changed identity")
	}
	changePath := parent + "/establishments/" + first + "/change"
	change := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "declared_role": "BRANCH", "trade_name": "Changed synthetic trade", "municipality_code": "5103403"}
	if call(changePath, change, 200, nil)["revision"] != float64(2) {
		t.Fatal("metadata revision did not advance")
	}
	call(changePath, change, 200, nil)
	change["operation_id"] = onboardingID(t)
	call(changePath, change, 409, nil)
	change["cnpj"] = syntheticLegalCNPJ("EXEMPL010003")
	call(changePath, change, 400, nil)
	delete(change, "cnpj")
	call(base+"/"+id+"/establishments/"+first+"/change", change, 404, nil)
	page := call(base+"/list", map[string]any{"limit": 1}, 200, nil)
	cursor := page["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing legal next cursor")
	}
	call(base+"/list", map[string]any{"limit": 1, "cursor": cursor}, 200, nil)
	call(base+"/list", map[string]any{"limit": 2, "cursor": cursor}, 400, nil)
	call(other+"/list", map[string]any{"limit": 1, "cursor": cursor}, 400, nil)
	establishments := call(parent+"/establishments/list", map[string]any{"limit": 1}, 200, nil)
	ec := establishments["next_cursor"].(string)
	if ec == "" {
		t.Fatal("missing establishment cursor")
	}
	ep := call(parent+"/establishments/list", map[string]any{"limit": 1, "cursor": ec}, 200, nil)
	if len(ep["items"].([]any)) != 1 || ep["next_cursor"] != "" {
		t.Fatal("establishment keyset lost or repeated row")
	}
	call(base+"/"+id+"/establishments/list", map[string]any{"limit": 1, "cursor": ec}, 400, nil)
	withdrawal := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	withdrawn := call(base+"/"+id+"/withdraw", withdrawal, 200, nil)
	if withdrawn["state"] != "REMOVED" {
		t.Fatal("unreferenced private declaration retained")
	}
	replay := call(base+"/"+id+"/withdraw", withdrawal, 200, nil)
	if replay["state"] != "REMOVED" || replay["replayed"] != true {
		t.Fatal("removed HTTP withdrawal cannot replay")
	}
	call(base+"/"+id+"/read", empty, 404, nil)
	withdrawal["operation_id"] = onboardingID(t)
	call(base+"/"+id+"/withdraw", withdrawal, 404, nil)
	// A current flag change blocks new configuration, preserving authorized reads.
	if _, err := fx.flags.Set(t.Context(), scopeFor(t, orgID), cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	company["operation_id"] = onboardingID(t)
	call(base+"/create", company, 403, nil)
	call(parent+"/read", empty, 200, nil)
	// Stale authentication preserves ordinary reads, but cannot reveal CPF or write.
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call(parent+"/read", empty, 200, nil)
	call(parent+"/identifier/read", empty, 401, nil)
	call(base+"/create", company, 401, nil)
	// Revoking the current company proof denies the next private read.
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	call(parent+"/read", empty, 401, nil)
}
