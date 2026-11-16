package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// TrackAuthority proves gps.track.read with the real current company proof
// and scoped roles over one stored trip inside the read transaction
// (EMP-GPS-012, EDR-281). The trip's author is its own record and its
// driver, the read is a recorded track, and structural grants resolve through
// the TripStructure kind; trip.trip.read never substitutes (BR-AUTH-011).
type TrackAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

var _ trackingpg.TrackAuthority = (*TrackAuthority)(nil)

func (g *TrackAuthority) CheckTrackTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor trackingapp.TrackActor, member, trip string) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || g.Structure == nil || g.Structure.Kind != TripStructure {
		return trackingapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, trip) != nil {
		return trackingapp.ErrTrackUnauthorized
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return trackingapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.Structure, TrackedTrip: true}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, trip, trackingapp.TrackPolicy())
	switch {
	case err == nil:
		return nil
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return trackingapp.ErrTrackUnauthorized
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return trackingapp.ErrNotFound
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return trackingapp.ErrUnavailable
	}
	return err
}
