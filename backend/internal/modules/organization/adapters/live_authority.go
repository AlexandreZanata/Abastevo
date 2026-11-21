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
	trackingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func mapTrackingAuthorityErr(err error) error {
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

// LiveAuthority proves gps.location.read_live in-transaction with current
// company proof and scoped roles (EMP-GPS-007).
type LiveAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

var _ trackingpg.LiveAuthority = (*LiveAuthority)(nil)

func (g *LiveAuthority) CheckLiveTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor trackingapp.LiveActor, member, target, unitFilter string) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || g.Structure == nil || g.Structure.Kind != TripStructure {
		return trackingapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return trackingapp.ErrTrackUnauthorized
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return trackingapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.Structure, LiveTrip: true}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, trackingapp.LivePolicy())
	return mapTrackingAuthorityErr(err)
}

// GeofenceAuthority proves gps.geofence.manage in-transaction (EMP-GPS-013).
type GeofenceAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Structure    *ResourceStructure
}

var _ trackingpg.GeofenceAuthority = (*GeofenceAuthority)(nil)

func (g *GeofenceAuthority) CheckGeofenceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor trackingapp.GeofenceActor, member, target string, write bool) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || g.Structure == nil || g.Structure.Kind != GeofenceStructure {
		return trackingapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, target) != nil {
		return trackingapp.ErrTrackUnauthorized
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return trackingapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities, Structure: g.Structure}
	err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, target, trackingapp.GeofencePolicy(write))
	return mapTrackingAuthorityErr(err)
}

// RoutePlanAuthority proves route.plan.manage in-transaction and checks
// active root + owner existence (EMP-ROUTE-002).
type RoutePlanAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
}

var _ trackingpg.RoutePlanAuthority = (*RoutePlanAuthority)(nil)

func (g *RoutePlanAuthority) CheckRoutePlanTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor trackingapp.RouteActor, member string, kind trackingdomain.RouteOwnerKind, ownerID string, write bool) error {
	if g == nil || meterMissing(tx) || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) {
		return trackingapp.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, ownerID) != nil {
		return trackingapp.ErrTrackUnauthorized
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return trackingapp.ErrUnavailable
	}
	guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
	if err := guard.check(ctx, tx, scope, org.CompanyActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, member, scope.OrgID().String(), trackingapp.RoutePlanPolicy(write)); err != nil {
		return mapTrackingAuthorityErr(err)
	}
	var exists bool
	if kind == trackingdomain.RouteOwnerMission {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_trip_missions WHERE org_id=$1 AND id=$2)`, scope.OrgID().String(), ownerID).Scan(&exists); err != nil {
			return trackingapp.ErrUnavailable
		}
	} else {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_trips WHERE org_id=$1 AND id=$2)`, scope.OrgID().String(), ownerID).Scan(&exists); err != nil {
			return trackingapp.ErrUnavailable
		}
	}
	if !exists {
		return trackingapp.ErrNotFound
	}
	return nil
}
