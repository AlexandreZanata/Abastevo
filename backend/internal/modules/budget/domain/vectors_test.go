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

	budget "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/budget/domain"
	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	corpusPath = "../../../../../contracts/testdata/enterprise/budget/budget-v1.psv"
	corpusHash = "fb9b99c33a0e82a131d2ae375276bd4b49380cfdb30ed479f089080fbaf03985"
	defaultAt  = "2026-11-14T12:00:00Z"
)

type fields map[string]string

var (
	errMalformedFixture = errors.New("fixture: malformed grammar")
	wantCounts          = map[string]int{"ENUM": 24, "LINE": 44, "CAPACITY": 94, "RESERVE": 40, "POST": 35, "SEQUENCE": 24}
	errorCodes          = map[string]error{"INVALID": budget.ErrInvalid, "SCOPE": budget.ErrScope,
		"OUT_OF_PERIOD": budget.ErrOutOfPeriod, "DIMENSION_MISMATCH": budget.ErrDimension, "DUPLICATE": budget.ErrDuplicate,
		"HOLD_NOT_ACTIVE": budget.ErrHoldNotActive, "NOT_APPROVED": budget.ErrNotApproved,
		"STALE_AVAILABLE": budget.ErrStaleAvailable}
	outcomes    = []string{"ACTIVATE", "EXCEEDS", "POST", "RELEASED", "EXPIRED"}
	lineKeys    = "org line_id line_id_org start end dimension target target_kind target_org filter limit"
	factKeys    = lineKeys + " line_org zone unit_zone items holds"
	allowedArgs = map[string]string{
		"ENUM":     "kind value_hex",
		"LINE":     lineKeys,
		"CAPACITY": factKeys + " claimed",
		"RESERVE":  factKeys + " decide_org hold hold_org hold_line amount at category unit cc asset employee tag_org",
		"POST":     factKeys + " decide_org post hold hold_org",
		"SEQUENCE": factKeys + " steps",
	}
	itemKeys = "id org origin source category paid company at state unit cc asset employee tag_org"
	holdKeys = "id org line amount at state category unit cc asset employee tag_org by by_org"
	stepKeys = map[string]string{"RESERVE": "do id org line amount at category unit cc asset employee tag_org",
		"RELEASE": "do id", "EXPIRE": "do id", "POST": "do hold " + itemKeys}
	tagKeys = []struct {
		key  string
		kind budget.DimensionKind
	}{{"unit", budget.DimensionUnit}, {"cc", budget.DimensionCostCentre}, {"asset", budget.DimensionAsset},
		{"employee", budget.DimensionEmployee}}
	aliases = map[string]int{"A": 1, "B": 2, "E": 11, "E2": 12, "E3": 13, "E4": 14, "E5": 15, "E6": 16, "E7": 17,
		"E8": 18, "E9": 19, "F": 21, "F2": 22, "F3": 23, "L": 101, "L2": 102, "H": 201, "H2": 202, "H3": 203, "H4": 204,
		"U": 301, "U2": 302, "C": 311, "C2": 312, "V": 321, "V2": 322, "D": 331, "D2": 332}
)

func uuid(n int) string { return fmt.Sprintf("9e000000-0000-4000-8000-%012d", n) }

func fixtureID(name string) string {
	if n, ok := aliases[name]; ok {
		return uuid(n)
	}
	return name
}

func alias(id string) string {
	for name, n := range aliases {
		if uuid(n) == id {
			return name
		}
	}
	return id
}

func (f fields) get(key, other string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return other
}

// split parses `key<kv>value` pairs separated by sep, splitting at the first kv.
func split(raw, sep, kv string) (fields, error) {
	out := fields{}
	for _, part := range strings.Split(raw, sep) {
		k, v, ok := strings.Cut(part, kv)
		if !ok || strings.TrimSpace(k) == "" || k != strings.TrimSpace(k) {
			return nil, errMalformedFixture
		}
		if _, has := out[k]; has {
			return nil, errMalformedFixture
		}
		out[k] = v
	}
	return out, nil
}

func decodeHex(raw string) (string, error) {
	v, e := hex.DecodeString(raw)
	if e != nil || raw != strings.ToLower(raw) || !utf8.Valid(v) {
		return "", errMalformedFixture
	}
	return string(v), nil
}

func allowed(f fields, keys string) error {
	for k := range f {
		if !strings.Contains(" "+keys+" ", " "+k+" ") {
			return errMalformedFixture
		}
	}
	return nil
}

func TestSharedReaderRejectsMalformedGrammar(t *testing.T) {
	for _, raw := range []string{"novalue", "=x", "a=1;a=2", " =1", " a=1", "a =1"} {
		if _, e := split(raw, ";", "="); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed field map accepted", raw)
		}
	}
	for _, raw := range []string{"id", "id:E,id:E2", ":E", "id:E,,company:1"} {
		if _, e := split(raw, ",", ":"); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed item accepted", raw)
		}
	}
	if f, e := split("at:2026-11-14T12:00:00Z", ",", ":"); e != nil || f["at"] != defaultAt {
		t.Fatal("instant value must split at the first colon only")
	}
	for _, raw := range []string{"ff", "eda080", "4F", "4"} {
		if _, e := decodeHex(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed fixture hex accepted", raw)
		}
	}
	if allowed(fields{"id": "E", "colour": "x"}, itemKeys) == nil || allowed(fields{"org": "A"}, "org") != nil {
		t.Fatal("argument allowlist is not strict")
	}
	if errorFields(errors.New("foreign"))["error"] != "UNEXPECTED" || errorFields(nil)["error"] != "UNEXPECTED" {
		t.Fatal("foreign error mapped to a documented code")
	}
	for code, target := range errorCodes {
		if errorFields(fmt.Errorf("wrapped: %w", target))["error"] != code {
			t.Fatal("documented error not mapped", code)
		}
	}
}

// vectorScope leaves a malformed organization as the zero scope so that the
// domain itself, not the fixture reader, refuses it.
func vectorScope(raw string) kernel.OrgScope {
	id, e := kernel.ParseOrgID(fixtureID(raw))
	if e != nil {
		return kernel.OrgScope{}
	}
	s, _ := kernel.NewOrgScope(id)
	return s
}

func optional(f fields, key string) *string {
	v, ok := f[key]
	if !ok || v == "?" {
		return nil
	}
	return &v
}

func zones(f fields) orgtime.ReportingZones {
	return orgtime.ReportingZones{Organization: f.get("zone", "America/Sao_Paulo"), Unit: optional(f, "unit_zone")}
}

func instant(t *testing.T, raw string) orgtime.Declaration {
	t.Helper()
	at, e := orgtime.ParseInstant(raw)
	if e != nil {
		t.Fatal("fixture instant", raw, e)
	}
	return orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}
}

// vectorLine builds the line in scope org: reference, optional target, dimension, line.
func vectorLine(f fields, org string) (budget.Line, error) {
	s := vectorScope(org)
	ref, e := budget.NewLineRef(vectorScope(f.get("line_id_org", org)), fixtureID(f.get("line_id", "L")))
	if e != nil {
		return budget.Line{}, e
	}
	kind := f.get("dimension", "ORGANIZATION")
	var target *budget.Target
	if raw := optional(f, "target"); raw != nil {
		x, e := budget.NewTarget(vectorScope(f.get("target_org", org)), budget.DimensionKind(f.get("target_kind", kind)), fixtureID(*raw))
		if e != nil {
			return budget.Line{}, e
		}
		target = &x
	}
	dimension, e := budget.NewDimension(s, budget.DimensionKind(kind), target, optional(f, "filter"))
	if e != nil {
		return budget.Line{}, e
	}
	limit := f.get("limit", "500000")
	return budget.NewLine(s, budget.LineInput{Reference: ref, Start: f.get("start", "2026-11-01"), End: f.get("end", "2026-12-01"),
		Dimension: dimension, Limit: optional(fields{"limit": limit}, "limit")})
}

func vectorTags(f fields, org string) (budget.Tags, error) {
	s := vectorScope(f.get("tag_org", org))
	var targets []budget.Target
	for _, k := range tagKeys {
		if raw, ok := f[k.key]; ok {
			x, e := budget.NewTarget(s, k.kind, fixtureID(raw))
			if e != nil {
				return budget.Tags{}, e
			}
			targets = append(targets, x)
		}
	}
	return budget.NewTags(s, targets...)
}

// vectorConsumption builds one EMP-EXP-001 expense from explicit fixture
// defaults; an expense construction error is a fixture bug, not a case result.
func vectorConsumption(t *testing.T, f fields, org string, z orgtime.ReportingZones) (budget.Consumption, error) {
	t.Helper()
	if allowed(f, itemKeys) != nil || f["id"] == "" || f["company"] == "" {
		t.Fatal("malformed consumption item", f)
	}
	own := f.get("org", org)
	s := vectorScope(own)
	ref, e := expense.NewExpenseRef(s, fixtureID(f["id"]))
	if e != nil {
		t.Fatal("fixture expense ref", e)
	}
	kind, e := expense.ParseOriginKind(f.get("origin", "DIRECT"))
	if e != nil {
		t.Fatal("fixture origin", e)
	}
	var source *expense.SourceRef
	if raw := optional(f, "source"); raw != nil {
		src, e := expense.NewSourceRef(s, fixtureID(*raw))
		if e != nil {
			t.Fatal("fixture source", e)
		}
		source = &src
	}
	origin, e := expense.NewOrigin(s, kind, source)
	if e != nil {
		t.Fatal("fixture origin", e)
	}
	category, e := expense.ParseCategory(f.get("category", "FUEL"))
	if e != nil {
		t.Fatal("fixture category", e)
	}
	company := f["company"]
	paid := f.get("paid", company)
	amounts, e := expense.NewAmounts(&paid, &company)
	if e != nil {
		t.Fatal("fixture amounts", e)
	}
	competence := instant(t, f.get("at", defaultAt))
	x, e := expense.NewExpense(s, expense.ExpenseInput{Reference: ref, Origin: origin, Category: category, Amounts: amounts,
		Competence: &competence, Zones: z})
	if e != nil {
		t.Fatal("fixture expense", e)
	}
	tags, e := vectorTags(f, own)
	if e != nil {
		return budget.Consumption{}, e
	}
	return budget.Consumption{Entry: expense.Entry{Expense: x, State: expense.ReviewState(f.get("state", "APPROVED"))}, Tags: tags}, nil
}

func vectorHold(t *testing.T, f fields, org string, z orgtime.ReportingZones) (budget.Hold, error) {
	t.Helper()
	if allowed(f, holdKeys) != nil || f["id"] == "" {
		t.Fatal("malformed hold item", f)
	}
	own := f.get("org", org)
	s := vectorScope(own)
	ref, e := budget.NewHoldRef(s, fixtureID(f["id"]))
	if e != nil {
		return budget.Hold{}, e
	}
	line, e := budget.NewLineRef(s, fixtureID(f.get("line", "L")))
	if e != nil {
		return budget.Hold{}, e
	}
	tags, e := vectorTags(f, own)
	if e != nil {
		return budget.Hold{}, e
	}
	var by *expense.ExpenseRef
	if raw, ok := f["by"]; ok {
		r, e := expense.NewExpenseRef(vectorScope(f.get("by_org", own)), fixtureID(raw))
		if e != nil {
			t.Fatal("fixture consuming expense", e)
		}
		by = &r
	}
	return budget.NewHold(s, budget.HoldInput{Reference: ref, Line: line, Amount: optional(f, "amount"),
		ReservedAt: instant(t, f.get("at", defaultAt)).OccurredAt, Zones: z, Category: f.get("category", "FUEL"), Tags: tags,
		State: budget.HoldState(f.get("state", "ACTIVE")), ConsumedBy: by})
}

// vectorRequest reads a reservation request; idKey/orgKey/lineKey name the
// fields that carry its identity, organization and line.
func vectorRequest(t *testing.T, f fields, org, idKey, orgKey, lineKey string, z orgtime.ReportingZones) (budget.HoldRequest, error) {
	t.Helper()
	own := f.get(orgKey, org)
	s := vectorScope(own)
	ref, e := budget.NewHoldRef(s, fixtureID(f.get(idKey, "H")))
	if e != nil {
		return budget.HoldRequest{}, e
	}
	line, e := budget.NewLineRef(s, fixtureID(f.get(lineKey, "L")))
	if e != nil {
		return budget.HoldRequest{}, e
	}
	tags, e := vectorTags(f, org)
	if e != nil {
		return budget.HoldRequest{}, e
	}
	return budget.HoldRequest{Reference: ref, Line: line, Amount: optional(f, "amount"),
		ReservedAt: instant(t, f.get("at", defaultAt)).OccurredAt, Zones: z, Category: f.get("category", "FUEL"), Tags: tags}, nil
}

func list(t *testing.T, raw string) []fields {
	t.Helper()
	if raw == "" {
		return nil
	}
	var out []fields
	for _, part := range strings.Split(raw, "~") {
		f, e := split(part, ",", ":")
		if e != nil {
			t.Fatal("malformed list item", part)
		}
		out = append(out, f)
	}
	return out
}

type facts struct {
	line        budget.Line
	consumption []budget.Consumption
	holds       []budget.Hold
	scope       kernel.OrgScope
	org         string
	reporting   orgtime.ReportingZones
}

func vectorFacts(t *testing.T, f fields) (facts, error) {
	t.Helper()
	org := f["org"]
	out := facts{scope: vectorScope(org), org: org, reporting: zones(f)}
	var e error
	if out.line, e = vectorLine(f, f.get("line_org", org)); e != nil {
		return facts{}, e
	}
	for _, item := range list(t, f["items"]) {
		c, e := vectorConsumption(t, item, org, out.reporting)
		if e != nil {
			return facts{}, e
		}
		out.consumption = append(out.consumption, c)
	}
	for _, item := range list(t, f["holds"]) {
		h, e := vectorHold(t, item, org, out.reporting)
		if e != nil {
			return facts{}, e
		}
		out.holds = append(out.holds, h)
	}
	return out, nil
}

func (x facts) capacity() (budget.Capacity, error) {
	return budget.ComputeCapacity(x.scope, x.line, x.consumption, x.holds)
}

func capacityFields(c budget.Capacity) fields {
	return fields{"limit": strconv.FormatInt(c.LimitCentavos(), 10), "consumed": c.ConsumedCentavos().String(),
		"consumed_count": strconv.Itoa(c.ConsumedCount()), "pending": c.PendingCentavos().String(),
		"pending_count": strconv.Itoa(c.PendingCount()), "excluded_count": strconv.Itoa(c.ExcludedCount()),
		"held": c.HeldCentavos().String(), "active_holds": strconv.Itoa(c.ActiveHolds()),
		"closed_holds": strconv.Itoa(c.ClosedHolds()), "available": c.AvailableCentavos().String(),
		"over_committed": strconv.FormatBool(c.OverCommitted())}
}

func lineFields(l budget.Line) fields {
	d := l.Dimension()
	target, filter := "?", "?"
	if x, ok := d.Target(); ok {
		target = alias(x.ID())
	}
	if c, ok := d.Filter(); ok {
		filter = c.Code()
	}
	return fields{"org": alias(l.Reference().OrganizationID().String()), "id": alias(l.Reference().ID()),
		"start": l.Period().Start(), "end": l.Period().End(), "dimension": string(d.Kind()), "target": target,
		"filter": filter, "limit": strconv.FormatInt(l.LimitCentavos(), 10)}
}

func holdRef(f fields, key, org string) (*budget.HoldRef, error) {
	raw, ok := f[key]
	if !ok {
		return nil, nil
	}
	ref, e := budget.NewHoldRef(vectorScope(f.get("hold_org", org)), fixtureID(raw))
	if e != nil {
		return nil, e
	}
	return &ref, nil
}

func runCapacity(t *testing.T, f fields) (fields, error) {
	x, e := vectorFacts(t, f)
	if e != nil {
		return nil, e
	}
	c, e := x.capacity()
	if e != nil {
		return nil, e
	}
	out := capacityFields(c)
	if raw, ok := f["claimed"]; ok {
		if e := c.Reconcile(raw); e != nil {
			return nil, e
		}
		out["current"] = "true"
	}
	return out, nil
}

func runReserve(t *testing.T, f fields) (fields, error) {
	x, e := vectorFacts(t, f)
	if e != nil {
		return nil, e
	}
	c, e := x.capacity()
	if e != nil {
		return nil, e
	}
	req, e := vectorRequest(t, f, x.org, "hold", "hold_org", "hold_line", x.reporting)
	if e != nil {
		return nil, e
	}
	r, e := budget.Reserve(vectorScope(f.get("decide_org", x.org)), c, req)
	if e != nil {
		return nil, e
	}
	out := fields{"outcome": string(r.Outcome()), "requested": strconv.FormatInt(r.RequestedCentavos(), 10),
		"available_before": r.AvailableBeforeCentavos().String(), "available_after": r.AvailableAfterCentavos().String(),
		"shortfall": r.ShortfallCentavos().String(), "hold": "?"}
	if h, ok := r.Hold(); ok {
		out["hold"], out["hold_line"], out["day"], out["hold_state"] = alias(h.Reference().ID()), alias(h.Line().ID()),
			h.Reporting().Day, string(h.State())
	}
	return out, nil
}

func postingFields(p budget.Posting) fields {
	out := fields{"outcome": string(p.Outcome()), "amount": strconv.FormatInt(p.AmountCentavos(), 10),
		"available_before": p.AvailableBeforeCentavos().String(), "available_after": p.AvailableAfterCentavos().String(),
		"shortfall": p.ShortfallCentavos().String(), "converted": "?"}
	if h, ok := p.ConsumedHold(); ok {
		by, _ := h.ConsumedBy()
		out["converted"], out["converted_state"], out["consumed_by"] = alias(h.Reference().ID()), string(h.State()), alias(by.ID())
	}
	return out
}

func runPost(t *testing.T, f fields) (fields, error) {
	x, e := vectorFacts(t, f)
	if e != nil {
		return nil, e
	}
	c, e := x.capacity()
	if e != nil {
		return nil, e
	}
	items := list(t, f["post"])
	if len(items) != 1 {
		t.Fatal("POST needs exactly one item")
	}
	posted, e := vectorConsumption(t, items[0], x.org, x.reporting)
	if e != nil {
		return nil, e
	}
	ref, e := holdRef(f, "hold", x.org)
	if e != nil {
		return nil, e
	}
	p, e := budget.Post(vectorScope(f.get("decide_org", x.org)), c, posted, ref)
	if e != nil {
		return nil, e
	}
	return postingFields(p), nil
}

func holdIndex(holds []budget.Hold, id string) int {
	for i, h := range holds {
		if h.Reference().ID() == id {
			return i
		}
	}
	return -1
}

// runSequence replays serialized steps the way the owning transaction does
// under the line's row lock: recompute capacity from the current facts, decide,
// then append or replace facts only on success.
func runSequence(t *testing.T, f fields) (fields, error) {
	x, e := vectorFacts(t, f)
	if e != nil {
		return nil, e
	}
	var done []string
	for _, step := range list(t, f["steps"]) {
		keys, ok := stepKeys[step["do"]]
		if !ok || allowed(step, keys) != nil {
			t.Fatal("malformed step", step)
		}
		switch step["do"] {
		case "RESERVE":
			c, e := x.capacity()
			if e != nil {
				return nil, e
			}
			req, e := vectorRequest(t, step, x.org, "id", "org", "line", x.reporting)
			if e != nil {
				return nil, e
			}
			r, e := budget.Reserve(x.scope, c, req)
			if e != nil {
				return nil, e
			}
			done = append(done, string(r.Outcome()))
			if h, ok := r.Hold(); ok {
				x.holds = append(x.holds, h)
			}
		case "RELEASE", "EXPIRE":
			i := holdIndex(x.holds, fixtureID(step["id"]))
			var h budget.Hold
			if i >= 0 {
				h = x.holds[i]
			}
			transition := h.Release
			if step["do"] == "EXPIRE" {
				transition = h.Expire
			}
			next, e := transition()
			if e != nil {
				return nil, e
			}
			if i < 0 {
				t.Fatal("transition of an absent hold succeeded")
			}
			x.holds[i] = next
			done = append(done, string(next.State()))
		case "POST":
			c, e := x.capacity()
			if e != nil {
				return nil, e
			}
			item := fields{}
			for k, v := range step {
				if k != "do" && k != "hold" {
					item[k] = v
				}
			}
			posted, e := vectorConsumption(t, item, x.org, x.reporting)
			if e != nil {
				return nil, e
			}
			ref, e := holdRef(step, "hold", x.org)
			if e != nil {
				return nil, e
			}
			p, e := budget.Post(x.scope, c, posted, ref)
			if e != nil {
				return nil, e
			}
			done = append(done, string(p.Outcome()))
			if p.Outcome() == budget.OutcomePost {
				x.consumption = append(x.consumption, posted)
				if h, ok := p.ConsumedHold(); ok {
					x.holds[holdIndex(x.holds, h.Reference().ID())] = h
				}
			}
		}
	}
	c, e := x.capacity()
	if e != nil {
		return nil, e
	}
	all := capacityFields(c)
	return fields{"outcomes": strings.Join(done, ","), "consumed": all["consumed"], "held": all["held"],
		"available": all["available"], "active_holds": all["active_holds"], "closed_holds": all["closed_holds"]}, nil
}

func runVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "ENUM":
		raw, e := decodeHex(f["value_hex"])
		if e != nil {
			t.Fatal(e)
		}
		switch f["kind"] {
		case "DIMENSION":
			v, e := budget.ParseDimensionKind(raw)
			return fields{"value": string(v)}, e
		case "HOLD_STATE":
			v, e := budget.ParseHoldState(raw)
			return fields{"value": string(v)}, e
		}
		t.Fatal("unknown fixture enum namespace")
	case "LINE":
		l, e := vectorLine(f, f["org"])
		if e != nil {
			return nil, e
		}
		return lineFields(l), nil
	case "CAPACITY":
		return runCapacity(t, f)
	case "RESERVE":
		return runReserve(t, f)
	case "POST":
		return runPost(t, f)
	case "SEQUENCE":
		return runSequence(t, f)
	}
	t.Fatal("unknown operation")
	return nil, nil
}

// errorFields maps exactly one documented budget error; anything else,
// including an error of a reused kernel, is UNEXPECTED.
func errorFields(e error) fields {
	found := ""
	for code, target := range errorCodes {
		if e != nil && errors.Is(e, target) {
			if found != "" {
				return fields{"error": "UNEXPECTED"}
			}
			found = code
		}
	}
	if found == "" {
		return fields{"error": "UNEXPECTED"}
	}
	return fields{"error": found}
}

func TestSharedBudgetVectors(t *testing.T) {
	raw, e := os.ReadFile(corpusPath)
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
	counts, ids, codes, seen := map[string]int{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || p[0] != strings.TrimSpace(p[0]) || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case", p[0])
		}
		if _, ok := wantCounts[p[1]]; !ok {
			t.Fatal("unknown shared operation", p[1])
		}
		ids[p[0]] = true
		counts[p[1]]++
		args, er := split(p[2], ";", "=")
		if er != nil {
			t.Fatal(p[0], er)
		}
		want, er := split(p[3], ";", "=")
		if er != nil {
			t.Fatal(p[0], er)
		}
		if allowed(args, allowedArgs[p[1]]) != nil {
			t.Fatal("unknown fixture argument", p[0])
		}
		if code, ok := want["error"]; ok {
			if len(want) != 1 || errorCodes[code] == nil {
				t.Fatal("error result carries output or an undocumented code", p[0])
			}
			codes[code] = true
		}
		for _, outcome := range strings.Split(want["outcome"]+","+want["outcomes"], ",") {
			seen[outcome] = true
		}
		seen[want["dimension"]], seen[want["value"]] = true, true
		t.Run(p[0], func(t *testing.T) {
			got, err := runVector(t, p[1], args)
			if err != nil {
				got = errorFields(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shared mismatch: got %v expected %v", got, want)
			}
		})
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if len(ids) != 261 || !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", len(ids), counts)
	}
	if len(codes) != len(errorCodes) {
		t.Fatal("error code without a shared vector", codes)
	}
	for _, v := range append(outcomes, "ORGANIZATION", "UNIT", "COST_CENTRE", "ASSET", "EMPLOYEE", "ACTIVE", "CONSUMED") {
		if !seen[v] {
			t.Fatal("documented value without a shared vector", v)
		}
	}
}
