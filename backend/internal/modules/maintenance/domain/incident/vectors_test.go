package incident_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
)

const oraclePath = "../../../../../../contracts/testdata/enterprise/checklist/incident-v1.psv"
const oracleSHA256 = "401ba762533077b059ffd5c39161451622adb6cf1fcf40b7de8ca8b235549119"

type fields map[string]string

func (f fields) get(key, fallback string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return fallback
}

type vector struct {
	id, operation  string
	args, expected fields
}

var outputs = map[string][]string{
	"REPORT": {"type", "kind", "state", "revision", "blocks"},
	"DECIDE": {"from", "to", "revision", "action"},
}
var inputs = map[string]map[string]bool{
	"REPORT": {"type": true, "kind": true, "description": true, "location": true, "blocks": true},
	"DECIDE": {"state": true, "revision": true, "blocks": true, "report": true, "reporter": true, "action": true, "expected": true, "note": true, "duplicate": true, "handler": true},
}
var counts = map[string]int{"REPORT": 35, "DECIDE": 63}
var codes = map[string]error{"INVALID": incident.ErrInvalid, "CONFLICT": incident.ErrConflict, "TRANSITION": incident.ErrTransition, "SELF_APPROVAL": incident.ErrSelfApproval}

func parseFields(raw string) (fields, error) {
	out := fields{}
	if raw == "" {
		return out, nil
	}
	for _, item := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("malformed field")
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("duplicate field")
		}
		out[key] = value
	}
	return out, nil
}

func loadVectors(raw []byte) ([]vector, error) {
	bad := func(why string) ([]vector, error) { return nil, fmt.Errorf("invalid incident oracle: %s", why) }
	if !utf8.Valid(raw) {
		return bad("utf8")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 99 || lines[0] != "id|operation|args|expected" {
		return bad("header or size")
	}
	seen, got := map[string]bool{}, map[string]int{}
	out := make([]vector, 0, 98)
	for _, line := range lines[1:] {
		p := strings.Split(line, "|")
		if len(p) != 4 || p[0] == "" || seen[p[0]] {
			return bad("row")
		}
		seen[p[0]] = true
		want, known := outputs[p[1]]
		if !known {
			return bad("operation")
		}
		args, err := parseFields(p[2])
		if err != nil {
			return bad("args")
		}
		for key := range args {
			if !inputs[p[1]][key] {
				return bad("arg key")
			}
		}
		expected, err := parseFields(p[3])
		if err != nil || len(expected) == 0 {
			return bad("expected")
		}
		if code, ok := expected["error"]; ok {
			if _, known := codes[code]; !known || len(expected) != 1 {
				return bad("error map")
			}
		} else {
			if len(expected) != len(want) {
				return bad("expected size")
			}
			for _, key := range want {
				if _, ok := expected[key]; !ok {
					return bad("expected key")
				}
			}
		}
		got[p[1]]++
		out = append(out, vector{p[0], p[1], args, expected})
	}
	if !reflect.DeepEqual(got, counts) {
		return bad("counts")
	}
	return out, nil
}

// text decodes the hex:/rep: grammar; malformed fixture text is a loader error.
func text(raw string) (string, error) {
	switch {
	case strings.HasPrefix(raw, "hex:"):
		body := strings.TrimPrefix(raw, "hex:")
		b, err := hex.DecodeString(body)
		if err != nil || strings.ToLower(body) != body {
			return "", fmt.Errorf("fixture hex")
		}
		return string(b), nil
	case strings.HasPrefix(raw, "rep:"):
		count, rest, ok := strings.Cut(strings.TrimPrefix(raw, "rep:"), ":")
		n, err := strconv.Atoi(count)
		if !ok || err != nil || n < 1 {
			return "", fmt.Errorf("fixture repeat")
		}
		unit, err := text(rest)
		if err != nil {
			return "", err
		}
		return strings.Repeat(unit, n), nil
	}
	return raw, nil
}

// optional maps "?" to absent and any other value through the text grammar.
func optional(raw string) (*string, error) {
	if raw == "?" {
		return nil, nil
	}
	v, err := text(raw)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// alias maps R<n>, A<n>, M<n>, E<n> to fixed synthetic UUIDs.
func alias(raw string) string {
	if len(raw) > 1 {
		if n, err := strconv.Atoi(raw[1:]); err == nil && n >= 0 && strconv.Itoa(n) == raw[1:] {
			base := map[byte]int{'R': 4000, 'A': 5000, 'M': 6000, 'E': 7000}[raw[0]]
			if base != 0 {
				return fmt.Sprintf("9c000000-0000-4000-8000-%012d", base+n)
			}
		}
	}
	return raw
}

func identity(raw string) []string {
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		out = append(out, alias(part))
	}
	return out
}

func boolean(raw string) (bool, error) {
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("fixture boolean")
}

func integer(raw string) (int64, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || strconv.FormatInt(n, 10) != raw {
		return 0, fmt.Errorf("fixture integer")
	}
	return n, nil
}

var errFixture = errors.New("fixture")

func apply(v vector) (fields, error) {
	a := v.args
	switch v.operation {
	case "REPORT":
		kind := a.get("kind", "?")
		if kind == "?" {
			kind = ""
		} else if decoded, err := text(kind); err == nil {
			kind = decoded
		} else {
			return nil, errFixture
		}
		description, err := text(a.get("description", "Synthetic damage"))
		if err != nil {
			return nil, errFixture
		}
		location, err := optional(a.get("location", "?"))
		if err != nil {
			return nil, errFixture
		}
		blocks, err := boolean(a.get("blocks", "false"))
		if err != nil {
			return nil, errFixture
		}
		r, err := incident.NewReport(incident.Draft{Type: incident.ReportType(a.get("type", "DAMAGE")), Kind: incident.Kind(kind), Description: description, Location: location, BlocksUse: blocks})
		if err != nil {
			return nil, err
		}
		k := string(r.Kind)
		if k == "" {
			k = "?"
		}
		return fields{"type": string(r.Type), "kind": k, "state": string(r.State), "revision": strconv.FormatInt(r.Revision, 10), "blocks": strconv.FormatBool(r.BlocksUse)}, nil
	case "DECIDE":
		revision, err := integer(a.get("revision", "1"))
		if err != nil {
			return nil, errFixture
		}
		expected, err := integer(a.get("expected", strconv.FormatInt(revision, 10)))
		if err != nil {
			return nil, errFixture
		}
		blocks, err := boolean(a.get("blocks", "false"))
		if err != nil {
			return nil, errFixture
		}
		note, err := optional(a.get("note", "?"))
		if err != nil {
			return nil, errFixture
		}
		duplicate := a.get("duplicate", "?")
		if duplicate == "?" {
			duplicate = ""
		}
		current := incident.Current{State: incident.State(a.get("state", "REPORTED")), Revision: revision, BlocksUse: blocks, ReportID: alias(a.get("report", "R1")), Reporter: identity(a.get("reporter", "A1,M1,E1"))}
		command := incident.Command{Action: incident.Action(a.get("action", "")), ExpectedRevision: expected, Note: note, DuplicateOf: alias(duplicate), Handler: identity(a.get("handler", "A2,M2,E2"))}
		step, err := incident.Decide(current, command)
		if err != nil {
			return nil, err
		}
		return fields{"from": string(step.From), "to": string(step.To), "revision": strconv.FormatInt(step.Revision, 10), "action": string(step.Action)}, nil
	}
	return nil, errFixture
}

func domainCode(err error) (string, bool) {
	for code, target := range codes {
		if errors.Is(err, target) {
			return code, true
		}
	}
	return "", false
}

func TestIncidentSharedOracle(t *testing.T) {
	raw, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != oracleSHA256 {
		t.Fatalf("incident oracle digest %x", sum)
	}
	vectors, err := loadVectors(raw)
	if err != nil {
		t.Fatal(err)
	}
	operations, seenCodes, actions, states := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, v := range vectors {
		operations[v.operation] = true
		if code, ok := v.expected["error"]; ok {
			seenCodes[code+"/"+v.operation] = true
			seenCodes[code] = true
		} else if v.operation == "DECIDE" {
			actions[v.expected["action"]] = true
			states[v.expected["from"]], states[v.expected["to"]] = true, true
		}
		t.Run(v.id, func(t *testing.T) {
			got, err := apply(v)
			if err != nil {
				code, ok := domainCode(err)
				if !ok {
					t.Fatalf("unexpected error %v", err)
				}
				got = fields{"error": code}
			}
			if !reflect.DeepEqual(got, v.expected) {
				t.Fatalf("got %v want %v", got, v.expected)
			}
		})
	}
	if len(operations) != len(outputs) {
		t.Error("every operation needs a vector")
	}
	for code := range codes {
		if !seenCodes[code] {
			t.Error("error code without a vector", code)
		}
	}
	for op := range outputs {
		if !seenCodes["INVALID/"+op] {
			t.Error("operation without an INVALID vector", op)
		}
	}
	for _, a := range incident.Actions() {
		if !actions[string(a)] {
			t.Error("action without an accepted vector", a)
		}
	}
	for _, s := range incident.States() {
		if !states[string(s)] {
			t.Error("state without an accepted vector", s)
		}
	}
}

func TestIncidentOracleGrammarCannotSkipOrReinterpretRows(t *testing.T) {
	raw, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	original := string(raw)
	for name, bad := range map[string]string{
		"header":         strings.Replace(original, "id|operation|args|expected", "wrong", 1),
		"blank row":      original + "\n",
		"duplicate id":   strings.Replace(original, "report-incident-breakdown|", "report-damage-defaults|", 1),
		"unknown op":     strings.Replace(original, "|DECIDE|action=OPEN|", "|CLOSE|action=OPEN|", 1),
		"unknown arg":    strings.Replace(original, "|type=FIRE|", "|style=FIRE|", 1),
		"missing output": strings.Replace(original, ";action=ACKNOWLEDGE\n", "\n", 1),
		"extra output":   strings.Replace(original, ";blocks=true\n", ";blocks=true;more=1\n", 1),
		"unknown error":  strings.Replace(original, "error=TRANSITION", "error=LATER", 1),
		"error and data": strings.Replace(original, "error=SELF_APPROVAL", "error=SELF_APPROVAL;to=RESOLVED", 1),
		"dropped row":    strings.Replace(original, "decide-action-empty|DECIDE|action=|error=INVALID\n", "", 1),
	} {
		if bad == original {
			t.Fatalf("%s mutation did not apply", name)
		}
		if _, err := loadVectors([]byte(bad)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := loadVectors([]byte{0xff}); err == nil {
		t.Fatal("malformed UTF-8 accepted")
	}
	keys := make([]string, 0, len(codes))
	for code := range codes {
		keys = append(keys, code)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "CONFLICT,INVALID,SELF_APPROVAL,TRANSITION" {
		t.Fatal("closed error vocabulary changed", keys)
	}
}
