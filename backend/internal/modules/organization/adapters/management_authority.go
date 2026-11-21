package adapters

import (
	"context"
	"errors"

	intelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// ManagementAuthority proves in-transaction company session, capability,
// membership, and role authorization for F6 management, stock, maintenance,
// intelligence, findings, audit, and consolidation endpoints.
type ManagementAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

func (g *ManagementAuthority) CheckManagementTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor intelapp.Actor, member string, target string, policy access.Policy, ownTarget bool) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || scope.Validate() != nil {
		return intelapp.ErrUnavailable
	}
	if actor.AccountID == "" || actor.SessionID == "" || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return intelapp.ErrUnauthenticated
	}
	if policy.Validate() != nil {
		return intelapp.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return intelapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, OwnTarget: ownTarget}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, policy)
	switch {
	case err == nil:
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return intelapp.ErrUnauthenticated
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return intelapp.ErrNotFound
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return intelapp.ErrUnavailable
	default:
		return err
	}
	if policy.Effect == access.Read {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return intelapp.ErrUnavailable
	}
	if state != "ACTIVE" {
		return access.ErrSetupIncomplete
	}
	return nil
}
