package jobs

import (
	"context"
	"errors"
	"testing"

	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func observedSweep(run func(context.Context) (evidenceapp.SweepReport, error)) Sweep {
	return Sweep{Run: run, BeginObservation: func(context.Context) (int64, error) { return 1, nil }, FinishObservation: func(context.Context, int64, bool) error { return nil }}
}

func TestSweepHandleRejectsBadPayload(t *testing.T) {
	h := observedSweep(func(context.Context) (evidenceapp.SweepReport, error) {
		return evidenceapp.SweepReport{}, nil
	})
	for name, raw := range map[string]string{
		"malformed": `{`,
		"version":   `{"version":2}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(raw)}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (Sweep{}).Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(`{"version":1}`)}); err == nil {
		t.Error("nil run accepted")
	}
}

func TestSweepHandleRunsAndPropagates(t *testing.T) {
	called := false
	h := observedSweep(func(context.Context) (evidenceapp.SweepReport, error) {
		called = true
		return evidenceapp.SweepReport{ExpiredSessions: 2, QuarantinesDeleted: 2, HealthKnown: true, OverdueKnown: true}, nil
	})
	if err := h.Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(`{"version":1}`)}); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if !called {
		t.Error("sweep did not run")
	}
	boom := errors.New("db down")
	failing := observedSweep(func(context.Context) (evidenceapp.SweepReport, error) {
		return evidenceapp.SweepReport{}, boom
	})
	if err := failing.Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(`{"version":1}`)}); !errors.Is(err, boom) {
		t.Errorf("failure = %v", err)
	}
}

func TestSweepHandleNeverAcknowledgesUnknownPartialOrOverdue(t *testing.T) {
	for name, rep := range map[string]evidenceapp.SweepReport{
		"unknown": {},
		"storage": {HealthKnown: true, OverdueKnown: true, StorageErrors: 1},
		"overdue": {HealthKnown: true, OverdueKnown: true, Overdue: evidenceapp.OverdueReport{Originals: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			h := observedSweep(func(context.Context) (evidenceapp.SweepReport, error) { return rep, nil })
			acknowledged := false
			h.FinishObservation = func(_ context.Context, epoch int64, healthy bool) error {
				acknowledged = true
				if epoch != 1 || healthy {
					t.Fatal("failed sweep recorded healthy", epoch, healthy)
				}
				return nil
			}
			if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); err == nil {
				t.Fatal("unresolved sweep acknowledged")
			}
			if !acknowledged {
				t.Fatal("failed sweep left health unknown without a receipt attempt")
			}
		})
	}
}

func TestSweepObservationFailuresPreventSuccess(t *testing.T) {
	boom := errors.New("synthetic observation outage")
	called := false
	h := observedSweep(func(context.Context) (evidenceapp.SweepReport, error) {
		called = true
		return evidenceapp.SweepReport{HealthKnown: true, OverdueKnown: true}, nil
	})
	h.BeginObservation = func(context.Context) (int64, error) { return 0, boom }
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); !errors.Is(err, boom) || called {
		t.Fatal("start failure ran unobserved work", called, err)
	}
	h.BeginObservation = func(context.Context) (int64, error) { return 2, nil }
	h.FinishObservation = func(_ context.Context, epoch int64, healthy bool) error {
		if epoch != 2 || !healthy {
			t.Fatal(epoch, healthy)
		}
		return boom
	}
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); !errors.Is(err, boom) || !called {
		t.Fatal("receipt failure acknowledged healthy", called, err)
	}
	h.FinishObservation = nil
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); err == nil {
		t.Fatal("missing receipt port accepted")
	}
}

func TestSweepCancelledRunCannotAcknowledgeHealthy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := observedSweep(func(context.Context) (evidenceapp.SweepReport, error) {
		cancel()
		return evidenceapp.SweepReport{HealthKnown: true, OverdueKnown: true}, nil
	})
	acknowledged := false
	h.FinishObservation = func(ack context.Context, epoch int64, healthy bool) error {
		acknowledged = true
		if ack.Err() != nil || healthy || epoch != 1 {
			t.Fatal("cancelled cleanup acknowledged healthy or skipped receipt", ack.Err(), epoch, healthy)
		}
		if _, ok := ack.Deadline(); !ok {
			t.Fatal("receipt is unbounded")
		}
		return nil
	}
	if err := h.Handle(ctx, jobs.Job{Payload: []byte(`{"version":1}`)}); !errors.Is(err, context.Canceled) || !acknowledged {
		t.Fatal(acknowledged, err)
	}
}
