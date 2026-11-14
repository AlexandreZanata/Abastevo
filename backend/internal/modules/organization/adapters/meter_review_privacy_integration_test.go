//go:build integration

package adapters_test

import (
	"context"
	"errors"
	cloudpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	cloudapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestMeterReviewPersonalErasurePreservesHeldEvidenceAndAllowsRealReviewer(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	op, ref, ack, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM org_sync_review_items x WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(x)ORDER BY review_item_id)FROM org_meter_held_readings x WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(x)ORDER BY operation_id)FROM org_sync_acknowledgements x WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM org_meter_readings x WHERE org_id=$1),(SELECT jsonb_agg(to_jsonb(x)ORDER BY operation_id)FROM org_audit_events x WHERE org_id=$1))::text`, fx.scope.OrgID().String()).Scan(&value); err != nil {
			t.Fatal("review evidence snapshot", err)
		}
		return value
	}
	before := snapshot()
	runtime := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON cloud_records TO "+runtime); err != nil {
		t.Fatal(err)
	}
	personal := cloudpg.NewStore(fx.f.Runtime)
	principal := cloudapp.Principal{AccountID: fx.person.auth.Account.ID, FamilyID: fx.person.auth.Session.FamilyID}
	for _, dataset := range []string{"org_sync_review_items", "org_meter_held_readings", "org_sync_review_resolutions"} {
		if rows, err := personal.List(t.Context(), principal, dataset, 100, ""); !errors.Is(err, cloud.ErrInvalid) || len(rows) != 0 {
			t.Fatal("personal inventory admitted private review history")
		}
	}
	// Reuse the exact installed erasure projections and their established grants.
	// The caller and the SECURITY DEFINER owner are both ordinary NOBYPASSRLS
	// roles; the new history tables are deliberately absent from erasure grants.
	for _, grant := range []string{
		"GRANT SELECT,INSERT,UPDATE ON cloud_state,driver_earning_owner_state TO ",
		"GRANT SELECT,INSERT ON cloud_erased_ids,cloud_device_revocations,subscription_retired_tokens TO ",
		"GRANT SELECT,UPDATE ON cloud_records,cloud_changes,cloud_devices,entitlement_grants,contributor_github_links,staff_assignments,subscription_purchases,subscription_operations,subscription_verification_slots TO ",
		"GRANT SELECT,DELETE ON driver_earning_adjustments,driver_earning_revisions,driver_earnings,driver_earning_commands,driver_earning_tombstones,driver_distance_commands,driver_distance_observations,driver_shift_commands,driver_shift_gaps,driver_shifts,driver_cost_profile_items,driver_cost_commands,driver_cost_tombstones,driver_cost_profiles,driver_maintenance_commands,driver_maintenance_tombstones,driver_maintenance_schedules,driver_reconciliation_commands,driver_reconciliation_records,driver_reconciliation_decisions,driver_report_snapshots,contributor_claims,contributor_profile_edits,contributor_links,contributor_profiles,forum_comment_reactions,forum_post_reactions TO ",
	} {
		if _, err := fx.f.Admin.Exec(t.Context(), grant+runtime); err != nil {
			t.Fatal(err)
		}
	}
	role := pgx.Identifier{fx.f.RuntimeRole + "_meter_eraser"}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE ON SCHEMA public TO "+role+";GRANT SELECT,UPDATE ON organizations,accounts,org_role_assignments,org_teams,org_team_links,org_ownership_transfers,org_device_enrollments TO "+role+";GRANT SELECT ON org_employee_identities,payment_billing_exits TO "+role+";GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices,org_ownership_notices,org_invitation_notices,org_invitation_acceptance_notices TO "+role+";GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+role+";GRANT SELECT,UPDATE ON org_invitations,org_invitation_acceptances TO "+role+";ALTER TABLE org_memberships OWNER TO "+role+";ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role+";ALTER FUNCTION org_membership_check_account_exit(uuid) OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := fx.f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	callerScope := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, callerScope); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, fx.person.auth.Account.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
			return err
		}
		var restored string
		if err := tx.QueryRow(t.Context(), `SELECT current_setting('anpfuel.org_id',true)`).Scan(&restored); err != nil {
			return err
		}
		if restored != callerScope {
			t.Error("personal erasure leaked company scope into caller")
		}
		return nil
	}); err != nil {
		t.Fatal("ordinary actual personal erasure", err)
	}

	if snapshot() != before {
		t.Fatal("personal erasure rewrote original held facts/ACK/audit")
	}
	var ended bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT p.state='ENDED' AND p.account_id IS NULL AND e.state='REVOKED' FROM org_memberships p JOIN org_device_enrollments e ON(e.org_id,e.membership_id)=(p.org_id,p.id) WHERE p.org_id=$1 AND p.id=$2 AND e.id=$3`, fx.scope.OrgID().String(), fx.person.member, fx.source.EnrollmentID).Scan(&ended); err != nil || !ended {
		t.Fatal("actual author source was not reduced", err)
	}
	if rows, err := store.OwnMeterReviewResults(t.Context(), fx.source, []protocol.ReviewSelector{{OperationID: op.ID(), PayloadDigest: op.PayloadDigest()}}); !errors.Is(err, syncapp.ErrSourceDenied) || len(rows) != 0 {
		t.Fatal("ended author retained private result access", err)
	}
	decision, _ := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: ref.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
	resolved, _, err := store.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision)
	if err != nil || resolved.Snapshot().Outcome != protocol.ReviewAccepted {
		t.Fatal("real reviewer could not apply original fact after source ended", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT r.author_membership_id=$3::uuid AND r.enrollment_id=$4::uuid AND r.received_at=a.received_at AND a.canonical_ack=$5::bytea AND a.result='ACCEPTED_FOR_REVIEW' FROM org_meter_readings r JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id)=(r.org_id,r.operation_id) WHERE r.org_id=$1 AND r.operation_id=$2`, fx.scope.OrgID().String(), op.ID(), fx.person.member, fx.source.EnrollmentID, ack.CanonicalBytes()).Scan(&exact); err != nil || !exact {
		t.Fatal("reviewed branch required fake original actor or retagged immutable source", err)
	}
}
