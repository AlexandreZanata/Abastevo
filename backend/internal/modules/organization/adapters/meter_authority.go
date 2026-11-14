package adapters

import (
	"context"
	"errors"
	"reflect"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// MeterAuthority binds real source proof to the retained caller transaction.
// It never accepts an arbitrary permission or assumes a driver assignment.
type MeterAuthority struct {
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Devices      *memberpg.DeviceStore
}

var _ fleet.MeterAuthority = (*MeterAuthority)(nil)

func meterMissing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func, reflect.Map, reflect.Slice:
		return v.IsNil()
	}
	return false
}

type meterRoleQueries interface {
	CandidatesDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) ([]permission.Assignment, error)
}

func (g *MeterAuthority) currentMeterDevice(ctx context.Context, queries database.OrgDBTX, scope kernel.OrgScope, source syncapp.Source, member string, write bool) (time.Time, error) {
	if write {
		_, at, err := g.Devices.CheckCurrentOperationalDeviceDBTX(ctx, queries, scope, source.Actor, member, source.EnrollmentID)
		return at, meterAuthorityError(err)
	}
	_, at, err := g.Devices.CheckCurrentDeviceDBTX(ctx, queries, scope, source.Actor, member, source.EnrollmentID)
	return at, meterAuthorityError(err)
}

// meterTarget reads the real scoped identity and current unit, but supplies no
// assignment. AUTH-006 owns future unit/asset coverage; current SELF stays closed.
func meterTarget(ctx context.Context, queries database.OrgDBTX, scope kernel.OrgScope, asset string) error {
	var id, unit string
	if err := queries.QueryRow(ctx, `SELECT id::text,COALESCE(unit_id::text,'') FROM org_assets WHERE org_id=$1 AND id=$2`, scope.OrgID().String(), asset).Scan(&id, &unit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fleet.ErrNotFound
		}
		return fleet.ErrUnavailable
	}
	if id != asset || unit != "" && legalIdentity(scope, unit) != nil {
		return fleet.ErrUnavailable
	}
	return nil
}

func meterAuthorityError(err error) error {
	switch {
	case errors.Is(err, org.ErrUnauthenticated), errors.Is(err, members.ErrEmployeeUnauthenticated):
		return fleet.ErrUnauthenticated
	case errors.Is(err, org.ErrUnavailable), errors.Is(err, members.ErrRegisterUnavailable):
		return fleet.ErrUnavailable
	case errors.Is(err, org.ErrNotFound), errors.Is(err, members.ErrEmployeeNotFound):
		return fleet.ErrNotFound
	case errors.Is(err, capability.ErrEntitlementRequired):
		return access.ErrEntitlementRequired
	}
	return err
}

func (g *MeterAuthority) CheckMeterTx(ctx context.Context, tx syncapp.DBTX, scope kernel.OrgScope, source fleet.MeterSource, member, asset string, action fleet.MeterAction) error {
	if g == nil || meterMissing(g.Memberships) || meterMissing(g.Roles) || meterMissing(g.Capabilities) || meterMissing(tx) {
		return fleet.ErrUnavailable
	}
	if source.Validate(scope) != nil || legalIdentity(scope, member) != nil || legalIdentity(scope, asset) != nil {
		return fleet.ErrUnauthenticated
	}
	if action != fleet.MeterInstallAction && action != fleet.MeterRecordAction && action != fleet.MeterReadAction {
		return fleet.ErrUnavailable
	}
	if source.Web != nil {
		if action == fleet.MeterRecordAction {
			return fleet.ErrUnauthenticated
		}
		retained, ok := tx.(pgx.Tx)
		if !ok || meterMissing(retained) {
			return fleet.ErrUnavailable
		}
		if err := database.ValidateOrgDBTX(ctx, retained, scope); err != nil {
			return fleet.ErrUnavailable
		}
		actor := org.CompanyActor{AccountID: source.Web.AccountID, SessionID: source.Web.SessionID}
		guard := companyAuthority{Memberships: g.Memberships, Roles: g.Roles, Capabilities: g.Capabilities}
		if err := guard.check(ctx, retained, scope, actor, member, asset, fleet.MeterPolicy(action)); err != nil {
			return meterAuthorityError(err)
		}
		// The root is only an explicit malformed/schema admission selector. It
		// grants no record effect; actual repository commands revalidate IDs.
		if action != fleet.MeterReadAction || asset != scope.OrgID().String() {
			if err := meterTarget(ctx, retained, scope, asset); err != nil {
				return err
			}
		}
		return meterAuthorityError(guard.check(ctx, retained, scope, actor, member, asset, fleet.MeterPolicy(action)))
	}
	if action != fleet.MeterRecordAction && action != fleet.MeterReadAction || source.Device.MembershipID != member {
		return fleet.ErrUnauthenticated
	}
	if g.Devices == nil {
		return fleet.ErrUnavailable
	}
	roles, ok := g.Roles.(meterRoleQueries)
	if !ok {
		return fleet.ErrUnavailable
	}
	if err := database.ValidateOrgDBTX(ctx, tx, scope); err != nil {
		return fleet.ErrUnavailable
	}
	current := source.Device
	// Both checks operate on the exact borrowed port; the signed device path
	// never asserts or obtains a pgx.Tx, connection or lifecycle callback.
	if _, err := g.currentMeterDevice(ctx, tx, scope, *current, member, action == fleet.MeterRecordAction); err != nil {
		return err
	}
	rootFallback := asset == scope.OrgID().String()
	if !rootFallback {
		if err := meterTarget(ctx, tx, scope, asset); err != nil {
			return err
		}
	}
	assignments, err := roles.CandidatesDBTX(ctx, tx, scope, member)
	if err != nil {
		return err
	}
	if rootFallback {
		filtered := make([]permission.Assignment, 0, len(assignments))
		for _, a := range assignments {
			v, err := a.Snapshot(scope)
			if err != nil {
				return fleet.ErrUnavailable
			}
			if v.Scope == permission.OrgScope {
				filtered = append(filtered, a)
			}
		}
		assignments = filtered
	}
	// Fresh source time is sampled after target/role waits and reevaluates role
	// expiry. Repeating this check before commit remains the caller's contract.
	now, err := g.currentMeterDevice(ctx, tx, scope, *current, member, action == fleet.MeterRecordAction)
	if err != nil {
		return err
	}
	_, err = permission.CoveringRules(scope, member, assignments, fleet.MeterPolicy(action).Permission, permission.ResourceFacts{OrgID: scope.OrgID(), ID: asset}, now)
	return err
}
