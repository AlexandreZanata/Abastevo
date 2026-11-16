package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StructureKind names the one record type a route group protects. A source
// never infers the kind from request data (EDR-161).
type StructureKind uint8

const (
	// AssetStructure: the protected record is an asset; it resolves to itself
	// and to its FLEET-005 allocation at the evaluated instant.
	AssetStructure StructureKind = iota + 1
	// HeldMeterReadingStructure: a held reading resolves to the asset and unit
	// it stored when held, no cost centre, and the teams whose links covered
	// its author employee at the acknowledgement receipt instant.
	HeldMeterReadingStructure
	// TripStructure: a stored trip resolves to its asset, the unit and cost
	// centre it stored at the start, and the teams whose links covered its
	// author or driver at the start receipt (EMP-TRIP-004, EDR-222).
	TripStructure
	// MissionStructure: a stored mission resolves to the unit and the
	// optional asset it names (EMP-TRIP-008, EDR-231).
	MissionStructure
	// IncidentStructure: a damage or incident report resolves to its asset
	// and the unit and cost centre it stored at its declared instant
	// (EMP-CHECK-003, EDR-241); no team facts yet.
	IncidentStructure
	// HeldTripStructure: a held trip fact (EMP-TRIP-007, EDR-263). A held
	// start resolves to the asset it declared, that asset's FLEET-005 unit
	// and cost centre at the declared instant and the teams covering its
	// author or driver at the receipt; a held finish, event, submission or
	// EMP-TRIP-011 lifecycle command to the stored structure of the trip it
	// names (as TripStructure).
	HeldTripStructure
)

// ResourceStructure is the EMP-AUTH-006 structural scope lookup for one
// record kind. It reads server state only and sets only structural facts.
// Prepare uses the pool form; the company authority re-check uses the same
// lookup inside the owning transaction with database time.
type ResourceStructure struct {
	Kind        StructureKind
	Pool        *pgxpool.Pool
	Allocations fleet.AllocationSnapshotPort
}

var _ access.StructureSource = (*ResourceStructure)(nil)

var errStructureNotFound = errors.New("organization: structural record not found")

// heldReadingStructure reads only immutable held facts and retained team link
// periods; the link window is half-open like every other server period.
const heldReadingStructure = `SELECT h.asset_id::text,COALESCE(h.unit_id::text,''),
 COALESCE((SELECT array_agg(DISTINCT l.team_id::text) FROM org_team_links l WHERE l.org_id=i.org_id AND l.employee_id=i.author_employee_id
  AND l.started_at<=a.received_at AND(l.ended_at IS NULL OR l.ended_at>a.received_at)),'{}'::text[])
FROM org_sync_review_items i
JOIN org_meter_held_readings h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
WHERE i.org_id=$1 AND i.id=$2 AND i.operation_type='meter.reading.record' AND i.operation_schema=1`

// incidentStructure reads only the immutable report snapshot.
const incidentStructure = `SELECT asset_id::text,COALESCE(unit_id::text,''),COALESCE(cost_centre_id::text,'') FROM org_incident_reports WHERE org_id=$1 AND id=$2`

// tripStructure reads only the immutable start snapshot and retained team
// link periods (half-open, at the trip's server receipt).
const tripStructure = `SELECT t.asset_id::text,COALESCE(t.unit_id::text,''),COALESCE(t.cost_centre_id::text,''),
 COALESCE((SELECT array_agg(DISTINCT l.team_id::text) FROM org_team_links l WHERE l.org_id=t.org_id AND l.employee_id IN(t.author_employee_id,t.driver_employee_id)
  AND l.started_at<=t.received_at AND(l.ended_at IS NULL OR l.ended_at>t.received_at)),'{}'::text[])
FROM org_trips t WHERE t.org_id=$1 AND t.id=$2`

// heldTripStructure reads only immutable held facts, the retained
// allocation periods and team link periods (half-open, at the receipt).
const heldTripStructure = `SELECT x.asset_id::text,COALESCE(x.unit_id::text,''),COALESCE(x.cost_centre_id::text,''),
 COALESCE((SELECT array_agg(DISTINCT l.team_id::text) FROM org_team_links l WHERE l.org_id=$1 AND l.employee_id IN(x.author_employee_id,x.driver_employee_id)
  AND l.started_at<=x.received_at AND(l.ended_at IS NULL OR l.ended_at>x.received_at)),'{}'::text[])
FROM(SELECT h.asset_id,u.unit_id,CASE WHEN u.unit_id IS NULL THEN NULL ELSE c.cost_centre_id END AS cost_centre_id,i.author_employee_id,h.driver_employee_id,a.received_at
 FROM org_sync_review_items i JOIN org_trip_held_starts h ON(h.org_id,h.review_item_id,h.operation_id)=(i.org_id,i.id,i.operation_id)
 JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.proof_version)
 LEFT JOIN org_asset_unit_periods u ON u.org_id=h.org_id AND u.asset_id=h.asset_id AND u.valid_from<=h.occurred_at AND(u.valid_to IS NULL OR h.occurred_at<u.valid_to)
 LEFT JOIN org_asset_cost_centre_periods c ON c.org_id=h.org_id AND c.asset_id=h.asset_id AND c.valid_from<=h.occurred_at AND(c.valid_to IS NULL OR h.occurred_at<c.valid_to)
 WHERE i.org_id=$1 AND i.id=$2 AND i.operation_type='trip.start'
 UNION ALL
 SELECT t.asset_id,t.unit_id,t.cost_centre_id,t.author_employee_id,t.driver_employee_id,t.received_at
 FROM org_sync_review_items i JOIN org_trips t ON t.org_id=i.org_id AND t.id=COALESCE((SELECT f.trip_id FROM org_trip_held_finishes f WHERE f.org_id=i.org_id AND f.review_item_id=i.id),
  (SELECT e.trip_id FROM org_trip_held_events e WHERE e.org_id=i.org_id AND e.review_item_id=i.id),(SELECT s.trip_id FROM org_trip_held_submissions s WHERE s.org_id=i.org_id AND s.review_item_id=i.id),
  (SELECT h.trip_id FROM org_trip_held_transitions h WHERE h.org_id=i.org_id AND h.review_item_id=i.id))
 WHERE i.org_id=$1 AND i.id=$2 AND i.operation_type IN('trip.finish','trip.event.record','trip.submit','trip.pause','trip.resume','trip.cancel')) x`

// missionStructure reads the mission's immutable unit and asset.
const missionStructure = `SELECT unit_id::text,COALESCE(asset_id::text,'') FROM org_trip_missions WHERE org_id=$1 AND id=$2`

func (r *ResourceStructure) ResolveStructure(ctx context.Context, scope kernel.OrgScope, policy access.Policy, facts permissions.ResourceFacts, at time.Time) (permissions.ResourceFacts, error) {
	if r == nil || r.Pool == nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	var result permissions.ResourceFacts
	err := database.WithOrgTx(ctx, r.Pool, scope, func(tx pgx.Tx) error {
		var err error
		result, err = r.resolveTx(ctx, tx, scope, policy, facts, at)
		return err
	})
	switch {
	case err == nil:
		return result, nil
	case ctx.Err() != nil:
		return permissions.ResourceFacts{}, ctx.Err()
	case errors.Is(err, errStructureNotFound):
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{}, access.ErrUnavailable
}

func (r *ResourceStructure) resolveTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, policy access.Policy, facts permissions.ResourceFacts, at time.Time) (permissions.ResourceFacts, error) {
	if r == nil || tx == nil || at.IsZero() || facts.Validate(scope) != nil || database.ValidateOrgTx(ctx, tx, scope) != nil {
		return permissions.ResourceFacts{}, org.ErrUnavailable
	}
	out := facts
	out.UnitID, out.CostCentreID, out.AssetID, out.Teams = "", "", "", nil
	// An organization-level resource (list, root selector) carries no structure.
	if facts.ID == scope.OrgID().String() {
		return out, nil
	}
	switch {
	// EMP-METER-004 (EDR-188): meter decisions cover the reading's asset
	// through its current allocation, as every register action does.
	case r.Kind == AssetStructure && (policy.Permission == "fleet.asset.read" || policy.Permission == "fleet.asset.manage" || policy.Permission == "fleet.asset.assign" || policy.Permission == "fleet.reservation.manage" || policy.Permission == "meter.reading.correct"):
		if meterMissing(r.Allocations) {
			return permissions.ResourceFacts{}, org.ErrUnavailable
		}
		snapshot, err := r.Allocations.UnitAndCostCentreAtTx(ctx, tx, scope, facts.ID, at)
		switch {
		case errors.Is(err, fleet.ErrUnallocated):
			out.AssetID = facts.ID
		case errors.Is(err, fleet.ErrNotFound):
			return permissions.ResourceFacts{}, errStructureNotFound
		case err != nil:
			return permissions.ResourceFacts{}, org.ErrUnavailable
		default:
			if snapshot.AssetID != facts.ID {
				return permissions.ResourceFacts{}, org.ErrUnavailable
			}
			out.AssetID, out.UnitID, out.CostCentreID = facts.ID, snapshot.UnitID, snapshot.CostCentreID
		}
	case r.Kind == HeldMeterReadingStructure && policy.Permission == "meter.reading.correct":
		var teams []string
		if err := tx.QueryRow(ctx, heldReadingStructure, scope.OrgID().String(), facts.ID).Scan(&out.AssetID, &out.UnitID, &teams); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return permissions.ResourceFacts{}, errStructureNotFound
			}
			return permissions.ResourceFacts{}, org.ErrUnavailable
		}
		slices.Sort(teams)
		out.Teams = teams
	case r.Kind == MissionStructure && policy.Permission == "trip.mission.manage":
		if err := tx.QueryRow(ctx, missionStructure, scope.OrgID().String(), facts.ID).Scan(&out.UnitID, &out.AssetID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return permissions.ResourceFacts{}, errStructureNotFound
			}
			return permissions.ResourceFacts{}, org.ErrUnavailable
		}
	case r.Kind == IncidentStructure && policy.Permission == "check.incident.manage":
		if err := tx.QueryRow(ctx, incidentStructure, scope.OrgID().String(), facts.ID).Scan(&out.AssetID, &out.UnitID, &out.CostCentreID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return permissions.ResourceFacts{}, errStructureNotFound
			}
			return permissions.ResourceFacts{}, org.ErrUnavailable
		}
	// EMP-TRIP-007 (EDR-263): a held trip fact under trip.trip.review.
	case r.Kind == HeldTripStructure && policy.Permission == "trip.trip.review":
		var teams []string
		if err := tx.QueryRow(ctx, heldTripStructure, scope.OrgID().String(), facts.ID).Scan(&out.AssetID, &out.UnitID, &out.CostCentreID, &teams); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return permissions.ResourceFacts{}, errStructureNotFound
			}
			return permissions.ResourceFacts{}, org.ErrUnavailable
		}
		slices.Sort(teams)
		out.Teams = teams
	// EMP-TRIP-015 (EDR-236): an amendment corrects the trip's readings under
	// meter.reading.correct over the same stored structure.
	// EMP-TRIP-014 (EDR-259): a closure covers the trip through the same
	// stored structure; EMP-GPS-012 (EDR-281): a recorded-track read uses the
	// same snapshot.
	case r.Kind == TripStructure && (policy.Permission == "trip.trip.review" || policy.Permission == "meter.reading.correct" || policy.Permission == "trip.trip.close_for_other" || policy.Permission == "gps.track.read"):
		var teams []string
		if err := tx.QueryRow(ctx, tripStructure, scope.OrgID().String(), facts.ID).Scan(&out.AssetID, &out.UnitID, &out.CostCentreID, &teams); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return permissions.ResourceFacts{}, errStructureNotFound
			}
			return permissions.ResourceFacts{}, org.ErrUnavailable
		}
		slices.Sort(teams)
		out.Teams = teams
	}
	if out.Validate(scope) != nil {
		return permissions.ResourceFacts{}, org.ErrUnavailable
	}
	return out, nil
}

// structureTx adds the structural facts to an in-transaction re-check only
// when the member holds a current structural grant for the permission.
func (r *ResourceStructure) structureTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string, assignments []permissions.Assignment, policy access.Policy, facts permissions.ResourceFacts, at time.Time) (permissions.ResourceFacts, error) {
	if r == nil {
		return facts, nil
	}
	structural, err := permissions.StructuralCandidate(scope, member, assignments, policy.Permission, at)
	if err != nil {
		return permissions.ResourceFacts{}, org.ErrUnavailable
	}
	if !structural {
		return facts, nil
	}
	resolved, err := r.resolveTx(ctx, tx, scope, policy, facts, at)
	if errors.Is(err, errStructureNotFound) {
		return permissions.ResourceFacts{}, org.ErrNotFound
	}
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	merged, err := facts.WithStructure(scope, resolved)
	if err != nil {
		return permissions.ResourceFacts{}, org.ErrUnavailable
	}
	return merged, nil
}
