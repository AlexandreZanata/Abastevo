package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC05-B deadline-aware catch-up and alarms (AUD-06, B-BR-M04):
// bounded batches converge a growing backlog run over run (measured
// throughput, not an assumed drain); the post-pass overdue signal
// names the remaining backlog per namespace for the next run and
// its alarm.

func TestSweepCatchUpConvergesGrowingBacklog(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := &fakeSweepStore{}
	keys := map[string]bool{}
	for i := 0; i < 250; i++ {
		key := fmt.Sprintf("f/backlog-%d", i)
		store.finals = append(store.finals, domain.ObjectRef{
			ID: key, FinalKey: key,
			CreatedAt:  now.Add(-25 * time.Hour),
			ReceivedAt: now.Add(-25 * time.Hour),
		})
		keys[key] = true
	}
	deps := sweepDeps(store)
	deps.Storage = &fakeSweepStorage{keys: keys, fail: map[string]bool{}}
	deleted := 0
	runs := (250 + FinalCleanupBatch - 1) / FinalCleanupBatch
 for run := 0; run < runs; run++ {
		rep, err := Sweep(context.Background(), deps)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if rep.FinalsDeleted > FinalCleanupBatch { t.Fatalf("unbounded final pass: %d",rep.FinalsDeleted) }
 deleted += rep.FinalsDeleted
		if run < runs-1 && rep.Overdue.Count == 0 {
			t.Fatalf("run %d must still report backlog, got %+v", run, rep.Overdue)
		}
	}
	if deleted != 250 {
		t.Fatalf("catch-up deleted = %d, want 250 in bounded runs", deleted)
	}
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if rep.FinalsDeleted != 0 || rep.Overdue.Unhealthy() {
		t.Fatalf("converged pass = %+v, want quiet zero", rep)
	}
}

func TestSweepOverdueSignalNamesNamespaces(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	oldFinal := now.Add(-30 * time.Hour)
	store := &fakeSweepStore{
		overdue: []domain.ObjectRef{{
			ID: "o-old", FinalKey: "f/old",
			CreatedAt:  oldFinal,
			ReceivedAt: oldFinal,
		}},
		overdueOriginals: []domain.SessionRef{{
			ID: "s-old", QuarantineKey: "q/old",
			Status: domain.StateIssued, CreatedAt: now.Add(-50 * time.Hour),
		}},
	}
	deps := sweepDeps(store)
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Overdue.Count != 1 || !rep.Overdue.Oldest.Equal(oldFinal) {
		t.Errorf("finals signal = %+v, want 1 at %v", rep.Overdue, oldFinal)
	}
	if rep.Overdue.Originals != 1 {
		t.Errorf("originals signal = %+v, want 1", rep.Overdue)
	}
	if !rep.Overdue.Unhealthy() {
		t.Error("backlog pass must alarm")
	}
}
