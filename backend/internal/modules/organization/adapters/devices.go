package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	auth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

type DeviceAuthority struct{ OffboardingAuthority }

var _ members.DeviceAuthority = (*DeviceAuthority)(nil)

func (g *DeviceAuthority) CheckManagerDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member string, write bool) error {
	if g == nil || g.Memberships == nil || g.Roles == nil || g.Capabilities == nil || actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return members.ErrRegisterUnavailable
	}
	e := (companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}).check(ctx, tx, scope, app.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), members.DevicePolicy(write))
	switch {
	case errors.Is(e, app.ErrUnauthenticated):
		return members.ErrEmployeeUnauthenticated
	case errors.Is(e, app.ErrUnavailable):
		return members.ErrRegisterUnavailable
	case errors.Is(e, app.ErrNotFound):
		return members.ErrEmployeeNotFound
	case errors.Is(e, cap.ErrEntitlementRequired):
		return auth.ErrEntitlementRequired
	}
	return e
}

func (g *DeviceAuthority) CheckOwnDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor members.EmployeeActor, member string, write bool) error {
	if g == nil {
		return members.ErrRegisterUnavailable
	}
	return g.CheckOffboardingTx(ctx, tx, scope, actor, member, true, write)
}
