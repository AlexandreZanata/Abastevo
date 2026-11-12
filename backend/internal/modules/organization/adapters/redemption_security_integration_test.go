//go:build integration

package adapters_test

import (
	"errors"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
)

func TestRedemptionUniformInvalidityLockoutConsentAndAlreadyMember(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	a := redemptionActor(t, fx)
	v, code := redeemCode(t, fx, "DRIVER")
	wrong := strings.Split(code, ".")
	wrong[2] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	bad := strings.Join(wrong, ".")
	for n := 1; n <= 5; n++ {
		if n == 3 {
			bad = strings.Join(wrong[:2], ".") + "."
		}
		if _, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: bad}); !errors.Is(e, app.ErrTokenInvalid) {
			t.Fatal("wrong secret", n, e)
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND failed_attempts=$2`, 1, v.ID, n)
	}
	if _, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code}); !errors.Is(e, app.ErrTokenInvalid) {
		t.Fatal("locked secret", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_notices WHERE invitation_id=$1 AND cause='TOO_MANY_ATTEMPTS'`, 1, v.ID)
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, v.MembershipID)
	_, code2 := redeemCode(t, fx, "DRIVER")
	p, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code2})
	if e != nil {
		t.Fatal(e)
	}
	c := app.RedemptionCommand{Action: "ACCEPT", OperationID: onboardingID(t), Code: code2, Consent: true, ConsentDigest: strings.Repeat("0", 64)}
	if _, e = s.Redeem(t.Context(), a, c); !errors.Is(e, app.ErrConsentChanged) {
		t.Fatal("unreviewed consent", e)
	}
	c.ConsentDigest = p.ConsentDigest
	if _, e = s.Redeem(t.Context(), a, c); e != nil {
		t.Fatal(e)
	}
	v3, code3 := redeemCode(t, fx, "DRIVER")
	p, e = s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code3})
	if e != nil {
		t.Fatal(e)
	}
	c = app.RedemptionCommand{Action: "ACCEPT", OperationID: onboardingID(t), Code: code3, Consent: true, ConsentDigest: p.ConsentDigest}
	if _, e = s.Redeem(t.Context(), a, c); !errors.Is(e, app.ErrAlreadyMember) {
		t.Fatal("duplicate live account", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='PENDING'`, 1, v3.ID)
	decline := app.RedemptionCommand{Action: "DECLINE", OperationID: onboardingID(t), Code: code3}
	if _, e := s.Redeem(t.Context(), a, decline); !errors.Is(e, app.ErrAlreadyMember) {
		t.Fatal("existing member decline consumed token", e)
	}
	refuser := redemptionActor(t, fx)
	out, e := s.Redeem(t.Context(), refuser, decline)
	if e != nil || out.State != "DECLINED" {
		t.Fatal("decline", e)
	}
	out, e = s.Redeem(t.Context(), refuser, decline)
	if e != nil || !out.Replayed {
		t.Fatal("decline retry", e)
	}
	if _, e = s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: "AIN1." + onboardingID(t) + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}); !errors.Is(e, memberpg.ErrUnknownInvitationLocator) {
		t.Fatal("unknown", e)
	}
}
func TestRedemptionConcurrentSameAndDifferentCallers(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_operation", false: "different_accounts"}[same], func(t *testing.T) {
			fx := invitationFixture(t)
			s := redemptionStore(t, fx)
			v, code := redeemCode(t, fx, "DRIVER")
			a := redemptionActor(t, fx)
			p, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			if e != nil {
				t.Fatal(e)
			}
			c := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest}
			actors := make([]app.EmployeeActor, 12)
			for i := range actors {
				actors[i] = a
				if !same {
					actors[i] = redemptionActor(t, fx)
				}
			}
			digests := map[string]string{}
			for _, actor := range actors {
				preview, e := s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "PREVIEW", Code: code})
				if e != nil {
					t.Fatal(e)
				}
				digests[actor.AccountID] = preview.ConsentDigest
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make(chan struct {
				replay bool
				e      error
			}, 12)
			for _, actor := range actors {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					op := c
					op.ConsentDigest = digests[actor.AccountID]
					if !same {
						op.OperationID = onboardingID(t)
					}
					out, e := s.Redeem(t.Context(), actor, op)
					results <- struct {
						replay bool
						e      error
					}{out.Replayed, e}
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			success, replays, invalid := 0, 0, 0
			for r := range results {
				if r.e == nil {
					if r.replay {
						replays++
					} else {
						success++
					}
				} else if errors.Is(r.e, app.ErrTokenInvalid) {
					invalid++
				} else {
					t.Fatal("unexpected race result", r.e)
				}
			}
			if success != 1 || (same && replays != 11) || (!same && invalid != 11) {
				t.Fatal("race outcomes", success, replays, invalid)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ACTIVE'`, 1, v.MembershipID)
			invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, 1, v.MembershipID)
		})
	}
}
func TestRedemptionSourceReductionsProofAndLateRollback(t *testing.T) {
	for _, producer := range []string{"receipt", "notice", "audit"} {
		t.Run(producer, func(t *testing.T) {
			fx := invitationFixture(t)
			s := redemptionStore(t, fx)
			a := redemptionActor(t, fx)
			v, code := redeemCode(t, fx, "DRIVER")
			p, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			if e != nil {
				t.Fatal(e)
			}
			table := map[string]string{"receipt": "org_invitation_person_commands", "notice": "org_invitation_acceptance_notices", "audit": "org_audit_events"}[producer]
			trigger := "synthetic_redemption_late_failure"
			sql := "CREATE FUNCTION " + trigger + "()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic failure';END $$;CREATE CONSTRAINT TRIGGER " + trigger + " AFTER INSERT ON " + pgx.Identifier{table}.Sanitize() + " DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION " + trigger + "()"
			if _, e = fx.f.Admin.Exec(t.Context(), sql); e != nil {
				t.Fatal(e)
			}
			c := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest}
			out, e := s.Redeem(t.Context(), a, c)
			if e == nil || out.MembershipID != "" {
				t.Fatal("provisional acceptance", e)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='INVITED' AND account_id IS NULL`, 1, v.MembershipID)
			invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, 0, v.MembershipID)
			invitationCount(t, fx, `SELECT count(*) FROM org_invitation_person_commands WHERE invitation_id=$1`, 0, v.ID)
			if _, e = fx.f.Admin.Exec(t.Context(), "DROP TRIGGER "+trigger+" ON "+pgx.Identifier{table}.Sanitize()); e != nil {
				t.Fatal(e)
			}
			if _, e = s.Redeem(t.Context(), a, c); e != nil {
				t.Fatal("recovery", e)
			}
			if _, e = fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, a.SessionID); e != nil {
				t.Fatal(e)
			}
			if _, e = s.Redeem(t.Context(), a, c); !errors.Is(e, app.ErrEmployeeUnauthenticated) {
				t.Fatal("revoked replay", e)
			}
		})
	}
	t.Run("accepted_contact_change", func(t *testing.T) {
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
		if e = unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,work_email)VALUES($1,$2,'Synthetic recipient','changed@example.invalid')`, fx.scope.OrgID().String(), v.EmployeeID); e != nil {
			t.Fatal(e)
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND state='ENDED' AND cause='CONTACT_CHANGED' AND source_revision=1`, 1, v.ID)
		invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, v.MembershipID)
		old, e := s.Redeem(t.Context(), a, c)
		if e != nil || !old.Replayed || old.MembershipState != d.MembershipInvited {
			t.Fatal("original receipt after end", e)
		}
	})
}

func TestRedemptionConcurrentConfirmRejectAndContactReduction(t *testing.T) {
	for _, contact := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirm_reject", true: "confirm_contact"}[contact], func(t *testing.T) {
			fx := invitationFixture(t)
			s := redemptionStore(t, fx)
			actor := redemptionActor(t, fx)
			v, code := redeemCode(t, fx, "AUDITOR")
			p, e := s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest}); e != nil {
				t.Fatal(e)
			}
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make(chan error, 12)
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					if contact && i == 11 {
						results <- unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,work_email)VALUES($1,$2,'Synthetic current recipient','concurrent@example.invalid')`, fx.scope.OrgID().String(), v.EmployeeID)
						return
					}
					action := "CONFIRM"
					if !contact && i%2 == 1 {
						action = "REJECT"
					}
					_, _, e := s.ManageAdmission(t.Context(), fx.scope, invitationActor(fx), fx.member, v.ID, action, 1, onboardingID(t))
					results <- e
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			success := 0
			for e := range results {
				if e == nil {
					success++
				} else if !errors.Is(e, app.ErrInvitationConflict) {
					t.Fatal("unexpected confirmation race", e)
				}
			}
			if (!contact && success != 1) || (contact && (success < 1 || success > 2)) {
				t.Fatal("race winners", success)
			}
			var state, member string
			var roles int
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT a.state,m.state,(SELECT count(*) FROM org_role_assignments r WHERE r.org_id=m.org_id AND r.membership_id=m.id)::integer FROM org_invitation_acceptances a JOIN org_memberships m ON(m.org_id,m.id)=(a.org_id,a.membership_id) WHERE a.invitation_id=$1`, v.ID).Scan(&state, &member, &roles); e != nil {
				t.Fatal(e)
			}
			if state == "ACTIVE" {
				if member != "ACTIVE" || roles != 1 {
					t.Fatal("partial activation")
				}
			} else if member != "ENDED" || roles != 0 {
				t.Fatal("reduction granted roles", state, member, roles)
			}
		})
	}
}
func TestRedemptionReplacedAndExpiredSecretsAreUniform(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	a := redemptionActor(t, fx)
	v, code := redeemCode(t, fx, "DRIVER")
	if _, _, _, e := inviteWrite(t, fx, app.InvitationWrite{Action: app.InvitationResend, OperationID: onboardingID(t), InvitationID: v.ID, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code}); !errors.Is(e, app.ErrTokenInvalid) {
		t.Fatal("superseded", e)
	}
	expired, code2 := redeemCode(t, fx, "DRIVER")
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitations DISABLE TRIGGER org_invitation_guard;ALTER TABLE org_invitations DISABLE TRIGGER org_invitation_project`); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE org_invitations SET created_at=created_at-interval '31 days',expires_at=expires_at-interval '31 days',changed_at=changed_at-interval '31 days' WHERE id=$1`, expired.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitations ENABLE TRIGGER org_invitation_guard;ALTER TABLE org_invitations ENABLE TRIGGER org_invitation_project`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code2}); !errors.Is(e, app.ErrTokenInvalid) {
		t.Fatal("expired", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='EXPIRED'`, 1, expired.ID)
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, expired.MembershipID)
}

func TestRedemptionPersonalMembershipsStayIndependentAcrossOrganizations(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	a := redemptionActor(t, fx)
	first, code := redeemCode(t, fx, "DRIVER")
	preview, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}); e != nil {
		t.Fatal(e)
	}
	next := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	other := invitationFixtureState{legalFixtureState: next, invites: fx.invites}
	second, code2 := redeemCode(t, other, "DRIVER")
	p, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "PREVIEW", Code: code2})
	if e != nil {
		t.Fatal(e)
	}
	accepted, e := s.Redeem(t.Context(), a, app.RedemptionCommand{Action: "ACCEPT", Code: code2, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest})
	if e != nil || accepted.OrgID != other.scope.OrgID().String() {
		t.Fatal("independent account admission", e)
	}
	if first.MembershipID == second.MembershipID {
		t.Fatal("membership period shared across organizations")
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE account_id=$1 AND state='ACTIVE'`, 2, a.AccountID)
	if _, _, e := s.ManageAdmission(t.Context(), fx.scope, invitationActor(fx), fx.member, second.ID, "READ", 0, ""); !errors.Is(e, app.ErrEmployeeNotFound) {
		t.Fatal("foreign acceptance disclosed", e)
	}
}
