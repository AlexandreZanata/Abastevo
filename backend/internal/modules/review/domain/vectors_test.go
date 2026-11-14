package domain_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
)

const (
	corpusPath   = "../../../../../contracts/testdata/enterprise/review/review-v1.psv"
	corpusSHA256 = "fac6c92c9c6929dc59f724b6f9912149d2ec92206138ada14beeec2739dffa08"
	corpusRows   = 232
	setupReason  = "Synthetic reason"
	authorSpec   = "a:1,m:11,e:21"
	reviewerSpec = "a:2,m:12,e:22"
)

var (
	wantOperations = map[string]int{"STATE": 16, "COMMAND": 15, "APPLY": 155, "REPLAY": 22, "PRESENT": 24}
	allowedKeys    = map[string]string{
		"STATE":   "value_hex",
		"COMMAND": "value_hex",
		"APPLY":   "steps command revision reason_hex reason_repeat actor author beneficiary permitted",
		"REPLAY":  "history",
		"PRESENT": "server local",
	}
	requiredKeys = map[string]string{"STATE": "value_hex", "COMMAND": "value_hex", "APPLY": "command", "REPLAY": "history", "PRESENT": "server local"}
	errorCodes   = map[string]error{
		"INVALID":            review.ErrInvalid,
		"REASON_REQUIRED":    review.ErrReasonRequired,
		"FORBIDDEN":          review.ErrForbidden,
		"SELF_APPROVAL":      review.ErrSelfApproval,
		"CONFLICT":           review.ErrConflict,
		"INVALID_TRANSITION": review.ErrInvalidTransition,
	}
	allStates = []review.State{review.Draft, review.Submitted, review.InReview, review.Approved, review.Returned, review.Rejected, review.Cancelled}
	allKinds  = []review.Kind{review.Submit, review.Cancel, review.Resubmit, review.Open, review.Approve, review.Return, review.Reject}
)

type fields map[string]string

var errFixture = errors.New("fixture: malformed grammar")

func fieldMap(raw string) (fields, error) {
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" || k != strings.TrimSpace(k) {
			return nil, errFixture
		}
		if _, has := out[k]; has {
			return nil, errFixture
		}
		out[k] = v
	}
	return out, nil
}

func text(raw string) (string, error) {
	v, e := hex.DecodeString(raw)
	if e != nil || raw != strings.ToLower(raw) || !utf8.Valid(v) {
		return "", errFixture
	}
	return string(v), nil
}

func uuid(n int64) string { return fmt.Sprintf("9e000000-0000-4000-8000-%012d", n) }

func decimal(raw string) (int64, bool) {
	if raw == "" || len(raw) > 12 {
		return 0, false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	return n, e == nil
}

// identity reads `a:N,m:N,e:N`; a decimal N is a fixture UUID, anything else
// is literal input for the domain to validate. The empty spec is an empty set.
func identity(spec string) (review.Identity, error) {
	var out review.Identity
	if spec == "" {
		return out, nil
	}
	for _, part := range strings.Split(spec, ",") {
		kind, value, ok := strings.Cut(part, ":")
		if !ok {
			return out, errFixture
		}
		if n, ok := decimal(value); ok {
			value = uuid(n)
		}
		switch kind {
		case "a":
			out.Accounts = append(out.Accounts, value)
		case "m":
			out.Memberships = append(out.Memberships, value)
		case "e":
			out.Employees = append(out.Employees, value)
		default:
			return out, errFixture
		}
	}
	return out, nil
}

func mustIdentity(t *testing.T, spec string) review.Identity {
	t.Helper()
	v, e := identity(spec)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func defaultFacts(t *testing.T, kind review.Kind) review.Facts {
	t.Helper()
	actor := reviewerSpec
	if kind.Role() == review.RoleAuthor {
		actor = authorSpec
	}
	return review.Facts{Actor: mustIdentity(t, actor), Author: mustIdentity(t, authorSpec), Permitted: true}
}

func historyText(entries []review.Entry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%s:%d:%s:%s", e.Kind, e.Revision, e.State, hex.EncodeToString([]byte(e.Reason))))
	}
	return strings.Join(parts, "~")
}

func parseHistory(raw string) ([]review.Entry, error) {
	if raw == "" {
		return nil, nil
	}
	var out []review.Entry
	for _, item := range strings.Split(raw, "~") {
		p := strings.Split(item, ":")
		if len(p) != 4 {
			return nil, errFixture
		}
		rev, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil {
			return nil, errFixture
		}
		reason, e := text(p[3])
		if e != nil {
			return nil, errFixture
		}
		out = append(out, review.Entry{Kind: review.Kind(p[0]), Revision: rev, State: review.State(p[2]), Reason: reason})
	}
	return out, nil
}

type pair struct {
	state review.State
	kind  review.Kind
}

func caseFields(c review.Case) fields {
	return fields{"state": string(c.State()), "revision": strconv.FormatInt(c.Revision(), 10), "history": historyText(c.History())}
}

type caseSnapshot struct {
	state    review.State
	revision int64
	history  []review.Entry
}

func snapshot(c review.Case) caseSnapshot { return caseSnapshot{c.State(), c.Revision(), c.History()} }

func sameCase(a, b review.Case) bool { return reflect.DeepEqual(snapshot(a), snapshot(b)) }

func runApply(t *testing.T, f fields, pairs map[pair]bool) (fields, error) {
	t.Helper()
	c := review.NewDraft()
	if steps := f["steps"]; steps != "" {
		for _, step := range strings.Split(steps, ",") {
			kind := review.Kind(step)
			cmd := review.Command{Kind: kind, Revision: c.Revision()}
			if kind == review.Return || kind == review.Reject {
				cmd.Reason = setupReason
			}
			next, e := c.Apply(cmd, defaultFacts(t, kind))
			if e != nil {
				t.Fatalf("fixture setup step %s refused: %v", step, e)
			}
			c = next
		}
	}
	kind := review.Kind(f["command"])
	cmd := review.Command{Kind: kind, Revision: c.Revision()}
	if raw, ok := f["revision"]; ok {
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil {
			t.Fatal("malformed fixture revision")
		}
		cmd.Revision = n
	}
	if raw, ok := f["reason_hex"]; ok {
		unit, e := text(raw)
		if e != nil {
			t.Fatal(e)
		}
		repeat := 1
		if r, ok := f["reason_repeat"]; ok {
			n, ok := decimal(r)
			if !ok || n < 1 {
				t.Fatal("malformed fixture repeat")
			}
			repeat = int(n)
		}
		cmd.Reason = strings.Repeat(unit, repeat)
	} else if _, ok := f["reason_repeat"]; ok {
		t.Fatal("repeat without reason")
	}
	facts := defaultFacts(t, kind)
	if raw, ok := f["actor"]; ok {
		facts.Actor = mustIdentity(t, raw)
	}
	if raw, ok := f["author"]; ok {
		facts.Author = mustIdentity(t, raw)
	}
	if raw, ok := f["beneficiary"]; ok && raw != "?" {
		b := mustIdentity(t, raw)
		facts.Beneficiary = &b
	}
	if raw, ok := f["permitted"]; ok {
		switch raw {
		case "true":
		case "false", "?":
			facts.Permitted = false
		default:
			t.Fatal("malformed fixture permission")
		}
	}
	if _, e := review.ParseKind(string(kind)); e == nil {
		pairs[pair{c.State(), kind}] = true
	}
	before := snapshot(c)
	next, e := c.Apply(cmd, facts)
	if e != nil {
		if !reflect.DeepEqual(snapshot(c), before) {
			t.Fatal("a refused command changed the input case")
		}
		if next.State() != "" || next.Revision() != 0 || next.History() != nil {
			t.Fatal("a refused command produced a case")
		}
		return nil, e
	}
	replayed, re := review.Replay(next.History())
	if re != nil || !sameCase(replayed, next) {
		t.Fatal("accepted history does not replay to the same case", re)
	}
	if !reflect.DeepEqual(snapshot(c), before) {
		t.Fatal("an accepted command changed the input case")
	}
	return caseFields(next), nil
}

func runVector(t *testing.T, op string, f fields, pairs map[pair]bool) (fields, error) {
	t.Helper()
	switch op {
	case "STATE":
		raw, e := text(f["value_hex"])
		if e != nil {
			t.Fatal(e)
		}
		s, e := review.ParseState(raw)
		if e != nil {
			return nil, e
		}
		basis, e := s.Basis()
		if e != nil {
			return nil, e
		}
		return fields{"state": string(s), "terminal": strconv.FormatBool(s.Terminal()), "basis": string(basis)}, nil
	case "COMMAND":
		raw, e := text(f["value_hex"])
		if e != nil {
			t.Fatal(e)
		}
		k, e := review.ParseKind(raw)
		if e != nil {
			return nil, e
		}
		return fields{"command": string(k), "role": string(k.Role()), "reason": string(k.ReasonPolicy())}, nil
	case "APPLY":
		return runApply(t, f, pairs)
	case "REPLAY":
		entries, e := parseHistory(f["history"])
		if e != nil {
			t.Fatal(e)
		}
		c, e := review.Replay(entries)
		if e != nil {
			return nil, e
		}
		return fields{"state": string(c.State()), "revision": strconv.FormatInt(c.Revision(), 10), "entries": strconv.Itoa(len(c.History()))}, nil
	case "PRESENT":
		var server *review.State
		var local *review.Kind
		if raw := f["server"]; raw != "?" {
			s := review.State(raw)
			server = &s
		}
		if raw := f["local"]; raw != "?" {
			k := review.Kind(raw)
			local = &k
		}
		p, e := review.Present(server, local)
		if e != nil {
			return nil, e
		}
		return fields{"state": string(p.State), "server_confirmed": strconv.FormatBool(p.ServerConfirmed), "ready_to_send": strconv.FormatBool(p.ReadyToSend)}, nil
	}
	t.Fatal("unknown operation")
	return nil, nil
}

func errorFields(e error) fields {
	for code, sentinel := range errorCodes {
		if errors.Is(e, sentinel) {
			return fields{"error": code}
		}
	}
	return fields{"error": "UNEXPECTED"}
}

type row struct {
	id, op     string
	args, want fields
}

func loadCorpus(raw []byte) ([]row, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		return nil, errFixture
	}
	var rows []row
	ids := map[string]bool{}
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || p[0] != strings.TrimSpace(p[0]) || ids[p[0]] {
			return nil, errFixture
		}
		keys, ok := allowedKeys[p[1]]
		if !ok {
			return nil, errFixture
		}
		args, e := fieldMap(p[2])
		if e != nil {
			return nil, e
		}
		want, e := fieldMap(p[3])
		if e != nil {
			return nil, e
		}
		for k := range args {
			if !strings.Contains(" "+keys+" ", " "+k+" ") {
				return nil, errFixture
			}
		}
		for _, k := range strings.Fields(requiredKeys[p[1]]) {
			if _, ok := args[k]; !ok {
				return nil, errFixture
			}
		}
		if code, ok := want["error"]; ok {
			if _, known := errorCodes[code]; !known || len(want) != 1 {
				return nil, errFixture
			}
		}
		ids[p[0]] = true
		rows = append(rows, row{p[0], p[1], args, want})
	}
	return rows, scanner.Err()
}

func TestSharedReaderRejectsMalformedGrammar(t *testing.T) {
	header := "id|operation|args|expected\n"
	for name, body := range map[string]string{
		"no header":         "x|STATE|value_hex=|error=INVALID\n",
		"unknown operation": header + "x|OTHER|value_hex=|error=INVALID\n",
		"unknown key":       header + "x|STATE|value=|error=INVALID\n",
		"missing key":       header + "x|APPLY|steps=SUBMIT|error=INVALID\n",
		"duplicate id":      header + "x|STATE|value_hex=|error=INVALID\nx|STATE|value_hex=|error=INVALID\n",
		"duplicate field":   header + "x|STATE|value_hex=;value_hex=|error=INVALID\n",
		"blank id":          header + " |STATE|value_hex=|error=INVALID\n",
		"padded id":         header + "x |STATE|value_hex=|error=INVALID\n",
		"unknown error":     header + "x|STATE|value_hex=|error=UNEXPECTED\n",
		"error with fields": header + "x|STATE|value_hex=|error=INVALID;state=DRAFT\n",
		"five columns":      header + "x|STATE|value_hex=|error=INVALID|\n",
		"field without =":   header + "x|STATE|value_hex|error=INVALID\n",
	} {
		if _, e := loadCorpus([]byte(body)); e == nil {
			t.Fatal("malformed corpus accepted:", name)
		}
	}
	for _, raw := range []string{"ff", "4", "4A", "eda080"} {
		if _, e := text(raw); e == nil {
			t.Fatal("malformed fixture hex accepted", raw)
		}
	}
	for _, spec := range []string{"x:1", "a1", "a:1,"} {
		if _, e := identity(spec); e == nil {
			t.Fatal("malformed fixture identity accepted", spec)
		}
	}
	for _, raw := range []string{"SUBMIT:1:SUBMITTED", "SUBMIT:x:SUBMITTED:", "SUBMIT:1:SUBMITTED:zz"} {
		if _, e := parseHistory(raw); e == nil {
			t.Fatal("malformed fixture history accepted", raw)
		}
	}
}

func TestSharedReviewVectors(t *testing.T) {
	raw, e := os.ReadFile(corpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != corpusSHA256 {
		t.Fatal("shared corpus hash changed")
	}
	rows, e := loadCorpus(raw)
	if e != nil {
		t.Fatal(e)
	}
	counts := map[string]int{}
	codes := map[string]bool{}
	for _, r := range rows {
		counts[r.op]++
		if code, ok := r.want["error"]; ok {
			codes[code] = true
		}
	}
	if len(rows) != corpusRows || !reflect.DeepEqual(counts, wantOperations) {
		t.Fatal("incomplete corpus", len(rows), counts)
	}
	if len(codes) != len(errorCodes) {
		t.Fatal("every closed error code needs a vector", codes)
	}
	pairs := map[pair]bool{}
	for _, r := range rows {
		t.Run(r.id, func(t *testing.T) {
			got, err := runVector(t, r.op, r.args, pairs)
			if err != nil {
				got = errorFields(err)
			}
			if !reflect.DeepEqual(got, r.want) {
				t.Fatalf("shared mismatch: got %v expected %v", got, r.want)
			}
		})
	}
	for _, s := range allStates {
		for _, k := range allKinds {
			if !pairs[pair{s, k}] {
				t.Errorf("no APPLY vector exercises %s from %s", k, s)
			}
		}
	}
}
