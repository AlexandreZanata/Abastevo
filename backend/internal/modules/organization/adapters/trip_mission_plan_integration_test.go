//go:build integration

package adapters_test

import (
	"errors"
	"sync"
	"testing"

	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// The one file-name constant of EMP-TRIP-009; the coordinator may renumber it.
const missionPlanMigration = "000231_organization_trip_mission_plans.sql"

func planText(v string) *string { return &v }

// cargoPlan is the BUC-EMP-192 plan: a text destination, a reason, the
// organization's cost centre and a cargo description.
func cargoPlan(centre string) *tripapp.MissionPlanFields {
	return &tripapp.MissionPlanFields{DestinationText: planText("Centro de distribuição Norte"), Reason: "Entrega de peças de reposição", CostCentreID: centre, Load: "CARGO",
		CargoDescription: planText("Peças de reposição")}
}

func (m missionState) costCentre(t *testing.T, state string) string {
	t.Helper()
	grantCostCentreTables(t, m.legalFixtureState)
	id := onboardingID(t)
	if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic client contract',$3,'ACTIVE')`, m.scope.OrgID().String(), id, "CTR-"+id[:8]); err != nil {
		t.Fatal(err)
	}
	if state == "ARCHIVED" {
		if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, m.scope.OrgID().String(), id); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func tripappCancel(mission string, revision int64) tripapp.MissionCommandInput {
	return tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: mission, ExpectedRevision: revision, Reason: "Cliente cancelou"}
}

func (m missionState) planWith(t *testing.T, actor tripapp.MissionActor, member, mission string, revision int64, plan *tripapp.MissionPlanFields) (tripapp.MissionAck, bool, error) {
	t.Helper()
	return m.run(t, actor, member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, MissionID: mission, ExpectedRevision: revision, Plan: plan}))
}

func (m missionState) plan(t *testing.T, mission string, revision int64, plan *tripapp.MissionPlanFields) (tripapp.MissionAck, error) {
	t.Helper()
	ack, _, err := m.planWith(t, ownerMissionActor(m.legalFixtureState), m.member, mission, revision, plan)
	return ack, err
}

type planCounts struct{ plans, transitions, receipts, audits int }

func (m missionState) planCounts(t *testing.T, operation string) planCounts {
	t.Helper()
	var c planCounts
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_mission_plans WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_trip_mission_commands WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, m.scope.OrgID().String(), operation).Scan(&c.plans, &c.transitions, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

// executionFacts is every field fact a plan must never touch: trips,
// finishes, readings, logbook events, review entries, reservations, blocks,
// review items and the assets.
func (m missionState) executionFacts(t *testing.T) string {
	t.Helper()
	var v string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_trips x WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(x) ORDER BY trip_id) FROM org_trip_finishes x WHERE org_id=$1),
(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_meter_readings x WHERE org_id=$1),(SELECT count(*) FROM org_trip_events WHERE org_id=$1),(SELECT count(*) FROM org_trip_review_entries WHERE org_id=$1),
(SELECT count(*) FROM org_asset_reservations WHERE org_id=$1),(SELECT count(*) FROM org_asset_blocks WHERE org_id=$1),(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1),
(SELECT jsonb_agg(to_jsonb(a)-'updated_at' ORDER BY id) FROM org_assets a WHERE org_id=$1))::text`, m.scope.OrgID().String()).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// B-BR-EMP-381..383/385 (AC, BUC-EMP-192/193): a planning revision records
// its authorized source, survives assignment, re-planning withdraws a
// pending assignment, an accepted or cancelled mission cannot be re-planned,
// a stale revision is CONFLICT, replay and reuse behave, references outside
// the organization are not found and archived ones ineligible, and no trip
// fact changes.
func TestTripMissionPlanRecordsSourceAndStaysApartFromExecution(t *testing.T) {
	m := missionFixture(t)
	org := m.scope.OrgID().String()
	d := m.syncFixtureState
	completedAt := m.declaredNow(t)
	start := m.startOp(t, d, completedAt, nil)
	ack, err := m.process(t, d, start)
	m.accepted(t, start, ack, err, 1, 1)
	end := m.finishOp(t, d, tripOf(start), completedAt.Add(1000), nil)
	ack, err = m.process(t, d, end)
	m.accepted(t, end, ack, err, 2, 1)
	centre := m.costCentre(t, "ACTIVE")
	south := m.secondUnit(t)
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	before := m.executionFacts(t)

	operation := onboardingID(t)
	input := tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: operation, MissionID: mission, ExpectedRevision: 1, Plan: cargoPlan(centre)}
	planned, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
	if err != nil || replayed || planned.Revision != 2 || planned.State != tripdomain.MissionPlanned || planned.AssigneeEmployeeID != "" || planned.Warning != nil {
		t.Fatal("plan of a draft", err, planned)
	}
	if c := m.planCounts(t, operation); c != (planCounts{plans: 1, transitions: 1, receipts: 1, audits: 1}) {
		t.Fatalf("plan not stored exactly once: %+v", c)
	}
	var source string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT p.planned_by::text||'/'||p.planned_by_membership_id::text||'/'||t.kind||'/'||t.source||'/'||a.action||'/'||c.result_state
 FROM org_trip_mission_plans p JOIN org_trip_mission_transitions t USING(org_id,mission_id,revision,operation_id)
 JOIN org_audit_events a ON(a.org_id,a.operation_id)=(p.org_id,p.operation_id) JOIN org_trip_mission_commands c ON(c.org_id,c.operation_id)=(p.org_id,p.operation_id)
 WHERE p.org_id=$1 AND p.operation_id=$2`, org, operation).Scan(&source); err != nil || source != m.actor.AccountID+"/"+m.member+"/PLAN/COMPANY/trip.mission.planned/PLANNED" {
		t.Fatal("the planning revision does not record its authorized source", source, err)
	}
	again, replayed, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, input))
	if err != nil || !replayed || again.Revision != 2 || again.State != tripdomain.MissionPlanned {
		t.Fatal("plan replay", err, again)
	}
	reused := input
	reused.Plan = cargoPlan("")
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, reused)); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("reused plan identifier", err)
	}
	if c := m.planCounts(t, operation); c != (planCounts{plans: 1, transitions: 1, receipts: 1, audits: 1}) {
		t.Fatalf("replay or reuse stored again: %+v", c)
	}
	if after := m.executionFacts(t); after != before {
		t.Fatal("a plan changed a trip, reading, event, review, reservation, block or asset")
	}

	t.Run("references", func(t *testing.T) {
		archivedCentre := m.costCentre(t, "ARCHIVED")
		archivedUnit := onboardingID(t)
		if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic West','WEST','UTC','ACTIVE')`, org, archivedUnit); err != nil {
			t.Fatal(err)
		}
		if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, archivedUnit); err != nil {
			t.Fatal(err)
		}
		other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
		var foreignUnit, foreignCentre string
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 LIMIT 1`, other.scope.OrgID().String()).Scan(&foreignUnit); err != nil {
			t.Fatal(err)
		}
		foreignCentre = onboardingID(t)
		grantCostCentreTables(t, other)
		if err := unitSchemaExec(t, other, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Foreign centre','FOREIGN','ACTIVE')`, other.scope.OrgID().String(), foreignCentre); err != nil {
			t.Fatal(err)
		}
		unitPlan := func(unit string) *tripapp.MissionPlanFields {
			return &tripapp.MissionPlanFields{DestinationUnitID: unit, Reason: "Transporte da equipe", Load: "PASSENGERS", Passengers: func() *int { n := 4; return &n }()}
		}
		for name, c := range map[string]struct {
			plan *tripapp.MissionPlanFields
			want error
		}{
			"foreign-unit":    {unitPlan(foreignUnit), tripapp.ErrNotFound},
			"unknown-unit":    {unitPlan(onboardingID(t)), tripapp.ErrNotFound},
			"archived-unit":   {unitPlan(archivedUnit), tripapp.ErrMissionIneligible},
			"foreign-centre":  {cargoPlan(foreignCentre), tripapp.ErrNotFound},
			"unknown-centre":  {cargoPlan(onboardingID(t)), tripapp.ErrNotFound},
			"archived-centre": {cargoPlan(archivedCentre), tripapp.ErrMissionIneligible},
			"stale-revision":  {cargoPlan(centre), tripdomain.ErrConflict},
			"unknown-mission": {cargoPlan(centre), tripapp.ErrNotFound},
			"future-revision": {cargoPlan(centre), tripdomain.ErrConflict},
		} {
			t.Run(name, func(t *testing.T) {
				target, revision := mission, int64(2)
				switch name {
				case "stale-revision":
					revision = 1
				case "future-revision":
					revision = 3
				case "unknown-mission":
					target = onboardingID(t)
				}
				op := onboardingID(t)
				_, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: op, MissionID: target, ExpectedRevision: revision, Plan: c.plan}))
				wantMissionError(t, err, c.want)
				if got := m.planCounts(t, op); got != (planCounts{}) {
					t.Fatalf("a refused plan left state: %+v", got)
				}
				if h := m.head(t, mission); h.revision != 2 || h.state != "PLANNED" {
					t.Fatal("a refused plan changed the mission", h)
				}
			})
		}
		if _, err := m.plan(t, mission, 2, unitPlan(south)); err != nil {
			t.Fatal("re-plan with another unit of the organization as destination", err)
		}
	})

	// Assignment keeps the plan; re-planning an ASSIGNED mission withdraws the
	// assignee, so the answer still in flight is FORBIDDEN (B-BR-EMP-356: not
	// the current assignee) and stores nothing.
	driver := m.driver(t)
	if _, err := m.assignTo(t, mission, 3, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	read, err := m.missions.ReadMission(t.Context(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, mission)
	if err != nil || read.Plan == nil || read.Plan.Revision != 3 || read.Plan.Fields.DestinationUnitID != south || read.Plan.Fields.Load != "PASSENGERS" || read.State != tripdomain.MissionAssigned {
		t.Fatal("the plan does not survive assignment", err, read.Plan)
	}
	pending := m.respondOp(t, driver, mission, 4, "ACCEPT", nil, nil)
	replanned, err := m.plan(t, mission, 4, cargoPlan(centre))
	if err != nil || replanned.State != tripdomain.MissionPlanned || replanned.Revision != 5 {
		t.Fatal("re-plan of an assigned mission", err, replanned)
	}
	if h := m.head(t, mission); h.state != "PLANNED" || h.assignee != "" || h.member != "" || h.answer != "" {
		t.Fatal("re-planning did not withdraw the assignment", h)
	}
	ack, err = m.deliver(t, driver, pending)
	m.denied(t, pending, ack, err, orgapi.Forbidden)
	mine, err := m.missions.ListMissions(t.Context(), m.scope, personMissionActor(driver.person), driver.person.member, true, tripapp.MissionFilter{Limit: 10})
	if err != nil || len(mine.Missions) != 0 {
		t.Fatal("a withdrawn assignment stays in the member's own list", err, len(mine.Missions))
	}
	if _, err := m.assignTo(t, mission, 5, driver.person.employee); err != nil {
		t.Fatal("the same member is assigned again after re-planning", err)
	}
	mine, err = m.missions.ListMissions(t.Context(), m.scope, personMissionActor(driver.person), driver.person.member, true, tripapp.MissionFilter{Limit: 10})
	if err != nil || len(mine.Missions) != 1 || mine.Missions[0].Plan == nil || mine.Missions[0].Plan.Revision != 5 || *mine.Missions[0].Plan.Fields.DestinationText != "Centro de distribuição Norte" {
		t.Fatal("the assignee's own list shows the current plan", err)
	}
	accept := m.respondOp(t, driver, mission, 6, "ACCEPT", nil, nil)
	if ack, err := m.deliver(t, driver, accept); err != nil || ack.Result() != protocol.Accepted {
		t.Fatal("acceptance of the planned mission", err)
	}
	// An accepted plan is not rewritten; a cancelled mission is terminal.
	op := onboardingID(t)
	_, _, err = m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: op, MissionID: mission, ExpectedRevision: 7, Plan: cargoPlan("")}))
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	if c := m.planCounts(t, op); c != (planCounts{}) {
		t.Fatalf("a refused re-plan of an accepted mission left state: %+v", c)
	}
	if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: mission, ExpectedRevision: 7, Reason: "Cliente cancelou"})); err != nil {
		t.Fatal(err)
	}
	_, err = m.plan(t, mission, 8, cargoPlan(""))
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	var history, plans string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT string_agg(kind||':'||COALESCE(from_state,'-')||'>'||to_state,',' ORDER BY revision) FROM org_trip_mission_transitions WHERE org_id=$1 AND mission_id=$2`, org, mission).Scan(&history); err != nil ||
		history != "CREATE:->DRAFT,PLAN:DRAFT>PLANNED,PLAN:PLANNED>PLANNED,ASSIGN:PLANNED>ASSIGNED,PLAN:ASSIGNED>PLANNED,ASSIGN:PLANNED>ASSIGNED,ACCEPT:ASSIGNED>ACCEPTED,CANCEL:ACCEPTED>CANCELLED" {
		t.Fatal("transition history", history, err)
	}
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT string_agg(revision||':'||load_kind,',' ORDER BY revision) FROM org_trip_mission_plans WHERE org_id=$1 AND mission_id=$2`, org, mission).Scan(&plans); err != nil ||
		plans != "2:CARGO,3:PASSENGERS,5:CARGO" {
		t.Fatal("planning revisions are not kept", plans, err)
	}
	read, err = m.missions.ReadMission(t.Context(), m.scope, ownerMissionActor(m.legalFixtureState), m.member, mission)
	if err != nil || read.State != tripdomain.MissionCancelled || read.Plan == nil || read.Plan.Revision != 5 {
		t.Fatal("the cancelled mission keeps its last plan", err)
	}
	if after := m.executionFacts(t); after != before {
		t.Fatal("planning, assignment or the answer changed a field fact")
	}
}

// B-BR-EMP-384, AUTH-006, BR-TEN: a UNIT-scoped planner plans only missions of
// its unit; another organization's mission is not found; AUDITOR and DRIVER
// hold no planning key.
func TestTripMissionPlanScopeIsolationAndAuditor(t *testing.T) {
	m := missionFixture(t)
	south := m.secondUnit(t)
	north := m.create(t, m.unit, "").MissionID
	southern := m.create(t, south, "").MissionID
	manager := m.driver(t)
	matrixAssign(t, m.deviceFixtureState, matrixPerson{member: manager.person.member}, "FLEET_MANAGER", "UNIT", "ACTIVE")
	actor := personMissionActor(manager.person)
	if _, _, err := m.planWith(t, actor, manager.person.member, north, 1, cargoPlan("")); err != nil {
		t.Fatal("unit planner plans a mission of its unit", err)
	}
	_, _, err := m.planWith(t, actor, manager.person.member, southern, 1, cargoPlan(""))
	wantMissionError(t, err, permission.ErrPermissionDenied)
	for _, role := range []string{"AUDITOR", "DRIVER"} {
		person := m.driver(t)
		scope := "ORG"
		if role == "DRIVER" {
			scope = "SELF"
		}
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: person.person.member}, role, scope, "ACTIVE")
		_, _, err := m.planWith(t, personMissionActor(person.person), person.person.member, north, 2, cargoPlan(""))
		wantMissionError(t, err, permission.ErrPermissionDenied)
	}
	if h := m.head(t, southern); h.revision != 1 || h.state != "DRAFT" {
		t.Fatal("an out-of-scope plan changed the mission", h)
	}
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	_, _, err = m.missions.CommandMission(t.Context(), other.scope, ownerMissionActor(other), other.member, "", missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, MissionID: north, ExpectedRevision: 2, Plan: cargoPlan("")}))
	wantMissionError(t, err, tripapp.ErrNotFound)
	if h := m.head(t, north); h.revision != 2 || h.state != "PLANNED" {
		t.Fatal("another organization's plan changed the mission", h)
	}
}

// 08-state-model §4.7, B-BR-EMP-385: a plan and an assignment decided on one
// revision race behind the held root; the first queued wins, the other is
// CONFLICT and stores nothing, in both orders.
func TestTripMissionPlanRacesAssignment(t *testing.T) {
	m := missionFixture(t)
	driver := m.driver(t)
	for _, order := range []string{"plan-first", "assignment-first"} {
		t.Run(order, func(t *testing.T) {
			mission := m.create(t, m.unit, "").MissionID
			planOp := onboardingID(t)
			var planErr, assignErr error
			plan := func() {
				_, _, planErr = m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionPlanCommand, OperationID: planOp, MissionID: mission, ExpectedRevision: 1, Plan: cargoPlan("")}))
			}
			assign := func() { _, assignErr = m.assignTo(t, mission, 1, driver.person.employee) }
			calls := []func(){plan, assign}
			if order == "assignment-first" {
				calls[0], calls[1] = assign, plan
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
			h := m.head(t, mission)
			if order == "plan-first" {
				if planErr != nil || !errors.Is(assignErr, tripdomain.ErrConflict) || h.state != "PLANNED" || h.revision != 2 || h.assignee != "" {
					t.Fatal("plan-first outcome", planErr, assignErr, h)
				}
				return
			}
			if assignErr != nil || !errors.Is(planErr, tripdomain.ErrConflict) || h.state != "ASSIGNED" || h.revision != 2 || h.assignee != driver.person.employee {
				t.Fatal("assignment-first outcome", planErr, assignErr, h)
			}
			if c := m.planCounts(t, planOp); c != (planCounts{}) {
				t.Fatalf("the losing plan stored state: %+v", c)
			}
		})
	}
}
