package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

type boundedSweepStorage struct {
	t          *testing.T
	q, f       int
	failSource bool
}

func (s *boundedSweepStorage) bounded(ctx context.Context) {
	s.t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("copy lacks 5s bound")
	}
}
func (s *boundedSweepStorage) DeleteQuarantine(ctx context.Context, _ string) error {
	s.bounded(ctx)
	s.q++
	if s.failSource {
		return context.DeadlineExceeded
	}
	return nil
}
func (s *boundedSweepStorage) DeleteFinal(ctx context.Context, _ string) error {
	s.bounded(ctx)
	s.f++
	return nil
}

func TestSweepHardBatchAndCopyBounds(t *testing.T) {
	store := &fakeSweepStore{}
	deps := sweepDeps(store)
	now := deps.Clock()
	for i := 0; i < 100; i++ {
		store.cands = append(store.cands, ref(fmt.Sprint(i), fmt.Sprint("q/", i)))
		store.finals = append(store.finals, domain.ObjectRef{ID: fmt.Sprint(i), FinalKey: fmt.Sprint("f/", i), ReceivedAt: now.Add(-23 * time.Hour)})
	}
	storage := &boundedSweepStorage{t: t}
	deps.Storage = storage
	deps.Batch = 1000000
	rep, err := Sweep(context.Background(), deps)
	if err != nil || rep.QuarantinesDeleted != 32 || rep.FinalsDeleted != 32 || storage.q != 32 || storage.f != 32 {
		t.Fatal(rep, err, storage)
	}
}

func TestSweepOriginalFailureCannotPreventFinalDisposal(t *testing.T) {
	store := &fakeSweepStore{cands: []domain.SessionRef{ref("source", "q/a")}}
	deps := sweepDeps(store)
	store.finals = []domain.ObjectRef{{ID: "final", FinalKey: "f/a", ReceivedAt: deps.Clock().Add(-23 * time.Hour)}}
	storage := &boundedSweepStorage{t: t, failSource: true}
	deps.Storage = storage
	rep, err := Sweep(context.Background(), deps)
	if !errors.Is(err, context.DeadlineExceeded) || rep.StorageErrors != 1 || rep.FinalsDeleted != 1 || storage.f != 1 || !rep.HealthKnown || !rep.OverdueKnown {
		t.Fatal(rep, err)
	}
}

type deadlineObservedStore struct {
	*fakeSweepStore
	t *testing.T
}

func (s deadlineObservedStore) ExpireIdleSessions(ctx context.Context, at time.Time, batch int) ([]domain.SessionRef, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 20*time.Second {
		s.t.Fatal("source slice unbounded")
	}
	return s.fakeSweepStore.ExpireIdleSessions(ctx, at, batch)
}
func (s deadlineObservedStore) ReconcileFinalOutputs(ctx context.Context, _ time.Time, _ int, _ func(context.Context, string) error) (FinalCleanupReport, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 20*time.Second {
		s.t.Fatal("final slice unbounded")
	}
	return FinalCleanupReport{}, nil
}
func (s deadlineObservedStore) PurgeObjectsBefore(ctx context.Context, _ time.Time) (int64, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 45*time.Second {
		s.t.Fatal("whole pass unbounded")
	}
	return 0, ctx.Err()
}
func TestSweepSlicesAndCancellationAreBounded(t *testing.T) {
	store := &fakeSweepStore{}
	deps := sweepDeps(store)
	deps.Store = deadlineObservedStore{store, t}
	if _, err := Sweep(context.Background(), deps); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Sweep(ctx, deps); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled pass acknowledged", err)
	}
}

func TestFinalCleanupMarginDoesNotRedefineOverdue(t *testing.T) {
	anchor := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	obj := domain.ObjectRef{ReceivedAt: anchor}
	if FinalCleanupDue(obj, anchor.Add(22*time.Hour-time.Nanosecond)) || !FinalCleanupDue(obj, anchor.Add(22*time.Hour)) || FinalDue(obj, anchor.Add(23*time.Hour)) || !FinalDue(obj, anchor.Add(24*time.Hour)) {
		t.Fatal("cleanup reserve changed the hard retention deadline")
	}
}
