package domain_test

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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const corpusHash = "27eaec5e45b641ab7ee31b70681d96690560a12a655f2f43d0f2cc2f55f1e383"

type fields map[string]string

var errUnknownEnumSelector = errors.New("fixture: unknown enum namespace")

func validateEnumSelector(raw string) error {
	switch raw {
	case "DIMENSION", "STORAGE", "EXTRACTION", "STRUCTURE", "CONFERENCE", "RECONCILIATION", "FISCAL", "TARGET":
		return nil
	}
	return errUnknownEnumSelector
}

func TestSharedReaderRejectsUnknownEnumSelector(t *testing.T) {
	for _, raw := range []string{"", "STATE", "storage", "STORAGE ", "VALIDATED"} {
		if !errors.Is(validateEnumSelector(raw), errUnknownEnumSelector) {
			t.Fatal("malformed fixture namespace reached domain dispatch")
		}
	}
	for _, raw := range []string{"DIMENSION", "STORAGE", "EXTRACTION", "STRUCTURE", "CONFERENCE", "RECONCILIATION", "FISCAL", "TARGET"} {
		if validateEnumSelector(raw) != nil {
			t.Fatal("documented fixture namespace refused")
		}
	}
}

var aliases = map[string]int{"A": 1, "B": 2, "D": 201, "N": 202, "E1": 211, "E2": 212, "E3": 213, "F1": 221, "W1": 231, "T1": 241, "S1": 251, "P1": 261, "R1": 271}
var digests = map[string]string{"H1": strings.Repeat("1a", 32), "H2": strings.Repeat("2b", 32)}

func uuid(n int) string { return fmt.Sprintf("9d000000-0000-4000-8000-%012d", n) }
func fixtureID(raw string) string {
	if n, ok := aliases[raw]; ok {
		return uuid(n)
	}
	if raw == "?" {
		return ""
	}
	return raw
}
func alias(id string) string {
	for k, n := range aliases {
		if uuid(n) == id {
			return k
		}
	}
	return id
}
func fixtureDigest(raw string) string {
	if v, ok := digests[raw]; ok {
		return v
	}
	if raw == "?" {
		return ""
	}
	return raw
}
func digestAlias(raw string) string {
	for k, v := range digests {
		if v == raw {
			return k
		}
	}
	return raw
}
func absent(raw string) string {
	if raw == "" {
		return "?"
	}
	return raw
}
func (f fields) get(key, other string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return other
}
func fieldMap(t *testing.T, raw string) fields {
	t.Helper()
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			t.Fatal("malformed fixture field")
		}
		if _, has := out[k]; has {
			t.Fatal("duplicate fixture field")
		}
		out[k] = v
	}
	return out
}
func vectorScope(raw string) (kernel.OrgScope, error) {
	id, e := kernel.ParseOrgID(fixtureID(raw))
	if e != nil {
		return kernel.OrgScope{}, doc.ErrScope
	}
	s, e := kernel.NewOrgScope(id)
	if e != nil {
		return kernel.OrgScope{}, doc.ErrScope
	}
	return s, nil
}
func vectorTarget(t *testing.T, raw, owner string) (doc.Target, error) {
	t.Helper()
	kindRaw, rest, ok := strings.Cut(raw, "/")
	if !ok {
		t.Fatal("malformed fixture target")
	}
	idRaw, org, scoped := strings.Cut(rest, "@")
	if !scoped {
		org = owner
	}
	kind, e := doc.ParseTargetKind(kindRaw)
	if e != nil {
		return doc.Target{}, e
	}
	s, e := vectorScope(org)
	if e != nil {
		return doc.Target{}, e
	}
	return doc.NewTarget(s, kind, fixtureID(idRaw))
}
func targetText(target doc.Target) string { return string(target.Kind()) + "/" + alias(target.ID()) }

func applyStep(t *testing.T, d doc.Document, owner, step string, match *string) (doc.Document, error) {
	t.Helper()
	p := strings.Split(step, ",")
	switch p[0] {
	case "T":
		if len(p) != 4 && len(p) != 5 {
			t.Fatal("malformed transition step")
		}
		dim, e := doc.ParseDimension(p[1])
		if e != nil {
			return d, e
		}
		from, e := doc.ParseState(dim, p[2])
		if e != nil {
			return d, e
		}
		to, e := doc.ParseState(dim, p[3])
		if e != nil {
			return d, e
		}
		command := doc.Transition{Dimension: dim, From: from, To: to}
		if len(p) == 5 {
			at, e := orgtime.ParseInstant(p[4])
			if e != nil {
				return d, doc.ErrInvalid
			}
			command.ConsultedAt = &at
		}
		return d.Apply(command)
	case "AMOUNT":
		if len(p) != 2 {
			t.Fatal("malformed amount step")
		}
		n, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil {
			return d, doc.ErrInvalid
		}
		return d.WithAmount(n)
	case "LINK":
		if len(p) != 2 {
			t.Fatal("malformed link step")
		}
		target, e := vectorTarget(t, p[1], owner)
		if e != nil {
			return d, e
		}
		return d.Link(target)
	case "ALLOC":
		lines := []doc.AllocationLine{}
		for _, item := range p[1:] {
			raw, amount, ok := strings.Cut(item, ":")
			if !ok {
				t.Fatal("malformed allocation item")
			}
			target, e := vectorTarget(t, raw, owner)
			if e != nil {
				return d, e
			}
			n, e := strconv.ParseInt(amount, 10, 64)
			if e != nil {
				return d, doc.ErrInvalid
			}
			lines = append(lines, doc.AllocationLine{Target: target, Centavos: n})
		}
		return d.Allocate(lines)
	case "SPLIT":
		targets := []doc.Target{}
		for _, item := range p[1:] {
			target, e := vectorTarget(t, item, owner)
			if e != nil {
				return d, e
			}
			targets = append(targets, target)
		}
		return d.SplitEvenly(targets)
	case "DIGEST":
		if len(p) != 2 {
			t.Fatal("malformed digest step")
		}
		ok, e := d.MatchesOriginal(fixtureDigest(p[1]))
		if e != nil {
			return d, e
		}
		*match = strconv.FormatBool(ok)
		return d, nil
	}
	t.Fatal("unknown fixture step")
	return d, nil
}

func steps(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "~")
}

func run(t *testing.T, org, id, digest, raw string) (doc.Document, string, []string, error) {
	t.Helper()
	s, e := vectorScope(org)
	if e != nil {
		return doc.Document{}, "", nil, e
	}
	d, e := doc.New(s, fixtureID(id), fixtureDigest(digest))
	if e != nil {
		return doc.Document{}, "", nil, e
	}
	match, refused := "?", []string{}
	for i, step := range steps(raw) {
		next, e := applyStep(t, d, org, step, &match)
		if e != nil {
			refused = append(refused, strconv.Itoa(i+1)+":"+errorCode(e))
			continue
		}
		d = next
	}
	return d, match, refused, nil
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}
func linksText(d doc.Document) string {
	out := []string{}
	for _, target := range d.Links() {
		out = append(out, targetText(target))
	}
	return joinOrDash(out)
}
func allocationsText(d doc.Document) string {
	out := []string{}
	for _, line := range d.Allocations() {
		out = append(out, targetText(line.Target)+":"+strconv.FormatInt(line.Centavos, 10))
	}
	return joinOrDash(out)
}
func optionalInt(n *int64) string {
	if n == nil {
		return "?"
	}
	return strconv.FormatInt(*n, 10)
}
func project(d doc.Document, match string, refused []string) fields {
	s := d.Statuses()
	at := "?"
	if v := d.ConsultedAt(); v != nil {
		at = orgtime.FormatInstant(*v)
	}
	return fields{
		"org": alias(d.OrganizationID().String()), "id": alias(d.ID()), "digest": digestAlias(d.Digest()),
		"replaces": absent(alias(d.Replaces())), "replaced_by": absent(alias(d.ReplacedBy())),
		"storage": string(s.Storage), "extraction": string(s.Extraction), "structure": string(s.Structure),
		"conference": string(s.Conference), "reconciliation": string(s.Reconciliation), "fiscal": string(s.Fiscal),
		"consulted_at": at, "amount": optionalInt(d.Amount()), "allocated": strconv.FormatInt(d.Allocated(), 10),
		"remaining": optionalInt(d.Remaining()), "links": linksText(d), "allocations": allocationsText(d),
		"match": match, "refused": joinOrDash(refused),
	}
}

func build(t *testing.T, org, id, digest, raw string) doc.Document {
	t.Helper()
	d, _, refused, e := run(t, org, id, digest, raw)
	if e != nil || len(refused) != 0 {
		t.Fatal("replacement fixture setup refused", e, refused)
	}
	return d
}

func runVector(t *testing.T, operation string, f fields) (fields, error) {
	t.Helper()
	switch operation {
	case "ENUM":
		value := f["value"]
		switch f["kind"] {
		case "DIMENSION":
			d, e := doc.ParseDimension(value)
			if e != nil {
				return nil, e
			}
			return fields{"value": string(d), "derived": strconv.FormatBool(d.Derived())}, nil
		case "TARGET":
			k, e := doc.ParseTargetKind(value)
			if e != nil {
				return nil, e
			}
			return fields{"value": string(k), "allocatable": strconv.FormatBool(k.Allocatable())}, nil
		default:
			dim, e := doc.ParseDimension(f["kind"])
			if e != nil {
				return nil, e
			}
			s, e := doc.ParseState(dim, value)
			if e != nil {
				return nil, e
			}
			initial, e := doc.Initial(dim)
			if e != nil {
				return nil, e
			}
			return fields{"value": string(s), "initial": strconv.FormatBool(s == initial)}, nil
		}
	case "ALLOWED":
		dim, e := doc.ParseDimension(f["dimension"])
		if e != nil {
			return nil, e
		}
		from, e := doc.ParseState(dim, f["from"])
		if e != nil {
			return nil, e
		}
		next, e := doc.Allowed(dim, from)
		if e != nil {
			return nil, e
		}
		out := []string{}
		for _, s := range next {
			out = append(out, string(s))
		}
		return fields{"to": joinOrDash(out)}, nil
	case "SCENARIO":
		d, match, refused, e := run(t, f.get("org", "A"), f.get("id", "D"), f.get("digest", "H1"), f["steps"])
		if e != nil {
			return nil, e
		}
		return project(d, match, refused), nil
	case "REPLACE":
		old := build(t, "A", "D", "H1", f["old_steps"])
		successor := build(t, f.get("new_org", "A"), f.get("new", "N"), f.get("new_digest", "H2"), f["new_steps"])
		o, n, e := doc.Replace(old, successor)
		if e != nil {
			return nil, e
		}
		oldStatus, newStatus := o.Statuses(), n.Statuses()
		return fields{
			"old_id": alias(o.ID()), "old_digest": digestAlias(o.Digest()), "old_storage": string(oldStatus.Storage),
			"old_replaced_by": absent(alias(o.ReplacedBy())), "old_conference": string(oldStatus.Conference), "old_fiscal": string(oldStatus.Fiscal),
			"old_reconciliation": string(oldStatus.Reconciliation), "old_links": linksText(o), "old_allocations": allocationsText(o),
			"new_id": alias(n.ID()), "new_digest": digestAlias(n.Digest()), "new_storage": string(newStatus.Storage),
			"new_replaces": absent(alias(n.Replaces())), "new_links": linksText(n),
		}, nil
	}
	t.Fatal("unknown operation")
	return nil, nil
}

var errorCodes = []struct {
	err  error
	code string
}{
	{doc.ErrInvalid, "INVALID"}, {doc.ErrScope, "SCOPE"}, {doc.ErrTransition, "INVALID_TRANSITION"},
	{doc.ErrConflict, "CONFLICT"}, {doc.ErrDerived, "DERIVED"}, {doc.ErrDuplicate, "DUPLICATE"},
	{doc.ErrNotLinked, "NOT_LINKED"}, {doc.ErrNotAllocatable, "NOT_ALLOCATABLE"},
	{doc.ErrExceedsAmount, "EXCEEDS_AMOUNT"}, {doc.ErrAmountMissing, "AMOUNT_MISSING"},
}

func errorCode(e error) string {
	for _, c := range errorCodes {
		if errors.Is(e, c.err) {
			return c.code
		}
	}
	return "UNEXPECTED"
}

func TestSharedDocumentVectors(t *testing.T) {
	raw, e := os.ReadFile("../../../../../contracts/testdata/enterprise/document/document-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != corpusHash {
		t.Fatal("shared corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared corpus header")
	}
	allowed := map[string]string{"ENUM": "kind value", "ALLOWED": "dimension from", "SCENARIO": "org id digest steps", "REPLACE": "old_steps new new_org new_digest new_steps"}
	wantCounts := map[string]int{"ENUM": 64, "ALLOWED": 29, "SCENARIO": 146, "REPLACE": 12}
	counts, ids, seenCodes := map[string]int{}, map[string]bool{}, map[string]bool{}
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case")
		}
		if _, ok := wantCounts[p[1]]; !ok {
			t.Fatal("unknown shared operation")
		}
		ids[p[0]] = true
		counts[p[1]]++
		args, want := fieldMap(t, p[2]), fieldMap(t, p[3])
		if p[1] == "ENUM" && validateEnumSelector(args["kind"]) != nil {
			t.Fatal("malformed fixture enum namespace")
		}
		for k := range args {
			if !strings.Contains(" "+allowed[p[1]]+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument")
			}
		}
		if code, ok := want["error"]; ok {
			seenCodes[code] = true
		}
		if r, ok := want["refused"]; ok && r != "-" {
			for _, item := range strings.Split(r, ",") {
				_, code, _ := strings.Cut(item, ":")
				seenCodes[code] = true
			}
		}
		t.Run(p[0], func(t *testing.T) {
			got, err := runVector(t, p[1], args)
			if err != nil {
				got = fields{"error": errorCode(err)}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shared mismatch: got %v expected %v", got, want)
			}
		})
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", counts)
	}
	all := []string{}
	for _, c := range errorCodes {
		all = append(all, c.code)
	}
	seen := []string{}
	for c := range seenCodes {
		seen = append(seen, c)
	}
	sort.Strings(all)
	sort.Strings(seen)
	if !reflect.DeepEqual(all, seen) {
		t.Fatal("error code coverage differs", seen)
	}
}
