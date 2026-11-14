//go:build integration

package adapters_test

import (
	"errors"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
	"time"
)

func meterEvidenceFixture(t *testing.T) (syncFixtureState, *fleetpg.MeterStore, app.MeterInstallResult) {
	t.Helper()
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx.legalFixtureState)
	store.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	installed, _, err := meterInstall(t, fx.legalFixtureState, store, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-4*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	return fx, store, installed
}
func meterObservedCommand(t *testing.T, installed app.MeterInstallResult, at time.Time, value int64) app.MeterReadingCommand {
	t.Helper()
	v, err := meter.NewValue(meter.Odometer, value, nil)
	if err != nil {
		t.Fatal(err)
	}
	command, err := app.NewMeterReadingCommand(app.MeterReadingInput{OperationID: onboardingID(t), AssetID: installed.AssetID, MeterID: installed.MeterID, SegmentID: installed.SegmentID, ReadingID: onboardingID(t), Value: v, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}, Origin: meter.Standalone, Method: meter.Typed})
	if err != nil {
		t.Fatal(err)
	}
	return command
}
func meterRecord(t *testing.T, fx syncFixtureState, store *fleetpg.MeterStore, source syncapp.Source, command app.MeterReadingCommand) (app.MeterReadingResult, bool, error) {
	t.Helper()
	var result app.MeterReadingResult
	var replay bool
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var err error
		result, replay, err = store.RecordEmbedded(t.Context(), syncapp.Borrow(tx), fx.scope, app.MeterSource{Device: &source}, source.MembershipID, command)
		return err
	})
	if err != nil {
		return app.MeterReadingResult{}, false, err
	}
	return result, replay, nil
}
func TestMeterEmbeddedRollbackReplayAndNanosecondOrdering(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	command := meterObservedCommand(t, installed, at, 7)
	rollback := errors.New("synthetic parent rollback")
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, _, err := store.RecordEmbedded(t.Context(), syncapp.Borrow(tx), fx.scope, app.MeterSource{Device: &fx.source}, fx.source.MembershipID, command); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("embedded parent failure changed", err)
	}
	var count int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&count); err != nil || count != 1 {
		t.Fatal("embedded evidence committed outside parent", err)
	}
	var original meter.Reading
	for i := range 3 {
		result, replay, err := meterRecord(t, fx, store, fx.source, command)
		if err != nil || replay != (i > 0) {
			t.Fatal("embedded original evidence replay", err)
		}
		if i == 0 {
			original = result.Reading
		} else if !result.Reading.Snapshot().ReceivedAt.Equal(original.Snapshot().ReceivedAt) {
			t.Fatal("replay replaced original receipt instant")
		}
	}
	for _, c := range []app.MeterReadingCommand{meterObservedCommand(t, installed, at.Add(-time.Nanosecond), 7), meterObservedCommand(t, installed, at.Add(-time.Second), 7)} {
		if _, _, err := meterRecord(t, fx, store, fx.source, c); err != nil {
			t.Fatal("equal-valued distinct observation discarded", err)
		}
	}
	latest, err := store.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, installed.AssetID, installed.MeterID)
	if err != nil || latest.Latest == nil || latest.Latest.Snapshot().Reference.ID() != original.Snapshot().Reference.ID() || !latest.Latest.Snapshot().Declared.OccurredAt.Equal(at) {
		t.Fatal("arrival or rounded microsecond replaced actual observation order", err)
	}
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&count); err != nil || count != 4 {
		t.Fatal("reading identity deduplicated equal values", err)
	}
}
func TestMeterSchemaProtectsHistoryFromOrdinaryMaintenanceAndParentDeletion(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	asset, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx)
	installed, _, err := meterInstall(t, fx, store, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_meters,org_meter_segments,org_meter_readings,org_meter_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE org_meter_readings SET value=value+1 WHERE org_id=$1`, `DELETE FROM org_meter_readings WHERE org_id=$1`, `DELETE FROM org_meters WHERE org_id=$1`, `UPDATE org_meter_segments SET opening_reading_id=opening_reading_id WHERE org_id=$1`, `DELETE FROM org_meter_commands WHERE org_id=$1`} {
		fleetSQLCode(t, fleetSQL(t, fx, sql, fx.scope.OrgID().String()), "55000")
	}
	fleetSQLCode(t, fleetSQL(t, fx, `DELETE FROM org_assets WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), installed.AssetID), "P7903")
	if _, _, err := fleetWrite(t, fx, assets, fleetCommand(t, app.AssetWriteInput{Kind: app.DiscardAsset, AssetID: installed.AssetID, ExpectedRevision: 2})); !errors.Is(err, app.ErrConflict) {
		t.Fatal("opening reading did not make draft used", err)
	}
	var n int
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings`).Scan(&n); err != nil || n != 0 {
		t.Fatal("unscoped observation read escaped forced RLS", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meters WHERE org_id=$1)=1`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("immutable parent/history damaged", err)
	}
}
func TestMeterInstallationLateCommitFailureIsUnavailableAndAtomic(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	asset, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_meter_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='synthetic publication failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_meter_commit_failure AFTER INSERT ON org_meter_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_meter_commit_failure()`); err != nil {
		t.Fatal(err)
	}
	result, replay, err := meterInstall(t, fx, store, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-time.Hour))))
	if !errors.Is(err, app.ErrUnavailable) || replay || result.MeterID != "" {
		t.Fatal("internal commit failure exposed client validation or ACK", err)
	}
	var clean bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_meters WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_meter_commands WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND action='meter.installed') AND(SELECT revision=1 FROM org_assets WHERE org_id=$1 AND id=$2)`, fx.scope.OrgID().String(), asset.AssetID).Scan(&clean); err != nil || !clean {
		t.Fatal("late failure left partial meter/asset/audit effects", err)
	}
}
func TestMeterConcurrentOriginalRecordsRemainOnce(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	command := meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Minute), 12)
	start := make(chan struct{})
	type outcome struct {
		r   app.MeterReadingResult
		err error
	}
	out := make(chan outcome, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { <-start; r, _, err := meterRecord(t, fx, store, fx.source, command); out <- outcome{r, err} })
	}
	close(start)
	wg.Wait()
	close(out)
	var first meter.Reading
	for result := range out {
		if result.err != nil {
			t.Fatal("concurrent same-meter receipt", result.err)
		}
		if first.Snapshot().Reference.ID() == "" {
			first = result.r.Reading
		} else if !first.Snapshot().ReceivedAt.Equal(result.r.Reading.Snapshot().ReceivedAt) {
			t.Fatal("concurrent replay acknowledged different evidence")
		}
	}
	var count int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&count); err != nil || count != 2 {
		t.Fatal("concurrent replay duplicated immutable reading", err)
	}
}

func TestMeterEmbeddedRejectsActualOneNanosecondBeforeServiceBeforeInsert(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	view, err := store.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, installed.AssetID, installed.MeterID)
	if err != nil {
		t.Fatal(err)
	}
	command := meterObservedCommand(t, installed, view.InstalledAt.Add(-time.Nanosecond), 1)
	result, replay, err := meterRecord(t, fx, store, fx.source, command)
	if !errors.Is(err, meter.ErrInvalid) || replay || result.Reading.Snapshot().Reference.ID() != "" {
		t.Fatal("known invalid service instant reached SQL/publication", err)
	}
	var count int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&count); err != nil || count != 1 {
		t.Fatal("outside-service observation changed history", err)
	}
}

func TestMeterConcurrentInstallationReplaysOriginalAndEnforcesServiceKind(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	asset, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx)
	input := meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-time.Hour))
	command := meterInstallCommand(t, input)
	start := make(chan struct{})
	type answer struct {
		result app.MeterInstallResult
		err    error
	}
	out := make(chan answer, 12)
	var wg sync.WaitGroup
	for range 12 {
		id, segment := onboardingID(t), onboardingID(t)
		wg.Go(func() {
			<-start
			r, _, err := store.InstallMeter(t.Context(), fx.scope, fleetActor(fx), fx.member, id, segment, command)
			out <- answer{r, err}
		})
	}
	close(start)
	wg.Wait()
	close(out)
	var first app.MeterInstallResult
	for result := range out {
		if result.err != nil {
			t.Fatal("concurrent installation replay", result.err)
		}
		if first.MeterID == "" {
			first = result.result
		} else if first != result.result {
			t.Fatal("concurrent install returned different immutable ACK")
		}
	}
	input.OperationID = onboardingID(t)
	input.OpeningReadingID = onboardingID(t)
	input.ExpectedAssetRevision = 2
	if _, _, err := meterInstall(t, fx, store, meterInstallCommand(t, input)); !errors.Is(err, meter.ErrAlreadyInService) {
		t.Fatal("second live meter of same kind accepted", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meters WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_commands WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='meter.installed')=1`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("concurrent installation duplicate effects/audit", err)
	}
}
