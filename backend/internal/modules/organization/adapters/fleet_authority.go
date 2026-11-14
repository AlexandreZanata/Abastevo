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
// The only admitted policies are the two fixed fleet actions. Resource facts
// contain no invented assignment: SELF remains unproven until its real owner
// integrates that source. The retained transaction owns every proof/root wait.
type FleetAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ fleet.FleetAuthority = (*FleetAuthority)(nil)

func (g *FleetAuthority) CheckFleetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor fleet.FleetActor, member, target string, action fleet.FleetAction) error {
	if g == nil || tx == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return fleet.ErrUnavailable
	}
	if action != fleet.FleetRead && action != fleet.FleetManage {
		return fleet.ErrUnavailable
	}
	policy := fleet.Policy(action == fleet.FleetManage)
	if policy.Validate() != nil {
		return fleet.ErrUnavailable
	}
	err := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy)
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
