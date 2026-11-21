package adapters

import (
	"context"

	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/jackc/pgx/v5"
)

type FuelingReviewAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

func (g FuelingReviewAuthority) CheckFuelingReviewTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope,
	actor organization.CompanyActor, member, target string, write bool,
) error {
	if tx == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil ||
		g.Structure == nil || g.Structure.Kind != FuelingReviewStructure || actor.Validate() != nil ||
		legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return fuel.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.Structure}
	if err := guard.check(ctx, tx, scope, actor, member, target, fuel.ReviewPolicy(write)); err != nil {
		return err
	}
	if write {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
			return err
		}
		if state != "ACTIVE" {
			return access.ErrSetupIncomplete
		}
	}
	return nil
}
