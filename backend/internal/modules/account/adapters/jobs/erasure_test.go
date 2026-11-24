package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestErasureJobAttemptsEveryCleanupDespiteFailures(t *testing.T) {
	steps := []struct {
		name string
		err  error
	}{
		{name: "erasure", err: errors.New("erasure failure")},
		{name: "verifiers", err: errors.New("code cleanup failure")},
		{name: "audit", err: errors.New("audit cleanup failure")},
	}
	for _, failed := range []int{-1, 0, 1, 2, 3} {
		var attempted [3]int
		run := func(i int) func(context.Context) error {
			return func(context.Context) error {
				attempted[i]++
				if failed == i || failed == len(steps) {
					return steps[i].err
				}
				return nil
			}
		}
		handler := Erasure{
			Reconcile: run(0), PurgeEmailCodes: run(1), PurgeRecoveryAudit: run(2),
		}
		err := handler.Handle(t.Context(), jobs.Job{Payload: []byte(`{"version":1}`)})
		for i, step := range steps {
			if attempted[i] != 1 {
				t.Errorf("run with failure %d: %s attempted %d times", failed, step.name, attempted[i])
			}
			if got, want := errors.Is(err, step.err), failed == i || failed == len(steps); got != want {
				t.Errorf("run with failure %d: %s error = %v, expected %v", failed, step.name, err, want)
			}
		}
		if failed == -1 && err != nil {
			t.Errorf("successful cleanup returned %v", err)
		}
	}
}
