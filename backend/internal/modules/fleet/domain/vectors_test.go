package domain_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type catalog struct{}

func (catalog) Category(code string) (fleet.CategoryDescriptor, bool) {
	switch code {
	case "CAT_A", "CAT_B":
		return fleet.CategoryDescriptor{Code: code}, true
	case "BAD_CATEGORY":
		return fleet.CategoryDescriptor{Code: "CAT_A"}, true
	}
	return fleet.CategoryDescriptor{}, false
}
func (catalog) Product(code string) (fleet.ProductDescriptor, bool) {
	unit := fleet.Unit("")
	switch code {
	case "LIQUID_A", "LIQUID_B", "\ue000", "\U00010000":
		unit = fleet.Liter
	case "GAS_A":
		unit = fleet.CubicMetre
	case "POWER_A":
		unit = fleet.KilowattHour
	case "BAD_UNIT":
		unit = fleet.Unit("INVALID")
	case "BAD_PRODUCT":
		return fleet.ProductDescriptor{Code: "LIQUID_A", Unit: fleet.Liter}, true
	default:
		return fleet.ProductDescriptor{}, false
	}
	return fleet.ProductDescriptor{Code: code, Unit: unit}, true
}
func fieldMap(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, field := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" {
			t.Fatal("invalid vector field")
		}
		if _, duplicate := out[key]; duplicate {
			t.Fatal("duplicate vector field")
		}
		out[key] = value
	}
	return out
}
func text(t *testing.T, raw string) string {
	t.Helper()
	value, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal("invalid vector UTF-8 hexadecimal")
	}
	return string(value)
}
func hexText(raw string) string { return hex.EncodeToString([]byte(raw)) }
func scope(t *testing.T, alias string) kernel.OrgScope {
	t.Helper()
	switch alias {
	case "?":
		return kernel.OrgScope{}
	case "A":
		alias = "7b0e2a54-0000-4000-8000-000000000001"
	case "B":
		alias = "7b0e2a54-0000-4000-8000-000000000002"
	}
	id, err := kernel.ParseOrgID(alias)
	if err != nil {
		t.Fatal("invalid organization fixture")
	}
	value, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal("invalid scope fixture")
	}
	return value
}
func assetID(alias string) (fleet.AssetID, error) {
	switch alias {
	case "X":
		alias = "7b0e2a54-0000-4000-8000-000000000011"
	case "Y":
		alias = "7b0e2a54-0000-4000-8000-000000000012"
	}
	return fleet.ParseAssetID(alias)
}
func ref(t *testing.T, organization, asset string) (fleet.AssetRef, error) {
	t.Helper()
	id, err := assetID(asset)
	if err != nil {
		return fleet.AssetRef{}, err
	}
	return fleet.NewAssetRef(scope(t, organization), id)
}
func localID(alias string) string {
	switch alias {
	case "U":
		return "7b0e2a54-0000-4000-8000-000000000021"
	case "M":
		return "7b0e2a54-0000-4000-8000-000000000031"
	}
	return alias
}
func profile(t *testing.T, args map[string]string) (fleet.Profile, error) {
	t.Helper()
	value := fleet.Profile{Kind: fleet.Kind(args["kind"]), Category: args["category"], Name: text(t, args["name_hex"]),
		FleetNumber: text(t, args["fleet_hex"]), Plate: text(t, args["plate_hex"]), SerialNumber: text(t, args["serial_hex"]), UsageBasis: fleet.Basis(args["basis"])}
	if args["tanks"] != "" {
		for _, item := range strings.Split(args["tanks"], "~") {
			fields := strings.Split(item, ":")
			if len(fields) != 3 {
				t.Fatal("invalid tank fixture")
			}
			tank, err := fleet.NewTank(catalog{}, fleet.TankRole(fields[0]), fields[1], strings.Split(fields[2], ","))
			if err != nil {
				return value, err
			}
			value.Tanks = append(value.Tanks, tank)
		}
	}
	if args["references"] != "" {
		for _, item := range strings.Split(args["references"], "~") {
			fields := strings.Split(item, ":")
			if len(fields) != 5 {
				t.Fatal("invalid reference fixture")
			}
			expected, err := fleet.NewExpectedConsumption(catalog{}, fields[0], fleet.Basis(fields[1]), fields[2], text(t, fields[3]), fields[4])
			if err != nil {
				return value, err
			}
			value.Expected = append(value.Expected, expected)
		}
	}
	return value, nil
}
func classify(t *testing.T, err error) map[string]string {
	t.Helper()
	if err == nil {
		t.Fatal("missing domain error")
	}
	switch {
	case errors.Is(err, fleet.ErrInvalid):
		return map[string]string{"error": "INVALID"}
	case errors.Is(err, fleet.ErrScope):
		return map[string]string{"error": "SCOPE"}
	case errors.Is(err, fleet.ErrKindLocked):
		return map[string]string{"error": "KIND_LOCKED"}
	case errors.Is(err, fleet.ErrIncomplete):
		var incomplete *fleet.ActivationError
		if !errors.As(err, &incomplete) {
			t.Fatal("missing activation checklist")
		}
		var fields []string
		for _, item := range incomplete.Missing() {
			fields = append(fields, string(item))
		}
		return map[string]string{"error": "INCOMPLETE", "missing": strings.Join(fields, ",")}
	}
	t.Fatal("unrecognized domain error")
	return nil
}
func execute(t *testing.T, operation string, args map[string]string) map[string]string {
	t.Helper()
	switch operation {
	case "NORMALIZE":
		var key string
		var err error
		switch args["type"] {
		case "PLATE":
			key, err = fleet.NormalizePlate(text(t, args["value_hex"]))
		case "FLEET":
			key, err = fleet.NormalizeFleetNumber(text(t, args["value_hex"]))
		default:
			err = fleet.ErrInvalid
		}
		if err != nil {
			return classify(t, err)
		}
		return map[string]string{"key_hex": hexText(key)}
	case "TANK":
		value, err := fleet.NewTank(catalog{}, fleet.TankRole(args["role"]), args["capacity"], strings.Split(args["products"], ","))
		if err != nil {
			return classify(t, err)
		}
		snapshot := value.Snapshot()
		return map[string]string{"role": string(snapshot.Role), "capacity": strconv.FormatInt(snapshot.Capacity, 10), "unit": string(snapshot.Unit), "products": strings.Join(snapshot.Products, ",")}
	case "EXPECTED":
		value, err := fleet.NewExpectedConsumption(catalog{}, args["product"], fleet.Basis(args["basis"]), args["value"], text(t, args["source_hex"]), args["date"])
		if err != nil {
			return classify(t, err)
		}
		snapshot := value.Snapshot()
		return map[string]string{"product": snapshot.Product, "basis": string(snapshot.Basis), "value": strconv.FormatInt(snapshot.Value, 10), "unit": string(snapshot.Unit), "source_hex": hexText(snapshot.Source), "date": snapshot.Date}
	case "VALIDATE", "ACTIVATE":
		draft, err := profile(t, args)
		if err != nil {
			return classify(t, err)
		}
		reference, err := ref(t, "A", "X")
		if err != nil {
			t.Fatal("invalid asset fixture")
		}
		asset, err := fleet.NewDraft(reference, draft, catalog{})
		if err != nil {
			return classify(t, err)
		}
		if operation == "VALIDATE" {
			value, err := asset.Snapshot(scope(t, "A"))
			if err != nil {
				t.Fatal(err)
			}
			return map[string]string{"kind": string(value.Profile.Kind), "category": value.Profile.Category, "name_hex": hexText(value.Profile.Name),
				"fleet_key_hex": hexText(value.FleetKey), "plate_key_hex": hexText(value.PlateKey), "serial_hex": hexText(value.Profile.SerialNumber), "basis": string(value.Profile.UsageBasis),
				"tanks": strconv.Itoa(len(value.Profile.Tanks)), "references": strconv.Itoa(len(value.Profile.Expected))}
		}
		facts := fleet.ActivationFacts{}
		if args["unit_org"] != "?" || args["unit_id"] != "?" {
			unit, err := fleet.NewUnitRef(scope(t, args["unit_org"]), localID(args["unit_id"]))
			if err != nil {
				return classify(t, err)
			}
			facts.Unit = &unit
		}
		if args["meters"] != "" {
			for _, meter := range strings.Split(args["meters"], "~") {
				fields := strings.Split(meter, ",")
				if len(fields) != 4 {
					t.Fatal("invalid meter fixture")
				}
				asset, err := ref(t, fields[0], fields[1])
				if err != nil {
					return classify(t, err)
				}
				active, err := strconv.ParseBool(fields[3])
				if err != nil {
					t.Fatal("invalid meter state fixture")
				}
				fact, err := fleet.NewMeterFact(asset, localID(fields[2]), active)
				if err != nil {
					return classify(t, err)
				}
				facts.Meters = append(facts.Meters, fact)
			}
		}
		if err := asset.CheckActivation(scope(t, "A"), facts); err != nil {
			return classify(t, err)
		}
		return map[string]string{"ready": "true"}
	case "CHANGE_KIND":
		accepted, err := strconv.ParseBool(args["accepted"])
		if err != nil {
			t.Fatal("invalid accepted fixture")
		}
		kind, err := fleet.ChangeKind(fleet.Kind(args["from"]), fleet.Kind(args["to"]), accepted)
		if err != nil {
			return classify(t, err)
		}
		return map[string]string{"kind": string(kind)}
	case "STATE":
		state, err := fleet.ParseState(args["value"])
		if err != nil {
			return classify(t, err)
		}
		return map[string]string{"state": string(state)}
	case "SCOPE":
		asset, err := ref(t, args["org"], args["asset"])
		if err != nil {
			return classify(t, err)
		}
		if err := asset.Validate(scope(t, args["request_org"])); err != nil {
			return classify(t, err)
		}
		return map[string]string{"valid": "true"}
	}
	t.Fatal("unknown vector operation")
	return nil
}
func TestSharedAssetVectors(t *testing.T) {
	file, err := os.Open("../../../../../contracts/testdata/enterprise/fleet/asset-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid vector header")
	}
	seen := map[string]bool{}
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "|")
		if len(fields) != 4 || fields[0] == "" || seen[fields[0]] {
			t.Fatal("invalid or duplicate vector")
		}
		seen[fields[0]] = true
		t.Run(fields[0], func(t *testing.T) {
			got := execute(t, fields[1], fieldMap(t, fields[2]))
			want := fieldMap(t, fields[3])
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
	if scanner.Err() != nil || len(seen) != 133 {
		t.Fatalf("rows=%d scanner=%v", len(seen), scanner.Err())
	}
}
