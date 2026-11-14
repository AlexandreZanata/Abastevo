package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	doc "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, e := kernel.ParseOrgID(uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	s, e := kernel.NewOrgScope(id)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

// ok binds a (value, error) result to a test that must not see the error.
func ok[T any](v T, e error) func(*testing.T) T {
	return func(t *testing.T) T {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
}
func digest(n int) string { return strings.Repeat(fmt.Sprintf("%02x", n), 32) }
func newDoc(t *testing.T, org, id int) doc.Document {
	t.Helper()
	return ok(doc.New(scope(t, org), uuid(id), digest(id%256)))(t)
}
func step(t *testing.T, d doc.Document, dim doc.Dimension, from, to doc.State) doc.Document {
	t.Helper()
	return ok(d.Apply(doc.Transition{Dimension: dim, From: from, To: to}))(t)
}
func stored(t *testing.T, org, id int) doc.Document {
	t.Helper()
	d := step(t, newDoc(t, org, id), doc.Storage, doc.Uploading, doc.Received)
	return step(t, d, doc.Storage, doc.Received, doc.Stored)
}
func expense(t *testing.T, org, id int) doc.Target {
	t.Helper()
	return ok(doc.NewTarget(scope(t, org), doc.Expense, uuid(id)))(t)
}

func TestTargetConstructionIsScopedAndClosed(t *testing.T) {
	if _, e := doc.NewTarget(kernel.OrgScope{}, doc.Expense, uuid(9)); !errors.Is(e, doc.ErrScope) {
		t.Fatal("zero scope accepted", e)
	}
	for _, kind := range []doc.TargetKind{"", "MAINTENANCE", "expense", "EXPENSE "} {
		if _, e := doc.NewTarget(scope(t, 1), kind, uuid(9)); !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("undeclared kind accepted", kind)
		}
	}
	for _, id := range []string{"", "00000000-0000-0000-0000-000000000000", strings.ToUpper(uuid(9)), "not-a-uuid"} {
		if _, e := doc.NewTarget(scope(t, 1), doc.Expense, id); !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("malformed target identifier accepted", id)
		}
	}
	target := expense(t, 1, 9)
	if target.Kind() != doc.Expense || target.ID() != uuid(9) || target.OrganizationID() != scope(t, 1).OrgID() {
		t.Fatal("target accessors changed")
	}
}

func TestZeroValuesAreRefused(t *testing.T) {
	var zero doc.Document
	valid := newDoc(t, 1, 201)
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	checks := map[string]error{}
	_, checks["apply"] = zero.Apply(doc.Transition{Dimension: doc.Fiscal, From: doc.NotConsulted, To: doc.Unavailable, ConsultedAt: &at})
	_, checks["amount"] = zero.WithAmount(1)
	_, checks["link"] = zero.Link(expense(t, 1, 9))
	_, checks["allocate"] = zero.Allocate(nil)
	_, checks["split"] = zero.SplitEvenly([]doc.Target{expense(t, 1, 9)})
	_, checks["match"] = zero.MatchesOriginal(digest(1))
	_, _, checks["replace-old"] = doc.Replace(zero, valid)
	_, _, checks["replace-new"] = doc.Replace(valid, zero)
	_, checks["zero-target-link"] = valid.Link(doc.Target{})
	withAmount := ok(valid.WithAmount(10))(t)
	_, checks["zero-target-allocate"] = withAmount.Allocate([]doc.AllocationLine{{Centavos: 1}})
	_, checks["zero-target-split"] = withAmount.SplitEvenly([]doc.Target{{}})
	_, checks["unknown-dimension"] = valid.Apply(doc.Transition{Dimension: "VALIDATION", From: doc.Uploading, To: doc.Received})
	_, checks["foreign-state"] = valid.Apply(doc.Transition{Dimension: doc.Storage, From: doc.ExtractionNone, To: doc.Received})
	_, checks["foreign-target-state"] = valid.Apply(doc.Transition{Dimension: doc.Storage, From: doc.Uploading, To: doc.Unconferred})
	for name, e := range checks {
		if !errors.Is(e, doc.ErrInvalid) {
			t.Error(name, "accepted or wrong error", e)
		}
	}
	if _, e := doc.New(kernel.OrgScope{}, uuid(201), digest(1)); !errors.Is(e, doc.ErrScope) {
		t.Fatal("document created without an organization scope", e)
	}
	if s := zero.Statuses(); s != (doc.Statuses{}) || zero.Amount() != nil || zero.Remaining() != nil || zero.Allocated() != 0 {
		t.Fatal("zero document projected statuses")
	}
	for _, d := range []doc.Dimension{"", "VALIDATION", "storage"} {
		if _, e := doc.Initial(d); !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("unknown dimension has an initial state")
		}
		if _, e := doc.Allowed(d, doc.Uploading); !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("unknown dimension has transitions")
		}
		if d.Derived() {
			t.Fatal("unknown dimension is derived")
		}
	}
	if _, e := doc.Allowed(doc.Storage, doc.ExtractionNone); !errors.Is(e, doc.ErrInvalid) {
		t.Fatal("foreign state has storage transitions")
	}
}

func TestReplacementChainKeepsBothOriginals(t *testing.T) {
	first, second, third, other := stored(t, 1, 201), stored(t, 1, 202), stored(t, 1, 203), stored(t, 1, 204)
	oldFirst, second2 := must2(t, first, second)
	if oldFirst.Statuses().Storage != doc.Replaced || oldFirst.ReplacedBy() != uuid(202) || second2.Replaces() != uuid(201) || oldFirst.Digest() != first.Digest() || second2.Digest() != second.Digest() {
		t.Fatal("replacement lost an original or a reference")
	}
	if first.Statuses().Storage != doc.Stored || first.ReplacedBy() != "" || second.Replaces() != "" {
		t.Fatal("replacement mutated its inputs")
	}
	if _, _, e := doc.Replace(oldFirst, third); !errors.Is(e, doc.ErrTransition) {
		t.Fatal("a replaced original was replaced twice", e)
	}
	if _, _, e := doc.Replace(other, second2); !errors.Is(e, doc.ErrDuplicate) {
		t.Fatal("a successor replaced two originals", e)
	}
	oldSecond, third2 := must2(t, second2, third)
	if oldSecond.Replaces() != uuid(201) || oldSecond.ReplacedBy() != uuid(203) || third2.Replaces() != uuid(202) {
		t.Fatal("replacement chain lost a reference")
	}
	deleted := step(t, oldFirst, doc.Storage, doc.Replaced, doc.Deleted)
	if deleted.ReplacedBy() != uuid(202) || deleted.Digest() != first.Digest() {
		t.Fatal("retention disposal forgot the replacement history")
	}
}

func must2(t *testing.T, old, successor doc.Document) (doc.Document, doc.Document) {
	t.Helper()
	o, n, e := doc.Replace(old, successor)
	if e != nil {
		t.Fatal(e)
	}
	return o, n
}

func TestAllocationAndSplitReuseTheExistingLineBound(t *testing.T) {
	d := ok(newDoc(t, 1, 201).WithAmount(1000000000000))(t)
	targets := make([]doc.Target, 0, 1001)
	for i := 0; i < 1001; i++ {
		target := expense(t, 1, 10000+i)
		targets = append(targets, target)
		d = ok(d.Link(target))(t)
	}
	lines := make([]doc.AllocationLine, 0, 1001)
	for _, target := range targets {
		lines = append(lines, doc.AllocationLine{Target: target, Centavos: 1})
	}
	if _, e := d.Allocate(lines); !errors.Is(e, doc.ErrInvalid) {
		t.Fatal("1001 allocation lines accepted", e)
	}
	if _, e := d.SplitEvenly(targets); !errors.Is(e, doc.ErrInvalid) {
		t.Fatal("1001 split targets accepted", e)
	}
	full := ok(d.Allocate(lines[:1000]))(t)
	if full.Allocated() != 1000 || len(full.Allocations()) != 1000 || full.Statuses().Reconciliation != doc.PartiallyAllocated {
		t.Fatal("1000 bounded lines refused")
	}
	split := ok(d.SplitEvenly(targets[:1000]))(t)
	if split.Allocated() != 1000000000000 || split.Statuses().Reconciliation != doc.Reconciled {
		t.Fatal("bounded split did not preserve the whole")
	}
}

func TestFiscalConsultationInstant(t *testing.T) {
	base := newDoc(t, 1, 201)
	south := time.FixedZone("BRT", -3*3600)
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, south)
	d := ok(base.Apply(doc.Transition{Dimension: doc.Fiscal, From: doc.NotConsulted, To: doc.AuthorizedAsOf, ConsultedAt: &at}))(t)
	got := d.ConsultedAt()
	if got == nil || !got.Equal(at) || got.Location() != time.UTC {
		t.Fatal("consultation instant not normalized to UTC")
	}
	at = at.Add(time.Hour)
	*got = got.Add(48 * time.Hour)
	if again := d.ConsultedAt(); !again.Equal(time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("consultation instant aliased")
	}
	for _, bad := range []time.Time{time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(0, 12, 31, 23, 59, 59, 0, time.UTC)} {
		if _, e := base.Apply(doc.Transition{Dimension: doc.Fiscal, From: doc.NotConsulted, To: doc.Unavailable, ConsultedAt: &bad}); !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("out-of-profile instant accepted", bad)
		}
	}
	minimum := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, e := base.Apply(doc.Transition{Dimension: doc.Fiscal, From: doc.NotConsulted, To: doc.Unavailable, ConsultedAt: &minimum}); e != nil {
		t.Fatal("minimum profile instant refused", e)
	}
}

func TestDocumentValuesAreImmutableCopies(t *testing.T) {
	base := ok(newDoc(t, 1, 201).WithAmount(100))(t)
	a, b, c := expense(t, 1, 211), expense(t, 1, 212), expense(t, 1, 213)
	one := ok(base.Link(a))(t)
	two := ok(one.Link(b))(t)
	three := ok(one.Link(c))(t)
	if got := two.Links(); len(got) != 2 || got[1] != b {
		t.Fatal("sibling link overwrote a shared backing array")
	}
	if got := three.Links(); len(got) != 2 || got[1] != c {
		t.Fatal("sibling link lost")
	}
	if len(base.Links()) != 0 || len(one.Links()) != 1 {
		t.Fatal("link mutated its receiver")
	}
	links := two.Links()
	links[0] = c
	if two.Links()[0] != a {
		t.Fatal("links slice aliased")
	}
	lines := []doc.AllocationLine{{Target: a, Centavos: 60}, {Target: b, Centavos: 40}}
	allocated := ok(two.Allocate(lines))(t)
	lines[0].Centavos = 100
	got := allocated.Allocations()
	got[1].Centavos = 99
	if all := allocated.Allocations(); all[0].Centavos != 60 || all[1].Centavos != 40 || allocated.Allocated() != 100 {
		t.Fatal("allocations aliased")
	}
	amount := allocated.Amount()
	*amount = 1
	if *allocated.Amount() != 100 || len(two.Allocations()) != 0 {
		t.Fatal("amount aliased or receiver mutated")
	}
}

func TestDocumentValuesAreRedacted(t *testing.T) {
	d := ok(stored(t, 1, 201).Link(expense(t, 1, 211)))(t)
	d = ok(d.WithAmount(97350))(t)
	d = ok(d.Allocate([]doc.AllocationLine{{Target: expense(t, 1, 211), Centavos: 97350}}))(t)
	secrets := []string{uuid(201), uuid(211), uuid(1), digest(201 % 256), "97350"}
	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("document", "doc", d, "target", d.Links()[0], "line", d.Allocations()[0].Target)
	encoded, e := json.Marshal(struct {
		D doc.Document
		T doc.Target
	}{d, d.Links()[0]})
	if e != nil {
		t.Fatal(e)
	}
	for _, text := range []string{fmt.Sprint(d), fmt.Sprintf("%+v %#v %s %q", d, d, d, d), fmt.Sprintf("%v", d.Links()[0]), logged.String(), string(encoded)} {
		for _, secret := range secrets {
			if strings.Contains(text, secret) {
				t.Fatal("document value leaked into text output")
			}
		}
	}
}
