package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// CheckCurrentDeviceTx retains normal current proof and the exact enrolled
// period in the caller's transaction. It cannot issue custody leases or admit
// terminal/late intake. Call again after downstream waits and before commit.
func (s *DeviceStore) CheckCurrentDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.CheckCurrentDeviceDBTX(ctx, tx, scope, a, member, enrollment)
}

// CheckCurrentIntakeDeviceTx is the framework source check; it preserves normal
// current/read proof while locking the actual capability snapshot before root.
func (s *DeviceStore) CheckCurrentIntakeDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.CheckCurrentIntakeDeviceDBTX(ctx, tx, scope, a, member, enrollment)
}
func (s *DeviceStore) CheckCurrentIntakeDeviceDBTX(ctx context.Context, queries database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.checkCurrentDeviceDBTX(ctx, queries, scope, a, member, enrollment, false, true)
}

func (s *DeviceStore) CheckCurrentDeviceDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.checkCurrentDeviceDBTX(ctx, tx, scope, a, member, enrollment, false, false)
}

// CheckCurrentOperationalDeviceDBTX is reserved for current source-owned
// operational writes. It adds the actual base-capability write lock while
// keeping normal enrolled proof, not enrollment freshness, under borrowed IO.
func (s *DeviceStore) CheckCurrentOperationalDeviceDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.checkCurrentDeviceDBTX(ctx, tx, scope, a, member, enrollment, true, false)
}

type deviceQueryCapabilities interface {
	GuardWriteDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) error
}
type deviceSnapshotCapabilities interface {
	LockSnapshotDBTX(context.Context, database.OrgDBTX, kernel.OrgScope, string) (capability.State, error)
}

func (s *DeviceStore) checkCurrentDeviceDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string, operationalWrite, snapshot bool) (app.DeviceRecord, time.Time, error) {
	if !s.ready() {
		return app.DeviceRecord{}, time.Time{}, app.ErrRegisterUnavailable
	}
	if scope.Validate() != nil || a.Validate() != nil || !validUUID(member) || !validUUID(enrollment) {
		return app.DeviceRecord{}, time.Time{}, d.ErrInvalid
	}
	if e := check(ctx, tx, scope); e != nil {
		return app.DeviceRecord{}, time.Time{}, e
	}
	var guard func() error
	if operationalWrite {
		capabilities, ok := s.Capabilities.(deviceQueryCapabilities)
		if !ok {
			return app.DeviceRecord{}, time.Time{}, app.ErrRegisterUnavailable
		}
		guard = func() error {
			err := capabilities.GuardWriteDBTX(ctx, tx, scope, "organizations")
			if errors.Is(err, capability.ErrEntitlementRequired) {
				return access.ErrEntitlementRequired
			}
			return err
		}
	} else if snapshot {
		capabilities, ok := s.Capabilities.(deviceSnapshotCapabilities)
		if !ok {
			return app.DeviceRecord{}, time.Time{}, app.ErrRegisterUnavailable
		}
		guard = func() error { _, err := capabilities.LockSnapshotDBTX(ctx, tx, scope, "organizations"); return err }
	}
	current, e := s.signedFor(ctx, tx, scope, a, false, true, operationalWrite, guard)
	if e != nil {
		return app.DeviceRecord{}, time.Time{}, deviceError(ctx, e)
	}
	m, e := current.Snapshot(scope)
	if e != nil || m.ID != member {
		return app.DeviceRecord{}, time.Time{}, app.ErrEmployeeNotFound
	}
	row, e := q.New(tx).OwnedDeviceContact(ctx, q.OwnedDeviceContactParams{OrgID: scope.OrgID().String(), EnrollmentID: enrollment, MembershipID: member, AccountID: a.AccountID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint})
	if e != nil {
		return app.DeviceRecord{}, time.Time{}, deviceError(ctx, e)
	}
	v, e := deviceRecord(scope, q.GetDeviceRow(row))
	if e != nil {
		return app.DeviceRecord{}, time.Time{}, e
	}
	state, e := v.Enrollment.Snapshot(scope)
	if e != nil || state.State != d.DeviceEnrolled {
		return app.DeviceRecord{}, time.Time{}, app.ErrDeviceRevoked
	}
	now, e := contactProof(ctx, tx, a)
	if e != nil {
		return app.DeviceRecord{}, time.Time{}, deviceError(ctx, e)
	}
	return v, now, nil
}
