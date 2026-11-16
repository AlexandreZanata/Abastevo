package domain_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-010: the planned stops are a redacted, independent value and the
// derivation never changes its input facts.
func TestPlannedStopsAreRedactedIndependentAndBounded(t *testing.T) {
	none, err := trip.NewPlannedStops(nil)
	if err != nil || none.Len() != 0 || len(none.Places()) != 0 {
		t.Fatal("absent stops", err, none.Len())
	}
	if legs := none.Legs(); len(legs) != 1 || legs[0].From.Kind != trip.PointStart || legs[0].To.Kind != trip.PointDestination || legs[0].Sequence != 1 {
		t.Fatal("a plan without stops has one leg from the start to the destination", legs)
	}
	places := []string{"Cliente Jardim Primavera", "Posto de apoio BR-163"}
	stops, err := trip.NewPlannedStops(places)
	if err != nil || stops.Len() != 2 {
		t.Fatal(err)
	}
	if text := fmt.Sprintf("%v %+v %s", stops, stops, stops); strings.Contains(text, "Cliente") || strings.Contains(text, "Posto") {
		t.Fatal("planned stops print their places", text)
	}
	got := stops.Places()
	got[0] = "changed"
	if stops.Places()[0] != places[0] {
		t.Fatal("Places exposes the stored slice")
	}
	legs := stops.Legs()
	want := []trip.PlannedLeg{
		{Sequence: 1, From: trip.PlannedPoint{Kind: trip.PointStart}, To: trip.PlannedPoint{Kind: trip.PointStop, Stop: 1}},
		{Sequence: 2, From: trip.PlannedPoint{Kind: trip.PointStop, Stop: 1}, To: trip.PlannedPoint{Kind: trip.PointStop, Stop: 2}},
		{Sequence: 3, From: trip.PlannedPoint{Kind: trip.PointStop, Stop: 2}, To: trip.PlannedPoint{Kind: trip.PointDestination}},
	}
	if !slices.Equal(legs, want) {
		t.Fatal("planned legs", legs)
	}
	if _, err := trip.NewPlannedStops(make([]string, trip.MaxPlannedStops+1)); err == nil {
		t.Fatal("more than the bound accepted")
	}
}

func TestObserveStopsNeverChangesItsInputAndReturnsCopies(t *testing.T) {
	start := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	facts := []trip.StopFact{
		{Mark: trip.MarkDeparture, ID: "e2", At: start.Add(40 * time.Minute)},
		{Mark: trip.MarkArrival, ID: "e1", At: start.Add(10 * time.Minute)},
	}
	before := slices.Clone(facts)
	end := trip.StopFact{Mark: trip.MarkFinish, ID: "t", At: start.Add(time.Hour)}
	got, err := trip.ObserveStops(trip.StopFact{Mark: trip.MarkStart, ID: "t", At: start}, &end, facts)
	if err != nil || len(got.Stops) != 1 || len(got.Legs) != 2 {
		t.Fatal("derivation", err, got)
	}
	if !slices.Equal(facts, before) || end.Mark != trip.MarkFinish {
		t.Fatal("the derivation reordered or changed its input")
	}
	got.Stops[0].Arrival.ID = "changed"
	got.Legs[1].Arrival.ID = "changed"
	if facts[1].ID != "e1" || end.ID != "t" {
		t.Fatal("the derivation returned the caller's facts")
	}
	if _, err := trip.ObserveStops(trip.StopFact{Mark: trip.MarkArrival, ID: "t", At: start}, nil, nil); err == nil {
		t.Fatal("a start that is not a START accepted")
	}
	if _, err := trip.ObserveStops(trip.StopFact{Mark: trip.MarkStart, ID: "", At: start}, nil, nil); err == nil {
		t.Fatal("a start without identity accepted")
	}
	if _, err := trip.ObserveStops(trip.StopFact{Mark: trip.MarkStart, ID: "t"}, nil, nil); err == nil {
		t.Fatal("a start without instant accepted")
	}
	if _, err := trip.ObserveStops(trip.StopFact{Mark: trip.MarkStart, ID: "t", At: start}, nil, []trip.StopFact{{Mark: trip.MarkStop, At: start}}); err == nil {
		t.Fatal("a fact without identity accepted")
	}
}
