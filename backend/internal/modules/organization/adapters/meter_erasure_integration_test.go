//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cloudpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	cloudapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/jackc/pgx/v5"
)

func TestMeterPersonalErasurePreservesCompanyEvidenceAndExportBoundary(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	observation := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 17)
	recorded, replay, err := meterRecord(t, fx, store, fx.source, observation)
	if err != nil || replay {
		t.Fatal("actual enrolled source observation before erasure", err)
	}
	deviceReadingID := recorded.Reading.Snapshot().Reference.ID()
	if recorded.Reading.Snapshot().Author.ID() != fx.person.member || recorded.Reading.CaptureSource().ID() != fx.source.EnrollmentID || recorded.Reading.CaptureSource().Kind() != meter.CaptureEnrolledDevice {
		t.Fatal("fixture did not retain actual enrolled author/source")
	}
	// The erased account also creates a real WEB opening. A second kind on the
	// same asset avoids a synthetic provenance row or a second current meter of
	// one kind, and its command/audit must survive account unlinking unchanged.
	input := meterInstallInput(t, installed.AssetID, time.Now().UTC().Add(-2*time.Hour))
	input.ExpectedAssetRevision = 2
	input.Kind = meter.HourMeter
	input.Label = "Synthetic company meter privacy boundary"
	input.Value, err = meter.NewValue(meter.HourMeter, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	actor := app.FleetActor{AccountID: fx.person.web.AccountID, SessionID: fx.person.web.SessionID}
	web, replay, err := store.InstallMeter(t.Context(), fx.scope, actor, fx.person.member, onboardingID(t), onboardingID(t), meterInstallCommand(t, input))
	if err != nil || replay {
		t.Fatal("actual same-account WEB opening before erasure", err)
	}
	view, err := store.ReadLatestMeter(t.Context(), fx.scope, actor, fx.person.member, installed.AssetID, web.MeterID)
	if err != nil || view.Latest == nil || view.Latest.Snapshot().Author.ID() != fx.person.member || view.Latest.CaptureSource().Kind() != meter.CaptureWebSession || view.Latest.CaptureSource().ID() != actor.SessionID {
		t.Fatal("fixture did not retain actual WEB author/source", err)
	}

	snapshot := func() string {
		t.Helper()
		var value string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM org_assets x WHERE org_id=$1),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY asset_id,revision)FROM org_asset_revisions x WHERE org_id=$1),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM org_meters x WHERE org_id=$1),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM org_meter_segments x WHERE org_id=$1),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY id)FROM org_meter_readings x WHERE org_id=$1),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY operation_id)FROM org_meter_commands x WHERE org_id=$1),
 (SELECT jsonb_agg(to_jsonb(x)ORDER BY operation_id)FROM org_audit_events x WHERE org_id=$1))::text`, fx.scope.OrgID().String()).Scan(&value); err != nil {
			t.Fatal("company evidence snapshot", err)
		}
		return value
	}
	before := snapshot()
	runtime := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON cloud_records TO "+runtime); err != nil {
		t.Fatal(err)
	}
	personalVehicle := onboardingID(t)
	if _, err := fx.f.Admin.Exec(t.Context(), `INSERT INTO cloud_records(account_id,dataset,id,payload)VALUES($1,'vehicles',$2,'{"name":"Synthetic personal vehicle"}')`, fx.person.auth.Account.ID, personalVehicle); err != nil {
		t.Fatal(err)
	}
	personal := cloudpg.NewStore(fx.f.Runtime)
	principal := cloudapp.Principal{AccountID: fx.person.auth.Account.ID, FamilyID: fx.person.auth.Session.FamilyID}
	rows, err := personal.List(t.Context(), principal, "vehicles", 100, "")
	if err != nil || len(rows) != 1 || rows[0].ID != personalVehicle {
		t.Fatal("personal inventory control unavailable", err)
	}
	for _, dataset := range []string{"org_assets", "org_asset_revisions", "org_meters", "org_meter_segments", "org_meter_readings", "org_meter_commands", "org_audit_events", "enterprise"} {
		if rows, err := personal.List(t.Context(), principal, dataset, 100, ""); !errors.Is(err, cloud.ErrInvalid) || len(rows) != 0 {
			t.Fatal("fixed personal export admitted company dataset")
		}
	}
	privateIDs := map[string]bool{installed.AssetID: true, installed.MeterID: true, installed.SegmentID: true, installed.OpeningReadingID: true, web.MeterID: true, web.SegmentID: true, web.OpeningReadingID: true, deviceReadingID: true, input.OperationID: true}
	for _, dataset := range cloud.Datasets() {
		rows, err := personal.List(t.Context(), principal, dataset, 100, "")
		if err != nil {
			t.Fatal("actual personal dataset export", err)
		}
		for _, row := range rows {
			if privateIDs[row.ID] || strings.Contains(string(row.Payload), input.Label) {
				t.Fatal("company meter evidence entered personal export")
			}
			for id := range privateIDs {
				if strings.Contains(string(row.Payload), id) {
					t.Fatal("company locator entered personal export")
				}
			}
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
		t.Fatal("personal erasure rewrote company meter/command/audit history")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM org_meters WHERE org_id=$1)=2
 AND(SELECT count(*) FROM org_meter_segments WHERE org_id=$1)=2
 AND(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=3
 AND(SELECT count(*) FROM org_meter_commands WHERE org_id=$1)=2
 AND EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND id=$2 AND author_membership_id=$3 AND capture_kind='ENROLLED_DEVICE' AND enrollment_id=$4)
 AND EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND id=$5 AND author_membership_id=$3 AND capture_kind='WEB_SESSION' AND web_session_id=$6)
 AND EXISTS(SELECT 1 FROM org_memberships m JOIN org_device_enrollments d ON(d.org_id,d.membership_id)=(m.org_id,m.id) WHERE m.org_id=$1 AND m.id=$3 AND d.id=$4 AND m.state='ENDED' AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND d.state='REVOKED' AND d.revocation_class='ACCESS_ENDED')
 AND NOT EXISTS(SELECT 1 FROM cloud_records WHERE account_id=$7 AND payload IS NOT NULL)`, fx.scope.OrgID().String(), deviceReadingID, fx.person.member, fx.source.EnrollmentID, web.OpeningReadingID, actor.SessionID, actor.AccountID).Scan(&exact); err != nil || !exact {
		t.Fatal("erasure failed source reduction or retained historical evidence", err)
	}
	if _, _, err := meterRecord(t, fx, store, fx.source, observation); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("unlinked enrolled source still received retained private evidence", err)
	}
	if _, err := store.ReadLatestMeter(t.Context(), fx.scope, actor, fx.person.member, installed.AssetID, web.MeterID); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("unlinked WEB source still read retained private evidence", err)
	}
	if snapshot() != before {
		t.Fatal("refused erased source changed retained history")
	}
}
