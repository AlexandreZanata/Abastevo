package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC03-D monitoring and reconciliation (AUD-02/04): the sweep embeds
// intake posture (reserved/accepted/rejected declared buckets plus
// oldest incomplete) without secrets; failed (REJECTED) originals
// reconcile like every terminal copy; a health-query failure degrades
// to explicit UNKNOWN and an error after preceding cleanup.

func TestSweepEmbedsIntakeHealth(t *testing.T) {
	oldest := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	store := &fakeSweepStore{health: IntakeHealth{
		ReservedSessions: 3, ReservedBytes: 3 << 20,
		AcceptedToday: 5, AcceptedBytes: 5 << 20,
		RejectedToday: 1, RejectedBytes: 1 << 20,
		OldestIncomplete: oldest,
	}}
	rep, err := Sweep(context.Background(), sweepDeps(store))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Health.ReservedSessions != 3 || rep.Health.ReservedBytes != 3<<20 {
		t.Errorf("reserved = %+v", rep.Health)
	}
	if rep.Health.AcceptedToday != 5 || rep.Health.RejectedToday != 1 {
		t.Errorf("decided = %+v", rep.Health)
	}
	if !rep.Health.OldestIncomplete.Equal(oldest) {
		t.Errorf("oldest = %v, want %v", rep.Health.OldestIncomplete, oldest)
	}
}

func TestSweepHealthFailureReportsUnknownAndError(t *testing.T) {
	store := &fakeSweepStore{healthErr: context.DeadlineExceeded}
	rep, err := Sweep(context.Background(), sweepDeps(store))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("health failure was acknowledged: %v", err)
	}
	if rep.HealthKnown || !rep.OverdueKnown || rep.Health != (IntakeHealth{}) {
		t.Errorf("health = %+v, want zero snapshot", rep.Health)
	}
}

func TestSweepReconcilesRejectedOriginals(t *testing.T) {
	// A REJECTED session's quarantine bytes delete and mark exactly
	// like any terminal copy: failed objects leave nothing behind.
	store := &fakeSweepStore{
		cands: []domain.SessionRef{{ID: "s-rejected", QuarantineKey: "q/rejected"}},
	}
	deps := sweepDeps(store)
	storage, ok := deps.Storage.(*fakeSweepStorage)
	if !ok {
		t.Fatal("deps storage shape changed")
	}
	storage.keys["q/rejected"] = true
	rep, err := Sweep(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if rep.QuarantinesDeleted != 1 {
		t.Fatalf("quarantines deleted = %d, want 1", rep.QuarantinesDeleted)
	}
	if len(store.qdel) != 1 || store.qdel[0] != "s-rejected" {
		t.Errorf("marked = %v, want [s-rejected]", store.qdel)
	}
	if storage.keys["q/rejected"] {
		t.Error("rejected bytes still present after sweep")
	}
}

func TestSweepAuditFailureCannotReportHealthySuccess(t *testing.T) {
	store := &auditFailedSweep{fakeSweepStore: &fakeSweepStore{idle: []domain.SessionRef{{ID: "s1", QuarantineKey: "q/a"}}}}
	deps := sweepDeps(store.fakeSweepStore)
	deps.Store = store
	rep, err := Sweep(context.Background(), deps)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("audit outage reported successful healthy zero", rep, err)
	}
	if rep.QuarantinesDeleted != 1 || rep.OverdueKnown || !rep.HealthKnown {
		t.Fatal("audit failure lost partial cleanup or UNKNOWN", rep)
	}
}

type auditFailedSweep struct{ *fakeSweepStore }

func (*auditFailedSweep) OverdueFinals(context.Context, time.Time, int) ([]domain.ObjectRef, error) {
	return nil, context.DeadlineExceeded
}
