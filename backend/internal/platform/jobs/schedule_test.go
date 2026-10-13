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
	for _, tc := range []struct {
		interval time.Duration
		want     string
	}{
		{time.Minute, "2026-09-28T15:04:00Z"},
		{30 * time.Second, "2026-09-28T15:04:00Z"},
		{90 * time.Minute, "2026-09-28T15:00:00Z"},
		{0, ""},
		{-time.Minute, ""},
	} {
		if got := Period(tc.interval, at); got != tc.want {
			t.Errorf("interval %v = %q, want %q", tc.interval, got, tc.want)
		}
	}
	if Period(7*24*time.Hour, at) != Period(7*24*time.Hour, at.Add(24*time.Hour)) || Period(7*24*time.Hour, at) == Period(7*24*time.Hour, at.Add(7*24*time.Hour)) {
		t.Error("weekly interval was reduced to a day")
	}
	local := at.In(time.FixedZone("synthetic", -4*60*60))
	for _, interval := range []time.Duration{24 * time.Hour, time.Hour, time.Minute} {
		if Period(interval, local) != Period(interval, at) {
			t.Error("host timezone forked schedule identity", interval)
		}
	}
}
