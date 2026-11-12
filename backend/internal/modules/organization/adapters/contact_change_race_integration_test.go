//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
)

func TestContactChangeConcurrentRedemptionResendAndConfirmation(t *testing.T) {
	for _, action := range []string{"redeem", "resend", "confirm"} {
		t.Run(action, func(t *testing.T) {
			fx := invitationFixture(t)
			s := contactEmployees(t, fx)
			id := contactSeed(t, fx, s)
			role := "DRIVER"
			if action == "confirm" {
				role = "AUDITOR"
			}
			invitation, code := contactInvite(t, fx, id, role)
			person, r := redemptionActor(t, fx), redemptionStore(t, fx)
			preview, e := r.Redeem(t.Context(), person, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			if e != nil {
				t.Fatal(e)
			}
			accept := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}
			if action == "confirm" {
				if _, e := r.Redeem(t.Context(), person, accept); e != nil {
					t.Fatal(e)
				}
			}
			change := contactCommand(t, id, 1, "changed@example.invalid", "+5511999999999")
			resend := app.InvitationWrite{Action: app.InvitationResend, InvitationID: invitation.ID, ExpectedRevision: 1, OperationID: onboardingID(t)}
			plan, nextCode := invitationPlan(t, fx)
			operation := onboardingID(t)
			start, edited, competed := make(chan struct{}), make(chan error, 1), make(chan error, 1)
			go func() {
				<-start
				_, _, e := writeEmployee(t, fx.legalFixtureState, s, change)
				edited <- e
			}()
			go func() {
				<-start
				var e error
				switch action {
				case "redeem":
					_, e = r.Redeem(t.Context(), person, accept)
				case "resend":
					_, _, e = fx.invites.WriteInvitation(t.Context(), fx.scope, invitationActor(fx), fx.member, resend, plan)
				case "confirm":
					_, _, e = r.ManageAdmission(t.Context(), fx.scope, invitationActor(fx), fx.member, invitation.ID, "CONFIRM", 1, operation)
				}
				competed <- e
			}()
			close(start)
			if e := <-edited; e != nil {
				t.Fatal("contact race edit", e)
			}
			otherError := <-competed
			if otherError != nil && !errors.Is(otherError, app.ErrTokenInvalid) && !errors.Is(otherError, app.ErrInvitationConflict) {
				t.Fatal("contact race competitor", otherError)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE employee_id=$1 AND state='PENDING'`, 0, id)
			invitationCount(t, fx, `SELECT count(*) FROM org_employee_profiles WHERE employee_id=$1 AND revision=2 AND work_email='changed@example.invalid'`, 1, id)
			var state string
			var roles int
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state,(SELECT count(*) FROM org_role_assignments WHERE membership_id=$1) FROM org_memberships WHERE id=$1`, invitation.MembershipID).Scan(&state, &roles); e != nil {
				t.Fatal(e)
			}
			if state == "ACTIVE" {
				if action == "resend" || otherError != nil || roles != 1 {
					t.Fatal("contact race fabricated active access")
				}
			} else if state != "ENDED" || roles != 0 || action != "resend" && otherError == nil {
				t.Fatal("contact race left partial admission")
			}
			if action == "resend" {
				secrets := []string{code}
				if otherError == nil {
					secrets = append(secrets, nextCode)
				}
				for _, secret := range secrets {
					if _, e := r.Redeem(t.Context(), person, app.RedemptionCommand{Action: "PREVIEW", Code: secret}); !errors.Is(e, app.ErrTokenInvalid) {
						t.Fatal("resend race retained old-contact token", e)
					}
				}
			}
		})
	}
}
