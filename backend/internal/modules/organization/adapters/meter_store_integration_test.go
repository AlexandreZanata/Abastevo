//go:build integration

package adapters_test

import (
	"errors"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func meterStore(t *testing.T, fx legalFixtureState) *fleetpg.MeterStore {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_meters,org_meter_segments,org_meter_readings,org_meter_commands TO "+role+";GRANT UPDATE(id) ON org_meters TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &fleetpg.MeterStore{Pool: fx.f.Runtime, Authority: &orgpg.MeterAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func meterInstallInput(t *testing.T, asset string, at time.Time) app.MeterInstallInput {
	t.Helper()
	value, err := meter.NewValue(meter.Odometer, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return app.MeterInstallInput{OperationID: onboardingID(t), AssetID: asset, OpeningReadingID: onboardingID(t), ExpectedAssetRevision: 1, Kind: meter.Odometer, Label: "Synthetic observed odometer", Value: value, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}, Method: meter.Typed}
}
func meterInstallCommand(t *testing.T, v app.MeterInstallInput) app.MeterInstall {
	t.Helper()
	command, err := app.NewMeterInstall(v)
	if err != nil {
		t.Fatal("invalid synthetic meter input", err)
	}
	return command
}
func meterInstall(t *testing.T, fx legalFixtureState, s *fleetpg.MeterStore, c app.MeterInstall) (app.MeterInstallResult, bool, error) {
	t.Helper()
	return s.InstallMeter(t.Context(), fx.scope, fleetActor(fx), fx.member, onboardingID(t), onboardingID(t), c)
}
func TestMeterStoreInstallsObservedOpeningAndRetainsWebCapture(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	s := meterStore(t, fx)
	observed := time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	input := meterInstallInput(t, created.AssetID, observed)
	c := meterInstallCommand(t, input)
	r, replayed, err := meterInstall(t, fx, s, c)
	if err != nil || replayed || r.AssetID != created.AssetID || r.OpeningReadingID != input.OpeningReadingID || r.AssetRevision != 2 {
		t.Fatal("real observed installation", err)
	}
	view, err := s.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, r.MeterID)
	if err != nil || view.Latest == nil || !view.InstalledAt.Equal(observed) {
		t.Fatal("actual meter/latest unavailable or time narrowed", err)
	}
	capture := view.Latest.CaptureSource()
	evidence := view.Latest.Snapshot()
	if capture.Kind() != meter.CaptureWebSession || capture.ID() != fx.actor.SessionID || capture.OrganizationID() != fx.scope.OrgID() || evidence.Author.ID() != fx.member || evidence.Operation.ID() != input.OperationID || evidence.Reference.ID() != input.OpeningReadingID || evidence.Value.Count() != 0 || !evidence.Declared.OccurredAt.Equal(observed) || evidence.ReceivedAt.IsZero() {
		t.Fatal("opening evidence/provenance changed")
	}
	again, replayed, err := meterInstall(t, fx, s, c)
	if err != nil || !replayed || again != r {
		t.Fatal("original installation ACK changed", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meters WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_segments WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_commands WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='meter.installed')=1 AND(SELECT count(*) FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2)=2`, fx.scope.OrgID().String(), created.AssetID).Scan(&exact); err != nil || !exact {
		t.Fatal("installation effects/audit not exactly once", err)
	}
}
func TestMeterStorePreservesExactNineDigitDeclarationBounds(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	s := meterStore(t, fx)
	for _, raw := range []string{"0001-01-01T00:00:00Z", "0001-01-01T00:00:00.000000001Z", "9999-12-31T23:59:59.999999999Z", "2026-11-01T00:00:00.123456789-04:00", "2026-11-02T03:04:05.999999999Z"} {
		t.Run(raw, func(t *testing.T) {
			at, err := orgtime.ParseInstant(raw)
			if err != nil {
				t.Fatal(err)
			}
			created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetCreateInput()))
			if err != nil {
				t.Fatal(err)
			}
			input := meterInstallInput(t, created.AssetID, at)
			r, _, err := meterInstall(t, fx, s, meterInstallCommand(t, input))
			if err != nil {
				t.Fatal("exact bounded declaration rejected", err)
			}
			view, err := s.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, r.MeterID)
			if err != nil || !view.InstalledAt.Equal(at) {
				t.Fatal("installation time rounded across service boundary", err)
			}
			if at.Before(time.Now().UTC()) {
				if view.Latest == nil || !view.Latest.Snapshot().Declared.OccurredAt.Equal(at) {
					t.Fatal("persisted observation precision changed")
				}
			} else if view.Latest == nil || !view.Latest.Snapshot().Declared.OccurredAt.Equal(at) {
				t.Fatal("latest VALID evidence was hidden by an undocumented server-now cutoff")
			}
		})
	}
}

func TestMeterActualSourceActivatesPoweredAssetAndRejectsKnownAbsence(t *testing.T) {
	fx := legalFixture(t)
	assets := fleetStore(t, fx)
	s := meterStore(t, fx)
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
	created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = fleetWrite(t, fx, assets, fleetCommand(t, app.AssetWriteInput{Kind: app.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1}))
	var missing *fleet.ActivationError
	if !errors.As(err, &missing) || len(missing.Missing()) != 1 || missing.Missing()[0] != fleet.MissingMeter {
		t.Fatal("real available-but-empty meter source not reported precisely", err)
	}
	installed, _, err := meterInstall(t, fx, s, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := fleetWrite(t, fx, assets, fleetCommand(t, app.AssetWriteInput{Kind: app.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision}))
	if err != nil || active.State != fleet.Active {
		t.Fatal("actual observed in-service meter did not activate powered asset", err)
	}
}

func TestMeterSchemaDefersActiveTankCompletenessToWholeCommit(t *testing.T) {
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
	installed, _, err := meterInstall(t, fx, store, meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx, assets, fleetCommand(t, app.AssetWriteInput{Kind: app.ActivateAsset, AssetID: asset.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal(err)
	}
	fleetSQLCode(t, fleetSQL(t, fx, `DELETE FROM org_asset_tanks WHERE org_id=$1 AND asset_id=$2`, fx.scope.OrgID().String(), asset.AssetID), "23514")
	var intact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)=1 FROM org_asset_tanks WHERE org_id=$1 AND asset_id=$2`, fx.scope.OrgID().String(), asset.AssetID).Scan(&intact); err != nil || !intact {
		t.Fatal("invalid ACTIVE tank removal committed", err)
	}
}
