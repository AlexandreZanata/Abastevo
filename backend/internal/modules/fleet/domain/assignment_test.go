package domain_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

var assignmentStart = time.Date(2026, 5, 4, 11, 0, 0, 0, time.UTC)

func assignmentPeriod(t *testing.T, from time.Time, to *time.Time) fleet.AssignmentPeriod {
	t.Helper()
	p, err := fleet.NewAssignmentPeriod(from, to)
	if err != nil {
		t.Fatal("valid assignment period refused", from, to, err)
	}
	return p
}

func TestAssignmentKindsAreTheClosedVocabulary(t *testing.T) {
	for _, kind := range []fleet.AssignmentKind{fleet.PrimaryAssignment, fleet.AdditionalAssignment, fleet.TemporaryAssignment} {
		got, err := fleet.ParseAssignmentKind(string(kind))
		if err != nil || got != kind {
			t.Fatal("known kind refused", kind, err)
		}
	}
	for _, raw := range []string{"", "primary", "SUBSTITUTE", "POOL", " PRIMARY", "PRIMARY "} {
		if _, err := fleet.ParseAssignmentKind(raw); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("unknown kind accepted", raw, err)
		}
	}
}

// B-BR-EMP-243 / AC1: SCHEDULED before the start, ACTIVE inside the
// half-open period, ENDED from the end on; an empty period is always ENDED.
func TestAssignmentStateIsDerivedFromServerTimeWithHalfOpenPeriods(t *testing.T) {
	end := assignmentStart.Add(2 * time.Hour)
	closed := assignmentPeriod(t, assignmentStart, &end)
	open := assignmentPeriod(t, assignmentStart, nil)
	empty := assignmentPeriod(t, assignmentStart, &assignmentStart)
	for _, tc := range []struct {
		name   string
		period fleet.AssignmentPeriod
		at     time.Time
		want   fleet.AssignmentState
		in     bool
	}{
		{"closed before start", closed, assignmentStart.Add(-time.Nanosecond), fleet.AssignmentScheduled, false},
		{"closed at start", closed, assignmentStart, fleet.AssignmentActive, true},
		{"closed inside", closed, assignmentStart.Add(time.Hour), fleet.AssignmentActive, true},
		{"closed last nanosecond", closed, end.Add(-time.Nanosecond), fleet.AssignmentActive, true},
		{"closed at end", closed, end, fleet.AssignmentEnded, false},
		{"closed after end", closed, end.Add(time.Hour), fleet.AssignmentEnded, false},
		{"open before start", open, assignmentStart.Add(-time.Microsecond), fleet.AssignmentScheduled, false},
		{"open at start", open, assignmentStart, fleet.AssignmentActive, true},
		{"open far later", open, assignmentStart.AddDate(30, 0, 0), fleet.AssignmentActive, true},
		{"empty before start", empty, assignmentStart.Add(-time.Hour), fleet.AssignmentEnded, false},
		{"empty at start", empty, assignmentStart, fleet.AssignmentEnded, false},
		{"empty later", empty, assignmentStart.Add(time.Hour), fleet.AssignmentEnded, false},
		{"other zone same instant", closed, assignmentStart.In(time.FixedZone("BRT", -3*3600)), fleet.AssignmentActive, true},
	} {
		if got := tc.period.StateAt(tc.at); got != tc.want {
			t.Fatal(tc.name, "state", got, "want", tc.want)
		}
		if got := tc.period.Contains(tc.at); got != tc.in {
			t.Fatal(tc.name, "contains", got)
		}
	}
	if !empty.Empty() || closed.Empty() || open.Empty() {
		t.Fatal("empty period detection")
	}
}

func TestAssignmentPeriodIsExactAndNonAliasing(t *testing.T) {
	end := assignmentStart.Add(time.Hour)
	p := assignmentPeriod(t, assignmentStart.In(time.FixedZone("BRT", -3*3600)), &end)
	end = end.Add(time.Hour)
	if !p.To.Equal(assignmentStart.Add(time.Hour)) || p.From.Location() != time.UTC || p.To.Location() != time.UTC {
		t.Fatal("period aliases the caller's end or keeps a zone", p)
	}
	before := assignmentStart.Add(-time.Microsecond)
	for name, tc := range map[string]struct {
		from time.Time
		to   *time.Time
	}{
		"zero start":          {time.Time{}, nil},
		"sub-microsecond":     {assignmentStart.Add(time.Nanosecond), nil},
		"end sub-microsecond": {assignmentStart, func() *time.Time { v := assignmentStart.Add(time.Hour + time.Nanosecond); return &v }()},
		"end before start":    {assignmentStart, &before},
		"year 10000":          {time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), nil},
		"zero end":            {assignmentStart, &time.Time{}},
	} {
		if _, err := fleet.NewAssignmentPeriod(tc.from, tc.to); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid period accepted", name, err)
		}
	}
}

// §6.3 reversals: ending an ACTIVE period cuts it at the instant's
// microsecond floor; ending a SCHEDULED one leaves an empty period; an ENDED
// period refuses with ErrAssignmentEnded (B-BR-EMP-244).
func TestAssignmentEndShortensOnceAndRefusesEnded(t *testing.T) {
	end := assignmentStart.Add(3 * time.Hour)
	temporary := assignmentPeriod(t, assignmentStart, &end)
	open := assignmentPeriod(t, assignmentStart, nil)
	inside := assignmentStart.Add(time.Hour + 1500*time.Nanosecond)
	for name, tc := range map[string]struct {
		period fleet.AssignmentPeriod
		at     time.Time
		want   time.Time
	}{
		"active open":      {open, inside, assignmentStart.Add(time.Hour + time.Microsecond)},
		"active temporary": {temporary, inside, assignmentStart.Add(time.Hour + time.Microsecond)},
		"at start":         {open, assignmentStart, assignmentStart},
		"scheduled":        {open, assignmentStart.Add(-time.Hour), assignmentStart},
		"scheduled closed": {temporary, assignmentStart.Add(-time.Nanosecond), assignmentStart},
		"last nanosecond":  {temporary, end.Add(-time.Nanosecond), end.Add(-time.Microsecond)},
	} {
		got, err := tc.period.EndAt(tc.at)
		if err != nil || got.To == nil || !got.To.Equal(tc.want) || !got.From.Equal(tc.period.From) || got.StateAt(tc.at) != fleet.AssignmentEnded {
			t.Fatal("end", name, got, err)
		}
		if _, err := got.EndAt(tc.at); !errors.Is(err, fleet.ErrAssignmentEnded) {
			t.Fatal("second end accepted", name, err)
		}
	}
	for name, at := range map[string]time.Time{"at end": end, "after end": end.Add(time.Hour)} {
		if _, err := temporary.EndAt(at); !errors.Is(err, fleet.ErrAssignmentEnded) {
			t.Fatal("ended period ended again", name, err)
		}
	}
	if _, err := open.EndAt(time.Time{}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("zero end instant accepted", err)
	}
}

// AC3 and B-BR-EMP-243: TEMPORARY requires an end, a new period is non-empty
// and never starts before the server instant.
func TestNewAssignmentShapeAndTemporaryEnd(t *testing.T) {
	now := assignmentStart
	later := now.Add(48 * time.Hour)
	for _, kind := range []fleet.AssignmentKind{fleet.PrimaryAssignment, fleet.AdditionalAssignment, fleet.TemporaryAssignment} {
		if err := fleet.ValidNewAssignment(kind, assignmentPeriod(t, now, &later), now); err != nil {
			t.Fatal("bounded new assignment refused", kind, err)
		}
		if err := fleet.ValidNewAssignment(kind, assignmentPeriod(t, now.Add(time.Hour), &later), now); err != nil {
			t.Fatal("scheduled new assignment refused", kind, err)
		}
	}
	for _, kind := range []fleet.AssignmentKind{fleet.PrimaryAssignment, fleet.AdditionalAssignment} {
		if err := fleet.ValidNewAssignment(kind, assignmentPeriod(t, now, nil), now); err != nil {
			t.Fatal("open-ended new assignment refused", kind, err)
		}
	}
	for name, tc := range map[string]struct {
		kind   fleet.AssignmentKind
		period fleet.AssignmentPeriod
	}{
		"temporary without end": {fleet.TemporaryAssignment, assignmentPeriod(t, now, nil)},
		"past start":            {fleet.AdditionalAssignment, assignmentPeriod(t, now.Add(-time.Microsecond), &later)},
		"empty period":          {fleet.PrimaryAssignment, assignmentPeriod(t, now, &now)},
		"unknown kind":          {"POOL", assignmentPeriod(t, now, &later)},
	} {
		if err := fleet.ValidNewAssignment(tc.kind, tc.period, now); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid new assignment accepted", name, err)
		}
	}
	if err := fleet.ValidNewAssignment(fleet.PrimaryAssignment, fleet.AssignmentPeriod{From: now.Add(time.Nanosecond)}, now); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("inexact period accepted", err)
	}
}

// AC4 / BR-FLEET-018: one assignment per employee; a PRIMARY names one.
func TestAssigneesAreBoundedDistinctAndPrimaryNamesOne(t *testing.T) {
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", i+1)
		}
		return out
	}
	for _, kind := range []fleet.AssignmentKind{fleet.AdditionalAssignment, fleet.TemporaryAssignment} {
		for _, n := range []int{1, 2, fleet.MaxAssignees} {
			if err := fleet.ValidAssignees(kind, ids(n)); err != nil {
				t.Fatal("bounded assignee list refused", kind, n, err)
			}
		}
		for _, bad := range [][]string{nil, {}, ids(fleet.MaxAssignees + 1), append(ids(2), ids(1)...)} {
			if err := fleet.ValidAssignees(kind, bad); !errors.Is(err, fleet.ErrInvalid) {
				t.Fatal("invalid assignee list accepted", kind, len(bad), err)
			}
		}
	}
	if err := fleet.ValidAssignees(fleet.PrimaryAssignment, ids(1)); err != nil {
		t.Fatal("single primary refused", err)
	}
	if err := fleet.ValidAssignees(fleet.PrimaryAssignment, ids(2)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("two primaries in one command accepted", err)
	}
	if err := fleet.ValidAssignees("POOL", ids(1)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unknown kind accepted", err)
	}
	if fleet.MaxAssignees != 100 {
		t.Fatal("bulk bound drift")
	}
}
