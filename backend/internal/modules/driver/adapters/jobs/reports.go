package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const ReportCleanupKind = "driver-report-snapshot-cleanup"

type ReportCleanupStore interface {
	DeleteExpiredReportSnapshots(context.Context, time.Time, int) (int64, error)
}

type ReportCleanup struct {
	Store     ReportCleanupStore
	Retention time.Duration
	Limit     int
	Now       func() time.Time
}

func (ReportCleanup) Kind() string { return ReportCleanupKind }
func (ReportCleanup) Version() int { return 1 }
func (h ReportCleanup) Handle(ctx context.Context, _ jobs.Job) error {
	if h.Store == nil || h.Retention <= 0 || h.Limit < 1 || h.Limit > 1000 {
		return errors.New("driver: invalid report cleanup job")
	}
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}
	_, e := h.Store.DeleteExpiredReportSnapshots(ctx, now.Add(-h.Retention), h.Limit)
	return e
}
