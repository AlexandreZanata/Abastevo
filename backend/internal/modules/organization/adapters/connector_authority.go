package adapters

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	connector "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/jackc/pgx/v5"
)

type ConnectorAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (a ConnectorAuthority) CheckConnectionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor organization.CompanyActor, member string, action connector.Action) error {
	policy := connector.Policy(action)
	if tx == nil || a.Memberships == nil || a.Roles == nil || a.Capabilities == nil ||
		actor.Validate() != nil || legalIdentity(scope, member) != nil || policy.Validate() != nil {
		return connector.ErrUnavailable
	}
	return (companyAuthority{Memberships: a.Memberships, Roles: a.Roles, Capabilities: a.Capabilities}).check(
		ctx, tx, scope, actor, member, scope.OrgID().String(), policy,
	)
}
