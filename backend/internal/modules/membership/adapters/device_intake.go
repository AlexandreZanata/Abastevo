package adapters

import (
	"context"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5"
)

// CheckCurrentDeviceTx retains normal current proof and the exact enrolled
// period in the caller's transaction. It cannot issue custody leases or admit
// terminal/late intake. Call again after downstream waits and before commit.
func (s *DeviceStore) CheckCurrentDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	if !s.ready() {
		return app.DeviceRecord{}, time.Time{}, app.ErrRegisterUnavailable
	}
	if scope.Validate() != nil || a.Validate() != nil || !validUUID(member) || !validUUID(enrollment) {
		return app.DeviceRecord{}, time.Time{}, d.ErrInvalid
	}
	if e := check(ctx, tx, scope); e != nil {
		return app.DeviceRecord{}, time.Time{}, e
	}
	current, e := s.signed(ctx, tx, scope, a, false, true)
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
