//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

func transferProposal(t *testing.T, fx transferFixtureState, target string, step bool) d.OwnershipTransfer {
	t.Helper()
	return d.OwnershipTransfer{OrgID: fx.scope.OrgID().String(), TransferID: onboardingID(t), InitiatorMembershipID: fx.member, TargetMembershipID: target, StepDownToAdmin: step, OperationID: onboardingID(t)}
}
func proposeTransfer(t *testing.T, fx transferFixtureState, target string, step bool) d.OwnershipTransfer {
	t.Helper()
	v, replay, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, transferProposal(t, fx, target, step))
	if e != nil || replay {
		t.Fatal("proposal", e)
	}
	return v
}
func transferAssertCount(t *testing.T, fx transferFixtureState, sql string, want int, args ...any) {
	t.Helper()
	var n int
	if e := fx.f.Admin.QueryRow(t.Context(), sql, args...).Scan(&n); e != nil || n != want {
		t.Fatal("source count", n, want, e)
	}
}
func TestOwnershipSecurityImmutableReceiptsAndParticipantReads(t *testing.T) {
	fx := transferFixture(t)
	target, _ := createTransferMember(t, fx, false)
	stranger, _ := createTransferMember(t, fx, false)
	input := transferProposal(t, fx, target, false)
	pending, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if e != nil {
		t.Fatal(e)
	}
	op := onboardingID(t)
	accepted, _, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, pending.TransferID, op)
	if e != nil {
		t.Fatal(e)
	}
	// Exact proposal retries retain the original pending acknowledgement, even
	// after the underlying entity has advanced to ACCEPTED.
	again, replay, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if e != nil || !replay || !reflect.DeepEqual(again, pending) {
		t.Fatal("proposal receipt changed", e)
	}
	again, replay, e = fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, pending.TransferID, op)
	if e != nil || !replay || !reflect.DeepEqual(again, accepted) {
		t.Fatal("accept receipt changed", e)
	}
	if v, r, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, pending.TransferID, onboardingID(t)); !errors.Is(e, d.ErrTransferNotPending) || r || v != (d.OwnershipTransfer{}) {
		t.Fatal("new operation became replay", e)
	}
	for _, id := range []string{pending.TransferID, ""} {
		v, e := fx.store.GetCurrent(t.Context(), fx.scope, fx.actors[stranger], stranger, id)
		if !errors.Is(e, app.ErrNotFound) || v != (d.OwnershipTransfer{}) {
			t.Fatal("foreign participant read", e)
		}
	}
	if v, r, e := fx.store.Cancel(t.Context(), fx.scope, fx.actors[stranger], stranger, pending.TransferID, onboardingID(t)); !errors.Is(e, app.ErrNotFound) || r || v != (d.OwnershipTransfer{}) {
		t.Fatal("foreign terminal command", e)
	}
	if v, r, e := fx.store.Accept(t.Context(), fx.scope, fx.actor, fx.member, pending.TransferID, op); !errors.Is(e, app.ErrNotFound) || r || v != (d.OwnershipTransfer{}) {
		t.Fatal("initiator accepted", e)
	}
	transferAssertCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE target_id=$1 AND actor_id=$2 AND action='org.ownership_transfer.accepted'`, 1, pending.TransferID, fx.actors[target].AccountID)
	transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_commands`, 2)
}
func TestOwnershipSecurityCancelReplaySurvivesReplacement(t *testing.T) {
	for _, byTarget := range []bool{false, true} {
		t.Run(map[bool]string{true: "decline", false: "cancel"}[byTarget], func(t *testing.T) {
			fx := transferFixture(t)
			target, _ := createTransferMember(t, fx, false)
			other, _ := createTransferMember(t, fx, false)
			pending := proposeTransfer(t, fx, target, true)
			member := fx.member
			if byTarget {
				member = target
			}
			actor := fx.actors[member]
			op := onboardingID(t)
			done, _, e := fx.store.Cancel(t.Context(), fx.scope, actor, member, pending.TransferID, op)
			if e != nil {
				t.Fatal(e)
			}
			next := proposeTransfer(t, fx, other, true)
			old, replay, e := fx.store.Cancel(t.Context(), fx.scope, actor, member, pending.TransferID, op)
			if e != nil || !replay || !reflect.DeepEqual(old, done) {
				t.Fatal("cancel replay depended on current pending", e)
			}
			if v, r, e := fx.store.Cancel(t.Context(), fx.scope, fx.actor, fx.member, next.TransferID, op); !errors.Is(e, d.ErrTransferOperationReused) || r || v != (d.OwnershipTransfer{}) {
				t.Fatal("command reused for another transfer", e)
			}
			current, e := fx.store.GetCurrent(t.Context(), fx.scope, fx.actor, fx.member, "")
			if e != nil || current.TransferID != next.TransferID {
				t.Fatal("new pending changed", e)
			}
		})
	}
}
func TestOwnershipSecurityFreshProofAndCurrentAuthorityRequired(t *testing.T) {
	cases := []struct {
		name, sql string
		want      error
	}{
		{"cookie revoked", `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, app.ErrUnauthenticated},
		{"family revoked", `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, app.ErrUnauthenticated},
		{"freshness lost", `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, app.ErrUnauthenticated},
		{"account inactive", `UPDATE accounts SET status='suspended' WHERE id=$1`, app.ErrUnauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := transferFixture(t)
			target, _ := createTransferMember(t, fx, false)
			input := transferProposal(t, fx, target, false)
			first, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
			if e != nil {
				t.Fatal(e)
			}
			if tc.name == "account inactive" {
				backup, _ := createTransferMember(t, fx, false)
				if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp()-interval '1 minute',$4,'Synthetic backup')`, fx.scope.OrgID().String(), onboardingID(t), backup, fx.member); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := fx.f.Admin.Exec(t.Context(), tc.sql, fx.actor.AccountID); e != nil {
				t.Fatal(e)
			}
			if v, r, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input); !errors.Is(e, tc.want) || r || v != (d.OwnershipTransfer{}) {
				t.Fatal("stale proposal replay", e)
			}
			if v, e := fx.store.GetCurrent(t.Context(), fx.scope, fx.actor, fx.member, first.TransferID); !errors.Is(e, tc.want) || v != (d.OwnershipTransfer{}) {
				t.Fatal("stale private read", e)
			}
			if v, r, e := fx.store.Cancel(t.Context(), fx.scope, fx.actor, fx.member, first.TransferID, onboardingID(t)); !errors.Is(e, tc.want) || r || v != (d.OwnershipTransfer{}) {
				t.Fatal("stale reduction", e)
			}
			transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_commands`, 1)
		})
	}
	t.Run("forged actor membership", func(t *testing.T) {
		fx := transferFixture(t)
		target, _ := createTransferMember(t, fx, false)
		input := transferProposal(t, fx, target, false)
		actor := fx.actor
		actor.SessionID = fx.actors[target].SessionID
		if _, _, e := fx.store.Propose(t.Context(), fx.scope, actor, fx.member, input); !errors.Is(e, app.ErrUnauthenticated) {
			t.Fatal("foreign session accepted", e)
		}
		if _, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actors[target], fx.member, input); !errors.Is(e, access.ErrNoMembership) {
			t.Fatal("foreign member selector accepted", e)
		}
		transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_transfers`, 0)
	})
	t.Run("capability revoked permits reductions only", func(t *testing.T) {
		fx := transferFixture(t)
		target, _ := createTransferMember(t, fx, false)
		input := transferProposal(t, fx, target, false)
		first, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
			t.Fatal(e)
		}
		if _, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input); !errors.Is(e, cap.ErrEntitlementRequired) {
			t.Fatal("disabled replay", e)
		}
		if _, _, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, first.TransferID, onboardingID(t)); !errors.Is(e, cap.ErrEntitlementRequired) {
			t.Fatal("disabled acceptance", e)
		}
		if _, e := fx.store.GetCurrent(t.Context(), fx.scope, fx.actors[target], target, first.TransferID); e != nil {
			t.Fatal("disabled read denied", e)
		}
		if v, _, e := fx.store.Cancel(t.Context(), fx.scope, fx.actors[target], target, first.TransferID, onboardingID(t)); e != nil || v.State != d.OwnershipTransferDeclined {
			t.Fatal("disabled reduction denied", e)
		}
	})
}
func TestOwnershipSecurityExistingAdminStepDownAndLostSourceOwner(t *testing.T) {
	t.Run("reuse existing admin", func(t *testing.T) {
		fx := transferFixture(t)
		target, _ := createTransferMember(t, fx, false)
		if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp()-interval '1 minute',$3,'Synthetic admin')`, fx.scope.OrgID().String(), onboardingID(t), fx.member); e != nil {
			t.Fatal(e)
		}
		p := proposeTransfer(t, fx, target, true)
		if _, _, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, p.TransferID, onboardingID(t)); e != nil {
			t.Fatal("existing admin broke step-down", e)
		}
		transferAssertCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND role='ADMIN' AND ended_at IS NULL`, 1, fx.member)
	})
	t.Run("initiator no longer owner", func(t *testing.T) {
		fx := transferFixture(t)
		target, _ := createTransferMember(t, fx, false)
		backup, _ := createTransferMember(t, fx, false)
		if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp()-interval '1 minute',$4,'Synthetic backup owner')`, fx.scope.OrgID().String(), onboardingID(t), backup, fx.member); e != nil {
			t.Fatal(e)
		}
		p := proposeTransfer(t, fx, target, false)
		if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND role='OWNER'`, fx.scope.OrgID().String(), fx.member); e != nil {
			t.Fatal(e)
		}
		if v, r, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, p.TransferID, onboardingID(t)); !errors.Is(e, d.ErrTransferNotPending) || r || v != (d.OwnershipTransfer{}) {
			t.Fatal("stale owner transferred authority", e)
		}
		transferAssertCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND role='OWNER'`, 0, target)
		transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_commands`, 1)
	})
}
func TestOwnershipSecurityAtomicFailuresAndRecovery(t *testing.T) {
	for _, table := range []string{"org_audit_events", "org_ownership_commands", "org_ownership_notices"} {
		t.Run(table, func(t *testing.T) {
			fx := transferFixture(t)
			target, _ := createTransferMember(t, fx, false)
			p := proposeTransfer(t, fx, target, true)
			// A real deferred failure occurs after the role handover callback succeeds.
			if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_transfer_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='Synthetic private storage outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_transfer_failure AFTER INSERT ON `+table+` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_transfer_failure()`); e != nil {
				t.Fatal(e)
			}
			op := onboardingID(t)
			v, r, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, p.TransferID, op)
			if !errors.Is(e, app.ErrUnavailable) || r || v != (d.OwnershipTransfer{}) {
				t.Fatal("failed commit exposed success", e)
			}
			transferAssertCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND role='OWNER'`, 0, target)
			transferAssertCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND role='OWNER' AND ended_at IS NULL`, 1, fx.member)
			transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_commands`, 1)
			transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_notices`, 1)
			if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_transfer_failure ON `+table); e != nil {
				t.Fatal(e)
			}
			if _, r, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, p.TransferID, op); e != nil || r {
				t.Fatal("retry after rollback", e)
			}
		})
	}
}
func TestOwnershipSecurityConcurrentCommands(t *testing.T) {
	for _, mode := range []string{"same proposal", "distinct proposals", "same acceptance", "accept versus decline"} {
		t.Run(mode, func(t *testing.T) {
			fx := transferFixture(t)
			target, _ := createTransferMember(t, fx, false)
			input := transferProposal(t, fx, target, false)
			op := onboardingID(t)
			if mode == "same acceptance" || mode == "accept versus decline" {
				input = proposeTransfer(t, fx, target, false)
			}
			type result struct {
				v d.OwnershipTransfer
				r bool
				e error
			}
			ch := make(chan result, 12)
			start := make(chan struct{})
			var wg sync.WaitGroup
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			for i := range 12 {
				in := input
				id := op
				if mode == "distinct proposals" {
					in.OperationID = onboardingID(t)
					in.TransferID = onboardingID(t)
				}
				if mode == "accept versus decline" {
					id = onboardingID(t)
				}
				wg.Go(func() {
					<-start
					var v d.OwnershipTransfer
					var r bool
					var e error
					switch mode {
					case "same proposal", "distinct proposals":
						v, r, e = fx.store.Propose(ctx, fx.scope, fx.actor, fx.member, in)
					case "same acceptance":
						v, r, e = fx.store.Accept(ctx, fx.scope, fx.actors[target], target, in.TransferID, id)
					default:
						if i%2 == 0 {
							v, r, e = fx.store.Accept(ctx, fx.scope, fx.actors[target], target, in.TransferID, id)
						} else {
							v, r, e = fx.store.Cancel(ctx, fx.scope, fx.actors[target], target, in.TransferID, id)
						}
					}
					ch <- result{v, r, e}
				})
			}
			close(start)
			wg.Wait()
			close(ch)
			fresh, replays, refused := 0, 0, 0
			var original d.OwnershipTransfer
			for got := range ch {
				if got.e != nil {
					want := d.ErrTransferNotPending
					if mode == "distinct proposals" {
						want = d.ErrTransferPending
					}
					if !errors.Is(got.e, want) || got.r || got.v != (d.OwnershipTransfer{}) {
						t.Fatal("unsafe race outcome", got.e)
					}
					refused++
					continue
				}
				if got.r {
					replays++
				} else {
					fresh++
				}
				if original.TransferID == "" {
					original = got.v
				} else if (mode == "same proposal" || mode == "same acceptance") && !reflect.DeepEqual(original, got.v) {
					t.Fatal("concurrent receipt changed")
				}
			}
			if fresh != 1 {
				t.Fatal("multiple applied commands", fresh, replays, refused)
			}
			if mode == "same proposal" || mode == "same acceptance" {
				if replays != 11 || refused != 0 {
					t.Fatal("exact retries lost", fresh, replays, refused)
				}
			} else if refused != 11 || replays != 0 {
				t.Fatal("distinct commands coalesced", fresh, replays, refused)
			}
			transferAssertCount(t, fx, `SELECT count(*) FROM org_ownership_transfers`, 1)
		})
	}
}
func TestOwnershipSecurityDeniedOwnerProofHasNoPermission(t *testing.T) {
	fx := transferFixture(t)
	target, _ := createTransferMember(t, fx, false)
	stranger, _ := createTransferMember(t, fx, false)
	input := transferProposal(t, fx, target, false)
	input.InitiatorMembershipID = stranger
	var denied *roles.PermissionDenial
	if _, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actors[stranger], stranger, input); !errors.As(e, &denied) {
		t.Fatal("non-owner proposed", e)
	}
}

func TestOwnershipSourceProofPrecedesMissingReferenceResults(t *testing.T) {
	fx := transferFixture(t)
	target, _ := createTransferMember(t, fx, false)
	p := proposeTransfer(t, fx, target, false)
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, fx.actors[target].SessionID); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{p.TransferID, onboardingID(t)} {
		if v, r, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, id, onboardingID(t)); !errors.Is(e, app.ErrUnauthenticated) || r || v != (d.OwnershipTransfer{}) {
			t.Fatal("reference bypassed current proof", e)
		}
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, fx.actor.SessionID); e != nil {
		t.Fatal(e)
	}
	input := transferProposal(t, fx, onboardingID(t), false)
	if v, r, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input); !errors.Is(e, app.ErrUnauthenticated) || r || v != (d.OwnershipTransfer{}) {
		t.Fatal("missing target bypassed current proof", e)
	}
}
