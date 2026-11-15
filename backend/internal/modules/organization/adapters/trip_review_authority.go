package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// TripReviewAuthority proves `trip.trip.review` with the real current
// company proof and scoped roles over one stored trip (EMP-TRIP-004,
// EDR-222). Structural grants resolve through the TripStructure kind; the
// owning store compares the returned reviewer with the trip's author and
// driver (BR-REV-003).
type TripReviewAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

var (
	_ trippg.ReviewAuthority = (*TripReviewAuthority)(nil)
	_ trippg.AmendAuthority  = (*TripReviewAuthority)(nil)
)

func (g *TripReviewAuthority) CheckTripReviewTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.ReviewActor, member, target string, write bool) (tripapp.Reviewer, error) {
	policy := tripapp.ReviewPolicy(tripapp.ReviewReadAction)
	if write {
		policy = tripapp.ReviewPolicy(tripapp.ReviewDecideAction)
	}
	return g.check(ctx, tx, scope, actor, member, target, policy, write)
}

// CheckTripAmendTx proves meter.reading.correct over the stored trip with the
// same company proof, ACTIVE organization and structure, and returns the
// amendment's author (EMP-TRIP-015, EDR-236).
func (g *TripReviewAuthority) CheckTripAmendTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.ReviewActor, member, target string) (tripapp.Reviewer, error) {
	return g.check(ctx, tx, scope, actor, member, target, tripapp.AmendPolicy(), true)
}

func (g *TripReviewAuthority) check(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.ReviewActor, member, target string, policy access.Policy, write bool) (tripapp.Reviewer, error) {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || g.Structure != nil && g.Structure.Kind != TripStructure {
		return tripapp.Reviewer{}, tripapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return tripapp.Reviewer{}, tripapp.ErrReviewUnauthorized
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return tripapp.Reviewer{}, tripapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.Structure}
	proof := org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}
	if err := guard.check(ctx, tx, scope, proof, member, target, policy); err != nil {
		return tripapp.Reviewer{}, tripReviewAuthorityError(err)
	}
	if write {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&state); err != nil {
			return tripapp.Reviewer{}, tripapp.ErrUnavailable
		}
		if state != "ACTIVE" {
			return tripapp.Reviewer{}, access.ErrSetupIncomplete
		}
	}
	current, err := g.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if err != nil {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	membership, err := current.Snapshot(scope)
	if err != nil || membership.ID != member {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	employee, err := g.Memberships.GetEmployeeTx(ctx, tx, scope, membership.EmployeeID)
	if err != nil {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	person, err := employee.Snapshot(scope)
	if err != nil || person.State != memberdomain.EmployeeActive || person.MergedInto != "" {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	if err := guard.check(ctx, tx, scope, proof, member, target, policy); err != nil {
		return tripapp.Reviewer{}, tripReviewAuthorityError(err)
	}
	return tripapp.Reviewer{AccountID: actor.AccountID, MembershipID: membership.ID, EmployeeID: person.ID}, nil
}

// tripReviewAuthorityError keeps the shared access vocabulary the review
// route maps; unknown and setup causes stay explicit.
func tripReviewAuthorityError(err error) error {
	switch {
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return tripapp.ErrReviewUnauthorized
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return tripapp.ErrUnavailable
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return tripapp.ErrNotFound
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	}
	return err
}
