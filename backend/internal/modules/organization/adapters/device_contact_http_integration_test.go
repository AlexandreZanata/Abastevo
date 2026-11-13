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

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestDeviceContactActualSignedHTTPNonceAndExactTerminal(t *testing.T) {
	fx := devicesFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); e != nil {
		t.Fatal(e)
	}
	var super, bypass bool
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&super, &bypass); e != nil || super || bypass {
		t.Fatal("contact source is privileged", e)
	}
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	router := chi.NewRouter()
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{71}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); e != nil {
		t.Fatal(e)
	}
	_, file, _, _ := runtime.Caller(0)
	doc, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic exact HTTP contact"))
	if e != nil {
		t.Fatal(e)
	}
	r := deviceContactSelector(t, fx, first.Device)
	path := "/v1/orgs/" + fx.scope.OrgID().String() + "/devices/contact"
	body := map[string]any{"schema_version": 1, "request_id": r.RequestID, "family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken, "membership_id": r.MembershipID, "enrollment_id": r.EnrollmentID}
	dispatch := func(req *http.Request, want int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("contact status%d want%d: %s", w.Code, want, w.Body.String())
		}
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("contact JSON")
		}
		schema := "EnterpriseError"
		if want == 200 {
			schema = "EnterpriseDeviceContactResponse"
		}
		if err := doc.Components.Schemas[schema].Value.VisitJSON(out); err != nil {
			t.Fatal("contact wire violates contract", err)
		}
		for _, secret := range []string{fx.person.auth.Session.AccessToken, fx.person.signer.keyID, fx.person.signer.contributor, fx.person.signer.fingerprint, "Synthetic exact HTTP contact"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("contact leaked private metadata")
			}
		}
		if want != 200 {
			for _, key := range []string{"verdict", "revocation_class", "offline_validity_seconds", "membership_id", "enrollment_id"} {
				if _, ok := out[key]; ok {
					t.Fatal("generic error masqueraded as custody verdict")
				}
			}
		}
		return out
	}
	call := func(body any, want int) map[string]any {
		return dispatch(deviceSignedRequest(t, fx.registrar, fx.person.signer, path, body, time.Time{}), want)
	}
	before := deviceHistorySnapshot(t, fx)
	request := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, body, time.Time{})
	active := dispatch(request, 200)
	for key, want := range map[string]any{"verdict": "ACTIVE", "request_id": r.RequestID, "account_id": fx.person.auth.Account.ID, "api_origin": deviceTestOrigin, "org_id": fx.scope.OrgID().String(), "membership_id": r.MembershipID, "enrollment_id": r.EnrollmentID, "family_id": fx.person.auth.Session.FamilyID, "enrollment_revision": "1", "access_model": "EXISTING_ACCOUNT", "offline_validity_seconds": float64(2592000)} {
		if active[key] != want {
			t.Fatal("contact identity/policy not bound", key)
		}
	}
	raw, _ := json.Marshal(body)
	request.Body = io.NopCloser(bytes.NewReader(raw))
	request.ContentLength = int64(len(raw))
	dispatch(request, 401)
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Signature") }, func(r *http.Request) { r.Header.Set("Cookie", "synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Host = "foreign.example.invalid" }} {
		req := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, body, time.Time{})
		edit(req)
		dispatch(req, 401)
	}
	for key, value := range map[string]any{"offline_validity_seconds": 2592000, "account_id": fx.person.auth.Account.ID, "managed_unlock_seconds": 300} {
		bad := map[string]any{}
		for k, v := range body {
			bad[k] = v
		}
		bad[key] = value
		call(bad, 400)
	}
	for key, value := range map[string]any{"membership_id": fx.member, "enrollment_id": onboardingID(t)} {
		bad := map[string]any{}
		for k, v := range body {
			bad[k] = v
		}
		bad[key] = value
		call(bad, 404)
	}
	if deviceHistorySnapshot(t, fx) != before {
		t.Fatal("contact or refusal mutated source")
	}
	if _, e := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, deviceRevokeCommand(t, fx.scope, first.Device, d.LostOrCompromised)); e != nil {
		t.Fatal(e)
	}
	erased := call(body, 200)
	if erased["verdict"] != "ERASE" || erased["revocation_class"] != "LOST_OR_COMPROMISED" || erased["enrollment_revision"] != "2" {
		t.Fatal("exact lost verdict")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, fx.person.auth.Session.FamilyID); e != nil {
		t.Fatal(e)
	}
	call(body, 401) // Revoked personal proof is a generic refusal, never a tombstone.
}
