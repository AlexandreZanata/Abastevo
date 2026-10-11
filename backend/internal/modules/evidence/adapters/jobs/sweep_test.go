package jobs

import (
	"context"
	"errors"
	"testing"

	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestSweepHandleRejectsBadPayload(t *testing.T) {
	h := Sweep{Run: func(context.Context) (evidenceapp.SweepReport, error) {
		return evidenceapp.SweepReport{}, nil
	}}
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
	h := Sweep{Run: func(context.Context) (evidenceapp.SweepReport, error) {
		called = true
		return evidenceapp.SweepReport{ExpiredSessions: 2, QuarantinesDeleted: 2, HealthKnown: true, OverdueKnown: true}, nil
	}}
	if err := h.Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(`{"version":1}`)}); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if !called {
		t.Error("sweep did not run")
	}
	boom := errors.New("db down")
	failing := Sweep{Run: func(context.Context) (evidenceapp.SweepReport, error) {
		return evidenceapp.SweepReport{}, boom
	}}
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
			h := Sweep{Run: func(context.Context) (evidenceapp.SweepReport, error) { return rep, nil }}
			if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); err == nil {
				t.Fatal("unresolved sweep acknowledged")
			}
		})
	}
}
