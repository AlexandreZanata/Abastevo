package adapters

import (
	"context"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// MeterReviewAuthority reuses the real current company proof and scoped roles.
// The owning Store compares this actual reviewer with retained author identity.
type MeterReviewAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ fleet.MeterReviewAuthority = (*MeterReviewAuthority)(nil)

func (g *MeterReviewAuthority) CheckMeterReviewTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor fleet.FleetActor, member, target string, write bool) (fleet.MeterReviewReviewer, error) {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) {
		return fleet.MeterReviewReviewer{}, fleet.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return fleet.MeterReviewReviewer{}, fleet.ErrUnauthenticated
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return fleet.MeterReviewReviewer{}, fleet.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	proof := org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}
	if err := guard.check(ctx, tx, scope, proof, member, target, fleet.MeterReviewPolicy(write)); err != nil {
		return fleet.MeterReviewReviewer{}, meterAuthorityError(err)
	}
	if write {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
			return fleet.MeterReviewReviewer{}, fleet.ErrUnavailable
		}
		if state != "ACTIVE" {
			return fleet.MeterReviewReviewer{}, access.ErrSetupIncomplete
		}
	}
	current, err := g.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if err != nil {
		return fleet.MeterReviewReviewer{}, meterAuthorityError(err)
	}
	membership, err := current.Snapshot(scope)
	if err != nil || membership.ID != member {
		return fleet.MeterReviewReviewer{}, access.ErrNoMembership
	}
	employee, err := g.Memberships.GetEmployeeTx(ctx, tx, scope, membership.EmployeeID)
	if err != nil {
		return fleet.MeterReviewReviewer{}, meterAuthorityError(err)
	}
	person, err := employee.Snapshot(scope)
	if err != nil || person.State != memberdomain.EmployeeActive || person.MergedInto != "" {
		return fleet.MeterReviewReviewer{}, access.ErrNoMembership
	}
	if err := guard.check(ctx, tx, scope, proof, member, target, fleet.MeterReviewPolicy(write)); err != nil {
		return fleet.MeterReviewReviewer{}, meterAuthorityError(err)
	}
	return fleet.MeterReviewReviewer{AccountID: actor.AccountID, MembershipID: membership.ID, EmployeeID: person.ID}, nil
}
