//go:build integration

package adapters_test

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	employee "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	idn "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	orgdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
)

func employeeFixture(t *testing.T) (legalFixtureState, *memberpg.EmployeeStore) {
	t.Helper()
	fx := legalFixture(t)
	grantEmployeeTables(t, fx)
	catalog, e := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return fx, &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &adapters.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func grantEmployeeTables(t *testing.T, fx legalFixtureState) {
	t.Helper()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_employee_profiles TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()+"; GRANT SELECT,INSERT ON org_employee_commands,org_employee_unit_clearings,org_unit_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
}
func employeeActor(fx legalFixtureState) employee.EmployeeActor {
	return employee.EmployeeActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}
func employeeCommand(t *testing.T, v employee.EmployeeWriteInput) employee.EmployeeWrite {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, e := employee.NewEmployeeWrite(v)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func writeEmployee(t *testing.T, fx legalFixtureState, s *memberpg.EmployeeStore, c employee.EmployeeWrite) (employee.EmployeeResult, bool, error) {
	t.Helper()
	v, e := c.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	id := ""
	if v.Kind == employee.CreateEmployee {
		id = onboardingID(t)
	}
	return s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, id, c)
}
func TestEmployeeRegisterNoLoginDuplicatesPendingProfilesAndIsolation(t *testing.T) {
	fx, s := employeeFixture(t)
	var before, after string
	counts := `SELECT jsonb_build_array((SELECT count(*) FROM accounts),(SELECT count(*) FROM org_memberships),(SELECT count(*) FROM org_role_assignments))::text`
	if e := fx.f.Admin.QueryRow(t.Context(), counts).Scan(&before); e != nil {
		t.Fatal(e)
	}
	c := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Employee", RegistrationNumber: " R-01 ", WorkEmail: "Work@example.invalid"}})
	first, replay, e := writeEmployee(t, fx, s, c)
	if e != nil || replay || first.ProfileRevision != 1 || first.IdentityRevision != 1 || first.State != idn.EmployeeActive {
		t.Fatal("create", e)
	}
	if v, r, e := writeEmployee(t, fx, s, c); e != nil || !r || v != first {
		t.Fatal("retry", e)
	}
	duplicate := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Other", RegistrationNumber: "r-01"}, AllowPossibleDuplicate: true})
	if _, _, e := writeEmployee(t, fx, s, duplicate); e != employee.ErrRegistrationDuplicate {
		t.Fatal("registration override", e)
	}
	possible := employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "synthetic  employee", WorkEmail: "work@example.invalid"}}
	if _, _, e := writeEmployee(t, fx, s, employeeCommand(t, possible)); e != employee.ErrDuplicateReview {
		t.Fatal("silent duplicate", e)
	}
	page, e := s.MatchEmployees(t.Context(), fx.scope, employeeActor(fx), fx.member, employee.EmployeeMatch{Profile: possible.Profile, Limit: 1})
	if e != nil || len(page.Employees) != 1 {
		t.Fatal("review candidates", e)
	}
	possible.AllowPossibleDuplicate = true
	second, _, e := writeEmployee(t, fx, s, employeeCommand(t, possible))
	if e != nil || second.EmployeeID == first.EmployeeID {
		t.Fatal("explicit duplicate merged", e)
	}
	change := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.ChangeEmployee, EmployeeID: first.EmployeeID, ExpectedRevision: 1, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Correction"}})
	if r, _, e := writeEmployee(t, fx, s, change); e != nil || r.ProfileRevision != 2 || r.IdentityRevision != 1 {
		t.Fatal("profile affected access", e)
	}
	if _, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.ChangeEmployee, EmployeeID: first.EmployeeID, ExpectedRevision: 1, Profile: idn.EmployeeProfile{DisplayName: "Stale"}})); e != idn.ErrConflict {
		t.Fatal("stale", e)
	}
	if v, r, e := writeEmployee(t, fx, s, c); e != nil || !r || v != first {
		t.Fatal("retry returned current profile", e)
	}
	var creator string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member).Scan(&creator); e != nil {
		t.Fatal(e)
	}
	pending, e := s.ReadEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, creator)
	if e != nil {
		t.Fatal(e)
	}
	p, _ := pending.Snapshot(fx.scope)
	if p.ProfileRevision != 0 || p.Profile.DisplayName != "" {
		t.Fatal("invented creator name")
	}
	if r, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CompleteEmployee, EmployeeID: creator, ExpectedRevision: 1, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Creator"}})); e != nil || r.IdentityRevision != 1 || r.ProfileRevision != 1 {
		t.Fatal("profile completion", e)
	}
	foreign := fx.f.Organizations[1].Scope(t)
	for _, id := range []string{first.EmployeeID, onboardingID(t)} {
		if _, e := s.FindEmployee(t.Context(), foreign, id); e != employee.ErrEmployeeNotFound {
			t.Fatal("foreign existence hint", e)
		}
	}
	if _, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Foreign unit", HomeUnitID: onboardingID(t)}})); e != employee.ErrEmployeeNotFound {
		t.Fatal("foreign unit", e)
	}
	if e := fx.f.Admin.QueryRow(t.Context(), counts).Scan(&after); e != nil || before != after {
		t.Fatal("register provisioned access", e)
	}
	pageAll, e := s.ListEmployees(t.Context(), fx.scope, employeeActor(fx), fx.member, employee.EmployeeFilter{Limit: 2})
	if e != nil || len(pageAll.Employees) != 2 || pageAll.NextID == "" {
		t.Fatal("register paging", e)
	}
	tail, e := s.ListEmployees(t.Context(), fx.scope, employeeActor(fx), fx.member, employee.EmployeeFilter{Limit: 2, AfterID: pageAll.NextID})
	if e != nil || len(tail.Employees) != 1 || tail.NextID != "" {
		t.Fatal("register tail", e)
	}
}
func TestEmployeeRegisterSerializesDuplicatesRetriesAndCAS(t *testing.T) {
	fx, s := employeeFixture(t)
	for _, mode := range []string{"registration", "possible", "retry"} {
		t.Run(mode, func(t *testing.T) {
			shared := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic " + mode, RegistrationNumber: mode}})
			commands := make([]employee.EmployeeWrite, 12)
			ids := make([]string, 12)
			for i := range commands {
				ids[i] = onboardingID(t)
				v, _ := shared.Snapshot()
				if mode != "retry" {
					v.OperationID = onboardingID(t)
				}
				if mode == "possible" {
					v.Profile.RegistrationNumber = ""
					v.Profile.WorkPhone = "+5511999999999"
				}
				commands[i] = employeeCommand(t, v)
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make([]employee.EmployeeResult, 12)
			errs := make([]error, 12)
			replayed := make([]bool, 12)
			for i := range 12 {
				wg.Go(func() {
					<-start
					results[i], replayed[i], errs[i] = s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, ids[i], commands[i])
				})
			}
			close(start)
			wg.Wait()
			successes, retries := 0, 0
			var first employee.EmployeeResult
			for i, e := range errs {
				if e == nil {
					successes++
					if replayed[i] {
						retries++
					}
					if first.EmployeeID == "" {
						first = results[i]
					} else if mode == "retry" && first != results[i] {
						t.Fatal("different retry acknowledgement")
					}
				} else if mode == "registration" && e != employee.ErrRegistrationDuplicate || mode == "possible" && e != employee.ErrDuplicateReview || mode == "retry" {
					t.Fatal("race failure", e)
				}
			}
			if mode == "retry" {
				if successes != 12 || retries != 11 {
					t.Fatal("retry race", successes, retries)
				}
			} else if successes != 1 {
				t.Fatal("duplicate race", successes)
			}
		})
	}
	var rows int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_employee_profiles WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&rows); e != nil || rows != 3 {
		t.Fatal("partial duplicate rows", rows, e)
	}
}
func TestEmployeeRegisterRollsBackDeferredFailureAndRejectsChangedReplay(t *testing.T) {
	fx, s := employeeFixture(t)
	c := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Failed"}})
	v, _ := c.Snapshot()
	id := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_employee_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic deferred source';END $$;CREATE CONSTRAINT TRIGGER synthetic_employee_fail AFTER INSERT ON org_employee_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_employee_fail()`); e != nil {
		t.Fatal(e)
	}
	if r, replay, e := s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, id, c); e != employee.ErrRegisterUnavailable || replay || r != (employee.EmployeeResult{}) {
		t.Fatal("provisional deferred result", e)
	}
	var leftovers int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_employee_identities WHERE org_id=$1 AND id=$2)+(SELECT count(*) FROM org_employee_profiles WHERE org_id=$1 AND employee_id=$2)+(SELECT count(*) FROM org_employee_commands WHERE org_id=$1 AND operation_id=$3)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3)`, fx.scope.OrgID().String(), id, v.OperationID).Scan(&leftovers); e != nil || leftovers != 0 {
		t.Fatal("failed commit left state", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_employee_fail ON org_employee_commands`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, id, c); e != nil {
		t.Fatal("recovery", e)
	}
	v.Profile.DisplayName = "Altered"
	if _, _, e := writeEmployee(t, fx, s, employeeCommand(t, v)); e != employee.ErrEmployeeOperationReused {
		t.Fatal("altered retry", e)
	}
}
func TestEmployeeHomeUnitUsageClearAndRestoreNeverRelinks(t *testing.T) {
	fx, s := employeeFixture(t)
	units := operatingUnitStore(t, fx)
	orgID := fx.scope.OrgID().String()
	unitID := onboardingID(t)
	create := operatingUnitCommand(t, org.UnitWriteInput{Kind: org.CreateUnit, Profile: orgdomain.UnitProfile{Name: "Synthetic Branch", Code: "BR"}})
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, unitID, create); e != nil {
		t.Fatal(e)
	}
	first, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Unit Worker", HomeUnitID: unitID}}))
	if e != nil {
		t.Fatal(e)
	}
	archive := operatingUnitCommand(t, org.UnitWriteInput{Kind: org.ArchiveUnit, UnitID: unitID, ExpectedRevision: 1})
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive); !errors.Is(e, orgdomain.ErrStructureInUse) {
		t.Fatal("active employee without login ignored", e)
	} else {
		var usage org.StructureUsageError
		if !errors.As(e, &usage) || usage.Usage().Employees != 1 {
			t.Fatal("employee counts missing")
		}
	}
	// This has no membership/access. Existing lifecycle SQL still owns the state transition.
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, orgID, first.EmployeeID); e != nil {
		t.Fatal(e)
	}
	archived, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive)
	if e != nil || archived.Revision != 2 {
		t.Fatal("unit archive clearing", e)
	}
	got, e := s.ReadEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, first.EmployeeID)
	if e != nil {
		t.Fatal(e)
	}
	snap, _ := got.Snapshot(fx.scope)
	identity, _ := snap.Employee.Snapshot(fx.scope)
	if snap.Profile.HomeUnitID != "" || snap.ProfileRevision != 2 || identity.Revision != 2 || identity.State != idn.EmployeeInactive {
		t.Fatal("clear affected identity or retained unit")
	}
	var causes int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_employee_unit_clearings c JOIN org_unit_commands u USING(org_id,operation_id,unit_id,kind) JOIN org_audit_events a USING(org_id,operation_id) WHERE c.org_id=$1 AND c.employee_id=$2 AND u.result_revision=c.unit_revision AND a.action='org.unit.archived'`, orgID, first.EmployeeID).Scan(&causes); e != nil || causes != 1 {
		t.Fatal("archive cause missing", e)
	}
	restore := operatingUnitCommand(t, org.UnitWriteInput{Kind: org.RestoreUnit, UnitID: unitID, ExpectedRevision: 2})
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", restore); e != nil {
		t.Fatal(e)
	}
	got, e = s.ReadEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, first.EmployeeID)
	if e != nil {
		t.Fatal(e)
	}
	snap, _ = got.Snapshot(fx.scope)
	if snap.Profile.HomeUnitID != "" || snap.ProfileRevision != 2 {
		t.Fatal("restore silently relinked")
	}
	if r, replay, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive); e != nil || !replay || r != archived {
		t.Fatal("archive retry repeated clearing", e)
	}
	if r, replay, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.ChangeEmployee, EmployeeID: first.EmployeeID, ExpectedRevision: 1, Profile: idn.EmployeeProfile{DisplayName: "Stale Home"}})); e != idn.ErrConflict || replay || r != (employee.EmployeeResult{}) {
		t.Fatal("stale prior profile overwrote clearing", e)
	}
}

func TestEmployeeSchemaPrivateRefusalsHideProfileContents(t *testing.T) {
	fx, _ := employeeFixture(t)
	orgID := fx.scope.OrgID().String()
	first, second := onboardingID(t), onboardingID(t)
	for _, id := range []string{first, second} {
		if e := unitSchemaExec(t, fx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, orgID, id); e != nil {
			t.Fatal(e)
		}
	}
	insert := `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,registration_number,work_email) VALUES($1,$2,$3,$4,$5)`
	if e := unitSchemaExec(t, fx, insert, orgID, first, "Synthetic Private", "SYN-SECRET", "work@example.invalid"); e != nil {
		t.Fatal(e)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, insert, orgID, second, "Synthetic Invalid\nProfile", "", ""), "P7902")
	costCentreSchemaCode(t, unitSchemaExec(t, fx, insert, orgID, second, "Synthetic Duplicate", "syn-secret", ""), "P7901")
	costCentreSchemaCode(t, unitSchemaExec(t, fx, `DELETE FROM org_employee_profiles WHERE org_id=$1 AND employee_id=$2`, orgID, first), "55000")
	var leaked int
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_employee_profiles`).Scan(&leaked); e != nil || leaked != 0 {
		t.Fatal("unscoped private profiles", e)
	}
}

func TestEmployeeUnitClearingFailedCommitIsUnavailableAndAtomic(t *testing.T) {
	fx, s := employeeFixture(t)
	units := operatingUnitStore(t, fx)
	unitID := onboardingID(t)
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, unitID, operatingUnitCommand(t, org.UnitWriteInput{Kind: org.CreateUnit, Profile: orgdomain.UnitProfile{Name: "Synthetic Clearing", Code: "CLR"}})); e != nil {
		t.Fatal(e)
	}
	first, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Clearing Worker", HomeUnitID: unitID}}))
	if e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), first.EmployeeID); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_employee_clearing_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic clearing cause';END $$;CREATE CONSTRAINT TRIGGER synthetic_employee_clearing_failure AFTER INSERT ON org_employee_unit_clearings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_employee_clearing_failure()`); e != nil {
		t.Fatal(e)
	}
	archive := operatingUnitCommand(t, org.UnitWriteInput{Kind: org.ArchiveUnit, UnitID: unitID, ExpectedRevision: 1})
	if r, replay, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive); e != org.ErrUnavailable || replay || r != (org.UnitResult{}) {
		t.Fatal("late archive failure exposed provisional refusal", e)
	}
	record, e := s.ReadEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, first.EmployeeID)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := record.Snapshot(fx.scope)
	if v.ProfileRevision != 1 || v.Profile.HomeUnitID != unitID {
		t.Fatal("failed clearing mutated profile")
	}
	command, _ := archive.Snapshot()
	var leftovers int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_employee_unit_clearings WHERE org_id=$1 AND employee_id=$2)+(SELECT count(*) FROM org_unit_commands WHERE org_id=$1 AND operation_id=$3)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3)`, fx.scope.OrgID().String(), first.EmployeeID, command.OperationID).Scan(&leftovers); e != nil || leftovers != 0 {
		t.Fatal("failed unit archive retained evidence", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_employee_clearing_failure ON org_employee_unit_clearings`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive); e != nil {
		t.Fatal("archive recovery", e)
	}
}

func TestEmployeeHomeUnitAdmissionAndArchiveSerialize(t *testing.T) {
	fx, s := employeeFixture(t)
	units := operatingUnitStore(t, fx)
	for i := range 12 {
		unitID, newID := onboardingID(t), onboardingID(t)
		if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, unitID, operatingUnitCommand(t, org.UnitWriteInput{Kind: org.CreateUnit, Profile: orgdomain.UnitProfile{Name: "Synthetic Race " + strconv.Itoa(i), Code: "R" + strconv.Itoa(i)}})); e != nil {
			t.Fatal(e)
		}
		admission := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Race Employee", HomeUnitID: unitID}})
		archive := operatingUnitCommand(t, org.UnitWriteInput{Kind: org.ArchiveUnit, UnitID: unitID, ExpectedRevision: 1})
		var admitted, archived error
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Go(func() {
			<-start
			_, _, admitted = s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, newID, admission)
		})
		wg.Go(func() {
			<-start
			_, _, archived = units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive)
		})
		close(start)
		wg.Wait()
		if admitted == nil {
			if !errors.Is(archived, orgdomain.ErrStructureInUse) {
				t.Fatal("archive admitted current employee", archived)
			}
		} else if admitted != employee.ErrEmployeeNotFound || archived != nil {
			t.Fatal("archive/admission race", admitted, archived)
		}
		var invalid int
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_employee_profiles p JOIN org_units u ON(u.org_id,u.id)=(p.org_id,p.home_unit_id) JOIN org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id) WHERE p.org_id=$1 AND u.id=$2 AND u.state='ARCHIVED' AND e.state='ACTIVE'`, fx.scope.OrgID().String(), unitID).Scan(&invalid); e != nil || invalid != 0 {
			t.Fatal("archived active home link", e)
		}
	}
	first, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic CAS"}}))
	if e != nil {
		t.Fatal(e)
	}
	errs := make([]error, 12)
	commands := make([]employee.EmployeeWrite, 12)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 12 {
		commands[i] = employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.ChangeEmployee, EmployeeID: first.EmployeeID, ExpectedRevision: 1, Profile: idn.EmployeeProfile{DisplayName: "Synthetic CAS " + strconv.Itoa(i)}})
		wg.Go(func() { <-start; _, _, errs[i] = writeEmployee(t, fx, s, commands[i]) })
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		} else if e != idn.ErrConflict {
			t.Fatal("CAS race", e)
		}
	}
	if wins != 1 {
		t.Fatal("CAS winners", wins)
	}
}

func TestEmployeeRegistrationSpansAllStatesButIsOrganizationScoped(t *testing.T) {
	fx, s := employeeFixture(t)
	first, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Retained Registration", RegistrationNumber: "HISTORY"}}))
	if e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"INACTIVE", "TERMINATED"} {
		if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), first.EmployeeID, state); e != nil {
			t.Fatal(e)
		}
		if _, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Reuse", RegistrationNumber: "history"}})); e != employee.ErrRegistrationDuplicate {
			t.Fatal("inactive/terminated registration reused", state, e)
		}
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, e := writeEmployee(t, other, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Independent", RegistrationNumber: "history"}})); e != nil {
		t.Fatal("independent organization collision", e)
	}
}
func TestEmployeeUnitArchiveClearsRetainedMergedProfile(t *testing.T) {
	fx, s := employeeFixture(t)
	units := operatingUnitStore(t, fx)
	unitID := onboardingID(t)
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, unitID, operatingUnitCommand(t, org.UnitWriteInput{Kind: org.CreateUnit, Profile: orgdomain.UnitProfile{Name: "Synthetic Merged Home", Code: "MERGE"}})); e != nil {
		t.Fatal(e)
	}
	merged, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Retained Merged", HomeUnitID: unitID}}))
	if e != nil {
		t.Fatal(e)
	}
	target, _, e := writeEmployee(t, fx, s, employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Merge Target"}}))
	if e != nil {
		t.Fatal(e)
	}
	// This rehearses a later lifecycle command's resulting retained state only;
	// IDN-002 neither exposes merging nor changes membership state.
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='TERMINATED',merged_into=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), merged.EmployeeID, target.EmployeeID); e != nil {
		t.Fatal(e)
	}
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", operatingUnitCommand(t, org.UnitWriteInput{Kind: org.ArchiveUnit, UnitID: unitID, ExpectedRevision: 1})); e != nil {
		t.Fatal("merged history prevented archive", e)
	}
	r, e := s.ReadEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, merged.EmployeeID)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := r.Snapshot(fx.scope)
	identity, _ := v.Employee.Snapshot(fx.scope)
	if v.Profile.HomeUnitID != "" || v.ProfileRevision != 2 || identity.State != idn.EmployeeTerminated || identity.MergedInto != target.EmployeeID {
		t.Fatal("archive changed merge history")
	}
}

func TestEmployeeAuditFailureNeverProvisionsIdentity(t *testing.T) {
	fx, s := employeeFixture(t)
	id := onboardingID(t)
	c := employeeCommand(t, employee.EmployeeWriteInput{Kind: employee.CreateEmployee, Profile: idn.EmployeeProfile{DisplayName: "Synthetic Audit Refusal"}})
	v, _ := c.Snapshot()
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_employee_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='idn.employee.created' THEN RAISE EXCEPTION 'synthetic audit unavailable';END IF;RETURN NEW;END $$;CREATE TRIGGER synthetic_employee_audit_failure BEFORE INSERT ON org_audit_events FOR EACH ROW EXECUTE FUNCTION synthetic_employee_audit_failure()`); e != nil {
		t.Fatal(e)
	}
	if r, replay, e := s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, id, c); e != employee.ErrRegisterUnavailable || replay || r != (employee.EmployeeResult{}) {
		t.Fatal("audit failure acknowledged identity", e)
	}
	var rows int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_employee_identities WHERE org_id=$1 AND id=$2)+(SELECT count(*) FROM org_employee_profiles WHERE org_id=$1 AND employee_id=$2)+(SELECT count(*) FROM org_employee_commands WHERE org_id=$1 AND operation_id=$3)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3)`, fx.scope.OrgID().String(), id, v.OperationID).Scan(&rows); e != nil || rows != 0 {
		t.Fatal("audit failure left a partial employee", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_employee_audit_failure ON org_audit_events`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.WriteEmployee(t.Context(), fx.scope, employeeActor(fx), fx.member, id, c); e != nil {
		t.Fatal("audit recovery", e)
	}
}
