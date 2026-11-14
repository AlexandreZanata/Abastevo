//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestMeterIntakeCapabilityFlipDoesNotInvertRetainedSourceLocks(t *testing.T) {
	fx := meterIntakeHTTP(t)
	var beforeDeadlocks int64
	var deferred bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT deadlocks FROM pg_stat_database WHERE datname=current_database()`).Scan(&beforeDeadlocks); err != nil {
		t.Fatal(err)
	}
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT condeferrable AND condeferred FROM pg_constraint WHERE conname='org_audit_events_root_fk'`).Scan(&deferred); err != nil || !deferred {
		t.Fatal("actual audit root FK must be deferred", err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	meters.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	config := fx.config
	config.NoAsyncPlans = map[syncapp.OperationKey]map[string]syncapp.FollowupMode{{Type: fleetpg.MeterReadingOperationType, Schema: 1}: {protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}}
	confirmed := make(chan int, 1)
	release := make(chan struct{})
	var first atomic.Bool
	config.ConfirmSource = func(ctx context.Context, tx pgx.Tx, source syncapp.Source) (time.Time, error) {
		_, at, err := fx.devices.CheckCurrentIntakeDeviceTx(ctx, tx, source.Scope, source.Actor, source.MembershipID, source.EnrollmentID)
		if err != nil {
			return time.Time{}, syncapp.SourceDenied(err)
		}
		if first.CompareAndSwap(false, true) {
			var pid int
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return time.Time{}, err
			}
			confirmed <- pid
			select {
			case <-release:
			case <-ctx.Done():
				return time.Time{}, ctx.Err()
			}
		}
		return at, nil
	}
	store := syncpg.NewStore(config)
	op := fx.operation(t, "184190001", nil)
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Second)
	defer cancel()
	type answer struct {
		ack protocol.Acknowledgement
		err error
	}
	intake := make(chan answer, 1)
	go func() {
		ack, err := store.Process(ctx, fx.source, op, protocol.DefaultLimits(), handler)
		intake <- answer{ack, err}
	}()
	var pid int
	select {
	case pid = <-confirmed:
	case result := <-intake:
		t.Fatal("real intake failed before current source gate", result.err)
	case <-ctx.Done():
		t.Fatal("current source gate timed out")
	}
	flip := make(chan error, 1)
	flipID := onboardingID(t)
	go func() {
		_, err := fx.flags.Set(ctx, fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{OperationID: flipID, Capability: "organizations", Enabled: false, ExpectedRevision: 1})
		flip <- err
	}()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	observed, commitWait := false, false
	for !observed {
		if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*)>0,COALESCE(bool_or(lower(btrim(query))='commit'),false) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND $2=ANY(pg_blocking_pids(pid))`, fx.f.RuntimeRole, pid).Scan(&observed, &commitWait); err != nil {
			t.Fatal("lock observation failed")
		}
		if !observed {
			select {
			case err := <-flip:
				t.Fatal("capability flip did not contend with retained actual source", err)
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatal("no actual capability/source lock wait observed")
			}
		}
	}
	close(release)
	result := <-intake
	flipErr := <-flip
	var afterDeadlocks int64
	deadline := time.Now().Add(time.Second)
	for {
		if err := fx.f.Admin.QueryRow(ctx, `SELECT deadlocks FROM pg_stat_database WHERE datname=current_database()`).Scan(&afterDeadlocks); err != nil {
			t.Fatal("deadlock witness unavailable")
		}
		if result.err == nil && flipErr == nil || afterDeadlocks > beforeDeadlocks || time.Now().After(deadline) {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("deadlock witness timed out")
		}
	}
	t.Logf("actual deferred audit FK=%t; capability wait at COMMIT=%t; database deadlocks delta=%d", deferred, commitWait, afterDeadlocks-beforeDeadlocks)
	if result.err != nil || flipErr != nil || afterDeadlocks != beforeDeadlocks {
		t.Fatal("intake/capability lock inversion prevented convergence", result.err, flipErr)
	}
	if result.ack.Result() != protocol.Accepted {
		t.Fatal("actual meter operation did not commit before capability flip")
	}
	// The same real source may still read its immutable receipt with cap off;
	// the producer must not manufacture a new write or rewrite the old ACK.
	replay, err := store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), handler)
	if err != nil || !bytes.Equal(replay.CanonicalBytes(), result.ack.CanonicalBytes()) {
		t.Fatal("capability-off current receipt read changed", err)
	}
	var rows int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), op.ID()).Scan(&rows); err != nil || rows != 1 {
		t.Fatal("concurrent intake duplicated or lost evidence", err)
	}
	t.Logf("actual retained source/capability lock wait converged; immutable reading count%d", rows)
}

func TestMeterBorrowedSourcePreservesReadWriteSetupAndRenewedProof(t *testing.T) {
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	guard := &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	check := func(label string, scope kernel.OrgScope, source syncapp.Source, asset string, action fleet.MeterAction, want error) {
		t.Helper()
		err := database.WithOrgTx(t.Context(), fx.f.Runtime, scope, func(tx pgx.Tx) error {
			borrowed := syncapp.Borrow(tx)
			if _, ok := any(borrowed).(pgx.Tx); ok {
				t.Fatal("borrowed source exposes transaction lifecycle")
			}
			actual := guard.CheckMeterTx(t.Context(), borrowed, scope, fleet.MeterSource{Device: &source}, source.MembershipID, asset, action)
			if !errors.Is(actual, want) {
				return fmt.Errorf("%s: source policy mismatch: %w", label, actual)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check("current record", fx.scope, fx.source, created.AssetID, fleet.MeterRecordAction, nil)
	check("current receipt read", fx.scope, fx.source, created.AssetID, fleet.MeterReadAction, nil)
	check("foreign actual asset", fx.scope, fx.source, onboardingID(t), fleet.MeterReadAction, fleet.ErrNotFound)
	check("device install forbidden", fx.scope, fx.source, created.AssetID, fleet.MeterInstallAction, fleet.ErrUnauthenticated)
	// The original personal login is old, while a real refresh and new verified
	// signature supply a valid current proof in the SAME retained family.
	if _, err := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET issued_at=clock_timestamp()-interval '30 minutes' WHERE id=$1`, fx.person.auth.Session.FamilyID); err != nil {
		t.Fatal(err)
	}
	previous := fx.source
	renewed, err := fx.accounts.Refresh(t.Context(), fx.person.auth.Session.FamilyID, fx.person.auth.Session.RefreshToken)
	if err != nil {
		t.Fatal("actual same-family refresh failed", err)
	}
	if renewed.FamilyID != fx.person.auth.Session.FamilyID {
		t.Fatal("refresh replaced source family")
	}
	fx.person.auth.Session = renewed
	fx.person.actor = deviceVerifyActor(t, fx.deviceFixtureState, fx.person, fx.person.signer, time.Time{})
	current := fx.source
	current.Actor = fx.person.actor
	var old bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT issued_at<clock_timestamp()-interval '15 minutes' FROM account_session_families WHERE id=$1`, renewed.FamilyID).Scan(&old); err != nil || !old {
		t.Fatal("original personal authentication was not retained", err)
	}
	check("rotated token never reused", fx.scope, previous, created.AssetID, fleet.MeterRecordAction, fleet.ErrUnauthenticated)
	check("renewed operational proof", fx.scope, current, created.AssetID, fleet.MeterRecordAction, nil)
	if _, err := fx.flags.Set(t.Context(), fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	check("capability-off current read", fx.scope, current, created.AssetID, fleet.MeterReadAction, nil)
	check("capability-off new write", fx.scope, current, created.AssetID, fleet.MeterRecordAction, access.ErrEntitlementRequired)
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		borrowed := syncapp.Borrow(tx)
		state, err := fx.flags.LockSnapshotDBTX(t.Context(), borrowed, fx.scope, "organizations")
		if err != nil || state.Enabled || state.Revision != 2 {
			return fmt.Errorf("disabled source snapshot was fabricated: %w", err)
		}
		_, _, err = fx.devices.CheckCurrentIntakeDeviceDBTX(t.Context(), borrowed, fx.scope, current.Actor, current.MembershipID, current.EnrollmentID)
		return err
	}); err != nil {
		t.Fatal("intake read mode requires no enabled write capability", err)
	}
	// Create a genuinely pending organization instead of rewriting ACTIVE state.
	pending, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	pendingScope := scopeFor(t, pending.OrgID.String())
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, pendingScope, func(tx pgx.Tx) error {
		state, err := fx.flags.LockSnapshotDBTX(t.Context(), syncapp.Borrow(tx), pendingScope, "organizations")
		if err != nil || state.Enabled || state.Revision != 0 {
			return fmt.Errorf("absent capability invented readiness: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.flags.Set(t.Context(), pendingScope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	pendingFX := fx.deviceFixtureState
	pendingFX.scope = pendingScope
	pendingFX.member = pending.MembershipID
	pendingFX.person = deviceNewPerson(t, pendingFX)
	matrixAssign(t, pendingFX, matrixPerson{member: pendingFX.person.member}, "ADMIN", "ORG", "ACTIVE")
	enrolled, err := pendingFX.devices.EnrollDevice(t.Context(), pendingScope, pendingFX.person.actor, deviceEnrollCommand(t, "Synthetic pending source"))
	if err != nil {
		t.Fatal("legacy pending enrollment changed", err)
	}
	enrollment, err := enrolled.Device.Enrollment.Snapshot(pendingScope)
	if err != nil {
		t.Fatal(err)
	}
	pendingSource := syncapp.Source{Scope: pendingScope, Origin: deviceTestOrigin, Actor: pendingFX.person.actor, MembershipID: pendingFX.person.member, EnrollmentID: enrollment.ID}
	pendingAsset, _, err := fleetWrite(t, pendingFX.legalFixtureState, fleetStore(t, pendingFX.legalFixtureState), fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	check("pending source read", pendingScope, pendingSource, pendingAsset.AssetID, fleet.MeterReadAction, nil)
	check("pending operational write", pendingScope, pendingSource, pendingAsset.AssetID, fleet.MeterRecordAction, access.ErrSetupIncomplete)
}
