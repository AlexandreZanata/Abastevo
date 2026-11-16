package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// TrackingPolicyAuthority supplies the current company proof of the tracking
// policy command and read (B-BR-EMP-467/469, EDR-302). The policy is
// organization-level, so both actions target the organization itself and a
// unit-, cost-centre-, asset-, team- or SELF-scoped grant never covers them;
// the change also requires an ACTIVE root.
type TrackingPolicyAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ trackingapp.PolicyAuthority = (*TrackingPolicyAuthority)(nil)

func (g *TrackingPolicyAuthority) CheckPolicyTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor trackingapp.PolicyActor, member string, action trackingapp.PolicyAction) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || scope.Validate() != nil {
		return trackingapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return trackingapp.ErrPolicyUnauthorized
	}
	policy := trackingapp.PolicySettingsPolicy(action)
	if policy.Validate() != nil {
		return trackingapp.ErrUnavailable
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return trackingapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), policy)
	switch {
	case err == nil:
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return trackingapp.ErrPolicyUnauthorized
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return trackingapp.ErrNotFound
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return trackingapp.ErrUnavailable
	default:
		return err
	}
	if action != trackingapp.PolicyManage {
		return nil
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
		return trackingapp.ErrUnavailable
	}
	if state != "ACTIVE" {
		return access.ErrSetupIncomplete
	}
	return nil
}
