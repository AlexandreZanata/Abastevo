package adapters

import (
	"context"
	"errors"

	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

const tripFieldPlan = `SELECT COALESCE((
 SELECT jsonb_build_object('id',p.id,'revision',p.revision,'owner_kind',p.owner_kind,'owner_id',p.owner_id,
  'waypoints',p.waypoints,'avoid_toll',p.avoid_toll,'avoid_motorway',p.avoid_motorway,
  'avoid_ferry',p.avoid_ferry,'avoid_unpaved',p.avoid_unpaved,'heavy_asset_notice',p.heavy_asset_notice,
  'planned_distance_metres',p.planned_distance_m,'planned_distance_source',p.planned_distance_source,
  'updated_at',p.created_at)
 FROM org_route_plans p
 WHERE p.org_id=$1 AND ((p.owner_kind='TRIP' AND p.owner_id=$2) OR
  (p.owner_kind='MISSION' AND p.owner_id=(SELECT mission_id FROM org_trip_mission_links WHERE org_id=$1 AND trip_id=$2)))
 ORDER BY (p.owner_kind='TRIP') DESC,p.revision DESC LIMIT 1
),'null'::jsonb)`

func (s *ReferenceStore) PullTripField(ctx context.Context, source app.Source, tripID string) (app.TripField, error) {
	var field app.TripField
	if s == nil || s.Pool == nil || s.Confirm == nil || s.Authority == nil || source.Validate() != nil {
		return field, app.ErrUnavailable
	}
	org := source.Scope.OrgID().String()
	err := database.WithOrgTx(ctx, s.Pool, source.Scope, func(tx pgx.Tx) error {
		now, err := s.Confirm(ctx, tx, source)
		if err != nil {
			return err
		}
		authority, err := s.Authority.LoadAuthorityTx(ctx, tx, source.Scope, source.Actor.AccountID)
		if err != nil {
			return err
		}
		if authority.MembershipID != source.MembershipID || authority.EmployeeID == "" {
			return orgapi.NotFound
		}
		var asset, unit, costCentre, zone string
		err = tx.QueryRow(ctx, `SELECT asset_id::text,COALESCE(unit_id::text,''),COALESCE(cost_centre_id::text,''),reporting_zone
         FROM org_trips WHERE org_id=$1 AND id=$2 AND author_membership_id=$3 AND driver_employee_id=$4
         AND state IN ('IN_PROGRESS','PAUSED') FOR SHARE`, org, tripID, source.MembershipID, authority.EmployeeID).Scan(&asset, &unit, &costCentre, &zone)
		if errors.Is(err, pgx.ErrNoRows) {
			return orgapi.NotFound
		}
		if err != nil {
			return err
		}
		facts := permission.ResourceFacts{OrgID: source.Scope.OrgID(), ID: tripID, AssetID: asset, UnitID: unit,
			CostCentreID: costCentre, AuthoredBy: source.MembershipID, TripDriver: source.MembershipID}
		if _, err := permission.CoveringRules(source.Scope, source.MembershipID, authority.Assignments, "trip.trip.read", facts, now); err != nil {
			return orgapi.NotFound
		}
		field.TripID = tripID
		if err := tx.QueryRow(ctx, tripFieldPlan, org, tripID).Scan(&field.Plan); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `WITH pending AS (
           SELECT expiry_day-(($4::timestamptz AT TIME ZONE $5)::date) AS days, lead_days
           FROM org_compliance_items WHERE org_id=$1 AND state IN ('UPCOMING','DUE','OVERDUE')
           AND ((subject_kind='ASSET' AND subject_id=$2) OR (subject_kind='EMPLOYEE' AND subject_id=$3))
         )
         SELECT count(*) FILTER (WHERE days<0),count(*) FILTER (WHERE days BETWEEN 0 AND lead_days),
           min(days) FILTER (WHERE days BETWEEN 0 AND lead_days) FROM pending`,
			org, asset, authority.EmployeeID, now, zone).Scan(&field.Expired, &field.DueSoon, &field.NearestDays); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_work_orders
         WHERE org_id=$1 AND asset_id=$2 AND requester_membership_id=$3 AND origin_kind='DRIVER_REQUEST'
         AND state IN ('REQUESTED','QUOTED','APPROVED','SCHEDULED','IN_EXECUTION')`, org, asset, source.MembershipID).Scan(&field.OpenMaintenance); err != nil {
			return err
		}
		var voucher app.TripVoucher
		err = tx.QueryRow(ctx, `SELECT v.id::text,v.fuel_product,v.unit,v.max_quantity_milli,v.max_centavos,
           floor(extract(epoch FROM v.valid_until))::bigint,v.state
         FROM org_fuel_authorizations v JOIN org_station_agreements a ON a.org_id=v.org_id AND a.id=v.agreement_id
         WHERE v.org_id=$1 AND v.employee_id=$2 AND (v.asset_id=$3 OR v.asset_id IS NULL)
           AND a.state='ACTIVE' AND $4::timestamptz>=a.valid_from AND $4::timestamptz<a.valid_until
         ORDER BY CASE WHEN v.state='ISSUED' AND v.valid_until>$4 THEN v.valid_until END ASC NULLS LAST,
           v.valid_until DESC,v.id LIMIT 1`,
			org, authority.EmployeeID, asset, now).Scan(&voucher.ID, &voucher.Product, &voucher.Unit,
			&voucher.MaxQuantityMilli, &voucher.MaxCentavos, &voucher.ValidUntil, &voucher.State)
		if err == nil {
			field.Voucher = &voucher
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = s.Confirm(ctx, tx, source)
		return err
	})
	return field, err
}
