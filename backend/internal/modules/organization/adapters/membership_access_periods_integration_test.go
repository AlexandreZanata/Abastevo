//go:build integration

package adapters_test

import (
	"context"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	orgd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestMembershipAccessPreservesExpiredAndFuturePeriods(t *testing.T) {
	fx := membershipAccessFixture(t)
	target := accessMember(t, fx)
	initial := accessRead(t, fx, target)
	expired, future := onboardingID(t), onboardingID(t)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason)VALUES($1,$2,$3,'AUDITOR','ORG',$1,clock_timestamp(),clock_timestamp()+interval '1 second',$4,'Synthetic finite role')`, fx.scope.OrgID().String(), expired, target, fx.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'DRIVER','SELF',$3,clock_timestamp()+interval '1 day',$4,'Synthetic future role')`, fx.scope.OrgID().String(), future, target, fx.member); e != nil {
		t.Fatal(e)
	}
	var before string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(r)ORDER BY id)::text FROM org_role_assignments r WHERE org_id=$1 AND membership_id=$2`, fx.scope.OrgID().String(), target).Scan(&before); e != nil {
		t.Fatal(e)
	}
	pause, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: initial.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic bounded pause"})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	elapsed := false
	for !elapsed {
		select {
		case <-ctx.Done():
			t.Fatal("period did not expire")
		case <-tick.C:
			if e := fx.f.Admin.QueryRow(ctx, `SELECT valid_to<=clock_timestamp() FROM org_role_assignments WHERE id=$1`, expired).Scan(&elapsed); e != nil {
				t.Fatal(e)
			}
		}
	}
	if _, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipReactivate, MembershipID: target, ExpectedRevision: pause.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic bounded resume"}); e != nil {
		t.Fatal(e)
	}
	rows, e := accesspg.NewStore(fx.f.Runtime).Candidates(t.Context(), fx.scope, target)
	if e != nil || len(rows) != 1 {
		t.Fatal("expired/future authority restored", len(rows), e)
	}
	var after string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(r)ORDER BY id)::text FROM org_role_assignments r WHERE org_id=$1 AND membership_id=$2`, fx.scope.OrgID().String(), target).Scan(&after); e != nil || before != after {
		t.Fatal("period history changed", e)
	}
}
func TestMembershipAccessNoRoleOrInactiveEmployeeCannotReactivate(t *testing.T) {
	for _, cause := range []string{"no_role", "employee"} {
		t.Run(cause, func(t *testing.T) {
			fx := membershipAccessFixture(t)
			target := accessMember(t, fx)
			v := accessRead(t, fx, target)
			pause, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: v.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic pause"})
			if e != nil {
				t.Fatal(e)
			}
			if cause == "no_role" {
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, fx.scope.OrgID().String(), target); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.State.EmployeeID); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipReactivate, MembershipID: target, ExpectedRevision: pause.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic invalid resume"}); !errors.Is(e, d.ErrConditions) {
				t.Fatal("invalid resume", e)
			}
			if unitState(t, fx, target) != "SUSPENDED" {
				t.Fatal("invalid resume changed state")
			}
			invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_membership_access_commands WHERE membership_id=$1`, 1, target)
		})
	}
}
func TestMembershipAccessConsumesPriorSourcesWithoutResurrectingThem(t *testing.T) {
	fx := membershipAccessFixture(t)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()+";GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	target := accessMember(t, fx)
	before := accessRead(t, fx, target)
	issuer := fx.transfer.actors[target]
	plan, _ := invitationPlan(t, fx.invitationFixtureState)
	employee := invitationEmployee(t, fx.invitationFixtureState)
	invitation, _, e := fx.invites.WriteInvitation(t.Context(), fx.scope, app.EmployeeActor{AccountID: issuer.AccountID, SessionID: issuer.SessionID}, target, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}, plan)
	if e != nil {
		t.Fatal(e)
	}
	inv, _ := invitation.Snapshot(fx.scope)
	roles := accesspg.NewStore(fx.f.Runtime)
	catalog, e := audit.NewCatalog(orgpg.OwnershipTransferAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	transfer := orgpg.NewOwnershipTransferStore(fx.f.Runtime, roles, auditpg.NewStore(fx.f.Runtime, catalog))
	transfer.Memberships = fx.store.Memberships
	transfer.Capabilities = fx.flags
	proposal, _, e := transfer.Propose(t.Context(), fx.scope, fx.actor, fx.member, orgd.OwnershipTransfer{OrgID: fx.scope.OrgID().String(), TransferID: onboardingID(t), InitiatorMembershipID: fx.member, TargetMembershipID: target, OperationID: onboardingID(t)})
	if e != nil {
		t.Fatal(e)
	}
	settingsCatalog, e := audit.NewCatalog(orgpg.SettingsAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	settings := &orgpg.SettingsStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Audit: auditpg.NewStore(fx.f.Runtime, settingsCatalog)}
	read, e := settings.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if e != nil {
		t.Fatal(e)
	}
	sv, e := read.Settings.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	values := sv.Values
	values.Responsibilities.PlatformNotices.PrimaryID = target
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, sv.Revision, values, "")); e != nil {
		t.Fatal(e)
	}
	pause, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipSuspend, MembershipID: target, ExpectedRevision: before.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic source pause"})
	if e != nil {
		t.Fatal(e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='REVOKED' AND source_id=$2 AND source_revision=$3`, 1, inv.ID, target, pause.State.Revision)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_ownership_transfers WHERE transfer_id=$1 AND state='CANCELLED'`, 1, proposal.TransferID)
	if _, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipReactivate, MembershipID: target, ExpectedRevision: pause.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic source resume"}); e != nil {
		t.Fatal(e)
	}
	read, e = settings.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if e != nil {
		t.Fatal(e)
	}
	sv, e = read.Settings.Snapshot(fx.scope)
	if e != nil || sv.Values.Responsibilities.PlatformNotices.PrimaryID != "" {
		t.Fatal("designation resurrected", e)
	}
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='REVOKED'`, 1, inv.ID)
	invitationCount(t, fx.invitationFixtureState, `SELECT count(*) FROM org_ownership_transfers WHERE transfer_id=$1 AND state='CANCELLED'`, 1, proposal.TransferID)
	// Ending a period still uses its old rules; the new path cannot revive it.
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, e := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), target)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	terminal := accessRead(t, fx, target)
	if _, e := accessWrite(t, fx, app.MembershipAccessWrite{Action: app.MembershipReactivate, MembershipID: target, ExpectedRevision: terminal.State.Revision, OperationID: onboardingID(t), Reason: "Synthetic ended resume"}); !errors.Is(e, d.ErrTransition) {
		t.Fatal("ended period reopened", e)
	}
}
