package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type cleanupStub struct {
	before time.Time
	limit  int
	err    error
}

func (s *cleanupStub) DeleteExpiredReportSnapshots(_ context.Context, before time.Time, limit int) (int64, error) {
	s.before, s.limit = before, limit
	return 1, s.err
}

func TestReportCleanupUsesBoundedRetention(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	stub := &cleanupStub{}
	h := ReportCleanup{Store: stub, Retention: 24 * time.Hour, Limit: 3, Now: func() time.Time { return now }}
	if h.Kind() != ReportCleanupKind || h.Version() != 1 {
		t.Fatal("kind/version")
	}
	if e := h.Handle(context.Background(), jobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if !stub.before.Equal(now.Add(-24*time.Hour)) || stub.limit != 3 {
		t.Fatal(stub.before, stub.limit)
	}
	if e := (ReportCleanup{Store: stub, Retention: 0, Limit: 3}).Handle(context.Background(), jobs.Job{}); e == nil {
		t.Fatal("invalid retention accepted")
	}
	stub.err = errors.New("outage")
	if e := h.Handle(context.Background(), jobs.Job{}); !errors.Is(e, stub.err) {
		t.Fatal("outage hidden", e)
	}
}
