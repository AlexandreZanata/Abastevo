package domain

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDomainStdlibOnly(t *testing.T) {
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, ".") {
				t.Errorf("%s imports non-stdlib %q", name, path)
			}
		}
	}
}

func validDecisionParams() DecisionParams {
	return DecisionParams{
		ID: "t0000000-0000-4000-8000-000000000001", ContributorRef: "tok-c1",
		Tier: TierEstablished, Reason: "pilot review batch 7",
		CaseRefs: []string{"case-101", "case-102"}, OccurredAt: time.Now(),
	}
}

func TestNewDecisionTiers(t *testing.T) {
	d, evt, err := NewDecision(validDecisionParams())
	if err != nil {
		t.Fatalf("established rejected: %v", err)
	}
	if d.Tier != TierEstablished || d.PolicyVersion != PolicyV1 {
		t.Errorf("decision = %+v", d)
	}
	if evt.ContributorRef != "tok-c1" || evt.Tier != TierEstablished || evt.Name() != "ContributorTrustChanged" {
		t.Errorf("event = %+v", evt)
	}
	p := validDecisionParams()
	p.Tier = TierNew
	p.CaseRefs = nil
	if _, _, err := NewDecision(p); err != nil {
		t.Errorf("plain NEW rejected: %v", err)
	}
	p.Tier = TierBlocked
	p.CaseRefs = nil
	if _, _, err := NewDecision(p); !errors.Is(err, ErrCaseRequired) {
		t.Errorf("caseless block = %v", err)
	}
	p.CaseRefs = []string{"case-9"}
	if _, _, err := NewDecision(p); err != nil {
		t.Errorf("audited block rejected: %v", err)
	}
}

func TestNewDecisionValidation(t *testing.T) {
	p := validDecisionParams()
	p.Tier = "GOLD"
	if _, _, err := NewDecision(p); !errors.Is(err, ErrUnknownTier) {
		t.Errorf("unknown tier = %v", err)
	}
	p = validDecisionParams()
	p.CaseRefs = nil
	if _, _, err := NewDecision(p); !errors.Is(err, ErrCaseRequired) {
		t.Errorf("caseless established = %v", err)
	}
	cases := map[string]func(*DecisionParams){
		"id":          func(p *DecisionParams) { p.ID = "" },
		"contributor": func(p *DecisionParams) { p.ContributorRef = " " },
		"reason":      func(p *DecisionParams) { p.Reason = "" },
		"clock":       func(p *DecisionParams) { p.OccurredAt = time.Time{} },
	}
	for name, mutate := range cases {
		params := validDecisionParams()
		mutate(&params)
		if _, _, err := NewDecision(params); !errors.Is(err, ErrInvalidDecision) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func TestEvaluateBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   ReviewHistory
		want string
	}{
		{"fresh identity", ReviewHistory{}, TierNew},
		{"young key", ReviewHistory{KeyAgeDays: 13, ReviewedSuccesses: 50, ActiveDays: 20}, TierNew},
		{"few successes", ReviewHistory{KeyAgeDays: 14, ReviewedSuccesses: 9, ActiveDays: 20}, TierNew},
		{"few days", ReviewHistory{KeyAgeDays: 14, ReviewedSuccesses: 10, ActiveDays: 4}, TierNew},
		{"exact floor", ReviewHistory{KeyAgeDays: 14, ReviewedSuccesses: 10, ActiveDays: 5}, TierEstablished},
		{"well above", ReviewHistory{KeyAgeDays: 90, ReviewedSuccesses: 200, ActiveDays: 60}, TierEstablished},
		{"abuse blocks promotion", ReviewHistory{KeyAgeDays: 90, ReviewedSuccesses: 200, ActiveDays: 60, UpheldAbuse: true}, TierNew},
	}
	for _, c := range cases {
		if got := Evaluate(c.in); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPaymentInvarianceByConstruction(t *testing.T) {
	// Promotion reads exactly four inputs; purchase, email, device and
	// volume fields cannot influence what does not exist.
	forbidden := []string{"pay", "purchase", "subscri", "email", "device", "premium", "volume", "count"}
	typ := reflect.TypeOf(ReviewHistory{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		for _, bad := range forbidden {
			if strings.Contains(name, bad) {
				t.Errorf("ReviewHistory.%s invites %q influence", typ.Field(i).Name, bad)
			}
		}
	}
	if typ.NumField() != 4 {
		t.Errorf("ReviewHistory has %d fields, want exactly the four reviewed inputs", typ.NumField())
	}
}

func TestCurrentTierFollowsLatest(t *testing.T) {
	if got := CurrentTier(nil); got != TierNew {
		t.Errorf("empty history = %q, want NEW", got)
	}
	at := time.Now()
	mk := func(tier string, n int) Decision {
		d, _, err := NewDecision(DecisionParams{
			ID:             "t0000000-0000-4000-8000-0000000000" + string(rune('0'+n)),
			ContributorRef: "tok-c1", Tier: tier, Reason: "review",
			CaseRefs: []string{"case-1"}, OccurredAt: at.Add(time.Duration(n) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	// Rehabilitation arrives as a new decision: BLOCKED reverses through
	// audited history, never through edits.
	history := []Decision{mk(TierEstablished, 1), mk(TierBlocked, 2), mk(TierEstablished, 3)}
	if got := CurrentTier(history); got != TierEstablished {
		t.Errorf("current = %q, want ESTABLISHED", got)
	}
	if got := CurrentTier(history[:2]); got != TierBlocked {
		t.Errorf("current = %q, want BLOCKED", got)
	}
}
