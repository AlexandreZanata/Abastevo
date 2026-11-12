//go:build integration

package adapters_test

import (
	"errors"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"testing"
)

func TestInvitationServerExpiryFailureRecoveryAndNewPeriod(t *testing.T) {
	fx := invitationFixture(t)
	employee := invitationEmployee(t, fx)
	pending := invitationIssue(t, fx, employee)
	v, _ := pending.Snapshot(fx.scope)
	// Isolated fixture simulates elapsed wall time without waiting days. Only
	// these two INSERT/UPDATE guards are suspended by fixture admin; the runtime
	// API has no clock input and no permission to disable either trigger.
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitations DISABLE TRIGGER org_invitation_guard;ALTER TABLE org_invitations DISABLE TRIGGER org_invitation_project`); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE org_invitations SET created_at=created_at-interval '31 days',expires_at=expires_at-interval '31 days',changed_at=changed_at-interval '31 days' WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_invitations ENABLE TRIGGER org_invitation_guard;ALTER TABLE org_invitations ENABLE TRIGGER org_invitation_project;CREATE FUNCTION synthetic_invitation_expiry_outage()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic expiry notice failure';END $$;CREATE CONSTRAINT TRIGGER synthetic_invitation_expiry_outage AFTER INSERT ON org_invitation_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='EXPIRED') EXECUTE FUNCTION synthetic_invitation_expiry_outage()`); e != nil {
		t.Fatal(e)
	}
	if _, e := invitationRead(t, fx, v.ID); !errors.Is(e, members.ErrRegisterUnavailable) {
		t.Fatal("expiry commit failure acknowledged", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='PENDING'`, 1, v.ID)
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='INVITED'`, 1, v.MembershipID)
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_invitation_expiry_outage ON org_invitation_notices`); e != nil {
		t.Fatal(e)
	}
	expired, e := invitationRead(t, fx, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	ev, _ := expired.Snapshot(fx.scope)
	if ev.State != d.InvitationExpired || ev.Cause != "EXPIRED" {
		t.Fatal("server expiry not materialized")
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED' AND end_reason='ADMISSION_ENDED'`, 1, v.MembershipID)
	fresh := invitationIssue(t, fx, employee)
	nv, _ := fresh.Snapshot(fx.scope)
	if nv.MembershipID == v.MembershipID {
		t.Fatal("terminal membership resurrected")
	}
}
