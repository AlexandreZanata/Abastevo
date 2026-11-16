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

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
)

const segmentsOracleSHA256 = "296afeb474264ba4305004cbcc13c56761c192eca8c7ab9b9eede6e9cf15d21f"

var segmentOutputs = map[string][]string{
	"CHECK_WRAP":      {"state", "flags", "retroactive", "previous", "next"},
	"REEVALUATE_WRAP": {"changes"},
	"CUMULATIVE":      {"reason", "usage", "equivalent", "anchor"},
	"USAGE":           {"reason", "usage", "from", "to"},
	"REPLACE":         {"previous", "close", "open", "implied", "reason"},
	"RESET":           {"previous", "close", "open", "implied", "reason"},
	"ROLLOVER":        {"previous", "close", "open", "implied", "reason"},
}
var segmentArgs = map[string]map[string]bool{
	"CHECK_WRAP":      {"kind": true, "rate": true, "skew": true, "modulus": true, "series": true, "candidate": true},
	"REEVALUATE_WRAP": {"kind": true, "rate": true, "skew": true, "modulus": true, "series": true},
	"CUMULATIVE":      {"kind": true, "spans": true, "readings": true, "at": true},
	"USAGE":           {"kind": true, "spans": true, "readings": true, "start": true, "end": true},
	"REPLACE":         {"kind": true, "modulus": true, "series": true, "at": true, "final": true, "new_modulus": true, "opening": true},
	"RESET":           {"kind": true, "modulus": true, "series": true, "at": true, "before": true, "after": true},
	"ROLLOVER":        {"kind": true, "modulus": true, "series": true, "target": true},
}
var segmentCounts = map[string]int{"CHECK_WRAP": 31, "REEVALUATE_WRAP": 13, "CUMULATIVE": 35, "USAGE": 19, "REPLACE": 17, "RESET": 12, "ROLLOVER": 16}

func loadSegmentVectors(raw []byte) ([]validationVector, error) {
	bad := func(why string) ([]validationVector, error) {
		return nil, fmt.Errorf("invalid segment oracle: %s", why)
	}
	if !utf8.Valid(raw) {
		return bad("utf8")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 144 || lines[0] != "id|operation|args|expected" {
		return bad("header or size")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	out := make([]validationVector, 0, 143)
	for _, line := range lines[1:] {
		p := strings.Split(line, "|")
		if len(p) != 4 || p[0] == "" || seen[p[0]] {
			return bad("row")
		}
		seen[p[0]] = true
		outputs, known := segmentOutputs[p[1]]
		if !known {
			return bad("operation")
		}
		args, err := validationFields(p[2])
		if err != nil {
			return bad("args")
		}
		for key := range args {
			if !segmentArgs[p[1]][key] {
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
	if !reflect.DeepEqual(counts, segmentCounts) {
		return bad("counts")
	}
	return out, nil
}

func segmentID(alias string) string {
	if len(alias) > 1 {
		base := map[byte]int{'R': 200, 'S': 400, 'M': 500}[alias[0]]
		if n, err := strconv.Atoi(alias[1:]); base > 0 && err == nil && n >= 0 {
			return fmt.Sprintf("8c000000-0000-4000-8000-%012d", base+n)
		}
	}
	return alias
}
func segmentLabel(id string) string {
	if id == "" {
		return "?"
	}
	if raw, ok := strings.CutPrefix(id, "8c000000-0000-4000-8000-"); ok {
		if n, err := strconv.Atoi(raw); err == nil {
			for _, p := range []struct {
				prefix string
				base   int
			}{{"M", 500}, {"S", 400}, {"R", 200}} {
				if n >= p.base {
					return p.prefix + strconv.Itoa(n-p.base)
				}
			}
		}
	}
	return id
}

func optionalInt(f fields, key string) (*int64, error) {
	raw, ok := f[key]
	if !ok {
		return nil, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func segmentSeries(raw string) ([]meter.Entry, error) {
	series, err := validationSeries(raw)
	for i := range series {
		series[i].ID = segmentID(validationLabel(series[i].ID))
	}
	return series, err
}

func segmentSpans(raw string) ([]meter.Span, error) {
	var out []meter.Span
	if raw == "" {
		return out, nil
	}
	for _, item := range strings.Split(raw, "~") {
		p := strings.Split(item, ",")
		if len(p) != 7 {
			return nil, fmt.Errorf("span shape")
		}
		opened, err := validationInstant(p[3])
		if err != nil {
			return nil, err
		}
		open, err := strconv.ParseInt(p[4], 10, 64)
		if err != nil {
			return nil, err
		}
		s := meter.Span{ID: segmentID(p[0]), Meter: segmentID(p[1]), Reason: meter.SegmentReason(p[2]), OpenedAt: opened, Open: open}
		if p[5] != "-" {
			if s.ClosedAt, err = validationInstant(p[5]); err != nil {
				return nil, err
			}
			s.Closed = true
			if p[6] != "?" {
				v, err := strconv.ParseInt(p[6], 10, 64)
				if err != nil {
					return nil, err
				}
				s.Close = &v
			}
		} else if p[6] != "-" {
			return nil, fmt.Errorf("open span with a close")
		}
		out = append(out, s)
	}
	return out, nil
}

func segmentAnchors(raw string) ([]meter.Anchor, error) {
	var out []meter.Anchor
	if raw == "" {
		return out, nil
	}
	for _, item := range strings.Split(raw, "~") {
		p := strings.Split(item, ",")
		if len(p) != 5 {
			return nil, fmt.Errorf("reading shape")
		}
		v, err := strconv.ParseInt(p[2], 10, 64)
		if err != nil {
			return nil, err
		}
		occurred, err := validationInstant(p[3])
		if err != nil {
			return nil, err
		}
		received, err := validationInstant(p[4])
		if err != nil {
			return nil, err
		}
		out = append(out, meter.Anchor{ID: segmentID(p[0]), Segment: segmentID(p[1]), Value: v, OccurredAt: occurred, ReceivedAt: received})
	}
	return out, nil
}

func figureValue(known bool, v int64) string {
	if !known {
		return "?"
	}
	return strconv.FormatInt(v, 10)
}

func boundaryFields(b meter.Boundary) fields {
	closing := "?"
	if b.Close != nil {
		closing = strconv.FormatInt(*b.Close, 10)
	}
	return fields{"previous": segmentLabel(b.Previous), "close": closing, "open": strconv.FormatInt(b.Open, 10), "implied": figureValue(b.Reason == "", b.Implied), "reason": string(b.Reason)}
}

func applySegmentVector(v validationVector) (fields, error) {
	kind := meter.Kind(v.args.get("kind", "ODOMETER"))
	modulus, err := optionalInt(v.args, "modulus")
	if err != nil {
		return nil, fmt.Errorf("fixture modulus: %w", err)
	}
	switch v.operation {
	case "CHECK_WRAP", "REEVALUATE_WRAP":
		policy, err := validationPolicy(v.args)
		if err != nil {
			return nil, err
		}
		series, err := segmentSeries(v.args["series"])
		if err != nil {
			return nil, fmt.Errorf("fixture series: %w", err)
		}
		if v.operation == "CHECK_WRAP" {
			candidate, _, err := validationPoint(v.args["candidate"], 4)
			if err != nil {
				return nil, fmt.Errorf("fixture candidate: %w", err)
			}
			candidate.ID = segmentID(validationLabel(candidate.ID))
			verdict, err := policy.CheckMeter(kind, modulus, series, candidate)
			if err != nil {
				return nil, err
			}
			return fields{"state": string(verdict.State), "flags": joinFlags(verdict.Flags), "retroactive": strconv.FormatBool(verdict.Retroactive), "previous": segmentLabel(verdict.Previous), "next": segmentLabel(verdict.Next)}, nil
		}
		changes, err := policy.ReevaluateMeter(kind, modulus, series)
		if err != nil {
			return nil, err
		}
		parts := make([]string, len(changes))
		for i, c := range changes {
			note := ""
			if c.Verdict.Retroactive {
				note = meter.NoteRetroactive
			}
			parts[i] = segmentLabel(c.ID) + ":" + string(c.Verdict.State) + ":" + joinFlags(c.Verdict.Flags) + ":" + note
		}
		return fields{"changes": strings.Join(parts, "~")}, nil
	case "CUMULATIVE", "USAGE":
		spans, err := segmentSpans(v.args["spans"])
		if err != nil {
			return nil, fmt.Errorf("fixture spans: %w", err)
		}
		anchors, err := segmentAnchors(v.args["readings"])
		if err != nil {
			return nil, fmt.Errorf("fixture readings: %w", err)
		}
		track, err := meter.NewTrack(kind, spans, anchors)
		if err != nil {
			return nil, err
		}
		if v.operation == "CUMULATIVE" {
			at, err := validationInstant(v.args["at"])
			if err != nil {
				return nil, err
			}
			f, err := track.Cumulative(at)
			if err != nil {
				return nil, err
			}
			return fields{"reason": string(f.Reason), "usage": figureValue(f.Known(), f.Value), "equivalent": figureValue(f.Known(), f.Equivalent), "anchor": segmentLabel(f.To)}, nil
		}
		start, err := validationInstant(v.args["start"])
		if err != nil {
			return nil, err
		}
		end, err := validationInstant(v.args["end"])
		if err != nil {
			return nil, err
		}
		f, err := track.Between(start, end)
		if err != nil {
			return nil, err
		}
		return fields{"reason": string(f.Reason), "usage": figureValue(f.Known(), f.Value), "from": segmentLabel(f.From), "to": segmentLabel(f.To)}, nil
	}
	series, err := segmentSeries(v.args["series"])
	if err != nil {
		return nil, fmt.Errorf("fixture series: %w", err)
	}
	var b meter.Boundary
	switch v.operation {
	case "REPLACE":
		at, err := validationInstant(v.args["at"])
		if err != nil {
			return nil, err
		}
		var final *int64
		if v.args["final"] != "?" {
			if final, err = optionalInt(v.args, "final"); err != nil {
				return nil, err
			}
		}
		newModulus, err := optionalInt(v.args, "new_modulus")
		if err != nil {
			return nil, err
		}
		opening, err := strconv.ParseInt(v.args["opening"], 10, 64)
		if err != nil {
			return nil, err
		}
		b, err = meter.PlanReplacement(kind, modulus, series, at, final, newModulus, opening)
		if err != nil {
			return nil, err
		}
	case "RESET":
		at, err := validationInstant(v.args["at"])
		if err != nil {
			return nil, err
		}
		before, err := strconv.ParseInt(v.args["before"], 10, 64)
		if err != nil {
			return nil, err
		}
		after, err := strconv.ParseInt(v.args["after"], 10, 64)
		if err != nil {
			return nil, err
		}
		if b, err = meter.PlanReset(kind, modulus, series, at, before, after); err != nil {
			return nil, err
		}
	case "ROLLOVER":
		if b, err = meter.PlanRollover(kind, modulus, series, segmentID(v.args["target"])); err != nil {
			return nil, err
		}
	}
	return boundaryFields(b), nil
}

func TestSegmentSharedOracle(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../contracts/testdata/enterprise/meter/segments-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != segmentsOracleSHA256 {
		t.Fatalf("segment oracle digest %s", got)
	}
	vectors, err := loadSegmentVectors(raw)
	if err != nil {
		t.Fatal(err)
	}
	operations, text := map[string]bool{}, ""
	for _, v := range vectors {
		operations[v.operation] = true
		for _, value := range v.expected {
			text += value + ";"
		}
		t.Run(v.id, func(t *testing.T) {
			got, err := applySegmentVector(v)
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
	if len(operations) != len(segmentOutputs) {
		t.Fatal("every operation needs a vector")
	}
	for _, code := range []string{"POSSIBLE_ROLLOVER", "NO_READING_BEFORE", "METER_GAP", "INVALID"} {
		if !strings.Contains(text, code) {
			t.Fatalf("%s has no vector", code)
		}
	}
}

func TestSegmentOracleGrammarCannotSkipOrReinterpretRows(t *testing.T) {
	raw, err := os.ReadFile("../../../../../../contracts/testdata/enterprise/meter/segments-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	original := string(raw)
	for name, bad := range map[string]string{
		"header":         strings.Replace(original, "id|operation|args|expected", "wrong", 1),
		"blank row":      original + "\n",
		"duplicate id":   strings.Replace(original, "d-without-modulus-below|", "d-wrap-candidate|", 1),
		"unknown op":     strings.Replace(original, "|CHECK_WRAP|", "|CHECK|", 1),
		"unknown arg":    strings.Replace(original, "|CUMULATIVE|spans=", "|CUMULATIVE|segments=", 1),
		"missing output": strings.Replace(original, ";anchor=R8\n", "\n", 1),
		"extra output":   strings.Replace(original, "changes=\n", "changes=;more=1\n", 1),
		"bad error":      strings.Replace(original, "error=INVALID", "error=SCOPE", 1),
	} {
		if _, err := loadSegmentVectors([]byte(bad)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := loadSegmentVectors([]byte{0xff}); err == nil {
		t.Fatal("malformed UTF-8 accepted")
	}
}
