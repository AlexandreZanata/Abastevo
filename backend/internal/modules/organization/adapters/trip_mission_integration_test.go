//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// The one file-name constant of EMP-TRIP-008; the coordinator may renumber it.
const missionMigration = "000227_organization_trip_missions.sql"

// grantMissionTables gives the ordinary runtime role exactly the EMP-TRIP-008
// table rights and, once migrations 000231, 000232 and 000256 exist, the
// EMP-TRIP-009 planning revisions, the EMP-TRIP-013 vehicle changes and the
// EMP-TRIP-010 planned stops (the 000227 and 000231 rehearsals run before
// them).
func grantMissionTables(t *testing.T, fx legalFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_trip_missions TO "+role+";GRANT SELECT,INSERT ON org_trip_mission_transitions,org_trip_mission_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	var plans, vehicles, stops bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_trip_mission_plans') IS NOT NULL,to_regclass('org_trip_mission_vehicles') IS NOT NULL,
 to_regclass('org_trip_mission_plan_stops') IS NOT NULL`).Scan(&plans, &vehicles, &stops); err != nil {
		t.Fatal(err)
	}
	if plans {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_mission_plans TO "+role+";GRANT SELECT ON org_cost_centres TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	if vehicles {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_mission_vehicles TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	// EMP-TRIP-010: every plan read attaches the planned stops (000256).
	if stops {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_mission_plan_stops TO "+role); err != nil {
			t.Fatal(err)
		}
	}
}

// missionState is the trip fixture (ACTIVE vehicle assigned to the ADMIN
// device member d, unit, signed intake) plus the real mission store and the
// trip.mission.respond owner.
type missionState struct {
	tripState
	missions *trippg.MissionStore
	respond  syncapp.Handler
	intake   *syncpg.Store
}

func missionStoreFor(t *testing.T, fx legalFixtureState, x tripState) *trippg.MissionStore {
	t.Helper()
	grantMissionTables(t, fx)
	catalog, err := audit.NewCatalog(trippg.MissionAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	structure := &orgpg.ResourceStructure{Kind: orgpg.MissionStructure, Pool: fx.f.Runtime}
	authority := &orgpg.MissionAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Structure: structure}
	return &trippg.MissionStore{Pool: fx.f.Runtime, Authority: authority, Audit: auditpg.NewStore(fx.f.Runtime, catalog), Availability: x.assets}
}

func missionFixture(t *testing.T) missionState {
	t.Helper()
	return missionFixtureOn(t, tripFixture(t))
}

func missionFixtureOn(t *testing.T, x tripState) missionState {
	t.Helper()
	m := missionState{tripState: x, missions: missionStoreFor(t, x.legalFixtureState, x)}
	var err error
	if m.respond, err = trippg.NewMissionResponseHandler(x.authority, &trippg.MissionStore{}); err != nil {
		t.Fatal(err)
	}
	config := tripIntakeConfig(x.syncFixtureState)
	config.NoAsyncPlans[syncapp.OperationKey{Type: tripapp.MissionRespondOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	m.intake = syncpg.NewStore(config)
	return m
}

func ownerMissionActor(fx legalFixtureState) tripapp.MissionActor {
	return tripapp.MissionActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}

func personMissionActor(p devicePerson) tripapp.MissionActor {
	return tripapp.MissionActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}
}

func missionCommand(t *testing.T, v tripapp.MissionCommandInput) tripapp.MissionCommand {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := tripapp.NewMissionCommand(v)
	if err != nil {
		t.Fatal("invalid fixture mission command", err)
	}
	return c
}

func (m missionState) run(t *testing.T, actor tripapp.MissionActor, member string, c tripapp.MissionCommand) (tripapp.MissionAck, bool, error) {
	t.Helper()
	v, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	if v.Kind == tripapp.MissionCreate {
		id = onboardingID(t)
	}
	return m.missions.CommandMission(t.Context(), m.scope, actor, member, id, c)
}

// window is a planned window one to three hours after the database clock.
func (m missionState) window(t *testing.T) (time.Time, time.Time) {
	t.Helper()
	start := lifecycleDBNow(t, m.legalFixtureState).Truncate(time.Second).Add(time.Hour)
	return start, start.Add(2 * time.Hour)
}

func (m missionState) create(t *testing.T, unit, asset string) tripapp.MissionAck {
	t.Helper()
	start, end := m.window(t)
	ack, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCreate, UnitID: unit, AssetID: asset,
		Title: "Entrega sintética no cliente", StartsAt: start, EndsAt: end}))
	if err != nil {
		t.Fatal("mission creation", err)
	}
	return ack
}

func (m missionState) assignTo(t *testing.T, mission string, revision int64, employee string) (tripapp.MissionAck, error) {
	t.Helper()
	ack, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionAssignCommand, MissionID: mission,
		ExpectedRevision: revision, AssigneeEmployeeID: employee}))
	return ack, err
}

type missionHead struct {
	state, assignee, member, answer string
	revision                        int64
	reason                          *string
}

func (m missionState) head(t *testing.T, mission string) missionHead {
	t.Helper()
	var h missionHead
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT state,revision,COALESCE(assignee_employee_id::text,''),COALESCE(assignee_membership_id::text,''),COALESCE(answer,''),answer_reason FROM org_trip_missions WHERE org_id=$1 AND id=$2`,
		m.scope.OrgID().String(), mission).Scan(&h.state, &h.revision, &h.assignee, &h.member, &h.answer, &h.reason); err != nil {
		t.Fatal(err)
	}
	return h
}

type missionCounts struct{ heads, transitions, receipts, audits int }

func (m missionState) counts(t *testing.T, operation string) missionCounts {
	t.Helper()
	var c missionCounts
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_missions WHERE org_id=$1 AND create_operation_id=$2),(SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_trip_mission_commands WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, m.scope.OrgID().String(), operation).Scan(&c.heads, &c.transitions, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

// secondUnit adds an ACTIVE operating unit to the organization.
func (m missionState) secondUnit(t *testing.T) string {
	t.Helper()
	unit := onboardingID(t)
	if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic South','SOUTH','UTC','ACTIVE')`, m.scope.OrgID().String(), unit); err != nil {
		t.Fatal(err)
	}
	return unit
}

func wantMissionError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

// B-BR-EMP-353..355/358: create, assign, reassign and cancel through the
// real store with exact replay, reuse, stale revisions and eligibility.
func TestTripMissionCommandsReplayConflictsAndEligibility(t *testing.T) {
	m := missionFixture(t)
	owner, org := ownerMissionActor(m.legalFixtureState), m.scope.OrgID().String()
	start, end := m.window(t)
	createInput := tripapp.MissionCommandInput{Kind: tripapp.MissionCreate, OperationID: onboardingID(t), UnitID: m.unit, AssetID: m.vehicle.AssetID, Title: "Entrega sintética", StartsAt: start, EndsAt: end}
	created, replayed, err := m.run(t, owner, m.member, missionCommand(t, createInput))
	if err != nil || replayed || created.Revision != 1 || created.State != tripdomain.MissionDraft || created.Warning != nil {
		t.Fatal("mission creation", err, created.State)
	}
	if c := m.counts(t, createInput.OperationID); c != (missionCounts{heads: 1, transitions: 1, receipts: 1, audits: 1}) {
		t.Fatalf("creation not stored exactly once: %+v", c)
	}
	again, replayed, err := m.missions.CommandMission(t.Context(), m.scope, owner, m.member, onboardingID(t), missionCommand(t, createInput))
	if err != nil || !replayed || again.MissionID != created.MissionID || again.Revision != 1 {
		t.Fatal("creation replay did not return the first result", err)
	}
	reused := createInput
	reused.Title = "Outra intenção"
	if _, _, err := m.run(t, owner, m.member, missionCommand(t, reused)); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("reused creation identifier", err)
	}
	if c := m.counts(t, createInput.OperationID); c != (missionCounts{heads: 1, transitions: 1, receipts: 1, audits: 1}) {
		t.Fatalf("replay or reuse stored again: %+v", c)
	}
	t.Run("creation-refusals", func(t *testing.T) {
		for name, edit := range map[string]func(*tripapp.MissionCommandInput) error{
			"unknown-unit":  func(v *tripapp.MissionCommandInput) error { v.UnitID = onboardingID(t); return tripapp.ErrNotFound },
			"unknown-asset": func(v *tripapp.MissionCommandInput) error { v.AssetID = onboardingID(t); return tripapp.ErrNotFound },
			"implement-asset": func(v *tripapp.MissionCommandInput) error {
				v.AssetID = m.installed.AssetID
				return tripapp.ErrMissionIneligible
			},
			"draft-asset": func(v *tripapp.MissionCommandInput) error {
				v.AssetID = m.draftAsset(t)
				return tripapp.ErrMissionIneligible
			},
			"other-organization-unit": func(v *tripapp.MissionCommandInput) error { v.UnitID = m.foreignUnit(t); return tripapp.ErrNotFound },
		} {
			t.Run(name, func(t *testing.T) {
				v := createInput
				v.OperationID = onboardingID(t)
				want := edit(&v)
				_, _, err := m.run(t, owner, m.member, missionCommand(t, v))
				wantMissionError(t, err, want)
				if c := m.counts(t, v.OperationID); c != (missionCounts{}) {
					t.Fatalf("refused creation left state: %+v", c)
				}
			})
		}
	})
	mission := created.MissionID
	driver := m.driver(t)
	t.Run("ineligible-assignees", func(t *testing.T) {
		finance := m.driver(t)
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: finance.person.member}, "FINANCE", "ORG", "ACTIVE")
		_, err := m.assignTo(t, mission, 1, finance.person.employee)
		wantMissionError(t, err, tripapp.ErrMissionIneligible)
		_, err = m.assignTo(t, mission, 1, onboardingID(t))
		wantMissionError(t, err, tripapp.ErrNotFound)
		if h := m.head(t, mission); h.revision != 1 || h.state != "DRAFT" {
			t.Fatal("refused assignment changed the mission", h)
		}
	})
	// The driver is not assigned to the vehicle: the FLEET-007 start decision
	// at the planned start is returned as a warning, never a refusal.
	assigned, err := m.assignTo(t, mission, 1, driver.person.employee)
	if err != nil || assigned.Revision != 2 || assigned.State != tripdomain.MissionAssigned || assigned.AssigneeEmployeeID != driver.person.employee ||
		assigned.Warning == nil || assigned.Warning.Code != "ASSET_UNAVAILABLE" || assigned.Warning.Availability != "AVAILABLE" {
		t.Fatal("assignment with availability warning", err, assigned.Warning)
	}
	if h := m.head(t, mission); h.state != "ASSIGNED" || h.member != driver.person.member || h.revision != 2 {
		t.Fatal("assigned head", h)
	}
	if n := m.tripsOn(t); n != 0 {
		t.Fatal("an assignment started a trip", n)
	}
	_, err = m.assignTo(t, mission, 2, driver.person.employee)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	_, err = m.assignTo(t, mission, 1, m.person.employee)
	wantMissionError(t, err, tripdomain.ErrConflict)
	// d is assigned to the vehicle: admitted, no warning.
	reassigned, err := m.assignTo(t, mission, 2, m.person.employee)
	if err != nil || reassigned.Revision != 3 || reassigned.Warning != nil {
		t.Fatal("reassignment", err, reassigned.Warning)
	}
	cancelInput := tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, OperationID: onboardingID(t), MissionID: mission, ExpectedRevision: 3, Reason: "Cliente cancelou a entrega"}
	cancelled, _, err := m.run(t, owner, m.member, missionCommand(t, cancelInput))
	if err != nil || cancelled.Revision != 4 || cancelled.State != tripdomain.MissionCancelled || cancelled.AssigneeEmployeeID != m.person.employee {
		t.Fatal("cancellation", err, cancelled)
	}
	_, err = m.assignTo(t, mission, 4, driver.person.employee)
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	cancelInput.OperationID, cancelInput.ExpectedRevision = onboardingID(t), 4
	_, _, err = m.run(t, owner, m.member, missionCommand(t, cancelInput))
	wantMissionError(t, err, tripdomain.ErrInvalidTransition)
	var transitions int
	var history string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*),string_agg(kind||':'||COALESCE(from_state,'-')||'>'||to_state,',' ORDER BY revision) FROM org_trip_mission_transitions WHERE org_id=$1 AND mission_id=$2`, org, mission).Scan(&transitions, &history); err != nil ||
		transitions != 4 || history != "CREATE:->DRAFT,ASSIGN:DRAFT>ASSIGNED,ASSIGN:ASSIGNED>ASSIGNED,CANCEL:ASSIGNED>CANCELLED" {
		t.Fatal("transition history", transitions, history, err)
	}
	var warned string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT string_agg(kind||':'||COALESCE(warning_code,'-'),',' ORDER BY result_revision) FROM org_trip_mission_commands WHERE org_id=$1 AND mission_id=$2`, org, mission).Scan(&warned); err != nil ||
		warned != "CREATE:-,ASSIGN:ASSET_UNAVAILABLE,ASSIGN:-,CANCEL:-" {
		t.Fatal("receipts and their warnings", warned, err)
	}
	t.Run("reads", func(t *testing.T) {
		v, err := m.missions.ReadMission(t.Context(), m.scope, owner, m.member, mission)
		if err != nil || v.State != tripdomain.MissionCancelled || v.Revision != 4 || v.AssetID != m.vehicle.AssetID || v.UnitID != m.unit || v.AssigneeEmployeeID != m.person.employee {
			t.Fatal("owner read", err, v.State)
		}
		other := m.create(t, m.unit, "")
		if _, err := m.assignTo(t, other.MissionID, 1, driver.person.employee); err != nil {
			t.Fatal(err)
		}
		page, err := m.missions.ListMissions(t.Context(), m.scope, owner, m.member, false, tripapp.MissionFilter{Limit: 1})
		if err != nil || len(page.Missions) != 1 || page.NextID == "" {
			t.Fatal("first organization page", err, len(page.Missions))
		}
		rest, err := m.missions.ListMissions(t.Context(), m.scope, owner, m.member, false, tripapp.MissionFilter{Limit: 10, AfterID: page.NextID})
		if err != nil || len(rest.Missions) != 1 || rest.NextID != "" || rest.Missions[0].MissionID == page.Missions[0].MissionID {
			t.Fatal("second organization page", err, len(rest.Missions))
		}
		mine, err := m.missions.ListMissions(t.Context(), m.scope, personMissionActor(driver.person), driver.person.member, true, tripapp.MissionFilter{Limit: 10})
		if err != nil || len(mine.Missions) != 1 || mine.Missions[0].MissionID != other.MissionID {
			t.Fatal("the driver's own list holds only its current assignment", err, len(mine.Missions))
		}
		_, err = m.missions.ListMissions(t.Context(), m.scope, personMissionActor(driver.person), driver.person.member, false, tripapp.MissionFilter{Limit: 10})
		wantMissionError(t, err, permission.ErrPermissionDenied)
		_, err = m.missions.ReadMission(t.Context(), m.scope, owner, m.member, onboardingID(t))
		wantMissionError(t, err, tripapp.ErrNotFound)
	})
}

func (m missionState) draftAsset(t *testing.T) string {
	t.Helper()
	created, _, err := fleetWrite(t, m.legalFixtureState, m.assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	return created.AssetID
}

func (m missionState) foreignUnit(t *testing.T) string {
	t.Helper()
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	var unit string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 LIMIT 1`, other.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	return unit
}

func (m missionState) tripsOn(t *testing.T) int {
	t.Helper()
	var n int
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trips WHERE org_id=$1 AND asset_id=$2`, m.scope.OrgID().String(), m.vehicle.AssetID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// BR-AUTH-004/005, BR-TEN-006, AUTH-006: another organization's mission is
// not found; a UNIT-scoped manager acts only on missions of its unit and
// never on organization-level actions; AUDITOR holds no mission key.
func TestTripMissionScopeIsolationAndAuditor(t *testing.T) {
	m := missionFixture(t)
	south := m.secondUnit(t)
	north := m.create(t, m.unit, "")
	southern := m.create(t, south, "")
	manager := m.driver(t)
	matrixAssign(t, m.deviceFixtureState, matrixPerson{member: manager.person.member}, "FLEET_MANAGER", "UNIT", "ACTIVE")
	actor := personMissionActor(manager.person)
	if v, err := m.missions.ReadMission(t.Context(), m.scope, actor, manager.person.member, north.MissionID); err != nil || v.MissionID != north.MissionID {
		t.Fatal("unit manager reads a mission of its unit", err)
	}
	_, err := m.missions.ReadMission(t.Context(), m.scope, actor, manager.person.member, southern.MissionID)
	wantMissionError(t, err, permission.ErrPermissionDenied)
	_, _, err = m.run(t, actor, manager.person.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionAssignCommand, MissionID: southern.MissionID, ExpectedRevision: 1, AssigneeEmployeeID: m.person.employee}))
	wantMissionError(t, err, permission.ErrPermissionDenied)
	ack, _, err := m.run(t, actor, manager.person.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionAssignCommand, MissionID: north.MissionID, ExpectedRevision: 1, AssigneeEmployeeID: m.person.employee}))
	if err != nil || ack.Revision != 2 {
		t.Fatal("unit manager assigns in its unit", err)
	}
	start, end := m.window(t)
	_, _, err = m.run(t, actor, manager.person.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCreate, UnitID: m.unit, Title: "Fora do escopo", StartsAt: start, EndsAt: end}))
	wantMissionError(t, err, permission.ErrPermissionDenied)
	_, err = m.missions.ListMissions(t.Context(), m.scope, actor, manager.person.member, false, tripapp.MissionFilter{Limit: 10})
	wantMissionError(t, err, permission.ErrPermissionDenied)
	if h := m.head(t, southern.MissionID); h.revision != 1 {
		t.Fatal("an out-of-scope command changed the mission", h)
	}

	auditor := m.driver(t)
	matrixAssign(t, m.deviceFixtureState, matrixPerson{member: auditor.person.member}, "AUDITOR", "ORG", "ACTIVE")
	_, err = m.missions.ReadMission(t.Context(), m.scope, personMissionActor(auditor.person), auditor.person.member, north.MissionID)
	wantMissionError(t, err, permission.ErrPermissionDenied)
	_, _, err = m.run(t, personMissionActor(auditor.person), auditor.person.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: north.MissionID, ExpectedRevision: 2, Reason: "Auditor"}))
	wantMissionError(t, err, permission.ErrPermissionDenied)

	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	foreign := missionState{tripState: m.tripState}
	foreign.legalFixtureState = other
	foreign.missions = m.missions
	_, err = foreign.missions.ReadMission(t.Context(), other.scope, ownerMissionActor(other), other.member, north.MissionID)
	wantMissionError(t, err, tripapp.ErrNotFound)
	_, _, err = foreign.missions.CommandMission(t.Context(), other.scope, ownerMissionActor(other), other.member, "", missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: north.MissionID, ExpectedRevision: 2, Reason: "Outra organização"}))
	wantMissionError(t, err, tripapp.ErrNotFound)
	if page, err := foreign.missions.ListMissions(t.Context(), other.scope, ownerMissionActor(other), other.member, false, tripapp.MissionFilter{Limit: 10}); err != nil || len(page.Missions) != 0 {
		t.Fatal("another organization lists none", err, len(page.Missions))
	}
}

// waitLockWaiters waits until n sessions wait on a lock.
func waitLockWaiters(t *testing.T, x missionState, n int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("concurrent commands never waited on the held lock")
}

// holdRoot locks the organization root row until release is called.
func holdRoot(t *testing.T, x missionState) func() {
	t.Helper()
	tx, err := x.f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, x.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	return func() { _ = tx.Rollback(context.Background()) }
}

// 08-state-model §4.7: two assignments decided on the same revision race
// behind the held root; the first queued wins and the other is CONFLICT with
// nothing stored, in both orders.
func TestTripMissionConcurrentAssignmentsResolveToOne(t *testing.T) {
	m := missionFixture(t)
	first, second := m.driver(t), m.driver(t)
	for _, order := range []string{"first-queued-wins", "second-queued-wins"} {
		t.Run(order, func(t *testing.T) {
			mission := m.create(t, m.unit, "").MissionID
			employees := []string{first.person.employee, second.person.employee}
			if order == "second-queued-wins" {
				employees[0], employees[1] = employees[1], employees[0]
			}
			release := holdRoot(t, m)
			t.Cleanup(release) // a failed wait must never leave the root locked
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i, employee := range employees {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, errs[i] = m.assignTo(t, mission, 1, employee)
				}()
				waitLockWaiters(t, m, i+1)
			}
			release()
			wg.Wait()
			if errs[0] != nil || !errors.Is(errs[1], tripdomain.ErrConflict) {
				t.Fatal("race outcome", errs)
			}
			h := m.head(t, mission)
			if h.revision != 2 || h.assignee != employees[0] {
				t.Fatal("the first queued assignment did not win alone", h)
			}
			var n int
			if err := m.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1 AND mission_id=$2`, m.scope.OrgID().String(), mission).Scan(&n); err != nil || n != 2 {
				t.Fatal("the losing assignment stored a transition", n, err)
			}
		})
	}
}

// ---- trip.mission.respond through the signed intake (EDR-229) ----

func (m missionState) respondOp(t *testing.T, d syncFixtureState, mission string, expected int64, response string, reason any, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = tripapp.MissionRespondOperationType
		raw["entity"] = map[string]any{"type": tripapp.MissionEntityType, "id": mission, "expected_revision": expected}
		raw["occurred_at"] = orgtime.FormatInstant(m.declaredNow(t))
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = map[string]any{"response": response, "reason": reason}
		if edit != nil {
			edit(raw, raw["payload"].(map[string]any))
		}
	})
}

func (m missionState) deliver(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return m.intake.Process(t.Context(), d.source, op, protocol.DefaultLimits(), m.respond)
}

type responseCounts struct{ transitions, effects, claims, receipts, audits int }

func (m missionState) responseCounts(t *testing.T, op protocol.Operation) responseCounts {
	t.Helper()
	var c responseCounts
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`,
		m.scope.OrgID().String(), op.ID()).Scan(&c.transitions, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (m missionState) denied(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected a %s transport denial, got %v", want, err)
	}
	if c := m.responseCounts(t, op); c != (responseCounts{}) {
		t.Fatalf("denied response stored state: %+v", c)
	}
}

func (m missionState) replays(t *testing.T, d syncFixtureState, op protocol.Operation, ack protocol.Acknowledgement) {
	t.Helper()
	for range 2 {
		again, err := m.deliver(t, d, op)
		if err != nil || !bytes.Equal(again.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored acknowledgement", err)
		}
	}
}

// reviewAndTripState is everything a response must never touch.
func (m missionState) untouched(t *testing.T) string {
	t.Helper()
	var v string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT count(*) FROM org_trips WHERE org_id=$1),(SELECT count(*) FROM org_trip_review_entries WHERE org_id=$1),
(SELECT count(*) FROM org_asset_reservations WHERE org_id=$1),(SELECT count(*) FROM org_asset_blocks WHERE org_id=$1),(SELECT count(*) FROM org_sync_review_items WHERE org_id=$1),
(SELECT jsonb_agg(to_jsonb(a)-'updated_at' ORDER BY id) FROM org_assets a WHERE org_id=$1))::text`, m.scope.OrgID().String()).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// B-BR-EMP-356, AC: only the assigned eligible member responds; an accepted
// response changes only the mission (no trip, reservation, block, review or
// asset change); exact replay, digest reuse, a second response and a stale
// one, another member, a malformed payload and another organization's mission.
func TestTripMissionResponseAtReceipt(t *testing.T) {
	m := missionFixture(t)
	d := m.syncFixtureState
	driver := m.driver(t)
	mission := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	if _, err := m.assignTo(t, mission, 1, driver.person.employee); err != nil {
		t.Fatal(err)
	}
	before := m.untouched(t)

	t.Run("another-member-forbidden", func(t *testing.T) {
		op := m.respondOp(t, d, mission, 2, "ACCEPT", nil, nil)
		ack, err := m.deliver(t, d, op)
		m.denied(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("malformed-is-final", func(t *testing.T) {
		for name, op := range map[string]protocol.Operation{
			"decline-without-reason": m.respondOp(t, d, mission, 2, "DECLINE", nil, nil),
			"accept-with-reason":     m.respondOp(t, d, mission, 2, "ACCEPT", "Motivo", nil),
			"unknown-response":       m.respondOp(t, d, mission, 2, "MAYBE", nil, nil),
			"no-expected-revision": m.respondOp(t, d, mission, 2, "ACCEPT", nil, func(e, _ map[string]any) {
				e["entity"] = map[string]any{"type": tripapp.MissionEntityType, "id": mission, "expected_revision": nil}
			}),
			"extra-payload-key": m.respondOp(t, d, mission, 2, "ACCEPT", nil, func(_, p map[string]any) { p["asset_id"] = m.vehicle.AssetID }),
		} {
			t.Run(name, func(t *testing.T) {
				ack, err := m.deliver(t, d, op)
				if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
					t.Fatal("expected a final MALFORMED", err, ack.Result(), ack.Code())
				}
				if c := m.responseCounts(t, op); c != (responseCounts{claims: 1, receipts: 1, audits: 1}) {
					t.Fatalf("malformed response stored a fact: %+v", c)
				}
			})
		}
	})
	t.Run("other-organization-mission-not-found", func(t *testing.T) {
		other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
		if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
			t.Fatal(err)
		}
		grantMissionTables(t, other)
		foreignStore := missionStoreFor(t, other, m.tripState)
		var unit string
		if err := m.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 LIMIT 1`, other.scope.OrgID().String()).Scan(&unit); err != nil {
			t.Fatal(err)
		}
		start, end := m.window(t)
		foreign, _, err := foreignStore.CommandMission(t.Context(), other.scope, ownerMissionActor(other), other.member, onboardingID(t), missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCreate, UnitID: unit, Title: "Missão alheia", StartsAt: start, EndsAt: end}))
		if err != nil {
			t.Fatal(err)
		}
		op := m.respondOp(t, driver, foreign.MissionID, 1, "ACCEPT", nil, nil)
		ack, err := m.deliver(t, driver, op)
		m.denied(t, op, ack, err, orgapi.NotFound)
	})

	accept := m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil)
	ack, err := m.deliver(t, driver, accept)
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 3 {
		t.Fatal("the assignee's acceptance", err, ack.Result(), ack.Code())
	}
	if c := m.responseCounts(t, accept); c != (responseCounts{transitions: 1, effects: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("acceptance not stored exactly once: %+v", c)
	}
	if h := m.head(t, mission); h.state != "ACCEPTED" || h.revision != 3 || h.answer != "ACCEPT" || h.member != driver.person.member {
		t.Fatal("accepted head", h)
	}
	if after := m.untouched(t); after != before {
		t.Fatal("a response changed trip, review, reservation, block or asset state")
	}
	m.replays(t, driver, accept, ack)
	reuse := m.respondOp(t, driver, mission, 2, "DECLINE", "Mudei de ideia", func(e, _ map[string]any) { e["operation_id"] = accept.ID() })
	if again, err := m.deliver(t, driver, reuse); err != nil || again.Result() != protocol.Rejected || again.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("same identifier with another digest", err, again.Result(), again.Code())
	}
	t.Run("second-response-conflict", func(t *testing.T) {
		for name, op := range map[string]protocol.Operation{
			"decline-on-answered-revision": m.respondOp(t, driver, mission, 2, "DECLINE", "Mudei de ideia", nil),
			"accept-again-on-answered":     m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil),
			"decline-on-current-revision":  m.respondOp(t, driver, mission, 3, "DECLINE", "Mudei de ideia", nil),
		} {
			t.Run(name, func(t *testing.T) {
				ack, err := m.deliver(t, driver, op)
				m.denied(t, op, ack, err, orgapi.Conflict)
			})
		}
		if h := m.head(t, mission); h.state != "ACCEPTED" || h.revision != 3 {
			t.Fatal("a conflicting response changed the mission", h)
		}
	})
}

// Revalidation at receipt (EDR-229): a response declared while the member
// was the assignee is decided on current facts only — a reassignment, an
// ended role, an ended membership and a cancellation all refuse it.
func TestTripMissionResponseRevalidatedAtReceipt(t *testing.T) {
	m := missionFixture(t)
	assigned := func(t *testing.T, driver syncFixtureState) string {
		t.Helper()
		mission := m.create(t, m.unit, "").MissionID
		if _, err := m.assignTo(t, mission, 1, driver.person.employee); err != nil {
			t.Fatal(err)
		}
		return mission
	}
	t.Run("reassigned-before-receipt", func(t *testing.T) {
		driver := m.driver(t)
		mission := assigned(t, driver)
		op := m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil) // declared while assigned
		if _, err := m.assignTo(t, mission, 2, m.person.employee); err != nil {
			t.Fatal(err)
		}
		ack, err := m.deliver(t, driver, op)
		m.denied(t, op, ack, err, orgapi.Forbidden)
		taken := m.respondOp(t, m.syncFixtureState, mission, 3, "ACCEPT", nil, nil)
		if ack, err := m.deliver(t, m.syncFixtureState, taken); err != nil || ack.Result() != protocol.Accepted {
			t.Fatal("the new assignee responds", err)
		}
	})
	t.Run("role-ended-before-receipt", func(t *testing.T) {
		driver := m.driver(t)
		mission := assigned(t, driver)
		op := m.respondOp(t, driver, mission, 2, "DECLINE", "Sem carteira válida", nil)
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: driver.person.member}, "FINANCE", "ORG", "ACTIVE")
		ack, err := m.deliver(t, driver, op)
		m.denied(t, op, ack, err, orgapi.Forbidden)
		if h := m.head(t, mission); h.state != "ASSIGNED" || h.revision != 2 {
			t.Fatal("the refused response changed the mission", h)
		}
	})
	t.Run("membership-ended-before-receipt", func(t *testing.T) {
		driver := m.driver(t)
		mission := assigned(t, driver)
		op := m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil)
		if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, m.scope.OrgID().String(), driver.person.member); err != nil {
			t.Fatal(err)
		}
		_, err := m.deliver(t, driver, op)
		if !errors.Is(err, syncapp.ErrSourceDenied) {
			t.Fatal("an ended membership's response was not denied at the source", err)
		}
		if c := m.responseCounts(t, op); c != (responseCounts{}) {
			t.Fatalf("ended membership stored state: %+v", c)
		}
		if h := m.head(t, mission); h.state != "ASSIGNED" || h.revision != 2 {
			t.Fatal("the refused response changed the mission", h)
		}
	})
	t.Run("cancelled-before-receipt", func(t *testing.T) {
		driver := m.driver(t)
		mission := assigned(t, driver)
		op := m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil)
		if _, _, err := m.run(t, ownerMissionActor(m.legalFixtureState), m.member, missionCommand(t, tripapp.MissionCommandInput{Kind: tripapp.MissionCancelCommand, MissionID: mission, ExpectedRevision: 2, Reason: "Cancelada"})); err != nil {
			t.Fatal(err)
		}
		ack, err := m.deliver(t, driver, op)
		m.denied(t, op, ack, err, orgapi.Conflict)
	})
}

// 08-state-model §4.7: a response and a reassignment racing on one mission
// serialize on its row behind a held lock; both orders end in one
// consistent state and the loser stores nothing.
func TestTripMissionResponseRacesReassignment(t *testing.T) {
	m := missionFixture(t)
	for _, order := range []string{"response-first", "reassignment-first"} {
		t.Run(order, func(t *testing.T) {
			driver := m.driver(t)
			mission := m.create(t, m.unit, "").MissionID
			if _, err := m.assignTo(t, mission, 1, driver.person.employee); err != nil {
				t.Fatal(err)
			}
			op := m.respondOp(t, driver, mission, 2, "ACCEPT", nil, nil)
			tx, err := m.f.Admin.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
			if _, err := tx.Exec(t.Context(), `SELECT 1 FROM org_trip_missions WHERE org_id=$1 AND id=$2 FOR UPDATE`, m.scope.OrgID().String(), mission); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			var responseErr, reassignErr error
			var ack protocol.Acknowledgement
			respond := func() {
				defer wg.Done()
				ack, responseErr = m.deliver(t, driver, op)
			}
			reassign := func() {
				defer wg.Done()
				_, reassignErr = m.assignTo(t, mission, 2, m.person.employee)
			}
			calls := []func(){respond, reassign}
			if order == "reassignment-first" {
				calls[0], calls[1] = reassign, respond
			}
			for i, call := range calls {
				wg.Add(1)
				go call()
				waitLockWaiters(t, m, i+1)
			}
			_ = tx.Rollback(context.Background())
			wg.Wait()
			h := m.head(t, mission)
			if order == "response-first" {
				if responseErr != nil || ack.Result() != protocol.Accepted || !errors.Is(reassignErr, tripdomain.ErrConflict) || h.state != "ACCEPTED" || h.member != driver.person.member || h.revision != 3 {
					t.Fatal("response-first outcome", responseErr, reassignErr, h)
				}
				return
			}
			if reassignErr != nil || h.state != "ASSIGNED" || h.member != m.person.member || h.revision != 3 {
				t.Fatal("reassignment-first outcome", reassignErr, h)
			}
			m.denied(t, op, ack, responseErr, orgapi.Forbidden)
		})
	}
}
