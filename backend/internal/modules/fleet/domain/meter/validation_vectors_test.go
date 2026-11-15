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
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
)

const validationOracleSHA256 = "65f0cc26b961c8f2cd661b11f89e8b242816a6d4426254efa8a942b4477bee95"

type validationVector struct {
	id, operation  string
	args, expected fields
}

var validationOutputs = map[string][]string{
	"POLICY":     {"rate", "skew"},
	"CHECK":      {"state", "flags", "retroactive", "previous", "next"},
	"REEVALUATE": {"changes"},
}
var validationArgs = map[string]map[string]bool{
	"POLICY":     {"rate": true, "skew": true},
	"CHECK":      {"kind": true, "rate": true, "skew": true, "series": true, "candidate": true},
	"REEVALUATE": {"kind": true, "rate": true, "skew": true, "series": true},
}

func validationFields(raw string) (fields, error) {
	out := fields{}
	for _, item := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("malformed validation field")
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("duplicate validation field")
		}
		out[key] = value
	}
	return out, nil
}

func loadValidationVectors(raw []byte) ([]validationVector, error) {
	bad := func(why string) ([]validationVector, error) {
		return nil, fmt.Errorf("invalid validation oracle: %s", why)
	}
	if !utf8.Valid(raw) {
		return bad("utf8")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 105 || lines[0] != "id|operation|args|expected" {
		return bad("header or size")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	out := make([]validationVector, 0, 104)
	for _, line := range lines[1:] {
		p := strings.Split(line, "|")
		if len(p) != 4 || p[0] == "" || seen[p[0]] {
			return bad("row")
		}
		seen[p[0]] = true
		outputs, known := validationOutputs[p[1]]
		if !known {
			return bad("operation")
		}
		args, err := validationFields(p[2])
		if err != nil {
			return bad("args")
		}
		for key := range args {
			if !validationArgs[p[1]][key] {
				return bad("arg key")
			}
		}
		expected, err := validationFields(p[3])
		if err != nil {
			return bad("expected")
		}
		if code, ok := expected["error"]; ok {
			if len(expected) != 1 || code != "INVALID" {
				return bad("error map")
			}
		} else {
			if len(expected) != len(outputs) {
				return bad("expected size")
			}
			for _, key := range outputs {
				if _, ok := expected[key]; !ok {
					return bad("expected key")
				}
			}
		}
		counts[p[1]]++
		out = append(out, validationVector{p[0], p[1], args, expected})
	}
	if counts["POLICY"] != 8 || counts["CHECK"] != 79 || counts["REEVALUATE"] != 17 {
		return bad("counts")
	}
	return out, nil
}

func validationID(alias string) string {
	if len(alias) > 1 && alias[0] == 'R' {
		if n, err := strconv.Atoi(alias[1:]); err == nil && n >= 0 {
			return fmt.Sprintf("8c000000-0000-4000-8000-%012d", 200+n)
		}
	}
	return alias
}
func validationLabel(id string) string {
	if id == "" {
		return "?"
	}
	if strings.HasPrefix(id, "8c000000-0000-4000-8000-") {
		if n, err := strconv.Atoi(strings.TrimPrefix(id, "8c000000-0000-4000-8000-")); err == nil && n >= 200 {
			return "R" + strconv.Itoa(n-200)
		}
	}
	return id
}

func validationInstant(raw string) (time.Time, error) {
	if !strings.HasSuffix(raw, "Z") {
		return time.Time{}, fmt.Errorf("instant")
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func validationPolicy(f fields) (meter.Plausibility, error) {
	rate, err := strconv.ParseInt(f.get("rate", "150000"), 10, 64)
	if err != nil {
		return meter.Plausibility{}, err
	}
	skew, err := strconv.ParseInt(f.get("skew", "300"), 10, 64)
	if err != nil {
		return meter.Plausibility{}, err
	}
	return meter.NewPlausibility(rate, skew)
}

func validationPoint(raw string, parts int) (meter.Point, []string, error) {
	p := strings.Split(raw, ",")
	if len(p) != parts {
		return meter.Point{}, nil, fmt.Errorf("point shape")
	}
	value, err := strconv.ParseInt(p[1], 10, 64)
	if err != nil {
		return meter.Point{}, nil, err
	}
	occurred, err := validationInstant(p[2])
	if err != nil {
		return meter.Point{}, nil, err
	}
	received, err := validationInstant(p[3])
	if err != nil {
		return meter.Point{}, nil, err
	}
	return meter.Point{ID: validationID(p[0]), Value: value, OccurredAt: occurred, ReceivedAt: received}, p, nil
}

func validationSeries(raw string) ([]meter.Entry, error) {
	if raw == "" {
		return nil, nil
	}
	var out []meter.Entry
	for _, item := range strings.Split(raw, "~") {
		point, p, err := validationPoint(item, 6)
		if err != nil {
			return nil, err
		}
		entry := meter.Entry{Point: point, State: meter.State(p[4])}
		if p[5] != "" {
			for _, flag := range strings.Split(p[5], "+") {
				entry.Flags = append(entry.Flags, meter.Flag(flag))
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func joinFlags(flags []meter.Flag) string {
	parts := make([]string, len(flags))
	for i, f := range flags {
		parts[i] = string(f)
	}
	return strings.Join(parts, "+")
}

func applyValidationVector(v validationVector) (fields, error) {
	policy, err := validationPolicy(v.args)
	if v.operation == "POLICY" {
		if err != nil {
			return nil, err
		}
		return fields{"rate": strconv.FormatInt(policy.DistancePerHour(), 10), "skew": strconv.FormatInt(policy.ClockSkewSeconds(), 10)}, nil
	}
	if err != nil {
		return nil, err
	}
	series, err := validationSeries(v.args["series"])
	if err != nil {
		return nil, fmt.Errorf("fixture series: %w", err)
	}
	kind := meter.Kind(v.args.get("kind", "ODOMETER"))
	if v.operation == "CHECK" {
		candidate, _, err := validationPoint(v.args["candidate"], 4)
		if err != nil {
			return nil, fmt.Errorf("fixture candidate: %w", err)
		}
		verdict, err := policy.Check(kind, series, candidate)
		if err != nil {
			return nil, err
		}
		return fields{"state": string(verdict.State), "flags": joinFlags(verdict.Flags), "retroactive": strconv.FormatBool(verdict.Retroactive), "previous": validationLabel(verdict.Previous), "next": validationLabel(verdict.Next)}, nil
	}
	changes, err := policy.Reevaluate(kind, series)
	if err != nil {
		return nil, err
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		note := ""
		if c.Verdict.Retroactive {
			note = meter.NoteRetroactive
		}
		parts[i] = validationLabel(c.ID) + ":" + string(c.Verdict.State) + ":" + joinFlags(c.Verdict.Flags) + ":" + note
	}
	return fields{"changes": strings.Join(parts, "~")}, nil
}

func TestValidationSharedOracle(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../contracts/testdata/enterprise/meter/validation-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != validationOracleSHA256 {
		t.Fatalf("validation oracle digest %s", got)
	}
	vectors, err := loadValidationVectors(raw)
	if err != nil {
		t.Fatal(err)
	}
	operations, codes := map[string]bool{}, map[string]bool{}
	for _, v := range vectors {
		operations[v.operation] = true
		codes[v.expected["error"]] = true
		for _, flag := range strings.Split(v.expected["flags"]+"+"+v.expected["changes"], "+") {
			codes[flag] = true
		}
		t.Run(v.id, func(t *testing.T) {
			got, err := applyValidationVector(v)
			if err != nil {
				if !errors.Is(err, meter.ErrInvalid) {
					t.Fatalf("unexpected error %v", err)
				}
				got = fields{"error": "INVALID"}
			}
			if !reflect.DeepEqual(got, v.expected) {
				t.Fatalf("got %v want %v", got, v.expected)
			}
		})
	}
	if len(operations) != 3 || !codes["INVALID"] {
		t.Fatal("every operation and the error code need a vector")
	}
	for _, flag := range []string{"READING_BELOW_PREVIOUS", "READING_ABOVE_NEXT", "IMPLAUSIBLE_RATE", "FUTURE_INSTANT"} {
		found := false
		for code := range codes {
			if strings.Contains(code, flag) {
				found = true
			}
		}
		if !found {
			t.Fatalf("flag %s has no vector", flag)
		}
	}
}

func TestValidationOracleGrammarCannotSkipOrReinterpretRows(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../contracts/testdata/enterprise/meter/validation-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	original := string(raw)
	for name, bad := range map[string]string{
		"header":         strings.Replace(original, "id|operation|args|expected", "wrong", 1),
		"blank row":      original + "\n",
		"duplicate id":   strings.Replace(original, "policy-min-rate-zero-skew|", "policy-default|", 1),
		"unknown op":     strings.Replace(original, "|POLICY|", "|UNKNOWN|", 1),
		"unknown arg":    strings.Replace(original, "rate=150000;skew=300|rate", "speed=150000;skew=300|rate", 1),
		"missing output": strings.Replace(original, ";next=R2\n", "\n", 1),
		"extra output":   strings.Replace(original, "changes=\n", "changes=;more=1\n", 1),
		"bad error":      strings.Replace(original, "error=INVALID", "error=SCOPE", 1),
	} {
		if _, err := loadValidationVectors([]byte(bad)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := loadValidationVectors([]byte{0xff}); err == nil {
		t.Fatal("malformed UTF-8 accepted")
	}
}
