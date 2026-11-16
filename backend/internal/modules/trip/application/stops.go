package application

import (
	"context"
	"slices"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-010 stops and legs: planned versus actual (B-BR-EMP-475..480,
// EDR-307..310). The member reads, for an own trip, the plan of the mission
// it executes beside the stops and legs derived from the trip's typed facts,
// under the EMP-TRIP-012 own-trips policy (trip.trip.start over the
// requesting membership).

// StopsPolicy is the own-trips read policy (EDR-309).
func StopsPolicy() access.Policy { return HandoverPolicy(OwnTripsAction) }

// ObservedFact is one stored boundary of an observed stop or leg with its
// recorded times: the declared instant, entry zone and device offset
// (BR-TIME-001) and the server receipt (BR-TIME-002).
type ObservedFact struct {
	private
	Mark       trip.StopMark
	FactID     string
	Declared   orgtime.Declaration
	ReceivedAt time.Time
}

// AfterReceipt flags a boundary declared after its own server receipt
// (BR-TIME-006: reported, never rewritten).
func (f ObservedFact) AfterReceipt() bool { return f.Declared.OccurredAt.After(f.ReceivedAt) }

// ObservedStopView is one derived stop with its recorded boundaries.
type ObservedStopView struct {
	private
	Sequence           int
	Arrival, Departure *ObservedFact
	Issue              trip.StopIssue
}

// ObservedLegView is one derived leg with its recorded boundaries.
type ObservedLegView struct {
	private
	Sequence           int
	Departure, Arrival *ObservedFact
	Issue              trip.StopIssue
}

// HeldObservation is one STOP, ARRIVAL or DEPARTURE event of the trip kept for
// review (EMP-TRIP-003/015); it is never a stop.
type HeldObservation struct {
	private
	EventID  string
	Type     trip.LogbookType
	Declared orgtime.Declaration
	Code     string
}

// PlannedRecord is the current planning revision of the mission the trip
// executes (EMP-TRIP-023 link), as stored.
type PlannedRecord struct {
	private
	MissionID         string
	Revision          int64
	PlannedAt         time.Time
	StartsAt, EndsAt  time.Time
	DestinationText   *string
	DestinationUnitID string
	Stops             []string
}

// PlannedView is the plan shown beside the observed side, with its derived
// legs (no distance or duration).
type PlannedView struct {
	private
	MissionID         string
	Revision          int64
	PlannedAt         time.Time
	StartsAt, EndsAt  time.Time
	DestinationText   *string
	DestinationUnitID string
	Stops             []string
	Legs              []trip.PlannedLeg
}

// StopsRecord is what the store reads for one own trip: the trip, its start,
// its end (finish or cancellation; nil while open or after a closure), its
// accepted STOP, ARRIVAL and DEPARTURE events, its held ones and the plan.
type StopsRecord struct {
	private
	TripID, AssetID string
	State           trip.State
	Start           ObservedFact
	End             *ObservedFact
	Events          []ObservedFact
	Held            []HeldObservation
	Plan            *PlannedRecord
}

// TripStopsView is the member's planned-versus-observed read of one trip;
// AfterEnd lists the accepted observations declared after the recorded end.
type TripStopsView struct {
	private
	TripID, AssetID string
	State           trip.State
	Start           ObservedFact
	End             *ObservedFact
	Planned         *PlannedView
	Stops           []ObservedStopView
	Legs            []ObservedLegView
	AfterEnd        []ObservedFact
	Held            []HeldObservation
}

// StopsRepository reads one own trip in one forced-RLS transaction that
// proves the own-trips authority before and after the read; a trip the
// member neither started nor currently holds is ErrNotFound.
type StopsRepository interface {
	ReadOwnTripStops(context.Context, kernel.OrgScope, ReviewActor, string, string) (StopsRecord, error)
}

// StopsService serves the planned-versus-observed read.
type StopsService struct {
	Enabled    bool
	Repository StopsRepository
}

// Read proves the live own-trips decision before and after the store and
// derives the observed side from the stored facts.
func (s StopsService) Read(ctx context.Context, scope kernel.OrgScope, actor ReviewActor, decision access.Decision, tripID string) (TripStopsView, error) {
	if !s.Enabled {
		return TripStopsView{}, ErrReviewDisabled
	}
	if !canonicalID(tripID) {
		return TripStopsView{}, ErrNotFound
	}
	member := decision.Subject().MembershipID()
	policy := StopsPolicy()
	if err := authorizedReview(ctx, scope, actor, decision, member, policy); err != nil {
		return TripStopsView{}, err
	}
	if absentPort(s.Repository) {
		return TripStopsView{}, ErrUnavailable
	}
	record, err := s.Repository.ReadOwnTripStops(ctx, scope, actor, member, tripID)
	if guard := authorizedReview(ctx, scope, actor, decision, member, policy); guard != nil {
		return TripStopsView{}, guard
	}
	if err != nil {
		return TripStopsView{}, err
	}
	if record.TripID != tripID {
		return TripStopsView{}, ErrUnavailable
	}
	return BuildTripStops(record)
}

// validObserved checks one stored boundary: its kind, identity, declaration
// and receipt.
func validObserved(f ObservedFact, marks ...trip.StopMark) bool {
	if !slices.Contains(marks, f.Mark) || !canonicalID(f.FactID) || f.ReceivedAt.IsZero() {
		return false
	}
	_, err := orgtime.ValidateDeclaration(f.Declared)
	return err == nil
}

func copyFact(f *ObservedFact) *ObservedFact {
	if f == nil {
		return nil
	}
	v := *f
	return &v
}

// plannedView validates the stored planning revision and derives its legs.
func plannedView(p PlannedRecord) (PlannedView, error) {
	if !canonicalID(p.MissionID) || p.Revision < 2 || !validMissionInstant(p.PlannedAt) {
		return PlannedView{}, ErrUnavailable
	}
	if _, err := trip.NewMissionWindow(p.StartsAt, p.EndsAt); err != nil {
		return PlannedView{}, ErrUnavailable
	}
	if (p.DestinationText == nil) == (p.DestinationUnitID == "") || p.DestinationUnitID != "" && !canonicalID(p.DestinationUnitID) ||
		p.DestinationText != nil && trip.ValidPlanText(*p.DestinationText) != nil {
		return PlannedView{}, ErrUnavailable
	}
	stops, err := trip.NewPlannedStops(p.Stops)
	if err != nil {
		return PlannedView{}, ErrUnavailable
	}
	return PlannedView{MissionID: p.MissionID, Revision: p.Revision, PlannedAt: p.PlannedAt, StartsAt: p.StartsAt, EndsAt: p.EndsAt,
		DestinationText: copyPointer(p.DestinationText), DestinationUnitID: p.DestinationUnitID, Stops: stops.Places(), Legs: stops.Legs()}, nil
}

// BuildTripStops validates a stored record and derives its observed stops and
// legs (B-BR-EMP-476/477) and planned legs (B-BR-EMP-475); a record that
// breaks a stored invariant is ErrUnavailable, never repaired.
func BuildTripStops(r StopsRecord) (TripStopsView, error) {
	if !canonicalID(r.TripID) || !canonicalID(r.AssetID) {
		return TripStopsView{}, ErrUnavailable
	}
	if _, err := trip.ParseState(string(r.State)); err != nil {
		return TripStopsView{}, ErrUnavailable
	}
	if !validObserved(r.Start, trip.MarkStart) || r.Start.FactID != r.TripID {
		return TripStopsView{}, ErrUnavailable
	}
	// Only a finished trip ends by its finish, only a cancelled one by its
	// cancellation; an open trip or a supervisor closure has no end instant.
	switch {
	case r.End == nil:
		if r.State == trip.Cancelled {
			return TripStopsView{}, ErrUnavailable
		}
	case r.State == trip.Completed:
		if !validObserved(*r.End, trip.MarkFinish) || r.End.FactID != r.TripID {
			return TripStopsView{}, ErrUnavailable
		}
	case r.State == trip.Cancelled:
		if !validObserved(*r.End, trip.MarkCancel) {
			return TripStopsView{}, ErrUnavailable
		}
	default:
		return TripStopsView{}, ErrUnavailable
	}
	start := trip.StopFact{Mark: r.Start.Mark, ID: r.Start.FactID, At: r.Start.Declared.OccurredAt}
	var end *trip.StopFact
	if r.End != nil {
		end = &trip.StopFact{Mark: r.End.Mark, ID: r.End.FactID, At: r.End.Declared.OccurredAt}
	}
	byID := make(map[string]ObservedFact, len(r.Events)+2)
	facts := make([]trip.StopFact, 0, len(r.Events))
	for _, e := range r.Events {
		if !validObserved(e, trip.MarkStop, trip.MarkArrival, trip.MarkDeparture) {
			return TripStopsView{}, ErrUnavailable
		}
		byID[e.FactID] = e
		facts = append(facts, trip.StopFact{Mark: e.Mark, ID: e.FactID, At: e.Declared.OccurredAt})
	}
	observed, err := trip.ObserveStops(start, end, facts)
	if err != nil {
		return TripStopsView{}, ErrUnavailable
	}
	stored := func(f *trip.StopFact) *ObservedFact {
		switch {
		case f == nil:
			return nil
		case f.Mark == trip.MarkStart:
			return copyFact(&r.Start)
		case f.Mark == trip.MarkFinish || f.Mark == trip.MarkCancel:
			return copyFact(r.End)
		}
		v := byID[f.ID]
		return &v
	}
	out := TripStopsView{TripID: r.TripID, AssetID: r.AssetID, State: r.State, Start: r.Start, End: copyFact(r.End),
		Stops: make([]ObservedStopView, 0, len(observed.Stops)), Legs: make([]ObservedLegView, 0, len(observed.Legs)),
		AfterEnd: make([]ObservedFact, 0, len(observed.AfterEnd)), Held: make([]HeldObservation, 0, len(r.Held))}
	for _, s := range observed.Stops {
		out.Stops = append(out.Stops, ObservedStopView{Sequence: s.Sequence, Arrival: stored(s.Arrival), Departure: stored(s.Departure), Issue: s.Issue})
	}
	for _, l := range observed.Legs {
		out.Legs = append(out.Legs, ObservedLegView{Sequence: l.Sequence, Departure: stored(l.Departure), Arrival: stored(l.Arrival), Issue: l.Issue})
	}
	for i := range observed.AfterEnd {
		out.AfterEnd = append(out.AfterEnd, *stored(&observed.AfterEnd[i]))
	}
	seen := map[string]bool{}
	for _, h := range r.Held {
		if !canonicalID(h.EventID) || seen[h.EventID] || h.Type != trip.LogbookStop && h.Type != trip.LogbookArrival && h.Type != trip.LogbookDeparture ||
			h.Code != TripAlreadyClosed && h.Code != StaleRevision {
			return TripStopsView{}, ErrUnavailable
		}
		if _, err := orgtime.ValidateDeclaration(h.Declared); err != nil {
			return TripStopsView{}, ErrUnavailable
		}
		seen[h.EventID] = true
		out.Held = append(out.Held, h)
	}
	if r.Plan != nil {
		planned, err := plannedView(*r.Plan)
		if err != nil {
			return TripStopsView{}, err
		}
		out.Planned = &planned
	}
	return out, nil
}
