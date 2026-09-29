package jobs

import (
	"testing"
	"time"
)

func TestPeriodBuckets(t *testing.T) {
	at := time.Date(2026, 9, 28, 15, 4, 5, 0, time.UTC)
	if got := Period(24*time.Hour, at); got != "2026-09-28" {
		t.Errorf("daily = %q", got)
	}
	if got := Period(time.Hour, at); got != "2026-09-28T15" {
		t.Errorf("hourly = %q", got)
	}
	if got := Period(7*24*time.Hour, at); got != "2026-09-28" {
		t.Errorf("weekly bucket = %q", got)
	}
}
