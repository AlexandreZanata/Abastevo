package domain

import (
	"testing"
	"time"
)

func TestFineAmountExactTextContract(t *testing.T) {
	for text, want := range map[string]int64{"0,00": 0, "1,01": 101, "1234,56": 123456, "1.234,56": 123456, "92233720368547758,07": 9223372036854775807} {
		got, ok := ParseFineAmount(text)
		if !ok || got != want {
			t.Fatal("exact fine amount", text, got, ok)
		}
	}
	for _, text := range []string{"", "01,00", "-1,00", "1.2,00", "1,1", "1e3", "92233720368547758,08", "+1,00", "1 000,00"} {
		if _, ok := ParseFineAmount(text); ok {
			t.Fatal("malformed source money accepted", text)
		}
	}
}
func TestFineMonthSourceAgeCannotBeRefreshedByLocalPublication(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, text := range []string{"2026-01", "2026-04", "2026-11", "2026-13", "2026-01-01", ""} {
		stale, reason := FineMonthStale(text, now)
		if !stale || reason == "" {
			t.Fatal("source clock not conservatively stale", text)
		}
	}
	if stale, reason := FineMonthStale("2026-10", now); stale || reason != "" {
		t.Fatal("current extraction month stale", reason)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if stale, _ := FineMonthStale("2026-01", start.Add(180*24*time.Hour)); stale {
		t.Fatal("180-day boundary changed")
	}
	if stale, _ := FineMonthStale("2026-01", start.Add(180*24*time.Hour+time.Second)); !stale {
		t.Fatal("uncertain month age incorrectly fresh")
	}
}

func TestFineDayClockAgeDoesNotInventAnExtractionInstant(t *testing.T) {
	start := time.Date(2020, 2, 6, 0, 0, 0, 0, time.UTC)
	if stale, _ := FineSourceClockStale("DAY", "2020-02-06", start.Add(180*24*time.Hour)); stale {
		t.Fatal("day boundary changed")
	}
	if stale, _ := FineSourceClockStale("DAY", "2020-02-06", start.Add(180*24*time.Hour+time.Second)); !stale {
		t.Fatal("old source day made fresh")
	}
	for _, clock := range []struct{ precision, value string }{{"DAY", "2020-02-30"}, {"DAY", "2020-02-06T00:00:00Z"}, {"MINUTE", "2020-02-06"}, {"DAY", "0000-01-01"}} {
		if stale, _ := FineSourceClockStale(clock.precision, clock.value, start); !stale {
			t.Fatal("uncertain clock fresh")
		}
	}
	if stale, _ := FineSourceClockStale("DAY", "2020-02-06", start.Add(-time.Second)); !stale {
		t.Fatal("future source clock fresh")
	}
}
