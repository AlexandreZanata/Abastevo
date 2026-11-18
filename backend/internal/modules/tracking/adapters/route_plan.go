package adapters

import (
	"context"
	"encoding/json"
	"errors"

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

const (
	RoutePlanSavedAction = "tracking.route_plan.saved"
	RoutePlanTargetType  = "route_plan"
)

func RoutePlanAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: RoutePlanSavedAction, TargetType: RoutePlanTargetType}}
}

type RoutePlanAuthority interface {
	CheckRoutePlanTx(context.Context, pgx.Tx, kernel.OrgScope, app.RouteActor, string, domain.RouteOwnerKind, string, bool) error
}

type RoutePlanStore struct {
	Pool      *pgxpool.Pool
	Authority RoutePlanAuthority
	Audit     auditapp.TransactionRunner
	NewID     func() (string, error)
}

var _ app.RoutePlanRepository = (*RoutePlanStore)(nil)

func scanRoutePlan(rows pgx.Rows) (domain.RoutePlan, error) {
	var p domain.RoutePlan
	var ownerKind string
	var wRaw []byte
	var pSrc *string
	var compID *string
	if err := rows.Scan(&p.ID, &ownerKind, &p.OwnerID, &p.Revision, &wRaw,
		&p.Options.AvoidToll, &p.Options.AvoidMotorway, &p.Options.AvoidFerry, &p.Options.AvoidUnpaved,
		&p.PlannedDistanceMetres, &pSrc, &compID, &p.HeavyAssetNotice, &p.CreatedAt); err != nil {
		return domain.RoutePlan{}, err
	}
	p.OwnerKind = domain.RouteOwnerKind(ownerKind)
	if pSrc != nil {
		s := domain.PlannedDistanceSource(*pSrc)
		p.PlannedDistanceSource = &s
	}
	if compID != nil {
		p.ComputationID = *compID
	}
	if err := json.Unmarshal(wRaw, &p.Waypoints); err != nil {
		return domain.RoutePlan{}, app.ErrUnavailable
	}
	p.CreatedAt = p.CreatedAt.UTC()
	return p, nil
}

func readRouteComputationsTx(ctx context.Context, tx pgx.Tx, org string, kind domain.RouteOwnerKind, ownerID string) ([]domain.RouteComputation, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,owner_id::text,owner_kind,plan_revision,engine_id,map_data_version,profile,kind,
 avoid_toll,avoid_motorway,avoid_ferry,avoid_unpaved,distance_m,duration_seconds,line_lat_e7,line_lon_e7,attribution,computed_at
 FROM org_route_computations WHERE org_id=$1 AND owner_kind=$2 AND owner_id=$3
 ORDER BY plan_revision DESC, computed_at DESC LIMIT 50`, org, string(kind), ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comps []domain.RouteComputation
	for rows.Next() {
		var c domain.RouteComputation
		var okind, prof, ckind string
		var lLat, lLon []int32
		if err := rows.Scan(&c.ID, &c.OwnerID, &okind, &c.PlanRevision, &c.EngineID, &c.MapDataVersion, &prof, &ckind,
			&c.Options.AvoidToll, &c.Options.AvoidMotorway, &c.Options.AvoidFerry, &c.Options.AvoidUnpaved,
			&c.DistanceMetres, &c.DurationSeconds, &lLat, &lLon, &c.Attribution, &c.ComputedAt); err != nil {
			return nil, err
		}
		c.OwnerKind = domain.RouteOwnerKind(okind)
		c.Profile = domain.RouteProfile(prof)
		c.Kind = domain.RouteComputationKind(ckind)
		c.ComputedAt = c.ComputedAt.UTC()
		for i := range lLat {
			c.Line = append(c.Line, domain.Point{Lat: lLat[i], Lon: lLon[i]})
		}
		comps = append(comps, c)
	}
	return comps, rows.Err()
}

func (s *RoutePlanStore) ReadRoutePlan(ctx context.Context, scope kernel.OrgScope, actor app.RouteActor, decision access.Decision, request app.RoutePlanReadRequest) (app.RoutePlanView, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) {
		return app.RoutePlanView{}, app.ErrUnavailable
	}
	if err := decision.Check(ctx, scope, actor.AccountID, app.RoutePlanPermission, scope.OrgID().String()); err != nil {
		return app.RoutePlanView{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org := scope.OrgID().String()
	view := app.RoutePlanView{OwnerKind: request.OwnerKind, OwnerID: request.OwnerID}

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckRoutePlanTx(ctx, tx, scope, actor, member, request.OwnerKind, request.OwnerID, false); err != nil {
			return err
		}
		if request.OwnerKind == domain.RouteOwnerMission {
			if err := tx.QueryRow(ctx, `SELECT unit_id::text FROM org_trip_missions WHERE org_id=$1 AND id=$2`, org, request.OwnerID).Scan(&view.UnitID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return app.ErrNotFound
				}
				return err
			}
		} else {
			if err := tx.QueryRow(ctx, `SELECT COALESCE(unit_id::text,'') FROM org_trips WHERE org_id=$1 AND id=$2`, org, request.OwnerID).Scan(&view.UnitID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return app.ErrNotFound
				}
				return err
			}
		}

		rows, err := tx.Query(ctx, `SELECT id::text,owner_kind,owner_id::text,revision,waypoints,
 avoid_toll,avoid_motorway,avoid_ferry,avoid_unpaved,planned_distance_m,planned_distance_source,computation_id::text,heavy_asset_notice,created_at
 FROM org_route_plans WHERE org_id=$1 AND owner_kind=$2 AND owner_id=$3
 ORDER BY revision DESC LIMIT 50`, org, string(request.OwnerKind), request.OwnerID)
		if err != nil {
			return err
		}
		for rows.Next() {
			p, err := scanRoutePlan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			view.History = append(view.History, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		if len(view.History) > 0 {
			latest := view.History[0]
			view.Plan = &latest
		}
		comps, err := readRouteComputationsTx(ctx, tx, org, request.OwnerKind, request.OwnerID)
		if err != nil {
			return err
		}
		view.Computations = comps
		if len(comps) > 0 {
			latestComp := comps[0]
			view.LatestComputation = &latestComp
		}
		return nil
	})
	if err != nil {
		return app.RoutePlanView{}, err
	}
	return view, nil
}

func (s *RoutePlanStore) SaveRoutePlan(ctx context.Context, scope kernel.OrgScope, actor app.RouteActor, decision access.Decision, cmd app.RoutePlanSaveCommand) (app.RoutePlanAck, error) {
	if s == nil || s.Pool == nil || absent(s.Authority) || absent(s.Audit) || s.NewID == nil {
		return app.RoutePlanAck{}, app.ErrUnavailable
	}
	if err := decision.Check(ctx, scope, actor.AccountID, app.RoutePlanPermission, scope.OrgID().String()); err != nil {
		return app.RoutePlanAck{}, app.ErrUnavailable
	}
	member := decision.Subject().MembershipID()
	org, digest := scope.OrgID().String(), cmd.Hash()
	var ack app.RoutePlanAck

	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckRoutePlanTx(ctx, tx, scope, actor, member, cmd.OwnerKind, cmd.OwnerID, true); err != nil {
			return err
		}
		var priorActor, priorHash, priorOwner, priorPlanID string
		var priorRev int64
		err := tx.QueryRow(ctx, `SELECT actor_account_id::text,request_hash,owner_id::text,plan_id::text,result_revision
 FROM org_route_plan_commands WHERE org_id=$1 AND operation_id=$2`, org, cmd.OperationID).
			Scan(&priorActor, &priorHash, &priorOwner, &priorPlanID, &priorRev)
		if err == nil {
			if priorActor != actor.AccountID || priorHash != digest || priorOwner != cmd.OwnerID {
				return app.ErrRouteOperationReused
			}
			rows, err := tx.Query(ctx, `SELECT id::text,owner_kind,owner_id::text,revision,waypoints,
 avoid_toll,avoid_motorway,avoid_ferry,avoid_unpaved,planned_distance_m,planned_distance_source,computation_id::text,heavy_asset_notice,created_at
 FROM org_route_plans WHERE org_id=$1 AND id=$2`, org, priorPlanID)
			if err != nil {
				return err
			}
			defer rows.Close()
			if !rows.Next() {
				return app.ErrUnavailable
			}
			p, err := scanRoutePlan(rows)
			if err != nil {
				return err
			}
			ack = app.RoutePlanAck{Plan: p, Replayed: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		planID, err := s.NewID()
		if err != nil {
			return app.ErrUnavailable
		}
		event := audit.Event{OperationID: cmd.OperationID, ActorID: actor.AccountID, Action: RoutePlanSavedAction, TargetType: RoutePlanTargetType, TargetID: planID, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			var currentRev int64
			if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM org_route_plans WHERE org_id=$1 AND owner_kind=$2 AND owner_id=$3`,
				org, string(cmd.OwnerKind), cmd.OwnerID).Scan(&currentRev); err != nil {
				return err
			}
			expected := int64(0)
			if cmd.ExpectedRevision != nil {
				expected = *cmd.ExpectedRevision
			}
			if currentRev != expected {
				return app.ErrRouteConflict
			}
			nextRev := currentRev + 1
			var compID *string
			var compVal *domain.RouteComputation
			if cmd.Computation != nil {
				cid, err := s.NewID()
				if err != nil {
					return app.ErrUnavailable
				}
				validatedComp, err := domain.ValidateRouteComputation(domain.RouteComputation{
					ID:              cid,
					OwnerID:         cmd.OwnerID,
					OwnerKind:       cmd.OwnerKind,
					PlanRevision:    nextRev,
					EngineID:        cmd.Computation.EngineID,
					MapDataVersion:  cmd.Computation.MapDataVersion,
					Profile:         cmd.Computation.Profile,
					Kind:            cmd.Computation.Kind,
					Options:         cmd.Options,
					DistanceMetres:  cmd.Computation.DistanceMetres,
					DurationSeconds: cmd.Computation.DurationSeconds,
					Line:            cmd.Computation.Line,
					Attribution:     cmd.Computation.Attribution,
					ComputedAt:      cmd.Computation.ComputedAt,
				})
				if err != nil {
					return err
				}
				compID = &cid
				compVal = &validatedComp
			}
			cidStr := ""
			if compID != nil {
				cidStr = *compID
			}
			validatedPlan, err := domain.ValidateRoutePlanInput(cmd.OwnerKind, cmd.OwnerID, nextRev, cmd.Waypoints, cmd.Options, cmd.PlannedDistanceMetres, cmd.PlannedDistanceSource, cidStr)
			if err != nil {
				return err
			}
			validatedPlan.ID = planID
			validatedPlan.HeavyAssetNotice = cmd.HeavyAssetNotice

			wJSON, err := json.Marshal(validatedPlan.Waypoints)
			if err != nil {
				return app.ErrUnavailable
			}
			var pSrc *string
			if validatedPlan.PlannedDistanceSource != nil {
				s := string(*validatedPlan.PlannedDistanceSource)
				pSrc = &s
			}
			if err := tx.QueryRow(ctx, `INSERT INTO org_route_plans(
 org_id,id,owner_kind,owner_id,revision,waypoints,avoid_toll,avoid_motorway,avoid_ferry,avoid_unpaved,planned_distance_m,planned_distance_source,computation_id,heavy_asset_notice
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::uuid,$14) RETURNING created_at`,
				org, planID, string(cmd.OwnerKind), cmd.OwnerID, nextRev, wJSON,
				cmd.Options.AvoidToll, cmd.Options.AvoidMotorway, cmd.Options.AvoidFerry, cmd.Options.AvoidUnpaved,
				validatedPlan.PlannedDistanceMetres, pSrc, compID, cmd.HeavyAssetNotice).Scan(&validatedPlan.CreatedAt); err != nil {
				return err
			}
			validatedPlan.CreatedAt = validatedPlan.CreatedAt.UTC()

			if compVal != nil {
				lLat := make([]int32, len(compVal.Line))
				lLon := make([]int32, len(compVal.Line))
				for i, pt := range compVal.Line {
					lLat[i], lLon[i] = pt.Lat, pt.Lon
				}
				if _, err := tx.Exec(ctx, `INSERT INTO org_route_computations(
 org_id,id,owner_kind,owner_id,plan_revision,engine_id,map_data_version,profile,kind,avoid_toll,avoid_motorway,avoid_ferry,avoid_unpaved,distance_m,duration_seconds,line_lat_e7,line_lon_e7,attribution,computed_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
					org, compVal.ID, string(cmd.OwnerKind), cmd.OwnerID, nextRev, compVal.EngineID, compVal.MapDataVersion, string(compVal.Profile), string(compVal.Kind),
					compVal.Options.AvoidToll, compVal.Options.AvoidMotorway, compVal.Options.AvoidFerry, compVal.Options.AvoidUnpaved,
					compVal.DistanceMetres, compVal.DurationSeconds, lLat, lLon, compVal.Attribution, compVal.ComputedAt.UTC()); err != nil {
					return err
				}
			}

			if _, err := tx.Exec(ctx, `INSERT INTO org_route_plan_commands(org_id,operation_id,owner_kind,owner_id,plan_id,actor_account_id,actor_membership_id,request_hash,result_revision)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
				org, cmd.OperationID, string(cmd.OwnerKind), cmd.OwnerID, planID, actor.AccountID, member, digest, nextRev); err != nil {
				return err
			}
			ack = app.RoutePlanAck{Plan: validatedPlan, Computation: compVal}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrRouteOperationReused
		}
		return nil
	})
	if err != nil {
		return app.RoutePlanAck{}, err
	}
	return ack, nil
}
