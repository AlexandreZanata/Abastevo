//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

type ownershipRaceResult struct {
	transfer d.OwnershipTransfer
	replay   bool
	err      error
}

func ownershipRaceProposal(t *testing.T, fx transferFixtureState, target string) d.OwnershipTransfer {
	t.Helper()
	input := d.OwnershipTransfer{OrgID: fx.scope.OrgID().String(), TransferID: onboardingID(t), InitiatorMembershipID: fx.member, TargetMembershipID: target, OperationID: onboardingID(t)}
	result, replay, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if e != nil || replay {
		t.Fatal("race proposal", e)
	}
	return result
}
func ownershipObservedWait(t *testing.T, fx transferFixtureState, query string, done <-chan ownershipRaceResult) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-done:
			t.Fatal("operation did not wait for the intended lock", result.err)
		case <-deadline.C:
			t.Fatal("did not observe ownership source lock", query)
		case <-ticker.C:
			var observed bool
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock' AND query LIKE $2)`, fx.f.RuntimeRole, "%"+query+"%").Scan(&observed); e != nil {
				t.Fatal(e)
			}
			if observed {
				return
			}
		}
	}
}
func ownershipRaceFinish(t *testing.T, done <-chan ownershipRaceResult) ownershipRaceResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("ownership race did not finish")
		return ownershipRaceResult{}
	}
}

func TestOwnershipCurrentInitiatorOwnerReductionRaceBothOrders(t *testing.T) {
	for _, winner := range []string{"owner reduction", "acceptance"} {
		t.Run(winner, func(t *testing.T) {
			fx := transferFixture(t)
			target, _ := createTransferMember(t, fx, false)
			survivor, _ := createTransferMember(t, fx, false)
			// A distinct owner makes the tested removal legal independently of whether
			// acceptance creates its new owner first.
			if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'synthetic race surviving owner')`, fx.scope.OrgID().String(), onboardingID(t), survivor, fx.member); e != nil {
				t.Fatal(e)
			}
			transfer := ownershipRaceProposal(t, fx, target)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			done := make(chan ownershipRaceResult, 1)
			accept := func() {
				value, replay, e := fx.store.Accept(ctx, fx.scope, fx.actors[target], target, transfer.TransferID, onboardingID(t))
				done <- ownershipRaceResult{value, replay, e}
			}
			end := `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND role='OWNER' AND ended_at IS NULL`
			if winner == "owner reduction" {
				reducing, e := fx.f.Runtime.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer func() { _ = reducing.Rollback(context.Background()) }()
				if _, e := reducing.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); e != nil {
					t.Fatal(e)
				}
				if _, e := reducing.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, fx.actor.AccountID); e != nil {
					t.Fatal(e)
				}
				if _, e := reducing.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
					t.Fatal(e)
				}
				if _, e := reducing.Exec(ctx, end, fx.scope.OrgID().String(), fx.member); e != nil {
					t.Fatal(e)
				}
				go accept()
				ownershipObservedWait(t, fx, "LockLegalAccount", done)
				if e := reducing.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				result := ownershipRaceFinish(t, done)
				if !errors.Is(result.err, d.ErrTransferNotPending) || result.replay || result.transfer != (d.OwnershipTransfer{}) {
					t.Fatal("stale initiator authority accepted", result.err)
				}
			} else {
				blocker, e := fx.f.Admin.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer func() { _ = blocker.Rollback(context.Background()) }()
				if _, e := blocker.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
					t.Fatal(e)
				}
				go accept()
				ownershipObservedWait(t, fx, "LockLegalOrganization", done)
				removal := make(chan ownershipRaceResult, 1)
				go func() {
					e := database.WithOrgTx(ctx, fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
						if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, fx.actor.AccountID); e != nil {
							return e
						}
						if _, e := tx.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); e != nil {
							return e
						}
						_, e := tx.Exec(ctx, end, fx.scope.OrgID().String(), fx.member)
						return e
					})
					removal <- ownershipRaceResult{err: e}
				}()
				ownershipObservedWait(t, fx, "SELECT id FROM accounts", removal)
				if e := blocker.Rollback(ctx); e != nil {
					t.Fatal(e)
				}
				result := ownershipRaceFinish(t, done)
				if result.err != nil || result.replay || result.transfer.State != d.OwnershipTransferAccepted {
					t.Fatal("valid preceding consent failed", result.err)
				}
				if result := ownershipRaceFinish(t, removal); result.err != nil {
					t.Fatal("serialized owner removal", result.err)
				}
			}
			var valid bool
			expected := "PENDING"
			if winner == "acceptance" {
				expected = "ACCEPTED"
			}
			if e := fx.f.Admin.QueryRow(ctx, `SELECT t.state=$3 AND NOT EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=t.org_id AND membership_id=t.initiator_membership_id AND role='OWNER' AND ended_at IS NULL) AND(SELECT count(*) FROM org_role_assignments WHERE org_id=t.org_id AND membership_id=t.target_membership_id AND role='OWNER' AND ended_at IS NULL)=CASE WHEN $3='ACCEPTED' THEN 1 ELSE 0 END FROM org_ownership_transfers t WHERE t.org_id=$1 AND t.transfer_id=$2`, fx.scope.OrgID().String(), transfer.TransferID, expected).Scan(&valid); e != nil || !valid {
				t.Fatal("race left nonserial authority", e)
			}
		})
	}
}

func TestOwnershipWriteRechecksProofAfterObservedRootAndCapabilityWait(t *testing.T) {
	for _, lock := range []string{"root", "capability"} {
		t.Run(lock, func(t *testing.T) {
			fx := transferFixture(t)
			target, _ := createTransferMember(t, fx, false)
			// The proof starts fresh. It expires by the database clock while the
			// operation retains its original proof rows and waits at the next gate.
			if _, e := fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',created_at=clock_timestamp()-interval '15 minutes'+interval '2 seconds',expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, fx.actor.SessionID); e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			blocker, e := fx.f.Admin.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = blocker.Rollback(context.Background()) }()
			query := "LockLegalOrganization"
			if lock == "root" {
				_, e = blocker.Exec(ctx, `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String())
			} else {
				query = "LockStateForWrite"
				_, e = blocker.Exec(ctx, `SELECT org_id FROM org_capabilities WHERE org_id=$1 AND capability='organizations' FOR UPDATE`, fx.scope.OrgID().String())
			}
			if e != nil {
				t.Fatal(e)
			}
			done := make(chan ownershipRaceResult, 1)
			input := d.OwnershipTransfer{OrgID: fx.scope.OrgID().String(), TransferID: onboardingID(t), InitiatorMembershipID: fx.member, TargetMembershipID: target, OperationID: onboardingID(t)}
			go func() {
				value, replay, e := fx.store.Propose(ctx, fx.scope, fx.actor, fx.member, input)
				done <- ownershipRaceResult{value, replay, e}
			}()
			ownershipObservedWait(t, fx, query, done)
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			expired := false
			for !expired {
				select {
				case <-deadline.C:
					t.Fatal("database proof did not expire")
				case <-ticker.C:
					if e := fx.f.Admin.QueryRow(ctx, `SELECT authenticated_at<=clock_timestamp()-interval '15 minutes' FROM company_web_sessions WHERE id=$1`, fx.actor.SessionID).Scan(&expired); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := blocker.Rollback(ctx); e != nil {
				t.Fatal(e)
			}
			result := ownershipRaceFinish(t, done)
			if !errors.Is(result.err, app.ErrUnauthenticated) || result.replay || result.transfer != (d.OwnershipTransfer{}) {
				t.Fatal("elapsed proof authorized pending transfer", result.err)
			}
			var count int
			if e := fx.f.Admin.QueryRow(ctx, `SELECT(SELECT count(*) FROM org_ownership_transfers WHERE org_id=$1)+(SELECT count(*) FROM org_ownership_commands WHERE org_id=$1)+(SELECT count(*) FROM org_ownership_notices WHERE org_id=$1)+(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action LIKE 'org.ownership_transfer.%')`, fx.scope.OrgID().String()).Scan(&count); e != nil || count != 0 {
				t.Fatal("elapsed proof left ownership content, receipt, notice, or audit", count, e)
			}
		})
	}
}

func TestOwnershipStepDownClosesCurrentAndScheduledOwnerPeriods(t *testing.T) {
	fx := transferFixture(t)
	target, _ := createTransferMember(t, fx, false)
	survivor, _ := createTransferMember(t, fx, false)
	org := fx.scope.OrgID().String()
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'synthetic period surviving owner')`, org, onboardingID(t), survivor, fx.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND role='OWNER' AND ended_at IS NULL`, org, fx.member); e != nil {
		t.Fatal(e)
	}
	var boundary time.Time
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()+interval '1 hour'`).Scan(&boundary); e != nil {
		t.Fatal(e)
	}
	currentID, futureID := onboardingID(t), onboardingID(t)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,$3,'synthetic finite current owner')`, org, currentID, fx.member, boundary); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,$4,$3,'synthetic scheduled owner')`, org, futureID, fx.member, boundary); e != nil {
		t.Fatal(e)
	}
	input := d.OwnershipTransfer{OrgID: org, TransferID: onboardingID(t), InitiatorMembershipID: fx.member, TargetMembershipID: target, OperationID: onboardingID(t), StepDownToAdmin: true}
	transfer, replay, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if e != nil || replay {
		t.Fatal("period union proposal", e)
	}
	accepted, replay, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, transfer.TransferID, onboardingID(t))
	if e != nil || replay || accepted.State != d.OwnershipTransferAccepted {
		t.Fatal("period union handover", e)
	}
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*)=2 AND bool_and(ended_at IS NOT NULL AND revision=2) FROM org_role_assignments WHERE org_id=$1 AND id IN($2,$3)) AND NOT EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=$1 AND membership_id=$4 AND role='OWNER' AND ended_at IS NULL) AND EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=$1 AND membership_id=$4 AND role='ADMIN' AND ended_at IS NULL)`, org, currentID, futureID, fx.member).Scan(&valid); e != nil || !valid {
		t.Fatal("step-down retained scheduled owner authority", e)
	}
}

func TestOwnershipStepDownPreservesScheduledAdminAndFillsCurrentGap(t *testing.T) {
	fx := transferFixture(t)
	target, _ := createTransferMember(t, fx, false)
	org, futureID := fx.scope.OrgID().String(), onboardingID(t)
	var boundary time.Time
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()+interval '1 hour'`).Scan(&boundary); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'ADMIN','ORG',$1,$4,$3,'synthetic scheduled administrator')`, org, futureID, fx.member, boundary); e != nil {
		t.Fatal(e)
	}
	input := d.OwnershipTransfer{OrgID: org, TransferID: onboardingID(t), InitiatorMembershipID: fx.member, TargetMembershipID: target, OperationID: onboardingID(t), StepDownToAdmin: true}
	transfer, replay, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if e != nil || replay {
		t.Fatal("scheduled admin proposal", e)
	}
	accepted, replay, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[target], target, transfer.TransferID, onboardingID(t))
	if e != nil || replay || accepted.State != d.OwnershipTransferAccepted {
		t.Fatal("scheduled admin handover", e)
	}
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=$1 AND id=$2 AND revision=1 AND ended_at IS NULL AND valid_from=$4) AND EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=$1 AND membership_id=$3 AND role='ADMIN' AND id<>$2 AND ended_at IS NULL AND valid_from<=clock_timestamp() AND valid_to=$4)`, org, futureID, fx.member, boundary).Scan(&valid); e != nil || !valid {
		t.Fatal("step-down rewrote future admin or left no current authority", e)
	}
}
