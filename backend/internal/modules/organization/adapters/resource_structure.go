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
	case r.Kind == AssetStructure && (policy.Permission == "fleet.asset.read" || policy.Permission == "fleet.asset.manage" || policy.Permission == "fleet.asset.assign" || policy.Permission == "fleet.reservation.manage"):
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
