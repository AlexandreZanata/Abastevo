//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func reviewTeamRevision(t *testing.T, fx legalFixtureState, team string) int64 {
	t.Helper()
	var revision int64
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT revision FROM org_teams WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), team).Scan(&revision); e != nil {
		t.Fatal(e)
	}
	return revision
}
func reviewAccount(t *testing.T, fx legalFixtureState, alias string) string {
	t.Helper()
	id := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, id, alias); e != nil {
		t.Fatal(e)
	}
	return id
}

// An invitation that never granted access is not an access reduction: ending it
// keeps the employee's team periods, while suspension or end of access still
// closes them with their existing causes.
func TestEndedInvitationKeepsTeamLinksButAccessReductionsCloseThem(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	org := fx.scope.OrgID().String()
	team, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Invitation Team"}}))
	if e != nil {
		t.Fatal(e)
	}
	account := reviewAccount(t, fx, "synthetic-team-invited")
	employeeID, memberID := onboardingID(t), onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employeeID); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, org, memberID, employeeID, account); e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employeeID, ExpectedRevision: 1})); e != nil {
		t.Fatal(e)
	}
	before := reviewTeamRevision(t, fx, team.TeamID)
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='ENDED',end_reason='ADMISSION_ENDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, memberID); e != nil {
		t.Fatal("end invitation", e)
	}
	var current int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_team_links WHERE org_id=$1 AND team_id=$2 AND employee_id=$3 AND ended_at IS NULL`, org, team.TeamID, employeeID).Scan(&current); e != nil || current != 1 || reviewTeamRevision(t, fx, team.TeamID) != before {
		t.Fatal("ended invitation closed a team period", current, e)
	}
	for _, reduction := range []struct{ sql, reason string }{
		{`UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, "MEMBERSHIP_SUSPENDED"},
		{`UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, "MEMBERSHIP_ENDED"},
	} {
		employee, member := teamDriver(t, fx, reviewAccount(t, fx, "synthetic-team-"+reduction.reason))
		if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employee, ExpectedRevision: reviewTeamRevision(t, fx, team.TeamID)})); e != nil {
			t.Fatal(e)
		}
		if e := unitSchemaExec(t, fx, reduction.sql, org, member); e != nil {
			t.Fatal(reduction.reason, e)
		}
		var reason string
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT end_reason FROM org_team_links WHERE org_id=$1 AND team_id=$2 AND employee_id=$3 AND ended_at IS NOT NULL`, org, team.TeamID, employee).Scan(&reason); e != nil || reason != reduction.reason {
			t.Fatal("access reduction kept a team period", reason, e)
		}
	}
}

// A designee whose account later became unavailable keeps an unchanged slot:
// unrelated changes still commit. A changed slot still requires an available member.
func TestUnchangedUnavailableDesigneeDoesNotBlockUnrelatedSettings(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	suspendedAccount := reviewAccount(t, fx, "synthetic-settings-suspended")
	_, suspended := teamDriver(t, fx, suspendedAccount)
	deletingAccount := reviewAccount(t, fx, "synthetic-settings-deleting")
	_, deleting := teamDriver(t, fx, deletingAccount)
	v := settingsRead(t, fx, s, 0).Values
	v.Responsibilities.ReviewFallback = d.ResponsibilityPair{PrimaryID: suspended}
	v.Responsibilities.DataProtection.SubstituteID = deleting
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, v, "")); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE accounts SET status='suspended' WHERE id=$1`, suspendedAccount); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO payment_billing_exits(id,account_id,operation_key,reason,state,requested_at,last_event_at) VALUES($1,$2,'synthetic-settings-deletion','ACCOUNT_DELETION','PENDING',clock_timestamp(),clock_timestamp())`, onboardingID(t), deletingAccount); e != nil {
		t.Fatal(e)
	}
	for _, step := range []struct {
		revision int64
		days     int
	}{{2, 45}, {3, 50}} {
		next := settingsRead(t, fx, s, 0).Values
		next.ClosurePeriodDays = step.days
		if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, step.revision, next, "")); e != nil {
			t.Fatal("unchanged unavailable designee blocked an unrelated change", step.days, e)
		}
	}
	head := settingsRead(t, fx, s, 0)
	if head.Revision != 4 || head.Values.ClosurePeriodDays != 50 || head.Values.Responsibilities.ReviewFallback.PrimaryID != suspended || head.Values.Responsibilities.DataProtection.SubstituteID != deleting {
		t.Fatal("unrelated change rewrote designations")
	}
	for _, member := range []string{suspended, deleting} {
		changed := head.Values
		changed.Responsibilities.Billing.SubstituteID = member
		if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 4, changed, "")); !errors.Is(e, d.ErrInvalid) {
			t.Fatal("unavailable member accepted in a changed slot", e)
		}
	}
	if settingsRead(t, fx, s, 0).Revision != 4 {
		t.Fatal("refused designation changed the head")
	}
}

// Every full CNPJ passes through an establishment; the all-zero number is not a
// registration even though its check digits compute. A root alone stays valid.
func TestAllZeroCNPJEstablishmentIsRefused(t *testing.T) {
	fx := legalFixture(t)
	org := fx.scope.OrgID().String()
	for _, root := range []struct{ identifier, cnpj string }{{"00000000", "00000000000000"}, {"EXEMPL01", "EXEMPL01000170"}} {
		entity := onboardingID(t)
		// A COMPANY root commits only with its first establishment.
		e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			if _, e := tx.Exec(t.Context(), `INSERT INTO org_legal_entities(org_id,id,kind,legal_name,identifier,purpose,state) VALUES($1,$2,'COMPANY','Synthetic root company',$3,'','DECLARED')`, org, entity, root.identifier); e != nil {
				return e
			}
			_, e := tx.Exec(t.Context(), `INSERT INTO org_establishments(org_id,id,legal_entity_id,cnpj) VALUES($1,$2,$3,$4)`, org, onboardingID(t), entity, root.cnpj)
			return e
		})
		if root.identifier == "00000000" {
			teamSQLCode(t, e, "23514")
		} else if e != nil {
			t.Fatal("valid company declaration refused", e)
		}
	}
	var zero int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_establishments WHERE cnpj='00000000000000'`).Scan(&zero); e != nil || zero != 0 {
		t.Fatal("all-zero establishment persisted", zero, e)
	}
}
