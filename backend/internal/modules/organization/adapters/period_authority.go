package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// PeriodAuthority supplies current company proof through the period port.
// Periods are organization-wide (EDR-148), so the only target is the
// organization itself: a unit- or SELF-scoped grant never covers it. Writes
// also require an ACTIVE root. The retained transaction owns every wait.
type PeriodAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ period.PeriodAuthority = (*PeriodAuthority)(nil)

func (g *PeriodAuthority) CheckPeriodTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor period.PeriodActor, member string, action period.PeriodAction) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) {
		return period.ErrUnavailable
	}
	policy := period.PeriodPolicy(action)
	if policy.Validate() != nil {
		return period.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return period.ErrUnauthenticated
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return period.ErrUnavailable
	}
	err := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), policy)
	switch {
	case err == nil:
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated):
		return period.ErrUnauthenticated
	case errors.Is(err, org.ErrUnavailable):
		return period.ErrUnavailable
	case errors.Is(err, org.ErrNotFound):
		return period.ErrNotFound
	default:
		return err
	}
	if action == period.PeriodRead {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return period.ErrUnavailable
	}
	if state != "ACTIVE" {
		return access.ErrSetupIncomplete
	}
	return nil
}
