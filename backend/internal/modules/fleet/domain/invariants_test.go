package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"log/slog"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func privateAsset(t *testing.T) (fleet.Asset, fleet.Profile) {
	t.Helper()
	tank, err := fleet.NewTank(catalog{}, fleet.Main, "275000", []string{"LIQUID_A", "LIQUID_B"})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := fleet.NewExpectedConsumption(catalog{}, "LIQUID_A", fleet.Distance, "8500", "Synthetic private maker", "2026-11-01")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := ref(t, "A", "X")
	if err != nil {
		t.Fatal(err)
	}
	profile := fleet.Profile{Kind: fleet.Vehicle, Category: "CAT_A", Name: "Synthetic private asset", FleetNumber: "PRIVATE-FLEET", Plate: "ABC1D23", SerialNumber: "Private serial", UsageBasis: fleet.Distance, Tanks: []fleet.Tank{tank}, Expected: []fleet.ExpectedConsumption{expected}}
	asset, err := fleet.NewDraft(reference, profile, catalog{})
	if err != nil {
		t.Fatal(err)
	}
	return asset, profile
}
func TestPrivateValuesRedactEveryFormattingAndSerializationSurface(t *testing.T) {
	asset, profile := privateAsset(t)
	snapshot, err := asset.Snapshot(scope(t, "A"))
	if err != nil {
		t.Fatal(err)
	}
	unit, err := fleet.NewUnitRef(scope(t, "A"), localID("U"))
	if err != nil {
		t.Fatal(err)
	}
	meter, err := fleet.NewMeterFact(snapshot.Ref, localID("M"), true)
	if err != nil {
		t.Fatal(err)
	}
	values := []any{asset, snapshot, profile, snapshot.Ref, snapshot.Ref.ID(), profile.Tanks[0], profile.Tanks[0].Snapshot(), profile.Expected[0], profile.Expected[0].Snapshot(), unit, meter,
		fleet.ActivationFacts{Unit: &unit, Meters: []fleet.MeterFact{meter}}, fleet.ProductDescriptor{Code: "Private product", Unit: fleet.Liter}, fleet.CategoryDescriptor{Code: "Private category"}}
	private := []string{"Synthetic private", "PRIVATE-FLEET", "ABC1D23", "Private serial", "Private product", "Private category", "7b0e2a54", "8500", "275000"}
	check := func(encoded string) {
		t.Helper()
		if !strings.Contains(encoded, "redacted") {
			t.Fatal("redaction absent")
		}
		for _, value := range private {
			if strings.Contains(encoded, value) {
				t.Fatal("private field escaped through a diagnostic")
			}
		}
	}
	for _, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			check(fmt.Sprintf(verb, value))
		}
		check(fmt.Sprint([]any{value}, map[string]any{"nested": value}))
		encoded, err := json.Marshal(map[string]any{"nested": []any{value}})
		if err != nil {
			t.Fatal(err)
		}
		check(string(encoded))
		var out bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&out, nil))
		logger.Info("synthetic", "value", value, "nested", map[string]any{"value": value})
		check(out.String())
	}
}
func TestConstructionAndSnapshotsDoNotShareMutableCollections(t *testing.T) {
	products := []string{"LIQUID_B", "LIQUID_A"}
	tank, err := fleet.NewTank(catalog{}, fleet.Main, "275000", products)
	if err != nil {
		t.Fatal(err)
	}
	products[0] = "UNKNOWN"
	view := tank.Snapshot()
	view.Products[0] = "UNKNOWN"
	if got := tank.Snapshot().Products; !reflect.DeepEqual(got, []string{"LIQUID_A", "LIQUID_B"}) {
		t.Fatal("mutable product alias")
	}
	asset, original := privateAsset(t)
	original.Tanks[0] = fleet.Tank{}
	original.Expected[0] = fleet.ExpectedConsumption{}
	snapshot, err := asset.Snapshot(scope(t, "A"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Profile.Name = "Changed"
	snapshot.Profile.Tanks[0] = fleet.Tank{}
	snapshot.Profile.Expected[0] = fleet.ExpectedConsumption{}
	fresh, err := asset.Snapshot(scope(t, "A"))
	if err != nil || fresh.Profile.Name != "Synthetic private asset" || fresh.Profile.Tanks[0].Snapshot().Capacity != 275000 || fresh.Profile.Expected[0].Snapshot().Value != 8500 {
		t.Fatal("mutable profile alias")
	}
}
func TestActivationRequiresTypedCurrentFactsAndDoesNotChangeDraft(t *testing.T) {
	asset, _ := privateAsset(t)
	snapshot, _ := asset.Snapshot(scope(t, "A"))
	unit, _ := fleet.NewUnitRef(scope(t, "A"), localID("U"))
	meter, _ := fleet.NewMeterFact(snapshot.Ref, localID("M"), true)
	if err := asset.CheckActivation(scope(t, "A"), fleet.ActivationFacts{Unit: &unit, Meters: []fleet.MeterFact{meter}}); err != nil {
		t.Fatal(err)
	}
	after, _ := asset.Snapshot(scope(t, "A"))
	if after.State != fleet.Draft {
		t.Fatal("pure check changed lifecycle")
	}
	if err := asset.CheckActivation(scope(t, "B"), fleet.ActivationFacts{}); !errors.Is(err, fleet.ErrScope) {
		t.Fatal("foreign scope accepted")
	}
	if _, err := asset.Snapshot(scope(t, "B")); !errors.Is(err, fleet.ErrScope) {
		t.Fatal("foreign snapshot accepted")
	}
	zeroUnit := fleet.UnitRef{}
	if err := asset.CheckActivation(scope(t, "A"), fleet.ActivationFacts{Unit: &zeroUnit}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero unit accepted")
	}
	if err := asset.CheckActivation(scope(t, "A"), fleet.ActivationFacts{Unit: &unit, Meters: []fleet.MeterFact{{}}}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero meter accepted")
	}
	err := asset.CheckActivation(scope(t, "A"), fleet.ActivationFacts{})
	var missing *fleet.ActivationError
	if !errors.As(err, &missing) {
		t.Fatal("missing checklist absent")
	}
	list := missing.Missing()
	list[0] = fleet.Missing("REPLACED")
	if !reflect.DeepEqual(missing.Missing(), []fleet.Missing{fleet.MissingUnit, fleet.MissingMeter}) {
		t.Fatal("mutable checklist alias")
	}
}
func TestReferenceTypesRejectZeroAndNonCanonicalUUIDs(t *testing.T) {
	for _, raw := range []string{"", "personal-vehicle", "00000000-0000-0000-0000-000000000000", "7B0E2A54-0000-4000-8000-000000000011"} {
		if _, err := fleet.ParseAssetID(raw); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid asset identifier accepted")
		}
	}
	id, _ := assetID("X")
	if _, err := fleet.NewAssetRef(kernel.OrgScope{}, id); !errors.Is(err, fleet.ErrScope) {
		t.Fatal("zero scope accepted")
	}
	if _, err := fleet.NewAssetRef(scope(t, "A"), fleet.AssetID{}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero id accepted")
	}
	if _, err := fleet.NewUnitRef(kernel.OrgScope{}, localID("U")); !errors.Is(err, fleet.ErrScope) {
		t.Fatal("zero unit scope accepted")
	}
	if _, err := fleet.NewMeterFact(fleet.AssetRef{}, localID("M"), true); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero asset fact accepted")
	}
	if _, err := fleet.NewDraft(fleet.AssetRef{}, fleet.Profile{}, nil); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero draft accepted")
	}
	if _, err := (fleet.Asset{}).Snapshot(scope(t, "A")); err == nil {
		t.Fatal("zero aggregate accepted")
	}
	if err := (fleet.Asset{}).CheckActivation(scope(t, "A"), fleet.ActivationFacts{}); err == nil {
		t.Fatal("zero aggregate activated")
	}
	if value := id.Value(); value != "7b0e2a54-0000-4000-8000-000000000011" {
		t.Fatal("typed id changed")
	}
	reference, _ := ref(t, "A", "X")
	if reference.ID() != id || reference.OrganizationID() != scope(t, "A").OrgID() {
		t.Fatal("scoped identity changed")
	}
}

type alteredCatalog struct{ unit fleet.Unit }

func (alteredCatalog) Category(code string) (fleet.CategoryDescriptor, bool) {
	return fleet.CategoryDescriptor{Code: code}, true
}
func (value alteredCatalog) Product(code string) (fleet.ProductDescriptor, bool) {
	return fleet.ProductDescriptor{Code: code, Unit: value.unit}, true
}
func TestCapturedValuesCannotBorrowChangedCatalogUnitsOrZeroParts(t *testing.T) {
	_, profile := privateAsset(t)
	reference, _ := ref(t, "A", "X")
	if _, err := fleet.NewDraft(reference, profile, alteredCatalog{unit: fleet.CubicMetre}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("changed catalog unit accepted")
	}
	profile.Tanks = []fleet.Tank{{}}
	if _, err := fleet.NewDraft(reference, profile, catalog{}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero tank accepted")
	}
	_, profile = privateAsset(t)
	profile.Expected = []fleet.ExpectedConsumption{{}}
	if _, err := fleet.NewDraft(reference, profile, catalog{}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero reference accepted")
	}
	if _, err := fleet.NewTank(nil, fleet.Main, "1", []string{"LIQUID_A"}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("missing catalog accepted")
	}
	if _, err := fleet.NewExpectedConsumption(nil, "LIQUID_A", fleet.Distance, "1", "Synthetic", "2026-11-01"); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("missing product catalog accepted")
	}
	_, profile = privateAsset(t)
	if _, err := fleet.NewDraft(reference, profile, nil); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("missing category catalog accepted")
	}
}
func TestUnicodeMetadataControlsEncodingAndExactReferenceBounds(t *testing.T) {
	_, profile := privateAsset(t)
	reference, _ := ref(t, "A", "X")
	for _, raw := range []string{string([]byte{0xff}), "A\x00B", "A\x7fB", "A\u0085B", strings.Repeat("🚚", 81)} {
		profile.Name = raw
		if _, err := fleet.NewDraft(reference, profile, catalog{}); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid metadata accepted")
		}
	}
	source := strings.Repeat("🚚", 280)
	value, err := fleet.NewExpectedConsumption(catalog{}, "LIQUID_A", fleet.Distance, strconv.FormatInt(math.MaxInt64, 10), source, "2024-02-29")
	if err != nil || value.Snapshot().Value != math.MaxInt64 || value.Snapshot().Source != source {
		t.Fatal("exact representational boundary changed")
	}
	if _, err := fleet.NewExpectedConsumption(catalog{}, "LIQUID_A", fleet.Distance, "1", string([]byte{0xff}), "2026-11-01"); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("malformed source accepted")
	}
}
func TestDomainImportsOnlyStandardLibraryAndApprovedKernels(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "asset.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(path, ".") && path != "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel" && path != "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain" {
			t.Fatal("asset domain imported a service or personal module")
		}
	}
}

type pointerCatalog struct {
	code string
	unit fleet.Unit
}

func (value *pointerCatalog) Category(string) (fleet.CategoryDescriptor, bool) {
	return fleet.CategoryDescriptor{Code: value.code}, true
}
func (value *pointerCatalog) Product(string) (fleet.ProductDescriptor, bool) {
	return fleet.ProductDescriptor{Code: value.code, Unit: value.unit}, true
}
func TestTypedNilCatalogIsUnavailableInsteadOfPanicking(t *testing.T) {
	var absent *pointerCatalog
	if _, err := fleet.NewTank(absent, fleet.Main, "1", []string{"LIQUID_A"}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("typed nil product catalog accepted")
	}
	reference, _ := ref(t, "A", "X")
	_, profile := privateAsset(t)
	if _, err := fleet.NewDraft(reference, profile, absent); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("typed nil category catalog accepted")
	}
}

type changingCatalog struct{ calls int }

func (*changingCatalog) Category(code string) (fleet.CategoryDescriptor, bool) {
	return fleet.CategoryDescriptor{Code: code}, true
}
func (value *changingCatalog) Product(code string) (fleet.ProductDescriptor, bool) {
	value.calls++
	unit := fleet.Liter
	if value.calls%2 == 0 {
		unit = fleet.CubicMetre
	}
	return fleet.ProductDescriptor{Code: code, Unit: unit}, true
}
func TestOneProductCannotHaveConflictingCapturedUnitsAcrossTanks(t *testing.T) {
	tankL, err := fleet.NewTank(alteredCatalog{unit: fleet.Liter}, fleet.Main, "1", []string{"LIQUID_A"})
	if err != nil {
		t.Fatal(err)
	}
	tankM3, err := fleet.NewTank(alteredCatalog{unit: fleet.CubicMetre}, fleet.Auxiliary, "1", []string{"LIQUID_A"})
	if err != nil {
		t.Fatal(err)
	}
	reference, _ := ref(t, "A", "X")
	profile := fleet.Profile{Kind: fleet.Vehicle, Category: "CAT_A", Name: "Synthetic", FleetNumber: "F01", UsageBasis: fleet.Distance, Tanks: []fleet.Tank{tankL, tankM3}}
	if _, err := fleet.NewDraft(reference, profile, &changingCatalog{}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("one product acquired conflicting units during validation")
	}
}
