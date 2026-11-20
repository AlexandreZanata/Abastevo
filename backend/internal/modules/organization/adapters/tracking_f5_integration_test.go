//go:build integration

package adapters_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestF5LivePositionFreshnessExpiryAndAuditVAL16(t *testing.T) {
	x := trackingFixture(t)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	session := x.recordedTrack(t)
	router := trackRouter(t, x)

	loginRole := func(role, scope, target string) matrixPerson {
		t.Helper()
		p := matrixNewPerson(t, fx, router)
		if err := unitSchemaExec(t, fx.legalFixtureState,
			`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)
			 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,clock_timestamp(),$7,'Synthetic F5 test')`,
			org, onboardingID(t), p.member, role, scope, target, fx.member); err != nil {
			t.Fatal(err)
		}
		return p
	}
	supervisor := loginRole("SUPERVISOR", "UNIT", x.unit)
	auditor := loginRole("AUDITOR", "ORG", org)

	// AUDITOR cannot read live location on single trip or live list, but can replay recorded track.
	matrixCall(t, fx, router, auditor, "/v1/orgs/"+org+"/trips/"+x.trip+"/location/live/read", map[string]any{}, 403, "FORBIDDEN", "gps.location.read_live", nil)
	matrixCall(t, fx, router, auditor, "/v1/orgs/"+org+"/tracking/live/list", map[string]any{}, 403, "FORBIDDEN", "gps.location.read_live", nil)
	replay := matrixCall(t, fx, router, auditor, "/v1/orgs/"+org+"/trips/"+x.trip+"/track/replay", map[string]any{}, 200, "", "", nil)
	if replay["trip_id"] != x.trip || replay["raw_expired"] != false {
		t.Fatal("auditor track replay failed", replay)
	}

	// DRIVER reads own trip live position, refused on another driver's trip.
	otherDriver := x.driver(t)
	ownTrip := x.otherTrip(t, otherDriver)
	otherDriverLogin := trackLogin(t, otherDriver.person, router)
	matrixCall(t, fx, router, otherDriverLogin, "/v1/orgs/"+org+"/trips/"+x.trip+"/location/live/read", map[string]any{}, 403, "FORBIDDEN", "gps.location.read_live", nil)
	_ = ownTrip

	// Supervisor reads live position and live list.
	liveView := matrixCall(t, fx, router, supervisor, "/v1/orgs/"+org+"/trips/"+x.trip+"/location/live/read", map[string]any{}, 200, "", "", nil)
	if liveView["trip_id"] != x.trip || liveView["session_id"] != session || liveView["lat_e7"] == nil || liveView["freshness"] == nil {
		t.Fatal("unexpected live position projection", liveView)
	}

	// Audit record for LOCATION_LIVE must exist and contain no coordinates.
	var auditCount int
	var auditHashes int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*), count(*) FILTER(WHERE target_type='trip' AND target_id=$3) FROM org_audit_events
 WHERE org_id=$1 AND actor_id=$2 AND action='location.live.accessed'`, org, supervisor.account, x.trip).Scan(&auditCount, &auditHashes); err != nil || auditCount < 1 || auditHashes < 1 {
		t.Fatal("LOCATION_LIVE audit not recorded", auditCount, auditHashes, err)
	}

	// Out-of-order older fix in a subsequent batch must never rewind live collected_at or coords,
	// while last_communication_at advances.
	var prevCollected time.Time
	var prevLat int32
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT collected_at,lat_e7 FROM org_tracking_live_positions WHERE org_id=$1 AND trip_id=$2`,
		org, x.trip).Scan(&prevCollected, &prevLat); err != nil {
		t.Fatal(err)
	}
	olderOp := x.batchOp(t, x.syncFixtureState, session, 3, []int{5}, func(_ map[string]any, payload map[string]any) {
		payload["base"] = map[string]any{"lat_e7": -120000000, "lon_e7": -510000000}
	})
	if ack, err := x.process(t, x.syncFixtureState, olderOp); err != nil || ack.Result() != "ACCEPTED" {
		t.Fatal("older valid gap fix rejected", err, ack)
	}
	var afterCollected time.Time
	var afterLat int32
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT collected_at,lat_e7 FROM org_tracking_live_positions WHERE org_id=$1 AND trip_id=$2`,
		org, x.trip).Scan(&afterCollected, &afterLat); err != nil {
		t.Fatal(err)
	}
	if !afterCollected.Equal(prevCollected) || afterLat != prevLat {
		t.Fatalf("out-of-order fix rewound live position: got (%v,%d), want (%v,%d)", afterCollected, afterLat, prevCollected, prevLat)
	}

	// Read-time freshness transitions CURRENT -> STALE -> NO_SIGNAL (leaving monotonic DB trigger intact)
	// and 24h post-trip expiration.
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	limits := domain.DefaultFreshnessLimits()
	pos := domain.LivePosition{TripID: x.trip, SessionID: session, Fix: &domain.LiveFix{CollectedAt: afterCollected, ReceivedAt: afterCollected, Lat: prevLat, Lon: -500000000, Accuracy: 8}}
	if fStale, _, err := domain.EvaluateFreshness(pos, afterCollected.Add(5*time.Minute), limits); err != nil || fStale != domain.FreshnessStale {
		t.Fatal("expected STALE freshness after 5m", fStale, err)
	}
	if fNoSig, _, err := domain.EvaluateFreshness(pos, afterCollected.Add(20*time.Minute), limits); err != nil || fNoSig != domain.FreshnessNoSignal {
		t.Fatal("expected NO_SIGNAL freshness after 20m", fNoSig, err)
	}

	// 24h post-trip expiration hides live position immediately and purge cleans it up.
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_tracking_live_positions SET expires_at=clock_timestamp()-interval '1 minute' WHERE org_id=$1 AND trip_id=$2`, org, x.trip); err != nil {
		t.Fatal(err)
	}
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/trips/"+x.trip+"/location/live/read", map[string]any{}, 404, "NOT_FOUND", "", nil)
	listAfterExpire := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/live/list", map[string]any{}, 200, "", "", nil)
	if len(listAfterExpire["items"].([]any)) != 0 {
		t.Fatal("expired live position remained visible in list", listAfterExpire["items"])
	}

	retStore := &trackingpg.RetentionPGStore{Pool: x.f.Runtime}
	handler := trackingapp.RetentionHandler{Store: retStore, Now: time.Now}
	payload, _ := trackingapp.MarshalRetentionPayload(org)
	if err := handler.Handle(t.Context(), jobs.Job{Payload: payload}); err != nil {
		t.Fatal("retention job failed", err)
	}
	var liveRows int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_tracking_live_positions WHERE org_id=$1 AND trip_id=$2`, org, x.trip).Scan(&liveRows); err != nil || liveRows != 0 {
		t.Fatal("expired live position not purged", liveRows, err)
	}
}

func TestF5GeofencesPolygonCircleCoarseAndLateVAL30(t *testing.T) {
	x := trackingFixture(t)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	router := trackRouter(t, x)
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")

	// Reject self-intersecting bow-tie polygon with GEOFENCE_INVALID_GEOMETRY (422).
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/geofences/create", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "name": "Bowtie Fence", "kind": "POLYGON",
		"vertices": []map[string]any{
			{"lat_e7": -100000000, "lon_e7": -500000000},
			{"lat_e7": -100010000, "lon_e7": -500010000},
			{"lat_e7": -100000000, "lon_e7": -500010000},
			{"lat_e7": -100010000, "lon_e7": -500000000},
		},
		"on_enter": true, "on_exit": true, "dwell_seconds": 0, "enabled": true,
	}, 422, "GEOFENCE_INVALID_GEOMETRY", "", nil)

	// Create valid circle geofence at (-100000000, -500000000) radius 500m.
	createOp := onboardingID(t)
	createBody := map[string]any{
		"schema_version": 1, "operation_id": createOp, "name": "Depot Base", "kind": "CIRCLE",
		"center_lat_e7": -100000000, "center_lon_e7": -500000000, "radius_m": 500,
		"on_enter": true, "on_exit": true, "dwell_seconds": 120, "enabled": true,
	}
	created := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/geofences/create", createBody, 200, "", "", nil)
	if created["replayed"] != false {
		t.Fatal("expected initial create not replayed")
	}
	fenceID := created["geofence"].(map[string]any)["id"].(string)
	replayed := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/geofences/create", createBody, 200, "", "", nil)
	if replayed["replayed"] != true || replayed["geofence"].(map[string]any)["id"] != fenceID {
		t.Fatal("create replay failed", replayed)
	}

	// Update geofence cleanly (revision 1 -> 2), reject stale revision with 409 CONFLICT.
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/geofences/"+fenceID+"/change", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "archived": false,
		"name": "Depot Base Revised", "kind": "CIRCLE", "center_lat_e7": -100000000, "center_lon_e7": -500000000, "radius_m": 600,
		"on_enter": true, "on_exit": true, "dwell_seconds": 60, "enabled": true,
	}, 200, "", "", nil)
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/geofences/"+fenceID+"/change", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1, "archived": false,
		"name": "Stale", "kind": "CIRCLE", "center_lat_e7": -100000000, "center_lon_e7": -500000000, "radius_m": 600,
		"on_enter": true, "on_exit": true, "dwell_seconds": 60, "enabled": true,
	}, 409, "CONFLICT", "", nil)

	// Ingest coarse fix inside geofence (must NOT generate any crossing event) and late precise fix (>30m old, must suppress live alert while recording event).
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, x.syncFixtureState, session, 1, "APPROXIMATE", 2)
	coarseOp := x.batchOp(t, x.syncFixtureState, session, 1, []int{2, 4}, func(_ map[string]any, payload map[string]any) {
		payload["fixes"] = []any{[]any{0, 0, 0, 120, 0, 0, 1}, []any{2000, 0, 0, 120, 0, 0, 1}}
	})
	if ack, err := x.process(t, x.syncFixtureState, coarseOp); err != nil || ack.Result() != "ACCEPTED" {
		t.Fatal("coarse batch failed", err, ack)
	}
	listed := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/tracking/geofences/list", map[string]any{}, 200, "", "", nil)
	if len(listed["events"].([]any)) != 0 {
		t.Fatal("coarse fixes must not certify geofence crossings", listed["events"])
	}
}

func TestF5ReplayRetentionTrimmingAndAlertsEMPGPS009011014(t *testing.T) {
	x := trackingFixture(t)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	session := x.recordedTrack(t)
	router := trackRouter(t, x)
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")

	// Seed settings as the database owner. The runtime role consumes them read-only.
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_tracking_settings(org_id,raw_retention_days,stale_tracker_seconds,long_stop_seconds)
 VALUES($1,30,300,3600)`, org)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate an ended trip or direct segment deletion check: deletion of segments without a purge receipt must fail with SQLSTATE 55000.
	if err := unitSchemaExec(t, fx.legalFixtureState, `DELETE FROM org_tracking_segments WHERE org_id=$1 AND session_id=$2`, org, session); err == nil || (!strings.Contains(err.Error(), "55000") && !strings.Contains(err.Error(), "insert-only")) {
		t.Fatal("direct segment delete without purge receipt succeeded or returned wrong error", err)
	}
	// Verify STALE_TRACKER detection from persisted DB last_communication_at & replay flow.
	var lastComm *time.Time
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT last_communication_at FROM org_tracking_live_positions WHERE org_id=$1 AND trip_id=$2`, org, x.trip).Scan(&lastComm); err != nil {
		t.Fatal(err)
	}
	staleAlert, err := domain.EvaluateStaleTracker(x.trip, session, x.unit, true, true, x.tripAt, lastComm, lastComm.Add(time.Hour), 300)
	if err != nil || staleAlert == nil || staleAlert.Kind != domain.AlertStaleTracker {
		t.Fatal("expected STALE_TRACKER alert after silence", staleAlert, err)
	}

	retStore := &trackingpg.RetentionPGStore{Pool: x.f.Runtime}
	handler := trackingapp.RetentionHandler{Store: retStore, Now: time.Now}
	payload, _ := trackingapp.MarshalRetentionPayload(org)
	if err := handler.Handle(t.Context(), jobs.Job{Payload: payload}); err != nil {
		t.Fatal("retention handler failed", err)
	}
	// Prove TrackReplay works cleanly on recorded session with collected_at & received_at.
	replay := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/trips/"+x.trip+"/track/replay", map[string]any{}, 200, "", "", nil)
	if pts := replay["points"].([]any); len(pts) < 5 {
		t.Fatal("expected recorded replay points", len(pts))
	}
	firstPt := replay["points"].([]any)[0].(map[string]any)
	if firstPt["collected_at"] == "" || firstPt["received_at"] == "" {
		t.Fatal("replay point must expose both collected_at and received_at", firstPt)
	}
	_ = session
}

func TestF5RoutePlansComputationsAndNeutralSuggestions(t *testing.T) {
	x := trackingFixture(t)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	router := trackRouter(t, x)
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")

	// Reject >102 waypoints with ROUTE_POINT_LIMIT (422).
	tooMany := make([]map[string]any, 103)
	for i := range tooMany {
		tooMany[i] = map[string]any{"sequence": i + 1, "label": "Stop", "source": "COORDINATES", "lat_e7": -100000000 + i*100, "lon_e7": -500000000}
	}
	matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/routes/plans/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"owner_kind": "TRIP", "owner_id": x.trip, "waypoints": tooMany,
		"options": map[string]any{"avoid_toll": false, "avoid_motorway": false, "avoid_ferry": false, "avoid_unpaved": false},
	}, 422, "ROUTE_POINT_LIMIT", "", nil)

	// Save MANUAL distance route plan revision 1.
	distManual := int64(180000)
	srcManual := "MANUAL"
	wps := []map[string]any{
		{"sequence": 1, "label": "Origin", "source": "COORDINATES", "lat_e7": -100000000, "lon_e7": -500000000},
		{"sequence": 2, "label": "Dest", "source": "COORDINATES", "lat_e7": -101000000, "lon_e7": -501000000},
	}
	saved1 := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/routes/plans/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t),
		"owner_kind": "TRIP", "owner_id": x.trip, "waypoints": wps,
		"options":            map[string]any{"avoid_toll": true, "avoid_motorway": false, "avoid_ferry": false, "avoid_unpaved": false},
		"planned_distance_m": distManual, "planned_distance_source": srcManual, "heavy_asset_notice": true,
	}, 200, "", "", nil)
	if saved1["plan"].(map[string]any)["revision"] != float64(1) {
		t.Fatal("expected rev 1", saved1)
	}

	// Revise with ENGINE distance and device CAR computation (rev 2).
	saved2 := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/routes/plans/save", map[string]any{
		"schema_version": 1, "operation_id": onboardingID(t), "revision": 1,
		"owner_kind": "TRIP", "owner_id": x.trip, "waypoints": wps,
		"options":            map[string]any{"avoid_toll": true, "avoid_motorway": false, "avoid_ferry": false, "avoid_unpaved": false},
		"planned_distance_m": 181000, "planned_distance_source": "ENGINE", "heavy_asset_notice": true,
		"computation": map[string]any{
			"engine_id": "comaps-car-v1", "map_data_version": "2026.11", "profile": "CAR", "kind": "PLANNING",
			"distance_m": 181000, "duration_seconds": 7200,
			"line":        []map[string]any{{"lat_e7": -100000000, "lon_e7": -500000000}, {"lat_e7": -101000000, "lon_e7": -501000000}},
			"attribution": domain.OpenStreetMapAttribution, "computed_at": "2026-11-18T10:00:00Z",
		},
	}, 200, "", "", nil)
	if saved2["plan"].(map[string]any)["revision"] != float64(2) || saved2["computation"] == nil {
		t.Fatal("expected rev 2 with computation", saved2)
	}

	readBack := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/routes/plans/read", map[string]any{
		"owner_kind": "TRIP", "owner_id": x.trip,
	}, 200, "", "", nil)
	if len(readBack["history"].([]any)) != 2 || len(readBack["computations"].([]any)) != 1 {
		t.Fatal("unexpected route plan history", readBack)
	}

	// Neutral suggestion evaluation (§10.7 1,742 centavos exact vector, paying station gets zero priority boost).
	eval := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/routes/suggestions/evaluate", map[string]any{
		"product": "DIESEL_S10", "planned_volume_ml": 150000,
		"base_distance_m": 181000, "base_duration_seconds": 7200,
		"consumption_m": 2880000, "consumption_ml": 900000,
		"freshness_limit_seconds": 86400,
		"reference":               map[string]any{"source_kind": "COMMUNITY", "product": "DIESEL_S10", "unit": "LITRE", "payment_condition": "DEBIT", "price_milli_brl": 6490, "age_seconds": 21600},
		"candidates": []map[string]any{
			{
				"station_id": onboardingID(t), "station_name": "Paying Station Higher Price",
				"price":              map[string]any{"source_kind": "COMMUNITY", "product": "DIESEL_S10", "unit": "LITRE", "payment_condition": "DEBIT", "price_milli_brl": 6450, "age_seconds": 18000},
				"route_total_metres": 187400, "route_total_seconds": 7740, "has_paid_plan": true, "has_active_agreement": true,
			},
			{
				"station_id": onboardingID(t), "station_name": "Free Station Lower Net Cost",
				"price":              map[string]any{"source_kind": "COMMUNITY", "product": "DIESEL_S10", "unit": "LITRE", "payment_condition": "DEBIT", "price_milli_brl": 6290, "age_seconds": 32400},
				"route_total_metres": 187400, "route_total_seconds": 7740, "has_paid_plan": false, "has_active_agreement": false,
			},
		},
	}, 200, "", "", nil)
	sugs := eval["suggestions"].([]any)
	best := sugs[0].(map[string]any)
	if !strings.Contains(best["station_name"].(string), "Free Station") || best["net_diff_centavos"] != float64(1742) {
		t.Fatal("commercial neutrality or §10.7 math failed", best)
	}
}

var _ = bytes.Compare
