package fiscal_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	doc "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain/fiscal"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	corpusPath = "../../../../../../contracts/testdata/enterprise/fiscal/fiscal-v1.psv"
	corpusHash = "ad8cd1e15a4359b3e622741cd23030420612f506a0ff2238c40ce20502d84f08"
	goOnlyRows = 118
	maxRepeat  = 2000000
)

type fields map[string]string

var allOutcomes = []string{"STRUCTURE_OK", "STRUCTURE_INVALID", "UNSUPPORTED"}

var allChecks = []string{"KEY_FORMAT", "KEY_CHECK_DIGIT", "KEY_FIELDS", "QR_SIZE", "QR_URL", "QR_PARAMETERS",
	"QR_VERSION", "QR_FIELDS", "QR_MODEL", "XML_SIZE", "XML_PARSE", "XML_ROOT", "XML_ENVELOPE", "XML_VERSION",
	"XML_FIELDS", "XML_ISSUER", "XML_KEY_CONSISTENCY", "XML_ITEMS", "XML_TOTAL", "KEY_MATCH"}

// reasonOutcomes is the oracle's own closed reason catalog.
var reasonOutcomes = map[string]string{
	"KEY_LENGTH": "STRUCTURE_INVALID", "KEY_CHARACTER": "STRUCTURE_INVALID", "KEY_CHECK_DIGIT": "STRUCTURE_INVALID",
	"KEY_STATE": "STRUCTURE_INVALID", "KEY_MONTH": "STRUCTURE_INVALID", "KEY_MODEL_UNSUPPORTED": "UNSUPPORTED",
	"QR_TOO_LARGE": "STRUCTURE_INVALID", "QR_NOT_URL": "UNSUPPORTED", "QR_HOST_UNSUPPORTED": "UNSUPPORTED",
	"QR_MALFORMED": "STRUCTURE_INVALID", "QR_FORMAT_UNSUPPORTED": "UNSUPPORTED",
	"QR_UNEXPECTED_PARAMETER": "STRUCTURE_INVALID", "QR_VERSION_UNSUPPORTED": "UNSUPPORTED",
	"QR_VARIANT_UNSUPPORTED": "UNSUPPORTED", "QR_FIELDS": "STRUCTURE_INVALID", "QR_MODEL_UNSUPPORTED": "UNSUPPORTED",
	"XML_TOO_LARGE": "STRUCTURE_INVALID", "XML_DOCTYPE": "STRUCTURE_INVALID", "XML_INSTRUCTION": "STRUCTURE_INVALID",
	"XML_DEPTH": "STRUCTURE_INVALID", "XML_ELEMENTS": "STRUCTURE_INVALID", "XML_MALFORMED": "STRUCTURE_INVALID",
	"XML_ROOT_UNSUPPORTED": "UNSUPPORTED", "XML_ENVELOPE": "STRUCTURE_INVALID", "XML_VERSION_UNSUPPORTED": "UNSUPPORTED",
	"XML_FIELDS": "STRUCTURE_INVALID", "XML_ISSUER": "STRUCTURE_INVALID", "XML_ISSUER_UNSUPPORTED": "UNSUPPORTED",
	"XML_KEY_MISMATCH": "STRUCTURE_INVALID", "XML_ITEMS": "STRUCTURE_INVALID", "XML_TOTAL": "STRUCTURE_INVALID",
	"KEY_MISMATCH": "STRUCTURE_INVALID",
}

var operations = map[string]string{
	"KEY": "text unit repeat tail", "QR": "text unit repeat tail", "XML": "text unit repeat tail",
	"COMPARE": "left right", "FEED": "input",
}

var errMalformedFixture = errors.New("fixture: malformed")

func fieldMap(raw string) (fields, error) {
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, errMalformedFixture
		}
		if _, has := out[k]; has {
			return nil, errMalformedFixture
		}
		out[k] = v
	}
	return out, nil
}

func hexValue(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

// decodeFixture undoes the oracle's own %XX layer (uppercase hex only).
func decodeFixture(raw string) (string, error) {
	var out []byte
	for i := 0; i < len(raw); i++ {
		if raw[i] != '%' {
			out = append(out, raw[i])
			continue
		}
		if i+2 >= len(raw) {
			return "", errMalformedFixture
		}
		hi, ok1 := hexValue(raw[i+1])
		lo, ok2 := hexValue(raw[i+2])
		if !ok1 || !ok2 {
			return "", errMalformedFixture
		}
		out = append(out, hi<<4|lo)
		i += 2
	}
	return string(out), nil
}

func buildInput(args fields) (string, error) {
	text, err := decodeFixture(args["text"])
	if err != nil {
		return "", err
	}
	if _, ok := args["text"]; !ok {
		return "", errMalformedFixture
	}
	rawRepeat, repeated := args["repeat"]
	_, hasUnit := args["unit"]
	_, hasTail := args["tail"]
	if !repeated {
		if hasUnit || hasTail {
			return "", errMalformedFixture
		}
		return text, nil
	}
	n, err := strconv.Atoi(rawRepeat)
	if err != nil || !hasUnit || n < 1 || n > maxRepeat || strconv.Itoa(n) != rawRepeat {
		return "", errMalformedFixture
	}
	unit, err := decodeFixture(args["unit"])
	if err != nil || unit == "" {
		return "", errMalformedFixture
	}
	tail, err := decodeFixture(args["tail"])
	if err != nil {
		return "", err
	}
	return text + strings.Repeat(unit, n) + tail, nil
}

func operand(raw string) (string, string, error) {
	decoded, err := decodeFixture(raw)
	if err != nil {
		return "", "", err
	}
	kind, text, ok := strings.Cut(decoded, ":")
	if !ok || (kind != "KEY" && kind != "QR" && kind != "XML") {
		return "", "", errMalformedFixture
	}
	return kind, text, nil
}

func check(kind, text string) fiscal.Result {
	switch kind {
	case "KEY":
		return fiscal.CheckAccessKey(text)
	case "QR":
		return fiscal.CheckQRCode(text)
	}
	return fiscal.CheckXML([]byte(text))
}

func joinChecks(checks []fiscal.Check) string {
	out := make([]string, len(checks))
	for i, c := range checks {
		out[i] = string(c)
	}
	return strings.Join(out, ",")
}

func amountFields(out fields, a fiscal.Amount, present bool) {
	out["total"], out["centavos"] = "?", "?"
	if !present {
		return
	}
	out["total"] = a.Text()
	if n, ok := a.Centavos(); ok {
		out["centavos"] = strconv.FormatInt(n, 10)
	}
}

func project(r fiscal.Result) fields {
	reason := "-"
	if r.Reason() != "" {
		reason = string(r.Reason())
	}
	out := fields{"kind": string(r.Kind()), "outcome": string(r.Outcome()), "reason": reason, "checks": joinChecks(r.Checks())}
	k, hasKey := r.AccessKey()
	q, hasQR := r.QRCode()
	x, hasXML := r.XML()
	if r.Outcome() != fiscal.StructureOK {
		if hasKey || hasQR || hasXML {
			out["leak"] = "true"
		}
		return out
	}
	if !hasKey || hasQR != (r.Kind() == fiscal.QRCodeInput) || hasXML != (r.Kind() == fiscal.XMLInput) {
		out["shape"] = "unexpected"
	}
	out["key"] = k.Text()
	switch r.Kind() {
	case fiscal.AccessKeyInput:
		out["uf"], out["year_month"], out["issuer"], out["model"] = k.State(), k.YearMonth(), k.Issuer(), k.Model()
		out["series"], out["number"], out["emission"] = k.Series(), k.Number(), k.EmissionType()
		out["code"], out["digit"] = k.NumericCode(), k.CheckDigit()
	case fiscal.QRCodeInput:
		if q.AccessKey().Text() != k.Text() {
			out["shape"] = "key"
		}
		out["version"], out["environment"], out["layout"] = q.Version(), string(q.Environment()), string(q.Layout())
		total, ok := q.Total()
		amountFields(out, total, ok)
	case fiscal.XMLInput:
		if x.AccessKey().Text() != k.Text() {
			out["shape"] = "key"
		}
		out["items"] = strconv.Itoa(x.Items())
		amountFields(out, x.Total(), true)
	}
	return out
}

func feed(t *testing.T, r fiscal.Result) fields {
	t.Helper()
	id, err := kernel.ParseOrgID("9d000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	d, err := doc.New(scope, "9d000000-0000-4000-8000-000000000201", strings.Repeat("1a", 32))
	if err != nil {
		t.Fatal(err)
	}
	if d, err = d.Apply(doc.Transition{Dimension: doc.Structure, From: doc.NotApplicable, To: doc.Unchecked}); err != nil {
		t.Fatal(err)
	}
	transition := "-"
	if command, ok := r.StructureTransition(); ok {
		transition = string(command.To)
		if d, err = d.Apply(command); err != nil {
			t.Fatal("structure command refused by the document domain", err)
		}
	}
	s := d.Statuses()
	consulted := "?"
	if d.ConsultedAt() != nil {
		consulted = "present"
	}
	return fields{"outcome": string(r.Outcome()), "transition": transition, "structure": string(s.Structure),
		"fiscal": string(s.Fiscal), "consulted_at": consulted}
}

func runVector(t *testing.T, operation string, args fields) (fields, error) {
	t.Helper()
	switch operation {
	case "KEY", "QR", "XML":
		text, err := buildInput(args)
		if err != nil {
			t.Fatal(err)
		}
		return project(check(operation, text)), nil
	case "COMPARE":
		lk, lt, err := operand(args["left"])
		if err != nil {
			t.Fatal(err)
		}
		rk, rt, err := operand(args["right"])
		if err != nil {
			t.Fatal(err)
		}
		r, err := fiscal.Compare(check(lk, lt), check(rk, rt))
		if err != nil {
			return nil, err
		}
		return project(r), nil
	case "FEED":
		kind, text, err := operand(args["input"])
		if err != nil {
			t.Fatal(err)
		}
		return feed(t, check(kind, text)), nil
	}
	t.Fatal("unknown operation")
	return nil, nil
}

func errorCode(err error) string {
	if errors.Is(err, fiscal.ErrNotComparable) {
		return "NOT_COMPARABLE"
	}
	return "UNEXPECTED"
}

func TestSharedReaderRejectsMalformedFixtures(t *testing.T) {
	for _, raw := range []string{"%", "%7", "%7c", "%G0", "abc%"} {
		if _, err := decodeFixture(raw); err == nil {
			t.Fatal("malformed fixture escape accepted", raw)
		}
	}
	for _, args := range []fields{{"unit": "a"}, {"text": "", "repeat": "2"}, {"text": "", "unit": "a", "repeat": "0"},
		{"text": "", "unit": "a", "repeat": "02"}, {"text": "", "unit": "", "repeat": "2"}, {"text": "", "tail": "x"},
		{"text": "", "unit": "a", "repeat": "2000001"}} {
		if _, err := buildInput(args); err == nil {
			t.Fatal("malformed fixture input accepted", args)
		}
	}
	for _, raw := range []string{"KEY", "PDF:x", "key:x", ":x"} {
		if _, _, err := operand(raw); err == nil {
			t.Fatal("malformed fixture operand accepted", raw)
		}
	}
	if _, err := fieldMap("a=1;a=2"); err == nil {
		t.Fatal("duplicate field accepted")
	}
	if _, err := fieldMap("novalue"); err == nil {
		t.Fatal("field without equals accepted")
	}
}

func sortedKeys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestSharedFiscalVectors(t *testing.T) {
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != corpusHash {
		t.Fatal("shared corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared corpus header")
	}
	wantCounts := map[string]int{"KEY": 64, "QR": 110, "XML": 111, "COMPARE": 13, "FEED": 9}
	counts, ids := map[string]int{}, map[string]bool{}
	seenOutcomes, seenReasons, seenChecks, seenErrors := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	goOnly := 0
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case")
		}
		allowed, ok := operations[p[1]]
		if !ok {
			t.Fatal("unknown shared operation")
		}
		ids[p[0]] = true
		counts[p[1]]++
		args, err := fieldMap(p[2])
		if err != nil {
			t.Fatal(p[0], err)
		}
		want, err := fieldMap(p[3])
		if err != nil {
			t.Fatal(p[0], err)
		}
		for k := range args {
			if !strings.Contains(" "+allowed+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument", p[0], k)
			}
		}
		if p[1] == "XML" || strings.Contains(p[2], "=XML:") {
			goOnly++
		}
		if code, ok := want["error"]; ok {
			seenErrors[code] = true
		}
		if v, ok := want["outcome"]; ok {
			seenOutcomes[v] = true
		}
		if v, ok := want["reason"]; ok && v != "-" {
			seenReasons[v] = true
			if reasonOutcomes[v] != want["outcome"] {
				t.Fatal("oracle reason paired with another outcome", p[0])
			}
		}
		if v, ok := want["checks"]; ok {
			for _, c := range strings.Split(v, ",") {
				seenChecks[c] = true
			}
		}
		t.Run(p[0], func(t *testing.T) {
			got, err := runVector(t, p[1], args)
			if err != nil {
				got = fields{"error": errorCode(err)}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shared mismatch:\n got %v\nwant %v", got, want)
			}
		})
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", counts)
	}
	if goOnly != goOnlyRows {
		t.Fatal("Go-only row count differs from the README", goOnly)
	}
	reasons := []string{}
	for r := range reasonOutcomes {
		reasons = append(reasons, r)
	}
	if !reflect.DeepEqual(sortedKeys(seenReasons), sortedCopy(reasons)) {
		t.Fatal("reason coverage differs", sortedKeys(seenReasons))
	}
	if !reflect.DeepEqual(sortedKeys(seenChecks), sortedCopy(allChecks)) {
		t.Fatal("check coverage differs", sortedKeys(seenChecks))
	}
	if !reflect.DeepEqual(sortedKeys(seenOutcomes), sortedCopy(allOutcomes)) {
		t.Fatal("outcome coverage differs", sortedKeys(seenOutcomes))
	}
	if !reflect.DeepEqual(sortedKeys(seenErrors), []string{"NOT_COMPARABLE"}) {
		t.Fatal("error coverage differs", sortedKeys(seenErrors))
	}
}

func TestReasonOutcomesMatchOracle(t *testing.T) {
	for raw, outcome := range reasonOutcomes {
		if got := string(fiscal.Reason(raw).Outcome()); got != outcome {
			t.Fatal("reason outcome differs", raw, got)
		}
	}
	for _, raw := range []string{"", "VALID", "AUTHORIZED", "key_length", "KEY_LENGTH "} {
		if fiscal.Reason(raw).Outcome() != "" {
			t.Fatal("unknown reason has an outcome", raw)
		}
	}
}
