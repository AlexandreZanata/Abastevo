//go:build integration

package adapters_test

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func teamSchemaFixture(t *testing.T) legalFixtureState {
	t.Helper()
	fx := legalFixture(t)
	grantTeamTables(t, fx)
	return fx
}
func grantTeamTables(t *testing.T, fx legalFixtureState) {
	t.Helper()
	r := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_teams,org_team_links TO "+r+"; GRANT SELECT,INSERT ON org_team_commands TO "+r); e != nil {
		t.Fatal(e)
	}
}
func teamSQLCode(t *testing.T, err error, want string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != want {
		t.Fatalf("team SQL refusal: want %s got %v", want, err)
	}
	if pe.Detail != "" {
		t.Fatal("private SQL refusal disclosed details")
	}
}
func teamSchemaLink(t *testing.T, fx legalFixtureState, teamID, employeeID string) error {
	t.Helper()
	op, id := onboardingID(t), onboardingID(t)
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, e := tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash) VALUES($1,$2,$3,'org.team.changed','team',$4,'member_added',repeat('a',64))`, fx.scope.OrgID().String(), op, fx.actor.AccountID, teamID); e != nil {
			return e
		}
		_, e := tx.Exec(t.Context(), `INSERT INTO org_team_links(org_id,id,team_id,employee_id,created_operation_id) VALUES($1,$2,$3,$4,$5)`, fx.scope.OrgID().String(), id, teamID, employeeID, op)
		return e
	})
}
func TestTeamSchemaNamesLinksHistoryAndForeignIsolation(t *testing.T) {
	fx := teamSchemaFixture(t)
	org := fx.scope.OrgID().String()
	teamID, employeeID := onboardingID(t), onboardingID(t)
	create := `INSERT INTO org_teams(org_id,id,name,state) VALUES($1,$2,$3,'ACTIVE')`
	if e := unitSchemaExec(t, fx, create, org, teamID, "Synthetic Crew"); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, create, org, onboardingID(t), "synthetic crew"), "P8001")
	if e := unitSchemaExec(t, fx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employeeID); e != nil {
		t.Fatal(e)
	}
	if e := teamSchemaLink(t, fx, teamID, employeeID); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, teamSchemaLink(t, fx, teamID, employeeID), "P8002")
	teamSQLCode(t, teamSchemaLink(t, fx, teamID, onboardingID(t)), "23503")
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_teams SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, teamID), "P8007")
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, employeeID); e != nil {
		t.Fatal("employee reduction", e)
	}
	var revision, ended int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT revision,(SELECT count(*) FROM org_team_links WHERE org_id=$1 AND team_id=$2 AND ended_at IS NOT NULL AND end_reason='EMPLOYEE_INACTIVE' AND source_employee_revision=2) FROM org_teams WHERE org_id=$1 AND id=$2`, org, teamID).Scan(&revision, &ended); e != nil || revision != 3 || ended != 1 {
		t.Fatal("reduction history/revision", revision, ended, e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_employee_identities SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, employeeID); e != nil {
		t.Fatal(e)
	}
	var active int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_team_links WHERE org_id=$1 AND team_id=$2 AND ended_at IS NULL`, org, teamID).Scan(&active); e != nil || active != 0 {
		t.Fatal("reactivation resurrected links", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_teams SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, teamID); e != nil {
		t.Fatal("history prevented archive", e)
	}
	teamSQLCode(t, teamSchemaLink(t, fx, teamID, employeeID), "55000")
	if e := unitSchemaExec(t, fx, create, org, onboardingID(t), "SYNTHETIC CREW"); e != nil {
		t.Fatal("archived name reserved", e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_teams SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, teamID), "P8001")
	teamSQLCode(t, unitSchemaExec(t, fx, `DELETE FROM org_team_links WHERE org_id=$1`, org), "55000")
	var leaked int
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_team_links`).Scan(&leaked); e != nil || leaked != 0 {
		t.Fatal("unscoped team links", e)
	}
}
func TestTeamSchemaSerializesDuplicateAdmission(t *testing.T) {
	fx := teamSchemaFixture(t)
	org := fx.scope.OrgID().String()
	teamID, employeeID := onboardingID(t), onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_teams(org_id,id,name,state) VALUES($1,$2,'Synthetic Race','ACTIVE')`, org, teamID); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employeeID); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 12)
	for i := range 12 {
		wg.Go(func() { <-start; errs[i] = teamSchemaLink(t, fx, teamID, employeeID) })
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		} else {
			teamSQLCode(t, e, "P8002")
		}
	}
	if wins != 1 {
		t.Fatal("duplicate link winners", wins)
	}
}

func TestTeamUnitArchiveAndSchemaAdmissionSerialize(t *testing.T) {
	fx := teamSchemaFixture(t)
	org := fx.scope.OrgID().String()
	var employeeID string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, org, fx.member).Scan(&employeeID); e != nil {
		t.Fatal(e)
	}
	for i := range 12 {
		unitID, teamID := onboardingID(t), onboardingID(t)
		if e := unitSchemaExec(t, fx, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,$3,$4,'UTC','ACTIVE')`, org, unitID, "Synthetic Team Admission Unit "+strconv.Itoa(i), "TR"+strconv.Itoa(i)); e != nil {
			t.Fatal(e)
		}
		if e := unitSchemaExec(t, fx, `INSERT INTO org_teams(org_id,id,name,unit_id,state) VALUES($1,$2,$3,$4,'ACTIVE')`, org, teamID, "Synthetic Team Unit Race "+strconv.Itoa(i), unitID); e != nil {
			t.Fatal(e)
		}
		var admitted, archived error
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Go(func() { <-start; admitted = teamSchemaLink(t, fx, teamID, employeeID) })
		wg.Go(func() {
			<-start
			archived = unitSchemaExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, unitID)
		})
		close(start)
		wg.Wait()
		if admitted == nil {
			teamSQLCode(t, archived, "P7703")
		} else {
			teamSQLCode(t, admitted, "23503")
			if archived != nil {
				t.Fatal("unit archive/admission both failed", archived)
			}
		}
		var invalid int
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_team_links l JOIN org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id) JOIN org_units u ON(u.org_id,u.id)=(t.org_id,t.unit_id) WHERE l.org_id=$1 AND u.id=$2 AND l.ended_at IS NULL AND u.state='ARCHIVED'`, org, unitID).Scan(&invalid); e != nil || invalid != 0 {
			t.Fatal("archived unit admitted team employee", e)
		}
	}
}
