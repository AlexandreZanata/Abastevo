//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-023 (B-BR-EMP-412..414, EDR-265/266): reservation-to-trip
// integration on real PostGIS with the ordinary runtime role.

func missionReservationFixture(t *testing.T) missionState {
	t.Helper()
	m := missionFixture(t)
	m.missions.Reservations = m.assets
	return m
}

// missionAt creates a mission on the vehicle for [start, end), assigns the
// member d and records d's signed acceptance (revision 3).
func (m missionState) missionAt(t *testing.T, d syncFixtureState, asset string, start, end time.Time) string {
	t.Helper()
	created, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCreate, UnitID: m.unit, AssetID: asset,
		Title: "Entrega sintética no cliente", StartsAt: start, EndsAt: end}))
	if err != nil {
		t.Fatal("mission creation", err)
	}
	if _, err := m.assignTo(t, created.MissionID, 1, d.person.employee); err != nil {
		t.Fatal("mission assignment", err)
	}
	ack, err := m.deliver(t, d, m.respondOp(t, d, created.MissionID, 2, "ACCEPT", nil, nil))
	if err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("mission acceptance", err, ack.Result(), ack.Code())
	}
	return created.MissionID
}

func (m missionState) reserveMission(t *testing.T, mission string, revision int64, operation, purpose string) (tripapp.MissionAck, bool, error) {
	t.Helper()
	if operation == "" {
		operation = onboardingID(t)
	}
	c := missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionReserveCommand, OperationID: operation, MissionID: mission, ExpectedRevision: revision, Reason: purpose})
	return m.missions.CommandMission(t.Context(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, onboardingID(t), c)
}

func (m missionState) confirmReservation(t *testing.T, asset, reservation string) {
	t.Helper()
	if r, _, err := actOn(t, m.legalFixtureState, m.assets, true, fa.ReservationActInput{AssetID: asset, ReservationID: reservation}); err != nil || r.State != fleet.ReservationConfirmed {
		t.Fatal("reservation confirmation", r.State, err)
	}
}

// missionStartOp is member d's start on the vehicle naming the mission.
func (m missionState) missionStartOp(t *testing.T, d syncFixtureState, mission string, declared time.Time) protocol.Operation {
	t.Helper()
	return m.startOp(t, d, declared, func(e, _ map[string]any) {
		e["soft_references"] = []any{map[string]any{"type": tripapp.MissionReferenceType, "id": mission}}
	})
}

type reservationLink struct {
	state, mission, trip string
	fulfilled            bool
}

func (m missionState) reservationLink(t *testing.T, id string) reservationLink {
	t.Helper()
	var r reservationLink
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT state,COALESCE(mission_id::text,''),COALESCE(trip_id::text,''),fulfilled_at IS NOT NULL FROM org_asset_reservations WHERE org_id=$1 AND id=$2`,
		m.scope.OrgID().String(), id).Scan(&r.state, &r.mission, &r.trip, &r.fulfilled); err != nil {
		t.Fatal(err)
	}
	return r
}

// linkedMission is the mission linked to the trip, "" for none.
func (m missionState) linkedMission(t *testing.T, trip string) string {
	t.Helper()
	var mission string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT COALESCE((SELECT mission_id::text FROM org_trip_mission_links WHERE org_id=$1 AND trip_id=$2),'')`, m.scope.OrgID().String(), trip).Scan(&mission); err != nil {
		t.Fatal(err)
	}
	return mission
}

type missionStep struct {
	kind, source, from, to, trip, operation string
}

func (m missionState) lastStep(t *testing.T, mission string) missionStep {
	t.Helper()
	var s missionStep
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT kind,source,COALESCE(from_state,''),to_state,COALESCE(trip_id::text,''),operation_id::text FROM org_trip_mission_transitions
WHERE org_id=$1 AND mission_id=$2 ORDER BY revision DESC LIMIT 1`, m.scope.OrgID().String(), mission).Scan(&s.kind, &s.source, &s.from, &s.to, &s.trip, &s.operation); err != nil {
		t.Fatal(err)
	}
	return s
}

func (m missionState) countWhere(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := m.f.Admin.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// B-BR-EMP-412..414, BUC-EMP-209: the planner reserves the accepted
// mission's vehicle, a fleet manager confirms, the driver's start naming the
// mission is accepted, linked, moves the mission to IN_PROGRESS and consumes
// the reservation once; a second start on the vehicle is held ASSET_BUSY; the
// outcome job never misses the consumed reservation; the finish completes
// the mission.
func TestTripMissionLinkedStartConsumesItsReservationAndCompletes(t *testing.T) {
	m := missionReservationFixture(t)
	d := m.syncFixtureState
	org := m.scope.OrgID().String()
	start := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(10 * time.Minute)
	mission := m.missionAt(t, d, m.vehicle.AssetID, start, start.Add(2*time.Hour))

	operation := onboardingID(t)
	reserved, replayed, err := m.reserveMission(t, mission, 3, operation, "Entrega sintética no cliente")
	if err != nil || replayed || reserved.Revision != 4 || reserved.State != tripdomain.MissionAccepted || reserved.ReservationID == "" || reserved.AssigneeEmployeeID != d.person.employee {
		t.Fatal("mission reservation request", reserved, replayed, err)
	}
	if r := m.reservationLink(t, reserved.ReservationID); r.state != "REQUESTED" || r.mission != mission || r.trip != "" || r.fulfilled {
		t.Fatal("requested mission reservation", r)
	}
	var startsAt, endsAt time.Time
	var employee *string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT starts_at,ends_at,employee_id::text FROM org_asset_reservations WHERE org_id=$1 AND id=$2`, org, reserved.ReservationID).Scan(&startsAt, &endsAt, &employee); err != nil ||
		!startsAt.Equal(start) || !endsAt.Equal(start.Add(2*time.Hour)) || employee != nil {
		t.Fatal("the reservation is not the mission's window without a reserved employee", startsAt, endsAt, employee, err)
	}
	if n := m.countWhere(t, `SELECT (SELECT count(*) FROM org_asset_reservation_commands WHERE org_id=$1 AND operation_id=$2 AND kind='REQUEST' AND target_id=$3)
 +(SELECT count(*) FROM org_trip_mission_commands WHERE org_id=$1 AND operation_id=$2 AND kind='RESERVE')+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2 AND action='trip.mission.reservation_requested')
 +(SELECT count(*) FROM job_queue WHERE dedupe_key LIKE '%'||$3::text)`, org, operation, reserved.ReservationID); n != 4 {
		t.Fatal("reservation request receipts, audit and outcome job", n)
	}
	if step := m.lastStep(t, mission); step.kind != "RESERVE" || step.source != "COMPANY" || step.from != "ACCEPTED" || step.to != "ACCEPTED" {
		t.Fatal("RESERVE revision", step)
	}
	again, replayed, err := m.reserveMission(t, mission, 3, operation, "Entrega sintética no cliente")
	if err != nil || !replayed || again.ReservationID != reserved.ReservationID || again.Revision != 4 {
		t.Fatal("reservation request replay", again, replayed, err)
	}
	if _, _, err := m.reserveMission(t, mission, 3, operation, "Outra finalidade"); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("same operation with another purpose", err)
	}
	m.confirmReservation(t, m.vehicle.AssetID, reserved.ReservationID)

	op := m.missionStartOp(t, d, mission, m.declaredNow(t))
	ack, err := m.process(t, d, op)
	m.accepted(t, op, ack, err, 1, 1)
	trip := tripOf(op)
	if m.linkedMission(t, trip) != mission {
		t.Fatal("accepted start not linked to its mission")
	}
	if h := m.head(t, mission); h.state != "IN_PROGRESS" || h.revision != 5 {
		t.Fatal("mission did not follow its trip's start", h)
	}
	if step := m.lastStep(t, mission); step.kind != "START" || step.source != "TRIP" || step.trip != trip || step.operation != op.ID() || step.from != "ACCEPTED" {
		t.Fatal("START revision", step)
	}
	if r := m.reservationLink(t, reserved.ReservationID); r.state != "FULFILLED" || r.trip != trip || !r.fulfilled {
		t.Fatal("the linked start did not consume the reservation", r)
	}
	for range 2 {
		resent, err := m.process(t, d, op)
		if err != nil || !bytes.Equal(resent.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resent start did not return the stored acknowledgement", err)
		}
	}
	reuse := m.startOp(t, d, m.declaredNow(t), func(e, p map[string]any) {
		e["operation_id"] = op.ID()
		e["entity"] = map[string]any{"type": tripapp.EntityType, "id": trip, "expected_revision": nil}
		e["soft_references"] = []any{}
	})
	if again, err := m.process(t, d, reuse); err != nil || again.Result() != protocol.Rejected || again.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("same start identifier with another digest", err, again.Result(), again.Code())
	}
	if m.countWhere(t, `SELECT count(*) FROM org_trip_mission_links WHERE org_id=$1`, org) != 1 {
		t.Fatal("a replay or reuse stored a second link")
	}

	// Parallel active use: a second start on the vehicle naming the same
	// (now IN_PROGRESS) mission is held ASSET_BUSY; nothing is linked.
	second := m.missionStartOp(t, d, mission, m.declaredNow(t))
	held, err := m.process(t, d, second)
	m.held(t, second, held, err, tripapp.AssetBusy)
	if m.linkedMission(t, tripOf(second)) != "" || m.head(t, mission).revision != 5 {
		t.Fatal("a held start touched the mission")
	}

	// The outcome job never misses a consumed reservation.
	for range 2 {
		if state, err := m.assets.SettleReservation(t.Context(), m.scope, reserved.ReservationID, fleet.MissReservation); err != nil || state != fleet.ReservationFulfilled {
			t.Fatal("outcome job on a fulfilled reservation", state, err)
		}
	}
	if m.countWhere(t, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='fleet.reservation.missed'`, org) != 0 {
		t.Fatal("a fulfilled reservation was missed")
	}

	finish := m.finishOp(t, d, trip, m.declaredNow(t).Add(time.Second), nil)
	done, err := m.process(t, d, finish)
	m.accepted(t, finish, done, err, 2, 1)
	if h := m.head(t, mission); h.state != "COMPLETED" || h.revision != 6 {
		t.Fatal("mission did not complete with its trip", h)
	}
	if step := m.lastStep(t, mission); step.kind != "COMPLETE" || step.source != "TRIP" || step.trip != trip || step.operation != finish.ID() || step.from != "IN_PROGRESS" {
		t.Fatal("COMPLETE revision", step)
	}
	if r := m.reservationLink(t, reserved.ReservationID); r.state != "FULFILLED" || r.trip != trip {
		t.Fatal("completion changed the consumed reservation", r)
	}
}

// B-BR-EMP-412, BR-OFFL-007: a start naming a missing, foreign or ineligible
// mission is held ORPHAN_REFERENCE with nothing linked; a normal held code
// keeps its priority; other reference forms stay RETRY/UNAVAILABLE.
func TestTripMissionReferenceOrphanAndUnsupportedForms(t *testing.T) {
	m := missionReservationFixture(t)
	d := m.syncFixtureState
	driver := m.driver(t)
	org := m.scope.OrgID().String()
	window := func() (time.Time, time.Time) {
		start := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(time.Hour)
		return start, start.Add(time.Hour)
	}
	other := m.otherVehicle(t, "")
	s1, e1 := window()
	accepted := m.missionAt(t, d, m.vehicle.AssetID, s1, e1)
	cancelled := m.missionAt(t, d, m.vehicle.AssetID, s1, e1)
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: cancelled,
		ExpectedRevision: 3, Reason: "Cliente cancelou"})); err != nil {
		t.Fatal(err)
	}
	assigned := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, assigned, 1, d.person.employee); err != nil {
		t.Fatal(err)
	}
	othersMission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, othersMission, 1, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	if ack, err := m.deliver(t, driver, m.respondOp(t, driver, othersMission, 2, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal(err)
	}
	otherVehicle := m.missionAt(t, d, other.AssetID, s1, e1)
	foreign := func() string {
		x := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
		if _, _, err := x.onboarding.Repository.ActivateOrganization(t.Context(), x.scope, x.actor, x.member, onboardingID(t), 1); err != nil {
			t.Fatal(err)
		}
		grantMissionTables(t, x)
		var unit string
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 LIMIT 1`, x.scope.OrgID().String()).Scan(&unit); err != nil {
			t.Fatal(err)
		}
		created, _, err := missionStoreFor(t, x, m.tripState).CommandMission(t.Context(), x.scope, ownerMissionActor(x), x.member, onboardingID(t),
			missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCreate, UnitID: unit, Title: "Missão alheia", StartsAt: s1, EndsAt: e1}))
		if err != nil {
			t.Fatal(err)
		}
		return created.MissionID
	}()
	for name, mission := range map[string]string{
		"unknown":          onboardingID(t),
		"other-org":        foreign,
		"cancelled":        cancelled,
		"not-accepted":     assigned,
		"another-assignee": othersMission,
		"another-vehicle":  otherVehicle,
	} {
		t.Run(name, func(t *testing.T) {
			before := m.head(t, accepted)
			op := m.missionStartOp(t, d, mission, m.declaredNow(t))
			ack, err := m.process(t, d, op)
			m.held(t, op, ack, err, tripapp.OrphanReference)
			if m.linkedMission(t, tripOf(op)) != "" || m.head(t, accepted) != before {
				t.Fatal("an orphan reference linked or changed a mission")
			}
			for range 2 {
				again, err := m.process(t, d, op)
				if err != nil || !bytes.Equal(again.CanonicalBytes(), ack.CanonicalBytes()) {
					t.Fatal("orphan replay", err)
				}
			}
		})
	}
	if n := m.countWhere(t, `SELECT count(*) FROM org_trip_mission_links WHERE org_id=$1`, org); n != 0 {
		t.Fatal("orphan references stored links", n)
	}
	if h := m.head(t, accepted); h.state != "ACCEPTED" || h.revision != 3 {
		t.Fatal("an eligible mission changed without its start", h)
	}
	for name, refs := range map[string][]any{
		"another-type": {map[string]any{"type": "fueling", "id": accepted}},
		"two-missions": {map[string]any{"type": tripapp.MissionReferenceType, "id": accepted}, map[string]any{"type": tripapp.MissionReferenceType, "id": cancelled}},
	} {
		t.Run("unsupported-"+name, func(t *testing.T) {
			op := m.startOp(t, d, m.declaredNow(t), func(e, _ map[string]any) { e["soft_references"] = refs })
			ack, err := m.process(t, d, op)
			if !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
				t.Fatal("unsupported reference form not kept RETRY/UNAVAILABLE", err, ack.Result())
			}
			if c := m.count(t, op); c != (tripCounts{}) {
				t.Fatalf("unsupported reference stored state: %+v", c)
			}
		})
	}
	// A normal held code keeps its priority over the orphan reference: a
	// confirmed reservation of another member's mission holds the vehicle
	// at the declared instant (ASSET_UNAVAILABLE).
	soon := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(10 * time.Second)
	guarded := m.missionAt(t, driver, m.vehicle.AssetID, soon, soon.Add(time.Hour))
	reserved, _, err := m.reserveMission(t, guarded, 3, "", "Reserva do outro motorista")
	if err != nil {
		t.Fatal(err)
	}
	m.confirmReservation(t, m.vehicle.AssetID, reserved.ReservationID)
	waitDBClock(t, m.legalFixtureState, soon)
	op := m.missionStartOp(t, d, onboardingID(t), m.declaredNow(t))
	ack, err := m.process(t, d, op)
	m.held(t, op, ack, err, tripapp.AssetUnavailable)
	if r := m.reservationLink(t, reserved.ReservationID); r.state != "CONFIRMED" || r.trip != "" {
		t.Fatal("a held start touched another mission's reservation", r)
	}
}

// BR-FLEET-031 (B-BR-EMP-412/414): inside a confirmed reservation of the
// mission, the mission's own start is admitted without an assignment of the
// starter and consumes the reservation; the same member's start without the
// reference is held ASSET_UNAVAILABLE.
func TestTripMissionReservationAdmitsItsOwnStartWithoutAssignment(t *testing.T) {
	m := missionReservationFixture(t)
	d := m.syncFixtureState
	org := m.scope.OrgID().String()
	var assignment string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_asset_assignments WHERE org_id=$1 AND asset_id=$2 AND employee_id=$3`, org, m.vehicle.AssetID, d.person.employee).Scan(&assignment); err != nil {
		t.Fatal(err)
	}
	m.end(t, m.vehicle.AssetID, assignment)
	soon := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(10 * time.Second)
	mission := m.missionAt(t, d, m.vehicle.AssetID, soon, soon.Add(time.Hour))
	reserved, _, err := m.reserveMission(t, mission, 3, "", "Reserva da missão")
	if err != nil {
		t.Fatal(err)
	}
	m.confirmReservation(t, m.vehicle.AssetID, reserved.ReservationID)
	waitDBClock(t, m.legalFixtureState, soon)
	without := m.startOp(t, d, m.declaredNow(t), nil)
	ack, err := m.process(t, d, without)
	m.held(t, without, ack, err, tripapp.AssetUnavailable)
	own := m.missionStartOp(t, d, mission, m.declaredNow(t))
	ack, err = m.process(t, d, own)
	m.accepted(t, own, ack, err, 1, 1)
	if r := m.reservationLink(t, reserved.ReservationID); r.state != "FULFILLED" || r.trip != tripOf(own) {
		t.Fatal("the reserving mission's start did not consume its reservation", r)
	}
	if m.linkedMission(t, tripOf(own)) != mission || m.head(t, mission).state != "IN_PROGRESS" {
		t.Fatal("the mission did not follow its start")
	}
}

// B-BR-EMP-414, BUC-EMP-210: the no-show release and the linked start race
// behind the organization root lock and resolve to exactly one outcome in
// both orders; a start delivered after the deadline never consumes the
// released (or due but unsettled) reservation.
func TestTripMissionReservationReleaseRacesLinkedStart(t *testing.T) {
	type outcome struct {
		ack   protocol.Acknowledgement
		err   error
		state fleet.ReservationState
		job   error
	}
	// setup reserves and confirms a short mission window whose deadline is
	// its end (start in 10 s, end 4 s later) and prepares the start declared
	// now, before the window (the starter is assigned to the vehicle).
	setup := func(t *testing.T) (missionState, string, string, protocol.Operation, time.Time) {
		m := missionReservationFixture(t)
		d := m.syncFixtureState
		start := exactNow(t, m.legalFixtureState).Truncate(time.Millisecond).Add(10 * time.Second)
		mission := m.missionAt(t, d, m.vehicle.AssetID, start, start.Add(4*time.Second))
		reserved, _, err := m.reserveMission(t, mission, 3, "", "Janela curta sintética")
		if err != nil {
			t.Fatal(err)
		}
		m.confirmReservation(t, m.vehicle.AssetID, reserved.ReservationID)
		return m, mission, reserved.ReservationID, m.missionStartOp(t, d, mission, m.declaredNow(t)), start.Add(4 * time.Second)
	}
	run := func(t *testing.T, m missionState, op protocol.Operation, reservation string, deadline time.Time, startFirst bool) outcome {
		var out outcome
		release := holdRoot(t, m)
		t.Cleanup(release)
		var wg sync.WaitGroup
		launchStart := func() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out.ack, out.err = m.trips.Process(context.Background(), m.syncFixtureState.source, op, protocol.DefaultLimits(), m.start)
			}()
		}
		launchJob := func() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out.state, out.job = m.assets.SettleReservation(context.Background(), m.scope, reservation, fleet.MissReservation)
			}()
		}
		if startFirst {
			// The start's transaction begins shortly before the deadline
			// (the intake bounds one operation to 10 s) and queues first;
			// the job's begins after the deadline and queues second.
			waitDBClock(t, m.legalFixtureState, deadline.Add(-2*time.Second))
			launchStart()
			waitLockWaiters(t, m, 1)
			waitDBClock(t, m.legalFixtureState, deadline)
			launchJob()
		} else {
			waitDBClock(t, m.legalFixtureState, deadline)
			launchJob()
			waitLockWaiters(t, m, 1)
			launchStart()
		}
		waitLockWaiters(t, m, 2)
		release()
		wg.Wait()
		return out
	}
	t.Run("start-before-release", func(t *testing.T) {
		m, mission, reservation, op, deadline := setup(t)
		out := run(t, m, op, reservation, deadline, true)
		m.accepted(t, op, out.ack, out.err, 1, 1)
		if out.job != nil || out.state != fleet.ReservationFulfilled {
			t.Fatal("the queued release did not find the consumed reservation", out.state, out.job)
		}
		if r := m.reservationLink(t, reservation); r.state != "FULFILLED" || r.trip != tripOf(op) {
			t.Fatal("start-first outcome", r)
		}
		if m.countWhere(t, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='fleet.reservation.missed'`, m.scope.OrgID().String()) != 0 || m.head(t, mission).state != "IN_PROGRESS" {
			t.Fatal("a consumed reservation was also released")
		}
	})
	t.Run("release-before-start", func(t *testing.T) {
		m, mission, reservation, op, deadline := setup(t)
		out := run(t, m, op, reservation, deadline, false)
		if out.job != nil || out.state != fleet.ReservationNoShow {
			t.Fatal("the release", out.state, out.job)
		}
		// The start is decided without the released reservation: accepted,
		// linked to its mission, the reservation stays NO_SHOW once.
		m.accepted(t, op, out.ack, out.err, 1, 1)
		if r := m.reservationLink(t, reservation); r.state != "NO_SHOW" || r.trip != "" || r.fulfilled {
			t.Fatal("an expired reservation was consumed", r)
		}
		if m.linkedMission(t, tripOf(op)) != mission || m.head(t, mission).state != "IN_PROGRESS" {
			t.Fatal("the start lost its mission link")
		}
		if m.countWhere(t, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='fleet.reservation.missed'`, m.scope.OrgID().String()) != 1 {
			t.Fatal("NO_SHOW not recorded exactly once")
		}
		if state, err := m.assets.SettleReservation(t.Context(), m.scope, reservation, fleet.MissReservation); err != nil || state != fleet.ReservationNoShow {
			t.Fatal("second job run", state, err)
		}
	})
	t.Run("job-stopped-due-reservation-never-consumed", func(t *testing.T) {
		m, mission, reservation, op, deadline := setup(t)
		waitDBClock(t, m.legalFixtureState, deadline)
		ack, err := m.process(t, m.syncFixtureState, op)
		m.accepted(t, op, ack, err, 1, 1)
		if r := m.reservationLink(t, reservation); r.state != "CONFIRMED" || r.trip != "" {
			t.Fatal("a due reservation was consumed by a late start", r)
		}
		if m.linkedMission(t, tripOf(op)) != mission {
			t.Fatal("late start not linked")
		}
		if state, err := m.assets.SettleReservation(t.Context(), m.scope, reservation, fleet.MissReservation); err != nil || state != fleet.ReservationNoShow {
			t.Fatal("the late job release", state, err)
		}
	})
}

// B-BR-EMP-413/414: the planner's RESERVE refusals; an IN_PROGRESS mission
// can only be cancelled by its planner, and a later completion of its trip
// leaves the cancelled mission unchanged.
func TestTripMissionReserveRefusalsAndCancelInProgress(t *testing.T) {
	m := missionReservationFixture(t)
	d := m.syncFixtureState
	start := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(10 * time.Minute)
	mission := m.missionAt(t, d, m.vehicle.AssetID, start, start.Add(time.Hour))
	draft := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	vehicleless := m.create(t, m.unit, "").MissionID
	if _, err := m.assignTo(t, vehicleless, 1, d.person.employee); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		mission  string
		revision int64
		want     error
	}{
		"stale-revision": {mission, 2, tripdomain.ErrConflict},
		"draft":          {draft, 1, tripdomain.ErrInvalidTransition},
		"no-vehicle":     {vehicleless, 2, tripdomain.ErrInvalid},
		"unknown":        {onboardingID(t), 1, tripapp.ErrNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			operation := onboardingID(t)
			_, _, err := m.reserveMission(t, c.mission, c.revision, operation, "Finalidade")
			wantMissionError(t, err, c.want)
			if mc := m.counts(t, operation); mc != (missionCounts{}) || m.countWhere(t, `SELECT count(*) FROM org_asset_reservations WHERE org_id=$1 AND operation_id=$2`, m.scope.OrgID().String(), operation) != 0 {
				t.Fatal("a refused request stored state", mc)
			}
		})
	}
	first, _, err := m.reserveMission(t, mission, 3, "", "Primeira reserva")
	if err != nil {
		t.Fatal(err)
	}
	m.confirmReservation(t, m.vehicle.AssetID, first.ReservationID)
	// Another mission on the same vehicle and window: only the window leaves.
	overlapping := m.missionAt(t, d, m.vehicle.AssetID, start, start.Add(time.Hour))
	operation := onboardingID(t)
	_, _, err = m.reserveMission(t, overlapping, 3, operation, "Sobreposta")
	var conflict fa.ReservationConflict
	if !errors.As(err, &conflict) || !conflict.From.Equal(start) || conflict.To == nil || !conflict.To.Equal(start.Add(time.Hour)) {
		t.Fatal("overlap with a confirmed window", err)
	}
	if mc := m.counts(t, operation); mc != (missionCounts{}) {
		t.Fatal("a conflicting request stored state", mc)
	}
	op := m.missionStartOp(t, d, mission, m.declaredNow(t))
	ack, err := m.process(t, d, op)
	m.accepted(t, op, ack, err, 1, 1)
	if _, _, err := m.reserveMission(t, mission, 5, "", "Em execução"); !errors.Is(err, tripdomain.ErrInvalidTransition) {
		t.Fatal("reserve on an IN_PROGRESS mission", err)
	}
	if _, err := m.assignTo(t, mission, 5, d.person.employee); !errors.Is(err, tripdomain.ErrInvalidTransition) {
		t.Fatal("assign on an IN_PROGRESS mission", err)
	}
	cancelled, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: mission,
		ExpectedRevision: 5, Reason: "Cliente desistiu"}))
	if err != nil || cancelled.State != tripdomain.MissionCancelled || cancelled.Revision != 6 {
		t.Fatal("planner cancel of an IN_PROGRESS mission", cancelled, err)
	}
	finish := m.finishOp(t, d, tripOf(op), m.declaredNow(t).Add(time.Second), nil)
	done, err := m.process(t, d, finish)
	m.accepted(t, finish, done, err, 2, 1)
	if h := m.head(t, mission); h.state != "CANCELLED" || h.revision != 6 {
		t.Fatal("a completed trip changed its cancelled mission", h)
	}
}

// Forced RLS, insert-only links and the database proofs of B-BR-EMP-412..414.
func TestTripMissionReservationSchemaGuards(t *testing.T) {
	m := missionReservationFixture(t)
	d := m.syncFixtureState
	org := m.scope.OrgID().String()
	var forced bool
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_trip_mission_links'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("links without forced RLS", err)
	}
	start := exactNow(t, m.legalFixtureState).Truncate(time.Second).Add(10 * time.Minute)
	mission := m.missionAt(t, d, m.vehicle.AssetID, start, start.Add(time.Hour))
	reserved, _, err := m.reserveMission(t, mission, 3, "", "Reserva")
	if err != nil {
		t.Fatal(err)
	}
	m.confirmReservation(t, m.vehicle.AssetID, reserved.ReservationID)
	// A trip without a link (the start names no mission).
	plain := m.startOp(t, d, m.declaredNow(t), nil)
	ack, err := m.process(t, d, plain)
	m.accepted(t, plain, ack, err, 1, 1)
	trip := tripOf(plain)
	role := m.f.RuntimeRole
	if _, err := m.f.Admin.Exec(t.Context(), `GRANT UPDATE,DELETE,TRUNCATE ON org_trip_mission_links TO "`+role+`"`); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		query      string
		args       []any
		code, text string
	}{
		"fulfil-without-link": {`UPDATE org_asset_reservations SET state='FULFILLED',revision=revision+1,trip_id=$3 WHERE org_id=$1 AND id=$2`, []any{org, reserved.ReservationID, trip}, "23514", "reservation fulfilment source unavailable"},
		"mission-link-change": {`UPDATE org_asset_reservations SET mission_id=NULL,revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, reserved.ReservationID}, "55000", "reservation is immutable"},
		"link-by-another-operation": {`INSERT INTO org_trip_mission_links(org_id,trip_id,mission_id,operation_id) VALUES($1,$2,$3,$4)`,
			[]any{org, trip, mission, onboardingID(t)}, "23514", "mission link source unavailable"},
		"link-of-another-member": {`INSERT INTO org_trip_mission_links(org_id,trip_id,mission_id,operation_id) VALUES($1,$2,$3,$4)`,
			[]any{org, trip, m.missionAt(t, m.driver(t), m.vehicle.AssetID, start, start.Add(time.Hour)), plain.ID()}, "23514", "mission link source unavailable"},
		"trip-source-from-company": {`INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,assignee_employee_id,assignee_membership_id)
 VALUES($1,$2,5,'START','ACCEPTED','IN_PROGRESS','COMPANY',$3,$4,$5,$6,$7)`, []any{org, mission, m.actor.AccountID, m.member, onboardingID(t), d.person.employee, d.person.member}, "23514", ""},
		"start-without-link": {`INSERT INTO org_trip_mission_transitions(org_id,mission_id,revision,kind,from_state,to_state,source,actor_account_id,actor_membership_id,operation_id,assignee_employee_id,assignee_membership_id,trip_id)
 VALUES($1,$2,5,'START','ACCEPTED','IN_PROGRESS','TRIP',$3,$4,$5,$6,$4,$7)`, []any{org, mission, d.person.auth.Account.ID, d.person.member, plain.ID(), d.person.employee, trip}, "23514", "mission trip source unavailable"},
		"reservation-without-reserve": {`INSERT INTO org_asset_reservations(org_id,id,asset_id,purpose,starts_at,ends_at,no_show_tolerance,state,revision,operation_id,requested_by,mission_id)
 VALUES($1,$2,$3,'Direta',$4,$5,3600,'REQUESTED',1,$6,$7,$8)`, []any{org, onboardingID(t), m.vehicle.AssetID, start.Add(2 * time.Hour), start.Add(3 * time.Hour), onboardingID(t), m.actor.AccountID, mission}, "23514", "mission reservation source unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			planSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, c.query, c.args...), c.code, c.text)
		})
	}
	// Links are insert-only.
	for name, query := range map[string]string{
		"link-update":   `UPDATE org_trip_mission_links SET recorded_at=recorded_at WHERE org_id=$1`,
		"link-delete":   `DELETE FROM org_trip_mission_links WHERE org_id=$1`,
		"link-truncate": `TRUNCATE org_trip_mission_links`,
	} {
		t.Run(name, func(t *testing.T) {
			args := []any{org}
			if name == "link-truncate" {
				args = nil
			}
			planSchemaCode(t, unitSchemaExec(t, m.legalFixtureState, query, args...), "55000", "")
		})
	}
	var visible int
	if err := m.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_links`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("links visible without the organization setting", visible, err)
	}
}
