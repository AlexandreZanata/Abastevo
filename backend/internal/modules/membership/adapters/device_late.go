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
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// CheckLateIntakeDevice proves, in its own scoped transaction, the intake-only
// late source of one exact owned enrollment period (EMP-OFFL-009).
func (s *DeviceStore) CheckLateIntakeDevice(ctx context.Context, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.LateIntakeDevice, error) {
	if !s.ready() {
		return app.LateIntakeDevice{}, app.ErrRegisterUnavailable
	}
	var out app.LateIntakeDevice
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var e error
		out, _, e = s.CheckLateIntakeDeviceDBTX(ctx, tx, scope, a, member, enrollment)
		return e
	})
	if e != nil {
		return app.LateIntakeDevice{}, deviceError(ctx, e)
	}
	return out, nil
}

// CheckLateIntakeDeviceTx is the same proof inside the caller's intake
// transaction; call it again after waits and before commit.
func (s *DeviceStore) CheckLateIntakeDeviceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.LateIntakeDevice, time.Time, error) {
	return s.CheckLateIntakeDeviceDBTX(ctx, tx, scope, a, member, enrollment)
}

// CheckLateIntakeDeviceDBTX keeps the normal intake lock order (account →
// personal proof → capability snapshot → root → exact period) and grants no
// membership, role, record or receipt authority. Only a period revoked
// ACCESS_ENDED whose key was never revoked LOST_OR_COMPROMISED in this
// organization is a late source (B-BR-EMP-302, EDR-199).
func (s *DeviceStore) CheckLateIntakeDeviceDBTX(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.LateIntakeDevice, time.Time, error) {
	if !s.ready() {
		return app.LateIntakeDevice{}, time.Time{}, app.ErrRegisterUnavailable
	}
	if scope.Validate() != nil || a.Validate() != nil || !validUUID(member) || !validUUID(enrollment) {
		return app.LateIntakeDevice{}, time.Time{}, d.ErrInvalid
	}
	if e := check(ctx, tx, scope); e != nil {
		return app.LateIntakeDevice{}, time.Time{}, e
	}
	out, e := s.lateIntake(ctx, tx, scope, a, member, enrollment)
	if e != nil {
		return app.LateIntakeDevice{}, time.Time{}, deviceError(ctx, e)
	}
	// Recheck clock-bound personal proof after every wait under retained locks.
	now, e := contactProof(ctx, tx, a)
	if e != nil {
		return app.LateIntakeDevice{}, time.Time{}, deviceError(ctx, e)
	}
	if now.Before(out.EndedAt) {
		return app.LateIntakeDevice{}, time.Time{}, app.ErrRegisterUnavailable
	}
	return out, now, nil
}

func (s *DeviceStore) lateIntake(ctx context.Context, tx database.OrgDBTX, scope kernel.OrgScope, a app.DeviceActor, member, enrollment string) (app.LateIntakeDevice, error) {
	queries := q.New(tx)
	owner, e := queries.DeviceAccount(ctx, a.AccountID)
	if e != nil {
		return app.LateIntakeDevice{}, e
	}
	if owner.Status != "active" || owner.DeletionPending {
		return app.LateIntakeDevice{}, app.ErrEmployeeUnauthenticated
	}
	if owner.ManagedOrg != "" {
		return app.LateIntakeDevice{}, app.ErrManagedRestricted
	}
	if _, e := contactProof(ctx, tx, a); e != nil {
		return app.LateIntakeDevice{}, e
	}
	capabilities, ok := s.Capabilities.(deviceSnapshotCapabilities)
	if !ok {
		return app.LateIntakeDevice{}, app.ErrRegisterUnavailable
	}
	// EDR-108: the capability snapshot is locked before the root. EMP-OFFL-012
	// owns disabled-capability disposition; until then late intake needs it.
	state, e := capabilities.LockSnapshotDBTX(ctx, tx, scope, "organizations")
	if e != nil {
		return app.LateIntakeDevice{}, e
	}
	if !state.Enabled {
		return app.LateIntakeDevice{}, access.ErrEntitlementRequired
	}
	var root string
	var ready, synthetic bool
	e = tx.QueryRow(ctx, `SELECT COALESCE(state,''),has_snapshot,EXISTS(SELECT 1 FROM org_onboarding_commands c WHERE c.org_id=organizations.org_id AND c.kind='CREATED' AND c.terms_kind='SYNTHETIC' AND c.terms_version=$2 AND c.terms_digest=$3) FROM organizations WHERE org_id=$1 FOR UPDATE`, scope.OrgID().String(), org.SyntheticTermsVersion, org.SyntheticTermsDigest).Scan(&root, &ready, &synthetic)
	if e != nil {
		return app.LateIntakeDevice{}, e
	}
	if !ready || !synthetic {
		return app.LateIntakeDevice{}, app.ErrRegisterUnavailable
	}
	switch org.State(root) {
	case org.Active:
	case org.PendingSetup:
		return app.LateIntakeDevice{}, access.ErrSetupIncomplete
	case org.Suspended:
		return app.LateIntakeDevice{}, access.ErrOrgSuspended
	case org.Archived:
		return app.LateIntakeDevice{}, access.ErrOrgArchived
	default:
		return app.LateIntakeDevice{}, app.ErrEmployeeNotFound
	}
	row, e := queries.LateIntakeEnrollment(ctx, q.LateIntakeEnrollmentParams{OrgID: scope.OrgID().String(), EnrollmentID: enrollment, MembershipID: member, AccountID: a.AccountID, KeyID: a.KeyID, ContributorID: a.ContributorID, Fingerprint: a.Fingerprint})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.LateIntakeDevice{}, app.ErrEmployeeNotFound
	}
	if e != nil {
		return app.LateIntakeDevice{}, e
	}
	if row.Barred || row.RevocationClass == string(d.LostOrCompromised) {
		return app.LateIntakeDevice{}, app.ErrDeviceRevoked
	}
	// A still-enrolled period is current, never late.
	if row.State != string(d.DeviceRevoked) || row.RevocationClass != string(d.AccessEnded) {
		return app.LateIntakeDevice{}, app.ErrEmployeeNotFound
	}
	enrolled, e := deviceStamp(row.EnrolledAt)
	if e != nil {
		return app.LateIntakeDevice{}, e
	}
	revoked, e := deviceStamp(row.RevokedAt)
	if e != nil {
		return app.LateIntakeDevice{}, e
	}
	out := app.LateIntakeDevice{End: app.LateEndDevice, EndedAt: revoked}
	switch row.SourceType {
	case "COMMAND":
	case "MEMBERSHIP":
		ended, e := deviceStamp(row.EndedAt)
		if e != nil || row.MembershipState != string(d.MembershipEnded) || ended.After(revoked) {
			return app.LateIntakeDevice{}, app.ErrRegisterUnavailable
		}
		out = app.LateIntakeDevice{End: app.LateEndMembership, EndedAt: ended}
	default:
		return app.LateIntakeDevice{}, app.ErrRegisterUnavailable
	}
	if out.EndedAt.Before(enrolled) || out.Validate() != nil {
		return app.LateIntakeDevice{}, app.ErrRegisterUnavailable
	}
	return out, nil
}
