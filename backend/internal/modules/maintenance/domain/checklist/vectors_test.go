package checklist_test

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
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
)

const oraclePath = "../../../../../../contracts/testdata/enterprise/checklist/checklist-v1.psv"
const oracleSHA256 = "4231d227e82782a2dedf03655bc69e48168664bd8ef7a10a733ce3d3140165e2"

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
	"TEMPLATE":  {"context", "target", "items", "critical", "optional", "photo"},
	"POLICY":    {"mode", "validity"},
	"EVALUATE":  {"outcome", "issues", "critical", "acknowledge"},
	"RESOLVE":   {"template", "match"},
	"POLICY_AT": {"revision", "mode", "validity"},
}
var inputs = map[string]map[string]bool{
	"TEMPLATE":  {"name": true, "context": true, "target": true, "items": true},
	"POLICY":    {"mode": true, "validity": true},
	"EVALUATE":  {"items": true, "mode": true, "validity": true, "answers": true},
	"RESOLVE":   {"context": true, "asset": true, "category": true, "candidates": true},
	"POLICY_AT": {"history": true, "at": true},
}
var counts = map[string]int{"TEMPLATE": 40, "POLICY": 11, "EVALUATE": 38, "RESOLVE": 22, "POLICY_AT": 16}
var codes = map[string]error{"INVALID": checklist.ErrInvalid, "ANSWER_NOT_ALLOWED": checklist.ErrAnswerNotAllowed, "PHOTO_REQUIRED": checklist.ErrPhotoRequired, "INCOMPLETE": checklist.ErrIncomplete}

func parseFields(raw string) (fields, error) {
	out := fields{}
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
	bad := func(why string) ([]vector, error) { return nil, fmt.Errorf("invalid checklist oracle: %s", why) }
	if !utf8.Valid(raw) {
		return bad("utf8")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 128 || lines[0] != "id|operation|args|expected" {
		return bad("header or size")
	}
	seen, got := map[string]bool{}, map[string]int{}
	out := make([]vector, 0, 127)
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
		if err != nil {
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
		b, err := hex.DecodeString(strings.TrimPrefix(raw, "hex:"))
		if err != nil || strings.ToLower(strings.TrimPrefix(raw, "hex:")) != strings.TrimPrefix(raw, "hex:") {
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

func alias(raw string) string {
	if len(raw) > 1 {
		if n, err := strconv.Atoi(raw[1:]); err == nil && n >= 0 && strconv.Itoa(n) == raw[1:] {
			switch raw[0] {
			case 'C':
				return fmt.Sprintf("9c000000-0000-4000-8000-%012d", 1000+n)
			case 'X':
				return fmt.Sprintf("9c000000-0000-4000-8000-%012d", 2000+n)
			case 'T':
				return fmt.Sprintf("9c000000-0000-4000-8000-%012d", 3000+n)
			}
		}
	}
	return raw
}

func label(id string) string {
	if id == "" {
		return "?"
	}
	if suffix, ok := strings.CutPrefix(id, "9c000000-0000-4000-8000-"); ok {
		if n, err := strconv.Atoi(suffix); err == nil {
			switch {
			case n > 1000 && n < 2000:
				return "C" + strconv.Itoa(n-1000)
			case n > 2000 && n < 3000:
				return "X" + strconv.Itoa(n-2000)
			case n > 3000 && n < 4000:
				return "T" + strconv.Itoa(n-3000)
			}
		}
	}
	return id
}

func target(raw string) (checklist.Target, error) {
	kind, rest, hasIDs := strings.Cut(raw, ":")
	out := checklist.Target{Kind: checklist.TargetKind(kind)}
	if !hasIDs {
		return out, nil
	}
	out.IDs = []string{}
	if count, ok := strings.CutPrefix(rest, "gen:"); ok {
		n, err := strconv.Atoi(count)
		if err != nil || n < 1 {
			return out, fmt.Errorf("fixture target")
		}
		prefix := "X"
		if kind == "CATEGORIES" {
			prefix = "C"
		}
		for i := 1; i <= n; i++ {
			out.IDs = append(out.IDs, alias(prefix+strconv.Itoa(i)))
		}
		return out, nil
	}
	if rest == "" {
		return out, nil
	}
	for _, id := range strings.Split(rest, ",") {
		out.IDs = append(out.IDs, alias(id))
	}
	return out, nil
}

func targetText(t checklist.Target) string {
	if len(t.IDs) == 0 {
		return string(t.Kind)
	}
	parts := make([]string, len(t.IDs))
	for i, id := range t.IDs {
		parts[i] = label(id)
	}
	return string(t.Kind) + ":" + strings.Join(parts, ",")
}

func items(raw string) ([]checklist.Item, error) {
	out := []checklist.Item{}
	if raw == "" {
		return out, nil
	}
	for _, token := range strings.Split(raw, "~") {
		if count, ok := strings.CutPrefix(token, "gen:"); ok {
			n, err := strconv.Atoi(count)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("fixture items")
			}
			for i := 1; i <= n; i++ {
				out = append(out, checklist.Item{Label: "Item " + strconv.Itoa(i)})
			}
			continue
		}
		flags, rawLabel, ok := strings.Cut(token, ":")
		if !ok || flags == "" {
			return nil, fmt.Errorf("fixture item")
		}
		value, err := text(rawLabel)
		if err != nil {
			return nil, err
		}
		item := checklist.Item{Label: value}
		if flags != "-" {
			for _, flag := range flags {
				switch {
				case flag == 'C' && !item.Critical:
					item.Critical = true
				case flag == 'O' && !item.Optional:
					item.Optional = true
				case flag == 'P' && !item.PhotoOnNotOK:
					item.PhotoOnNotOK = true
				default:
					return nil, fmt.Errorf("fixture flag")
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func positions(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, "+")
}

func flagPositions(list []checklist.Item, flag func(checklist.Item) bool) string {
	var out []int
	for i, item := range list {
		if flag(item) {
			out = append(out, i+1)
		}
	}
	return positions(out)
}

func policy(f fields) (checklist.Policy, error) {
	validity, err := strconv.ParseInt(f.get("validity", "43200"), 10, 64)
	if err != nil {
		return checklist.Policy{}, fmt.Errorf("fixture validity")
	}
	return checklist.NewPolicy(checklist.Mode(f.get("mode", "BLOCK")), validity)
}

func answers(raw string) ([]checklist.Response, error) {
	out := []checklist.Response{}
	if raw == "" {
		return out, nil
	}
	for _, token := range strings.Split(raw, "~") {
		repeat := 1
		if rest, ok := strings.CutPrefix(token, "gen:"); ok {
			count, inner, ok := strings.Cut(rest, ":")
			n, err := strconv.Atoi(count)
			if !ok || err != nil || n < 1 {
				return nil, fmt.Errorf("fixture answers")
			}
			repeat, token = n, inner
		}
		p := strings.SplitN(token, ",", 3)
		r := checklist.Response{Answer: checklist.Answer(p[0])}
		if p[0] == "?" {
			r.Answer = checklist.Unanswered
		}
		if len(p) > 1 {
			switch p[1] {
			case "0":
			case "1":
				r.Photo = true
			default:
				return nil, fmt.Errorf("fixture photo")
			}
		}
		if len(p) > 2 {
			note, err := text(p[2])
			if err != nil {
				return nil, err
			}
			r.Note = note
		}
		for i := 0; i < repeat; i++ {
			out = append(out, r)
		}
	}
	return out, nil
}

func instant(raw string) (time.Time, error) {
	if !strings.HasSuffix(raw, "Z") {
		return time.Time{}, fmt.Errorf("fixture instant")
	}
	return time.Parse(time.RFC3339Nano, raw)
}

func apply(v vector) (fields, error) {
	a := v.args
	switch v.operation {
	case "TEMPLATE":
		name, err := text(a.get("name", "Synthetic checklist"))
		if err != nil {
			return nil, err
		}
		t, err := target(a.get("target", "ORGANIZATION"))
		if err != nil {
			return nil, err
		}
		list, err := items(a.get("items", "gen:1"))
		if err != nil {
			return nil, err
		}
		d, err := checklist.NewDefinition(checklist.Definition{Name: name, Context: checklist.Context(a.get("context", "DEPARTURE")), Target: t, Items: list})
		if err != nil {
			return nil, err
		}
		return fields{"context": string(d.Context), "target": targetText(d.Target), "items": strconv.Itoa(len(d.Items)),
			"critical": flagPositions(d.Items, func(i checklist.Item) bool { return i.Critical }),
			"optional": flagPositions(d.Items, func(i checklist.Item) bool { return i.Optional }),
			"photo":    flagPositions(d.Items, func(i checklist.Item) bool { return i.PhotoOnNotOK })}, nil
	case "POLICY":
		p, err := policy(a)
		if err != nil {
			return nil, err
		}
		return fields{"mode": string(p.Mode), "validity": strconv.FormatInt(p.ValiditySeconds, 10)}, nil
	case "EVALUATE":
		list, err := items(a.get("items", "gen:1"))
		if err != nil {
			return nil, err
		}
		responses, err := answers(a["answers"])
		if err != nil {
			return nil, err
		}
		// An invalid policy value is still passed through as given.
		validity, err := strconv.ParseInt(a.get("validity", "43200"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("fixture validity")
		}
		e, err := checklist.Evaluate(list, checklist.Policy{Mode: checklist.Mode(a.get("mode", "BLOCK")), ValiditySeconds: validity}, responses)
		if err != nil {
			return nil, err
		}
		return fields{"outcome": string(e.Outcome), "issues": positions(e.Issues), "critical": positions(e.Critical), "acknowledge": strconv.FormatBool(e.AcknowledgementRequired)}, nil
	case "RESOLVE":
		var candidates []checklist.Candidate
		if raw := a["candidates"]; raw != "" {
			for _, token := range strings.Split(raw, "~") {
				p := strings.SplitN(token, "/", 3)
				if len(p) != 3 {
					return nil, fmt.Errorf("fixture candidate")
				}
				t, err := target(p[2])
				if err != nil {
					return nil, err
				}
				candidates = append(candidates, checklist.Candidate{TemplateID: alias(p[0]), Context: checklist.Context(p[1]), Target: t})
			}
		}
		r, err := checklist.Resolve(candidates, checklist.Context(a.get("context", "DEPARTURE")), alias(a.get("asset", "X1")), alias(a.get("category", "C1")))
		if err != nil {
			return nil, err
		}
		return fields{"template": label(r.TemplateID), "match": string(r.Match)}, nil
	case "POLICY_AT":
		var history []checklist.PolicyRevision
		if raw := a["history"]; raw != "" {
			for _, token := range strings.Split(raw, "~") {
				p := strings.Split(token, ",")
				if len(p) != 4 {
					return nil, fmt.Errorf("fixture revision")
				}
				revision, err1 := strconv.ParseInt(p[0], 10, 64)
				validity, err2 := strconv.ParseInt(p[2], 10, 64)
				at, err3 := instant(p[3])
				if err1 != nil || err2 != nil || err3 != nil {
					return nil, fmt.Errorf("fixture revision")
				}
				history = append(history, checklist.PolicyRevision{Revision: revision, Policy: checklist.Policy{Mode: checklist.Mode(p[1]), ValiditySeconds: validity}, EffectiveAt: at})
			}
		}
		at, err := instant(a["at"])
		if err != nil {
			return nil, err
		}
		r, found, err := checklist.PolicyAt(history, at)
		if err != nil {
			return nil, err
		}
		if !found {
			return fields{"revision": "?", "mode": "?", "validity": "?"}, nil
		}
		return fields{"revision": strconv.FormatInt(r.Revision, 10), "mode": string(r.Policy.Mode), "validity": strconv.FormatInt(r.Policy.ValiditySeconds, 10)}, nil
	}
	return nil, fmt.Errorf("fixture operation")
}

func domainCode(err error) (string, bool) {
	for code, sentinel := range codes {
		if errors.Is(err, sentinel) {
			return code, true
		}
	}
	return "", false
}

func TestChecklistSharedOracle(t *testing.T) {
	raw, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != oracleSHA256 {
		t.Fatalf("checklist oracle digest %x", sum)
	}
	vectors, err := loadVectors(raw)
	if err != nil {
		t.Fatal(err)
	}
	operations, seenCodes := map[string]bool{}, map[string]bool{}
	for _, v := range vectors {
		operations[v.operation] = true
		if code, ok := v.expected["error"]; ok {
			seenCodes[code+"/"+v.operation] = true
			seenCodes[code] = true
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
		t.Fatal("every operation needs a vector")
	}
	for code := range codes {
		if !seenCodes[code] {
			t.Fatal("error code without a vector", code)
		}
	}
	for op := range outputs {
		if !seenCodes["INVALID/"+op] {
			t.Fatal("operation without an INVALID vector", op)
		}
	}
}

func TestChecklistOracleGrammarCannotSkipOrReinterpretRows(t *testing.T) {
	raw, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	original := string(raw)
	for name, bad := range map[string]string{
		"header":         strings.Replace(original, "id|operation|args|expected", "wrong", 1),
		"blank row":      original + "\n",
		"duplicate id":   strings.Replace(original, "tpl-categories-sorted|", "tpl-org-default|", 1),
		"unknown op":     strings.Replace(original, "|POLICY|", "|UNKNOWN|", 1),
		"unknown arg":    strings.Replace(original, "|mode=WARN;validity=3600|", "|style=WARN;validity=3600|", 1),
		"missing output": strings.Replace(original, ";acknowledge=false\n", "\n", 1),
		"extra output":   strings.Replace(original, "match=NONE\n", "match=NONE;more=1\n", 1),
		"unknown error":  strings.Replace(original, "error=INCOMPLETE", "error=LATER", 1),
		"error and data": strings.Replace(original, "error=PHOTO_REQUIRED", "error=PHOTO_REQUIRED;outcome=BLOCKING", 1),
		"dropped row":    strings.Replace(original, "pol-zero|POLICY|mode=BLOCK;validity=0|error=INVALID\n", "", 1),
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
	if strings.Join(keys, ",") != "ANSWER_NOT_ALLOWED,INCOMPLETE,INVALID,PHOTO_REQUIRED" {
		t.Fatal("closed error vocabulary changed", keys)
	}
}
