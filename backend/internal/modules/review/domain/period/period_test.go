package period

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
)

// B-BR-EMP-211: only canonical calendar months are periods.
func TestParseMonthAcceptsOnlyCanonicalMonths(t *testing.T) {
	for _, raw := range []string{"2026-10", "0001-01", "9999-12", "2026-01", "2028-02"} {
		m, err := ParseMonth(raw)
		if err != nil || !m.Valid() || m.String() != raw {
			t.Fatalf("canonical month %q refused: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "2026-13", "2026-00", "0000-01", "2026-1", "26-10", " 2026-10", "2026-10 ", "2026-10-01", "+2026-10", "2026/10", "２０２６-10", "10000-01", "2026-1a", "-001-10"} {
		if m, err := ParseMonth(raw); !errors.Is(err, ErrInvalid) || m.Valid() || m != (Month{}) {
			t.Fatalf("non-canonical month %q accepted", raw)
		}
	}
	if (Month{}).Valid() || (Month{}).String() != "" || (Month{}).Start() != "" || (Month{}).End() != "" || (Month{}).Contains("2026-10-01") {
		t.Fatal("zero month is not a period")
	}
}

// B-BR-EMP-211: half-open ranges, including year ends and leap months.
func TestMonthRangeIsHalfOpen(t *testing.T) {
	for _, c := range []struct{ month, start, end, last, next, before string }{
		{"2026-10", "2026-10-01", "2026-11-01", "2026-10-31", "2026-11-01", "2026-09-30"},
		{"2026-12", "2026-12-01", "2027-01-01", "2026-12-31", "2027-01-01", "2026-11-30"},
		{"2028-02", "2028-02-01", "2028-03-01", "2028-02-29", "2028-03-01", "2028-01-31"},
		{"2026-02", "2026-02-01", "2026-03-01", "2026-02-28", "2026-03-01", "2026-01-31"},
		{"0001-01", "0001-01-01", "0001-02-01", "0001-01-31", "0001-02-01", ""},
		{"9999-12", "9999-12-01", "10000-01-01", "9999-12-31", "", "9999-11-30"},
	} {
		m, err := ParseMonth(c.month)
		if err != nil || m.Start() != c.start || m.End() != c.end {
			t.Fatalf("%s range %s..%s, want %s..%s (%v)", c.month, m.Start(), m.End(), c.start, c.end, err)
		}
		if !m.Contains(c.start) || !m.Contains(c.last) {
			t.Fatal("period lost an included day", c.month)
		}
		if c.next != "" && m.Contains(c.next) || c.before != "" && m.Contains(c.before) {
			t.Fatal("period admitted an excluded day", c.month)
		}
		for _, day := range []string{c.start, c.last} {
			if got, err := MonthOf(day); err != nil || got != m {
				t.Fatal("reporting day attributed to another period", day, got, err)
			}
		}
	}
	for _, day := range []string{"", "2026-02-30", "2026-10-31T00:00:00Z", "2026-10-1", "0000-12-31", "2026-13-01", " 2026-10-01"} {
		if _, err := MonthOf(day); !errors.Is(err, ErrInvalid) {
			t.Fatal("non-canonical reporting day accepted", day)
		}
		if m, _ := ParseMonth("2026-10"); m.Contains(day) {
			t.Fatal("non-canonical day contained", day)
		}
	}
}

func TestHeadParityAndInitialState(t *testing.T) {
	if Initial() != (Head{State: Open, Revision: 1}) || Initial().Validate() != nil {
		t.Fatal("implicit initial head must be OPEN revision 1")
	}
	for _, h := range []Head{{Locked, 2}, {Reopened, 3}, {Locked, 4}, {Locked, math.MaxInt64 - 1}, {Reopened, math.MaxInt64}} {
		if h.Validate() != nil {
			t.Fatal("valid head refused", h)
		}
	}
	for _, h := range []Head{{}, {Open, 0}, {Open, 2}, {Locked, 1}, {Locked, 3}, {Reopened, 2}, {Reopened, 1}, {"VOIDED", 2}, {"locked", 2}, {Locked, -2}, {Locked, 0}} {
		if !errors.Is(h.Validate(), ErrInvalid) {
			t.Fatal("inconsistent head accepted", h)
		}
	}
}

func TestDecideAcceptedTransitions(t *testing.T) {
	reason := "  Synthetic correction of an approved October cost  "
	for _, c := range []struct {
		head Head
		cmd  Command
		want Transition
	}{
		{Initial(), Command{ActionClose, 1, ""}, Transition{Lock, Open, Locked, 2, ClosePermission, ""}},
		{Head{Locked, 2}, Command{ActionReopen, 2, reason}, Transition{Reopen, Locked, Reopened, 3, ReopenPermission, reason}},
		{Head{Reopened, 3}, Command{ActionClose, 3, ""}, Transition{Relock, Reopened, Locked, 4, ClosePermission, ""}},
		{Head{Locked, 4}, Command{ActionReopen, 4, "x"}, Transition{Reopen, Locked, Reopened, 5, ReopenPermission, "x"}},
	} {
		got, err := Decide(c.head, c.cmd)
		if err != nil || got != c.want {
			t.Fatalf("transition from %v: got %+v %v want %+v", c.head, got, err, c.want)
		}
		if got.Validate() != nil || got.After() != (Head{c.want.To, c.want.Revision}) {
			t.Fatal("accepted transition is not a consistent stored fact")
		}
	}
}

func TestDecideRefusalsInOrder(t *testing.T) {
	ok := "Synthetic reason"
	for _, c := range []struct {
		name string
		head Head
		cmd  Command
		want error
	}{
		{"invalid head", Head{Locked, 3}, Command{ActionClose, 3, ""}, ErrInvalid},
		{"unknown action", Initial(), Command{"OPEN", 1, ""}, ErrInvalid},
		{"lower-case action", Initial(), Command{"close", 1, ""}, ErrInvalid},
		{"zero expected", Initial(), Command{ActionClose, 0, ""}, ErrInvalid},
		{"negative expected", Head{Locked, 2}, Command{ActionReopen, -2, ok}, ErrInvalid},
		{"close with reason", Initial(), Command{ActionClose, 1, ok}, ErrInvalid},
		{"revision overflow", Head{Reopened, math.MaxInt64}, Command{ActionClose, math.MaxInt64, ""}, ErrInvalid},
		{"stale close", Head{Locked, 2}, Command{ActionClose, 1, ""}, ErrConflict},
		{"future close", Initial(), Command{ActionClose, 2, ""}, ErrConflict},
		{"stale reopen without reason", Head{Reopened, 3}, Command{ActionReopen, 2, ""}, ErrConflict},
		{"double close", Head{Locked, 2}, Command{ActionClose, 2, ""}, ErrTransition},
		{"double close after relock", Head{Locked, 4}, Command{ActionClose, 4, ""}, ErrTransition},
		{"reopen open", Initial(), Command{ActionReopen, 1, ok}, ErrTransition},
		{"reopen reopened", Head{Reopened, 3}, Command{ActionReopen, 3, ok}, ErrTransition},
		{"reopen without reason", Head{Locked, 2}, Command{ActionReopen, 2, ""}, ErrReasonRequired},
		{"reopen blank reason", Head{Locked, 2}, Command{ActionReopen, 2, "  　 "}, ErrReasonRequired},
		{"reopen control reason", Head{Locked, 2}, Command{ActionReopen, 2, "Synthetic\nreason"}, ErrReasonInvalid},
		{"reopen long reason", Head{Locked, 2}, Command{ActionReopen, 2, strings.Repeat("é", 281)}, ErrReasonInvalid},
		{"reopen invalid UTF-8", Head{Locked, 2}, Command{ActionReopen, 2, "Synthetic \xff"}, ErrReasonInvalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decide(c.head, c.cmd)
			if !errors.Is(err, c.want) || got != (Transition{}) {
				t.Fatalf("got %+v %v, want %v", got, err, c.want)
			}
			for _, other := range []error{ErrInvalid, ErrConflict, ErrTransition, ErrReasonRequired, ErrReasonInvalid} {
				if other != c.want && errors.Is(err, other) {
					t.Fatal("ambiguous refusal")
				}
			}
		})
	}
}

// Exhaustive invariants: every accepted command appends exactly revision+1
// from the current state, and no stale or illegal command is ever accepted.
func TestDecideInvariants(t *testing.T) {
	heads := []Head{Initial()}
	for r := int64(2); r <= 9; r++ {
		if r%2 == 0 {
			heads = append(heads, Head{Locked, r})
		} else {
			heads = append(heads, Head{Reopened, r})
		}
	}
	accepted := 0
	for _, h := range heads {
		for _, a := range []Action{ActionClose, ActionReopen} {
			for expected := int64(0); expected <= 10; expected++ {
				for _, reason := range []string{"", "Synthetic reason", " "} {
					got, err := Decide(h, Command{a, expected, reason})
					if err != nil {
						if got != (Transition{}) {
							t.Fatal("refusal returned a transition")
						}
						continue
					}
					accepted++
					if expected != h.Revision || got.Revision != h.Revision+1 || got.From != h.State || got.Validate() != nil || got.After().Validate() != nil {
						t.Fatal("accepted transition broke the revision chain", h, a, expected)
					}
					switch {
					case a == ActionClose && h.State == Open:
						if got.Kind != Lock || got.To != Locked || reason != "" {
							t.Fatal("first close is not LOCK")
						}
					case a == ActionClose && h.State == Reopened:
						if got.Kind != Relock || got.To != Locked || reason != "" {
							t.Fatal("close of a reopened period is not RELOCK")
						}
					case a == ActionReopen && h.State == Locked:
						if got.Kind != Reopen || got.To != Reopened || got.Reason != reason || CheckReason(reason) != nil {
							t.Fatal("reopen lost or skipped its reason")
						}
					default:
						t.Fatal("illegal transition accepted", h, a)
					}
				}
			}
		}
	}
	// One accepted (action, reason) per head and current expected revision:
	// OPEN and REOPENED accept CLOSE without reason; LOCKED accepts REOPEN with reason.
	if accepted != len(heads) {
		t.Fatal("unexpected accepted count", accepted)
	}
}

func TestTransitionValidateRejectsInconsistentFacts(t *testing.T) {
	good := []Transition{
		{Lock, Open, Locked, 2, ClosePermission, ""},
		{Reopen, Locked, Reopened, 3, ReopenPermission, "Synthetic reason"},
		{Relock, Reopened, Locked, 4, ClosePermission, ""},
	}
	for _, v := range good {
		if v.Validate() != nil {
			t.Fatal("valid stored transition refused", v)
		}
	}
	for _, v := range []Transition{
		{},
		{Lock, Open, Locked, 4, ClosePermission, ""},
		{Lock, Open, Locked, 2, ReopenPermission, ""},
		{Lock, Open, Locked, 2, ClosePermission, "reason"},
		{Lock, Reopened, Locked, 2, ClosePermission, ""},
		{Reopen, Locked, Reopened, 3, ReopenPermission, ""},
		{Reopen, Locked, Reopened, 3, ClosePermission, "Synthetic reason"},
		{Reopen, Locked, Reopened, 2, ReopenPermission, "Synthetic reason"},
		{Reopen, Open, Reopened, 3, ReopenPermission, "Synthetic reason"},
		{Relock, Reopened, Locked, 2, ClosePermission, ""},
		{Relock, Reopened, Locked, 5, ClosePermission, ""},
		{Relock, Locked, Locked, 4, ClosePermission, ""},
		{"VOID", Locked, Open, 3, ClosePermission, ""},
	} {
		if !errors.Is(v.Validate(), ErrInvalid) {
			t.Fatal("inconsistent stored transition accepted", v)
		}
		if v.After() != (Head{}) {
			t.Fatal("inconsistent transition produced a head")
		}
	}
}

// The reopening reason that this package admits is exactly the reason the
// amendment kernel accepts as proof of a reopening (B-BR-EMP-214).
func TestReopeningReasonMatchesAmendmentKernel(t *testing.T) {
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	facts := func(reason string) amend.Facts {
		return amend.Facts{RecordID: "7b0e2a54-0000-4000-8000-000000009001", State: amend.StateApproved, Effective: 1, Latest: 1, ReportingDay: "2026-10-31",
			Period: amend.Period{State: amend.PeriodReopened, Reopening: &amend.Reopening{AuthorityProven: true, Reason: reason}}}
	}
	request := amend.Request{Intent: amend.IntentAmend, Expected: 1, Revision: 2, Base: 1, ActorID: "7b0e2a54-0000-4000-8000-000000009002", At: at, Reason: "Synthetic amendment", Authority: amend.Authority{Amend: true}}
	for _, reason := range []string{"", " ", "　", "x", "Synthetic reason", "  padded  ", strings.Repeat("a", 280), strings.Repeat("a", 281), strings.Repeat("é", 280), "tab\there", "nul\x00", "c1\u0085", "bad \xff", "emoji \U0001F600", "nbsp only", " \t ", strings.Repeat(" ", 281), "\x85", strings.Repeat("a", 280) + "\n"} {
		mine := CheckReason(reason)
		_, theirs := amend.Decide(facts(reason), request)
		if (mine == nil) != (theirs == nil) {
			t.Fatalf("reason %q: period %v, amendment %v", reason, mine, theirs)
		}
		if mine != nil && !errors.Is(mine, ErrReasonRequired) && !errors.Is(mine, ErrReasonInvalid) {
			t.Fatal("reason refusal is not a reason code")
		}
		if errors.Is(mine, ErrReasonRequired) != errors.Is(theirs, amend.ErrReasonRequired) {
			t.Fatalf("reason %q required/invalid classification differs", reason)
		}
	}
}
