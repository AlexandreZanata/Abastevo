//go:build integration

package adapters_test

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// This fixture exercises the source SQL as an ordinary scoped producer. HTTP
// proof and command receipts are tested separately by the owning API suite.
func ownershipSchemaFixture(t *testing.T) (legalFixtureState, string) {
	t.Helper()
	fx := legalFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE,TRUNCATE ON org_ownership_transfers TO "+role+";GRANT SELECT,INSERT,UPDATE,DELETE,TRUNCATE ON org_ownership_commands,org_ownership_notices TO "+role); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	account := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, account, "synthetic-ownership-schema-"+account); e != nil {
		t.Fatal(e)
	}
	_, target := teamDriver(t, fx, account)
	return fx, target
}
func ownershipSchemaProposal(t *testing.T, fx legalFixtureState, target string) (string, string) {
	t.Helper()
	id, op := onboardingID(t), onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_ownership_transfers(org_id,transfer_id,initiator_membership_id,target_membership_id,step_down_to_admin,state,created_at,expires_at,operation_id) VALUES($1,$2,$3,$4,false,'PENDING','2000-01-01T00:00:00Z','2100-01-01T00:00:00Z',$5)`, fx.scope.OrgID().String(), id, fx.member, target, op); e != nil {
		t.Fatal(e)
	}
	return id, op
}
func TestOwnershipHardeningSQLInitialSourceTimeHistoryAndReceipts(t *testing.T) {
	fx, target := ownershipSchemaFixture(t)
	org := fx.scope.OrgID().String()
	before := time.Now().UTC()
	id, op := ownershipSchemaProposal(t, fx, target)
	var created, expires time.Time
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT created_at,expires_at FROM org_ownership_transfers WHERE org_id=$1 AND transfer_id=$2`, org, id).Scan(&created, &expires); e != nil || created.Before(before) || expires.Sub(created) != 7*24*time.Hour {
		t.Fatal("invented proposal time survived", e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `INSERT INTO org_ownership_transfers(org_id,transfer_id,initiator_membership_id,target_membership_id,step_down_to_admin,state,created_at,expires_at,accepted_at,operation_id) VALUES($1,$2,$3,$4,false,'ACCEPTED',clock_timestamp(),clock_timestamp()+interval '7 days',clock_timestamp(),$5)`, org, onboardingID(t), fx.member, target, onboardingID(t)), "P8701")
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='PENDING' WHERE org_id=$1 AND transfer_id=$2`, org, id), "P8703")
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='CANCELLED',operation_id=$3 WHERE org_id=$1 AND transfer_id=$2`, org, id, onboardingID(t)), "P8703")
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='ACCEPTED' WHERE org_id=$1 AND transfer_id=$2`, org, id), "P8702")
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='EXPIRED' WHERE org_id=$1 AND transfer_id=$2`, org, id), "P8702")
	teamSQLCode(t, unitSchemaExec(t, fx, `DELETE FROM org_ownership_transfers WHERE org_id=$1 AND transfer_id=$2`, org, id), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx, `TRUNCATE org_ownership_transfers CASCADE`), "55000")
	// A receipt is backed by the matching actual account/action/target/hash audit,
	// not a membership ID or a caller-invented successful response.
	if e := unitSchemaExec(t, fx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash) VALUES($1,$2,$3,'org.ownership_transfer.proposed','ownership_transfer',$4,'proposed',repeat('a',64))`, org, op, fx.actor.AccountID, id); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `INSERT INTO org_ownership_commands(org_id,operation_id,actor_account_id,kind,request_hash,transfer_id,result) VALUES($1,$2,$3,'PROPOSE',repeat('b',64),$4,'{}')`, org, op, fx.actor.AccountID, id), "P8702")
	if e := unitSchemaExec(t, fx, `INSERT INTO org_ownership_commands(org_id,operation_id,actor_account_id,kind,request_hash,transfer_id,result) VALUES($1,$2,$3,'PROPOSE',repeat('a',64),$4,'{}')`, org, op, fx.actor.AccountID, id); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_commands SET result='{}' WHERE org_id=$1`, org), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx, `DELETE FROM org_ownership_commands WHERE org_id=$1`, org), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_notices SET delivery_state='PENDING_NOTIFICATION_CORE' WHERE org_id=$1`, org), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx, `INSERT INTO org_ownership_notices(org_id,transfer_id,kind) VALUES($1,$2,'ACCEPTED')`, org, id), "P8702")
	var count int
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_ownership_transfers`).Scan(&count); e != nil || count != 0 {
		t.Fatal("unscoped transfer visibility", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='CANCELLED',reduction_cause='MEMBERSHIP_REDUCED',source_membership_id=$3,source_membership_revision=2 WHERE org_id=$1 AND transfer_id=$2`, org, id, target); e == nil {
		t.Fatal("fabricated reduction source accepted")
	}
}
func TestOwnershipHardeningSQLMembershipAndArchivedSourceNotices(t *testing.T) {
	fx, target := ownershipSchemaFixture(t)
	org := fx.scope.OrgID().String()
	id, _ := ownershipSchemaProposal(t, fx, target)
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, target); e != nil {
		t.Fatal("target reduction", e)
	}
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT t.state='CANCELLED' AND t.reduction_cause='MEMBERSHIP_REDUCED' AND t.source_membership_id=m.id AND t.source_membership_revision=m.revision AND(SELECT count(*) FROM org_ownership_notices n WHERE n.org_id=t.org_id AND n.transfer_id=t.transfer_id)=2 FROM org_ownership_transfers t JOIN org_memberships m ON(m.org_id,m.id)=(t.org_id,t.target_membership_id) WHERE t.org_id=$1 AND t.transfer_id=$2`, org, id).Scan(&valid); e != nil || !valid {
		t.Fatal("causal target notice missing", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, target); e != nil {
		t.Fatal(e)
	}
	next, _ := ownershipSchemaProposal(t, fx, target)
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, org); e != nil {
		t.Fatal("archive cancellation", e)
	}
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state='CANCELLED' AND reduction_cause='ORGANIZATION_ARCHIVED' AND source_membership_id IS NULL FROM org_ownership_transfers WHERE org_id=$1 AND transfer_id=$2`, org, next).Scan(&valid); e != nil || !valid {
		t.Fatal("archive cause missing", e)
	}
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var n int
		if e := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_ownership_notices WHERE org_id=$1 AND delivery_state='PENDING_NOTIFICATION_CORE'`, org).Scan(&n); e != nil {
			return e
		}
		if n != 4 {
			t.Fatal("pending notice lifecycle", n)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}

func TestOwnershipHardeningSQLStepDownEndsCurrentAndScheduledOwnerUnion(t *testing.T) {
	fx, target := ownershipSchemaFixture(t)
	org := fx.scope.OrgID().String()
	survivor := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, survivor, "synthetic-ownership-future-survivor-"+survivor); e != nil {
		t.Fatal(e)
	}
	_, backup := teamDriver(t, fx, survivor)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'synthetic surviving owner')`, org, onboardingID(t), backup, fx.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND role='OWNER' AND ended_at IS NULL`, org, fx.member); e != nil {
		t.Fatal(e)
	}
	var boundary time.Time
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()+interval '1 hour'`).Scan(&boundary); e != nil {
		t.Fatal(e)
	}
	for _, period := range []struct {
		id     string
		future bool
	}{{onboardingID(t), false}, {onboardingID(t), true}} {
		query := `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,$3,'synthetic finite current owner')`
		if period.future {
			query = `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,$4,$3,'synthetic scheduled owner')`
		}
		if e := unitSchemaExec(t, fx, query, org, period.id, fx.member, boundary); e != nil {
			t.Fatal(e)
		}
	}
	transfer := onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_ownership_transfers(org_id,transfer_id,initiator_membership_id,target_membership_id,step_down_to_admin,state,expires_at,operation_id) VALUES($1,$2,$3,$4,true,'PENDING',clock_timestamp()+interval '168 hours',$5)`, org, transfer, fx.member, target, onboardingID(t)); e != nil {
		t.Fatal(e)
	}
	// Rehearse the handover's actual source effects. Ending only the presently
	// active period would let the supposedly demoted initiator regain OWNER.
	if e := unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'synthetic ownership acceptance')`, org, onboardingID(t), target, fx.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND role='OWNER' AND ended_at IS NULL AND valid_from<=clock_timestamp()`, org, fx.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp(),$4,'synthetic ownership step-down')`, org, onboardingID(t), fx.member, target); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='ACCEPTED' WHERE org_id=$1 AND transfer_id=$2`, org, transfer), "P8702")
	if e := unitSchemaExec(t, fx, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND role='OWNER' AND ended_at IS NULL`, org, fx.member); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_ownership_transfers SET state='ACCEPTED' WHERE org_id=$1 AND transfer_id=$2`, org, transfer); e != nil {
		t.Fatal("complete owner-period union could not step down", e)
	}
}
