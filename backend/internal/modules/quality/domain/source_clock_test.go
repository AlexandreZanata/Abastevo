package domain

import "testing"

func TestSourceCollectionDayPreservesUnzonedNativeClock(t *testing.T) {
	for _, source := range []string{"2025-01-04", "2025-01-04 12:34:56", "2024-02-29 00:00:00"} {
		day, ok := SourceCollectionDay(source)
		if !ok || day != source[:10] {
			t.Fatal(source, day, ok)
		}
	}
	for _, source := range []string{"2026-02-30 12:00:00", "2026-09-04 24:00:00", "2026-09-04 12:60:00", "2026-09-04 12:00:60", "2026-09-04T12:00:00", "2026-09-04 12:00:00Z", "2026-09-04 12:00:00.1", "2026-09-04 12:00:0x", "0000-01-01 00:00:00"} {
		if _, ok := SourceCollectionDay(source); ok {
			t.Fatal("bad source clock accepted", source)
		}
	}
}
