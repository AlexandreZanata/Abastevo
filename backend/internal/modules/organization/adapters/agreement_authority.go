package adapters

import (
	"context"
	"errors"

	agreeapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// AgreementAuthority proves in-transaction company session, capability,
// membership and role authorization for the Phase F7 station agreement and
// fueling authorization endpoints (agree.agreement.manage,
// agree.authorization.issue).
type AgreementAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

// CheckAgreementTx proves the actor for one agreement policy at the given
// target (the organization root, an agreement or an authorization).
func (g *AgreementAuthority) CheckAgreementTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor agreeapp.Actor, member, target string, policy access.Policy) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || scope.Validate() != nil {
		return agreeapp.ErrUnavailable
	}
	if actor.AccountID == "" || actor.SessionID == "" || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return agreeapp.ErrUnauthenticated
	}
	if policy.Validate() != nil {
		return agreeapp.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return agreeapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return agreeapp.ErrUnauthenticated
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return agreeapp.ErrNotFound
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return agreeapp.ErrUnavailable
	default:
		return err
	}
}
