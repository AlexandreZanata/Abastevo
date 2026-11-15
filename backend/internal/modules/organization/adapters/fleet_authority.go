package adapters

import (
	"context"
	"errors"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

// FleetAuthority supplies current company proof through the fleet-owned port.
// The only admitted policies are the fixed fleet actions. The only assignee
// fact is the EMP-FLEET-006 one (Assignee, or the member's own assignment
// list). The retained transaction owns every proof/root wait.
type FleetAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	// Structure resolves an asset's current allocation for structural grants
	// (EMP-AUTH-006, AssetStructure); nil keeps them uncovered.
	Structure *ResourceStructure
	// Assignee resolves whether an asset is assigned to the member for SELF
	// grants (EMP-FLEET-006); nil keeps SELF uncovered on assets.
	Assignee *ResourceAssignee
}

var _ fleet.FleetAuthority = (*FleetAuthority)(nil)

func (g *FleetAuthority) CheckFleetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor fleet.FleetActor, member, target string, action fleet.FleetAction) error {
	if g == nil || tx == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return fleet.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.Structure, Assignee: g.Assignee}
	var policy access.Policy
	switch action {
	case fleet.FleetRead, fleet.FleetManage:
		policy = fleet.Policy(action == fleet.FleetManage)
	case fleet.FleetAssign:
		policy = fleet.AssignPolicy()
	case fleet.FleetReserve:
		policy = fleet.ReservePolicy()
	case fleet.FleetReadOwnAssignments:
		// The member's own assignment list: no structure or asset lookup.
		if target != member {
			return fleet.ErrUnavailable
		}
		policy = fleet.Policy(false)
		guard.Structure, guard.Assignee, guard.OwnTarget = nil, nil, true
	default:
		return fleet.ErrUnavailable
	}
	if policy.Validate() != nil {
		return fleet.ErrUnavailable
	}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy)
	switch {
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated):
		return fleet.ErrUnauthenticated
	case errors.Is(err, org.ErrUnavailable):
		return fleet.ErrUnavailable
	case errors.Is(err, org.ErrNotFound):
		return fleet.ErrNotFound
	}
	return err
}
