package domain

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestRegionalWatchUsesExactStationMeanAndNewerComparablePeriods(t *testing.T) {
	before := WatchObservation{Revision: 4, Region: "MT", Method: "synthetic-method", SurveyEnd: "2026-10-31", MinimumStations: 5,
		Sample: Sample{Fuel: kernel.Product("GASOLINE_REGULAR"), Unit: kernel.Unit("L"), Stations: 5, Observations: 7,
			SumMilliBRL: 30000, MinimumMilliBRL: 5000, MaximumMilliBRL: 7000}}
	next := before
	next.Revision, next.SurveyEnd = 5, "2026-11-07"
	next.Sample.Stations, next.Sample.Observations, next.Sample.SumMilliBRL = 6, 9, 36000
	if got := CompareWatchObservations(before, next); got != WatchUnchanged {
		t.Fatalf("equal means returned %q", got)
	}
	next.Sample.SumMilliBRL++
	if got := CompareWatchObservations(before, next); got != WatchIncrease {
		t.Fatalf("an exact sub-milli-BRL difference returned %q", got)
	}
	next.Sample.SumMilliBRL -= 2
	if got := CompareWatchObservations(before, next); got != WatchDecrease {
		t.Fatalf("a lower mean returned %q", got)
	}
	for _, edit := range []func(*WatchObservation){
		func(v *WatchObservation) { v.Revision = before.Revision },
		func(v *WatchObservation) { v.SurveyEnd = before.SurveyEnd },
		func(v *WatchObservation) { v.Region = "PR" },
		func(v *WatchObservation) { v.Method = "different" },
		func(v *WatchObservation) { v.MinimumStations = 9 },
		func(v *WatchObservation) { v.Sample = Sample{} },
	} {
		incomparable := next
		edit(&incomparable)
		if got := CompareWatchObservations(before, incomparable); got != WatchUnavailable {
			t.Fatalf("incomparable observation returned %q", got)
		}
	}
}
