package adapters

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/jackc/pgx/v5"
)

type DataKeyAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (g DataKeyAuthority) CheckKeyActionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor organization.CompanyActor, member string, action data.Action) error {
	policy := data.KeyPolicy(action)
	if tx == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil ||
		actor.Validate() != nil || legalIdentity(scope, member) != nil || policy.Validate() != nil {
		return data.ErrUnavailable
	}
	return (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(
		ctx, tx, scope, actor, member, scope.OrgID().String(), policy,
	)
}
