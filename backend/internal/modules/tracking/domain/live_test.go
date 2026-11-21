package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFreshnessLimitsValidation(t *testing.T) {
	if err := DefaultFreshnessLimits().Validate(); err != nil {
		t.Fatalf("default limits rejected: %v", err)
	}
	for name, limits := range map[string]FreshnessLimits{
		"current too small":   {CurrentSeconds: 59, StaleSeconds: 300},
		"current too large":   {CurrentSeconds: 601, StaleSeconds: 900},
		"stale too small":     {CurrentSeconds: 180, StaleSeconds: 299},
		"stale too large":     {CurrentSeconds: 180, StaleSeconds: 3601},
		"current >= stale":    {CurrentSeconds: 300, StaleSeconds: 300},
		"current above stale": {CurrentSeconds: 400, StaleSeconds: 350},
	} {
		if err := limits.Validate(); err == nil {
			t.Fatalf("%s accepted unexpectedly", name)
		}
	}
}

func TestAdvanceLivePositionMonotonicityAndLateCommunication(t *testing.T) {
	t10 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	t12 := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	t1410 := time.Date(2026, 11, 18, 14, 10, 0, 0, time.UTC)
	pos := LivePosition{TripID: "trip-1", SessionID: "sess-1"}

	// Initial fix at 12:00 received at 12:01.
	pos, err := AdvanceLivePosition(pos, LiveFix{
		CollectedAt: t12,
		ReceivedAt:  t12.Add(time.Minute),
		Lat:         -235500000,
		Lon:         -466300000,
		Accuracy:    8,
		Source:      "PHONE",
	})
	if err != nil {
		t.Fatalf("initial advance failed: %v", err)
	}
	if pos.Revision != 1 || pos.Fix == nil || !pos.Fix.CollectedAt.Equal(t12) {
		t.Fatalf("unexpected initial live position: %+v", pos)
	}

	// Out-of-order fix from 10:00 received at 14:10 updates communication time,
	// never moving the projected fix backwards (§10.6, BR-GPS-030).
	pos, err = AdvanceLivePosition(pos, LiveFix{
		CollectedAt: t10,
		ReceivedAt:  t1410,
		Lat:         -100000000,
		Lon:         -500000000,
		Accuracy:    10,
		Source:      "PHONE",
	})
	if err != nil {
		t.Fatalf("late advance failed: %v", err)
	}
	if pos.Revision != 2 || !pos.Fix.CollectedAt.Equal(t12) || pos.Fix.Lat != -235500000 {
		t.Fatalf("older fix overwrote newer position")
	}
	if pos.LastCommunicationAt == nil || !pos.LastCommunicationAt.Equal(t1410) {
		t.Fatalf("last communication did not advance to 14:10")
	}

	// §10.6: at 14:10, the 12:00 fix stays NO_SIGNAL even though communication was at 14:10.
	freshness, age, err := EvaluateFreshness(pos, t1410, DefaultFreshnessLimits())
	if err != nil || freshness != FreshnessNoSignal || age == nil || *age != 7800 {
		t.Fatalf("expected NO_SIGNAL at 14:10, got %s %v %v", freshness, age, err)
	}
}

func TestEvaluateFreshnessVAL16Boundaries(t *testing.T) {
	// VAL-16: fix at 10:00:00, limit 120s.
	t10 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	pos := LivePosition{
		TripID: "trip-val16",
		Fix: &LiveFix{
			CollectedAt: t10,
			ReceivedAt:  t10,
			Lat:         -156000000,
			Lon:         -561000000,
			Accuracy:    12,
			Source:      "PHONE",
		},
	}
	limits := FreshnessLimits{CurrentSeconds: 120, StaleSeconds: 900}

	// At 10:01:30 -> age 90s -> CURRENT.
	f, age, err := EvaluateFreshness(pos, t10.Add(90*time.Second), limits)
	if err != nil || f != FreshnessCurrent || age == nil || *age != 90 {
		t.Fatalf("at 10:01:30 got %s age=%v err=%v", f, age, err)
	}
	// At 10:02:01 -> age 121s -> STALE.
	f, age, err = EvaluateFreshness(pos, t10.Add(121*time.Second), limits)
	if err != nil || f != FreshnessStale || age == nil || *age != 121 {
		t.Fatalf("at 10:02:01 got %s age=%v err=%v", f, age, err)
	}
	// At 10:15:01 -> age 901s -> NO_SIGNAL.
	f, age, err = EvaluateFreshness(pos, t10.Add(901*time.Second), limits)
	if err != nil || f != FreshnessNoSignal || age == nil || *age != 901 {
		t.Fatalf("at 10:15:01 got %s age=%v err=%v", f, age, err)
	}
	// No fix -> NO_SIGNAL with nil age.
	f, age, err = EvaluateFreshness(LivePosition{}, t10, limits)
	if err != nil || f != FreshnessNoSignal || age != nil {
		t.Fatalf("empty pos got %s age=%v err=%v", f, age, err)
	}
}

func TestLivePositionRetentionAndRedaction(t *testing.T) {
	ended := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	if LivePositionExpired(nil, ended.Add(48*time.Hour)) {
		t.Fatal("open trip should not expire")
	}
	if LivePositionExpired(&ended, ended.Add(23*time.Hour+59*time.Minute)) {
		t.Fatal("expired before 24h")
	}
	if !LivePositionExpired(&ended, ended.Add(24*time.Hour)) {
		t.Fatal("not expired at 24h")
	}
	deadline := ended.Add(24 * time.Hour)
	if LivePositionExpiryReached(nil, deadline) || LivePositionExpiryReached(&deadline, deadline.Add(-time.Second)) {
		t.Fatal("expires_at triggered before deadline")
	}
	if !LivePositionExpiryReached(&deadline, deadline) {
		t.Fatal("expires_at did not trigger at deadline")
	}
	fix := LiveFix{CollectedAt: ended, ReceivedAt: ended, Lat: -235505200, Lon: -466333080, Accuracy: 10}
	printed := fmt.Sprintf("%s %v", fix, LivePosition{Fix: &fix})
	if strings.Contains(printed, "235505200") || strings.Contains(printed, "466333080") {
		t.Fatalf("coordinates leaked in string representation: %s", printed)
	}
}
