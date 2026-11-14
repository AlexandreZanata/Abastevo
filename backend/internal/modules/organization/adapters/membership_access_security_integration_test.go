//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
	"time"
)

func TestMembershipAccessSelfForeignAndCurrentProof(t *testing.T) {
	fx := membershipAccessFixture(t)
	target := accessMember(t, fx)
	current := accessRead(t, fx, target)
	c := app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: fx.member, ExpectedRevision: accessRead(t, fx, fx.member).State.Revision, OperationID: onboardingID(t), Reason: "Synthetic self pause"}
	if _, e := accessWrite(t, fx, c); !errors.Is(e, d.ErrSelfSuspension) {
		t.Fatal("self suspension", e)
	}
	c.MembershipID = target
	c.ExpectedRevision = current.State.Revision
	c.OperationID = onboardingID(t)
	impostor := redemptionActor(t, fx.invitationFixtureState)
	for _, id := range []string{target, onboardingID(t)} {
		bad := c
		bad.MembershipID = id
		if _, e := fx.access.ChangeMembershipAccess(t.Context(), fx.scope, impostor, fx.member, bad); !errors.Is(e, access.ErrNoMembership) {
			t.Fatal("target oracle", e)
		}
	}
	c.MembershipID = onboardingID(t)
	if _, e := accessWrite(t, fx, c); !errors.Is(e, app.ErrEmployeeNotFound) {
		t.Fatal("foreign/missing", e)
	}
	c.MembershipID = target
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=(SELECT family_id FROM company_web_sessions WHERE id=$1)`, fx.actor.SessionID); e != nil {
		t.Fatal(e)
	}
	if _, e := accessWrite(t, fx, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
		t.Fatal("revoked family", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_commands`, 0)
	if state := unitState(t, fx, target); state != "ACTIVE" {
		t.Fatal("denial changed target")
	}
}
func unitState(t *testing.T, fx memberAccessFixture, id string) string {
	t.Helper()
	var state string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), id).Scan(&state); e != nil {
		t.Fatal(e)
	}
	return state
}
func TestMembershipAccessTwelveWayRetryAndCAS(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(map[bool]string{true: "same", false: "distinct"}[same], func(t *testing.T) {
			fx := membershipAccessFixture(t)
			target := accessMember(t, fx)
			v := accessRead(t, fx, target)
			c := app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: v.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic concurrent pause"}
			type result struct {
				value app.MembershipAccessResult
				e     error
			}
			results := make(chan result, 12)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					command := c
					if !same {
						command.OperationID = onboardingID(t)
					}
					out, e := accessWrite(t, fx, command)
					results <- result{out, e}
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			changed, replayed, conflicts := 0, 0, 0
			for r := range results {
				if r.e == nil {
					if r.value.Replayed {
						replayed++
					} else {
						changed++
					}
				} else if errors.Is(r.e, d.ErrConflict) {
					conflicts++
				} else {
					t.Fatal("race result", r.e)
				}
			}
			if changed != 1 || (same && replayed != 11) || (!same && conflicts != 11) {
				t.Fatal("CAS outcomes", changed, replayed, conflicts)
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_commands WHERE membership_id=$1`, 1, target)
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_notices WHERE membership_id=$1`, 1, target)
		})
	}
}
func TestMembershipAccessLastOwnerRace(t *testing.T) {
	fx := membershipAccessFixture(t)
	second := accessMember(t, fx)
	admin := accessMember(t, fx)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic surviving owner')`, fx.scope.OrgID().String(), onboardingID(t), second, fx.member); e != nil {
		t.Fatal(e)
	}
	manager := fx.transfer.actors[admin]
	actor := app.EmployeeActor{AccountID: manager.AccountID, SessionID: manager.SessionID}
	revisions := []int64{accessRead(t, fx, fx.member).State.Revision, accessRead(t, fx, second).State.Revision}
	targets := []string{fx.member, second}
	out := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := fx.access.ChangeMembershipAccess(t.Context(), fx.scope, actor, admin, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: targets[i], ExpectedRevision: revisions[i], OperationID: onboardingID(t), Reason: "Synthetic owner race"})
			out <- e
		}()
	}
	close(start)
	wg.Wait()
	close(out)
	won, protected := 0, 0
	for e := range out {
		if e == nil {
			won++
		} else if errors.Is(e, kernel.ErrLastOrgOwner) {
			protected++
		} else {
			t.Fatal("owner race result", e)
		}
	}
	if won != 1 || protected != 1 {
		t.Fatal("last owner not preserved", won, protected)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(DISTINCT m.id) FROM org_memberships m JOIN org_role_assignments r ON(r.org_id,r.membership_id)=(m.org_id,m.id) WHERE m.org_id=$1 AND m.state='ACTIVE' AND r.role='OWNER' AND r.ended_at IS NULL`, 1, fx.scope.OrgID().String())
}
func TestMembershipAccessDeferredFailureRollbackAndRecovery(t *testing.T) {
	for _, table := range []string{"org_membership_access_commands", "org_membership_access_notices", "org_audit_events"} {
		t.Run(table, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			target := accessMember(t, fx)
			initial := accessRead(t, fx, target)
			name := "synthetic_access_failure"
			if _, e := fx.f.Admin.Exec(t.Context(), "CREATE FUNCTION "+name+"()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic lifecycle failure';END $$;CREATE CONSTRAINT TRIGGER "+name+" AFTER INSERT ON "+pgx.Identifier{table}.Sanitize()+" DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION "+name+"()"); e != nil {
				t.Fatal(e)
			}
			c := app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: initial.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic atomic pause"}
			out, e := accessWrite(t, fx, c)
			if !errors.Is(e, app.ErrRegisterUnavailable) || out.OperationID != "" {
				t.Fatal("provisional result", e)
			}
			if unitState(t, fx, target) != "ACTIVE" {
				t.Fatal("partial suspension")
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_commands`, 0)
			if _, e := fx.f.Admin.Exec(t.Context(), "DROP TRIGGER "+name+" ON "+pgx.Identifier{table}.Sanitize()); e != nil {
				t.Fatal(e)
			}
			if _, e := accessWrite(t, fx, c); e != nil {
				t.Fatal("recovery", e)
			}
		})
	}
}
func TestMembershipAccessFreshProofAfterRootAndCapabilityWait(t *testing.T) {
	for _, gate := range []string{"root", "capability"} {
		t.Run(gate, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			target := accessMember(t, fx)
			v := accessRead(t, fx, target)
			action := app.MembershipSuspend
			if gate == "capability" {
				p, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: v.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic prepared pause"})
				if e != nil {
					t.Fatal(e)
				}
				v = p
				action = app.MembershipReactivate
			}
			if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',created_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, fx.actor.SessionID); e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			holder, e := fx.f.Admin.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer holder.Rollback(context.Background())
			query := "LockLegalOrganization"
			if gate == "root" {
				_, e = holder.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String())
			} else {
				query = "LockStateForWrite"
				_, e = holder.Exec(ctx, `SELECT org_id FROM org_capabilities WHERE org_id=$1 AND capability='organizations' FOR UPDATE`, fx.scope.OrgID().String())
			}
			if e != nil {
				t.Fatal(e)
			}
			done := make(chan ownershipRaceResult, 1)
			go func() {
				_, e := fx.access.ChangeMembershipAccess(ctx, fx.scope, accessActor(fx), fx.member, app.MembershipAccessWrite{Action: action, MembershipID: target, ExpectedRevision: v.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic waited command"})
				done <- ownershipRaceResult{err: e}
			}()
			ownershipObservedWait(t, fx.transfer, query, done)
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			expired := false
			for !expired {
				select {
				case <-timer.C:
					t.Fatal("proof clock boundary not observed")
				case <-ticker.C:
					if e := fx.f.Admin.QueryRow(ctx, `SELECT authenticated_at<=clock_timestamp()-interval '15 minutes' FROM company_web_sessions WHERE id=$1`, fx.actor.SessionID).Scan(&expired); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := holder.Rollback(ctx); e != nil {
				t.Fatal(e)
			}
			r := ownershipRaceFinish(t, done)
			if !errors.Is(r.err, app.ErrEmployeeUnauthenticated) {
				t.Fatal("stale waited proof", r.err)
			}
			if unitState(t, fx, target) != string(v.State.State) {
				t.Fatal("waited denial changed state")
			}
		})
	}
}
func TestMembershipAccessReductionsRemainAvailableWithDisabledCapability(t *testing.T) {
	fx := membershipAccessFixture(t)
	target := accessMember(t, fx)
	v := accessRead(t, fx, target)
	if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	paused, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: v.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic disabled reduction"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := fx.access.ReadMembershipAccess(t.Context(), fx.scope, accessActor(fx), fx.member, target); e != nil {
		t.Fatal("disabled read", e)
	}
	if _, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipReactivate, MembershipID: target, ExpectedRevision: paused.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic disabled resume"}); !errors.Is(e, access.ErrEntitlementRequired) {
		t.Fatal("disabled gain", e)
	}
	if rows, e := accesspg.NewStore(fx.f.Runtime).Candidates(t.Context(), fx.scope, target); e != nil || len(rows) != 0 {
		t.Fatal("suspended authority survived", e)
	}
}
