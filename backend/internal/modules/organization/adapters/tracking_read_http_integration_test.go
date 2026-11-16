//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// trackReadGrants are the runtime grants of the composed track read.
func trackReadGrants(t *testing.T, x trackingState) {
	t.Helper()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	tripReviewGrants(t, x.f)
}

// trackRouter is the actual composed enterprise router over the fixture.
func trackRouter(t *testing.T, x trackingState) chi.Router {
	t.Helper()
	trackReadGrants(t, x)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{41}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual track read composition failed", err)
	}
	return router
}

// trackLogin opens a company session for an existing device person.
func trackLogin(t *testing.T, p devicePerson, router chi.Router) matrixPerson {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"family_id": p.auth.Session.FamilyID, "access_token": p.auth.Session.AccessToken})
	if err != nil {
		t.Fatal(err)
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
		t.Fatal("company login of the device person failed", w.Code)
	}
	return matrixPerson{account: p.auth.Account.ID, member: p.member, employee: p.employee, cookie: w.Result().Cookies()[0], csrf: result.CSRF}
}

// trackAccesses counts LOCATION_TRACK records of a member on a trip.
func (x trackingState) trackAccesses(t *testing.T, member, trip string) (issued, served int) {
	t.Helper()
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE s.stage='ISSUED'),count(*) FILTER(WHERE s.stage='SERVED') FROM org_sensitive_access_events s
 JOIN org_audit_events a ON(a.org_id,a.operation_id)=(s.org_id,s.operation_id) WHERE s.org_id=$1 AND s.membership_id=$2 AND s.category='LOCATION_TRACK' AND a.target_type='trip' AND a.target_id=$3`,
		x.scope.OrgID().String(), member, trip).Scan(&issued, &served); err != nil {
		t.Fatal(err)
	}
	return issued, served
}

// recordedTrack starts tracking on the fixture trip and stores two batches.
func (x trackingState) recordedTrack(t *testing.T) string {
	t.Helper()
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	for seq, seconds := range [][]int{{1, 11, 21}, {31, 41}} {
		op := x.batchOp(t, d, session, seq+1, seconds, nil)
		ack, err := x.process(t, d, op)
		x.accepted(t, op, ack, err, int64(seq+1), trackingCounts{segments: 1})
	}
	return session
}

// BUC-EMP-220/221, B-BR-EMP-435..437: recorded tracks through the actual
// composed company route under gps.track.read only, every page recorded.
func TestTrackReadServesOnlyUnderTrackPermissionAndRecordsEveryPage(t *testing.T) {
	x := trackingFixture(t)
	session := x.recordedTrack(t)
	router := trackRouter(t, x)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	path := func(trip string) string { return "/v1/orgs/" + org + "/trips/" + trip + "/track/read" }
	call := func(actor matrixPerson, p string, body any, want int, code, key string) map[string]any {
		t.Helper()
		v := matrixCall(t, fx, router, actor, p, body, want, code, key, nil)
		if want != 200 {
			raw, _ := json.Marshal(v)
			if strings.Contains(string(raw), "lat_e7") || strings.Contains(string(raw), "100000") {
				t.Fatal("a refused read carries a position")
			}
		}
		return v
	}
	person := func(role, scope string) matrixPerson {
		p := matrixNewPerson(t, fx, router)
		matrixAssign(t, fx, p, role, scope, "ACTIVE")
		return p
	}
	owner, auditor, finance := person("OWNER", "ORG"), person("AUDITOR", "ORG"), person("FINANCE", "ORG")
	unitSupervisor := person("SUPERVISOR", "UNIT")

	read := call(owner, path(x.trip), map[string]any{}, 200, "", "")
	keys := make([]string, 0, len(read))
	for k := range read {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"data_mode", "next_cursor", "segments", "session", "trip_id"}) || read["trip_id"] != x.trip || read["next_cursor"] != "" || read["data_mode"] != "SYNTHETIC" {
		t.Fatal("track page shape", keys)
	}
	s := read["session"].(map[string]any)
	transitions := s["transitions"].([]any)
	segments := read["segments"].([]any)
	if s["session_id"] != session || s["state"] != "RECORDING" || s["policy"] != "EXPECTED" || len(transitions) != 2 || len(segments) != 2 {
		t.Fatal("track page session", s, len(segments))
	}
	first := segments[0].(map[string]any)
	fixes := first["fixes"].([]any)
	if first["sequence"] != float64(1) || first["source"] != "PHONE" || first["stored_count"] != float64(3) || len(fixes) != 3 {
		t.Fatal("track page segment", first)
	}
	if f := fixes[2].(map[string]any); f["lat_e7"] != float64(-99999997) || f["lon_e7"] != float64(-500000003) || f["coarse"] != false || f["accuracy_m"] != float64(8) || f["speed_cm_s"] != float64(1389) {
		t.Fatal("track fix", f)
	}
	if issued, served := x.trackAccesses(t, owner.member, x.trip); issued != 1 || served != 1 {
		t.Fatal("owner read not recorded once per stage", issued, served)
	}
	// AUDITOR: recorded tracks (RECORDED_TRACKS_ONLY), recorded per page.
	call(auditor, path(x.trip), map[string]any{}, 200, "", "")
	if issued, served := x.trackAccesses(t, auditor.member, x.trip); issued != 1 || served != 1 {
		t.Fatal("auditor read not recorded", issued, served)
	}
	// A UNIT-scoped supervisor of the trip's stored unit is covered.
	call(unitSupervisor, path(x.trip), map[string]any{}, 200, "", "")
	// Pages: one segment per page, then the rest; the cursor is bound.
	page := call(owner, path(x.trip), map[string]any{"limit": 1}, 200, "", "")
	next, _ := page["next_cursor"].(string)
	if len(page["segments"].([]any)) != 1 || next == "" {
		t.Fatal("first page", page["next_cursor"])
	}
	rest := call(owner, path(x.trip), map[string]any{"limit": 1, "cursor": next}, 200, "", "")
	if segs := rest["segments"].([]any); len(segs) != 1 || segs[0].(map[string]any)["sequence"] != float64(2) || rest["next_cursor"] != "" {
		t.Fatal("second page", rest)
	}
	call(auditor, path(x.trip), map[string]any{"limit": 1, "cursor": next}, 400, "MALFORMED", "")
	call(owner, path(x.trip), map[string]any{"limit": 21}, 400, "MALFORMED", "")
	call(owner, path(x.trip), map[string]any{"limit": 1, "trip_driver": owner.member}, 400, "MALFORMED", "")
	if issued, served := x.trackAccesses(t, owner.member, x.trip); issued != 3 || served != 3 {
		t.Fatal("every page is recorded", issued, served)
	}

	t.Run("trip-read-holder-without-track-permission-forbidden", func(t *testing.T) {
		call(finance, path(x.trip), map[string]any{}, 403, "FORBIDDEN", "gps.track.read")
	})
	t.Run("driver-reads-own-trips-only", func(t *testing.T) {
		driver := x.driver(t)
		own := x.otherTrip(t, driver)
		login := trackLogin(t, driver.person, router)
		if v := call(login, path(own), map[string]any{}, 200, "", ""); v["session"] != nil || len(v["segments"].([]any)) != 0 {
			t.Fatal("an untracked own trip is an empty page", v)
		}
		if issued, served := x.trackAccesses(t, driver.person.member, own); issued != 1 || served != 1 {
			t.Fatal("own read not recorded", issued, served)
		}
		call(login, path(x.trip), map[string]any{}, 403, "FORBIDDEN", "gps.track.read")
	})
	t.Run("another-unit-supervisor-forbidden", func(t *testing.T) {
		unit := onboardingID(t)
		if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic second track unit','TRACK2','UTC','ACTIVE')`, org, unit); err != nil {
			t.Fatal(err)
		}
		p := matrixNewPerson(t, fx, router)
		if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'SUPERVISOR','UNIT',$4,clock_timestamp(),$5,'Synthetic other unit supervisor')`,
			org, onboardingID(t), p.member, unit, fx.member); err != nil {
			t.Fatal(err)
		}
		call(p, path(x.trip), map[string]any{}, 403, "FORBIDDEN", "gps.track.read")
	})
	t.Run("staff-status-alone-grants-nothing", func(t *testing.T) {
		staff := person("FINANCE", "ORG")
		if _, err := x.f.Admin.Exec(t.Context(), `INSERT INTO staff_assignments(id,account_id,role,state,start_at,reason) VALUES($1,$2,'support_agent','ACTIVE',clock_timestamp()-interval '1 hour','Synthetic support staff without a track grant')`, onboardingID(t), staff.account); err != nil {
			t.Fatal(err)
		}
		call(staff, path(x.trip), map[string]any{}, 403, "FORBIDDEN", "gps.track.read")
		matrixAssign(t, fx, staff, "FINANCE", "ORG", "NONE")
		if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, staff.member); err != nil {
			t.Fatal(err)
		}
		call(staff, path(x.trip), map[string]any{}, 404, "NOT_FOUND", "")
	})
	t.Run("foreign-or-unknown-trip-not-found", func(t *testing.T) {
		call(owner, path(onboardingID(t)), map[string]any{}, 404, "NOT_FOUND", "")
		call(owner, "/v1/orgs/"+onboardingID(t)+"/trips/"+x.trip+"/track/read", map[string]any{}, 404, "NOT_FOUND", "")
	})
	t.Run("no-live-grant-for-auditor", func(t *testing.T) {
		rule, err := access.SystemCatalog().Lookup(access.Auditor, "gps.location.read_live")
		if err != nil || rule.Coverage() != access.Denied || audit.LocationLive.Permission() != "gps.location.read_live" {
			t.Fatal("an auditor holds a live location grant", err)
		}
	})
	if issued, served := x.trackAccesses(t, finance.member, x.trip); issued != 0 || served != 0 {
		t.Fatal("a refused read was recorded as served", issued, served)
	}
}

// B-BR-EMP-436: a failed access record serves nothing.
func TestTrackReadServesNothingWhenTheAccessLogFails(t *testing.T) {
	x := trackingFixture(t)
	x.recordedTrack(t)
	router := trackRouter(t, x)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
		t.Fatal(err)
	}
	v := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/trips/"+x.trip+"/track/read", map[string]any{}, 503, "UNAVAILABLE", "", nil)
	if raw, _ := json.Marshal(v); strings.Contains(string(raw), "lat_e7") || strings.Contains(string(raw), "segments") {
		t.Fatal("bytes served without an access record")
	}
	if issued, served := x.trackAccesses(t, owner.member, x.trip); issued != 0 || served != 0 {
		t.Fatal("a failed access left a record", issued, served)
	}
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/trips/"+x.trip+"/track/read", map[string]any{}, 200, "", "", nil)
	if issued, served := x.trackAccesses(t, owner.member, x.trip); issued != 1 || served != 1 {
		t.Fatal("the recovered read was not recorded", issued, served)
	}
}
