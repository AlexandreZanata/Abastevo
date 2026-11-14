//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

func contactEmployees(t *testing.T, fx invitationFixtureState) *memberpg.EmployeeStore {
	t.Helper()
	catalog, e := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &orgpg.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}

func contactSeed(t *testing.T, fx invitationFixtureState, employees *memberpg.EmployeeStore) string {
	t.Helper()
	v, _, e := writeEmployee(t, fx.legalFixtureState, employees, employeeCommand(t, app.EmployeeWriteInput{Kind: app.CreateEmployee, Profile: d.EmployeeProfile{DisplayName: "Synthetic contact recipient", WorkEmail: "old@example.invalid", WorkPhone: "+5511999999999"}}))
	if e != nil {
		t.Fatal(e)
	}
	return v.EmployeeID
}

func contactInvite(t *testing.T, fx invitationFixtureState, id, role string) (d.InvitationSnapshot, string) {
	t.Helper()
	v, _, code, e := inviteWrite(t, fx, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: id, Grants: []d.InvitationGrant{{Role: role, Scope: "SELF"}}})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := v.Snapshot(fx.scope)
	if e != nil || code == "" {
		t.Fatal("invitation", e)
	}
	return snap, code
}

func contactCommand(t *testing.T, id string, revision int64, email, phone string) app.EmployeeWrite {
	t.Helper()
	return employeeCommand(t, app.EmployeeWriteInput{Kind: app.ChangeEmployee, EmployeeID: id, ExpectedRevision: revision, Profile: d.EmployeeProfile{DisplayName: "Synthetic contact recipient", WorkEmail: email, WorkPhone: phone}})
}

// Compare complete source history rather than merely checking one pointer.
func contactSourceSnapshot(t *testing.T, fx invitationFixtureState) string {
	t.Helper()
	var value string
	e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY employee_id) FROM org_employee_profiles x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_employee_identities x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_role_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_invitations x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY invitation_id) FROM org_invitation_acceptances x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY invitation_id,revision) FROM org_invitation_notices x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY invitation_id,revision) FROM org_invitation_acceptance_notices x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id) FROM org_employee_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id) FROM org_audit_events x))::text`).Scan(&value)
	if e != nil {
		t.Fatal(e)
	}
	return value
}

func TestContactChangeFreshProofAfterProjectionWait(t *testing.T) {
	contactProofWait(t, false)
}

func TestContactChangeFreshProofAfterReceiptWait(t *testing.T) {
	contactProofWait(t, true)
}

func contactProofWait(t *testing.T, receipt bool) {
	t.Helper()
	fx := invitationFixture(t)
	s := contactEmployees(t, fx)
	id := contactSeed(t, fx, s)
	contactInvite(t, fx, id, "DRIVER")
	c := contactCommand(t, id, 1, "changed@example.invalid", "+5511999999999")
	if receipt {
		if _, _, e := writeEmployee(t, fx.legalFixtureState, s, c); e != nil {
			t.Fatal(e)
		}
		contactInvite(t, fx, id, "DRIVER")
	}
	before := contactSourceSnapshot(t, fx)
	if !receipt {
		if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_contact_wait() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(921013);RETURN NEW;END$$;CREATE TRIGGER zz_synthetic_contact_wait AFTER UPDATE ON org_employee_profiles FOR EACH ROW EXECUTE FUNCTION synthetic_contact_wait()`); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	holder, e := fx.f.Admin.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer holder.Rollback(context.Background())
	lock, query := `SELECT pg_advisory_xact_lock(921013)`, "%name: ChangeEmployeeProfile%"
	if receipt {
		lock, query = `LOCK TABLE org_employee_commands IN ACCESS EXCLUSIVE MODE`, "%name: EmployeeCommandReceipt%"
	}
	if _, e := holder.Exec(ctx, lock); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.f.Admin.Exec(ctx, `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',created_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, fx.actor.SessionID); e != nil {
		t.Fatal(e)
	}
	type result struct {
		value  app.EmployeeResult
		replay bool
		err    error
	}
	done := make(chan result, 1)
	go func() {
		v, replay, e := s.WriteEmployee(ctx, fx.scope, employeeActor(fx.legalFixtureState), fx.member, "", c)
		done <- result{v, replay, e}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waited, expired := false, false
	for !waited || !expired {
		select {
		case <-ctx.Done():
			t.Fatal("contact projection wait/clock boundary not observed")
		case r := <-done:
			t.Fatal("contact write completed before projection release", r.err)
		case <-ticker.C:
			if e := fx.f.Admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND query LIKE $2)`, fx.f.RuntimeRole, query).Scan(&waited); e != nil {
				t.Fatal(e)
			}
			if e := fx.f.Admin.QueryRow(ctx, `SELECT authenticated_at<=clock_timestamp()-interval '15 minutes' FROM company_web_sessions WHERE id=$1`, fx.actor.SessionID).Scan(&expired); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := holder.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case r := <-done:
		if !errors.Is(r.err, app.ErrEmployeeUnauthenticated) || r.replay || r.value != (app.EmployeeResult{}) {
			t.Fatal("expired source proof acknowledged contact change", r.err)
		}
	case <-ctx.Done():
		t.Fatal("contact write did not finish")
	}
	if contactSourceSnapshot(t, fx) != before {
		t.Fatal("expired proof left profile/admission/audit history")
	}
}

func contactAccessSnapshot(t *testing.T, fx invitationFixtureState) string {
	t.Helper()
	var out string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM account_session_families x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM accounts x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM org_memberships x WHERE state='ACTIVE' OR org_id<>$1),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x))::text`, fx.scope.OrgID().String()).Scan(&out); e != nil {
		t.Fatal(e)
	}
	return out
}

func TestContactChangePendingEquivalentClearingAndOriginalReplay(t *testing.T) {
	for _, kind := range []string{"email", "phone", "clear", "canonical", "metadata", "no_admission"} {
		t.Run(kind, func(t *testing.T) {
			fx := invitationFixture(t)
			s := contactEmployees(t, fx)
			id := contactSeed(t, fx, s)
			var invitation d.InvitationSnapshot
			code := ""
			if kind != "no_admission" {
				invitation, code = contactInvite(t, fx, id, "DRIVER")
			}
			input, _ := contactCommand(t, id, 1, "old@example.invalid", "+5511999999999").Snapshot()
			switch kind {
			case "email", "no_admission":
				input.Profile.WorkEmail = "new@example.invalid"
			case "phone":
				input.Profile.WorkPhone = "+5511888888888"
			case "clear":
				input.Profile.WorkEmail, input.Profile.WorkPhone = "", ""
			case "canonical":
				input.Profile.WorkEmail, input.Profile.WorkPhone = " OLD@example.invalid ", " +5511999999999 "
			case "metadata":
				input.Profile.JobTitle = "Synthetic description"
			}
			c := employeeCommand(t, input)
			access := contactAccessSnapshot(t, fx)
			out, replay, e := writeEmployee(t, fx.legalFixtureState, s, c)
			if e != nil || replay || out.ProfileRevision != 2 || out.IdentityRevision != 1 {
				t.Fatal("contact replacement", e)
			}
			if contactAccessSnapshot(t, fx) != access {
				t.Fatal("contact changed personal or active access")
			}
			if kind == "no_admission" {
				invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE employee_id=$1`, 0, id)
				return
			}
			changed := kind != "canonical" && kind != "metadata"
			if !changed {
				invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='PENDING' AND revision=1`, 1, invitation.ID)
				invitationCount(t, fx, `SELECT count(*) FROM org_invitation_notices WHERE invitation_id=$1 AND cause='CONTACT_CHANGED'`, 0, invitation.ID)
				return
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='REVOKED' AND cause='CONTACT_CHANGED' AND source_type='PROFILE' AND source_id=employee_id AND source_revision=2`, 1, invitation.ID)
			invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, invitation.MembershipID)
			invitationCount(t, fx, `SELECT count(*) FROM org_invitation_notices WHERE invitation_id=$1 AND revision=2 AND cause='CONTACT_CHANGED' AND source_revision=2`, 1, invitation.ID)
			r := redemptionStore(t, fx)
			person := redemptionActor(t, fx)
			if value, e := r.Redeem(t.Context(), person, app.RedemptionCommand{Action: "PREVIEW", Code: code}); !errors.Is(e, app.ErrTokenInvalid) || value.OrgID != "" {
				t.Fatal("old contact token usable", e)
			}
			next, _ := contactInvite(t, fx, id, "DRIVER")
			if next.MembershipID == invitation.MembershipID {
				t.Fatal("contact reissue reused ended reservation")
			}
			if again, replay, e := writeEmployee(t, fx.legalFixtureState, s, c); e != nil || !replay || again != out {
				t.Fatal("original acknowledgement", e)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='PENDING' AND revision=1`, 1, next.ID)
			input.Profile.WorkEmail = "different@example.invalid"
			if _, _, e := writeEmployee(t, fx.legalFixtureState, s, employeeCommand(t, input)); !errors.Is(e, app.ErrEmployeeOperationReused) {
				t.Fatal("changed retry intent", e)
			}
		})
	}
}

func TestContactChangeWaitingAndActiveAccessRemainIndependent(t *testing.T) {
	for _, role := range []string{"AUDITOR", "DRIVER"} {
		t.Run(role, func(t *testing.T) {
			fx := invitationFixture(t)
			s := contactEmployees(t, fx)
			id := contactSeed(t, fx, s)
			invitation, code := contactInvite(t, fx, id, role)
			person, r := redemptionActor(t, fx), redemptionStore(t, fx)
			preview, e := r.Redeem(t.Context(), person, app.RedemptionCommand{Action: "PREVIEW", Code: code})
			if e != nil {
				t.Fatal(e)
			}
			accept := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}
			original, e := r.Redeem(t.Context(), person, accept)
			if e != nil {
				t.Fatal(e)
			}
			other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
			teamDriver(t, other, person.AccountID)
			access := contactAccessSnapshot(t, fx)
			if _, _, e := writeEmployee(t, fx.legalFixtureState, s, contactCommand(t, id, 1, "changed@example.invalid", "+5511999999999")); e != nil {
				t.Fatal(e)
			}
			if contactAccessSnapshot(t, fx) != access {
				t.Fatal("contact replacement crossed active or personal boundary")
			}
			if role == "AUDITOR" {
				invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND state='ENDED' AND cause='CONTACT_CHANGED' AND source_type='PROFILE' AND source_revision=2`, 1, invitation.ID)
				invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, invitation.MembershipID)
				invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, 0, invitation.MembershipID)
				if _, _, e := r.ManageAdmission(t.Context(), fx.scope, invitationActor(fx), fx.member, invitation.ID, "CONFIRM", 1, onboardingID(t)); !errors.Is(e, app.ErrInvitationConflict) {
					t.Fatal("ended waiting admission confirmed", e)
				}
			} else {
				invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ACTIVE'`, 1, invitation.MembershipID)
			}
			again, e := r.Redeem(t.Context(), person, accept)
			if e != nil || !again.Replayed || again.MembershipState != original.MembershipState || !again.AcceptedAt.Equal(original.AcceptedAt) {
				t.Fatal("contact changed original acceptance receipt", e)
			}
		})
	}
}

func TestContactChangeTwelveWayRetryAndDistinctCAS(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_operation", false: "distinct_operations"}[same], func(t *testing.T) {
			fx := invitationFixture(t)
			s := contactEmployees(t, fx)
			id := contactSeed(t, fx, s)
			invitation, _ := contactInvite(t, fx, id, "DRIVER")
			base, _ := contactCommand(t, id, 1, "changed@example.invalid", "+5511999999999").Snapshot()
			commands := make([]app.EmployeeWrite, 12)
			for i := range commands {
				input := base
				if !same {
					input.OperationID = onboardingID(t)
				}
				commands[i] = employeeCommand(t, input)
			}
			errs, replays := make([]error, 12), make([]bool, 12)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range commands {
				wg.Go(func() { <-start; _, replays[i], errs[i] = writeEmployee(t, fx.legalFixtureState, s, commands[i]) })
			}
			close(start)
			wg.Wait()
			wins, replayed := 0, 0
			for i, e := range errs {
				if e == nil {
					wins++
					if replays[i] {
						replayed++
					}
				} else if same || !errors.Is(e, d.ErrConflict) {
					t.Fatal("contact contention", e)
				}
			}
			if same && (wins != 12 || replayed != 11) || !same && (wins != 1 || replayed != 0) {
				t.Fatal("contact CAS effects", wins, replayed)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_employee_profiles WHERE employee_id=$1 AND revision=2`, 1, id)
			invitationCount(t, fx, `SELECT count(*) FROM org_invitation_notices WHERE invitation_id=$1 AND cause='CONTACT_CHANGED'`, 1, invitation.ID)
		})
	}
}

func TestContactChangeLateFailuresRollbackAllSourcesAndRecover(t *testing.T) {
	for _, table := range []string{"org_audit_events", "org_employee_commands", "org_invitation_notices", "org_invitation_acceptance_notices"} {
		t.Run(table, func(t *testing.T) {
			fx := invitationFixture(t)
			s := contactEmployees(t, fx)
			id := contactSeed(t, fx, s)
			role := "DRIVER"
			if table == "org_invitation_acceptance_notices" {
				role = "AUDITOR"
			}
			_, code := contactInvite(t, fx, id, role)
			if role == "AUDITOR" {
				person, r := redemptionActor(t, fx), redemptionStore(t, fx)
				preview, e := r.Redeem(t.Context(), person, app.RedemptionCommand{Action: "PREVIEW", Code: code})
				if e != nil {
					t.Fatal(e)
				}
				if _, e := r.Redeem(t.Context(), person, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}); e != nil {
					t.Fatal(e)
				}
			}
			before := contactSourceSnapshot(t, fx)
			// Fixed table names above; no user input reaches the trigger DDL.
			if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_contact_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic deferred contact failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_contact_failure AFTER INSERT ON `+table+` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_contact_failure()`); e != nil {
				t.Fatal(e)
			}
			c := contactCommand(t, id, 1, "changed@example.invalid", "+5511999999999")
			if out, replay, e := writeEmployee(t, fx.legalFixtureState, s, c); !errors.Is(e, app.ErrRegisterUnavailable) || replay || out != (app.EmployeeResult{}) {
				t.Fatal("partial failed contact acknowledgement", e)
			}
			if contactSourceSnapshot(t, fx) != before {
				t.Fatal("failed contact changed profile, admission, notice or audit")
			}
			if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_contact_failure ON `+table); e != nil {
				t.Fatal(e)
			}
			if out, _, e := writeEmployee(t, fx.legalFixtureState, s, c); e != nil || out.ProfileRevision != 2 {
				t.Fatal("contact recovery", e)
			}
		})
	}
}

func TestContactChangeForeignNonManagerAndDisabledReplayKeepSources(t *testing.T) {
	fx := invitationFixture(t)
	s := contactEmployees(t, fx)
	id := contactSeed(t, fx, s)
	contactInvite(t, fx, id, "DRIVER")
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	foreign, _, e := writeEmployee(t, other, s, employeeCommand(t, app.EmployeeWriteInput{Kind: app.CreateEmployee, Profile: d.EmployeeProfile{DisplayName: "Synthetic foreign contact", WorkEmail: "foreign@example.invalid"}}))
	if e != nil {
		t.Fatal(e)
	}
	person := redemptionActor(t, fx)
	_, member := teamDriver(t, fx.legalFixtureState, person.AccountID)
	before := contactSourceSnapshot(t, fx)
	if _, _, e := writeEmployee(t, fx.legalFixtureState, s, contactCommand(t, foreign.EmployeeID, 1, "changed@example.invalid", "")); !errors.Is(e, app.ErrEmployeeNotFound) {
		t.Fatal("foreign contact write", e)
	}
	if _, _, e := s.WriteEmployee(t.Context(), fx.scope, person, member, "", contactCommand(t, id, 1, "changed@example.invalid", "")); e == nil {
		t.Fatal("non-manager changed contact")
	}
	if contactSourceSnapshot(t, fx) != before {
		t.Fatal("denied contact changed source history")
	}
	c := contactCommand(t, id, 1, "changed@example.invalid", "+5511999999999")
	if _, _, e := writeEmployee(t, fx.legalFixtureState, s, c); e != nil {
		t.Fatal(e)
	}
	contactInvite(t, fx, id, "DRIVER")
	if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	// Capability disable has its own real audit event. Compare from that
	// committed boundary to isolate the denied employee commands' effects.
	before = contactSourceSnapshot(t, fx)
	for _, command := range []app.EmployeeWrite{c, contactCommand(t, id, 2, "another@example.invalid", "+5511999999999")} {
		if out, replay, e := writeEmployee(t, fx.legalFixtureState, s, command); !errors.Is(e, access.ErrEntitlementRequired) || replay || out != (app.EmployeeResult{}) {
			t.Fatal("disabled contact or replay", e)
		}
	}
	if contactSourceSnapshot(t, fx) != before {
		t.Fatal("disabled contact changed source history")
	}
}
