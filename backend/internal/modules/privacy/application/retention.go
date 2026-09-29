package application

import (
	"context"
	"time"
)

// RetentionBatch bounds every purge pass: each category removes at
// most this many rows per run, oldest first, so one sweep never holds
// long locks or unbounded memory.
const RetentionBatch = 500

// Retention windows (initial minimization choices from
// SECURITY_PRIVACY, pending pilot review):
//
//   - ModerationClosedTTL: closed cases with audit rows age out after
//     12 months; open/triaged cases are never touched.
//   - LedgerHorizon: deletion-ledger rows age out with the backup
//     horizon (35 days: 7 daily + 4 weekly); rows covering no existing
//     backup carry no replay value.
//   - Challenges, idempotency windows and export archives purge on
//     their own expiry (no separate TTL). Rate windows self-clean on
//     check; evidence media purges on its hourly sweeper; the
//     24-month observation horizon reports metrics only until an
//     FK-consistent cascade design lands.
const (
	ModerationClosedTTL = 365 * 24 * time.Hour
	LedgerHorizon       = 35 * 24 * time.Hour
)

// CategoryReport is one category's sweep outcome: rows purged plus the
// oldest overdue instant observed before purging (zero when nothing
// was due). Operators read overdue ages as the backlog metric.
type CategoryReport struct {
	Category      string
	Purged        int64
	OldestOverdue time.Time
}

// RetentionReport is one sweep across every category.
type RetentionReport struct {
	GeneratedAt time.Time
	Categories  []CategoryReport
}

// RetentionPurge is one category's bounded purge pass.
type RetentionPurge func(ctx context.Context) (CategoryReport, error)

// RetentionPorts wires the sweep through the server clock and one
// purge pass per category.
type RetentionPorts struct {
	Clock  func() time.Time
	Purges []NamedPurge
}

// NamedPurge pairs a category name with its purge pass.
type NamedPurge struct {
	Name  string
	Purge RetentionPurge
}

// Retain runs every category purge and reports per-category outcomes.
// Completed categories converge, so a failing category aborts the run
// with prior progress kept: the next tick resumes where it stopped.
// The first error returns after all categories ran, so one stuck
// category cannot hide the others' metrics.
func Retain(ctx context.Context, p RetentionPorts) (RetentionReport, error) {
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	report := RetentionReport{GeneratedAt: now}
	var firstErr error
	for _, np := range p.Purges {
		res, err := np.Purge(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		res.Category = np.Name
		report.Categories = append(report.Categories, res)
	}
	return report, firstErr
}
