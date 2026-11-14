//go:build integration

package adapters_test

import (
	"errors"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

func rehireStore(t *testing.T, fx memberAccessFixture) *memberpg.RehireStore {
	t.Helper()
	fx.f.GrantRehireCommands(t)
	catalog, e := audit.NewCatalog(memberpg.RehireAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.RehireStore{Pool: fx.f.Runtime, Employees: fx.store.Memberships, Authority: &orgpg.RehireAuthority{MembershipAccessAuthority: orgpg.MembershipAccessAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func terminatedForRehire(t *testing.T, fx memberAccessFixture) (string, app.RehireCommand) {
	t.Helper()
	target := accessMember(t, fx)
	s := offboardingStore(t, fx)
	view := offboardingRead(t, fx, s, target)
	// Complete a real profile before ending access, then preserve its exact bytes.
	catalog, e := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	profiles := &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &orgpg.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	if _, _, e := writeEmployee(t, fx.legalFixtureState, profiles, employeeCommand(t, app.EmployeeWriteInput{Kind: app.CompleteEmployee, EmployeeID: view.State.EmployeeID, ExpectedRevision: 1, Profile: d.EmployeeProfile{DisplayName: "Synthetic Returning Employee", RegistrationNumber: onboardingID(t), WorkEmail: "rehire@example.invalid"}})); e != nil {
		t.Fatal(e)
	}
	c := offboardingCommand(t, view)
	c.TerminateEmployee = true
	out, e := s.CompleteOffboarding(t.Context(), fx.scope, accessActor(fx), fx.member, c)
	if e != nil {
		t.Fatal(e)
	}
	return target, app.RehireCommand{OperationID: onboardingID(t), EmployeeID: view.State.EmployeeID, ExpectedRevision: out.View.EmployeeRevision, Reason: "Synthetic new employment"}
}
func rehireWrite(t *testing.T, fx memberAccessFixture, s *memberpg.RehireStore, c app.RehireCommand) (app.RehireResult, error) {
	t.Helper()
	return s.Rehire(t.Context(), fx.scope, accessActor(fx), fx.member, c)
}
func rehireHistory(t *testing.T, fx memberAccessFixture, employee string) string {
	t.Helper()
	var snapshot string
	e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(m)ORDER BY m.id)FROM org_memberships m WHERE m.org_id=$1 AND m.employee_id=$2),(SELECT jsonb_agg(to_jsonb(r)ORDER BY r.id)FROM org_role_assignments r JOIN org_memberships m ON(m.org_id,m.id)=(r.org_id,r.membership_id)WHERE m.org_id=$1 AND m.employee_id=$2),(SELECT to_jsonb(p)FROM org_employee_profiles p WHERE p.org_id=$1 AND p.employee_id=$2),(SELECT jsonb_agg(to_jsonb(l)ORDER BY l.id)FROM org_team_links l WHERE l.org_id=$1 AND l.employee_id=$2))::text`, fx.scope.OrgID().String(), employee).Scan(&snapshot)
	if e != nil {
		t.Fatal(e)
	}
	return snapshot
}
func TestRehirePreservesHistoryAndRequiresDistinctInvitation(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := rehireStore(t, fx)
	old, c := terminatedForRehire(t, fx)
	before := rehireHistory(t, fx, c.EmployeeID)
	out, e := rehireWrite(t, fx, s, c)
	if e != nil || out.EmployeeID != c.EmployeeID || out.Revision != c.ExpectedRevision+1 || out.Replayed {
		t.Fatal("rehire", e)
	}
	if rehireHistory(t, fx, c.EmployeeID) != before {
		t.Fatal("historical/profile rewrite")
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_memberships WHERE org_id=$1 AND employee_id=$2 AND state<>'ENDED'`, 0, fx.scope.OrgID().String(), c.EmployeeID)
	again, e := rehireWrite(t, fx, s, c)
	if e != nil || !again.Replayed || again.OccurredAt != out.OccurredAt {
		t.Fatal("original retry", e)
	}
	changed := c
	changed.Reason = "Changed reason"
	if _, e := rehireWrite(t, fx, s, changed); !errors.Is(e, app.ErrAccessOperationReused) {
		t.Fatal("reason reuse", e)
	}
	// A later ordinary persisted employment transition cannot rewrite the acknowledgement.
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), c.EmployeeID); e != nil {
		t.Fatal(e)
	}
	again, e = rehireWrite(t, fx, s, c)
	if e != nil || !again.Replayed || again.State != d.EmployeeActive || again.Revision != out.Revision || again.OccurredAt != out.OccurredAt {
		t.Fatal("later-state receipt", e)
	}
	later := c
	later.OperationID = onboardingID(t)
	later.ExpectedRevision += 2
	if _, e := rehireWrite(t, fx, s, later); e != nil {
		t.Fatal("new employment epoch", e)
	}
	if rehireHistory(t, fx, c.EmployeeID) != before {
		t.Fatal("later rehire restored history")
	}
	plan, _ := invitationPlan(t, fx.invitationFixtureState)
	inv, _, e := fx.invites.WriteInvitation(t.Context(), fx.scope, accessActor(fx), fx.member, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: c.EmployeeID, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}, plan)
	if e != nil {
		t.Fatal("new invitation", e)
	}
	v, e := inv.Snapshot(fx.scope)
	if e != nil || v.MembershipID == old || v.EmployeeID != c.EmployeeID {
		t.Fatal("distinct period", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_memberships WHERE org_id=$1 AND id=$2 AND state='ENDED'`, 1, fx.scope.OrgID().String(), old)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_memberships WHERE org_id=$1 AND id=$2 AND state='INVITED' AND account_id IS NULL`, 1, fx.scope.OrgID().String(), v.MembershipID)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND membership_id IN($2,$3) AND ended_at IS NULL`, 0, fx.scope.OrgID().String(), old, v.MembershipID)
}
func TestRehireTwelveWayRetriesAndDistinctCAS(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(map[bool]string{true: "same", false: "distinct"}[same], func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := rehireStore(t, fx)
			_, base := terminatedForRehire(t, fx)
			before := rehireHistory(t, fx, base.EmployeeID)
			results := make(chan app.RehireResult, 12)
			errs := make(chan error, 12)
			var wg sync.WaitGroup
			for range 12 {
				c := base
				if !same {
					c.OperationID = onboardingID(t)
				}
				wg.Go(func() { out, e := rehireWrite(t, fx, s, c); results <- out; errs <- e })
			}
			wg.Wait()
			close(results)
			close(errs)
			won, lost, effects := 0, 0, 0
			for e := range errs {
				if e == nil {
					won++
				} else if errors.Is(e, d.ErrTransition) || errors.Is(e, d.ErrConflict) {
					lost++
				} else {
					t.Fatal("contender", e)
				}
			}
			for out := range results {
				if out.OperationID != "" && !out.Replayed {
					effects++
				}
			}
			if effects != 1 || same && won != 12 || !same && (won != 1 || lost != 11) {
				t.Fatal("retry/CAS", won, lost, effects)
			}
			if rehireHistory(t, fx, base.EmployeeID) != before {
				t.Fatal("contention changed history")
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_rehire_commands WHERE org_id=$1 AND employee_id=$2`, 1, fx.scope.OrgID().String(), base.EmployeeID)
		})
	}
}
func TestRehireDeferredFailureRollbackAndRecovery(t *testing.T) {
	for _, table := range []string{"org_employee_rehire_commands", "org_audit_events"} {
		t.Run(table, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := rehireStore(t, fx)
			_, c := terminatedForRehire(t, fx)
			before := rehireHistory(t, fx, c.EmployeeID)
			if _, e := fx.f.Admin.Exec(t.Context(), "CREATE FUNCTION syn_rehire_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic rehire failure';END $$;CREATE CONSTRAINT TRIGGER syn_rehire_failure AFTER INSERT ON "+pgx.Identifier{table}.Sanitize()+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION syn_rehire_failure()"); e != nil {
				t.Fatal(e)
			}
			out, e := rehireWrite(t, fx, s, c)
			if !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
				t.Fatal("provisional result", e)
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_identities WHERE org_id=$1 AND id=$2 AND state='TERMINATED' AND revision=$3`, 1, fx.scope.OrgID().String(), c.EmployeeID, c.ExpectedRevision)
			if rehireHistory(t, fx, c.EmployeeID) != before {
				t.Fatal("partial history change")
			}
			if _, e := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER syn_rehire_failure ON "+pgx.Identifier{table}.Sanitize()); e != nil {
				t.Fatal(e)
			}
			if _, e := rehireWrite(t, fx, s, c); e != nil {
				t.Fatal("recovery", e)
			}
		})
	}
}
