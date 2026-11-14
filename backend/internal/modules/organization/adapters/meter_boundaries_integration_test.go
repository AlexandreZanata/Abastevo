//go:build integration

package adapters_test

import (
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
	"time"
)

func TestMeterTwoActualEnrolledDevicesKeepDistinctEqualObservations(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	_, actor := deviceAddSigner(t, fx.deviceFixtureState, fx.person)
	enrolled, err := fx.devices.EnrollDevice(t.Context(), fx.scope, actor, deviceEnrollCommand(t, "Synthetic second actual device"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := enrolled.Device.Enrollment.Snapshot(fx.scope)
	if err != nil {
		t.Fatal(err)
	}
	second := fx.source
	second.Actor = actor
	second.EnrollmentID = snapshot.ID
	if second.EnrollmentID == fx.source.EnrollmentID {
		t.Fatal("fixture did not enroll a distinct real key")
	}
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Second).Add(999999999 * time.Nanosecond)
	commands := []app.MeterReadingCommand{meterObservedCommand(t, installed, at, 17), meterObservedCommand(t, installed, at, 17)}
	sources := []syncapp.Source{fx.source, second}
	start := make(chan struct{})
	type answer struct {
		r      app.MeterReadingResult
		err    error
		source int
	}
	out := make(chan answer, 2)
	var wg sync.WaitGroup
	for index := range 2 {
		wg.Go(func() {
			<-start
			r, _, err := meterRecord(t, fx, store, sources[index], commands[index])
			out <- answer{r, err, index}
		})
	}
	close(start)
	wg.Wait()
	close(out)
	for result := range out {
		if result.err != nil {
			t.Fatal("concurrent actual device evidence", result.err)
		}
		capture := result.r.Reading.CaptureSource()
		if capture.ID() != sources[result.source].EnrollmentID || result.r.Reading.Snapshot().Value.Count() != 17 || !result.r.Reading.Snapshot().Declared.OccurredAt.Equal(at) {
			t.Fatal("concurrent source or exact observation changed")
		}
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND count(DISTINCT enrollment_id)=2 FROM org_meter_readings WHERE org_id=$1 AND origin='STANDALONE'`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("equal values or devices were deduplicated", err)
	}
}
func TestMeterLatestFutureEvidenceDoesNotInventCurrentService(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	store := meterStore(t, fx)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.Profile.Kind = fleet.Vehicle
	input.Profile.UsageBasis = fleet.Distance
	input.UnitID = unit
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	asset, _, err := fleetWrite(t, fx, assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second).Add(999999999 * time.Nanosecond)
	installed, _, err := meterInstall(t, fx, store, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, future)))
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx), fx.member, asset.AssetID, installed.MeterID)
	if err != nil || view.Latest == nil || !view.Latest.Snapshot().Declared.OccurredAt.Equal(future) {
		t.Fatal("future VALID evidence hidden or retagged", err)
	}
	_, _, err = fleetWrite(t, fx, assets, fleetCommand(t, app.AssetWriteInput{Kind: app.ActivateAsset, AssetID: asset.AssetID, ExpectedRevision: installed.AssetRevision}))
	var missing *fleet.ActivationError
	if !errors.As(err, &missing) || len(missing.Missing()) != 1 || missing.Missing()[0] != fleet.MissingMeter {
		t.Fatal("future opening invented current meter availability", err)
	}
	var unchanged bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT state='DRAFT' AND revision=2 AND NOT EXISTS(SELECT 1 FROM org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id) FROM org_assets a WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), asset.AssetID).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("future service activated operational state", err)
	}
}
func TestMeterCompositeReferencesAndReadingLookupsStayPrivate(t *testing.T) {
	fx, store, installed := meterEvidenceFixture(t)
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	scope := scopeFor(t, other.OrgID.String())
	if _, err := fx.flags.Set(t.Context(), scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadLatestMeter(t.Context(), scope, fleetActor(fx.legalFixtureState), other.MembershipID, installed.AssetID, installed.MeterID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign meter disclosed", err)
	}
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, scope, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings`).Scan(&n); err != nil || n != 0 {
			t.Fatal("cross-org forced-RLS visibility", err)
		}
		if _, err := store.ReadMeterReadingTx(t.Context(), tx, scope, installed.AssetID, installed.MeterID, installed.SegmentID, installed.OpeningReadingID); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("foreign original reading lookup disclosed", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
