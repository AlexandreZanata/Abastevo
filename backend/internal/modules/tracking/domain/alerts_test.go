package domain

import (
	"testing"
	"time"
)

func TestEvaluateStaleTracker(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	if _, err := EvaluateStaleTracker("t", "s", "", true, true, t0, nil, t0, 299); err == nil {
		t.Fatal("threshold below 300s accepted")
	}
	if a, err := EvaluateStaleTracker("t", "s", "", true, false, t0, nil, t0.Add(2*time.Hour), DefaultStaleTrackerSeconds); err != nil || a != nil {
		t.Fatalf("suspended/non-recording session should not raise stale alert: %+v %v", a, err)
	}
	lastFix := t0.Add(10 * time.Minute)
	if a, err := EvaluateStaleTracker("t", "s", "u", true, true, t0, &lastFix, lastFix.Add(900*time.Second), DefaultStaleTrackerSeconds); err != nil || a != nil {
		t.Fatalf("at exact threshold 900s should not alert: %+v %v", a, err)
	}
	a, err := EvaluateStaleTracker("t", "s", "u", true, true, t0, &lastFix, lastFix.Add(901*time.Second), DefaultStaleTrackerSeconds)
	if err != nil || a == nil || a.Kind != AlertStaleTracker || a.DurationSeconds != 901 || !a.AnchorAt.Equal(lastFix) {
		t.Fatalf("expected stale tracker alert, got %+v %v", a, err)
	}
}

func TestEvaluateLongStop(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	baseLat, baseLon := int32(-235505200), int32(-466333080)
	far := pointOffsetNorthMetres(baseLat, baseLon, 500)
	near := pointOffsetNorthMetres(baseLat, baseLon, 40)
	fixes := []TrackFix{
		{At: t0, Lat: far.Lat, Lon: far.Lon, Accuracy: 8},
		{At: t0.Add(1 * time.Hour), Lat: baseLat, Lon: baseLon, Accuracy: 8},
		{At: t0.Add(3 * time.Hour), Lat: near.Lat, Lon: near.Lon, Accuracy: 8},
		{At: t0.Add(5 * time.Hour), Lat: baseLat, Lon: baseLon, Accuracy: 8},
	}

	// Current time 5 min after last fix -> triggers 4h long-stop alert anchored at t0+1h.
	a, err := EvaluateLongStop("t", "s", "u", fixes, t0.Add(5*time.Hour+5*time.Minute), DefaultLongStopSeconds)
	if err != nil || a == nil || a.Kind != AlertLongStop || !a.AnchorAt.Equal(t0.Add(1*time.Hour)) || a.DurationSeconds != 4*3600 {
		t.Fatalf("expected 4h long stop alert, got %+v %v", a, err)
	}

	// Old stop (>30 min before now) must not be reported as current.
	aOld, err := EvaluateLongStop("t", "s", "u", fixes, t0.Add(6*time.Hour), DefaultLongStopSeconds)
	if err != nil || aOld != nil {
		t.Fatalf("old stop raised as current: %+v %v", aOld, err)
	}
}
