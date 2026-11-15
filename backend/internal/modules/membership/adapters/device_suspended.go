package adapters

import (
	"context"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// CheckSuspendedIntakeDevice proves, in its own transaction, the current
// member's exact enrolled device while the organization may be SUSPENDED, for
// the EMP-OFFL-012 state assessment only (EDR-200). Every other proof, root
// and membership check is the current one; it grants no owner authority.
func (s *DeviceStore) CheckSuspendedIntakeDevice(ctx context.Context, scope kernel.OrgScope, a app.DeviceActor) (app.DeviceRecord, error) {
	if !s.ready() {
		return app.DeviceRecord{}, app.ErrRegisterUnavailable
	}
	var out app.DeviceRecord
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		capabilities, ok := s.Capabilities.(deviceSnapshotCapabilities)
		if !ok {
			return app.ErrRegisterUnavailable
		}
		guard := func() error { _, err := capabilities.LockSnapshotDBTX(ctx, tx, scope, "organizations"); return err }
		member, e := s.signedForState(ctx, tx, scope, a, false, true, false, true, guard)
		if e != nil {
			return e
		}
		membership, e := member.Snapshot(scope)
		if e != nil {
			return e
		}
		row, e := q.New(tx).DeviceByKey(ctx, q.DeviceByKeyParams{OrgID: scope.OrgID().String(), KeyID: a.KeyID, MembershipID: membership.ID})
		if e != nil {
			return e
		}
		out, e = deviceRecord(scope, q.GetDeviceRow(row))
		return e
	})
	if e != nil {
		return app.DeviceRecord{}, deviceError(ctx, e)
	}
	return out, nil
}

// CheckSuspendedIntakeDeviceTx is the in-transaction form for the intake source.
func (s *DeviceStore) CheckSuspendedIntakeDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.CheckSuspendedIntakeDeviceDBTX(ctx, tx, scope, a, member, enrollment)
}

func (s *DeviceStore) CheckSuspendedIntakeDeviceDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.DeviceRecord, time.Time, error) {
	return s.checkCurrentDeviceDBTX(ctx, tx, scope, a, member, enrollment, false, true, true)
}
