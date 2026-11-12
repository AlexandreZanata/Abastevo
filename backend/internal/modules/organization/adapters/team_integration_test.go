//go:build integration

package adapters_test

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	memberapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func teamStore(t *testing.T, fx legalFixtureState) *adapters.TeamStore {
	t.Helper()
	catalog, e := audit.NewCatalog(adapters.TeamAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &adapters.TeamStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func teamCommand(t *testing.T, v app.TeamWriteInput) app.TeamWrite {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, e := app.NewTeamWrite(v)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func teamWrite(t *testing.T, fx legalFixtureState, s *adapters.TeamStore, c app.TeamWrite) (app.TeamResult, bool, error) {
	t.Helper()
	v, e := c.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	id := ""
	if v.Kind == app.CreateTeam || v.Kind == app.AddTeamEmployee {
		id = onboardingID(t)
	}
	return s.WriteTeam(t.Context(), fx.scope, fx.actor, fx.member, id, c)
}
func TestTeamStoreLifecycleLinkPeriodsReplayAndForeignIsolation(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	org := fx.scope.OrgID().String()
	var employeeID string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, org, fx.member).Scan(&employeeID); e != nil {
		t.Fatal(e)
	}
	rolesBefore := func() string {
		t.Helper()
		var v string
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(r) ORDER BY org_id,id)::text FROM org_role_assignments r`).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}()
	create := teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Team"}})
	first, replay, e := teamWrite(t, fx, s, create)
	if e != nil || replay || first.Revision != 1 {
		t.Fatal("team create", e)
	}
	add := teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: first.TeamID, EmployeeID: employeeID, ExpectedRevision: 1})
	assigned, _, e := teamWrite(t, fx, s, add)
	if e != nil || assigned.Revision != 2 || assigned.LinkID == "" {
		t.Fatal("team add pending-profile employee", e)
	}
	if r, replay, e := teamWrite(t, fx, s, add); e != nil || !replay || r != assigned {
		t.Fatal("link replay", e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: first.TeamID, EmployeeID: employeeID, ExpectedRevision: 2})); e != d.ErrConflict {
		t.Fatal("duplicate current link", e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.ArchiveTeam, TeamID: first.TeamID, ExpectedRevision: 2})); !errors.Is(e, d.ErrStructureInUse) {
		t.Fatal("active link archive", e)
	}
	remove := teamCommand(t, app.TeamWriteInput{Kind: app.RemoveTeamEmployee, TeamID: first.TeamID, EmployeeID: employeeID, ExpectedRevision: 2})
	removed, _, e := teamWrite(t, fx, s, remove)
	if e != nil || removed.LinkID != assigned.LinkID || removed.Revision != 3 {
		t.Fatal("link removal", e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: first.TeamID, EmployeeID: onboardingID(t), ExpectedRevision: 3})); e != app.ErrNotFound {
		t.Fatal("foreign employee", e)
	}
	readd := teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: first.TeamID, EmployeeID: employeeID, ExpectedRevision: 3})
	reassigned, _, e := teamWrite(t, fx, s, readd)
	if e != nil || reassigned.LinkID == assigned.LinkID || reassigned.Revision != 4 {
		t.Fatal("re-add rewrote old period", e)
	}
	page, e := s.ListTeamLinks(t.Context(), fx.scope, fx.actor, fx.member, first.TeamID, app.TeamLinkFilter{Limit: 1})
	if e != nil || len(page.Links) != 1 || page.NextID == "" {
		t.Fatal("history page", e)
	}
	tail, e := s.ListTeamLinks(t.Context(), fx.scope, fx.actor, fx.member, first.TeamID, app.TeamLinkFilter{Limit: 1, AfterID: page.NextID})
	if e != nil || len(tail.Links) != 1 || tail.NextID != "" {
		t.Fatal("history tail", e)
	}
	if r, replay, e := teamWrite(t, fx, s, add); e != nil || !replay || r != assigned {
		t.Fatal("old add retry replaced by current", e)
	}
	if r, replay, e := teamWrite(t, fx, s, create); e != nil || !replay || r != first {
		t.Fatal("old create retry", e)
	}
	var rolesAfter string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(r) ORDER BY org_id,id)::text FROM org_role_assignments r`).Scan(&rolesAfter); e != nil || rolesBefore != rolesAfter {
		t.Fatal("team changed grants", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, fx.member); e == nil {
		t.Fatal("last owner suspension bypassed")
	}
	if _, e := s.FindTeam(t.Context(), fx.f.Organizations[1].Scope(t), first.TeamID); e != app.ErrNotFound {
		t.Fatal("foreign team", e)
	}
}
func TestTeamStoreConcurrentNamesReplayCASAndLateRollback(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	for _, mode := range []string{"names", "retry"} {
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 12)
		results := make([]app.TeamResult, 12)
		replays := make([]bool, 12)
		shared := teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Race " + mode}})
		commands := make([]app.TeamWrite, 12)
		ids := make([]string, 12)
		for i := range 12 {
			v, _ := shared.Snapshot()
			if mode == "names" {
				v.OperationID = onboardingID(t)
			}
			commands[i] = teamCommand(t, v)
			ids[i] = onboardingID(t)
		}
		for i := range 12 {
			wg.Go(func() {
				<-start
				results[i], replays[i], errs[i] = s.WriteTeam(t.Context(), fx.scope, fx.actor, fx.member, ids[i], commands[i])
			})
		}
		close(start)
		wg.Wait()
		wins, retried := 0, 0
		var ack app.TeamResult
		for i, e := range errs {
			if e == nil {
				wins++
				if replays[i] {
					retried++
				}
				if ack.TeamID == "" {
					ack = results[i]
				} else if mode == "retry" && ack != results[i] {
					t.Fatal("retry changed result")
				}
			} else if mode != "names" || e != app.ErrTeamNameDuplicate {
				t.Fatal("team race", e)
			}
		}
		if mode == "names" && wins != 1 || mode == "retry" && (wins != 12 || retried != 11) {
			t.Fatal("team race winners", wins, retried)
		}
	}
	first, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic CAS"}}))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 12)
	commands := make([]app.TeamWrite, 12)
	for i := range 12 {
		commands[i] = teamCommand(t, app.TeamWriteInput{Kind: app.ChangeTeam, TeamID: first.TeamID, ExpectedRevision: 1, Profile: d.TeamProfile{Name: "Synthetic CAS " + strconv.Itoa(i)}})
		wg.Go(func() { <-start; _, _, errs[i] = teamWrite(t, fx, s, commands[i]) })
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		} else if e != d.ErrConflict {
			t.Fatal("team CAS", e)
		}
	}
	if wins != 1 {
		t.Fatal("team CAS winners", wins)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_team_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic team receipt outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_team_commit_failure AFTER INSERT ON org_team_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_team_commit_failure()`); e != nil {
		t.Fatal(e)
	}
	failed := teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Failed"}})
	v, _ := failed.Snapshot()
	id := onboardingID(t)
	if r, replay, e := s.WriteTeam(t.Context(), fx.scope, fx.actor, fx.member, id, failed); e != app.ErrUnavailable || replay || r != (app.TeamResult{}) {
		t.Fatal("provisional failed result", e)
	}
	var remaining int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_teams WHERE org_id=$1 AND id=$2)+(SELECT count(*) FROM org_team_commands WHERE org_id=$1 AND operation_id=$3)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3)`, fx.scope.OrgID().String(), id, v.OperationID).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatal("failed team left state", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_team_commit_failure ON org_team_commands`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.WriteTeam(t.Context(), fx.scope, fx.actor, fx.member, id, failed); e != nil {
		t.Fatal("team recovery", e)
	}
}
func TestTeamUnitUsageIsDistinctAndRetainedUnitBlocksNewLinks(t *testing.T) {
	fx, employees := employeeFixture(t)
	r := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_teams,org_team_links TO "+r+"; GRANT SELECT,INSERT ON org_team_commands TO "+r); e != nil {
		t.Fatal(e)
	}
	s := teamStore(t, fx)
	units := operatingUnitStore(t, fx)
	unitID := onboardingID(t)
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, unitID, operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic Team Unit", Code: "TU"}})); e != nil {
		t.Fatal(e)
	}
	worker, _, e := writeEmployee(t, fx, employees, employeeCommand(t, memberapp.EmployeeWriteInput{Kind: memberapp.CreateEmployee, Profile: memberdomain.EmployeeProfile{DisplayName: "Synthetic Team Worker", HomeUnitID: unitID}}))
	if e != nil {
		t.Fatal(e)
	}
	teams := []app.TeamResult{}
	for _, name := range []string{"Synthetic First Crew", "Synthetic Second Crew"} {
		team, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: name, UnitID: unitID}}))
		if e != nil {
			t.Fatal(e)
		}
		teams = append(teams, team)
		if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: worker.EmployeeID, ExpectedRevision: 1})); e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT(org_unit_current_usage($1,$2)->>'employees')::int`, fx.scope.OrgID().String(), unitID).Scan(&count)
	}); e != nil || count != 1 {
		t.Fatal("employee double-counted", count, e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), worker.EmployeeID); e != nil {
		t.Fatal(e)
	}
	if _, _, e := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", operatingUnitCommand(t, app.UnitWriteInput{Kind: app.ArchiveUnit, UnitID: unitID, ExpectedRevision: 1})); e != nil {
		t.Fatal("empty structural team prevented unit archive", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), worker.EmployeeID); e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: teams[0].TeamID, EmployeeID: worker.EmployeeID, ExpectedRevision: 3})); e != app.ErrNotFound {
		t.Fatal("new link in archived unit", e)
	}
	archive := teamCommand(t, app.TeamWriteInput{Kind: app.ArchiveTeam, TeamID: teams[0].TeamID, ExpectedRevision: 3})
	if _, _, e := teamWrite(t, fx, s, archive); e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.RestoreTeam, TeamID: teams[0].TeamID, ExpectedRevision: 4})); e != app.ErrNotFound {
		t.Fatal("restore in archived unit", e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.ChangeTeam, TeamID: teams[0].TeamID, ExpectedRevision: 4, Profile: d.TeamProfile{Name: "Synthetic Corrected Unit"}})); e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.RestoreTeam, TeamID: teams[0].TeamID, ExpectedRevision: 5})); e != nil {
		t.Fatal(e)
	}
}

func TestTeamArchiveAndLinkAdmissionSerializeWithCAS(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	var employeeID string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member).Scan(&employeeID); e != nil {
		t.Fatal(e)
	}
	for i := range 12 {
		created, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Admission " + strconv.Itoa(i)}}))
		if e != nil {
			t.Fatal(e)
		}
		add := teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: created.TeamID, EmployeeID: employeeID, ExpectedRevision: 1})
		archive := teamCommand(t, app.TeamWriteInput{Kind: app.ArchiveTeam, TeamID: created.TeamID, ExpectedRevision: 1})
		var wg sync.WaitGroup
		start := make(chan struct{})
		var admitted, archived error
		wg.Go(func() { <-start; _, _, admitted = teamWrite(t, fx, s, add) })
		wg.Go(func() { <-start; _, _, archived = teamWrite(t, fx, s, archive) })
		close(start)
		wg.Wait()
		if admitted == nil {
			if archived != d.ErrConflict {
				t.Fatal("archive ignored concurrent link", archived)
			}
		} else if archived != nil || admitted != d.ErrConflict {
			t.Fatal("archive/link race", admitted, archived)
		}
		var invalid int
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_team_links l JOIN org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id) WHERE t.org_id=$1 AND t.id=$2 AND t.state='ARCHIVED' AND l.ended_at IS NULL`, fx.scope.OrgID().String(), created.TeamID).Scan(&invalid); e != nil || invalid != 0 {
			t.Fatal("archived current assignment", e)
		}
	}
	empty, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Unknown Source"}}))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unknown_team_usage(org_id uuid,team_id uuid,FOREIGN KEY(org_id,team_id) REFERENCES org_teams(org_id,id))`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.ArchiveTeam, TeamID: empty.TeamID, ExpectedRevision: 1})); e != app.ErrUnavailable {
		t.Fatal("unknown consumer inferred empty", e)
	}
}
func TestTeamLinkCommitFailureRollsBackLinkRevisionAndAudit(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	var employeeID string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member).Scan(&employeeID); e != nil {
		t.Fatal(e)
	}
	team, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Failed Link"}}))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_team_link_receipt_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic link receipt outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_team_link_receipt_failure AFTER INSERT ON org_team_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_team_link_receipt_failure()`); e != nil {
		t.Fatal(e)
	}
	command := teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employeeID, ExpectedRevision: 1})
	v, _ := command.Snapshot()
	if r, replay, e := teamWrite(t, fx, s, command); e != app.ErrUnavailable || replay || r != (app.TeamResult{}) {
		t.Fatal("failed link returned provisional acknowledgement", e)
	}
	var revision, partial int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT revision,(SELECT count(*) FROM org_team_links WHERE org_id=$1 AND team_id=$2)+(SELECT count(*) FROM org_team_commands WHERE org_id=$1 AND operation_id=$3)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$3) FROM org_teams WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), team.TeamID, v.OperationID).Scan(&revision, &partial); e != nil || revision != 1 || partial != 0 {
		t.Fatal("failed link changed data", revision, partial, e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_team_link_receipt_failure ON org_team_commands`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, command); e != nil {
		t.Fatal("link recovery", e)
	}
}
