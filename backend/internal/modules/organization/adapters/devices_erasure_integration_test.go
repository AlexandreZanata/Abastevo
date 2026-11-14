//go:build integration

package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestOrganizationDevicesOrdinaryAccountErasureEndsBothScopesAndPreservesReceipts(t *testing.T) {
	fx := devicesFixture(t)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	_, otherMember := teamDriver(t, other, fx.person.auth.Account.ID)
	otherProof := fx
	otherProof.legalFixtureState = other
	otherActor := deviceVerifyActor(t, otherProof, fx.person, fx.person.signer, time.Time{})
	if _, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic first erasure device")); e != nil {
		t.Fatal(e)
	}
	if _, e := fx.devices.EnrollDevice(t.Context(), other.scope, otherActor, deviceEnrollCommand(t, "Synthetic second erasure device")); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		t.Helper()
		var out string
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_device_commands x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM account_session_families x WHERE account_id=$1))::text`, fx.actor.AccountID).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := snapshot()
	// Exact operations used by the installed personal erasure triggers in
	// migrations075/081/133..143/164. This temporary fixture role/database is
	// dropped by orgtest; no grant applies to arbitrary or future tables.
	runtimeRole := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	for _, grant := range []string{
		"GRANT SELECT,INSERT,UPDATE ON cloud_state,driver_earning_owner_state TO ",
		"GRANT SELECT,INSERT ON cloud_erased_ids,cloud_device_revocations,subscription_retired_tokens TO ",
		"GRANT SELECT,UPDATE ON cloud_records,cloud_changes,cloud_devices,entitlement_grants,contributor_github_links,staff_assignments,subscription_purchases,subscription_operations,subscription_verification_slots TO ",
		"GRANT SELECT,DELETE ON driver_earning_adjustments,driver_earning_revisions,driver_earnings,driver_earning_commands,driver_earning_tombstones,driver_distance_commands,driver_distance_observations,driver_shift_commands,driver_shift_gaps,driver_shifts,driver_cost_profile_items,driver_cost_commands,driver_cost_tombstones,driver_cost_profiles,driver_maintenance_commands,driver_maintenance_tombstones,driver_maintenance_schedules,driver_reconciliation_commands,driver_reconciliation_records,driver_reconciliation_decisions,driver_report_snapshots,contributor_claims,contributor_profile_edits,contributor_links,contributor_profiles,forum_comment_reactions,forum_post_reactions TO ",
	} {
		if _, e := fx.f.Admin.Exec(t.Context(), grant+runtimeRole); e != nil {
			t.Fatal(e)
		}
	}
	eraser := fx.f.RuntimeRole + "_device_eraser"
	role := pgx.Identifier{eraser}.Sanitize()
	// Only the actual enterprise erasure projections are granted to the
	// disposable ordinary definer. No blanket table grant or BYPASSRLS.
	if _, e := fx.f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE ON SCHEMA public TO "+role+";GRANT SELECT,UPDATE ON organizations,accounts,org_role_assignments,org_teams,org_team_links,org_ownership_transfers,org_device_enrollments TO "+role+";GRANT SELECT ON org_employee_identities,payment_billing_exits TO "+role+";GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices,org_ownership_notices,org_invitation_notices,org_invitation_acceptance_notices TO "+role+";GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+role+";GRANT SELECT,UPDATE ON org_invitations,org_invitation_acceptances TO "+role+";ALTER TABLE org_memberships OWNER TO "+role+";ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role+";ALTER FUNCTION org_membership_check_account_exit(uuid) OWNER TO "+role); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := fx.f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	caller := onboardingID(t)
	if e := pgx.BeginFunc(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
		if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, caller); e != nil {
			return e
		}
		if _, e := tx.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, fx.person.auth.Account.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); e != nil {
			return e
		}
		var current string
		if e := tx.QueryRow(t.Context(), `SELECT current_setting('anpfuel.org_id',true)`).Scan(&current); e != nil {
			return e
		}
		if current != caller {
			t.Error("erasure leaked caller organization")
		}
		return nil
	}); e != nil {
		t.Fatal("ordinary erasure source", e)
	}
	for _, member := range []string{fx.person.member, otherMember} {
		var valid bool
		if e := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_device_enrollments d JOIN org_memberships m ON(m.org_id,m.id)=(d.org_id,d.membership_id) WHERE m.id=$1 AND m.state='ENDED' AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND d.state='REVOKED' AND d.revocation_class='ACCESS_ENDED' AND d.source_type='MEMBERSHIP' AND d.source_id=m.id AND d.source_revision=m.revision)`, member).Scan(&valid); e != nil || !valid {
			t.Fatal("erasure missed actual scoped device source", e)
		}
	}
	if snapshot() != before {
		t.Fatal("erasure rewrote device acknowledgement or unrelated personal family")
	}
	var unsafe bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls OR rolcanlogin FROM pg_roles WHERE rolname=$1`, eraser).Scan(&unsafe); e != nil || unsafe {
		t.Fatal("unsafe device erasure owner", e)
	}
}
