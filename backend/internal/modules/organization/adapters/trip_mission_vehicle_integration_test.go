//go:build integration

package adapters_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// The one file-name constant of EMP-TRIP-013; the coordinator may renumber it.
const missionVehicleMigration = "000232_organization_trip_mission_vehicles.sql"

// otherVehicle registers, meters and activates another vehicle of the unit
// and, when employee is set, assigns it to that employee (FLEET-006).
func (m missionState) otherVehicle(t *testing.T, employee string) fa.MeterInstallResult {
	t.Helper()
	input := fleetCreateInput()
	id := onboardingID(t)
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID, input.Profile.SerialNumber = fleet.Vehicle, fleet.Distance, m.unit, "SYNTHETIC-V-"+id[:8]
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, m.legalFixtureState, m.assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, m.legalFixtureState, m.meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, m.legalFixtureState).Add(-6*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if active, _, err := fleetWrite(t, m.legalFixtureState, m.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil || active.State != fleet.Active {
		t.Fatal("second vehicle not active", err)
	}
	if employee != "" {
		m.assign(t, employee, created.AssetID, fleet.PrimaryAssignment, nil, nil)
	}
	return installed
}

// startOn is member d's trip.start of a new trip on the vehicle with its own
// meter's reading.
func (m missionState) startOn(t *testing.T, d syncFixtureState, vehicle fa.MeterInstallResult, value string) protocol.Operation {
	t.Helper()
	reading := map[string]any{"meter_id": vehicle.MeterID, "segment_id": vehicle.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": value}
	payload := map[string]any{"asset_id": vehicle.AssetID, "driver_employee_id": d.person.employee, "reading": reading, "missing_reading_reason": nil, "run_clock": nil}
	return m.operation(t, d, tripapp.StartOperationType, onboardingID(t), m.declaredNow(t), payload, nil)
}

func vehicleCommand(mission string, revision int64, asset, reason string) tripapp.MissionCommandInput {
	return tripapp.MissionCommandInput{Kind: tripapp.MissionVehicleCommand, MissionID: mission, ExpectedRevision: revision, AssetID: asset, Reason: reason}
}

func (m missionState) changeVehicle(t *testing.T, mission string, revision int64, asset string) (tripapp.MissionAck, error) {
	t.Helper()
	ack, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, vehicleCommand(mission, revision, asset, "Veículo em manutenção")))
	return ack, err
}

type vehicleCounts struct{ records, transitions, receipts, audits int }

func (m missionState) vehicleCounts(t *testing.T, operation string) vehicleCounts {
	t.Helper()
	var c vehicleCounts
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_mission_vehicles WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_trip_mission_commands WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, m.scope.OrgID().String(), operation).Scan(&c.records, &c.transitions, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

// assetEvidence is everything stored for one vehicle's work: its trips,
// finishes, meters, segments and readings.
func (m missionState) assetEvidence(t *testing.T, asset string) string {
	t.Helper()
	var v string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_trips x WHERE org_id=$1 AND asset_id=$2),
(SELECT jsonb_agg(to_jsonb(f) ORDER BY f.trip_id) FROM org_trip_finishes f JOIN org_trips x ON(x.org_id,x.id)=(f.org_id,f.trip_id) WHERE f.org_id=$1 AND x.asset_id=$2),
(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_meters x WHERE org_id=$1 AND asset_id=$2),
(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM org_meter_readings r JOIN org_meters x ON(x.org_id,x.id)=(r.org_id,r.meter_id) WHERE r.org_id=$1 AND x.asset_id=$2))::text`,
		m.scope.OrgID().String(), asset).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (m missionState) scopedGrant(t *testing.T, member, role, scope, target string) {
	t.Helper()
	org := m.scope.OrgID().String()
	if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, member); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp(),NULL,$7,'Synthetic scoped grant')`,
		org, onboardingID(t), member, role, scope, target, m.member); err != nil {
		t.Fatal(err)
	}
}

// AC (B-BR-EMP-387/388/389, BUC-EMP-195): an accepted mission's vehicle is
// replaced only after the assignee's running trip on it is finished with its
// final reading; the mission stays ACCEPTED with its assignee and plan; the
// first vehicle's trips, meters and readings are unchanged; the next trip
// starts on the new vehicle with its own reading; the database refuses moving
// a reading or a trip to the other asset; replay, reuse, stale revisions and
// an answer in flight behave.
func TestTripMissionVehicleChangeKeepsEachVehicleEvidence(t *testing.T) {
	m := missionFixture(t)
	org := m.scope.OrgID().String()
	d := m.syncFixtureState
	second := m.otherVehicle(t, m.person.employee)
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.plan(t, mission, 1, cargoPlan("")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.assignTo(t, mission, 2, m.person.employee); err != nil {
		t.Fatal(err)
	}
	if ack, err := m.deliver(t, d, m.respondOp(t, d, mission, 3, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("acceptance", err)
	}
	running := m.startOp(t, d, m.declaredNow(t), nil)
	ack, err := m.process(t, d, running)
	m.accepted(t, running, ack, err, 1, 1)

	refused := onboardingID(t)
	input := vehicleCommand(mission, 4, second.AssetID, "Veículo em manutenção")
	input.OperationID = refused
	_, _, err = m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
	wantMissionError(t, err, tripapp.ErrMissionTripInProgress)
	if c := m.vehicleCounts(t, refused); c != (vehicleCounts{}) {
		t.Fatalf("a refused change left state: %+v", c)
	}
	finish := m.finishOp(t, d, tripOf(running), m.declaredNow(t), nil)
	ack, err = m.process(t, d, finish)
	m.accepted(t, finish, ack, err, 2, 1)
	first := m.assetEvidence(t, m.vehicle.AssetID)

	operation := onboardingID(t)
	input.OperationID = operation
	changed, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
	if err != nil || replayed || changed.Revision != 5 || changed.State != tripdomain.MissionAccepted || changed.AssigneeEmployeeID != m.person.employee {
		t.Fatal("vehicle change of an accepted mission", err, changed)
	}
	if c := m.vehicleCounts(t, operation); c != (vehicleCounts{records: 1, transitions: 1, receipts: 1, audits: 1}) {
		t.Fatalf("change not stored exactly once: %+v", c)
	}
	var record string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT v.previous_asset_id::text||'>'||v.asset_id::text||'/'||v.reason||'/'||v.changed_by::text||'/'||t.kind||':'||t.from_state||'>'||t.to_state||'/'||a.action
 FROM org_trip_mission_vehicles v JOIN org_trip_mission_transitions t USING(org_id,mission_id,revision,operation_id) JOIN org_audit_events a ON(a.org_id,a.operation_id)=(v.org_id,v.operation_id)
 WHERE v.org_id=$1 AND v.operation_id=$2`, org, operation).Scan(&record); err != nil ||
		record != m.vehicle.AssetID+">"+second.AssetID+"/Veículo em manutenção/"+m.actor.AccountID+"/VEHICLE:ACCEPTED>ACCEPTED/trip.mission.vehicle_changed" {
		t.Fatal("the change record", record, err)
	}
	if h := m.head(t, mission); h.state != "ACCEPTED" || h.revision != 5 || h.member != m.person.member || h.answer != "ACCEPT" {
		t.Fatal("the change touched state, assignee or answer", h)
	}
	read, err := m.missions.ReadMission(t.Context(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, mission)
	if err != nil || read.AssetID != second.AssetID || read.Plan == nil || read.Plan.Revision != 2 {
		t.Fatal("the mission reads its new vehicle and keeps its plan", err)
	}
	again, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
	if err != nil || !replayed || again.Revision != 5 {
		t.Fatal("change replay", err)
	}
	reused := vehicleCommand(mission, 4, second.AssetID, "Outro motivo")
	reused.OperationID = operation
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, reused)); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("reused change identifier", err)
	}
	_, err = m.changeVehicle(t, mission, 5, second.AssetID)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	_, err = m.changeVehicle(t, mission, 4, m.vehicle.AssetID)
	wantMissionError(t, err, tripdomain.ErrConflict)

	// The next trip starts on the new vehicle with its own reading; the first
	// vehicle's evidence is untouched and the meter histories stay apart.
	next := m.startOn(t, d, second, "1000")
	ack, err = m.process(t, d, next)
	m.accepted(t, next, ack, err, 1, 1)
	if m.assetEvidence(t, m.vehicle.AssetID) != first {
		t.Fatal("the change or the new trip touched the first vehicle's evidence")
	}
	var mixed int
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trips x JOIN org_meter_readings r ON r.org_id=x.org_id AND r.operation_id=x.operation_id JOIN org_meters mt ON(mt.org_id,mt.id)=(r.org_id,r.meter_id)
 WHERE x.org_id=$1 AND mt.asset_id<>x.asset_id`, org).Scan(&mixed); err != nil || mixed != 0 {
		t.Fatal("a trip reading belongs to another vehicle's meter", mixed, err)
	}
	if _, err := m.f.Admin.Exec(t.Context(), "GRANT UPDATE ON org_meter_readings,org_trips TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Run("reading-reassigned-to-another-asset", func(t *testing.T) {
		var readings int
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2 AND meter_id=$3`, org, finish.ID(), m.vehicle.MeterID).Scan(&readings); err != nil || readings != 1 {
			t.Fatal("the first vehicle's final reading is not where the test moves it from", readings, err)
		}
		err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_meter_readings SET meter_id=$2,segment_id=$3 WHERE org_id=$1 AND operation_id=$4`, org, second.MeterID, second.SegmentID, finish.ID())
		planSchemaCode(t, err, "55000", "")
	})
	t.Run("trip-reassigned-to-another-asset", func(t *testing.T) {
		err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_trips SET asset_id=$2 WHERE org_id=$1 AND id=$3`, org, second.AssetID, tripOf(running))
		planSchemaCode(t, err, "55000", "")
	})

	// The new vehicle runs the trip: another mission cannot take it (the
	// one-active-trip rule), and an answer in flight is CONFLICT after a change.
	other := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	driver := m.driver(t)
	if _, err := m.assignTo(t, other, 1, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	_, err = m.changeVehicle(t, other, 2, second.AssetID)
	wantMissionError(t, err, tripapp.ErrMissionAssetBusy)
	inFlight := m.respondOp(t, driver, other, 2, "ACCEPT", nil, nil)
	third := m.otherVehicle(t, "")
	if ack, err := m.changeVehicle(t, other, 2, third.AssetID); err != nil || ack.State != tripdomain.MissionAssigned || ack.Revision != 3 {
		t.Fatal("change of an assigned mission", err)
	}
	ack, err = m.deliver(t, driver, inFlight)
	m.denied(t, inFlight, ack, err, orgapi.Conflict)
	if ack, err := m.deliver(t, driver, m.respondOp(t, driver, other, 3, "ACCEPT", nil, nil)); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("the assignee answers the current revision", err)
	}
	var history string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT string_agg(kind||':'||COALESCE(from_state,'-')||'>'||to_state,',' ORDER BY revision) FROM org_trip_mission_transitions WHERE org_id=$1 AND mission_id=$2`, org, mission).Scan(&history); err != nil ||
		history != "CREATE:->DRAFT,PLAN:DRAFT>PLANNED,ASSIGN:PLANNED>ASSIGNED,ACCEPT:ASSIGNED>ACCEPTED,VEHICLE:ACCEPTED>ACCEPTED" {
		t.Fatal("transition history", history, err)
	}
}

// B-BR-EMP-387: the new vehicle must be an ACTIVE main asset of the
// organization, not blocked at the planned start; a DRAFT mission keeps its
// creation vehicle until planned; a planned mission without a vehicle gets
// one; a cancelled mission is terminal; a current assignee must
// still cover the mission with the new vehicle; the planner must cover the
// mission before and after; another organization is not found.
func TestTripMissionVehicleChangeRefusals(t *testing.T) {
	m := missionFixture(t)
	org := m.scope.OrgID().String()
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.plan(t, mission, 1, cargoPlan("")); err != nil {
		t.Fatal(err)
	}
	free := m.otherVehicle(t, "")
	blocked := m.otherVehicle(t, "")
	if _, err := blockAsset(t, m.legalFixtureState, m.assets, fa.BlockCreateInput{AssetID: blocked.AssetID, Reason: fleet.AdministrativeBlock, Note: "Synthetic administrative block"}); err != nil {
		t.Fatal(err)
	}
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	foreign, _, err := fleetWrite(t, other, m.assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		asset string
		want  error
	}{
		"unknown":         {onboardingID(t), tripapp.ErrNotFound},
		"other-org":       {foreign.AssetID, tripapp.ErrNotFound},
		"draft":           {m.draftAsset(t), tripapp.ErrMissionIneligible},
		"implement":       {m.installed.AssetID, tripapp.ErrMissionIneligible},
		"blocked":         {blocked.AssetID, tripapp.ErrMissionIneligible},
		"current-vehicle": {m.vehicle.AssetID, tripdomain.ErrInvalidTransition},
	} {
		t.Run(name, func(t *testing.T) {
			op := onboardingID(t)
			input := vehicleCommand(mission, 2, c.asset, "Troca")
			input.OperationID = op
			_, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
			wantMissionError(t, err, c.want)
			if got := m.vehicleCounts(t, op); got != (vehicleCounts{}) {
				t.Fatalf("a refused change left state: %+v", got)
			}
		})
	}
	if h := m.head(t, mission); h.revision != 2 {
		t.Fatal("a refused change moved the mission", h)
	}
	unassigned := m.create(t, m.unit, "").MissionID
	_, err = m.changeVehicle(t, unassigned, 1, free.AssetID)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	if _, err := m.plan(t, unassigned, 1, cargoPlan("")); err != nil {
		t.Fatal(err)
	}
	if ack, err := m.changeVehicle(t, unassigned, 2, free.AssetID); err != nil || ack.State != tripdomain.MissionPlanned || ack.Revision != 3 {
		t.Fatal("a planned mission without a vehicle gets one", err)
	}
	var previous *string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT previous_asset_id::text FROM org_trip_mission_vehicles WHERE org_id=$1 AND mission_id=$2`, org, unassigned).Scan(&previous); err != nil || previous != nil {
		t.Fatal("the first vehicle has no previous one", err)
	}
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripappCancel(unassigned, 3))); err != nil {
		t.Fatal(err)
	}
	_, err = m.changeVehicle(t, unassigned, 4, m.vehicle.AssetID)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)

	t.Run("assignee-must-cover-the-new-vehicle", func(t *testing.T) {
		scoped := m.driver(t)
		m.scopedGrant(t, scoped.person.member, "DRIVER", "ASSET", m.vehicle.AssetID)
		target := m.create(t, m.unit, m.vehicle.AssetID).MissionID
		if _, err := m.assignTo(t, target, 1, scoped.person.employee); err != nil {
			t.Fatal("an asset-scoped driver is assigned a mission of its asset", err)
		}
		_, err := m.changeVehicle(t, target, 2, free.AssetID)
		wantMissionError(t, err, tripapp.ErrMissionIneligible)
	})
	t.Run("planner-must-cover-before-and-after", func(t *testing.T) {
		planner := m.driver(t)
		m.scopedGrant(t, planner.person.member, "FLEET_MANAGER", "ASSET", m.vehicle.AssetID)
		target := m.create(t, m.unit, m.vehicle.AssetID).MissionID
		if _, err := m.plan(t, target, 1, cargoPlan("")); err != nil {
			t.Fatal(err)
		}
		actor := personMissionActor(planner.person)
		if _, err := m.missions.ReadMission(t.Context(), m.scope, actor, planner.person.member, target); err != nil {
			t.Fatal("the asset-scoped planner reads a mission of its asset", err)
		}
		_, _, err := m.run(t, actor, planner.person.member, missionCommand(t, vehicleCommand(target, 2, free.AssetID, "Troca")))
		wantMissionError(t, err, permission.ErrPermissionDenied)
		unit := m.driver(t)
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: unit.person.member}, "FLEET_MANAGER", "UNIT", "ACTIVE")
		if _, _, err := m.run(t, personMissionActor(unit.person), unit.person.member, missionCommand(t, vehicleCommand(target, 2, free.AssetID, "Troca"))); err != nil {
			t.Fatal("the unit planner changes a vehicle of its unit", err)
		}
		auditor := m.driver(t)
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: auditor.person.member}, "AUDITOR", "ORG", "ACTIVE")
		_, _, err = m.run(t, personMissionActor(auditor.person), auditor.person.member, missionCommand(t, vehicleCommand(target, 3, m.vehicle.AssetID, "Troca")))
		wantMissionError(t, err, permission.ErrPermissionDenied)
	})
	t.Run("other-organization", func(t *testing.T) {
		_, _, err := m.missions.CommandMission(t.Context(), other.scope, ownerMissionActor(other), other.member, "", missionCommand(t, vehicleCommand(mission, 2, free.AssetID, "Troca")))
		wantMissionError(t, err, tripapp.ErrNotFound)
	})
}

// 08-state-model §4.7: a vehicle change and an assignment decided on one
// revision race behind the held root; the first queued wins and the other is
// CONFLICT with nothing stored, in both orders.
func TestTripMissionVehicleChangeRacesAssignment(t *testing.T) {
	m := missionFixture(t)
	driver := m.driver(t)
	free := m.otherVehicle(t, "")
	for _, order := range []string{"change-first", "assignment-first"} {
		t.Run(order, func(t *testing.T) {
			mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
			if _, err := m.plan(t, mission, 1, cargoPlan("")); err != nil {
				t.Fatal(err)
			}
			op := onboardingID(t)
			var changeErr, assignErr error
			change := func() {
				input := vehicleCommand(mission, 2, free.AssetID, "Troca")
				input.OperationID = op
				_, _, changeErr = m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
			}
			assign := func() { _, assignErr = m.assignTo(t, mission, 2, driver.person.employee) }
			calls := []func(){change, assign}
			if order == "assignment-first" {
				calls[0], calls[1] = assign, change
			}
			release := holdRoot(t, m)
			t.Cleanup(release)
			var wg sync.WaitGroup
			for i, call := range calls {
				wg.Go(call)
				waitLockWaiters(t, m, i+1)
			}
			release()
			wg.Wait()
			var asset string
			if err := m.f.Admin.QueryRow(t.Context(), `SELECT asset_id::text FROM org_trip_missions WHERE org_id=$1 AND id=$2`, m.scope.OrgID().String(), mission).Scan(&asset); err != nil {
				t.Fatal(err)
			}
			h := m.head(t, mission)
			if order == "change-first" {
				if changeErr != nil || !errors.Is(assignErr, tripdomain.ErrConflict) || h.revision != 3 || h.state != "PLANNED" || asset != free.AssetID {
					t.Fatal("change-first outcome", changeErr, assignErr, h)
				}
				return
			}
			if assignErr != nil || !errors.Is(changeErr, tripdomain.ErrConflict) || h.revision != 3 || h.state != "ASSIGNED" || asset != m.vehicle.AssetID {
				t.Fatal("assignment-first outcome", changeErr, assignErr, h)
			}
			if c := m.vehicleCounts(t, op); c != (vehicleCounts{}) {
				t.Fatalf("the losing change stored state: %+v", c)
			}
		})
	}
}
