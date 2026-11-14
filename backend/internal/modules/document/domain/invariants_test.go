package domain_test

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	doc "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	orgmedia "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

var dimensionStates = map[doc.Dimension][]doc.State{
	doc.Storage:        {doc.Uploading, doc.Received, doc.Stored, doc.Quarantined, doc.StorageFailed, doc.Replaced, doc.Deleted},
	doc.Extraction:     {doc.ExtractionNone, doc.ExtractionPending, doc.ExtractionDone, doc.ExtractionFailed},
	doc.Structure:      {doc.NotApplicable, doc.Unchecked, doc.StructureOK, doc.StructureInvalid},
	doc.Conference:     {doc.Unconferred, doc.ConferredOK, doc.Divergent, doc.Illegible},
	doc.Reconciliation: {doc.Unallocated, doc.PartiallyAllocated, doc.Reconciled, doc.OverAllocated},
	doc.Fiscal:         {doc.NotConsulted, doc.AuthorizedAsOf, doc.CancelledAsOf, doc.Unavailable},
}

func statusOf(s doc.Statuses, d doc.Dimension) doc.State {
	return map[doc.Dimension]doc.State{doc.Storage: s.Storage, doc.Extraction: s.Extraction, doc.Structure: s.Structure,
		doc.Conference: s.Conference, doc.Reconciliation: s.Reconciliation, doc.Fiscal: s.Fiscal}[d]
}

// The reachable graph of every commanded dimension changes exactly that
// dimension; every refused pair changes nothing at all.
func TestEveryTransitionChangesExactlyOneDimension(t *testing.T) {
	base := ok(newDoc(t, 1, 201).WithAmount(97350))(t)
	base = ok(base.Link(expense(t, 1, 211)))(t)
	base = ok(base.Allocate([]doc.AllocationLine{{Target: expense(t, 1, 211), Centavos: 60000}}))(t)
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for dim, states := range dimensionStates {
		if dim.Derived() {
			continue
		}
		t.Run(string(dim), func(t *testing.T) {
			initial := ok(doc.Initial(dim))(t)
			if statusOf(base.Statuses(), dim) != initial {
				t.Fatal("initial state differs")
			}
			seen := map[doc.State]doc.Document{initial: base}
			queue := []doc.State{initial}
			for len(queue) > 0 {
				from := queue[0]
				queue = queue[1:]
				current := seen[from]
				next := ok(doc.Allowed(dim, from))(t)
				for _, to := range states {
					clock = clock.Add(time.Second)
					at := clock
					command := doc.Transition{Dimension: dim, From: from, To: to}
					if dim == doc.Fiscal {
						command.ConsultedAt = &at
					}
					after, e := current.Apply(command)
					if !contains(next, to) {
						if !errors.Is(e, doc.ErrTransition) || !reflect.DeepEqual(after, current) {
							t.Fatal("refused pair changed the document", from, to, e)
						}
						continue
					}
					if e != nil {
						t.Fatal("allowed pair refused", from, to, e)
					}
					assertOnlyDimension(t, current, after, dim, to)
					if _, ok := seen[to]; !ok {
						seen[to] = after
						queue = append(queue, to)
					}
				}
			}
			if dim != doc.Storage && len(seen) != len(states) {
				t.Fatal("unreachable declared state", seen)
			}
			if _, ok := seen[doc.Replaced]; dim == doc.Storage && (ok || len(seen) != len(states)-1) {
				t.Fatal("REPLACED must be reachable only by replacement")
			}
		})
	}
}

func contains(states []doc.State, s doc.State) bool {
	for _, v := range states {
		if v == s {
			return true
		}
	}
	return false
}

func assertOnlyDimension(t *testing.T, before, after doc.Document, dim doc.Dimension, to doc.State) {
	t.Helper()
	b, a := before.Statuses(), after.Statuses()
	for other := range dimensionStates {
		want := statusOf(b, other)
		if other == dim {
			want = to
		}
		if statusOf(a, other) != want {
			t.Fatal("transition changed another dimension", dim, other)
		}
	}
	if after.ID() != before.ID() || after.Digest() != before.Digest() || after.OrganizationID() != before.OrganizationID() ||
		after.Replaces() != before.Replaces() || after.ReplacedBy() != before.ReplacedBy() ||
		!reflect.DeepEqual(after.Links(), before.Links()) || !reflect.DeepEqual(after.Allocations(), before.Allocations()) ||
		!reflect.DeepEqual(after.Amount(), before.Amount()) {
		t.Fatal("transition overwrote identity, links or money", dim)
	}
	if (dim != doc.Fiscal) && !reflect.DeepEqual(after.ConsultedAt(), before.ConsultedAt()) {
		t.Fatal("non-fiscal transition changed the consultation instant")
	}
}

func TestDerivedReconciliationRefusesCommands(t *testing.T) {
	d := newDoc(t, 1, 201)
	if !doc.Reconciliation.Derived() {
		t.Fatal("reconciliation must be derived")
	}
	for _, from := range dimensionStates[doc.Reconciliation] {
		if _, e := doc.Allowed(doc.Reconciliation, from); !errors.Is(e, doc.ErrDerived) {
			t.Fatal("reconciliation exposes commands")
		}
		for _, to := range dimensionStates[doc.Reconciliation] {
			if after, e := d.Apply(doc.Transition{Dimension: doc.Reconciliation, From: from, To: to}); !errors.Is(e, doc.ErrDerived) || !reflect.DeepEqual(after, d) {
				t.Fatal("reconciliation command accepted")
			}
		}
	}
}

func TestDigestFormatMatchesPrivateObjectIdentity(t *testing.T) {
	s := scope(t, 1)
	good := digest(0x1a)
	object := orgmedia.Object{OrgID: s.OrgID().String(), DocumentID: uuid(201), SHA256: good, MIME: "image/jpeg", Bytes: 1, Revision: 1, Synthetic: true}
	if object.Validate(s, uuid(201)) != nil {
		t.Fatal("synthetic private object invalid")
	}
	d := ok(doc.New(s, object.DocumentID, object.SHA256))(t)
	if ok, e := d.MatchesOriginal(object.SHA256); e != nil || !ok {
		t.Fatal("document original differs from its private object identity")
	}
	for _, bad := range []string{strings.ToUpper(good), good[:63], good + "1", "g" + good[1:], ""} {
		object.SHA256 = bad
		_, e := doc.New(s, uuid(201), bad)
		if object.Validate(s, uuid(201)) == nil || !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("digest profiles disagree", bad)
		}
		if _, e = d.MatchesOriginal(bad); !errors.Is(e, doc.ErrInvalid) {
			t.Fatal("malformed digest compared")
		}
	}
}

func TestAllocationsNeverExceedTheirSource(t *testing.T) {
	r := rand.New(rand.NewPCG(185, 189))
	targets := []doc.Target{expense(t, 1, 211), expense(t, 1, 212), expense(t, 1, 213), expense(t, 1, 214)}
	base := newDoc(t, 1, 201)
	for _, target := range targets {
		base = ok(base.Link(target))(t)
	}
	for i := 0; i < 2000; i++ {
		amount := r.Int64N(200000)
		d := ok(base.WithAmount(amount))(t)
		lines := []doc.AllocationLine{}
		for _, target := range targets[:r.IntN(len(targets)+1)] {
			lines = append(lines, doc.AllocationLine{Target: target, Centavos: 1 + r.Int64N(100000)})
		}
		var sum int64
		for _, line := range lines {
			sum += line.Centavos
		}
		after, e := d.Allocate(lines)
		if sum > amount {
			if !errors.Is(e, doc.ErrExceedsAmount) || !reflect.DeepEqual(after, d) {
				t.Fatal("excess allocation recorded", amount, sum, e)
			}
			continue
		}
		if e != nil || after.Allocated() != sum || *after.Remaining() != amount-sum {
			t.Fatal("fitting allocation refused or misreported", amount, sum, e)
		}
		if want := expectedReconciliation(amount, sum); after.Statuses().Reconciliation != want {
			t.Fatal("reconciliation not derived from exact sums", amount, sum)
		}
		lower := r.Int64N(amount + 1)
		corrected := ok(after.WithAmount(lower))(t)
		if corrected.Statuses().Reconciliation != expectedReconciliation(lower, sum) || !reflect.DeepEqual(corrected.Allocations(), after.Allocations()) {
			t.Fatal("amount correction changed allocations or misderived", lower, sum)
		}
		count := 1 + r.IntN(len(targets))
		split := ok(d.SplitEvenly(targets[:count]))(t)
		weights := make([]exact.Line, count)
		for j := range weights {
			one := int64(1)
			weights[j] = exact.Line{Order: int64(j), Weight: &one}
		}
		reference, e := exact.Allocate(&amount, weights)
		if e != nil {
			t.Fatal(e)
		}
		got := split.Allocations()
		k := 0
		for j, part := range reference.Parts {
			if part.Centavos == 0 {
				continue
			}
			if k >= len(got) || got[k].Target != targets[j] || got[k].Centavos != part.Centavos {
				t.Fatal("even split differs from the BR-MONEY-007 allocator", amount, count)
			}
			k++
		}
		if k != len(got) || split.Allocated() != amount {
			t.Fatal("even split lost or invented centavos", amount, count)
		}
	}
}

func expectedReconciliation(amount, sum int64) doc.State {
	switch {
	case sum == 0:
		return doc.Unallocated
	case sum < amount:
		return doc.PartiallyAllocated
	case sum == amount:
		return doc.Reconciled
	}
	return doc.OverAllocated
}

func TestConcurrentDerivationsDoNotShareState(t *testing.T) {
	base := ok(newDoc(t, 1, 201).WithAmount(1000))(t)
	base = ok(base.Link(expense(t, 1, 211)))(t)
	snapshot := base
	var wg sync.WaitGroup
	results := make([]doc.Document, 16)
	targets := make([]doc.Target, len(results))
	for i := range targets {
		targets[i] = expense(t, 1, 300+i)
	}
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := targets[i]
			d, e := base.Link(target)
			if e == nil {
				d, e = d.Allocate([]doc.AllocationLine{{Target: target, Centavos: int64(i + 1)}})
			}
			if e != nil {
				t.Error(e)
				return
			}
			results[i] = d
		}(i)
	}
	wg.Wait()
	if !reflect.DeepEqual(base, snapshot) {
		t.Fatal("concurrent derivation mutated the shared document")
	}
	for i, d := range results {
		links := d.Links()
		if len(links) != 2 || links[1].ID() != uuid(300+i) || d.Allocated() != int64(i+1) {
			t.Fatal("concurrent derivations shared a backing array", i)
		}
	}
}
