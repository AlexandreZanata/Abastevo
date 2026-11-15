package payable_test

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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain/payable"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	corpusPath = "../../../../../../contracts/testdata/enterprise/payable/payable-v1.psv"
	corpusHash = "5ac0b07637b66631dba97e441a66c750af9441c867461da8899dcec132904282"
	corpusSize = 275
	fixtureAt  = "2026-11-14T12:00:00Z"
	ledgerNote = "Synthetic duplicate obligation"
)

type fields map[string]string

var (
	errUnknownEnumSelector = errors.New("fixture: unknown enum namespace")
	errMalformedFixture    = errors.New("fixture: malformed grammar")
	wantCounts             = map[string]int{"ENUM": 27, "SETTLEMENT": 25, "PAYABLE": 183, "LEDGER": 40}
	singleLineKeys         = "expense expense_org origin source paid company state payer paid_by paid_by_org amount"
	allowedArgs            = map[string]string{
		"ENUM": "kind value_hex", "SETTLEMENT": "org id id_org amount",
		"PAYABLE": "org id id_org omit kind beneficiary beneficiary_org supplier supplier_org document document_org " +
			"statement statement_org lines events assert " + singleLineKeys,
		"LEDGER": "org start end basis costs sources payables",
	}
	// errorCodes is the closed oracle vocabulary; INVALID and SCOPE come from
	// either package, cost-side codes from the reused expense domain.
	errorCodes = []struct {
		code    string
		targets []error
	}{
		{"INVALID", []error{payable.ErrInvalid, expense.ErrInvalid}},
		{"SCOPE", []error{payable.ErrScope, expense.ErrScope}},
		{"NOT_ELIGIBLE", []error{payable.ErrNotEligible}},
		{"EXCEEDS_SOURCE", []error{payable.ErrExceedsSource}},
		{"PARTIAL_REIMBURSEMENT", []error{payable.ErrPartialReimbursement}},
		{"DUPLICATE_SOURCE", []error{payable.ErrDuplicateSource}},
		{"DUPLICATE_ENTRY", []error{payable.ErrDuplicateEntry}},
		{"EXCEEDS_REMAINING", []error{payable.ErrExceedsRemaining}},
		{"CANCELLED", []error{payable.ErrCancelled}},
		{"HAS_SETTLEMENT", []error{payable.ErrHasSettlement}},
		{"STATE_MISMATCH", []error{payable.ErrStateMismatch}},
		{"NOT_COST_ORIGIN", []error{expense.ErrNotCostOrigin}},
		{"DUPLICATE_COST", []error{expense.ErrDuplicateCost}},
		{"OUT_OF_PERIOD", []error{expense.ErrOutOfPeriod}},
	}
	aliases = map[string]int{"A": 1, "B": 2, "E": 11, "E2": 12, "E3": 13, "E4": 14, "E5": 15,
		"F": 21, "F2": 22, "F3": 23, "F4": 24, "F5": 25, "W": 31, "U": 41, "U2": 42, "V": 51, "V2": 52, "D": 61,
		"T": 71, "T2": 72, "P": 81, "P2": 82, "P3": 83, "P4": 84, "S1": 91, "S2": 92, "S3": 93, "S4": 94, "S5": 95}
)

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

func validateEnumSelector(raw string) error {
	switch raw {
	case "KIND", "STATE":
		return nil
	}
	return errUnknownEnumSelector
}

func fieldMap(raw string) (fields, error) {
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
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

// decodeBytes decodes lowercase even-length hex into any byte string, so an
// invalid UTF-8 reason reaches the domain.
func decodeBytes(raw string) (string, error) {
	v, e := hex.DecodeString(raw)
	if e != nil || raw != strings.ToLower(raw) {
		return "", errMalformedFixture
	}
	return string(v), nil
}

// decodeText also requires UTF-8 (closed tokens).
func decodeText(raw string) (string, error) {
	v, e := decodeBytes(raw)
	if e != nil || !utf8.ValidString(v) {
		return "", errMalformedFixture
	}
	return v, nil
}

func errorCode(e error) string {
	found := ""
	for _, c := range errorCodes {
		for _, target := range c.targets {
			if errors.Is(e, target) {
				if found != "" && found != c.code {
					return "AMBIGUOUS"
				}
				found = c.code
			}
		}
	}
	if found == "" {
		return "UNEXPECTED"
	}
	return found
}

func TestSharedReaderRejectsMalformedGrammar(t *testing.T) {
	for _, raw := range []string{"", "kind", "KIND ", "STATES", "PAYER"} {
		if !errors.Is(validateEnumSelector(raw), errUnknownEnumSelector) {
			t.Fatal("malformed fixture namespace reached domain dispatch", raw)
		}
	}
	for _, raw := range []string{"KIND", "STATE"} {
		if validateEnumSelector(raw) != nil {
			t.Fatal("documented fixture namespace refused", raw)
		}
	}
	for _, raw := range []string{"novalue", "=x", "a=1;a=2", " =1", " a=1", "a =1"} {
		if _, e := fieldMap(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed field map accepted", raw)
		}
	}
	for _, raw := range []string{"ff0", "4F", "zz"} {
		if _, e := decodeBytes(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed fixture hex accepted", raw)
		}
	}
	if v, e := decodeBytes("ff"); e != nil || v != "\xff" {
		t.Fatal("raw reason bytes refused")
	}
	if _, e := decodeText("ff"); !errors.Is(e, errMalformedFixture) {
		t.Fatal("non-UTF-8 enum token accepted")
	}
	for _, e := range []error{nil, errors.New("foreign"), fueling.ErrInvalid, errMalformedFixture} {
		if errorCode(e) != "UNEXPECTED" {
			t.Fatal("foreign error mapped to an oracle code", e)
		}
	}
	if errorCode(errors.Join(payable.ErrInvalid, payable.ErrScope)) != "AMBIGUOUS" {
		t.Fatal("ambiguous error mapped to one code")
	}
	if errorCode(payable.ErrSecondCost) != "NOT_COST_ORIGIN" {
		t.Fatal("second cost not mapped to NOT_COST_ORIGIN")
	}
}

// vectorScope leaves a malformed organization as the zero scope so that the
// domain itself refuses it.
func vectorScope(raw string) kernel.OrgScope {
	id, e := kernel.ParseOrgID(fixtureID(raw))
	if e != nil {
		return kernel.OrgScope{}
	}
	s, _ := kernel.NewOrgScope(id)
	return s
}

// Foreign-package references the fixture cannot construct reach the domain
// as zero values.
func employeeRef(org, raw string) *fueling.EmployeeRef {
	r, e := fueling.NewEmployeeRef(vectorScope(org), fixtureID(raw))
	if e != nil {
		return &fueling.EmployeeRef{}
	}
	return &r
}

func supplierRef(org, raw string) *fueling.SupplierRef {
	r, e := fueling.NewSupplierRef(vectorScope(org), fixtureID(raw))
	if e != nil {
		return &fueling.SupplierRef{}
	}
	return &r
}

func documentRef(org, raw string) *fueling.DocumentRef {
	r, e := fueling.NewDocumentRef(vectorScope(org), fixtureID(raw))
	if e != nil {
		return &fueling.DocumentRef{}
	}
	return &r
}

// sourceExpense builds one expense with the fixed synthetic category,
// declaration and zone; origin kind errors are the expense package's.
func sourceExpense(org, id, kind, source, paid, company, at string) (expense.Expense, error) {
	s := vectorScope(org)
	ref, e := expense.NewExpenseRef(s, fixtureID(id))
	if e != nil {
		return expense.Expense{}, e
	}
	k, e := expense.ParseOriginKind(kind)
	if e != nil {
		return expense.Expense{}, e
	}
	var src *expense.SourceRef
	if source != "?" {
		r, e := expense.NewSourceRef(s, fixtureID(source))
		if e != nil {
			return expense.Expense{}, e
		}
		src = &r
	}
	origin, e := expense.NewOrigin(s, k, src)
	if e != nil {
		return expense.Expense{}, e
	}
	category, e := expense.ParseCategory("FUEL")
	if e != nil {
		return expense.Expense{}, e
	}
	amounts, e := expense.NewAmounts(&paid, &company)
	if e != nil {
		return expense.Expense{}, e
	}
	instant, e := orgtime.ParseInstant(at)
	if e != nil {
		return expense.Expense{}, e
	}
	return expense.NewExpense(s, expense.ExpenseInput{Reference: ref, Origin: origin, Category: category, Amounts: amounts,
		Competence: &orgtime.Declaration{OccurredAt: instant, EntryZone: "UTC"},
		Zones:      orgtime.ReportingZones{Organization: "America/Sao_Paulo"}})
}

type lineSpec struct{ expense, org, origin, source, paid, company, state, payer, paidBy, paidByOrg, amount string }

func vectorLine(t *testing.T, s lineSpec) payable.Line {
	t.Helper()
	var x expense.Expense
	if s.expense != "?" {
		built, e := sourceExpense(s.org, s.expense, s.origin, s.source, s.paid, s.company, fixtureAt)
		if e != nil && vectorScope(s.org).Validate() == nil {
			t.Fatal("fixture source expense construction failed", e)
		}
		x = built
	}
	l := payable.Line{Source: payable.Source{Expense: x, State: expense.ReviewState(s.state), Payer: fueling.Payer(s.payer)}}
	if s.paidBy != "?" {
		l.Source.PaidBy = employeeRef(s.paidByOrg, s.paidBy)
	}
	if s.amount != "?" {
		v := s.amount
		l.Amount = &v
	}
	return l
}

func vectorLines(t *testing.T, args fields, org, kind string) []payable.Line {
	t.Helper()
	raw, ok := args["lines"]
	if !ok {
		payer := "COMPANY"
		if kind == string(payable.Reimbursement) {
			payer = "EMPLOYEE"
		}
		payer = args.get("payer", payer)
		paidBy := "?"
		if payer == string(fueling.Employee) {
			paidBy = "U"
		}
		return []payable.Line{vectorLine(t, lineSpec{expense: args.get("expense", "E"), org: args.get("expense_org", org),
			origin: args.get("origin", "FUELING"), source: args.get("source", "F"), paid: args.get("paid", "97350"),
			company: args.get("company", "97350"), state: args.get("state", "APPROVED"), payer: payer,
			paidBy: args.get("paid_by", paidBy), paidByOrg: args.get("paid_by_org", org), amount: args.get("amount", "97350")})}
	}
	for k := range args {
		if strings.Contains(" "+singleLineKeys+" ", " "+k+" ") {
			t.Fatal("single-line fixture key combined with lines", k)
		}
	}
	var out []payable.Line
	if raw == "" {
		return out
	}
	for _, tuple := range strings.Split(raw, "~") {
		p := strings.Split(tuple, ",")
		if len(p) != 10 {
			t.Fatal("malformed line tuple")
		}
		out = append(out, vectorLine(t, lineSpec{expense: p[0], org: p[1], origin: p[2], source: p[3], paid: p[4],
			company: p[5], state: p[6], payer: p[7], paidBy: p[8], paidByOrg: org, amount: p[9]}))
	}
	return out
}

// party returns the alias of a counterparty: the kind default unless given,
// "?" forcing absence.
func party(args fields, kind, key, fallback string) (string, bool) {
	defaults := map[string]string{"REIMBURSEMENT": "beneficiary", "SUPPLIER": "supplier", "CARD_STATEMENT": "statement"}
	raw, ok := args[key]
	if !ok {
		if defaults[kind] != key {
			return "", false
		}
		raw = fallback
	}
	return raw, raw != "?"
}

func settle(p payable.Payable, scope kernel.OrgScope, entryOrg, id, amount string) (payable.Payable, error) {
	es := vectorScope(entryOrg)
	ref, e := payable.NewSettlementRef(es, fixtureID(id))
	if e != nil {
		return payable.Payable{}, e
	}
	s, e := payable.NewSettlement(es, ref, &amount)
	if e != nil {
		return payable.Payable{}, e
	}
	return p.Settle(scope, s)
}

func vectorPayable(t *testing.T, args fields) (payable.Payable, error) {
	t.Helper()
	org, kind := args.get("org", "A"), args.get("kind", "REIMBURSEMENT")
	s := vectorScope(org)
	in := payable.Input{Kind: payable.Kind(kind)}
	switch args.get("omit", "") {
	case "":
		ref, e := payable.NewPayableRef(vectorScope(args.get("id_org", org)), fixtureID(args.get("id", "P")))
		if e != nil {
			return payable.Payable{}, e
		}
		in.Reference = ref
	case "id":
	default:
		t.Fatal("unknown omit fixture field")
	}
	if a, ok := party(args, kind, "beneficiary", "U"); ok {
		in.Beneficiary = employeeRef(args.get("beneficiary_org", org), a)
	}
	if a, ok := party(args, kind, "supplier", "V"); ok {
		in.Supplier = supplierRef(args.get("supplier_org", org), a)
	}
	if a, ok := party(args, kind, "document", "?"); ok {
		in.Document = documentRef(args.get("document_org", org), a)
	}
	if a, ok := party(args, kind, "statement", "T"); ok {
		ref, e := payable.NewStatementRef(vectorScope(args.get("statement_org", org)), fixtureID(a))
		if e != nil {
			return payable.Payable{}, e
		}
		in.Statement = &ref
	}
	in.Lines = vectorLines(t, args, org, kind)
	p, e := payable.New(s, in)
	if e != nil {
		return payable.Payable{}, e
	}
	if raw, ok := args["events"]; ok {
		for _, event := range strings.Split(raw, "~") {
			part := strings.Split(event, ":")
			switch {
			case part[0] == "S" && (len(part) == 3 || len(part) == 4):
				entryOrg := org
				if len(part) == 4 {
					entryOrg = part[3]
				}
				p, e = settle(p, s, entryOrg, part[1], part[2])
			case part[0] == "C" && (len(part) == 2 || len(part) == 3):
				reason, er := decodeBytes(part[1])
				if er != nil {
					t.Fatal(er)
				}
				scopeOrg := org
				if len(part) == 3 {
					scopeOrg = part[2]
				}
				p, e = p.Cancel(vectorScope(scopeOrg), reason)
			default:
				t.Fatal("malformed fixture event", event)
			}
			if e != nil {
				return payable.Payable{}, e
			}
		}
	}
	if raw, ok := args["assert"]; ok {
		if e := p.CheckState(payable.State(raw)); e != nil {
			return payable.Payable{}, e
		}
	}
	return p, nil
}

func keyText(k expense.OriginKey) string {
	return alias(k.OrganizationID().String()) + ":" + string(k.Kind()) + ":" + alias(k.Source())
}

func payableFields(p payable.Payable) fields {
	var lines, keys, settlements []string
	for _, l := range p.Lines() {
		lines = append(lines, alias(l.Expense().ID())+":"+strconv.FormatInt(l.AmountCentavos(), 10)+":"+
			strconv.FormatInt(l.CompanyCentavos(), 10))
		keys = append(keys, keyText(l.Key()))
	}
	for _, s := range p.Settlements() {
		settlements = append(settlements, alias(s.Reference().ID())+":"+strconv.FormatInt(s.AmountCentavos(), 10))
	}
	settled := "-"
	if len(settlements) > 0 {
		settled = strings.Join(settlements, "+")
	}
	b, s, d, st := p.Beneficiary(), p.Supplier(), p.Document(), p.Statement()
	out := fields{"ready": "true", "org": alias(p.Reference().OrganizationID().String()), "id": alias(p.Reference().ID()),
		"kind": string(p.Kind()), "beneficiary": "?", "supplier": "?", "document": "?", "statement": "?",
		"lines": strings.Join(lines, "+"), "keys": strings.Join(keys, "+"),
		"obligation": strconv.FormatInt(p.ObligationCentavos(), 10), "settled": strconv.FormatInt(p.SettledCentavos(), 10),
		"remaining": strconv.FormatInt(p.RemainingCentavos(), 10), "state": string(p.State()), "settlements": settled,
		"cancelled": strconv.FormatBool(p.CancelReason() != "")}
	if b != nil {
		out["beneficiary"] = alias(b.ID())
	}
	if s != nil {
		out["supplier"] = alias(s.ID())
	}
	if d != nil {
		out["document"] = alias(d.ID())
	}
	if st != nil {
		out["statement"] = alias(st.ID())
	}
	return out
}

func ledgerItems(t *testing.T, raw string, pool map[string]expense.Entry) ([]expense.Entry, error) {
	t.Helper()
	var out []expense.Entry
	if raw == "" {
		return out, nil
	}
	for _, tuple := range strings.Split(raw, "~") {
		p := strings.Split(tuple, ",")
		if len(p) != 8 {
			t.Fatal("malformed ledger item tuple")
		}
		x, e := sourceExpense(p[1], p[0], p[2], p[3], p[4], p[5], p[6])
		if e != nil {
			return nil, e
		}
		state, e := expense.ParseReviewState(p[7])
		if e != nil {
			return nil, e
		}
		if _, dup := pool[p[0]]; dup {
			t.Fatal("ledger fixture alias repeated", p[0])
		}
		entry := expense.Entry{Expense: x, State: state}
		pool[p[0]] = entry
		out = append(out, entry)
	}
	return out, nil
}

func ledgerPayable(t *testing.T, tuple string, pool map[string]expense.Entry) (payable.Payable, error) {
	t.Helper()
	p := strings.Split(tuple, ",")
	if len(p) != 8 {
		t.Fatal("malformed ledger payable tuple")
	}
	org, kind := p[1], payable.Kind(p[2])
	s := vectorScope(org)
	ref, e := payable.NewPayableRef(s, fixtureID(p[0]))
	if e != nil {
		return payable.Payable{}, e
	}
	in := payable.Input{Reference: ref, Kind: kind}
	switch kind {
	case payable.Reimbursement:
		in.Beneficiary = employeeRef(org, p[3])
	case payable.Supplier:
		in.Supplier = supplierRef(org, p[3])
	case payable.CardStatement:
		st, e := payable.NewStatementRef(s, fixtureID(p[3]))
		if e != nil {
			return payable.Payable{}, e
		}
		in.Statement = &st
	default:
		t.Fatal("unknown ledger payable kind")
	}
	for _, part := range strings.Split(p[5], "+") {
		name, amount, ok := strings.Cut(part, ":")
		entry, known := pool[name]
		if !ok || !known {
			t.Fatal("malformed ledger payable line", part)
		}
		src := payable.Source{Expense: entry.Expense, State: entry.State, Payer: fueling.Payer(p[4])}
		if p[4] == string(fueling.Employee) {
			src.PaidBy = employeeRef(org, "U")
		}
		in.Lines = append(in.Lines, payable.Line{Source: src, Amount: &amount})
	}
	out, e := payable.New(s, in)
	if e != nil {
		return payable.Payable{}, e
	}
	if p[6] != "-" {
		for _, part := range strings.Split(p[6], "+") {
			id, amount, ok := strings.Cut(part, ":")
			if !ok {
				t.Fatal("malformed ledger settlement", part)
			}
			if out, e = settle(out, s, org, id, amount); e != nil {
				return payable.Payable{}, e
			}
		}
	}
	switch p[7] {
	case "-":
		return out, nil
	case "CANCEL":
		return out.Cancel(s, ledgerNote)
	}
	t.Fatal("malformed ledger cancel field")
	return payable.Payable{}, nil
}

func costFields(c expense.CostTotals) fields {
	return fields{"org": alias(c.OrganizationID().String()), "start": c.Period().Start(), "end": c.Period().End(),
		"basis": string(c.Basis()), "approved": c.ApprovedCentavos().String(),
		"approved_count": strconv.Itoa(c.ApprovedCount()), "pending": c.PendingCentavos().String(),
		"pending_count": strconv.Itoa(c.PendingCount()), "excluded_count": strconv.Itoa(c.ExcludedCount()),
		"figure": c.FigureCentavos().String(), "includes_pending": strconv.FormatBool(c.IncludesPending())}
}

func vectorLedger(t *testing.T, args fields) (fields, error) {
	t.Helper()
	pool := map[string]expense.Entry{}
	costs, e := ledgerItems(t, args["costs"], pool)
	if e != nil {
		return nil, e
	}
	if _, e = ledgerItems(t, args["sources"], pool); e != nil {
		return nil, e
	}
	var payables []payable.Payable
	if raw := args["payables"]; raw != "" {
		for _, tuple := range strings.Split(raw, "~") {
			p, e := ledgerPayable(t, tuple, pool)
			if e != nil {
				return nil, e
			}
			payables = append(payables, p)
		}
	}
	period, e := expense.NewReportingPeriod(args["start"], args["end"])
	if e != nil {
		return nil, e
	}
	scope, basis := vectorScope(args["org"]), expense.Basis(args["basis"])
	views, e := payable.ComputeViews(scope, period, basis, costs, payables)
	if e != nil {
		return nil, e
	}
	// Single-cost proof: the cost view equals the totals of the costs alone.
	alone, e := expense.Totals(scope, period, basis, costs)
	if e != nil || !reflect.DeepEqual(costFields(alone), costFields(views.Cost())) {
		t.Fatal("cost view differs from the expense totals of the costs alone", e)
	}
	out := costFields(views.Cost())
	for k, v := range map[string]string{"payable_count": strconv.Itoa(views.PayableCount()),
		"open_count": strconv.Itoa(views.OpenCount()), "partial_count": strconv.Itoa(views.PartiallySettledCount()),
		"settled_count": strconv.Itoa(views.SettledCount()), "cancelled_count": strconv.Itoa(views.CancelledCount()),
		"obligation": views.ObligationCentavos().String(), "settled": views.SettledCentavos().String(),
		"remaining": views.RemainingCentavos().String()} {
		out[k] = v
	}
	return out, nil
}

func runVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "ENUM":
		raw, e := decodeText(f["value_hex"])
		if e != nil {
			t.Fatal(e)
		}
		if f["kind"] == "KIND" {
			v, e := payable.ParseKind(raw)
			return fields{"value": string(v)}, e
		}
		v, e := payable.ParseState(raw)
		return fields{"value": string(v)}, e
	case "SETTLEMENT":
		org := f.get("org", "A")
		ref, e := payable.NewSettlementRef(vectorScope(f.get("id_org", org)), fixtureID(f.get("id", "S1")))
		if e != nil {
			return nil, e
		}
		var amount *string
		if raw := f.get("amount", "97350"); raw != "?" {
			amount = &raw
		}
		s, e := payable.NewSettlement(vectorScope(org), ref, amount)
		if e != nil {
			return nil, e
		}
		return fields{"org": alias(s.Reference().OrganizationID().String()), "id": alias(s.Reference().ID()),
			"amount": strconv.FormatInt(s.AmountCentavos(), 10)}, nil
	case "PAYABLE":
		p, e := vectorPayable(t, f)
		if e != nil {
			return nil, e
		}
		return payableFields(p), nil
	case "LEDGER":
		return vectorLedger(t, f)
	}
	t.Fatal("unknown operation")
	return nil, nil
}

func TestSharedPayableVectors(t *testing.T) {
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
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || p[0] != strings.TrimSpace(p[0]) || ids[p[0]] {
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
	if len(ids) != corpusSize || !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", len(ids), counts)
	}
	for _, c := range errorCodes {
		if !codes[c.code] {
			t.Fatal("error code without a shared vector", c.code)
		}
	}
	if len(codes) != len(errorCodes) {
		t.Fatal("undocumented error code in corpus", codes)
	}
}
