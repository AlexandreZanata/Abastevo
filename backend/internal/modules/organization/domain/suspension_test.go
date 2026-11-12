package domain

import (
	"testing"
	"time"
)

func TestSuspensionCausePreservesArchivedStateAndCannotReplaceOpenCause(t *testing.T) {
	scope := teamScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	root, e := Rehydrate(scope, Snapshot{ID: scope.OrgID(), Profile: Profile{Name: "Synthetic", TimeZone: "UTC"}, State: Archived, Revision: 4})
	if e != nil {
		t.Fatal(e)
	}
	suspended, e := root.SetSuspension(scope, 4, Security)
	if e != nil {
		t.Fatal(e)
	}
	snap, _ := suspended.Snapshot(scope)
	if snap.State != Archived || snap.Revision != 5 || snap.SuspensionReason != Security {
		t.Fatal("archive silently reopened")
	}
	if _, e := suspended.SetSuspension(scope, 5, Terms); e != ErrTransition {
		t.Fatal("open cause replaced", e)
	}
	if _, e := suspended.ResolveSuspension(scope, 5, Terms); e != ErrConflict {
		t.Fatal("wrong cause resolved", e)
	}
	resolved, e := suspended.ResolveSuspension(scope, 5, Security)
	if e != nil {
		t.Fatal(e)
	}
	snap, _ = resolved.Snapshot(scope)
	if snap.State != Archived || snap.SuspensionReason != "" || snap.Revision != 6 {
		t.Fatal("resolution reopened archive")
	}
}
func TestSuspensionIntakePreservesHistoryWithServerBoundaries(t *testing.T) {
	start := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, c := range []struct {
		name                       string
		state                      State
		declared, received, opened time.Time
		want                       SuspensionDisposition
	}{
		{"before", Active, start.Add(-time.Hour), start.Add(-time.Second), time.Time{}, SuspensionAllow},
		{"grace inclusive", Suspended, start.Add(-time.Second), start.Add(7 * 24 * time.Hour), time.Time{}, SuspensionReview},
		{"late", Suspended, start.Add(-time.Second), start.Add(7*24*time.Hour + time.Nanosecond), time.Time{}, SuspensionRefuse},
		{"new work", Suspended, start, start.Add(time.Minute), time.Time{}, SuspensionRefuse},
		{"actual opened work", Suspended, start.Add(time.Minute), start.Add(30 * 24 * time.Hour), start.Add(-time.Second), SuspensionReview},
		{"reactivated old fact", Active, start.Add(-time.Second), end.Add(time.Minute), time.Time{}, SuspensionReview},
		{"reactivated new fact", Active, end.Add(time.Second), end.Add(time.Minute), time.Time{}, SuspensionAllow},
		{"archive", Archived, start.Add(-time.Second), end.Add(time.Minute), time.Time{}, SuspensionRefuse},
	} {
		t.Run(c.name, func(t *testing.T) {
			period := SuspensionPeriod{StartedAt: start}
			if c.state == Active || c.state == Archived {
				period.EndedAt = end
			}
			periods := []SuspensionPeriod{period}
			if c.received.Before(start) {
				periods = nil
			}
			got, e := AssessSuspension(c.state, periods, SuspensionIntakeFacts{DeclaredAt: c.declared, ReceivedAt: c.received, ServerOpenedAt: c.opened})
			if e != nil || got.Disposition != c.want {
				t.Fatal("intake classification", e, got.Disposition)
			}
		})
	}
}

func TestSuspensionRejectsUnprovenHistoryAndInvalidRootChanges(t *testing.T) {
	scope := teamScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	other := teamScope(t, "7b0e2a54-0000-4000-8000-000000000302")
	root, e := Rehydrate(scope, Snapshot{ID: scope.OrgID(), Profile: Profile{Name: "Synthetic", TimeZone: "UTC"}, State: Active, Revision: 2})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := root.SetSuspension(other, 2, Security); e != ErrScope {
		t.Fatal("foreign scope", e)
	}
	if _, e := root.SetSuspension(scope, 1, Security); e != ErrConflict {
		t.Fatal("stale suspension", e)
	}
	if _, e := root.SetSuspension(scope, 2, "INVALID"); e != ErrInvalid {
		t.Fatal("arbitrary reason", e)
	}
	pending, _ := Rehydrate(scope, Snapshot{ID: scope.OrgID(), Profile: Profile{Name: "Synthetic", TimeZone: "UTC"}, State: PendingSetup, Revision: 1})
	if _, e := pending.SetSuspension(scope, 1, Security); e != ErrTransition {
		t.Fatal("pending root suspended", e)
	}
	closed, _ := Rehydrate(scope, Snapshot{ID: scope.OrgID(), State: Closed, Revision: 3})
	if _, e := closed.SetSuspension(scope, 3, Security); e != ErrClosed {
		t.Fatal("closed reopened", e)
	}
	suspended, e := root.SetSuspension(scope, 2, Security)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := suspended.ResolveSuspension(scope, 3, "INVALID"); e != ErrInvalid {
		t.Fatal("invalid resolution", e)
	}
	if _, e := suspended.ResolveSuspension(other, 3, Security); e != ErrScope {
		t.Fatal("foreign resolution", e)
	}
	active, e := suspended.ResolveSuspension(scope, 3, Security)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := active.Snapshot(scope)
	if v.State != Active || v.SuspensionReason != "" {
		t.Fatal("cause not resolved")
	}
	at := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	facts := SuspensionIntakeFacts{DeclaredAt: at, ReceivedAt: at.Add(time.Hour)}
	for _, c := range []struct {
		state   State
		periods []SuspensionPeriod
		facts   SuspensionIntakeFacts
	}{
		{Suspended, nil, facts}, {Active, []SuspensionPeriod{{StartedAt: at}}, facts},
		{Active, []SuspensionPeriod{{StartedAt: at.Add(2 * time.Hour)}}, facts},
		{Active, []SuspensionPeriod{{StartedAt: at, EndedAt: at.Add(-time.Second)}}, facts},
		{Active, []SuspensionPeriod{{StartedAt: at, EndedAt: at.Add(2 * time.Hour)}}, facts},
		{Active, []SuspensionPeriod{{StartedAt: at, EndedAt: at.Add(time.Minute)}, {StartedAt: at, EndedAt: at.Add(time.Minute)}}, facts},
		{Active, nil, SuspensionIntakeFacts{ReceivedAt: facts.ReceivedAt}},
		{Active, nil, SuspensionIntakeFacts{DeclaredAt: at, ReceivedAt: facts.ReceivedAt, ServerOpenedAt: at.Add(2 * time.Hour)}},
		{Active, make([]SuspensionPeriod, 101), facts}, {"UNKNOWN", nil, facts},
	} {
		if _, e := AssessSuspension(c.state, c.periods, c.facts); e == nil {
			t.Fatal("unproven history/clock admitted")
		}
	}
	for state, code := range map[State]string{PendingSetup: "ORG_SETUP_INCOMPLETE", Closed: "NOT_FOUND", Archived: "ORG_ARCHIVED"} {
		result, e := AssessSuspension(state, nil, facts)
		if e != nil || result.Disposition != SuspensionRefuse || result.Code != code {
			t.Fatal("source state gate", e)
		}
	}
	resolved := []SuspensionPeriod{{StartedAt: at, EndedAt: at.Add(time.Minute)}}
	review, e := AssessSuspension(Active, resolved, SuspensionIntakeFacts{DeclaredAt: at.Add(time.Second), ReceivedAt: at.Add(time.Hour)})
	if e != nil || review.Disposition != SuspensionReview {
		t.Fatal("fact during resolved suspension lost", e)
	}
}
