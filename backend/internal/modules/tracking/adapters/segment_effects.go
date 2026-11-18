package adapters

import (
	"context"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

func hasLiveSchemaTx(ctx context.Context, q syncapp.DBTX) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('org_tracking_live_positions') IS NOT NULL`).Scan(&ok); err != nil {
		return false, storeError(err)
	}
	return ok, nil
}

func touchLiveSessionTx(ctx context.Context, q syncapp.DBTX, scope kernel.OrgScope, tripID, sessionID string) error {
	ok, err := hasLiveSchemaTx(ctx, q)
	if err != nil || !ok {
		return err
	}
	var exp *time.Time
	err = q.QueryRow(ctx, `SELECT COALESCE(
 (SELECT f.occurred_at + interval '24 hours' FROM org_trip_finishes f WHERE f.org_id=$1 AND f.trip_id=$2),
 (SELECT tr.occurred_at + interval '24 hours' FROM org_trip_transitions tr WHERE tr.org_id=$1 AND tr.trip_id=$2 AND tr.command='CANCEL' LIMIT 1)
)`, scope.OrgID().String(), tripID).Scan(&exp)
	if err != nil {
		return storeError(err)
	}
	_, err = q.Exec(ctx, `INSERT INTO org_tracking_live_positions(org_id,trip_id,session_id,last_communication_at,expires_at)
 VALUES($1,$2,$3,clock_timestamp(),$4)
 ON CONFLICT(org_id,trip_id) DO UPDATE SET
  last_communication_at=GREATEST(COALESCE(org_tracking_live_positions.last_communication_at,clock_timestamp()),clock_timestamp()),
  expires_at=COALESCE(EXCLUDED.expires_at,org_tracking_live_positions.expires_at)`,
		scope.OrgID().String(), tripID, sessionID, exp)
	if err != nil {
		return storeError(err)
	}
	return nil
}

func projectSegmentLiveAndAlertsTx(ctx context.Context, q syncapp.DBTX, scope kernel.OrgScope, tripID, sessionID, operationID, source string, fixes []app.StoredFix) error {
	ok, err := hasLiveSchemaTx(ctx, q)
	if err != nil || !ok {
		return err
	}
	org := scope.OrgID().String()
	var exp *time.Time
	var unitID string
	if err := q.QueryRow(ctx, `SELECT COALESCE(t.unit_id::text,''), COALESCE(
 (SELECT f.occurred_at + interval '24 hours' FROM org_trip_finishes f WHERE f.org_id=$1 AND f.trip_id=$2),
 (SELECT tr.occurred_at + interval '24 hours' FROM org_trip_transitions tr WHERE tr.org_id=$1 AND tr.trip_id=$2 AND tr.command='CANCEL' LIMIT 1)
) FROM org_trips t WHERE t.org_id=$1 AND t.id=$2`, org, tripID).Scan(&unitID, &exp); err != nil {
		return storeError(err)
	}
	if len(fixes) == 0 {
		if _, err := q.Exec(ctx, `INSERT INTO org_tracking_live_positions(org_id,trip_id,session_id,last_communication_at,expires_at)
 VALUES($1,$2,$3,clock_timestamp(),$4)
 ON CONFLICT(org_id,trip_id) DO UPDATE SET
  last_communication_at=GREATEST(COALESCE(org_tracking_live_positions.last_communication_at,clock_timestamp()),clock_timestamp()),
  expires_at=COALESCE(EXCLUDED.expires_at,org_tracking_live_positions.expires_at)`, org, tripID, sessionID, exp); err != nil {
			return storeError(err)
		}
		return nil
	}
	latest := fixes[0]
	for _, f := range fixes[1:] {
		if f.At.After(latest.At) {
			latest = f
		}
	}
	if _, err := q.Exec(ctx, `INSERT INTO org_tracking_live_positions(
 org_id,trip_id,session_id,collected_at,received_at,last_communication_at,lat_e7,lon_e7,accuracy_m,speed_cm_s,bearing_deg,coarse,source,operation_id,expires_at
) VALUES($1,$2,$3,$4,clock_timestamp(),clock_timestamp(),$5,$6,$7,$8,$9,$10,$11,$12,$13)
 ON CONFLICT(org_id,trip_id) DO UPDATE SET
  collected_at=EXCLUDED.collected_at,
  received_at=EXCLUDED.received_at,
  last_communication_at=EXCLUDED.last_communication_at,
  lat_e7=EXCLUDED.lat_e7,
  lon_e7=EXCLUDED.lon_e7,
  accuracy_m=EXCLUDED.accuracy_m,
  speed_cm_s=EXCLUDED.speed_cm_s,
  bearing_deg=EXCLUDED.bearing_deg,
  coarse=EXCLUDED.coarse,
  source=EXCLUDED.source,
  operation_id=EXCLUDED.operation_id,
  expires_at=COALESCE(EXCLUDED.expires_at,org_tracking_live_positions.expires_at)`,
		org, tripID, sessionID, latest.At.UTC(), latest.Lat, latest.Lon, latest.Accuracy, latest.Speed, latest.Bearing, latest.Coarse, source, operationID, exp); err != nil {
		return storeError(err)
	}
	if err := evaluateSessionGeofencesTx(ctx, q, org, tripID, sessionID, unitID, fixes); err != nil {
		return err
	}
	return evaluateSessionLongStopTx(ctx, q, org, tripID, sessionID, unitID)
}

func evaluateSessionGeofencesTx(ctx context.Context, q syncapp.DBTX, org, tripID, sessionID, unitID string, fixes []app.StoredFix) error {
	sorted := make([]app.StoredFix, len(fixes))
	copy(sorted, fixes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })

	rows, err := q.Query(ctx, `SELECT id::text,name,COALESCE(unit_id::text,''),kind,
 COALESCE(center_lat_e7,0),COALESCE(center_lon_e7,0),COALESCE(radius_m,0),
 vertices_lat_e7,vertices_lon_e7,on_enter,on_exit,dwell_seconds,
 COALESCE(array_agg_rec.recs,'{}'::text[]),enabled
 FROM org_tracking_geofences g
 LEFT JOIN LATERAL(SELECT array_agg(r::text) AS recs FROM unnest(g.recipients) AS r) array_agg_rec ON true
 WHERE g.org_id=$1 AND g.enabled AND NOT g.archived AND(g.unit_id IS NULL OR g.unit_id::text=$2)`, org, unitID)
	if err != nil {
		return storeError(err)
	}
	type fenceRow struct {
		spec domain.GeofenceSpec
	}
	var fences []fenceRow
	for rows.Next() {
		var f domain.GeofenceSpec
		var kind string
		var vLat, vLon []int32
		if err := rows.Scan(&f.ID, &f.Name, &f.UnitID, &kind, &f.CenterLat, &f.CenterLon, &f.RadiusMetres, &vLat, &vLon, &f.OnEnter, &f.OnExit, &f.DwellSeconds, &f.Recipients, &f.Enabled); err != nil {
			rows.Close()
			return storeError(err)
		}
		f.Kind = domain.GeofenceKind(kind)
		for i := range vLat {
			f.Vertices = append(f.Vertices, domain.Point{Lat: vLat[i], Lon: vLon[i]})
		}
		fences = append(fences, fenceRow{spec: f})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return storeError(err)
	}
	rows.Close()
	if len(fences) == 0 {
		return nil
	}
	var dbNow time.Time
	if err := q.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
		return storeError(err)
	}
	dbNow = dbNow.UTC()

	trackFixes := make([]domain.TrackFix, len(sorted))
	for i, fx := range sorted {
		trackFixes[i] = domain.TrackFix{At: fx.At, Lat: fx.Lat, Lon: fx.Lon, Accuracy: fx.Accuracy, Coarse: fx.Coarse}
	}
	var lastFixAt time.Time
	if len(sorted) > 0 {
		lastFixAt = sorted[len(sorted)-1].At.UTC()
	}

	for _, fr := range fences {
		var st domain.GeofenceState
		var enteredAt, prevLastFixAt *time.Time
		err := q.QueryRow(ctx, `SELECT inside,entered_at,dwell_emitted,last_fix_at FROM org_tracking_geofence_states
 WHERE org_id=$1 AND geofence_id=$2 AND session_id=$3 FOR UPDATE`, org, fr.spec.ID, sessionID).
			Scan(&st.Inside, &enteredAt, &st.DwellEmitted, &prevLastFixAt)
		switch {
		case err == nil:
			st.Known = true
			if enteredAt != nil {
				st.EnteredAt = enteredAt.UTC()
			}
		case err == pgx.ErrNoRows:
			st.Known = false
		default:
			return storeError(err)
		}
		events, nextState := domain.EvaluateGeofence(fr.spec, tripID, sessionID, st, trackFixes, dbNow)
		for _, ev := range events {
			evID, idErr := jobs.NewUUIDv4()
			if idErr != nil {
				return app.ErrUnavailable
			}
			if _, err := q.Exec(ctx, `INSERT INTO org_tracking_geofence_events(org_id,id,geofence_id,trip_id,session_id,unit_id,event_kind,fix_at,late_suppressed)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9)
 ON CONFLICT(org_id,geofence_id,session_id,event_kind,fix_at) DO NOTHING`,
				org, evID, fr.spec.ID, tripID, sessionID, unitID, string(ev.Kind), ev.FixAt.UTC(), ev.LateSuppressed); err != nil {
				return storeError(err)
			}
			if !ev.LateSuppressed {
				alertID, idErr := jobs.NewUUIDv4()
				if idErr != nil {
					return app.ErrUnavailable
				}
				if _, err := q.Exec(ctx, `INSERT INTO org_tracking_alerts(org_id,id,kind,trip_id,session_id,unit_id,geofence_id,event_kind,anchor_at,observed_at,duration_seconds,threshold_seconds,radius_m,late_suppressed)
 VALUES($1,$2,'GEOFENCE',$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9,0,0,0,false)
 ON CONFLICT DO NOTHING`,
					org, alertID, tripID, sessionID, unitID, fr.spec.ID, string(ev.Kind), ev.FixAt.UTC(), dbNow); err != nil {
					return storeError(err)
				}
			}
		}
		var storeEntered *time.Time
		if !nextState.EnteredAt.IsZero() {
			t := nextState.EnteredAt.UTC()
			storeEntered = &t
		}
		var storeLast *time.Time
		if !lastFixAt.IsZero() {
			storeLast = &lastFixAt
		} else {
			storeLast = prevLastFixAt
		}
		if _, err := q.Exec(ctx, `INSERT INTO org_tracking_geofence_states(org_id,geofence_id,trip_id,session_id,inside,entered_at,dwell_emitted,last_fix_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT(org_id,geofence_id,session_id) DO UPDATE SET
  inside=EXCLUDED.inside,entered_at=EXCLUDED.entered_at,dwell_emitted=EXCLUDED.dwell_emitted,last_fix_at=EXCLUDED.last_fix_at`,
			org, fr.spec.ID, tripID, sessionID, nextState.Inside, storeEntered, nextState.DwellEmitted, storeLast); err != nil {
			return storeError(err)
		}
	}
	return nil
}

func evaluateSessionLongStopTx(ctx context.Context, q syncapp.DBTX, org, tripID, sessionID, unitID string) error {
	var threshold int64 = domain.DefaultLongStopSeconds
	var dbNow time.Time
	err := q.QueryRow(ctx, `SELECT COALESCE((SELECT s.long_stop_seconds FROM org_tracking_settings s WHERE s.org_id=$1),$2), clock_timestamp()`,
		org, domain.DefaultLongStopSeconds).Scan(&threshold, &dbNow)
	if err != nil {
		return storeError(err)
	}
	rows, err := q.Query(ctx, `SELECT u.collected_at,u.lat_e7,u.lon_e7,u.accuracy_m,u.coarse
 FROM org_tracking_segments seg
 CROSS JOIN LATERAL unnest(seg.collected_at,seg.lat_e7,seg.lon_e7,seg.accuracy_m,seg.coarse) AS u(collected_at,lat_e7,lon_e7,accuracy_m,coarse)
 WHERE seg.org_id=$1 AND seg.session_id=$2
 ORDER BY u.collected_at ASC`, org, sessionID)
	if err != nil {
		return storeError(err)
	}
	var fixes []domain.TrackFix
	for rows.Next() {
		var fx domain.TrackFix
		if err := rows.Scan(&fx.At, &fx.Lat, &fx.Lon, &fx.Accuracy, &fx.Coarse); err != nil {
			rows.Close()
			return storeError(err)
		}
		fx.At = fx.At.UTC()
		fixes = append(fixes, fx)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return storeError(err)
	}
	rows.Close()
	alert, err := domain.EvaluateLongStop(tripID, sessionID, unitID, fixes, dbNow.UTC(), threshold)
	if err != nil {
		return app.ErrUnavailable
	}
	if alert == nil {
		return nil
	}
	alertID, idErr := jobs.NewUUIDv4()
	if idErr != nil {
		return app.ErrUnavailable
	}
	_, err = q.Exec(ctx, `INSERT INTO org_tracking_alerts(org_id,id,kind,trip_id,session_id,unit_id,geofence_id,event_kind,anchor_at,observed_at,duration_seconds,threshold_seconds,radius_m,late_suppressed)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULL,'',$7,$8,$9,$10,$11,false)
 ON CONFLICT DO NOTHING`,
		org, alertID, string(alert.Kind), alert.TripID, alert.SessionID, unitID,
		alert.AnchorAt.UTC(), alert.ObservedAt.UTC(), alert.DurationSeconds, alert.ThresholdSeconds, alert.RadiusMetres)
	if err != nil {
		return storeError(err)
	}
	return nil
}

func tripRetentionWindowTx(ctx context.Context, q syncapp.DBTX, scope kernel.OrgScope, tripID string) (effectiveEnd *time.Time, retentionDays int, dbNow time.Time, err error) {
	org := scope.OrgID().String()
	retentionDays = domain.DefaultRawRetentionDays
	err = q.QueryRow(ctx, `SELECT
 COALESCE((SELECT f.occurred_at FROM org_trip_finishes f WHERE f.org_id=$1 AND f.trip_id=$2),
  (SELECT tr.occurred_at FROM org_trip_transitions tr WHERE tr.org_id=$1 AND tr.trip_id=$2 AND tr.command='CANCEL' LIMIT 1)),
 COALESCE((SELECT s.raw_retention_days FROM org_tracking_settings s WHERE s.org_id=$1), $3),
 clock_timestamp()`,
		org, tripID, domain.DefaultRawRetentionDays).Scan(&effectiveEnd, &retentionDays, &dbNow)
	if err != nil {
		return nil, 0, time.Time{}, storeError(err)
	}
	if effectiveEnd != nil {
		utc := effectiveEnd.UTC()
		effectiveEnd = &utc
	}
	return effectiveEnd, retentionDays, dbNow.UTC(), nil
}

// detectStaleTrackersTx inspects open recording sessions against the
// organization's stale_tracker_seconds threshold and persists idempotent
// coordinate-free STALE_TRACKER alerts (EMP-GPS-014, BR-GPS-031, BR-GPS-038).
func detectStaleTrackersTx(ctx context.Context, tx pgx.Tx, org string, nowOpt *time.Time) error {
	var threshold int64 = domain.DefaultStaleTrackerSeconds
	var dbNow time.Time
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT s.stale_tracker_seconds FROM org_tracking_settings s WHERE s.org_id=$1),$2), clock_timestamp()`,
		org, domain.DefaultStaleTrackerSeconds).Scan(&threshold, &dbNow); err != nil {
		return err
	}
	dbNow = dbNow.UTC()
	if nowOpt != nil {
		dbNow = nowOpt.UTC()
	}
	rows, err := tx.Query(ctx, `SELECT t.id::text,s.id::text,COALESCE(t.unit_id::text,''),t.state='IN_PROGRESS',
 COALESCE((SELECT tr.state='RECORDING' FROM org_tracking_transitions tr WHERE tr.org_id=s.org_id AND tr.session_id=s.id ORDER BY tr.seq DESC LIMIT 1),false),
 s.opened_at,lp.last_communication_at
 FROM org_tracking_sessions s
 JOIN org_trips t ON(t.org_id,t.id)=(s.org_id,s.trip_id)
 LEFT JOIN org_tracking_live_positions lp ON(lp.org_id,lp.trip_id)=(s.org_id,s.trip_id)
 WHERE s.org_id=$1 AND t.state='IN_PROGRESS'`, org)
	if err != nil {
		return err
	}
	type cand struct {
		tripID, sessionID, unitID string
		inProgress, recording     bool
		openedAt                  time.Time
		lastComm                  *time.Time
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.tripID, &c.sessionID, &c.unitID, &c.inProgress, &c.recording, &c.openedAt, &c.lastComm); err != nil {
			rows.Close()
			return err
		}
		cands = append(cands, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, c := range cands {
		alert, err := domain.EvaluateStaleTracker(c.tripID, c.sessionID, c.unitID, c.inProgress, c.recording, c.openedAt, c.lastComm, dbNow, threshold)
		if err != nil {
			return app.ErrUnavailable
		}
		if alert == nil {
			continue
		}
		alertID, idErr := jobs.NewUUIDv4()
		if idErr != nil {
			return app.ErrUnavailable
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_alerts(org_id,id,kind,trip_id,session_id,unit_id,geofence_id,event_kind,anchor_at,observed_at,duration_seconds,threshold_seconds,radius_m,late_suppressed)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULL,'',$7,$8,$9,$10,0,false)
 ON CONFLICT DO NOTHING`,
			org, alertID, string(alert.Kind), alert.TripID, alert.SessionID, c.unitID,
			alert.AnchorAt.UTC(), alert.ObservedAt.UTC(), alert.DurationSeconds, alert.ThresholdSeconds); err != nil {
			return err
		}
	}
	return nil
}
