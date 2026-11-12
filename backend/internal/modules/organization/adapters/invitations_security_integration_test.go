//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"reflect"
	"sync"
	"testing"
	"time"
)

func invitationIssue(t *testing.T, fx invitationFixtureState, employee string) d.Invitation {
	t.Helper()
	v, r, _, e := inviteWrite(t, fx, members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}})
	if e != nil || r {
		t.Fatal("issue", e)
	}
	return v
}
func invitationActor(fx invitationFixtureState) members.EmployeeActor {
	return members.EmployeeActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}
func invitationRead(t *testing.T, fx invitationFixtureState, id string) (d.Invitation, error) {
	t.Helper()
	page, e := fx.invites.ListInvitations(t.Context(), fx.scope, invitationActor(fx), fx.member, members.InvitationFilter{ID: id, Limit: 1})
	if e != nil {
		return d.Invitation{}, e
	}
	return page.Invitations[0], nil
}
func invitationCount(t *testing.T, fx invitationFixtureState, sql string, want int, args ...any) {
	t.Helper()
	var count int
	if e := fx.f.Admin.QueryRow(t.Context(), sql, args...).Scan(&count); e != nil || count != want {
		t.Fatal("invitation source count", count, want, e)
	}
}
func TestInvitationSourceAuthorizationIsolationAndRevocation(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      error
	}{
		{"revoked cookie", `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, members.ErrEmployeeUnauthenticated},
		{"revoked family", `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, members.ErrEmployeeUnauthenticated},
		{"stale proof", `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, members.ErrEmployeeUnauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := invitationFixture(t)
			employee := invitationEmployee(t, fx)
			c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
			first, _, _, e := inviteWrite(t, fx, c)
			if e != nil {
				t.Fatal(e)
			}
			v, _ := first.Snapshot(fx.scope)
			if _, e := fx.f.Admin.Exec(t.Context(), tc.sql, fx.actor.AccountID); e != nil {
				t.Fatal(e)
			}
			if got, r, code, e := inviteWrite(t, fx, c); !errors.Is(e, tc.want) || r || code != "" || !reflect.DeepEqual(got, d.Invitation{}) {
				t.Fatal("revoked retry", e)
			}
			if got, e := invitationRead(t, fx, v.ID); !errors.Is(e, tc.want) || !reflect.DeepEqual(got, d.Invitation{}) {
				t.Fatal("revoked read", e)
			}
			if _, _, _, e := inviteWrite(t, fx, members.InvitationWrite{Action: members.InvitationRevoke, OperationID: onboardingID(t), InvitationID: v.ID, ExpectedRevision: 1}); !errors.Is(e, tc.want) {
				t.Fatal("revoked reduction", e)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_invitation_commands`, 1)
		})
	}
	t.Run("foreign employee and operation reuse", func(t *testing.T) {
		fx := invitationFixture(t)
		c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: onboardingID(t), Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
		if _, _, _, e := inviteWrite(t, fx, c); !errors.Is(e, members.ErrEmployeeNotFound) {
			t.Fatal("foreign employee", e)
		}
		c.EmployeeID = invitationEmployee(t, fx)
		first, _, _, e := inviteWrite(t, fx, c)
		if e != nil {
			t.Fatal(e)
		}
		v, _ := first.Snapshot(fx.scope)
		c.EmployeeID = invitationEmployee(t, fx)
		if _, _, _, e := inviteWrite(t, fx, c); !errors.Is(e, members.ErrInvitationOperationReused) {
			t.Fatal("altered retry", e)
		}
		other, _, e := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
		if e != nil {
			t.Fatal(e)
		}
		if _, e := fx.invites.ListInvitations(t.Context(), scopeFor(t, other.OrgID.String()), invitationActor(fx), other.MembershipID, members.InvitationFilter{ID: v.ID, Limit: 1}); !errors.Is(e, members.ErrEmployeeNotFound) {
			t.Fatal("cross-org read", e)
		}
	})
	t.Run("literal subset and self invitation", func(t *testing.T) {
		fx := invitationFixture(t)
		employee := invitationEmployee(t, fx)
		c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []d.InvitationGrant{{Role: "FINANCE", Scope: "ORG"}}}
		var denied *permissions.PermissionDenial
		if _, _, _, e := inviteWrite(t, fx, c); !errors.As(e, &denied) {
			t.Fatal("owner wildcard escalation", e)
		}
		var ownEmployee string
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member).Scan(&ownEmployee); e != nil {
			t.Fatal(e)
		}
		c.EmployeeID = ownEmployee
		c.Grants = []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}
		if _, _, _, e := inviteWrite(t, fx, c); !errors.As(e, &denied) {
			t.Fatal("self invite", e)
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitations`, 0)
	})
	t.Run("disabled capability reductions and archive", func(t *testing.T) {
		fx := invitationFixture(t)
		first := invitationIssue(t, fx, invitationEmployee(t, fx))
		v, _ := first.Snapshot(fx.scope)
		if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
			t.Fatal(e)
		}
		if _, _, _, e := inviteWrite(t, fx, members.InvitationWrite{Action: members.InvitationResend, OperationID: onboardingID(t), InvitationID: v.ID, ExpectedRevision: 1}); !errors.Is(e, access.ErrEntitlementRequired) {
			t.Fatal("disabled resend", e)
		}
		if _, e := invitationRead(t, fx, v.ID); e != nil {
			t.Fatal("read disabled", e)
		}
		if _, _, _, e := inviteWrite(t, fx, members.InvitationWrite{Action: members.InvitationRevoke, OperationID: onboardingID(t), InvitationID: v.ID, ExpectedRevision: 1}); e != nil {
			t.Fatal("reduction disabled", e)
		}
		if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
			t.Fatal(e)
		}
		if _, e := invitationRead(t, fx, v.ID); e != nil {
			t.Fatal("archived authorized read", e)
		}
	})
}
func TestInvitationContextReductionRetainsActualSource(t *testing.T) {
	for _, kind := range []string{"contact", "contact completion", "employee", "issuer", "organization"} {
		t.Run(kind, func(t *testing.T) {
			fx := invitationFixture(t)
			employee := invitationEmployee(t, fx)
			if kind == "contact" {
				if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,work_email)VALUES($1,$2,'Synthetic recipient','old@example.invalid')`, fx.scope.OrgID().String(), employee); e != nil {
					t.Fatal(e)
				}
			}
			v, _ := invitationIssue(t, fx, employee).Snapshot(fx.scope)
			var e error
			switch kind {
			case "contact":
				e = unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_employee_profiles SET work_email='new@example.invalid',revision=revision+1 WHERE org_id=$1 AND employee_id=$2`, fx.scope.OrgID().String(), employee)
			case "contact completion":
				e = unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_employee_profiles(org_id,employee_id,display_name,work_email)VALUES($1,$2,'Synthetic recipient','first@example.invalid')`, fx.scope.OrgID().String(), employee)

			case "employee":
				e = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
					if _, e := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='ADMISSION_ENDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.MembershipID); e != nil {
						return e
					}
					_, e := tx.Exec(t.Context(), `UPDATE org_employee_identities SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), employee)
					return e
				})
			case "issuer":
				tf := transferFixtureState{legalFixtureState: fx.legalFixtureState, actors: map[string]orgapp.CompanyActor{}}
				backup, _ := createTransferMember(t, tf, false)
				if e = unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic surviving owner')`, fx.scope.OrgID().String(), onboardingID(t), backup, fx.member); e == nil {
					e = unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), fx.member)
				}
			case "organization":
				e = unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String())
			}
			if e != nil {
				t.Fatal("source reduction", e)
			}
			var state, cause, source string
			var revision int64
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state,cause,source_type,source_revision FROM org_invitations WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.ID).Scan(&state, &cause, &source, &revision); e != nil || state != "REVOKED" || cause == "" || source == "" || revision < 1 {
				t.Fatal("source projection", state, cause, source, revision, e)
			}
			if kind == "contact completion" && (source != "PROFILE" || revision != 1) {
				t.Fatal("initial contact source invented")
			}
			if kind == "contact" && (source != "PROFILE" || revision != 2) {
				t.Fatal("contact revision not retained")
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE org_id=$1 AND id=$2 AND state='ENDED'`, 1, fx.scope.OrgID().String(), v.MembershipID)
			invitationCount(t, fx, `SELECT count(*) FROM org_invitation_notices WHERE org_id=$1 AND invitation_id=$2`, 2, fx.scope.OrgID().String(), v.ID)
		})
	}
}
func TestInvitationConcurrentReplaceAndRetries(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprintf("same-%v", same), func(t *testing.T) {
			fx := invitationFixture(t)
			initial := invitationIssue(t, fx, invitationEmployee(t, fx))
			v, _ := initial.Snapshot(fx.scope)
			operation := onboardingID(t)
			type result struct {
				v d.Invitation
				r bool
				e error
			}
			out := make(chan result, 12)
			start := make(chan struct{})
			var wg sync.WaitGroup
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			for range 12 {
				op := operation
				if !same {
					op = onboardingID(t)
				}
				plan, _ := invitationPlan(t, fx)
				wg.Go(func() {
					<-start
					x, r, e := fx.invites.WriteInvitation(ctx, fx.scope, invitationActor(fx), fx.member, members.InvitationWrite{Action: members.InvitationResend, OperationID: op, InvitationID: v.ID, ExpectedRevision: 1}, plan)
					out <- result{x, r, e}
				})
			}
			close(start)
			wg.Wait()
			close(out)
			fresh, replayed, refused := 0, 0, 0
			id := ""
			for got := range out {
				if got.e != nil {
					if !errors.Is(got.e, members.ErrInvitationConflict) || got.r || !reflect.DeepEqual(got.v, d.Invitation{}) {
						t.Fatal("race error", got.e)
					}
					refused++
					continue
				}
				if got.r {
					replayed++
				} else {
					fresh++
				}
				s, _ := got.v.Snapshot(fx.scope)
				if id != "" && id != s.ID {
					t.Fatal("retry changed target")
				}
				id = s.ID
			}
			if fresh != 1 || same && (replayed != 11 || refused != 0) || !same && (refused != 11 || replayed != 0) {
				t.Fatal("unsafe replacement race", fresh, replayed, refused)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE state='PENDING'`, 1)
			invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE org_id=$1 AND employee_id=$2 AND state='INVITED'`, 1, fx.scope.OrgID().String(), v.EmployeeID)
		})
	}
}
func TestInvitationDeferredFailuresReturnNoResultAndRecover(t *testing.T) {
	for _, table := range []string{"org_audit_events", "org_invitation_commands", "org_invitation_notices"} {
		t.Run(table, func(t *testing.T) {
			fx := invitationFixture(t)
			employee := invitationEmployee(t, fx)
			c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
			if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_invitation_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic private outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_invitation_outage AFTER INSERT ON `+pgx.Identifier{table}.Sanitize()+` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_invitation_outage()`); e != nil {
				t.Fatal(e)
			}
			if v, r, code, e := inviteWrite(t, fx, c); !errors.Is(e, members.ErrRegisterUnavailable) || r || code != "" || !reflect.DeepEqual(v, d.Invitation{}) {
				t.Fatal("failed commit disclosed result", e)
			}
			for _, table := range []string{"org_invitations", "org_invitation_commands", "org_invitation_notices"} {
				invitationCount(t, fx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize(), 0)
			}
			invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE org_id=$1 AND employee_id=$2`, 0, fx.scope.OrgID().String(), employee)
			if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_invitation_outage ON `+pgx.Identifier{table}.Sanitize()); e != nil {
				t.Fatal(e)
			}
			if _, r, code, e := inviteWrite(t, fx, c); e != nil || r || code == "" {
				t.Fatal("recovery", e)
			}
		})
	}
}

func TestInvitationFreshProofExpiresWhileWaitingAtSourceGate(t *testing.T) {
	for _, gate := range []string{"root", "capability"} {
		t.Run(gate, func(t *testing.T) {
			fx := invitationFixture(t)
			employee := invitationEmployee(t, fx)
			plan, _ := invitationPlan(t, fx)
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
			out := make(chan ownershipRaceResult, 1)
			c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
			go func() {
				_, r, e := fx.invites.WriteInvitation(ctx, fx.scope, invitationActor(fx), fx.member, c, plan)
				out <- ownershipRaceResult{replay: r, err: e}
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
					t.Fatal("proof did not expire by database clock")
				case <-ticker.C:
					if e := fx.f.Admin.QueryRow(ctx, `SELECT authenticated_at<=clock_timestamp()-interval '15 minutes' FROM company_web_sessions WHERE id=$1`, fx.actor.SessionID).Scan(&expired); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := holder.Rollback(ctx); e != nil {
				t.Fatal(e)
			}
			got := ownershipRaceFinish(t, out)
			if !errors.Is(got.err, members.ErrEmployeeUnauthenticated) || got.replay {
				t.Fatal("expired waiting proof authorized invitation", got.err)
			}
			invitationCount(t, fx, `SELECT(SELECT count(*) FROM org_invitations)+(SELECT count(*) FROM org_invitation_commands)+(SELECT count(*) FROM org_invitation_notices)+(SELECT count(*) FROM org_audit_events WHERE action LIKE 'idn.invitation.%')`, 0)
		})
	}
}
