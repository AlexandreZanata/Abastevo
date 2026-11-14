package meter_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type captureVector struct {
	id, operation  string
	args, expected fields
}

func captureFields(raw string) (fields, error) {
	out := fields{}
	for _, item := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("malformed capture oracle")
		}
		if _, ok := out[key]; ok {
			return nil, fmt.Errorf("duplicate capture field")
		}
		out[key] = value
	}
	return out, nil
}
func loadCaptureVectors(raw []byte) ([]captureVector, error) {
	bad := func() ([]captureVector, error) { return nil, fmt.Errorf("invalid capture oracle") }
	if !utf8.Valid(raw) {
		return bad()
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 41 || lines[0] != "id|operation|args|expected" {
		return bad()
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	out := make([]captureVector, 0, 40)
	for _, line := range lines[1:] {
		p := strings.Split(line, "|")
		if len(p) != 4 || p[0] == "" || seen[p[0]] {
			return bad()
		}
		seen[p[0]] = true
		args, e := captureFields(p[2])
		if e != nil {
			return bad()
		}
		expected, e := captureFields(p[3])
		if e != nil {
			return bad()
		}
		allowed := map[string]bool{}
		keys := []string{"kind", "org", "id"}
		switch p[1] {
		case "SOURCE":
			for _, k := range keys {
				allowed[k] = true
			}
			if len(args) != 3 {
				return bad()
			}
		case "READING":
			for _, k := range []string{"legacy", "legacy_org", "capture", "capture_org", "id", "author", "author_org", "operation", "operation_org", "value", "state"} {
				allowed[k] = true
			}
			keys = append(keys, "value", "state")
		default:
			return bad()
		}
		for k := range args {
			if !allowed[k] {
				return bad()
			}
		}
		if code, ok := expected["error"]; ok {
			if len(expected) != 1 || code != "INVALID" && code != "SCOPE" {
				return bad()
			}
		} else {
			if len(expected) != len(keys) {
				return bad()
			}
			for _, k := range keys {
				if _, ok := expected[k]; !ok {
					return bad()
				}
			}
		}
		counts[p[1]]++
		out = append(out, captureVector{p[0], p[1], args, expected})
	}
	if counts["SOURCE"] != 17 || counts["READING"] != 23 {
		return bad()
	}
	return out, nil
}
func captureAlias(raw string) string {
	if n, ok := map[string]int{"A": 1, "B": 2, "D": 51, "W": 71, "E": 41, "O": 101}[raw]; ok {
		return fmt.Sprintf("8c000000-0000-4000-8000-%012d", n)
	}
	if raw == "?" {
		return ""
	}
	return raw
}
func captureLabel(raw string) string {
	for _, label := range []string{"A", "B", "D", "W", "E", "O"} {
		if captureAlias(label) == raw {
			return label
		}
	}
	return raw
}
func captureScope(raw string) kernel.OrgScope {
	id, e := kernel.ParseOrgID(captureAlias(raw))
	if e != nil || id.String() != captureAlias(raw) {
		return kernel.OrgScope{}
	}
	scope, _ := kernel.NewOrgScope(id)
	return scope
}
func captureOutput(source meter.CaptureSource) fields {
	return fields{"kind": string(source.Kind()), "org": captureLabel(source.OrganizationID().String()), "id": captureLabel(source.ID())}
}
func applyCaptureVector(v captureVector) (fields, error) {
	f := v.args
	if v.operation == "SOURCE" {
		source, e := meter.NewCaptureSource(captureScope(f["org"]), meter.CaptureKind(f["kind"]), captureAlias(f["id"]))
		if e != nil {
			return nil, e
		}
		return captureOutput(source), nil
	}
	s := captureScope("A")
	assetID, _ := fleet.ParseAssetID("8c000000-0000-4000-8000-000000000011")
	asset, _ := fleet.NewAssetRef(s, assetID)
	m, _ := meter.NewMeterRef(s, asset, "8c000000-0000-4000-8000-000000000021", meter.Odometer, nil)
	segment, _ := meter.NewSegmentRef(s, m, "8c000000-0000-4000-8000-000000000031")
	ref, _ := meter.NewReadingRef(s, segment, "8c000000-0000-4000-8000-000000000201")
	member, e := meter.NewMembershipRef(captureScope(f.get("author_org", "A")), captureAlias(f.get("author", "E")))
	if e != nil {
		return nil, e
	}
	operation, e := meter.NewOperationRef(captureScope(f.get("operation_org", "A")), captureAlias(f.get("operation", "O")))
	if e != nil {
		return nil, e
	}
	var device meter.DeviceRef
	if raw := f.get("legacy", "D"); raw != "?" {
		device, e = meter.NewDeviceRef(captureScope(f.get("legacy_org", "A")), captureAlias(raw))
		if e != nil {
			return nil, e
		}
	}
	var source *meter.CaptureSource
	if kind := f.get("capture", "NONE"); kind != "NONE" {
		if kind == "ZERO" {
			source = &meter.CaptureSource{}
		} else {
			value, e := meter.NewCaptureSource(captureScope(f.get("capture_org", "A")), meter.CaptureKind(kind), captureAlias(f.get("id", "D")))
			if e != nil {
				return nil, e
			}
			source = &value
		}
	}
	count, e := strconv.ParseInt(f.get("value", "17"), 10, 64)
	if e != nil {
		return nil, e
	}
	value, e := meter.NewValue(meter.Odometer, count, nil)
	if e != nil {
		return nil, e
	}
	at, _ := orgtime.ParseInstant("2026-11-02T00:00:00Z")
	received, _ := orgtime.ParseInstant("2026-11-03T00:00:00Z")
	r, e := meter.NewReading(s, meter.ReadingInput{Reference: ref, Value: value, Declared: &orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}, ReceivedAt: received, Author: member, Device: device, Capture: source, Operation: operation, Origin: meter.Standalone, Method: meter.Typed, State: meter.State(f.get("state", "VALID"))})
	if e != nil {
		return nil, e
	}
	out := captureOutput(r.CaptureSource())
	out["value"] = strconv.FormatInt(r.Snapshot().Value.Count(), 10)
	out["state"] = string(r.Snapshot().State)
	return out, nil
}
func TestCaptureSharedOracle(t *testing.T) {
	raw, e := os.ReadFile("../../../../../../contracts/testdata/enterprise/meter/capture-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "358c73f5b624e8529f5e130889fb34f3d4a3090232318449bd262fd6056e5020" {
		t.Fatal("capture oracle hash drift")
	}
	vectors, e := loadCaptureVectors(raw)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range vectors {
		t.Run(v.id, func(t *testing.T) {
			actual, e := applyCaptureVector(v)
			if e != nil {
				switch {
				case errors.Is(e, meter.ErrScope):
					actual = fields{"error": "SCOPE"}
				case errors.Is(e, meter.ErrInvalid):
					actual = fields{"error": "INVALID"}
				default:
					t.Fatal("unexpected domain failure", e)
				}
			}
			if !reflect.DeepEqual(actual, v.expected) {
				t.Fatal("capture oracle mismatch", actual, v.expected)
			}
		})
	}
}
func TestCaptureOracleGrammarCannotSkipOrReinterpretRows(t *testing.T) {
	raw, e := os.ReadFile("../../../../../../contracts/testdata/enterprise/meter/capture-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{[]byte(strings.Replace(string(raw), "id|operation|args|expected", "wrong", 1)), append(append([]byte(nil), raw...), '\n'), []byte(strings.Replace(string(raw), "source-web|SOURCE", "source-device|SOURCE", 1)), []byte(strings.Replace(string(raw), "|SOURCE|", "|UNKNOWN|", 1)), []byte(strings.Replace(string(raw), "org=A;kind=", "unknown=A;kind=", 1)), []byte(strings.Replace(string(raw), "kind=ENROLLED_DEVICE;org=A;id=D\n", "kind=ENROLLED_DEVICE;id=D\n", 1)), {0xff}} {
		if _, e := loadCaptureVectors(bad); e == nil {
			t.Fatal("malformed capture corpus accepted")
		}
	}
}
