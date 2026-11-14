//go:build integration

package adapters_test

import (
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

// ageAdmission moves one WAITING admission past its 168-hour deadline in this
// isolated database only; runtime still observes the database clock.
func ageAdmission(t *testing.T, fx invitationFixtureState, invitation string) {
	t.Helper()
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitation_acceptances DISABLE TRIGGER org_invitation_acceptance_guard;ALTER TABLE org_invitation_acceptances DISABLE TRIGGER org_invitation_acceptance_project`); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE org_invitation_acceptances SET accepted_at=accepted_at-interval '169 hours',deadline=deadline-interval '169 hours',changed_at=changed_at-interval '169 hours' WHERE invitation_id=$1 AND state='WAITING'`, invitation); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitation_acceptances ENABLE TRIGGER org_invitation_acceptance_guard;ALTER TABLE org_invitation_acceptances ENABLE TRIGGER org_invitation_acceptance_project`); e != nil {
		t.Fatal(e)
	}
}

func expiredAdmission(t *testing.T) (invitationFixtureState, d.InvitationSnapshot, app.EmployeeActor) {
	t.Helper()
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	person := redemptionActor(t, fx)
	v := invitationAccept(t, fx, s, person, "AUDITOR")
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='INVITED' AND account_id=$2`, 1, v.MembershipID, person.AccountID)
	ageAdmission(t, fx, v.ID)
	return fx, v, person
}

func assertAdmissionExpired(t *testing.T, fx invitationFixtureState, v d.InvitationSnapshot) {
	t.Helper()
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND state='EXPIRED' AND cause='CONFIRMATION_EXPIRED' AND revision=2`, 1, v.ID)
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED' AND end_reason='ADMISSION_ENDED'`, 1, v.MembershipID)
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptance_notices WHERE invitation_id=$1 AND state='EXPIRED'`, 1, v.ID)
}

// B-BR-EMP-174: an admission whose confirmation deadline passed is materialized
// as EXPIRED wherever its INVITED reservation would block a manager or person:
// re-issue for the same employee, the invitation list and a new acceptance.
func TestExpiredAdmissionNoLongerBlocksReissueListOrNewAcceptance(t *testing.T) {
	t.Run("reissue", func(t *testing.T) {
		fx, v, _ := expiredAdmission(t)
		issued, _, _, e := inviteWrite(t, fx, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: v.EmployeeID, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}})
		if e != nil {
			t.Fatal("expired admission blocked re-invitation", e)
		}
		assertAdmissionExpired(t, fx, v)
		next, e := issued.Snapshot(fx.scope)
		if e != nil || next.MembershipID == v.MembershipID || next.State != d.InvitationPending {
			t.Fatal("re-invitation reused the expired admission period", e)
		}
	})
	t.Run("list", func(t *testing.T) {
		fx, v, _ := expiredAdmission(t)
		if _, e := fx.invites.ListInvitations(t.Context(), fx.scope, invitationActor(fx), fx.member, app.InvitationFilter{Limit: 10}); e != nil {
			t.Fatal(e)
		}
		assertAdmissionExpired(t, fx, v)
	})
	t.Run("accept", func(t *testing.T) {
		fx, v, person := expiredAdmission(t)
		s := redemptionStore(t, fx)
		accepted := invitationAccept(t, fx, s, person, "DRIVER")
		assertAdmissionExpired(t, fx, v)
		invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ACTIVE' AND account_id=$2`, 1, accepted.MembershipID, person.AccountID)
	})
	t.Run("not yet due", func(t *testing.T) {
		fx := invitationFixture(t)
		s := redemptionStore(t, fx)
		person := redemptionActor(t, fx)
		v := invitationAccept(t, fx, s, person, "AUDITOR")
		if _, e := fx.invites.ListInvitations(t.Context(), fx.scope, invitationActor(fx), fx.member, app.InvitationFilter{Limit: 10}); e != nil {
			t.Fatal(e)
		}
		if _, _, _, e := inviteWrite(t, fx, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: v.EmployeeID, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}); e == nil {
			t.Fatal("live admission replaced before its deadline")
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND state='WAITING' AND revision=1`, 1, v.ID)
		invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='INVITED' AND account_id=$2`, 1, v.MembershipID, person.AccountID)
	})
}
