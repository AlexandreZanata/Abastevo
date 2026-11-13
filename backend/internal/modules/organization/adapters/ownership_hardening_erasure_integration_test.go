//go:build integration

package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestOwnershipHardeningOrdinaryTwoOrganizationErasureLastOwnerAndDeferredRollback(t *testing.T) {
	fx, target := ownershipSchemaFixture(t)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if e := unitSchemaExec(t, other, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, other.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	targetAccount := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, targetAccount, "synthetic-ownership-erasure-target-"+targetAccount); e != nil {
		t.Fatal(e)
	}
	_, otherTarget := teamDriver(t, other, targetAccount)
	first, _ := ownershipSchemaProposal(t, fx, target)
	second, _ := ownershipSchemaProposal(t, other, otherTarget)
	type pending struct {
		fx       legalFixtureState
		transfer string
	}
	proposals := []pending{{fx, first}, {other, second}}
	eraser := fx.f.RuntimeRole + "_ownership_eraser"
	role := pgx.Identifier{eraser}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE ON SCHEMA public TO "+role+";GRANT SELECT,UPDATE ON organizations,accounts,org_role_assignments,org_teams,org_team_links,org_device_enrollments,org_ownership_transfers TO "+role+";GRANT SELECT ON org_employee_identities,payment_billing_exits TO "+role+";GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices,org_ownership_notices TO "+role+";GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+role+";GRANT SELECT,UPDATE ON org_invitations,org_invitation_acceptances TO "+role+";GRANT SELECT,INSERT ON org_invitation_notices,org_invitation_acceptance_notices TO "+role+";ALTER TABLE org_memberships OWNER TO "+role+";ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role+";ALTER FUNCTION org_membership_check_account_exit(uuid) OWNER TO "+role); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := fx.f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	// Personal erasure's pre-existing tables use fixture DML rights. Its
	// enterprise definer is separately ordinary and has only scoped projections.
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT to_jsonb(a) FROM accounts a WHERE id=$1),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,transfer_id) FROM org_ownership_transfers x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,transfer_id,kind) FROM org_ownership_notices x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x))::text`, fx.actor.AccountID).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	erase := func(checkScope bool) error {
		return pgx.BeginFunc(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
			caller := onboardingID(t)
			if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, caller); e != nil {
				return e
			}
			if _, e := tx.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, fx.actor.AccountID); e != nil {
				return e
			}
			if _, e := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); e != nil {
				return e
			}
			if checkScope {
				var restored string
				if e := tx.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&restored); e != nil {
					return e
				}
				if restored != caller {
					t.Error("erasure leaked caller organization scope")
				}
			}
			return nil
		})
	}
	before := snapshot()
	teamSQLCode(t, erase(false), "P7501")
	if snapshot() != before {
		t.Fatal("last-owner rejection changed source, history, or notices")
	}
	// Add a distinct current personal owner in both organizations. The original
	// owner remains the initiator so erasure must consume that side of proposals.
	survivor := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, survivor, "synthetic-ownership-survivor-"+survivor); e != nil {
		t.Fatal(e)
	}
	for _, proposal := range proposals {
		_, member := teamDriver(t, proposal.fx, survivor)
		if e := unitSchemaExec(t, proposal.fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'synthetic erasure surviving owner')`, proposal.fx.scope.OrgID().String(), onboardingID(t), member, proposal.fx.member); e != nil {
			t.Fatal(e)
		}
	}
	before = snapshot()
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_ownership_erasure_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic ownership notification storage outage';END$$;CREATE CONSTRAINT TRIGGER synthetic_ownership_erasure_failure AFTER INSERT ON org_ownership_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='CANCELLED') EXECUTE FUNCTION synthetic_ownership_erasure_failure()`); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, erase(false), "23503")
	if snapshot() != before {
		t.Fatal("deferred projection failure left partial account, membership, roles, proposals, or notices")
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_ownership_erasure_failure ON org_ownership_notices`); e != nil {
		t.Fatal(e)
	}
	if e := erase(true); e != nil {
		t.Fatal("ordinary erasure recovery", e)
	}
	for _, proposal := range proposals {
		var valid bool
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT t.state='CANCELLED' AND t.reduction_cause='MEMBERSHIP_REDUCED' AND t.source_membership_id=m.id AND t.source_membership_revision=m.revision AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND m.state='ENDED' AND(SELECT count(*) FROM org_ownership_notices n WHERE n.org_id=t.org_id AND n.transfer_id=t.transfer_id)=2 AND NOT EXISTS(SELECT 1 FROM org_audit_events a WHERE a.org_id=t.org_id AND a.target_id=t.transfer_id) FROM org_ownership_transfers t JOIN org_memberships m ON(m.org_id,m.id)=(t.org_id,t.initiator_membership_id) WHERE t.org_id=$1 AND t.transfer_id=$2`, proposal.fx.scope.OrgID().String(), proposal.transfer).Scan(&valid); e != nil || !valid {
			t.Fatal("actual initiator erasure cause missing or manager audit invented", e)
		}
	}
	var unsafe bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls OR rolcanlogin FROM pg_roles WHERE rolname=$1`, eraser).Scan(&unsafe); e != nil || unsafe {
		t.Fatal("unsafe erasure owner", e)
	}
	var scope string
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&scope); e != nil || scope != "" {
		t.Fatal("erasure leaked pooled organization scope", e)
	}
}
