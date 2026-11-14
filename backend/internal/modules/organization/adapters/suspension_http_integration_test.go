//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestSuspensionComposedAdminHTTPProofOriginCSRFReplayAndCompanyGates(t *testing.T) {
	fx := suspensionFixture(t)
	const origin = "https://company.example.invalid"
	admin := &adminhttp.Handler{Service: fx.adminService, Origin: origin, ClientIP: func(*http.Request) (string, error) { return "198.51.100.12", nil }}
	router := chi.NewRouter()
	if e := enterprisesuite.RegisterAdmin(router, enterprisesuite.Options{Enabled: true, Pool: fx.f.Runtime}, admin); e != nil {
		t.Fatal(e)
	}
	path := "/v1/admin/organizations/" + fx.scope.OrgID().String()
	key := onboardingID(t)
	input := map[string]any{"schema_version": 1, "revision": 2, "reason_category": "SECURITY", "reason": "Synthetic lifecycle verification"}
	call := func(route string, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", origin+route, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Admin-CSRF", fx.credentials.CSRFToken)
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(&http.Cookie{Name: "__Host-abv_admin", Value: fx.credentials.Token})
		if edit != nil {
			edit(r)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatalf("operator route status %d want %d", w.Code, want)
		}
		if w.Code >= 400 && (strings.Contains(w.Body.String(), "Synthetic") || strings.Contains(w.Body.String(), fx.scope.OrgID().String())) {
			t.Fatal("private input in refusal")
		}
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("invalid lifecycle response")
		}
		return out
	}
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Del("Cookie") }, func(r *http.Request) { r.Header.Set("Cookie", "__Host-empresas-session="+fx.credentials.Token) }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic"); r.Header.Del("Cookie") }} {
		call(path+"/suspend", 401, edit)
	}
	call(path+"/suspend", 403, func(r *http.Request) { r.Header.Del("X-Admin-CSRF") })
	call(path+"/suspend", 403, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") })
	call(path+"/suspend", 400, func(r *http.Request) { r.Header.Del("Idempotency-Key") })
	for _, field := range []string{"org_id", "account_id", "roles", "state", "effective_at", "cause_resolved"} {
		input[field] = "synthetic"
		call(path+"/suspend", 400, nil)
		delete(input, field)
	}
	input["reason_category"] = "INVALID"
	call(path+"/suspend", 422, nil)
	input["reason_category"] = "SECURITY"
	call(path+"/suspend", 403, func(r *http.Request) { r.Header.Add("Origin", origin) })
	call(path+"/suspend", 403, func(r *http.Request) { r.Header.Add("X-Admin-CSRF", fx.credentials.CSRFToken) })
	call(path+"/suspend", 403, func(r *http.Request) { r.Header.Add("Idempotency-Key", key) })
	call(path+"/suspend", 401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") })
	call(path+"/suspend", 401, func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "__Host-abv_admin", Value: "synthetic"}) })
	call(path+"/suspend?ignored=synthetic", 400, nil)
	first := call(path+"/suspend", 200, nil)
	if first["revision"] != float64(3) || first["state"] != "SUSPENDED" || first["notice_state"] != "PENDING_NOTIFICATION_CORE" {
		t.Fatal("operator acknowledgement")
	}
	replay := call(path+"/suspend", 200, nil)
	if replay["period_id"] != first["period_id"] || replay["effective_at"] != first["effective_at"] {
		t.Fatal("original operator replay")
	}
	// These real company stores enforce current organization state independently
	// of the operator's authority and preserve permitted reads.
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+role); e != nil {
		t.Fatal(e)
	}
	settings := settingsStore(t, fx.legalFixtureState)
	current := settingsRead(t, fx.legalFixtureState, settings, 0)
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, current.Values, "")); e != access.ErrOrgSuspended {
		t.Fatal("suspended company configuration accepted", e)
	}
	key = onboardingID(t)
	input["revision"] = 3
	call(path+"/reactivate", 200, nil)
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, current.Values, "")); e != nil {
		t.Fatal("reactivation failed to restore permitted writes", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE admin_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, fx.session.ID); e != nil {
		t.Fatal(e)
	}
	call(path+"/reactivate", 401, nil)
	for _, options := range []enterprisesuite.Options{{Pool: fx.f.Runtime}, {Enabled: true, Pool: fx.f.Runtime}} {
		r := chi.NewRouter()
		var guard *adminhttp.Handler
		if !options.Enabled {
			guard = admin
		}
		if e := enterprisesuite.RegisterAdmin(r, options, guard); e != nil {
			t.Fatal(e)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", origin+path+"/suspend", nil))
		if w.Code != 404 {
			t.Fatal("operator route enabled without both gates")
		}
	}
}
