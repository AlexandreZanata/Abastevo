//go:build integration

package adapters_test

import (
	"context"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"testing"
	"time"
)

func TestRehireForeignMergedAndCurrentAuthority(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := rehireStore(t, fx)
	_, c := terminatedForRehire(t, fx)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	foreign := onboardingID(t)
	if e := unitSchemaExec(t, other, `INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, other.scope.OrgID().String(), foreign); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, other, `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1 WHERE org_id=$1 AND id=$2`, other.scope.OrgID().String(), foreign); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{foreign, onboardingID(t)} {
		bad := c
		bad.EmployeeID = id
		if _, e := rehireWrite(t, fx, s, bad); !errors.Is(e, app.ErrEmployeeNotFound) {
			t.Fatal("foreign/missing private target", e)
		}
	}
	merged, survivor := invitationEmployee(t, fx.invitationFixtureState), invitationEmployee(t, fx.invitationFixtureState)
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_employee_identities SET state='TERMINATED',merged_into=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), merged, survivor); e != nil {
		t.Fatal(e)
	}
	bad := c
	bad.EmployeeID = merged
	if _, e := rehireWrite(t, fx, s, bad); !errors.Is(e, d.ErrMerged) {
		t.Fatal("merged employee rehire", e)
	}
	stale := c
	stale.ExpectedRevision--
	if _, e := rehireWrite(t, fx, s, stale); !errors.Is(e, d.ErrConflict) {
		t.Fatal("stale identity revision", e)
	}
	if _, e := rehireWrite(t, fx, s, c); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: 1, Enabled: false}); e != nil {
		t.Fatal(e)
	}
	if out, e := rehireWrite(t, fx, s, c); !errors.Is(e, auth.ErrEntitlementRequired) || out.OperationID != "" {
		t.Fatal("disabled capability replay", e)
	}
}
func TestRehireRefusesCorruptLegacyOverlap(t *testing.T) {
	fx := membershipAccessFixture(t)
	s := rehireStore(t, fx)
	target := accessMember(t, fx)
	v := accessRead(t, fx, target)
	// Ordinary SQL refuses termination with live access; corrupted legacy input
	// is injected only by the disposable administrator, with the guard restored.
	teamSQLCode(t, unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.State.EmployeeID), "23514")
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
	if _, e := tx.Exec(t.Context(), `UPDATE org_employee_identities SET state='TERMINATED',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.State.EmployeeID); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(t.Context(), `ALTER TABLE org_employee_identities ENABLE TRIGGER org_employee_identity_revision`); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(t.Context()); e != nil {
		t.Fatal(e)
	}
	c := app.RehireCommand{OperationID: onboardingID(t), EmployeeID: v.State.EmployeeID, ExpectedRevision: 2, Reason: "Synthetic corrupt-state refusal"}
	if out, e := rehireWrite(t, fx, s, c); !errors.Is(e, d.ErrConflict) || out.OperationID != "" {
		t.Fatal("overlap accepted", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_rehire_commands`, 0)
}
func TestRehireFreshProofAfterRootAndCapabilityWait(t *testing.T) {
	for _, gate := range []string{"root", "capability"} {
		t.Run(gate, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			s := rehireStore(t, fx)
			_, c := terminatedForRehire(t, fx)
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
				_, e := s.Rehire(ctx, fx.scope, accessActor(fx), fx.member, c)
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
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_employee_identities WHERE org_id=$1 AND id=$2 AND state='TERMINATED' AND revision=$3`, 1, fx.scope.OrgID().String(), c.EmployeeID, c.ExpectedRevision)
		})
	}
}
