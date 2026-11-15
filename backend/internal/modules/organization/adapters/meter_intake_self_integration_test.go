//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// selfIntakeState is an ACTIVE asset with an actual meter, the real signed
// intake store, the actual held-review owner and the meter authority
// composed with the FLEET-006 assigned-asset source, as the runtime does.
type selfIntakeState struct {
	syncFixtureState
	assets    *fleetpg.AssetStore
	meters    *fleetpg.MeterStore
	installed fa.MeterInstallResult
	handler   syncapp.Handler
	intake    *syncpg.Store
	revision  int64
	unit      string
}

func selfIntakeFixture(t *testing.T) selfIntakeState {
	t.Helper()
	return selfIntakeFixtureOn(t, syncFixture(t))
}

// selfIntakeFixtureOn builds the SELF intake fixture on a given sync fixture,
// for example one whose database stops at an earlier migration image.
func selfIntakeFixtureOn(t *testing.T, sync syncFixtureState) selfIntakeState {
	t.Helper()
	fx, _, meters, installed, _, revision := lifecycleIntakeFixtureOn(t, sync)
	assets := assignmentStore(t, fx.legalFixtureState)
	meters.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices, Assignee: &orgpg.ResourceAssignee{Pool: fx.f.Runtime, Assignments: assets}}
	handler, err := fleetpg.NewMeterIntakeHandler(meters.Authority, meters)
	if err != nil {
		t.Fatal(err)
	}
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	return selfIntakeState{syncFixtureState: fx, assets: assets, meters: meters, installed: installed, handler: handler, intake: syncpg.NewStore(meterReviewIntakeConfig(fx)), revision: revision, unit: unit}
}

// driver is a new synthetic member whose only grant is DRIVER/SELF from its
// first retained period, with its own actual enrolled device.
func (x selfIntakeState) driver(t *testing.T) syncFixtureState {
	t.Helper()
	d := x.syncFixtureState
	d.person = deviceNewPerson(t, x.deviceFixtureState)
	d.source = meterHistoricalSource(t, x.deviceFixtureState, d.person)
	return d
}

func (x selfIntakeState) assign(t *testing.T, employee, asset string, kind fleet.AssignmentKind, start, end *time.Time) (string, assignmentRow) {
	t.Helper()
	result, _, err := assignmentWrite(t, x.legalFixtureState, x.assets, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: asset, Kind: kind, EmployeeIDs: []string{employee}, Start: start, End: end}))
	if err != nil || len(result.AssignmentIDs) != 1 {
		t.Fatal("actual assignment command", err)
	}
	return result.AssignmentIDs[0], assignmentRowsOf(t, x.legalFixtureState, asset)[result.AssignmentIDs[0]]
}

func (x selfIntakeState) end(t *testing.T, asset, id string) assignmentRow {
	t.Helper()
	if _, _, err := assignmentEnd(t, x.legalFixtureState, x.assets, fa.AssignmentEndInput{AssetID: asset, AssignmentID: id, Reason: "Synthetic end of driving duty"}); err != nil {
		t.Fatal("actual end command", err)
	}
	return assignmentRowsOf(t, x.legalFixtureState, asset)[id]
}

func (x selfIntakeState) operation(t *testing.T, d syncFixtureState, declared time.Time) protocol.Operation {
	t.Helper()
	return meterReviewOperation(t, d, meterObservedCommand(t, x.installed, declared, 7))
}

func (x selfIntakeState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.handler)
}

// refused proves a final source refusal with its exact bounded code and no
// claim, receipt, audit, reading, held fact or review item.
func (x selfIntakeState) refused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	var empty bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_meter_held_readings WHERE org_id=$1 AND operation_id=$2) AND NOT EXISTS(SELECT 1 FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&empty); err != nil || !empty {
		t.Fatal("refused SELF operation left a claim, receipt or fact", err)
	}
}

func (x selfIntakeState) accepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 {
		t.Fatal("covered SELF reading was not accepted", err, ack.Result(), ack.Code())
	}
	if readings, held, effects := lifecycleIntakeCount(t, x.syncFixtureState, op); readings != 1 || held != 0 || effects != 1 {
		t.Fatal("accepted SELF reading was not applied exactly once", readings, held, effects)
	}
}

func (x selfIntakeState) held(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, code string) {
	t.Helper()
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != code || ack.Snapshot().ReviewItemID == "" || ack.Snapshot().EntityRevision != nil {
		t.Fatal("SELF reading was not held for review", code, err, ack.Result(), ack.Code())
	}
	if readings, held, effects := lifecycleIntakeCount(t, x.syncFixtureState, op); readings != 0 || held != 1 || effects != 0 {
		t.Fatal("held SELF reading applied an effect", readings, held, effects)
	}
	var proof bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND operation_id=$2 AND code=$3)=1 AND(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2 AND review_proof_version=1)=1`, x.scope.OrgID().String(), op.ID(), code).Scan(&proof); err != nil || !proof {
		t.Fatal("held SELF reading lacks its real review item or version-1 proof", err)
	}
}

func (x selfIntakeState) replays(t *testing.T, d syncFixtureState, op protocol.Operation, original protocol.Acknowledgement) {
	t.Helper()
	replay, err := x.process(t, d, op)
	if err != nil || !bytes.Equal(replay.CanonicalBytes(), original.CanonicalBytes()) {
		t.Fatal("replay did not return the first result", err)
	}
}

// B-BR-EMP-259/260 on actual assignment periods through the real signed
// intake: never covered is FORBIDDEN with nothing stored; formerly covered is
// held PERMISSION_CHANGED; continuous coverage applies; half-open boundaries.
func TestMeterIntakeSelfAssignmentHistoryOnActualSignedIntake(t *testing.T) {
	x := selfIntakeFixture(t)
	asset := x.installed.AssetID
	now := lifecycleDBNow(t, x.legalFixtureState)

	t.Run("never-assigned-forbidden", func(t *testing.T) {
		d := x.driver(t)
		op := x.operation(t, d, now)
		ack, err := x.process(t, d, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("another-asset-forbidden", func(t *testing.T) {
		d := x.driver(t)
		other, _ := lifecycleAssetIn(t, x.legalFixtureState, x.assets, x.unit, fleet.Active)
		x.assign(t, d.person.employee, other, fleet.PrimaryAssignment, nil, nil)
		op := x.operation(t, d, lifecycleDBNow(t, x.legalFixtureState))
		ack, err := x.process(t, d, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("scheduled-after-receipt-forbidden", func(t *testing.T) {
		d := x.driver(t)
		start := lifecycleDBNow(t, x.legalFixtureState).Add(time.Hour).Truncate(time.Microsecond)
		x.assign(t, d.person.employee, asset, fleet.AdditionalAssignment, &start, nil)
		op := x.operation(t, d, start)
		ack, err := x.process(t, d, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("assigned-start-boundary-and-replay", func(t *testing.T) {
		d := x.driver(t)
		_, row := x.assign(t, d.person.employee, asset, fleet.PrimaryAssignment, nil, nil)
		exact := x.operation(t, d, row.from)
		ack, err := x.process(t, d, exact)
		x.accepted(t, exact, ack, err)
		// AC1 on the SELF path: three sends, one reading, identical ACKs.
		x.replays(t, d, exact, ack)
		x.replays(t, d, exact, ack)
		x.accepted(t, exact, ack, nil)
		before := x.operation(t, d, row.from.Add(-time.Nanosecond))
		held, err := x.process(t, d, before)
		x.held(t, before, held, err, "PERMISSION_CHANGED")
		x.replays(t, d, before, held)
	})
	t.Run("temporary-end-boundary", func(t *testing.T) {
		d := x.driver(t)
		until := lifecycleDBNow(t, x.legalFixtureState).Add(time.Hour).Truncate(time.Microsecond)
		_, row := x.assign(t, d.person.employee, asset, fleet.TemporaryAssignment, nil, &until)
		if row.to == nil || !row.to.Equal(until) {
			t.Fatal("temporary fixture lost its end")
		}
		inside := x.operation(t, d, until.Add(-time.Nanosecond))
		ack, err := x.process(t, d, inside)
		x.accepted(t, inside, ack, err)
		at := x.operation(t, d, until)
		held, err := x.process(t, d, at)
		x.held(t, at, held, err, "PERMISSION_CHANGED")
	})
	t.Run("ended-before-receipt-and-before-declaration-held", func(t *testing.T) {
		d := x.driver(t)
		id, row := x.assign(t, d.person.employee, asset, fleet.AdditionalAssignment, nil, nil)
		applied := x.operation(t, d, row.from)
		original, err := x.process(t, d, applied)
		x.accepted(t, applied, original, err)
		ended := x.end(t, asset, id)
		if ended.to == nil || ended.cause != "ENDED" || !ended.to.After(row.from) {
			t.Fatal("actual end command did not close the period")
		}
		for _, declared := range []time.Time{row.from, ended.to.Add(-time.Nanosecond), *ended.to, ended.to.Add(time.Second)} {
			op := x.operation(t, d, declared)
			ack, err := x.process(t, d, op)
			x.held(t, op, ack, err, "PERMISSION_CHANGED")
			x.replays(t, d, op, ack)
		}
		// The accepted reading replays only under current read coverage
		// (B-BR-EMP-246), exactly as an ORG member's replay after losing it.
		replay, err := x.process(t, d, applied)
		if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != orgapi.Forbidden || len(replay.CanonicalBytes()) != 0 {
			t.Fatal("accepted SELF reading replayed without current read coverage", err)
		}
		var kept bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT canonical_ack=$3 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), applied.ID(), original.CanonicalBytes()).Scan(&kept); err != nil || !kept {
			t.Fatal("refused replay changed the stored original receipt", err)
		}
	})
	t.Run("embedded-port-commits-and-rolls-back-with-parent", func(t *testing.T) {
		d := x.driver(t)
		_, row := x.assign(t, d.person.employee, asset, fleet.AdditionalAssignment, nil, nil)
		command := meterObservedCommand(t, x.installed, row.from, 9)
		rollback := errors.New("synthetic parent rollback")
		err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			if _, _, err := x.meters.RecordEmbedded(t.Context(), syncapp.Borrow(tx), x.scope, fa.MeterSource{Device: &d.source}, d.source.MembershipID, command); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatal("assigned SELF embedded reading refused inside its parent", err)
		}
		in, _ := command.Snapshot()
		if n := assignmentCount(t, x.legalFixtureState, `SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), in.OperationID); n != 0 {
			t.Fatal("embedded reading survived its parent rollback", n)
		}
		if _, replay, err := meterRecord(t, x.syncFixtureState, x.meters, d.source, command); err != nil || replay {
			t.Fatal("assigned SELF embedded reading did not commit with its parent", err)
		}
		stranger := x.driver(t)
		other := meterObservedCommand(t, x.installed, row.from, 9)
		if _, _, err := meterRecord(t, x.syncFixtureState, x.meters, stranger.source, other); err == nil {
			t.Fatal("never-assigned SELF member recorded an embedded reading")
		}
	})
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1 AND code='PERMISSION_CHANGED')=6 AND(SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1)=6 AND(SELECT count(*) FROM org_event_intents WHERE org_id=$1)=0`, x.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("SELF holds duplicated, lost or invented asynchronous work", err)
	}
}

// The port's periods equal its point answers, stay inside the membership and
// the organization, and flag only disposal ends (EDR-173).
func TestMeterIntakeSelfAssignmentPeriodsMatchThePortAndStayIsolated(t *testing.T) {
	x := selfIntakeFixture(t)
	asset := x.installed.AssetID
	d := x.driver(t)
	id, row := x.assign(t, d.person.employee, asset, fleet.AdditionalAssignment, nil, nil)
	ended := x.end(t, asset, id)
	_, open := x.assign(t, d.person.employee, asset, fleet.AdditionalAssignment, nil, nil)
	periods := func(scope kernel.OrgScope, txScope kernel.OrgScope, member, target string) ([]fa.AssignedPeriod, error) {
		var out []fa.AssignedPeriod
		err := database.WithOrgTx(t.Context(), x.f.Runtime, txScope, func(tx pgx.Tx) error {
			var err error
			out, err = x.assets.AssignmentPeriodsDBTX(t.Context(), syncapp.Borrow(tx), scope, member, target)
			return err
		})
		return out, err
	}
	got, err := periods(x.scope, x.scope, d.person.member, asset)
	if err != nil || len(got) != 2 || !got[0].From.Equal(row.from) || !got[0].To.Equal(*ended.to) || got[0].DisposalEnded || !got[1].From.Equal(open.from) || !got[1].To.IsZero() || got[1].DisposalEnded {
		t.Fatal("retained assignment periods differ from the stored periods", err, got)
	}
	for _, probe := range []struct {
		at   time.Time
		want bool
	}{{row.from.Add(-time.Microsecond), false}, {row.from, true}, {ended.to.Add(-time.Microsecond), true}, {*ended.to, false}, {open.from, true}} {
		if assigned := assignedAt(t, x.legalFixtureState, x.assets, d.person.member, asset, probe.at); assigned != probe.want {
			t.Fatal("point answer disagrees with the retained period", probe.at, assigned)
		}
	}
	if none, err := periods(x.scope, x.scope, x.driver(t).person.member, asset); err != nil || len(none) != 0 {
		t.Fatal("never-assigned member has periods", err, none)
	}
	other := legalFixtureFor(t, x.f, x.onboarding, x.actor, x.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var otherUnit string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&otherUnit); err != nil {
		t.Fatal(err)
	}
	foreignEmployee, foreignMember := assignmentPerson(t, other)
	foreignAsset, _ := lifecycleAssetIn(t, other, x.assets, otherUnit, fleet.Active)
	if _, _, err := assignmentWrite(t, other, x.assets, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: foreignAsset, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{foreignEmployee}})); err != nil {
		t.Fatal(err)
	}
	if mine, err := periods(other.scope, other.scope, foreignMember, foreignAsset); err != nil || len(mine) != 1 {
		t.Fatal("foreign fixture lacks its own period", err, mine)
	}
	if leaked, err := periods(x.scope, x.scope, foreignMember, foreignAsset); err != nil || len(leaked) != 0 {
		t.Fatal("another organization's assignment became a period", err, leaked)
	}
	if _, err := periods(other.scope, x.scope, foreignMember, foreignAsset); err == nil {
		t.Fatal("borrowed transaction crossed the organization scope")
	}
	// The signed intake of an assigned SELF member never reaches another
	// organization's asset: NOT_FOUND, nothing stored on either side.
	op := syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"], raw["occurred_at"], raw["entry_zone"] = fleetpg.MeterReadingOperationType, orgtime.FormatInstant(open.from), "UTC"
		raw["entity"] = map[string]any{"type": "meter_reading", "id": onboardingID(t), "expected_revision": nil}
		raw["payload"] = map[string]any{"asset_id": foreignAsset, "meter_id": x.installed.MeterID, "segment_id": x.installed.SegmentID, "kind": "ODOMETER", "value": "7", "method": "TYPED"}
	})
	ack, err := x.process(t, d, op)
	x.refused(t, op, ack, err, orgapi.NotFound)
	if n := assignmentCount(t, x.legalFixtureState, `SELECT count(*) FROM org_sync_operations WHERE org_id=$1`, other.scope.OrgID().String()); n != 0 {
		t.Fatal("foreign organization gained an intake claim", n)
	}
}

// B-BR-EMP-260: an assignment end and an intake serialize on the organization
// root lock. The order is forced by observed lock waits (no sleeps); each
// order decides consistently with the recorded end instant.
func TestMeterIntakeSelfAssignmentEndRacesIntakeInRootLockOrder(t *testing.T) {
	x := selfIntakeFixture(t)
	asset := x.installed.AssetID
	org := x.scope.OrgID().String()
	for _, endFirst := range []bool{true, false} {
		name := "intake-first-applies"
		if endFirst {
			name = "end-first-holds"
		}
		t.Run(name, func(t *testing.T) {
			d := x.driver(t)
			id, row := x.assign(t, d.person.employee, asset, fleet.AdditionalAssignment, nil, nil)
			op := x.operation(t, d, row.from)
			end, err := fa.NewAssignmentEnd(fa.AssignmentEndInput{OperationID: onboardingID(t), AssetID: asset, AssignmentID: id, Reason: "Synthetic concurrent end"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			holder, err := x.f.Runtime.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback(ctx)
			if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
				t.Fatal(err)
			}
			waiting := func(n int) {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					var count int
					if err := x.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock'`, x.f.RuntimeRole).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count >= n {
						return
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						t.Fatal("no lock wait observed", n)
					}
				}
			}
			ended := make(chan error, 1)
			type result struct {
				ack protocol.Acknowledgement
				err error
			}
			intake := make(chan result, 1)
			startEnd := func() {
				go func() {
					_, _, err := x.assets.EndAssignment(ctx, x.scope, fleetActor(x.legalFixtureState), x.member, end)
					ended <- err
				}()
			}
			startIntake := func() {
				go func() {
					ack, err := x.intake.Process(ctx, d.source, op, protocol.DefaultLimits(), x.handler)
					intake <- result{ack, err}
				}()
			}
			if endFirst {
				startEnd()
				waiting(1)
				startIntake()
			} else {
				startIntake()
				waiting(1)
				startEnd()
			}
			waiting(2)
			if err := holder.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-ended; err != nil {
				t.Fatal("concurrent end command failed", err)
			}
			got := <-intake
			closed := assignmentRowsOf(t, x.legalFixtureState, asset)[id]
			if closed.to == nil {
				t.Fatal("end command left the period open")
			}
			if endFirst {
				x.held(t, op, got.ack, got.err, "PERMISSION_CHANGED")
			} else {
				x.accepted(t, op, got.ack, got.err)
			}
			received, err := orgtime.ParseInstant(got.ack.Snapshot().ReceivedAt)
			if err != nil {
				t.Fatal(err)
			}
			if endFirst == closed.to.After(received) {
				t.Fatal("intake decision disagrees with the recorded end instant", endFirst, closed.to, received)
			}
		})
	}
}

// B-BR-EMP-261: the asset's disposal ends the SELF assignment; a reading
// declared at or after the disposal stays ASSET_UNAVAILABLE, one declared
// before it is PERMISSION_CHANGED, and a revert restores no assignment.
func TestMeterIntakeSelfDisposalHoldsAssetUnavailableAtOrAfterTheInstant(t *testing.T) {
	x := selfIntakeFixture(t)
	asset := x.installed.AssetID
	d := x.driver(t)
	id, row := x.assign(t, d.person.employee, asset, fleet.PrimaryAssignment, nil, nil)
	disposal := lifecycleDBNow(t, x.legalFixtureState).Truncate(time.Microsecond).Add(-time.Microsecond).Add(500 * time.Nanosecond)
	if !disposal.After(row.from.Add(time.Microsecond)) {
		t.Fatal("disposal fixture does not follow the assignment start")
	}
	dispose := lifecycleInputFor(t, x.legalFixtureState, fleet.Dispose, asset, x.revision)
	dispose.At = disposal
	disposed, _, err := lifecycleWrite(t, x.legalFixtureState, x.assets, lifecycleCommand(t, dispose))
	if err != nil {
		t.Fatal(err)
	}
	ended := assignmentRowsOf(t, x.legalFixtureState, asset)[id]
	if ended.cause != "ASSET_DISPOSED" || ended.to == nil || !ended.to.Equal(disposal.Truncate(time.Microsecond)) {
		t.Fatal("disposal did not end the SELF assignment at its instant", ended.cause)
	}
	var reviews []string
	for _, c := range []struct {
		declared time.Time
		code     string
	}{
		{row.from.Add(-time.Nanosecond), "PERMISSION_CHANGED"},
		{ended.to.Add(-time.Nanosecond), "PERMISSION_CHANGED"},
		{disposal.Add(-time.Nanosecond), "PERMISSION_CHANGED"},
		{disposal, "ASSET_UNAVAILABLE"},
		{disposal.Add(time.Nanosecond), "ASSET_UNAVAILABLE"},
		{disposal.Add(time.Millisecond), "ASSET_UNAVAILABLE"},
	} {
		op := x.operation(t, d, c.declared)
		ack, err := x.process(t, d, op)
		x.held(t, op, ack, err, c.code)
		x.replays(t, d, op, ack)
		if c.code == "ASSET_UNAVAILABLE" {
			reviews = append(reviews, ack.Snapshot().ReviewItemID)
		}
	}
	reviewer := x.meters.Reviews
	item, err := reviewer.ReadMeterReview(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, reviews[0])
	if err != nil || item.Code != "ASSET_UNAVAILABLE" {
		t.Fatal("SELF disposal hold unreadable by the existing reviewer", err)
	}
	decision, err := fa.NewMeterReviewDecision(fa.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: reviews[0], ExpectedRevision: 1, Decision: fa.MeterReviewAccept})
	if err != nil {
		t.Fatal(err)
	}
	if result, _, err := reviewer.ResolveMeterReview(t.Context(), x.scope, fleetActor(x.legalFixtureState), x.member, decision); err != nil || result.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("existing review decision unavailable for a SELF disposal hold", err)
	}
	if _, _, err := lifecycleWrite(t, x.legalFixtureState, x.assets, lifecycleCommand(t, lifecycleInputFor(t, x.legalFixtureState, fleet.RevertDisposal, asset, disposed.Revision))); err != nil {
		t.Fatal(err)
	}
	if after := assignmentRowsOf(t, x.legalFixtureState, asset)[id]; after.to == nil || !after.to.Equal(*ended.to) {
		t.Fatal("revert restored the ended assignment")
	}
	op := x.operation(t, d, disposal.Add(2*time.Millisecond))
	ack, err := x.process(t, d, op)
	x.held(t, op, ack, err, "PERMISSION_CHANGED")
}

// Two assigned SELF members record on one meter at once through the real
// signed intake: per-meter serialization keeps both distinct observations.
func TestMeterIntakeSelfTwoDevicesRecordAtOnceOnOneMeter(t *testing.T) {
	x := selfIntakeFixture(t)
	asset := x.installed.AssetID
	first, second := x.driver(t), x.driver(t)
	_, row := x.assign(t, first.person.employee, asset, fleet.PrimaryAssignment, nil, nil)
	x.assign(t, second.person.employee, asset, fleet.AdditionalAssignment, nil, nil)
	declared := lifecycleDBNow(t, x.legalFixtureState)
	if !declared.After(row.from) {
		t.Fatal("declared instant precedes the assignments")
	}
	drivers := []syncFixtureState{first, second}
	ops := []protocol.Operation{x.operation(t, first, declared), x.operation(t, second, declared)}
	start := make(chan struct{})
	type answer struct {
		ack protocol.Acknowledgement
		err error
	}
	out := make([]answer, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			<-start
			ack, err := x.intake.Process(t.Context(), drivers[i].source, ops[i], protocol.DefaultLimits(), x.handler)
			out[i] = answer{ack, err}
		})
	}
	close(start)
	wg.Wait()
	for i := range 2 {
		x.accepted(t, ops[i], out[i].ack, out[i].err)
	}
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND count(DISTINCT enrollment_id)=2 AND count(DISTINCT author_membership_id)=2 FROM org_meter_readings WHERE org_id=$1 AND operation_id=ANY($2::uuid[])`, x.scope.OrgID().String(), []string{ops[0].ID(), ops[1].ID()}).Scan(&exact); err != nil || !exact {
		t.Fatal("concurrent SELF devices lost or merged observations", err)
	}
}
