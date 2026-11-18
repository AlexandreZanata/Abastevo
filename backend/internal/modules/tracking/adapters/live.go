package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LiveAuthority re-proves gps.location.read_live in transaction (EMP-GPS-007).
type LiveAuthority interface {
	CheckLiveTx(context.Context, pgx.Tx, kernel.OrgScope, app.LiveActor, string, string, string) error
}

// LiveStore serves live positions and derived operational alerts. Every access
// writes a LOCATION_LIVE audit event before returning bytes.
type LiveStore struct {
	Pool      *pgxpool.Pool
	Authority LiveAuthority
	Access    auditapp.AccessRecorder
	NewID     func() (string, error)
	Now       func() time.Time
}

var _ app.LiveRepository = (*LiveStore)(nil)

func liveBinding(scope kernel.OrgScope, label string, ids ...string) string {
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)
	parts := append([]string{"tracking.live.v1", scope.OrgID().String(), label}, sorted...)
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func freshnessLimitsTx(ctx context.Context, tx pgx.Tx, org string, nowFn func() time.Time) (domain.FreshnessLimits, time.Time, error) {
	limits := domain.DefaultFreshnessLimits()
	var dbNow time.Time
	err := tx.QueryRow(ctx, `SELECT
 COALESCE((SELECT s.live_current_seconds FROM org_tracking_settings s WHERE s.org_id=$1),$2),
 COALESCE((SELECT s.live_stale_seconds FROM org_tracking_settings s WHERE s.org_id=$1),$3),
 clock_timestamp()`, org, domain.DefaultCurrentLimitSeconds, domain.DefaultStaleLimitSeconds).
		Scan(&limits.CurrentSeconds, &limits.StaleSeconds, &dbNow)
	if err != nil {
		return domain.FreshnessLimits{}, time.Time{}, err
	}
	if limits.Validate() != nil {
		return domain.FreshnessLimits{}, time.Time{}, app.ErrUnavailable
	}
	if nowFn != nil {
		return limits, nowFn().UTC(), nil
	}
	return limits, dbNow.UTC(), nil
}

func scanLiveFix(lat, lon, acc *int32, speed *int32, bearing *int16, coarse *bool, source, opID *string, col, rcv *time.Time) *domain.LiveFix {
	if lat == nil || lon == nil || acc == nil || coarse == nil || col == nil || rcv == nil {
		return nil
	}
	src := ""
	if source != nil {
		src = *source
	}
	op := ""
	if opID != nil {
		op = *opID
	}
	return &domain.LiveFix{
		CollectedAt: col.UTC(),
		ReceivedAt:  rcv.UTC(),
		Lat:         *lat,
		Lon:         *lon,
		Accuracy:    *acc,
		Speed:       speed,
		Bearing:     bearing,
		Coarse:      *coarse,
		Source:      src,
		OperationID: op,
	}
}

func (s *LiveStore) ReadLive(ctx context.Context, scope kernel.OrgScope, actor app.LiveActor, decision access.Decision, request app.LiveReadRequest) (app.LiveTripView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Access) || s.NewID == nil {
		return app.LiveTripView{}, app.ErrUnavailable
	}
	if err := decision.Check(ctx, scope, actor.AccountID, app.LivePermission, request.TripID); err != nil {
		return app.LiveTripView{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org := scope.OrgID().String()
	var view app.LiveTripView
	var ok bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckLiveTx(ctx, tx, scope, actor, member, request.TripID, ""); err != nil {
			return err
		}
		limits, dbNow, err := freshnessLimitsTx(ctx, tx, org, s.Now)
		if err != nil {
			return err
		}
		var rec app.LiveRecord
		var lat, lon, acc, speed *int32
		var bearing *int16
		var coarse *bool
		var src, opID *string
		var col, rcv *time.Time
		err = tx.QueryRow(ctx, `SELECT t.id::text,COALESCE(s.id::text,''),
 COALESCE((SELECT tr.state FROM org_tracking_transitions tr WHERE tr.org_id=s.org_id AND tr.session_id=s.id ORDER BY tr.seq DESC LIMIT 1),''),
 t.state,COALESCE(t.unit_id::text,''),COALESCE(u.name,''),t.asset_id::text,
 COALESCE(NULLIF(a.fleet_number,''),NULLIF(a.plate,''),a.name,''),t.driver_employee_id::text,
 COALESCE(e.display_name,''),COALESCE(lp.revision,0),
 lp.collected_at,lp.received_at,lp.last_communication_at,lp.lat_e7,lp.lon_e7,lp.accuracy_m,lp.speed_cm_s,lp.bearing_deg,lp.coarse,lp.source,lp.operation_id::text,lp.expires_at
 FROM org_trips t
 LEFT JOIN org_tracking_sessions s ON(s.org_id,s.trip_id)=(t.org_id,t.id)
 LEFT JOIN org_tracking_live_positions lp ON(lp.org_id,lp.trip_id)=(t.org_id,t.id)
 LEFT JOIN org_units u ON(u.org_id,u.id)=(t.org_id,t.unit_id)
 LEFT JOIN org_assets a ON(a.org_id,a.id)=(t.org_id,t.asset_id)
 LEFT JOIN org_employee_profiles e ON(e.org_id,e.employee_id)=(t.org_id,t.driver_employee_id)
 WHERE t.org_id=$1 AND t.id=$2`, org, request.TripID).
			Scan(&rec.TripID, &rec.SessionID, &rec.SessionState, &rec.TripState, &rec.UnitID, &rec.UnitName,
				&rec.AssetID, &rec.AssetLabel, &rec.DriverMembershipID, &rec.DriverName, &rec.Revision,
				&col, &rcv, &rec.LastCommunicationAt, &lat, &lon, &acc, &speed, &bearing, &coarse, &src, &opID, &rec.ExpiresAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return err
		}
		if rec.SessionID == "" {
			return app.ErrNotFound
		}
		rec.Fix = scanLiveFix(lat, lon, acc, speed, bearing, coarse, src, opID, col, rcv)
		view, ok = app.ProjectLiveView(rec, limits, dbNow)
		if !ok {
			return app.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return app.LiveTripView{}, err
	}
	reqID, err := s.NewID()
	if err != nil {
		return app.LiveTripView{}, app.ErrAccessUnrecorded
	}
	record := audit.SensitiveAccess{
		Category:     audit.LocationLive,
		Stage:        audit.AccessServed,
		AccountID:    actor.AccountID,
		MembershipID: member,
		TargetID:     request.TripID,
		AccessID:     reqID,
		RequestID:    reqID,
		Binding:      liveBinding(scope, "read", request.TripID),
	}
	if err := s.Access.RecordAccess(ctx, scope, record); err != nil {
		return app.LiveTripView{}, app.ErrAccessUnrecorded
	}
	return view, nil
}

func (s *LiveStore) ListLive(ctx context.Context, scope kernel.OrgScope, actor app.LiveActor, decision access.Decision, request app.LiveListRequest) (app.LiveListView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Access) || s.NewID == nil {
		return app.LiveListView{}, app.ErrUnavailable
	}
	target := scope.OrgID().String()
	if err := decision.Check(ctx, scope, actor.AccountID, app.LivePermission, target); err != nil {
		return app.LiveListView{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org := scope.OrgID().String()
	var out app.LiveListView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckLiveTx(ctx, tx, scope, actor, member, scope.OrgID().String(), request.UnitID); err != nil {
			return err
		}
		var nowPtr *time.Time
		if s.Now != nil {
			n := s.Now().UTC()
			nowPtr = &n
		}
		if err := detectStaleTrackersTx(ctx, tx, org, nowPtr); err != nil {
			return err
		}
		limits, dbNow, err := freshnessLimitsTx(ctx, tx, org, s.Now)
		if err != nil {
			return err
		}
		out.CurrentSeconds = limits.CurrentSeconds
		out.StaleSeconds = limits.StaleSeconds

		rows, err := tx.Query(ctx, `SELECT lp.trip_id::text,lp.session_id::text,
 COALESCE((SELECT tr.state FROM org_tracking_transitions tr WHERE tr.org_id=lp.org_id AND tr.session_id=lp.session_id ORDER BY tr.seq DESC LIMIT 1),''),
 t.state,COALESCE(t.unit_id::text,''),COALESCE(u.name,''),t.asset_id::text,
 COALESCE(NULLIF(a.fleet_number,''),NULLIF(a.plate,''),a.name,''),t.author_membership_id::text,
 COALESCE(e.display_name,''),lp.revision,
 lp.collected_at,lp.received_at,lp.last_communication_at,lp.lat_e7,lp.lon_e7,lp.accuracy_m,lp.speed_cm_s,lp.bearing_deg,lp.coarse,lp.source,lp.operation_id::text,lp.expires_at
 FROM org_tracking_live_positions lp
 JOIN org_trips t ON(t.org_id,t.id)=(lp.org_id,lp.trip_id)
 LEFT JOIN org_units u ON(u.org_id,u.id)=(t.org_id,t.unit_id)
 LEFT JOIN org_assets a ON(a.org_id,a.id)=(t.org_id,t.asset_id)
 LEFT JOIN org_employee_profiles e ON(e.org_id,e.employee_id)=(t.org_id,t.driver_employee_id)
 WHERE lp.org_id=$1 AND lp.revision>$2 AND ($3='' OR t.unit_id::text=$3)
  AND (lp.expires_at IS NULL OR lp.expires_at > clock_timestamp())
 ORDER BY lp.revision ASC LIMIT $4`, org, request.SinceRevision, request.UnitID, request.Limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var rec app.LiveRecord
			var lat, lon, acc, speed *int32
			var bearing *int16
			var coarse *bool
			var src, opID *string
			var col, rcv *time.Time
			if err := rows.Scan(&rec.TripID, &rec.SessionID, &rec.SessionState, &rec.TripState, &rec.UnitID, &rec.UnitName,
				&rec.AssetID, &rec.AssetLabel, &rec.DriverMembershipID, &rec.DriverName, &rec.Revision,
				&col, &rcv, &rec.LastCommunicationAt, &lat, &lon, &acc, &speed, &bearing, &coarse, &src, &opID, &rec.ExpiresAt); err != nil {
				rows.Close()
				return err
			}
			if rec.Revision > out.MaxRevision {
				out.MaxRevision = rec.Revision
			}
			rec.Fix = scanLiveFix(lat, lon, acc, speed, bearing, coarse, src, opID, col, rcv)
			if view, keep := app.ProjectLiveView(rec, limits, dbNow); keep {
				out.Items = append(out.Items, view)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		arows, err := tx.Query(ctx, `SELECT id::text,kind,trip_id::text,session_id::text,COALESCE(unit_id::text,''),
 COALESCE(geofence_id::text,''),event_kind,anchor_at,observed_at,duration_seconds,threshold_seconds,radius_m,late_suppressed
 FROM org_tracking_alerts WHERE org_id=$1 AND NOT late_suppressed AND ($2='' OR unit_id::text=$2)
 ORDER BY observed_at DESC LIMIT 50`, org, request.UnitID)
		if err != nil {
			return err
		}
		for arows.Next() {
			var av app.AlertView
			if err := arows.Scan(&av.ID, &av.Kind, &av.TripID, &av.SessionID, &av.UnitID, &av.GeofenceID,
				&av.EventKind, &av.AnchorAt, &av.ObservedAt, &av.DurationSeconds, &av.ThresholdSeconds, &av.RadiusMetres, &av.LateSuppressed); err != nil {
				arows.Close()
				return err
			}
			av.AnchorAt, av.ObservedAt = av.AnchorAt.UTC(), av.ObservedAt.UTC()
			out.Alerts = append(out.Alerts, av)
		}
		if err := arows.Err(); err != nil {
			arows.Close()
			return err
		}
		arows.Close()
		return nil
	})
	if err != nil {
		return app.LiveListView{}, err
	}
	if len(out.Items) == 0 {
		reqID, err := s.NewID()
		if err != nil {
			return app.LiveListView{}, app.ErrAccessUnrecorded
		}
		if err := s.Access.RecordAccess(ctx, scope, audit.SensitiveAccess{
			Category:     audit.LocationLive,
			Stage:        audit.AccessServed,
			AccountID:    actor.AccountID,
			MembershipID: member,
			TargetID:     org,
			AccessID:     reqID,
			RequestID:    reqID,
			Binding:      liveBinding(scope, "list-empty"),
		}); err != nil {
			return app.LiveListView{}, app.ErrAccessUnrecorded
		}
		return out, nil
	}
	for _, item := range out.Items {
		reqID, err := s.NewID()
		if err != nil {
			return app.LiveListView{}, app.ErrAccessUnrecorded
		}
		if err := s.Access.RecordAccess(ctx, scope, audit.SensitiveAccess{
			Category:     audit.LocationLive,
			Stage:        audit.AccessServed,
			AccountID:    actor.AccountID,
			MembershipID: member,
			TargetID:     item.TripID,
			AccessID:     reqID,
			RequestID:    reqID,
			Binding:      liveBinding(scope, "list", item.TripID),
		}); err != nil {
			return app.LiveListView{}, app.ErrAccessUnrecorded
		}
	}
	return out, nil
}
