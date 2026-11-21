package adapters

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/jackc/pgx/v5"
)

type SubscriptionAuthority struct {
	Memberships members.TransactionRepository
	Roles       access.TransactionRepository
}

func (g SubscriptionAuthority) CheckPlanReadTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor organization.CompanyActor, member string) error {
	if tx == nil || g.Memberships == nil || g.Roles == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return billing.ErrUnavailable
	}
	return (companyAuthority{Memberships: g.Memberships, Roles: g.Roles}).check(
		ctx, tx, scope, actor, member, scope.OrgID().String(), billing.ReadPlanPolicy(),
	)
}
