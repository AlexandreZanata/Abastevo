package domain

import (
	"slices"
	"strings"
	"time"
)

// EMP-TRIP-010 stops and legs: planned versus actual (B-BR-EMP-475..477,
// EDR-307/308). The planned side is a planning revision's ordered stops; the
// observed side is derived from the trip's typed facts and never stored.

// MaxPlannedStops is the technical bound of a plan's intermediate stops
// (EDR-307), not a product rule.
const MaxPlannedStops = 20

// PlannedStops is the validated, ordered list of a planning revision's
// intermediate stops; the zero value has none.
type PlannedStops struct {
	redacted
	places []string
}

// NewPlannedStops validates the planner's stops (B-BR-EMP-475): nil is no
// stop; otherwise 1..MaxPlannedStops places, each with the plan text profile.
// An empty non-nil list is ErrInvalid.
func NewPlannedStops(places []string) (PlannedStops, error) {
	if places == nil {
		return PlannedStops{}, nil
	}
	if len(places) == 0 || len(places) > MaxPlannedStops {
		return PlannedStops{}, ErrInvalid
	}
	for _, place := range places {
		if err := ValidPlanText(place); err != nil {
			return PlannedStops{}, ErrInvalid
		}
	}
	return PlannedStops{places: slices.Clone(places)}, nil
}

// Places returns an independent copy of the places in order.
func (p PlannedStops) Places() []string { return slices.Clone(p.places) }

// Len is the number of planned stops.
func (p PlannedStops) Len() int { return len(p.places) }

// PlannedPointKind is one end of a planned leg.
type PlannedPointKind string

const (
	PointStart       PlannedPointKind = "START"
	PointStop        PlannedPointKind = "STOP"
	PointDestination PlannedPointKind = "DESTINATION"
)

// PlannedPoint is the trip start, the planned stop with its 1-based sequence,
// or the plan's destination.
type PlannedPoint struct {
	Kind PlannedPointKind
	Stop int
}

// PlannedLeg is one derived planned leg; it carries no distance or duration.
type PlannedLeg struct {
	Sequence int
	From, To PlannedPoint
}

// Legs derives the planned legs: start → stop 1 → … → stop n → destination.
func (p PlannedStops) Legs() []PlannedLeg {
	legs := make([]PlannedLeg, 0, len(p.places)+1)
	from := PlannedPoint{Kind: PointStart}
	for i := range p.places {
		to := PlannedPoint{Kind: PointStop, Stop: i + 1}
		legs = append(legs, PlannedLeg{Sequence: len(legs) + 1, From: from, To: to})
		from = to
	}
	return append(legs, PlannedLeg{Sequence: len(legs) + 1, From: from, To: PlannedPoint{Kind: PointDestination}})
}

// StopMark is the kind of one fact that bounds an observed stop or leg.
type StopMark string

const (
	MarkStart     StopMark = "START"
	MarkStop      StopMark = "STOP"
	MarkArrival   StopMark = "ARRIVAL"
	MarkDeparture StopMark = "DEPARTURE"
	MarkFinish    StopMark = "FINISH"
	MarkCancel    StopMark = "CANCEL"
)

// StopFact is one timeline fact used by the derivation: its kind, identity
// and declared instant.
type StopFact struct {
	Mark StopMark
	ID   string
	At   time.Time
}

// StopIssue names an incoherent boundary (B-BR-EMP-477); "" is none.
type StopIssue string

const (
	ArrivalMissing   StopIssue = "ARRIVAL_MISSING"
	DepartureMissing StopIssue = "DEPARTURE_MISSING"
)

// ObservedStop is one derived stop: the fact that opened it, the departure
// that closed it (nil while open, missing or when the trip ended there).
type ObservedStop struct {
	Sequence           int
	Arrival, Departure *StopFact
	Issue              StopIssue
}

// ObservedLeg is one derived leg between two consecutive boundaries.
type ObservedLeg struct {
	Sequence           int
	Departure, Arrival *StopFact
	Issue              StopIssue
}

// Observation is the derived observed side of one trip. AfterEnd lists, in
// timeline order, the accepted facts declared after the recorded end (a
// device clock that moved): never a stop or leg boundary, never dropped.
type Observation struct {
	Stops    []ObservedStop
	Legs     []ObservedLeg
	AfterEnd []StopFact
}

// valid reports a fact with an identity and an instant.
func (f StopFact) valid() bool { return f.ID != "" && !f.At.IsZero() }

// compareStopFacts is the shared timeline order between events (B-BR-EMP-324):
// declared instant, then identity; the receipt is never a key.
func compareStopFacts(a, b StopFact) int {
	if c := a.At.Compare(b.At); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

func factRef(f StopFact) *StopFact { return &f }

// ObserveStops derives the observed stops and legs (B-BR-EMP-476/477) from
// the trip start, its optional end (FINISH or CANCEL) and its STOP, ARRIVAL
// and DEPARTURE facts in any delivery order. A STOP or ARRIVAL opens a stop
// and ends the moving leg; a DEPARTURE closes the open stop and opens the next
// leg; the end closes the moving leg or leaves the trip at its open stop.
// Incoherent sequences are surfaced (ArrivalMissing, DepartureMissing),
// never paired, reordered or dropped; a fact declared after the recorded end
// is listed in AfterEnd. A start that is not a START, an end that is neither
// FINISH nor CANCEL or precedes the start, another kind of fact, a fact before
// the start, a missing identity or instant, or a repeated fact identity is
// ErrInvalid. The input is never changed and the result holds copies.
func ObserveStops(start StopFact, end *StopFact, facts []StopFact) (Observation, error) {
	if start.Mark != MarkStart || !start.valid() {
		return Observation{}, ErrInvalid
	}
	if end != nil && (end.Mark != MarkFinish && end.Mark != MarkCancel || !end.valid() || end.At.Before(start.At)) {
		return Observation{}, ErrInvalid
	}
	var out Observation
	ordered := make([]StopFact, 0, len(facts))
	seen := make(map[string]bool, len(facts))
	for _, f := range facts {
		if f.Mark != MarkStop && f.Mark != MarkArrival && f.Mark != MarkDeparture || !f.valid() || seen[f.ID] || f.At.Before(start.At) {
			return Observation{}, ErrInvalid
		}
		seen[f.ID] = true
		if end != nil && f.At.After(end.At) {
			out.AfterEnd = append(out.AfterEnd, f)
		} else {
			ordered = append(ordered, f)
		}
	}
	slices.SortStableFunc(ordered, compareStopFacts)
	slices.SortStableFunc(out.AfterEnd, compareStopFacts)
	from, open := factRef(start), -1
	for _, f := range ordered {
		switch f.Mark {
		case MarkStop, MarkArrival:
			if open < 0 {
				out.Legs = append(out.Legs, ObservedLeg{Sequence: len(out.Legs) + 1, Departure: from, Arrival: factRef(f)})
			} else {
				out.Stops[open].Issue = DepartureMissing
				out.Legs = append(out.Legs, ObservedLeg{Sequence: len(out.Legs) + 1, Arrival: factRef(f), Issue: DepartureMissing})
			}
			out.Stops = append(out.Stops, ObservedStop{Sequence: len(out.Stops) + 1, Arrival: factRef(f)})
			open, from = len(out.Stops)-1, nil
		case MarkDeparture:
			if open >= 0 {
				out.Stops[open].Departure = factRef(f)
			} else {
				out.Stops = append(out.Stops, ObservedStop{Sequence: len(out.Stops) + 1, Departure: factRef(f), Issue: ArrivalMissing})
				out.Legs = append(out.Legs, ObservedLeg{Sequence: len(out.Legs) + 1, Departure: from, Issue: ArrivalMissing})
			}
			open, from = -1, factRef(f)
		}
	}
	if open < 0 {
		leg := ObservedLeg{Sequence: len(out.Legs) + 1, Departure: from}
		if end != nil {
			leg.Arrival = factRef(*end)
		}
		out.Legs = append(out.Legs, leg)
	}
	return out, nil
}
