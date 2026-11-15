package adapters_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

type absentAuthority struct{}

func (*absentAuthority) CheckFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, app.Target, app.Action) error {
	return nil
}
func (*absentAuthority) ClassifyFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, bool, time.Time) (app.Disposition, error) {
	return app.Apply, nil
}

type fakeReadings struct{}

func (fakeReadings) RecordEmbedded(context.Context, syncapp.DBTX, kernel.OrgScope, fleetapp.MeterSource, string, fleetapp.MeterReadingCommand) (fleetapp.MeterReadingResult, bool, error) {
	return fleetapp.MeterReadingResult{}, false, errors.New("unexpected")
}
func (fakeReadings) ReadMeterTargetTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, string, string) (fleetapp.MeterView, error) {
	return fleetapp.MeterView{}, errors.New("unexpected")
}
func (fakeReadings) ReadMeterReadingTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, string, string, string) (meter.Reading, error) {
	return meter.Reading{}, errors.New("unexpected")
}
func (fakeReadings) MeterAssetDisposalTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (*time.Time, error) {
	return nil, errors.New("unexpected")
}

// The owner registers only with real ports, declares no review receipt and
// the catalog permissions of fuelings §7 (EDR-175).
func TestIntakeHandlerNeedsRealPortsAndDeclaresItsOutcomes(t *testing.T) {
	var nilAuthority *absentAuthority
	for _, authority := range []app.Authority{nil, nilAuthority} {
		if handler, err := fuelpg.NewIntakeHandler(authority, nil, &fuelpg.Store{}); handler != nil || !errors.Is(err, syncapp.ErrUnavailable) {
			t.Fatal("incomplete fueling owner entered the registry")
		}
	}
	if handler, err := fuelpg.NewIntakeHandler(&absentAuthority{}, nil, &fuelpg.Store{}); handler != nil || !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("fueling owner without the fleet reading port")
	}
	var nilStore *fuelpg.Store
	if _, err := nilStore.AuthorEmployeeTx(t.Context(), nil, kernel.OrgScope{}, ""); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store answered")
	}
	if _, err := (&fuelpg.Store{}).PrepareFuelingTx(t.Context(), nil, kernel.OrgScope{}, syncapp.Source{}, app.Command{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without a port prepared")
	}
	handler, err := fuelpg.NewIntakeHandler(&absentAuthority{}, fakeReadings{}, &fuelpg.Store{})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := handler.Descriptor()
	if descriptor.Type != app.OperationType || descriptor.Schema != 1 || descriptor.Validate() != nil || descriptor.WritePolicy.Permission != "fuel.fueling.record" || descriptor.ReadPolicy.Permission != "fuel.fueling.read" ||
		!slices.Equal(descriptor.Results(), []string{protocol.Accepted, protocol.Rejected}) {
		t.Fatal("fueling descriptor drift")
	}
}
