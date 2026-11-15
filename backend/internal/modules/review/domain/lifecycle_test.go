package domain_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
)

func eligible(t *testing.T, kind review.Kind) review.Facts {
	t.Helper()
	return defaultFacts(t, kind)
}

func apply(t *testing.T, c review.Case, kind review.Kind, reason string) review.Case {
	t.Helper()
	next, e := c.Apply(review.Command{Kind: kind, Revision: c.Revision(), Reason: reason}, eligible(t, kind))
	if e != nil {
		t.Fatalf("%s refused: %v", kind, e)
	}
	return next
}

func TestFailureVocabularyIsClosedAndDistinct(t *testing.T) {
	seen := map[error]string{}
	for code, sentinel := range errorCodes {
		if other, dup := seen[sentinel]; dup {
			t.Fatal("shared sentinel", code, other)
		}
		seen[sentinel] = code
		for otherCode, other := range errorCodes {
			if otherCode != code && errors.Is(sentinel, other) {
				t.Fatal("sentinel wraps another", code, otherCode)
			}
		}
	}
	if review.MaxReasonScalars != 280 {
		t.Fatal("reason bound must reuse the existing 280-scalar rule")
	}
}

func TestInvalidTokensHaveNoBehaviour(t *testing.T) {
	bad := review.State("READY_TO_SEND")
	if bad.Terminal() {
		t.Fatal("unknown state reported terminal")
	}
	if _, e := bad.Basis(); !errors.Is(e, review.ErrInvalid) {
		t.Fatal("unknown state received a counting basis")
	}
	k := review.Kind("DECIDE")
	if k.Role() != "" || k.ReasonPolicy() != "" {
		t.Fatal("unknown command received a role or reason policy")
	}
	if _, e := (review.Case{}).Apply(review.Command{Kind: review.Submit, Revision: 1}, eligible(t, review.Submit)); !errors.Is(e, review.ErrInvalid) {
		t.Fatal("zero case accepted a command")
	}
	if (review.Case{}).History() != nil {
		t.Fatal("zero case exposes history")
	}
}

func TestDraftStartsAtRevisionOne(t *testing.T) {
	c := review.NewDraft()
	if c.State() != review.Draft || c.Revision() != 1 || len(c.History()) != 0 {
		t.Fatal("draft must start as DRAFT revision 1 with no history")
	}
}

func TestHistoryIsACopyAndAppendDoesNotAlias(t *testing.T) {
	c := apply(t, review.NewDraft(), review.Submit, "")
	c = apply(t, c, review.Open, "")
	h := c.History()
	h[0].State = review.Approved
	h[0].Reason = "forged"
	if c.History()[0].State != review.Submitted || c.History()[0].Reason != "" {
		t.Fatal("mutating returned history changed the case")
	}
	// Replay from an input with spare capacity, then branch twice.
	stored := make([]review.Entry, 2, 16)
	copy(stored, c.History())
	base, e := review.Replay(stored)
	if e != nil {
		t.Fatal(e)
	}
	stored[1].State = review.Approved
	if base.History()[1].State != review.InReview {
		t.Fatal("replay kept a reference to its input")
	}
	approved := apply(t, base, review.Approve, "")
	returned := apply(t, base, review.Return, "Missing receipt")
	if approved.History()[2].Kind != review.Approve || returned.History()[2].Kind != review.Return {
		t.Fatal("branches from one case share history storage")
	}
	if len(base.History()) != 2 || base.State() != review.InReview {
		t.Fatal("branching changed the base case")
	}
}

func TestConcurrentDecisionsOnOneValueAreIndependent(t *testing.T) {
	c := apply(t, apply(t, review.NewDraft(), review.Submit, ""), review.Open, "")
	kinds := []review.Kind{review.Approve, review.Return, review.Reject}
	results := make([]review.Case, 30)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := kinds[i%len(kinds)]
			reason := ""
			if kind != review.Approve {
				reason = "Reason " + string(kind)
			}
			next, e := c.Apply(review.Command{Kind: kind, Revision: 1, Reason: reason}, eligible(t, kind))
			if e != nil {
				t.Error(e)
				return
			}
			results[i] = next
		}(i)
	}
	wg.Wait()
	for i, r := range results {
		if h := r.History(); len(h) != 3 || h[2].Kind != kinds[i%len(kinds)] {
			t.Fatal("concurrent applications interfered")
		}
	}
	if c.State() != review.InReview || len(c.History()) != 2 {
		t.Fatal("concurrent applications changed the shared value")
	}
}

func TestReasonRuleMatchesExistingPrivateBound(t *testing.T) {
	c := apply(t, apply(t, review.NewDraft(), review.Submit, ""), review.Open, "")
	for name, tc := range map[string]struct {
		reason string
		want   error
	}{
		"invalid utf8":       {"bad\xffreason", review.ErrInvalid},
		"lone continuation":  {"\x80", review.ErrInvalid},
		"280 scalars":        {strings.Repeat("ç", 280), nil},
		"281 scalars":        {strings.Repeat("ç", 281), review.ErrInvalid},
		"format char kept":   {"zero\u200bwidth", nil},
		"only format char":   {"\u200b", nil},
		"vertical tab blank": {"\v", review.ErrInvalid},
		"em space blank":     {"\u2003", review.ErrReasonRequired},
	} {
		_, e := c.Apply(review.Command{Kind: review.Return, Revision: 1, Reason: tc.reason}, eligible(t, review.Return))
		if !errors.Is(e, tc.want) {
			t.Fatalf("%s: got %v want %v", name, e, tc.want)
		}
	}
}

func TestIdentityNeedsOneCanonicalIdentifier(t *testing.T) {
	c := apply(t, apply(t, review.NewDraft(), review.Submit, ""), review.Open, "")
	facts := eligible(t, review.Approve)
	facts.Beneficiary = &review.Identity{Employees: []string{uuid(23)}}
	if _, e := c.Apply(review.Command{Kind: review.Approve, Revision: 1}, facts); e != nil {
		t.Fatal("employee-only beneficiary refused", e)
	}
	facts.Actor = review.Identity{Accounts: []string{uuid(2), uuid(2)}}
	if _, e := c.Apply(review.Command{Kind: review.Approve, Revision: 1}, facts); e != nil {
		t.Fatal("duplicate identifier in one set refused", e)
	}
	facts.Actor = review.Identity{Accounts: []string{uuid(2)}, Memberships: []string{""}}
	if _, e := c.Apply(review.Command{Kind: review.Approve, Revision: 1}, facts); !errors.Is(e, review.ErrInvalid) {
		t.Fatal("empty identifier accepted", e)
	}
}
