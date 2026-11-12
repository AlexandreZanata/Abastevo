//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	"sync"
	"testing"
	"time"
)

func TestOffboardingLastOwnerRace(t *testing.T) {
	fx := membershipAccessFixture(t)
	second := accessMember(t, fx)
	admin := accessMember(t, fx)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic surviving owner')`, fx.scope.OrgID().String(), onboardingID(t), second, fx.member); e != nil {
		t.Fatal(e)
	}
	manager := fx.transfer.actors[admin]
	actor := app.EmployeeActor{AccountID: manager.AccountID, SessionID: manager.SessionID}
	s := offboardingStore(t, fx)
	commands := []app.OffboardingCommand{}
	for _, target := range []string{fx.member, second} {
		v, e := s.ReadOffboarding(t.Context(), fx.scope, actor, admin, target, false)
		if e != nil {
			t.Fatal(e)
		}
		commands = append(commands, offboardingCommand(t, v))
	}
	targets := []string{fx.member, second}
	out := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := s.CompleteOffboarding(t.Context(), fx.scope, actor, admin, commands[i])
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
func TestOffboardingFreshProofExpiresDuringObservedRootWait(t *testing.T) {
	for _, gate := range []string{"root"} {
		t.Run(gate, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			target := accessMember(t, fx)
			s := offboardingStore(t, fx)
			v := offboardingRead(t, fx, s, target)
			c := offboardingCommand(t, v)
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
				_, e := s.CompleteOffboarding(ctx, fx.scope, accessActor(fx), fx.member, c)
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
