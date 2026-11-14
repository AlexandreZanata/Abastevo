package http

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func projectionScope(t *testing.T, raw string) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(raw)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
func TestAssetProjectionPreservesExactValuesAndActualAllocation(t *testing.T) {
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	id, _ := d.ParseAssetID("7b0e2a54-0000-4000-8000-000000008201")
	ref, _ := d.NewAssetRef(scope, id)
	profile, err := (assetFields{Kind: d.Vehicle, Name: "Synthetic source", Plate: "abc-1d23", UsageBasis: d.Distance, Tanks: []tankFields{{Role: d.Main, CapacityMilli: "275000", Products: []string{"DIESEL_S10"}}}, ExpectedConsumption: []expectedFields{{Product: "DIESEL_S10", Basis: d.Distance, Value: "9007199254740993", Source: "Synthetic source", Date: "2026-11-14"}}}).profile()
	if err != nil {
		t.Fatal(err)
	}
	profile.Category = "7b0e2a54-0000-4000-8000-000000008301"
	value := app.AssetView{Ref: ref, Profile: profile, CategoryLabel: "Synthetic declared category", State: d.Active, Revision: 2, UnitID: "7b0e2a54-0000-4000-8000-000000000801", Allocation: &app.UnitAllocation{UnitID: "7b0e2a54-0000-4000-8000-000000000801", StartedAt: time.Date(2026, 11, 14, 12, 0, 0, 123000000, time.UTC)}}
	wire, err := assetView(scope, value)
	if err != nil {
		t.Fatal("valid source projection refused", err)
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if json.Unmarshal(raw, &roundtrip) != nil {
		t.Fatal("invalid projected JSON")
	}
	if roundtrip["plate"] != "abc-1d23" || roundtrip["tanks"].([]any)[0].(map[string]any)["capacity_milli"] != "275000" || roundtrip["expected_consumption"].([]any)[0].(map[string]any)["value"] != "9007199254740993" || roundtrip["initial_allocation"].(map[string]any)["started_at"] != "2026-11-14T12:00:00.123Z" {
		t.Fatal("raw source or exact allocation projection changed")
	}
	foreign := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008002")
	if _, err := assetView(foreign, value); err != orgapi.Unavailable {
		t.Fatal("foreign private projection published")
	}
	for name, change := range map[string]func(*app.AssetView){
		"active missing allocation": func(v *app.AssetView) { v.Allocation = nil },
		"active missing unit":       func(v *app.AssetView) { v.UnitID = ""; v.Allocation = nil },
		"unknown kind":              func(v *app.AssetView) { v.Profile.Kind = "GUESS" },
		"invalid category":          func(v *app.AssetView) { v.Profile.Category = "00000000-0000-0000-0000-000000000000" },
		"invalid category label":    func(v *app.AssetView) { v.CategoryLabel = "\x00Synthetic" },
		"mismatched allocation": func(v *app.AssetView) {
			v.Allocation = &app.UnitAllocation{UnitID: "7b0e2a54-0000-4000-8000-000000000802", StartedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := value
			change(&copy)
			if _, err := assetView(scope, copy); err != orgapi.Unavailable {
				t.Fatal("unproven source projection published")
			}
		})
	}
}

func TestAssetDiscardReceiptHasNoInventedLifecycle(t *testing.T) {
	value := app.AssetResult{AssetID: "7b0e2a54-0000-4000-8000-000000008201", Revision: 3, Discarded: true}
	wire, err := assetAck(value, true)
	if err != nil || wire["discarded"] != true || wire["replayed"] != true {
		t.Fatal("discard metadata lost")
	}
	if _, exists := wire["state"]; exists {
		t.Fatal("discard invented lifecycle")
	}
	value.State = d.Archived
	if _, err := assetAck(value, false); err != orgapi.Unavailable {
		t.Fatal("discard accepted invented lifecycle")
	}
	value.State = ""
	value.AssetID = ""
	if _, err := assetAck(value, false); err != orgapi.Unavailable {
		t.Fatal("invalid receipt selector published")
	}
	// An actual pure activation checklist maps its supplied facts only.
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	id, _ := d.ParseAssetID("7b0e2a54-0000-4000-8000-000000008201")
	ref, _ := d.NewAssetRef(scope, id)
	asset, err := d.NewDraft(ref, d.Profile{Kind: d.Vehicle, Name: "Synthetic", Plate: "ABC1D23", Category: "source-category", UsageBasis: d.Distance}, projectionCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	missing := asset.CheckActivation(scope, d.ActivationFacts{})
	if !errors.Is(assetMapped(missing), orgapi.ValidationFailed) || assetMapped(d.ErrIncomplete) != orgapi.Unavailable {
		t.Fatal("missing facts confused with provider absence")
	}
}

type projectionCatalog struct{}

func (projectionCatalog) Category(code string) (d.CategoryDescriptor, bool) {
	return d.CategoryDescriptor{Code: code}, code == "source-category"
}
func (projectionCatalog) Product(string) (d.ProductDescriptor, bool) {
	return d.ProductDescriptor{}, false
}
