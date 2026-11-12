//go:build integration

package adapters_test

import (
	"context"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"testing"
	"time"
)

func TestRedemptionConfirmationDeadlineRollbackAndRecovery(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	a := redemptionActor(t, fx)
	v, code := redeemCode(t, fx, "AUDITOR")
	p, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil {
		t.Fatal(e)
	}
	c := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest}
	if _, e = s.Redeem(t.Context(), a, c); e != nil {
		t.Fatal(e)
	}
	// Isolated synthetic clock fixture only. Runtime has no clock parameter or DDL.
	if _, e = fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitation_acceptances DISABLE TRIGGER org_invitation_acceptance_guard;ALTER TABLE org_invitation_acceptances DISABLE TRIGGER org_invitation_acceptance_project`); e != nil {
		t.Fatal(e)
	}
	if _, e = fx.f.Admin.Exec(t.Context(), `UPDATE org_invitation_acceptances SET accepted_at=accepted_at-interval '168 hours',deadline=deadline-interval '168 hours',changed_at=changed_at-interval '168 hours' WHERE invitation_id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitation_acceptances ENABLE TRIGGER org_invitation_acceptance_guard;ALTER TABLE org_invitation_acceptances ENABLE TRIGGER org_invitation_acceptance_project;CREATE FUNCTION synthetic_admission_expiry_failure()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic expiry outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_admission_expiry_failure AFTER INSERT ON org_invitation_acceptance_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.state='EXPIRED')EXECUTE FUNCTION synthetic_admission_expiry_failure()`); e != nil {
		t.Fatal(e)
	}
	manager := invitationActor(fx)
	out, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v.ID, "READ", 0, "")
	if e == nil || out.MembershipID != "" {
		t.Fatal("expiry provisional result", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND state='WAITING'`, 1, v.ID)
	if _, e = fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_admission_expiry_failure ON org_invitation_acceptance_notices`); e != nil {
		t.Fatal(e)
	}
	_, _, e = s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v.ID, "CONFIRM", 1, onboardingID(t))
	if !errors.Is(e, app.ErrInvitationConflict) {
		t.Fatal("expired confirmation", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND state='EXPIRED'`, 1, v.ID)
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, v.MembershipID)
	invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, 0, v.MembershipID)
	read, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v.ID, "READ", 0, "")
	if e != nil || read.State != d.AdmissionExpired {
		t.Fatal("expiry recovery", e)
	}
}
func TestRedemptionFreshProofExpiresAtObservedLock(t *testing.T) {
	for _, gate := range []string{"root", "capability"} {
		t.Run(gate, func(t *testing.T) {
			fx := invitationFixture(t)
			s := redemptionStore(t, fx)
			a := redemptionActor(t, fx)
			v, code := redeemCode(t, fx, "DRIVER")
			if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',created_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, a.SessionID); e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			holder, e := fx.f.Admin.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer holder.Rollback(context.Background())
			query := "LockSetupRoot"
			if gate == "root" {
				_, e = holder.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String())
			} else {
				query = "LockStateForWrite"
				_, e = holder.Exec(ctx, `SELECT org_id FROM org_capabilities WHERE org_id=$1 AND capability='organizations' FOR UPDATE`, fx.scope.OrgID().String())
			}
			if e != nil {
				t.Fatal(e)
			}
			out := make(chan ownershipRaceResult, 1)
			go func() {
				_, e := s.Redeem(ctx, a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
				out <- ownershipRaceResult{err: e}
			}()
			ownershipObservedWait(t, transferFixtureState{legalFixtureState: fx.legalFixtureState}, query, out)
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			expired := false
			for !expired {
				select {
				case <-timer.C:
					t.Fatal("proof expiry not observed")
				case <-ticker.C:
					if e := fx.f.Admin.QueryRow(ctx, `SELECT authenticated_at<=clock_timestamp()-interval '15 minutes' FROM company_web_sessions WHERE id=$1`, a.SessionID).Scan(&expired); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := holder.Rollback(ctx); e != nil {
				t.Fatal(e)
			}
			result := ownershipRaceFinish(t, out)
			if !errors.Is(result.err, app.ErrEmployeeUnauthenticated) {
				t.Fatal("stale waited proof", result.err)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='PENDING' AND failed_attempts=0`, 1, v.ID)
			invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances`, 0)
		})
	}
}
func TestRedemptionIssuerRightsAndOrganizationInvalidateCurrentPreview(t *testing.T) {
	for _, change := range []string{"issuer rights", "archive", "managed"} {
		t.Run(change, func(t *testing.T) {
			fx := invitationFixture(t)
			s := redemptionStore(t, fx)
			a := redemptionActor(t, fx)
			_, code := redeemCode(t, fx, "DRIVER")
			switch change {
			case "issuer rights":
				tf := transferFixtureState{legalFixtureState: fx.legalFixtureState, actors: map[string]orgapp.CompanyActor{}}
				backup, _ := createTransferMember(t, tf, false)
				if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic surviving owner')`, fx.scope.OrgID().String(), onboardingID(t), backup, fx.member); e != nil {
					t.Fatal(e)
				}
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET revision=revision+1,ended_at=clock_timestamp() WHERE org_id=$1 AND membership_id=$2 AND role='OWNER'`, fx.scope.OrgID().String(), fx.member); e != nil {
					t.Fatal(e)
				}
			case "archive":
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
					t.Fatal(e)
				}
			case "managed":
				a = managedRedemptionActor(t, fx)
			}
			_, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			want := app.ErrTokenInvalid
			if change == "managed" {
				want = app.ErrManagedRestricted
			}
			if !errors.Is(e, want) {
				t.Fatal("changed source accepted", e)
			}
		})
	}
}
