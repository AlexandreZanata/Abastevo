package adapters

import (
	"context"
	"errors"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// TripHandoverAuthority proves the EMP-TRIP-012 handover policies with the
// trip review company proof (EDR-255): trip.trip.close_for_other over the
// stored trip through TripStructure (a write: fresh proof and ACTIVE
// organization) or over the organization for the handover list, and
// trip.trip.start over the requesting membership for its own open trips. It
// also answers whether a receiver is eligible: current trip.trip.start
// covering the trip as the receiver's own record and a FLEET-006 assignment
// of the trip's asset at the database instant.
type TripHandoverAuthority struct {
	Review      *TripReviewAuthority
	Assignments fleet.AssignedAssetsPort
}

var _ trippg.HandoverAuthority = (*TripHandoverAuthority)(nil)

func (g *TripHandoverAuthority) CheckTripHandoverTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.ReviewActor, member, target string, action tripapp.HandoverAction) (tripapp.Reviewer, error) {
	if g == nil || g.Review == nil {
		return tripapp.Reviewer{}, tripapp.ErrUnavailable
	}
	policy := tripapp.HandoverPolicy(action)
	switch action {
	case tripapp.HandoverWriteAction:
		return g.Review.check(ctx, tx, scope, actor, member, target, policy, true, TripStructure)
	case tripapp.HandoverReadAction:
		if target != scope.OrgID().String() {
			return tripapp.Reviewer{}, tripapp.ErrUnavailable
		}
		return g.Review.check(ctx, tx, scope, actor, member, target, policy, false, TripStructure)
	case tripapp.OwnTripsAction:
		if target != member {
			return tripapp.Reviewer{}, tripapp.ErrUnavailable
		}
		return g.own(ctx, tx, scope, actor, member, policy)
	}
	return tripapp.Reviewer{}, tripapp.ErrUnavailable
}

// own proves trip.trip.start over the requesting membership itself (the
// EMP-TRIP-008 own-list pattern) and returns the member.
func (g *TripHandoverAuthority) own(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor tripapp.ReviewActor, member string, policy access.Policy) (tripapp.Reviewer, error) {
	r := g.Review
	if meterMissing(tx) || meterMissing(r.Memberships) || meterMissing(r.Roles) || meterMissing(r.Capabilities) {
		return tripapp.Reviewer{}, tripapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return tripapp.Reviewer{}, tripapp.ErrReviewUnauthorized
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return tripapp.Reviewer{}, tripapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: r.Memberships, Roles: r.Roles, Capabilities: r.Capabilities, OwnTarget: true}
	proof := org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}
	if err := guard.check(ctx, tx, scope, proof, member, member, policy); err != nil {
		return tripapp.Reviewer{}, tripReviewAuthorityError(err)
	}
	current, err := r.Memberships.ActiveByAccountTx(ctx, tx, scope, actor.AccountID)
	if err != nil {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	membership, err := current.Snapshot(scope)
	if err != nil || membership.ID != member {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	employee, err := r.Memberships.GetEmployeeTx(ctx, tx, scope, membership.EmployeeID)
	if err != nil {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	person, err := employee.Snapshot(scope)
	if err != nil || person.State != memberdomain.EmployeeActive || person.MergedInto != "" {
		return tripapp.Reviewer{}, access.ErrNoMembership
	}
	if err := guard.check(ctx, tx, scope, proof, member, member, policy); err != nil {
		return tripapp.Reviewer{}, tripReviewAuthorityError(err)
	}
	return tripapp.Reviewer{AccountID: actor.AccountID, MembershipID: membership.ID, EmployeeID: person.ID}, nil
}

// EligibleReceiverTx answers whether the receiving membership currently
// holds trip.trip.start covering the trip as its own record and is assigned
// to the trip's asset at the instant (B-BR-EMP-394). A denial is false, never
// an error with detail about the person.
func (g *TripHandoverAuthority) EligibleReceiverTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, receiver string, target tripapp.Target, at time.Time) (bool, error) {
	if g == nil || g.Review == nil || meterMissing(tx) || meterMissing(g.Review.Roles) || meterMissing(g.Assignments) || at.IsZero() || legalIdentity(scope, receiver) != nil ||
		legalIdentity(scope, target.TripID) != nil || legalIdentity(scope, target.AssetID) != nil || database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return false, tripapp.ErrUnavailable
	}
	for _, id := range []string{target.UnitID, target.CostCentreID} {
		if id != "" && legalIdentity(scope, id) != nil {
			return false, tripapp.ErrUnavailable
		}
	}
	assignments, err := g.Review.Roles.CandidatesTx(ctx, tx, scope, receiver)
	if err != nil {
		return false, tripapp.ErrUnavailable
	}
	resource := permission.ResourceFacts{OrgID: scope.OrgID(), ID: target.TripID, AuthoredBy: receiver, AssetID: target.AssetID, UnitID: target.UnitID, CostCentreID: target.CostCentreID}
	if _, err := permission.CoveringRules(scope, receiver, assignments, "trip.trip.start", resource, at); err != nil {
		if errors.Is(err, permission.ErrPermissionDenied) {
			return false, nil
		}
		return false, tripapp.ErrUnavailable
	}
	assigned, err := g.Assignments.AssignedAtDBTX(ctx, tx, scope, receiver, target.AssetID, at)
	if err != nil {
		return false, tripapp.ErrUnavailable
	}
	return assigned, nil
}
