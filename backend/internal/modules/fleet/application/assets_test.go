package application_test

import (
	"errors"
	"fmt"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

func id(n int) string { return fmt.Sprintf("ad000000-0000-4000-8000-%012d", n) }
func scope(t *testing.T) kernel.OrgScope {
	t.Helper()
	org, e := kernel.ParseOrgID(id(1))
	if e != nil {
		t.Fatal(e)
	}
	s, e := kernel.NewOrgScope(org)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func input(t *testing.T) app.AssetWriteInput {
	t.Helper()
	tank, e := fleet.NewTank(fuel.Catalog{}, fleet.Main, "275000", []string{"DIESEL_S10"})
	if e != nil {
		t.Fatal(e)
	}
	expected, e := fleet.NewExpectedConsumption(fuel.Catalog{}, "DIESEL_S10", fleet.Distance, "8500", "Synthetic manufacturer", "2026-11-14")
	if e != nil {
		t.Fatal(e)
	}
	return app.AssetWriteInput{Kind: app.CreateAsset, OperationID: id(10), Category: app.CategoryChoice{NewLabel: "Caminhão rígido"}, Profile: fleet.Profile{Kind: fleet.Vehicle, Name: "Synthetic truck", FleetNumber: " F-01 ", Plate: "abc-1d23", SerialNumber: "S-001", UsageBasis: fleet.Distance, Tanks: []fleet.Tank{tank}, Expected: []fleet.ExpectedConsumption{expected}}}
}
func TestAssetCommandCapturesExplicitCategoryAndIntent(t *testing.T) {
	s := scope(t)
	v := input(t)
	command, e := app.NewAssetWrite(v)
	if e != nil {
		t.Fatal(e)
	}
	snapshot, e := command.Snapshot()
	if e != nil || snapshot.Category.NewLabel != "Caminhão rígido" || snapshot.Profile.Category != "" {
		t.Fatal("category intent changed", e)
	}
	hash := command.Digest(s)
	if len(hash) != 64 {
		t.Fatal("missing immutable intent hash")
	}
	v.Profile.Tanks[0] = fleet.Tank{}
	v.Profile.Expected[0] = fleet.ExpectedConsumption{}
	v.Category.NewLabel = "changed"
	if command.Digest(s) != hash || snapshot.Profile.Tanks[0].Snapshot().Capacity != 275000 {
		t.Fatal("command input escaped")
	}
	snapshot.Profile.Tanks[0] = fleet.Tank{}
	if command.Digest(s) != hash {
		t.Fatal("command snapshot escaped")
	}
}
func TestAssetCommandShapeAndFilters(t *testing.T) {
	for _, v := range []app.CategoryChoice{{NewLabel: "Categoria"}, {ExistingID: id(30)}} {
		if e := v.Validate(); e != nil {
			t.Fatal("explicit category alternative refused", e)
		}
	}
	for _, v := range []app.CategoryChoice{{}, {ExistingID: id(30), NewLabel: "Categoria"}, {NewLabel: "\u00a0"}, {NewLabel: "Bad\nname"}, {ExistingID: "invalid"}} {
		if !errors.Is(v.Validate(), fleet.ErrInvalid) {
			t.Fatal("ambiguous/invalid category accepted")
		}
	}
	for _, f := range []app.AssetFilter{{Limit: 1}, {Limit: 100, AfterID: id(40), State: fleet.Archived, Kind: fleet.Implement}} {
		if e := f.Validate(); e != nil {
			t.Fatal("valid bounded filter refused", e)
		}
	}
}
func TestForgedDecisionCannotReachFleetStorage(t *testing.T) {
	s := scope(t)
	actor := app.FleetActor{AccountID: id(2), SessionID: id(3)}
	command, e := app.NewAssetWrite(input(t))
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = (app.AssetService{Enabled: true}).Write(t.Context(), s, actor, access.Decision{}, command)
	if !errors.Is(e, app.ErrUnauthenticated) {
		t.Fatal("forged decision did not fail current proof", e)
	}
	if _, _, e = (app.AssetService{}).Write(t.Context(), s, actor, access.Decision{}, command); !errors.Is(e, app.ErrSurfaceDisabled) {
		t.Fatal("default surface enabled", e)
	}
}
