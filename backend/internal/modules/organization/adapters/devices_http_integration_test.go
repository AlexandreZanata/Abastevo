//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestOrganizationDevicesActualSignedAndCompanyHTTP(t *testing.T) {
	fx := devicesFixture(t)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	manager := deviceNewPerson(t, fx)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp(),$4,'Synthetic device manager')`, fx.scope.OrgID().String(), onboardingID(t), manager.member, fx.member); e != nil {
		t.Fatal(e)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test location")
	}
	document, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(source), "../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	router := chi.NewRouter()
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{41}, 32), Pool: fx.f.Runtime, Accounts: fx.accounts}); e != nil {
		t.Fatal(e)
	}
	var cookie *http.Cookie
	csrf := ""
	dispatch := func(r *http.Request, want int, schema string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("device HTTP %s got%d want%d", r.URL.Path, w.Code, want)
		}
		if want == 429 && w.Header().Get("Retry-After") == "" {
			t.Fatal("device quota omitted retry delay")
		}
		var value map[string]any
		if json.Unmarshal(w.Body.Bytes(), &value) != nil {
			t.Fatal("device HTTP JSON")
		}
		if schema != "" {
			ref := document.Components.Schemas[schema]
			if ref == nil || ref.Value.VisitJSON(value) != nil {
				t.Fatal("actual device response violates contract", schema)
			}
		}
		for _, private := range []string{fx.person.auth.Session.FamilyID, fx.person.auth.Session.AccessToken, fx.person.signer.fingerprint, fx.person.signer.contributor, fx.person.signer.keyID} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("device API disclosed personal proof/inventory")
			}
		}
		if r.URL.Path == "/v1/org-access/session/login" && want == 200 {
			if len(w.Result().Cookies()) != 1 {
				t.Fatal("company cookie")
			}
			cookie = w.Result().Cookies()[0]
			csrf = value["csrf_token"].(string)
		}
		return value
	}
	cookieCall := func(path string, body any, want int, schema string, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", deviceTestOrigin)
		if cookie != nil {
			r.AddCookie(cookie)
			r.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(r)
		}
		return dispatch(r, want, schema)
	}
	login := func(p devicePerson) {
		t.Helper()
		cookieCall("/v1/org-access/session/login", map[string]any{"family_id": p.auth.Session.FamilyID, "access_token": p.auth.Session.AccessToken}, 200, "", nil)
	}
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org
	memberBase := base + "/memberships/" + fx.person.member + "/devices"
	check := map[string]any{"family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken}
	enroll := map[string]any{"family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken, "schema_version": 1, "operation_id": onboardingID(t), "label": "Synthetic signed organization device"}
	signCall := func(signer deviceLiveSigner, path string, body any, want int, schema string) map[string]any {
		t.Helper()
		return dispatch(deviceSignedRequest(t, fx.registrar, signer, path, body, time.Time{}), want, schema)
	}
	signCall(fx.person.signer, base+"/devices/check", check, 404, "EnterpriseError")
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Signature") }, func(r *http.Request) { r.Header.Set("Cookie", "__Host-empresas-session=synthetic") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }} {
		r := deviceSignedRequest(t, fx.registrar, fx.person.signer, base+"/devices/enroll", enroll, time.Time{})
		edit(r)
		dispatch(r, 401, "EnterpriseError")
	}
	for _, extra := range []map[string]any{{"label": nil}, {"key_id": fx.person.signer.keyID}, {"proof_confirmed": true}, {"device_limit": 5}} {
		body := map[string]any{}
		for k, v := range enroll {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		signCall(fx.person.signer, base+"/devices/enroll", body, 400, "EnterpriseError")
	}
	wrong := map[string]any{}
	for k, v := range enroll {
		wrong[k] = v
	}
	wrong["family_id"], wrong["access_token"] = manager.auth.Session.FamilyID, manager.auth.Session.AccessToken
	signCall(fx.person.signer, base+"/devices/enroll", wrong, 401, "EnterpriseError")
	request := deviceSignedRequest(t, fx.registrar, fx.person.signer, base+"/devices/enroll", enroll, time.Time{})
	first := dispatch(request, 200, "EnterpriseDeviceResult")
	id := first["device"].(map[string]any)["id"].(string)
	raw, _ := json.Marshal(enroll)
	request.Body = io.NopCloser(bytes.NewReader(raw))
	request.ContentLength = int64(len(raw))
	dispatch(request, 401, "EnterpriseError")
	if replay := signCall(fx.person.signer, base+"/devices/enroll", enroll, 200, "EnterpriseDeviceResult"); replay["replayed"] != true || replay["device"].(map[string]any)["enrolled_at"] != first["device"].(map[string]any)["enrolled_at"] {
		t.Fatal("original signed enrollment retry")
	}
	signCall(fx.person.signer, base+"/devices/check", check, 200, "EnterpriseDeviceView")
	secondSigner, _ := deviceAddSigner(t, fx, fx.person)
	secondBody := map[string]any{}
	for k, v := range enroll {
		secondBody[k] = v
	}
	secondBody["operation_id"] = onboardingID(t)
	secondBody["label"] = "Synthetic second organization device"
	second := signCall(secondSigner, base+"/devices/enroll", secondBody, 200, "EnterpriseDeviceResult")
	secondID := second["device"].(map[string]any)["id"].(string)
	login(fx.person)
	personCookie, personCSRF := cookie, csrf
	if page := cookieCall(memberBase+"/self/list", map[string]any{"limit": 1}, 200, "EnterpriseDevicePage", nil); len(page["devices"].([]any)) != 1 || page["next_cursor"] == "" {
		t.Fatal("own scoped device page")
	}
	cookieCall(memberBase+"/list", map[string]any{}, 403, "EnterpriseError", nil)
	login(manager)
	managerCookie, managerCSRF := cookie, csrf
	cookieCall(memberBase+"/list", map[string]any{}, 200, "EnterpriseDevicePage", nil)
	cookieCall(memberBase+"/self/list", map[string]any{}, 404, "EnterpriseError", nil)
	revoke := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "revocation_class": "LOST_OR_COMPROMISED", "reason": "  Synthetic lost organization device  "}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Del("X-CSRF") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }} {
		cookieCall(memberBase+"/"+id+"/revoke", revoke, 401, "EnterpriseError", edit)
	}
	for _, extra := range []map[string]any{{"revision": nil}, {"revocation_class": nil}, {"reason": nil}, {"family_id": fx.person.auth.Session.FamilyID}} {
		body := map[string]any{}
		for k, v := range revoke {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		cookieCall(memberBase+"/"+id+"/revoke", body, 400, "EnterpriseError", nil)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	_, otherMember := teamDriver(t, other, fx.person.auth.Account.ID)
	if _, _, e := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); e != nil {
		t.Fatal(e)
	}
	otherBase := "/v1/orgs/" + other.scope.OrgID().String()
	otherBody := map[string]any{}
	for k, v := range enroll {
		otherBody[k] = v
	}
	otherBody["operation_id"] = onboardingID(t)
	signCall(fx.person.signer, otherBase+"/devices/enroll", otherBody, 200, "EnterpriseDeviceResult")
	cookie, csrf = personCookie, personCSRF
	cookieCall(base+"/memberships/"+otherMember+"/devices/self/list", map[string]any{}, 404, "EnterpriseError", nil)
	cookie, csrf = managerCookie, managerCSRF
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_device_http_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic private device receipt failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_device_http_failure AFTER INSERT ON org_device_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_device_http_failure()`); e != nil {
		t.Fatal(e)
	}
	cookieCall(memberBase+"/"+id+"/revoke", revoke, 503, "EnterpriseError", nil)
	signCall(fx.person.signer, base+"/devices/check", check, 200, "EnterpriseDeviceView")
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_device_http_failure ON org_device_commands`); e != nil {
		t.Fatal(e)
	}
	revoked := cookieCall(memberBase+"/"+id+"/revoke", revoke, 200, "EnterpriseDeviceResult", nil)
	if revoked["device"].(map[string]any)["state"] != "REVOKED" || revoked["reason"] != revoke["reason"] {
		t.Fatal("HTTP revocation result")
	}
	if again := cookieCall(memberBase+"/"+id+"/revoke", revoke, 200, "EnterpriseDeviceResult", nil); again["replayed"] != true || again["device"].(map[string]any)["revoked_at"] != revoked["device"].(map[string]any)["revoked_at"] {
		t.Fatal("immutable HTTP revocation receipt")
	}
	if denied := signCall(fx.person.signer, base+"/devices/check", check, 403, "EnterpriseError"); denied["error"].(map[string]any)["code"] != "DEVICE_REVOKED" {
		t.Fatal("next server request revocation code")
	}
	signCall(fx.person.signer, base+"/devices/enroll", enroll, 403, "EnterpriseError")
	signCall(fx.person.signer, otherBase+"/devices/check", check, 200, "EnterpriseDeviceView")
	cookie, csrf = personCookie, personCSRF
	ownRevoke := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "revocation_class": "ACCESS_ENDED", "reason": "Synthetic no longer used"}
	cookieCall(memberBase+"/self/"+secondID+"/revoke", ownRevoke, 200, "EnterpriseDeviceResult", nil)
	signCall(secondSigner, base+"/devices/check", check, 403, "EnterpriseError")
	login(fx.person)
	cookieCall("/v1/org-access/session/read", map[string]any{}, 200, "", nil)
	thirdSigner, _ := deviceAddSigner(t, fx, fx.person)
	third := map[string]any{}
	for k, v := range enroll {
		third[k] = v
	}
	third["operation_id"] = onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE identity_rate_windows SET count=200 WHERE subject_digest LIKE 'orgquota:v1:%'`); e != nil {
		t.Fatal(e)
	}
	signCall(thirdSigner, base+"/devices/enroll", third, 429, "EnterpriseError")
	cookieCall(memberBase+"/self/list", map[string]any{}, 200, "EnterpriseDevicePage", nil)
}
