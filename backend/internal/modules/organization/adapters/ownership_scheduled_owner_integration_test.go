//go:build integration

package adapters_test

import (
	"errors"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func scheduledOwnerProposal(fx transferFixtureState, target string) d.OwnershipTransfer {
	now := time.Now().UTC()
	return d.OwnershipTransfer{OrgID: fx.scope.OrgID().String(), InitiatorMembershipID: fx.member, TargetMembershipID: target, StepDownToAdmin: false, State: d.OwnershipTransferPending, ExpiresAt: now.Add(d.OwnershipTransferValidity), CreatedAt: now}
}

func scheduleTransferOwner(t *testing.T, fx transferFixtureState, member string) {
	t.Helper()
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp()+interval '2 days',$4,'Synthetic scheduled ownership')`, fx.scope.OrgID().String(), onboardingID(t), member, fx.member); e != nil {
		t.Fatal(e)
	}
}

// B-BR-EMP-173: a recipient whose OWNER period is scheduled is already an
// owner. Propose and accept refuse with the existing conflict, and a role-period
// collision found during handover is a conflict, never a retryable outage.
func TestOwnershipTransferTreatsScheduledOwnerAsAlreadyOwner(t *testing.T) {
	t.Run("propose", func(t *testing.T) { scheduledOwnerPropose(t, transferFixture(t)) })
	t.Run("accept", func(t *testing.T) { scheduledOwnerAccept(t, transferFixture(t)) })
}
func scheduledOwnerPropose(t *testing.T, fx transferFixtureState) {
	scheduled, _ := createTransferMember(t, fx, false)
	scheduleTransferOwner(t, fx, scheduled)
	proposal := scheduledOwnerProposal(fx, scheduled)
	proposal.TransferID, proposal.OperationID = onboardingID(t), onboardingID(t)
	if _, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, proposal); !errors.Is(e, d.ErrTargetAlreadyOwner) {
		t.Fatal("transfer proposed to a scheduled owner", e)
	}
	var pending int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_ownership_transfers WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&pending); e != nil || pending != 0 {
		t.Fatal("refused proposal persisted", pending, e)
	}
}

func scheduledOwnerAccept(t *testing.T, fx transferFixtureState) {
	// The plan appears after a valid proposal: acceptance is the same conflict
	// on every retry and leaves ownership, the proposal and its receipt intact.
	late, _ := createTransferMember(t, fx, false)
	proposal := scheduledOwnerProposal(fx, late)
	proposal.TransferID, proposal.OperationID = onboardingID(t), onboardingID(t)
	if _, _, e := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, proposal); e != nil {
		t.Fatal("valid proposal", e)
	}
	scheduleTransferOwner(t, fx, late)
	for range 2 {
		if _, _, e := fx.store.Accept(t.Context(), fx.scope, fx.actors[late], late, proposal.TransferID, onboardingID(t)); !errors.Is(e, d.ErrTargetAlreadyOwner) {
			t.Fatal("acceptance by a scheduled owner", e)
		}
	}
	var intact bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT state FROM org_ownership_transfers WHERE org_id=$1 AND transfer_id=$2)='PENDING'
 AND(SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND membership_id=$3 AND role='OWNER')=1
 AND(SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND membership_id=$4 AND role='OWNER' AND ended_at IS NULL)=1
 AND NOT EXISTS(SELECT 1 FROM org_ownership_commands WHERE org_id=$1 AND kind='ACCEPT')`, fx.scope.OrgID().String(), proposal.TransferID, late, fx.member).Scan(&intact); e != nil || !intact {
		t.Fatal("refused acceptance changed ownership or history", e)
	}
}
