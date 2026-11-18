package domain

import (
	"testing"
	"time"
)

func TestRetentionWindowValidationAndExpiry(t *testing.T) {
	for _, invalid := range []int{0, 29, 366, -10} {
		if _, err := RawTrackExpired(nil, invalid, time.Now()); err == nil {
			t.Fatalf("invalid retention %d accepted", invalid)
		}
	}
	ended := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	if expired, err := RawTrackExpired(nil, DefaultRawRetentionDays, ended.Add(400*24*time.Hour)); err != nil || expired {
		t.Fatalf("open trip should not expire: %v %v", expired, err)
	}
	if expired, err := RawTrackExpired(&ended, 30, ended.Add(29*24*time.Hour)); err != nil || expired {
		t.Fatalf("29d < 30d window should not expire: %v %v", expired, err)
	}
	if expired, err := RawTrackExpired(&ended, 30, ended.Add(30*24*time.Hour)); err != nil || !expired {
		t.Fatalf("30d >= 30d window should expire: %v %v", expired, err)
	}
	if expired, err := RawTrackExpired(&ended, DefaultRawRetentionDays, ended.Add(90*24*time.Hour)); err != nil || !expired {
		t.Fatalf("90d default should expire: %v %v", expired, err)
	}
}

func TestTrimFixesAfterEnd(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	fixes := []TrackFix{
		{At: t0, Lat: 100, Lon: 100, Accuracy: 5},
		{At: t0.Add(5 * time.Minute), Lat: 200, Lon: 200, Accuracy: 5},
		{At: t0.Add(10 * time.Minute), Lat: 300, Lon: 300, Accuracy: 5},
	}
	end := t0.Add(5 * time.Minute)
	kept, trimmed := TrimFixesAfterEnd(fixes, &end)
	if trimmed != 1 || len(kept) != 2 || !kept[1].At.Equal(end) {
		t.Fatalf("unexpected trim result: kept=%d trimmed=%d", len(kept), trimmed)
	}
}
