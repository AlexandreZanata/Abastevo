//go:build integration

package jobs

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMinuteSchedulesConvergeAndResumeEachMinute(t *testing.T) {
	_, pool := freshQueue(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 12, 10, 1, 1, 0, time.UTC)
	var schedules []Schedule
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep"} {
		schedules = append(schedules, Schedule{Name: kind + "-every-minute", Kind: kind, Version: 1, Enabled: true, Interval: time.Minute, Build: func(string) map[string]any { return map[string]any{"version": 1} }})
	}
	for minute := 0; minute < 3; minute++ {
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s := NewScheduler(pool, schedules) // Separate process/restart view.
				s.now = func() time.Time { return at.Add(time.Duration(minute) * time.Minute) }
				if n, err := s.Tick(ctx); err != nil || n != len(schedules) {
					t.Error(n, err)
				}
			}()
		}
		wg.Wait()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_queue`).Scan(&count); err != nil || count != (minute+1)*len(schedules) {
			t.Fatalf("minute %d created %d jobs, want %d: %v", minute, count, (minute+1)*len(schedules), err)
		}
	}
	var malformed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_queue WHERE payload->>'version' != '1' OR status != 'queued'`).Scan(&malformed); err != nil || malformed != 0 {
		t.Fatal("invalid schedule payload/state", malformed, err)
	}
}
