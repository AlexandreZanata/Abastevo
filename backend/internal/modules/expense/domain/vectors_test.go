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

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	corpusPath = "../../../../../contracts/testdata/enterprise/expense/expense-v1.psv"
	corpusHash = "b411e1cfa55c0aca4e4c9cadfab2f286ec22c3215e3103c119a767e013885dfe"
)

type fields map[string]string

var (
	errUnknownEnumSelector = errors.New("fixture: unknown enum namespace")
	errMalformedFixture    = errors.New("fixture: malformed grammar")
	wantCounts             = map[string]int{"ENUM": 40, "AMOUNTS": 36, "CATEGORY": 24, "PERIOD": 17, "EXPENSE": 57, "TOTALS": 48}
	errorCodes             = []string{"INVALID", "SCOPE", "NOT_COST_ORIGIN", "COMPANY_EXCEEDS_PAID", "DUPLICATE_COST", "OUT_OF_PERIOD"}
	expenseKeys            = "org id id_org origin source source_org category paid company at entry offset zone unit_zone omit"
	allowedArgs            = map[string]string{
		"ENUM": "kind value_hex", "AMOUNTS": "paid company", "CATEGORY": "code", "PERIOD": "start end",
		"EXPENSE": expenseKeys, "TOTALS": "org start end basis zone unit_zone items",
	}
	omittable = map[string]bool{"id": true, "origin": true, "category": true, "amounts": true, "competence": true, "zone": true}
	aliases   = map[string]int{"A": 1, "B": 2, "E": 11, "E2": 12, "E3": 13, "E4": 14, "E5": 15, "E6": 16, "E7": 17, "E8": 18, "E9": 19,
		"F": 21, "F2": 22, "F3": 23, "W": 31, "W2": 32}
)

func validateEnumSelector(raw string) error {
	switch raw {
	case "ORIGIN", "STATE", "BASIS":
		return nil
	}
	return errUnknownEnumSelector
}

func uuid(n int) string { return fmt.Sprintf("9e000000-0000-4000-8000-%012d", n) }

func fixtureID(alias string) string {
	if n, ok := aliases[alias]; ok {
		return uuid(n)
	}
	return alias
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

func decodeHex(raw string) (string, error) {
	v, e := hex.DecodeString(raw)
	if e != nil || raw != strings.ToLower(raw) || !utf8.Valid(v) {
		return "", errMalformedFixture
	}
	return string(v), nil
}

func TestSharedReaderRejectsMalformedGrammar(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "origin", "ORIGIN ", "CATEGORY"} {
		if !errors.Is(validateEnumSelector(raw), errUnknownEnumSelector) {
			t.Fatal("malformed fixture namespace reached domain dispatch")
		}
	}
	for _, raw := range []string{"ORIGIN", "STATE", "BASIS"} {
		if validateEnumSelector(raw) != nil {
			t.Fatal("documented fixture namespace refused")
		}
	}
	for _, raw := range []string{"novalue", "=x", "a=1;a=2", " =1"} {
		if _, e := fieldMap(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed field map accepted", raw)
		}
	}
	for _, raw := range []string{"ff", "eda080", "4F", "4"} {
		if _, e := decodeHex(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed fixture hex accepted", raw)
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

// vectorExpense builds one expense from documented defaults plus overrides.
func vectorExpense(t *testing.T, args fields) (expense.Expense, error) {
	t.Helper()
	f := fields{"org": "A", "id": "E", "origin": "FUELING", "source": "F", "category": "FUEL", "paid": "97350", "company": "97350",
		"at": "2026-11-14T12:00:00Z", "entry": "UTC", "offset": "0", "zone": "America/Sao_Paulo", "unit_zone": "?"}
	for k, v := range args {
		f[k] = v
	}
	omitted := map[string]bool{}
	if raw, ok := f["omit"]; ok {
		for _, key := range strings.Split(raw, ",") {
			if !omittable[key] {
				t.Fatal("unknown omit fixture field")
			}
			omitted[key] = true
		}
	}
	org := f["org"]
	s := vectorScope(org)
	var e error
	in := expense.ExpenseInput{}
	if !omitted["id"] {
		if in.Reference, e = expense.NewExpenseRef(vectorScope(f.get("id_org", org)), fixtureID(f["id"])); e != nil {
			return expense.Expense{}, e
		}
	}
	if !omitted["origin"] {
		kind, er := expense.ParseOriginKind(f["origin"])
		if er != nil {
			return expense.Expense{}, er
		}
		var source *expense.SourceRef
		if raw := optional(f, "source"); raw != nil {
			ref, er := expense.NewSourceRef(vectorScope(f.get("source_org", org)), fixtureID(*raw))
			if er != nil {
				return expense.Expense{}, er
			}
			source = &ref
		}
		if in.Origin, e = expense.NewOrigin(s, kind, source); e != nil {
			return expense.Expense{}, e
		}
	}
	if !omitted["category"] {
		if in.Category, e = expense.ParseCategory(f["category"]); e != nil {
			return expense.Expense{}, e
		}
	}
	if !omitted["amounts"] {
		if in.Amounts, e = expense.NewAmounts(optional(f, "paid"), optional(f, "company")); e != nil {
			return expense.Expense{}, e
		}
	}
	if !omitted["competence"] {
		at, er := orgtime.ParseInstant(f["at"])
		if er != nil {
			return expense.Expense{}, expense.ErrInvalid
		}
		offset, er := strconv.ParseInt(f["offset"], 10, 64)
		if er != nil {
			return expense.Expense{}, expense.ErrInvalid
		}
		in.Competence = &orgtime.Declaration{OccurredAt: at, EntryZone: f["entry"], DeviceOffsetSeconds: offset}
	}
	if !omitted["zone"] {
		in.Zones = orgtime.ReportingZones{Organization: f["zone"], Unit: optional(f, "unit_zone")}
	}
	return expense.NewExpense(s, in)
}

func expenseFields(e expense.Expense) fields {
	ref, origin, key, comp, rep := e.Reference(), e.Origin(), e.OriginKey(), e.Competence(), e.Reporting()
	source := "?"
	if s := origin.Source(); s != nil {
		source = alias(s.ID())
	}
	return fields{"ready": "true", "org": alias(ref.OrganizationID().String()), "id": alias(ref.ID()), "origin": string(origin.Kind()),
		"source": source, "key": alias(key.OrganizationID().String()) + ":" + string(key.Kind()) + ":" + alias(key.Source()),
		"category": e.Category().Code(), "paid": strconv.FormatInt(e.Amounts().PaidCentavos(), 10),
		"company": strconv.FormatInt(e.Amounts().CompanyCentavos(), 10), "at": orgtime.FormatInstant(comp.OccurredAt),
		"entry": comp.EntryZone, "offset": strconv.FormatInt(comp.DeviceOffsetSeconds, 10), "day": rep.Day, "zone": rep.Zone,
		"reporting_source": rep.Source}
}

func vectorTotals(t *testing.T, args fields) (fields, error) {
	t.Helper()
	var entries []expense.Entry
	if raw := args["items"]; raw != "" {
		for _, tuple := range strings.Split(raw, "~") {
			p := strings.Split(tuple, ",")
			if len(p) != 8 {
				t.Fatal("malformed totals item tuple")
			}
			e, err := vectorExpense(t, fields{"id": p[0], "org": p[1], "origin": p[2], "source": p[3], "paid": p[4], "company": p[5],
				"at": p[6], "zone": args.get("zone", "America/Sao_Paulo"), "unit_zone": args.get("unit_zone", "?")})
			if err != nil {
				return nil, err
			}
			state, err := expense.ParseReviewState(p[7])
			if err != nil {
				return nil, err
			}
			entries = append(entries, expense.Entry{Expense: e, State: state})
		}
	}
	period, err := expense.NewReportingPeriod(args["start"], args["end"])
	if err != nil {
		return nil, err
	}
	basis, err := expense.ParseBasis(args["basis"])
	if err != nil {
		return nil, err
	}
	total, err := expense.Totals(vectorScope(args["org"]), period, basis, entries)
	if err != nil {
		return nil, err
	}
	return fields{"org": alias(total.OrganizationID().String()), "start": total.Period().Start(), "end": total.Period().End(),
		"basis": string(total.Basis()), "approved": total.ApprovedCentavos().String(),
		"approved_count": strconv.Itoa(total.ApprovedCount()), "pending": total.PendingCentavos().String(),
		"pending_count": strconv.Itoa(total.PendingCount()), "excluded_count": strconv.Itoa(total.ExcludedCount()),
		"figure": total.FigureCentavos().String(), "includes_pending": strconv.FormatBool(total.IncludesPending())}, nil
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
		case "ORIGIN":
			v, e := expense.ParseOriginKind(raw)
			return fields{"value": string(v)}, e
		case "STATE":
			v, e := expense.ParseReviewState(raw)
			if e != nil {
				return nil, e
			}
			c, e := v.Counting()
			return fields{"value": string(v), "counting": string(c)}, e
		case "BASIS":
			v, e := expense.ParseBasis(raw)
			return fields{"value": string(v)}, e
		}
		t.Fatal("unknown fixture enum namespace")
	case "AMOUNTS":
		a, e := expense.NewAmounts(optional(f, "paid"), optional(f, "company"))
		if e != nil {
			return nil, e
		}
		return fields{"paid": strconv.FormatInt(a.PaidCentavos(), 10), "company": strconv.FormatInt(a.CompanyCentavos(), 10)}, nil
	case "CATEGORY":
		c, e := expense.ParseCategory(f["code"])
		if e != nil {
			return nil, e
		}
		return fields{"code": c.Code()}, nil
	case "PERIOD":
		p, e := expense.NewReportingPeriod(f["start"], f["end"])
		if e != nil {
			return nil, e
		}
		return fields{"start": p.Start(), "end": p.End()}, nil
	case "EXPENSE":
		e, err := vectorExpense(t, f)
		if err != nil {
			return nil, err
		}
		return expenseFields(e), nil
	case "TOTALS":
		return vectorTotals(t, f)
	}
	t.Fatal("unknown operation")
	return nil, nil
}

func errorFields(e error) fields {
	for code, target := range map[string]error{"INVALID": expense.ErrInvalid, "SCOPE": expense.ErrScope,
		"NOT_COST_ORIGIN": expense.ErrNotCostOrigin, "COMPANY_EXCEEDS_PAID": expense.ErrCompanyExceedsPaid,
		"DUPLICATE_COST": expense.ErrDuplicateCost, "OUT_OF_PERIOD": expense.ErrOutOfPeriod} {
		if errors.Is(e, target) {
			return fields{"error": code}
		}
	}
	return fields{"error": "UNEXPECTED"}
}

func TestSharedExpenseVectors(t *testing.T) {
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
	counts, ids, codes := map[string]int{}, map[string]bool{}, map[string]bool{}
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
		args, er := fieldMap(p[2])
		if er != nil {
			t.Fatal(p[0], er)
		}
		want, er := fieldMap(p[3])
		if er != nil {
			t.Fatal(p[0], er)
		}
		if p[1] == "ENUM" && validateEnumSelector(args["kind"]) != nil {
			t.Fatal("malformed fixture enum namespace")
		}
		for k := range args {
			if !strings.Contains(" "+allowedArgs[p[1]]+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument", p[0], k)
			}
		}
		if code, ok := want["error"]; ok {
			if len(want) != 1 {
				t.Fatal("error result carries output", p[0])
			}
			codes[code] = true
		}
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
	if len(ids) != 222 || !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", len(ids), counts)
	}
	for _, code := range errorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(errorCodes) {
		t.Fatal("undocumented error code in corpus", codes)
	}
}
