//go:build integration

package adapters_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// grantAssigneeReads gives the ordinary runtime role the one table right the
// eligible-assignee read adds to the mission store (EDR-304).
func grantAssigneeReads(t *testing.T, m missionState) {
	t.Helper()
	if _, err := m.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_employee_profiles TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}

// candidate adds a synthetic employee with an ACTIVE account-linked
// membership holding exactly one role, and an optional profile.
func (m missionState) candidate(t *testing.T, role, scope, name, registration string) (string, string) {
	t.Helper()
	key, err := m.accounts.CreateKeyAccount(t.Context(), "synassignee"+onboardingID(t)[:8])
	if err != nil {
		t.Fatal(err)
	}
	proof, err := m.accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	employee, member := teamDriver(t, m.legalFixtureState, proof.Account.ID)
	if role != "DRIVER" || scope != "SELF" {
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: member}, role, scope, "ACTIVE")
	}
	if name != "" || registration != "" {
		// Fixture right only: the store itself needs SELECT (grantAssigneeReads).
		if _, err := m.f.Admin.Exec(t.Context(), "GRANT INSERT ON org_employee_profiles TO "+pgx.Identifier{m.f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,registration_number) VALUES($1,$2,$3,$4)`, m.scope.OrgID().String(), employee, name, registration); err != nil {
			t.Fatal(err)
		}
	}
	return employee, member
}

// eligible pages through the whole eligible-assignee read with the given
// limit and returns the items, the number of pages and the number of empty
// pages that still carried a cursor.
func (m missionState) eligible(t *testing.T, actor tripapp.MissionActor, member, mission string, limit int) ([]tripapp.MissionAssignee, int, int, error) {
	t.Helper()
	var all []tripapp.MissionAssignee
	after, pages, empty := "", 0, 0
	for {
		page, err := m.missions.ListMissionAssignees(t.Context(), m.scope, actor, member, mission, tripapp.MissionAssigneeFilter{Limit: limit, AfterID: after})
		if err != nil {
			return nil, pages, empty, err
		}
		pages++
		if len(page.Assignees) > limit || pages > 500 {
			t.Fatal("unbounded eligible-assignee page", len(page.Assignees), pages)
		}
		all = append(all, page.Assignees...)
		if page.NextID == "" {
			return all, pages, empty, nil
		}
		if len(page.Assignees) == 0 {
			empty++
		}
		if page.NextID <= after {
			t.Fatal("eligible-assignee cursor did not advance")
		}
		after = page.NextID
	}
}

// missionTotals snapshots every mission head, transition, receipt and audit
// event of the organization.
func (m missionState) missionTotals(t *testing.T) string {
	t.Helper()
	var v string
	if err := m.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_trip_missions x WHERE org_id=$1),
(SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1),(SELECT count(*) FROM org_trip_mission_commands WHERE org_id=$1),(SELECT count(*) FROM org_audit_events WHERE org_id=$1))::text`, m.scope.OrgID().String()).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func assigneeIDs(items []tripapp.MissionAssignee) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, v.EmployeeID)
	}
	return out
}

// acceptedBy proves, for every employee of the organization, that the
// assignment command accepts exactly the listed ones on a fresh mission with
// the same unit and vehicle (B-BR-EMP-470: the list is the command's rule).
func (m missionState) acceptedBy(t *testing.T, unit, asset string, listed []string) {
	t.Helper()
	rows, err := m.f.Admin.Query(t.Context(), `SELECT id::text FROM org_employee_identities WHERE org_id=$1 ORDER BY id`, m.scope.OrgID().String())
	if err != nil {
		t.Fatal(err)
	}
	employees, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	for _, employee := range employees {
		mission := m.create(t, unit, asset).MissionID
		_, err := m.assignTo(t, mission, 1, employee)
		if slices.Contains(listed, employee) != (err == nil) {
			t.Fatalf("the list and the assignment command disagree for an employee: listed=%t assignment=%v", slices.Contains(listed, employee), err)
		}
		if err != nil && !errors.Is(err, tripapp.ErrMissionIneligible) {
			t.Fatal("an unlisted employee was refused for another reason", err)
		}
	}
}

// B-BR-EMP-470, BUC-EMP-241: the read lists exactly the employees the
// assignment command accepts for that mission (FINANCE, AUDITOR, suspended,
// inactive, account-less and merged candidates excluded; a UNIT-scoped member
// only for its unit's missions), with only the profile's name and
// registration number, empty pages carrying a cursor, and no state change.
func TestTripMissionAssigneesListExactlyTheEligibleEmployees(t *testing.T) {
	m := missionFixture(t)
	grantAssigneeReads(t, m)
	owner, org := ownerMissionActor(m.legalFixtureState), m.scope.OrgID().String()
	south := m.secondUnit(t)
	driver, _ := m.candidate(t, "DRIVER", "SELF", "Bruno Sintético", "SYN-001")
	finance, _ := m.candidate(t, "FINANCE", "ORG", "Fin Sintética", "SYN-002")
	auditor, _ := m.candidate(t, "AUDITOR", "ORG", "", "")
	supervisor, _ := m.candidate(t, "SUPERVISOR", "UNIT", "Supervisora Norte", "")
	suspended, suspendedMember := m.candidate(t, "DRIVER", "SELF", "Motorista Suspenso", "")
	if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, suspendedMember); err != nil {
		t.Fatal(err)
	}
	inactive, inactiveMember := m.candidate(t, "DRIVER", "SELF", "Motorista Inativo", "")
	if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, inactiveMember); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, m.legalFixtureState, `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, inactive); err != nil {
		t.Fatal(err)
	}
	accountless := onboardingID(t)
	if err := unitSchemaExec(t, m.legalFixtureState, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, accountless); err != nil {
		t.Fatal(err)
	}
	// A merged record is TERMINATED and keeps no live membership (000193).
	merged := onboardingID(t)
	for _, statement := range []string{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1 WHERE org_id=$1 AND id=$2`,
		`UPDATE org_employee_identities SET merged_into=$3::uuid,revision=revision+1 WHERE org_id=$1 AND id=$2`} {
		args := []any{org, merged}
		if strings.Contains(statement, "$3") {
			args = append(args, driver)
		}
		if err := unitSchemaExec(t, m.legalFixtureState, statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	north := m.create(t, m.unit, m.vehicle.AssetID).MissionID
	southern := m.create(t, south, "").MissionID
	before, totals := m.untouched(t), m.missionTotals(t)

	all, pages, empty, err := m.eligible(t, owner, m.member, north, 1)
	if err != nil {
		t.Fatal("eligible assignees of the north mission", err)
	}
	listed := assigneeIDs(all)
	for _, want := range []string{driver, supervisor, m.person.employee} {
		if !slices.Contains(listed, want) {
			t.Fatal("an eligible member is missing from the list")
		}
	}
	for _, refused := range []string{finance, auditor, suspended, inactive, accountless, merged} {
		if slices.Contains(listed, refused) {
			t.Fatal("an ineligible employee is listed")
		}
	}
	if !slices.IsSorted(listed) || len(slices.Compact(slices.Clone(listed))) != len(listed) {
		t.Fatal("the list is not one ascending pass over employees", listed)
	}
	if empty == 0 || pages <= len(all) {
		t.Fatal("ineligible candidates must yield empty pages that still advance the cursor", pages, empty, len(all))
	}
	for _, v := range all {
		switch v.EmployeeID {
		case driver:
			if v.DisplayName != "Bruno Sintético" || v.RegistrationNumber != "SYN-001" {
				t.Fatal("profile name and registration are projected as stored", v.DisplayName, v.RegistrationNumber)
			}
		case m.person.employee:
			if v.DisplayName != "" || v.RegistrationNumber != "" {
				t.Fatal("an employee without profile has empty descriptive fields")
			}
		}
	}
	whole, wholePages, _, err := m.eligible(t, owner, m.member, north, tripapp.MaxMissionPage)
	if err != nil || wholePages != 1 || !slices.Equal(assigneeIDs(whole), listed) {
		t.Fatal("one large page lists the same employees", err, wholePages)
	}
	southernAll, _, _, err := m.eligible(t, owner, m.member, southern, 100)
	if err != nil {
		t.Fatal(err)
	}
	if southernListed := assigneeIDs(southernAll); slices.Contains(southernListed, supervisor) || !slices.Contains(southernListed, driver) {
		t.Fatal("a UNIT-scoped member is eligible only for its unit's missions")
	}
	if m.untouched(t) != before || m.missionTotals(t) != totals {
		t.Fatal("the eligible-assignee read changed state")
	}
	m.acceptedBy(t, m.unit, m.vehicle.AssetID, listed)
	m.acceptedBy(t, south, "", assigneeIDs(southernAll))
	_, err = m.missions.ListMissionAssignees(t.Context(), m.scope, owner, m.member, north, tripapp.MissionAssigneeFilter{Limit: 0})
	wantMissionError(t, err, tripdomain.ErrInvalid)
}

// BR-AUTH-004/013, BR-TEN-006: a UNIT-scoped planner reads the eligible
// assignees only of its unit's missions; AUDITOR and DRIVER hold no mission
// key; another organization's or an unknown mission is not found.
func TestTripMissionAssigneesScopeIsolationAndAuditor(t *testing.T) {
	m := missionFixture(t)
	grantAssigneeReads(t, m)
	south := m.secondUnit(t)
	north := m.create(t, m.unit, "").MissionID
	southern := m.create(t, south, "").MissionID
	manager := m.driver(t)
	matrixAssign(t, m.deviceFixtureState, matrixPerson{member: manager.person.member}, "FLEET_MANAGER", "UNIT", "ACTIVE")
	actor := personMissionActor(manager.person)
	if page, err := m.missions.ListMissionAssignees(t.Context(), m.scope, actor, manager.person.member, north, tripapp.MissionAssigneeFilter{Limit: 100}); err != nil || len(page.Assignees) == 0 {
		t.Fatal("the unit planner lists the eligible assignees of its unit's mission", err)
	}
	_, err := m.missions.ListMissionAssignees(t.Context(), m.scope, actor, manager.person.member, southern, tripapp.MissionAssigneeFilter{Limit: 100})
	wantMissionError(t, err, permission.ErrPermissionDenied)
	for _, role := range []struct{ role, scope string }{{"AUDITOR", "ORG"}, {"DRIVER", "SELF"}} {
		person := m.driver(t)
		matrixAssign(t, m.deviceFixtureState, matrixPerson{member: person.person.member}, role.role, role.scope, "ACTIVE")
		_, err := m.missions.ListMissionAssignees(t.Context(), m.scope, personMissionActor(person.person), person.person.member, north, tripapp.MissionAssigneeFilter{Limit: 100})
		wantMissionError(t, err, permission.ErrPermissionDenied)
	}
	owner := ownerMissionActor(m.legalFixtureState)
	_, err = m.missions.ListMissionAssignees(t.Context(), m.scope, owner, m.member, onboardingID(t), tripapp.MissionAssigneeFilter{Limit: 100})
	wantMissionError(t, err, tripapp.ErrNotFound)
	other := legalFixtureFor(t, m.f, m.onboarding, m.actor, m.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	_, err = m.missions.ListMissionAssignees(t.Context(), other.scope, ownerMissionActor(other), other.member, north, tripapp.MissionAssigneeFilter{Limit: 100})
	wantMissionError(t, err, tripapp.ErrNotFound)
}

// B-BR-EMP-470 through the actual composed company entry: no-store pages with
// a sealed cursor bound to the mission, the closed item shape, malformed and
// foreign cursors, unknown fields, FORBIDDEN naming the key for a driver, an
// auditor and an out-of-scope unit planner, and NOT_FOUND for an unknown
// mission and another organization's path.
func TestTripMissionAssigneesThroughComposedCompanyHTTP(t *testing.T) {
	m := missionFixture(t)
	fx := m.deviceFixtureState
	grantEmployeeTables(t, fx.legalFixtureState)
	_ = attachmentStore(t, fx.legalFixtureState)
	grantChecklistTables(t, fx.legalFixtureState)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{61}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual mission composition failed", err)
	}
	org := fx.scope.OrgID().String()
	base := "/v1/orgs/" + org + "/missions"
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	driver, auditor, manager := matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, driver, "DRIVER", "SELF", "ACTIVE")
	matrixAssign(t, fx, auditor, "AUDITOR", "ORG", "ACTIVE")
	matrixAssign(t, fx, manager, "FLEET_MANAGER", "UNIT", "ACTIVE")
	named, _ := m.candidate(t, "DRIVER", "SELF", "Bruno Sintético", "SYN-001")
	south := m.secondUnit(t)
	call := func(actor matrixPerson, path string, body any, want int, code, key string) map[string]any {
		t.Helper()
		return matrixCall(t, fx, router, actor, path, body, want, code, key, nil)
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	create := func(unit string) string {
		body := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "unit_id": unit, "title": "Entrega sintética",
			"starts_at": start.Format(time.RFC3339Nano), "ends_at": start.Add(2 * time.Hour).Format(time.RFC3339Nano)}
		id, _ := call(owner, base+"/create", body, 201, "", "")["mission_id"].(string)
		return id
	}
	mission, southern := create(m.unit), create(south)
	path := base + "/" + mission + "/assignees/list"
	var seen []string
	cursor, pages := "", 0
	for {
		body := map[string]any{"limit": 2}
		if cursor != "" {
			body["cursor"] = cursor
		}
		page := call(owner, path, body, 200, "", "")
		pages++
		items, ok := page["items"].([]any)
		if !ok || len(items) > 2 || page["data_mode"] != "SYNTHETIC" || len(page) != 3 {
			t.Fatal("eligible-assignee page shape", page)
		}
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if len(item) != 3 || item["employee_id"] == nil || item["display_name"] == nil || item["registration_number"] == nil {
				t.Fatal("eligible-assignee item carries exactly the three descriptive fields", item)
			}
			seen = append(seen, item["employee_id"].(string))
			if item["employee_id"] == named && (item["display_name"] != "Bruno Sintético" || item["registration_number"] != "SYN-001") {
				t.Fatal("named eligible employee", item)
			}
		}
		next, _ := page["next_cursor"].(string)
		if next == "" {
			break
		}
		if pages > 100 {
			t.Fatal("unbounded eligible-assignee pagination")
		}
		cursor = next
	}
	if !slices.Contains(seen, named) || !slices.Contains(seen, driver.employee) || slices.Contains(seen, auditor.employee) {
		t.Fatal("HTTP eligible assignees", len(seen))
	}
	first := call(owner, path, map[string]any{"limit": 1}, 200, "", "")
	if next, _ := first["next_cursor"].(string); next != "" {
		call(owner, base+"/"+southern+"/assignees/list", map[string]any{"limit": 1, "cursor": next}, 400, "MALFORMED", "")
		call(owner, path, map[string]any{"limit": 2, "cursor": next}, 400, "MALFORMED", "")
	} else {
		t.Fatal("a one-item page of several candidates must carry a cursor")
	}
	call(owner, path, map[string]any{"cursor": "forged"}, 400, "MALFORMED", "")
	call(owner, path, map[string]any{"limit": 101}, 400, "MALFORMED", "")
	call(owner, path, map[string]any{"limit": 2, "employee_id": named}, 400, "MALFORMED", "")
	call(driver, path, map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(auditor, path, map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	call(manager, base+"/"+southern+"/assignees/list", map[string]any{}, 403, "FORBIDDEN", "trip.mission.manage")
	if v := call(manager, path, map[string]any{}, 200, "", ""); len(v["items"].([]any)) == 0 {
		t.Fatal("the unit planner lists its unit's eligible assignees")
	}
	call(owner, base+"/"+onboardingID(t)+"/assignees/list", map[string]any{}, 404, "NOT_FOUND", "")
	call(owner, base+"/not-a-mission/assignees/list", map[string]any{}, 404, "NOT_FOUND", "")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	call(owner, "/v1/orgs/"+other.scope.OrgID().String()+"/missions/"+mission+"/assignees/list", map[string]any{}, 404, "NOT_FOUND", "")
}
