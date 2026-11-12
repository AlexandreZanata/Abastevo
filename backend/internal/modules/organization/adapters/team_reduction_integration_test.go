//go:build integration

package adapters_test

import (
	"context"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/jackc/pgx/v5"
)

func teamDriver(t *testing.T, fx legalFixtureState, accountID string) (string, string) {
	t.Helper()
	employeeID, memberID := onboardingID(t), onboardingID(t)
	org := fx.scope.OrgID().String()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, []any{org, employeeID}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, memberID, employeeID, accountID}},
		{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'DRIVER','SELF',$3,clock_timestamp()-interval '1 minute',$4,'synthetic team driver')`, []any{org, onboardingID(t), memberID, fx.member}},
		{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, memberID}},
	} {
		if e := unitSchemaExec(t, fx, q.sql, q.args...); e != nil {
			t.Fatal("driver fixture", e)
		}
	}
	return employeeID, memberID
}
func TestTeamReductionsAndOrdinaryDefinerErasurePreservePeriodsAcrossOrganizations(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	settings := settingsStore(t, fx)
	accountID := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-team-driver','active')`, accountID); e != nil {
		t.Fatal(e)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	type linked struct {
		fx                           legalFixtureState
		teamID, employeeID, memberID string
	}
	links := []linked{}
	for _, source := range []legalFixtureState{fx, other} {
		employeeID, memberID := teamDriver(t, source, accountID)
		team, _, e := teamWrite(t, source, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Reduction Team"}}))
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e := teamWrite(t, source, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employeeID, ExpectedRevision: 1})); e != nil {
			t.Fatal(e)
		}
		if e := unitSchemaExec(t, source, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, source.scope.OrgID().String(), memberID); e != nil {
			t.Fatal("suspend reduction", e)
		}
		if e := unitSchemaExec(t, source, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, source.scope.OrgID().String(), memberID); e != nil {
			t.Fatal(e)
		}
		page, e := s.ListTeamLinks(t.Context(), source.scope, source.actor, source.member, team.TeamID, app.TeamLinkFilter{Limit: 10, State: "ACTIVE"})
		if e != nil || len(page.Links) != 0 {
			t.Fatal("membership activation resurrected team", e)
		}
		if _, _, e := teamWrite(t, source, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employeeID, ExpectedRevision: 2})); e != d.ErrConflict {
			t.Fatal("reduction did not invalidate stale command", e)
		}
		if _, _, e := teamWrite(t, source, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employeeID, ExpectedRevision: 3})); e != nil {
			t.Fatal("explicit new period", e)
		}
		configured := settingsRead(t, source, settings, 0).Values
		configured.Responsibilities.ReviewFallback = d.ResponsibilityPair{PrimaryID: memberID, SubstituteID: source.member}
		if _, _, e := settings.ChangeSettings(t.Context(), source.scope, source.actor, source.member, settingsCommand(t, 1, configured, "")); e != nil {
			t.Fatal(e)
		}
		links = append(links, linked{source, team.TeamID, employeeID, memberID})
	}
	// Rehearse the existing erasure definer under an ordinary, non-login owner.
	eraser := fx.f.RuntimeRole + "_team_eraser"
	role := pgx.Identifier{eraser}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE; GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,UPDATE ON organizations,accounts,org_role_assignments,org_teams,org_team_links TO "+role+"; GRANT SELECT ON org_employee_identities,payment_billing_exits TO "+role+"; GRANT SELECT,UPDATE ON org_ownership_transfers TO "+role+"; GRANT SELECT,INSERT ON org_ownership_notices TO "+role+"; GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices TO "+role+"; GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+role+"; ALTER TABLE org_memberships OWNER TO "+role+"; ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role+"; ALTER FUNCTION org_membership_check_account_exit(uuid) OWNER TO "+role); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := fx.f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	// Existing personal-account erasure touches its own private tables too. This
	// disposable fixture uses their established broad DML grant; the protected
	// enterprise definer above remains ordinary and explicitly scoped.
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}

	// Erase inside a caller transaction that already selected an unrelated scope:
	// after the deferred checks run, that caller scope must be back in place.
	callerScope := onboardingID(t)
	if e := pgx.BeginFunc(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
		if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, callerScope); e != nil {
			return e
		}
		if _, e := tx.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, accountID); e != nil {
			return e
		}
		if _, e := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); e != nil {
			return e
		}
		var restored string
		if e := tx.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&restored); e != nil {
			return e
		}
		if restored != callerScope {
			t.Error("erasure leaked organization scope inside the caller transaction")
		}
		return nil
	}); e != nil {
		t.Fatal("ordinary definer team erasure", e)
	}
	for _, l := range links {
		var active, ended, revision int
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_team_links WHERE org_id=$1 AND team_id=$2 AND ended_at IS NULL),(SELECT count(*) FROM org_team_links WHERE org_id=$1 AND team_id=$2 AND ended_at IS NOT NULL),(SELECT revision FROM org_teams WHERE org_id=$1 AND id=$2)`, l.fx.scope.OrgID().String(), l.teamID).Scan(&active, &ended, &revision); e != nil || active != 0 || ended != 2 || revision != 5 {
			t.Fatal("erasure silently missed scoped links", active, ended, revision, e)
		}
		cleared := settingsRead(t, l.fx, settings, 0)
		if cleared.Revision != 3 || cleared.Source != d.SettingsMembershipReduction || cleared.SourceMembershipID != l.memberID || cleared.Values.Responsibilities.ReviewFallback.PrimaryID != "" || cleared.Values.Responsibilities.ReviewFallback.SubstituteID != l.fx.member {
			t.Fatal("ordinary definer did not clear only scoped designation")
		}
		var valid bool
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_team_links l JOIN org_memberships m ON(m.org_id,m.id)=(l.org_id,l.source_membership_id) WHERE l.org_id=$1 AND l.team_id=$2 AND l.end_reason='MEMBERSHIP_ENDED' AND l.source_membership_revision=m.revision AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND m.state='ENDED')`, l.fx.scope.OrgID().String(), l.teamID).Scan(&valid); e != nil || !valid {
			t.Fatal("erasure cause missing", e)
		}
	}
	var unsafe bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls OR rolcanlogin FROM pg_roles WHERE rolname=$1`, eraser).Scan(&unsafe); e != nil || unsafe {
		t.Fatal("privileged erasure evidence", e)
	}
	var scope string
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&scope); e != nil || scope != "" {
		t.Fatal("erasure leaked organization scope", e)
	}
}

func TestTeamReductionFailureRollsBackMembershipRolesAndProjection(t *testing.T) {
	fx := teamSchemaFixture(t)
	s := teamStore(t, fx)
	accountID := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-team-reduction-failure','active')`, accountID); e != nil {
		t.Fatal(e)
	}
	employeeID, memberID := teamDriver(t, fx, accountID)
	team, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.CreateTeam, Profile: d.TeamProfile{Name: "Synthetic Reduction Failure"}}))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e := teamWrite(t, fx, s, teamCommand(t, app.TeamWriteInput{Kind: app.AddTeamEmployee, TeamID: team.TeamID, EmployeeID: employeeID, ExpectedRevision: 1})); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_team_reduction_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic reduction storage outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_team_reduction_failure AFTER UPDATE ON org_team_links DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_team_reduction_failure()`); e != nil {
		t.Fatal(e)
	}
	// The projection must also permit safety reduction in a suspended root.
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	end := `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`
	teamSQLCode(t, unitSchemaExec(t, fx, end, fx.scope.OrgID().String(), memberID), "23503")
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT m.state='ACTIVE' AND m.revision=2 AND t.revision=2 AND EXISTS(SELECT 1 FROM org_team_links WHERE org_id=m.org_id AND team_id=t.id AND ended_at IS NULL) AND EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=m.org_id AND membership_id=m.id AND ended_at IS NULL) FROM org_memberships m JOIN org_teams t ON t.org_id=m.org_id AND t.id=$3 WHERE m.org_id=$1 AND m.id=$2`, fx.scope.OrgID().String(), memberID, team.TeamID).Scan(&valid); e != nil || !valid {
		t.Fatal("reduction failure left partial access/history", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_team_reduction_failure ON org_team_links`); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, end, fx.scope.OrgID().String(), memberID); e != nil {
		t.Fatal("reduction recovery", e)
	}
	page, e := s.ListTeamLinks(t.Context(), fx.scope, fx.actor, fx.member, team.TeamID, app.TeamLinkFilter{Limit: 10})
	if e != nil || len(page.Links) != 1 {
		t.Fatal(e)
	}
	link, _ := page.Links[0].Snapshot(fx.scope)
	if link.EndReason != d.TeamLinkMembershipEnded {
		t.Fatal("reduction cause")
	}
}
