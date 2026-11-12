//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"testing"
	"time"
)

func TestEmployeeMergeWarningsTerminatedHistoryForeignAndDisabledReplay(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := employeeMergeStore(t, fx)
	source := mergeEmployee(t, fx, "Synthetic Duplicate")
	_, _, e := writeEmployee(t, fx.legalFixtureState, mergeProfileStore(t, fx), employeeCommand(t, app.EmployeeWriteInput{Kind: app.CreateEmployee, Profile: d.EmployeeProfile{DisplayName: "Synthetic Duplicate", WorkEmail: "merge@example.invalid"}}))
	if !errors.Is(e, app.ErrDuplicateReview) {
		t.Fatal("automatic duplicate acceptance", e)
	}
	survivor := mergeEmployee(t, fx, "Synthetic Duplicate")
	c := mergeCommand(t, fx, mergePreview(t, fx, s, source, survivor))
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	foreign := onboardingID(t)
	if e := unitSchemaExec(t, other, `INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, other.scope.OrgID().String(), foreign); e != nil {
		t.Fatal(e)
	}
	if _, e := s.PreviewEmployeeMerge(t.Context(), fx.scope, accessActor(fx), fx.member, source, foreign); !errors.Is(e, app.ErrEmployeeNotFound) {
		t.Fatal("foreign survivor", e)
	}
	if _, e := s.ResolveEmployee(t.Context(), fx.scope, accessActor(fx), fx.member, foreign); !errors.Is(e, app.ErrEmployeeNotFound) {
		t.Fatal("foreign history", e)
	}
	if _, e := mergeWrite(t, fx, s, c); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: 1, Enabled: false}); e != nil {
		t.Fatal(e)
	}
	if out, e := mergeWrite(t, fx, s, c); !errors.Is(e, auth.ErrEntitlementRequired) || out.OperationID != "" {
		t.Fatal("disabled replay", e)
	}
}
func TestEmployeeMergeKeepsEndedMembershipAndActiveSurvivorAccount(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := employeeMergeStore(t, fx)
	old, c := terminatedForRehire(t, fx)
	live := accessMember(t, fx)
	survivor := accessRead(t, fx, live).State.EmployeeID
	before := mergeStableData(t, fx)
	command := mergeCommand(t, fx, mergePreview(t, fx, s, c.EmployeeID, survivor))
	if _, e := mergeWrite(t, fx, s, command); e != nil {
		t.Fatal("terminated source", e)
	}
	if mergeStableData(t, fx) != before {
		t.Fatal("ended membership/account/profile rewritten")
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_memberships WHERE id=$1 AND employee_id=$2 AND state='ENDED'`, 1, old, c.EmployeeID)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_memberships WHERE id=$1 AND employee_id=$2 AND state='ACTIVE'`, 1, live, survivor)
}
func TestEmployeeMergeResolutionRefusesCorruptCycleAndBoundsDepth(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		fx := membershipAccessFixture(t)
		s := employeeMergeStore(t, fx)
		a, b := mergeEmployee(t, fx, "Synthetic A"), mergeEmployee(t, fx, "Synthetic B")
		tx, e := fx.f.Admin.Begin(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(context.Background())
		if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(t.Context(), `ALTER TABLE org_employee_identities DISABLE TRIGGER org_employee_identity_revision`); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(t.Context(), `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1,merged_into=CASE WHEN id=$2 THEN $3::uuid ELSE $2::uuid END WHERE org_id=$1 AND id IN($2,$3)`, fx.scope.OrgID().String(), a, b); e != nil {
			t.Fatal(e)
		}
		if _, e := tx.Exec(t.Context(), `ALTER TABLE org_employee_identities ENABLE TRIGGER org_employee_identity_revision`); e != nil {
			t.Fatal(e)
		}
		if e := tx.Commit(t.Context()); e != nil {
			t.Fatal(e)
		}
		out, e := s.ResolveEmployee(t.Context(), fx.scope, accessActor(fx), fx.member, a)
		if !errors.Is(e, app.ErrRegisterUnavailable) || len(out.Chain) != 0 {
			t.Fatal("partial cyclic history", e)
		}
	})
	t.Run("depth", func(t *testing.T) {
		fx := membershipAccessFixture(t)
		s := employeeMergeStore(t, fx)
		first := mergeEmployee(t, fx, "Synthetic depth0")
		root := first
		for n := 1; n <= d.MaxEmployeeMergeDepth; n++ {
			next := mergeEmployee(t, fx, fmt.Sprintf("Synthetic depth%d", n))
			c := mergeCommand(t, fx, mergePreview(t, fx, s, root, next))
			if _, e := mergeWrite(t, fx, s, c); e != nil {
				t.Fatal("bounded chain", n, e)
			}
			root = next
		}
		out, e := s.ResolveEmployee(t.Context(), fx.scope, accessActor(fx), fx.member, first)
		if e != nil || len(out.Chain) != d.MaxEmployeeMergeDepth+1 {
			t.Fatal("accepted chain unreadable", e)
		}
		extra := mergeEmployee(t, fx, "Synthetic overflow")
		c := mergeCommand(t, fx, mergePreview(t, fx, s, root, extra))
		if _, e := mergeWrite(t, fx, s, c); !errors.Is(e, d.ErrMergeConflict) {
			t.Fatal("overflow merge", e)
		}
	})
}
func TestEmployeeMergeFreshProofAfterRootAndCapabilityWait(t *testing.T) {
	for _, gate := range []string{"root", "capability"} {
		t.Run(gate, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := employeeMergeStore(t, fx)
			a, b := mergeEmployee(t, fx, "Synthetic A"), mergeEmployee(t, fx, "Synthetic B")
			c := mergeCommand(t, fx, mergePreview(t, fx, s, a, b))
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
				_, e := s.MergeEmployees(ctx, fx.scope, accessActor(fx), fx.member, c)
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
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_identities WHERE org_id=$1 AND id=$2 AND state='ACTIVE' AND merged_into IS NULL AND revision=$3`, 1, fx.scope.OrgID().String(), c.SourceID, c.SourceRevision)
		})
	}
}
