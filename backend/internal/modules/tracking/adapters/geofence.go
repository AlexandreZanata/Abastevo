package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	GeofenceCreatedAction = "tracking.geofence.created"
	GeofenceChangedAction = "tracking.geofence.changed"
	GeofenceTargetType    = "geofence"
)

func GeofenceAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: GeofenceCreatedAction, TargetType: GeofenceTargetType},
		{Action: GeofenceChangedAction, TargetType: GeofenceTargetType},
	}
}

type GeofenceAuthority interface {
	CheckGeofenceTx(context.Context, pgx.Tx, kernel.OrgScope, app.GeofenceActor, string, string, bool) error
}

type GeofenceStore struct {
	Pool      *pgxpool.Pool
	Authority GeofenceAuthority
	Audit     auditapp.TransactionRunner
	NewID     func() (string, error)
	Now       func() time.Time
}

var _ app.GeofenceRepository = (*GeofenceStore)(nil)

func readGeofenceTx(ctx context.Context, tx pgx.Tx, org, id string) (app.GeofenceView, error) {
	var v app.GeofenceView
	var kind string
	var vLat, vLon []int32
	err := tx.QueryRow(ctx, `SELECT g.id::text,g.revision,g.name,COALESCE(g.unit_id::text,''),g.kind,
 COALESCE(g.center_lat_e7,0),COALESCE(g.center_lon_e7,0),COALESCE(g.radius_m,0),
 g.vertices_lat_e7,g.vertices_lon_e7,g.on_enter,g.on_exit,g.dwell_seconds,
 COALESCE(rec.recs,'{}'::text[]),g.enabled,g.archived,g.created_at,g.updated_at
 FROM org_tracking_geofences g
 LEFT JOIN LATERAL(SELECT array_agg(r::text) AS recs FROM unnest(g.recipients) AS r) rec ON true
 WHERE g.org_id=$1 AND g.id=$2`, org, id).
		Scan(&v.Spec.ID, &v.Revision, &v.Spec.Name, &v.Spec.UnitID, &kind, &v.Spec.CenterLat, &v.Spec.CenterLon,
			&v.Spec.RadiusMetres, &vLat, &vLon, &v.Spec.OnEnter, &v.Spec.OnExit, &v.Spec.DwellSeconds,
			&v.Spec.Recipients, &v.Spec.Enabled, &v.Archived, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return app.GeofenceView{}, err
	}
	v.Spec.Kind = domain.GeofenceKind(kind)
	for i := range vLat {
		v.Spec.Vertices = append(v.Spec.Vertices, domain.Point{Lat: vLat[i], Lon: vLon[i]})
	}
	v.CreatedAt, v.UpdatedAt = v.CreatedAt.UTC(), v.UpdatedAt.UTC()
	return v, nil
}

func geofenceVerticesArrays(verts []domain.Point) ([]int32, []int32) {
	lat := make([]int32, len(verts))
	lon := make([]int32, len(verts))
	for i, p := range verts {
		lat[i], lon[i] = p.Lat, p.Lon
	}
	return lat, lon
}

func (s *GeofenceStore) ListGeofences(ctx context.Context, scope kernel.OrgScope, actor app.GeofenceActor, decision access.Decision, request app.GeofenceListRequest) (app.GeofenceListView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) {
		return app.GeofenceListView{}, app.ErrUnavailable
	}
	target := scope.OrgID().String()
	if err := decision.Check(ctx, scope, actor.AccountID, app.GeofencePermission, target); err != nil {
		return app.GeofenceListView{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org := scope.OrgID().String()
	var out app.GeofenceListView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckGeofenceTx(ctx, tx, scope, actor, member, target, false); err != nil {
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
		rows, err := tx.Query(ctx, `SELECT g.id::text,g.revision,g.name,COALESCE(g.unit_id::text,''),g.kind,
 COALESCE(g.center_lat_e7,0),COALESCE(g.center_lon_e7,0),COALESCE(g.radius_m,0),
 g.vertices_lat_e7,g.vertices_lon_e7,g.on_enter,g.on_exit,g.dwell_seconds,
 COALESCE(rec.recs,'{}'::text[]),g.enabled,g.archived,g.created_at,g.updated_at
 FROM org_tracking_geofences g
 LEFT JOIN LATERAL(SELECT array_agg(r::text) AS recs FROM unnest(g.recipients) AS r) rec ON true
 WHERE g.org_id=$1 AND ($2 OR NOT g.archived) AND ($3='' OR g.unit_id::text=$3)
 ORDER BY g.created_at DESC, g.id`, org, request.IncludeArchived, request.UnitID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v app.GeofenceView
			var kind string
			var vLat, vLon []int32
			if err := rows.Scan(&v.Spec.ID, &v.Revision, &v.Spec.Name, &v.Spec.UnitID, &kind, &v.Spec.CenterLat, &v.Spec.CenterLon,
				&v.Spec.RadiusMetres, &vLat, &vLon, &v.Spec.OnEnter, &v.Spec.OnExit, &v.Spec.DwellSeconds,
				&v.Spec.Recipients, &v.Spec.Enabled, &v.Archived, &v.CreatedAt, &v.UpdatedAt); err != nil {
				rows.Close()
				return err
			}
			v.Spec.Kind = domain.GeofenceKind(kind)
			for i := range vLat {
				v.Spec.Vertices = append(v.Spec.Vertices, domain.Point{Lat: vLat[i], Lon: vLon[i]})
			}
			v.CreatedAt, v.UpdatedAt = v.CreatedAt.UTC(), v.UpdatedAt.UTC()
			out.Geofences = append(out.Geofences, v)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		erows, err := tx.Query(ctx, `SELECT ev.id::text,ev.geofence_id::text,g.name,ev.trip_id::text,ev.session_id::text,
 COALESCE(ev.unit_id::text,''),ev.event_kind,ev.fix_at,ev.detected_at,ev.late_suppressed
 FROM org_tracking_geofence_events ev
 JOIN org_tracking_geofences g ON(g.org_id,g.id)=(ev.org_id,ev.geofence_id)
 WHERE ev.org_id=$1 AND ($2='' OR ev.unit_id::text=$2)
 ORDER BY ev.detected_at DESC LIMIT 100`, org, request.UnitID)
		if err != nil {
			return err
		}
		for erows.Next() {
			var ev app.GeofenceEventView
			if err := erows.Scan(&ev.ID, &ev.GeofenceID, &ev.GeofenceName, &ev.TripID, &ev.SessionID,
				&ev.UnitID, &ev.EventKind, &ev.FixAt, &ev.DetectedAt, &ev.LateSuppressed); err != nil {
				erows.Close()
				return err
			}
			ev.FixAt, ev.DetectedAt = ev.FixAt.UTC(), ev.DetectedAt.UTC()
			out.Events = append(out.Events, ev)
		}
		if err := erows.Err(); err != nil {
			erows.Close()
			return err
		}
		erows.Close()

		arows, err := tx.Query(ctx, `SELECT id::text,kind,trip_id::text,session_id::text,COALESCE(unit_id::text,''),
 COALESCE(geofence_id::text,''),event_kind,anchor_at,observed_at,duration_seconds,threshold_seconds,radius_m,late_suppressed
 FROM org_tracking_alerts WHERE org_id=$1 AND NOT late_suppressed AND ($2='' OR unit_id::text=$2)
 ORDER BY observed_at DESC LIMIT 100`, org, request.UnitID)
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
		return arows.Err()
	})
	if err != nil {
		return app.GeofenceListView{}, err
	}
	return out, nil
}

func (s *GeofenceStore) CreateGeofence(ctx context.Context, scope kernel.OrgScope, actor app.GeofenceActor, decision access.Decision, cmd app.GeofenceCreateCommand) (app.GeofenceAck, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Audit) || s.NewID == nil {
		return app.GeofenceAck{}, app.ErrUnavailable
	}
	target := scope.OrgID().String()
	if err := decision.Check(ctx, scope, actor.AccountID, app.GeofencePermission, target); err != nil {
		return app.GeofenceAck{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org, digest := scope.OrgID().String(), cmd.Hash()
	var ack app.GeofenceAck

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckGeofenceTx(ctx, tx, scope, actor, member, target, true); err != nil {
			return err
		}
		var priorActor, priorHash, priorID string
		err := tx.QueryRow(ctx, `SELECT actor_account_id::text,request_hash,geofence_id::text FROM org_tracking_geofence_commands WHERE org_id=$1 AND operation_id=$2`,
			org, cmd.OperationID).Scan(&priorActor, &priorHash, &priorID)
		if err == nil {
			if priorActor != actor.AccountID || priorHash != digest {
				return app.ErrGeofenceOperationReused
			}
			view, err := readGeofenceTx(ctx, tx, org, priorID)
			if err != nil {
				return err
			}
			ack = app.GeofenceAck{Geofence: view, Replayed: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		geofenceID, err := s.NewID()
		if err != nil {
			return app.ErrUnavailable
		}
		event := audit.Event{OperationID: cmd.OperationID, ActorID: actor.AccountID, Action: GeofenceCreatedAction, TargetType: GeofenceTargetType, TargetID: geofenceID, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			var cLat, cLon, radius *int32
			vLat, vLon := geofenceVerticesArrays(cmd.Spec.Vertices)
			if cmd.Spec.Kind == domain.GeofenceCircle {
				cLat, cLon, radius = &cmd.Spec.CenterLat, &cmd.Spec.CenterLon, &cmd.Spec.RadiusMetres
			}
			recs := cmd.Spec.Recipients
			if recs == nil {
				recs = []string{}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_geofences(
 org_id,id,revision,name,unit_id,kind,center_lat_e7,center_lon_e7,radius_m,vertices_lat_e7,vertices_lon_e7,geometry,on_enter,on_exit,dwell_seconds,recipients,enabled,archived
) VALUES($1,$2,1,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,public.ST_SetSRID(public.ST_MakePoint(0,0),4326),$11,$12,$13,$14::uuid[],$15,false)`,
				org, geofenceID, cmd.Spec.Name, cmd.Spec.UnitID, string(cmd.Spec.Kind), cLat, cLon, radius, vLat, vLon,
				cmd.Spec.OnEnter, cmd.Spec.OnExit, cmd.Spec.DwellSeconds, recs, cmd.Spec.Enabled); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23514" {
					return domain.ErrGeofenceInvalidGeometry
				}
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_geofence_commands(org_id,operation_id,geofence_id,kind,actor_account_id,actor_membership_id,request_hash,result_revision)
 VALUES($1,$2,$3,'CREATE',$4,$5,$6,1)`, org, cmd.OperationID, geofenceID, actor.AccountID, member, digest); err != nil {
				return err
			}
			view, err := readGeofenceTx(ctx, tx, org, geofenceID)
			if err != nil {
				return err
			}
			ack = app.GeofenceAck{Geofence: view}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrGeofenceOperationReused
		}
		return nil
	})
	if err != nil {
		return app.GeofenceAck{}, err
	}
	return ack, nil
}

func (s *GeofenceStore) ChangeGeofence(ctx context.Context, scope kernel.OrgScope, actor app.GeofenceActor, decision access.Decision, cmd app.GeofenceChangeCommand) (app.GeofenceAck, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Audit) || s.NewID == nil {
		return app.GeofenceAck{}, app.ErrUnavailable
	}
	target := cmd.GeofenceID
	if err := decision.Check(ctx, scope, actor.AccountID, app.GeofencePermission, target); err != nil {
		return app.GeofenceAck{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org, digest := scope.OrgID().String(), cmd.Hash()
	var ack app.GeofenceAck

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckGeofenceTx(ctx, tx, scope, actor, member, target, true); err != nil {
			return err
		}
		var priorActor, priorHash, priorID string
		err := tx.QueryRow(ctx, `SELECT actor_account_id::text,request_hash,geofence_id::text FROM org_tracking_geofence_commands WHERE org_id=$1 AND operation_id=$2`,
			org, cmd.OperationID).Scan(&priorActor, &priorHash, &priorID)
		if err == nil {
			if priorActor != actor.AccountID || priorHash != digest || priorID != cmd.GeofenceID {
				return app.ErrGeofenceOperationReused
			}
			view, err := readGeofenceTx(ctx, tx, org, priorID)
			if err != nil {
				return err
			}
			ack = app.GeofenceAck{Geofence: view, Replayed: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		event := audit.Event{OperationID: cmd.OperationID, ActorID: actor.AccountID, Action: GeofenceChangedAction, TargetType: GeofenceTargetType, TargetID: cmd.GeofenceID, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			var currentRev int64
			if err := tx.QueryRow(ctx, `SELECT revision FROM org_tracking_geofences WHERE org_id=$1 AND id=$2 FOR UPDATE`, org, cmd.GeofenceID).Scan(&currentRev); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return app.ErrNotFound
				}
				return err
			}
			if currentRev != cmd.ExpectedRevision {
				return app.ErrGeofenceConflict
			}
			var cLat, cLon, radius *int32
			vLat, vLon := geofenceVerticesArrays(cmd.Spec.Vertices)
			if cmd.Spec.Kind == domain.GeofenceCircle {
				cLat, cLon, radius = &cmd.Spec.CenterLat, &cmd.Spec.CenterLon, &cmd.Spec.RadiusMetres
			}
			recs := cmd.Spec.Recipients
			if recs == nil {
				recs = []string{}
			}
			nextRev := currentRev + 1
			if _, err := tx.Exec(ctx, `UPDATE org_tracking_geofences SET
 revision=$3,name=$4,unit_id=NULLIF($5,'')::uuid,kind=$6,center_lat_e7=$7,center_lon_e7=$8,radius_m=$9,
 vertices_lat_e7=$10,vertices_lon_e7=$11,on_enter=$12,on_exit=$13,dwell_seconds=$14,recipients=$15::uuid[],enabled=$16,archived=$17
 WHERE org_id=$1 AND id=$2`,
				org, cmd.GeofenceID, nextRev, cmd.Spec.Name, cmd.Spec.UnitID, string(cmd.Spec.Kind), cLat, cLon, radius, vLat, vLon,
				cmd.Spec.OnEnter, cmd.Spec.OnExit, cmd.Spec.DwellSeconds, recs, cmd.Spec.Enabled, cmd.Archived); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23514" {
					return domain.ErrGeofenceInvalidGeometry
				}
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO org_tracking_geofence_commands(org_id,operation_id,geofence_id,kind,actor_account_id,actor_membership_id,request_hash,result_revision)
 VALUES($1,$2,$3,'CHANGE',$4,$5,$6,$7)`, org, cmd.OperationID, cmd.GeofenceID, actor.AccountID, member, digest, nextRev); err != nil {
				return err
			}
			view, err := readGeofenceTx(ctx, tx, org, cmd.GeofenceID)
			if err != nil {
				return err
			}
			ack = app.GeofenceAck{Geofence: view}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrGeofenceOperationReused
		}
		return nil
	})
	if err != nil {
		return app.GeofenceAck{}, err
	}
	return ack, nil
}
