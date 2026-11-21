package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
)

const (
	orgID  = "7b0e2a54-0000-4000-8000-000000000001"
	subsID = "7b0e2a54-0000-4000-8000-000000000002"
)

func scope(t *testing.T, raw string) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSubscriptionLifecycleRequiresMatchingVerifiedEvidence(t *testing.T) {
	s := scope(t, orgID)
	open, err := d.NewSubscription(s, subsID, d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 2})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 21, 11, 0, 0, 0, time.UTC)
	period := d.Period{Start: start, End: start.Add(24 * time.Hour)}
	event := d.LifecycleEvent{
		Scope: s, SubscriptionID: subsID, Kind: d.StartTrial, Source: d.TrialPolicy,
		At: start, Reference: "synthetic-policy-1", Period: period,
	}
	trial, trail, err := open.Apply(event)
	if err != nil || trial.Snapshot().State != d.Trialing || trail.Revision != 1 ||
		trail.Source != d.TrialPolicy || trail.At != start || trail.Reference != event.Reference {
		t.Fatalf("trial provenance: %+v %v", trail, err)
	}
	if _, _, err := trial.Apply(event); !errors.Is(err, d.ErrStaleSubscription) {
		t.Fatalf("trial replay applied twice: %v", err)
	}
	settled := d.LifecycleEvent{
		Scope: s, SubscriptionID: subsID, ExpectedRevision: 1, Kind: d.ConfirmSettlement,
		Source: d.VerifiedBilling, At: start.Add(time.Hour), Reference: "synthetic-settlement-1", Period: period,
	}
	for _, alter := range []func(*d.LifecycleEvent){
		func(e *d.LifecycleEvent) { e.Source = d.TrialPolicy },
		func(e *d.LifecycleEvent) { e.Scope = scope(t, "7b0e2a54-0000-4000-8000-000000000099") },
		func(e *d.LifecycleEvent) { e.At = start.Add(-time.Second) },
		func(e *d.LifecycleEvent) { e.Reference = "" },
		func(e *d.LifecycleEvent) { e.Period.End = e.Period.Start },
	} {
		invalid := settled
		alter(&invalid)
		if _, _, err := trial.Apply(invalid); err == nil {
			t.Fatal("unbound or invalid settlement changed subscription state")
		}
	}
	active, trail, err := trial.Apply(settled)
	if err != nil || trail.From != d.Trialing || trail.To != d.Active || active.Snapshot().Period != period {
		t.Fatalf("verified settlement did not activate exact period: %+v %v", trail, err)
	}
	overdue, _, err := active.Apply(d.LifecycleEvent{
		Scope: s, SubscriptionID: subsID, ExpectedRevision: 2, Kind: d.ReportPastDue, Source: d.VerifiedBilling,
		At: start.Add(2 * time.Hour), Reference: "synthetic-overdue-1",
	})
	if err != nil || overdue.Snapshot().State != d.PastDue {
		t.Fatalf("overdue invoice: %v", err)
	}
	settled.ExpectedRevision, settled.At, settled.Reference = 3, start.Add(3*time.Hour), "synthetic-settlement-2"
	recovered, trail, err := overdue.Apply(settled)
	if err != nil || trail.From != d.PastDue || trail.To != d.Active || recovered.Snapshot().Revision != 4 {
		t.Fatalf("verified settlement did not restore overdue subscription: %+v %v", trail, err)
	}
}

func TestSubscriptionSuspensionRestoresRecordedStateAndCancellationIsTerminal(t *testing.T) {
	s := scope(t, orgID)
	sub, err := d.NewSubscription(s, subsID, d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 21, 11, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	event := d.LifecycleEvent{Scope: s, SubscriptionID: subsID, Reference: "synthetic-reviewed-record"}
	apply := func(kind d.EventKind, source d.EvidenceSource, at time.Time, period d.Period) d.Transition {
		t.Helper()
		event.Kind, event.Source, event.At, event.Period = kind, source, at, period
		event.ExpectedRevision = sub.Snapshot().Revision
		next, transition, err := sub.Apply(event)
		if err != nil {
			t.Fatal(err)
		}
		sub = next
		return transition
	}
	apply(d.ConfirmSettlement, d.VerifiedBilling, start, d.Period{Start: start, End: end})
	apply(d.ReportPastDue, d.VerifiedBilling, start.Add(time.Hour), d.Period{})
	apply(d.SuspendAccess, d.ReviewedOperator, start.Add(2*time.Hour), d.Period{})
	if sub.Snapshot().State != d.Suspended {
		t.Fatal("reviewed suspension did not apply")
	}
	if restored := apply(d.RestoreAccess, d.ReviewedOperator, start.Add(3*time.Hour), d.Period{}); restored.To != d.PastDue {
		t.Fatal("suspension restoration inferred payment")
	}
	apply(d.ScheduleCancellation, d.VerifiedBilling, start.Add(4*time.Hour), d.Period{})
	if sub.Snapshot().State != d.Cancelling || sub.Snapshot().PriorState != d.PastDue {
		t.Fatal("pending cancellation lost its previous state")
	}
	event.Kind, event.Source, event.At, event.ExpectedRevision = d.FinishTerm, d.ServerClock, end.Add(-time.Second), sub.Snapshot().Revision
	if _, _, err := sub.Apply(event); !errors.Is(err, d.ErrInvalidTransition) {
		t.Fatalf("clock ended the term early: %v", err)
	}
	apply(d.FinishTerm, d.ServerClock, end, d.Period{})
	event.Kind, event.Source, event.At, event.ExpectedRevision = d.ConfirmSettlement, d.VerifiedBilling, end, sub.Snapshot().Revision
	event.Period = d.Period{Start: end, End: end.Add(24 * time.Hour)}
	if _, _, err := sub.Apply(event); !errors.Is(err, d.ErrInvalidTransition) {
		t.Fatalf("verified late payment reopened terminal subscription: %v", err)
	}
}
