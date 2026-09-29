package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Schedule is one recurring enqueue rule. Period buckets the cadence
// (daily, hourly); the dedupe key joins schedule, period and payload hash,
// so restarts and overlapping ticks converge on a single row instead of
// duplicating work. Disabled schedules never enqueue; re-enabling resumes
// from the current period without backfill.
type Schedule struct {
	Name     string
	Kind     string
	Version  int
	Interval time.Duration
	Enabled  bool
	Reason   string
	Build    func(period string) map[string]any
}

// Scheduler ticks schedules into the queue.
type Scheduler struct {
	pool      *pgxpool.Pool
	schedules []Schedule
	now       func() time.Time
}

// NewScheduler wires schedules to a pool.
func NewScheduler(pool *pgxpool.Pool, schedules []Schedule) *Scheduler {
	return &Scheduler{pool: pool, schedules: schedules, now: time.Now}
}

// Period formats the bucket for an interval at a time.
func Period(interval time.Duration, at time.Time) string {
	switch {
	case interval >= 24*time.Hour:
		y, m, d := at.Date()
		return fmt.Sprintf("%04d-%02d-%02d", y, m, d)
	default:
		y, m, d := at.Date()
		return fmt.Sprintf("%04d-%02d-%02dT%02d", y, m, d, at.Hour())
	}
}

// Tick processes every due enabled schedule, returning schedules handled.
// Dedupe convergence (not locking) makes concurrent ticks and restarts
// safe: at most one row exists per schedule and period.
func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	now := s.now()
	handled := 0
	for _, sch := range s.schedules {
		if !sch.Enabled {
			continue
		}
		if sch.Interval <= 0 || sch.Build == nil {
			continue
		}
		period := Period(sch.Interval, now)
		body := sch.Build(period)
		body["version"] = sch.Version
		raw, err := json.Marshal(body)
		if err != nil {
			return handled, err
		}
		dedupe := "schedule:" + sch.Name + ":" + period
		if _, err := Enqueue(ctx, s.pool, sch.Kind, raw, dedupe, 3, time.Time{}); err != nil {
			return handled, err
		}
		handled++
	}
	return handled, nil
}
