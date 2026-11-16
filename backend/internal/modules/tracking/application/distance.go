package application

import (
	"context"
	"errors"
	"reflect"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// Trip distance sources (EMP-GPS-006, B-BR-EMP-491..494, EDR-318..320): a
// server port with no route. The GPS part is a recorded-track read proved and
// recorded by the store; the odometer part is the METER-006 usage between the
// trip's instants; the answer is the pure tracking evaluation.

// ErrDistanceDisabled means the distance port is not composed.
var ErrDistanceDisabled = errors.New("tracking: distance read disabled")

// absent reports a missing port, including a typed nil.
func absent(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// DistanceRequest names the trip and the reader's current membership.
type DistanceRequest struct {
	private
	TripID, MembershipID string
}

func (r DistanceRequest) Validate() error {
	if !canonicalID(r.TripID) || !canonicalID(r.MembershipID) {
		return ErrMalformed
	}
	return nil
}

// OdometerAnchor is one reading the odometer distance used.
type OdometerAnchor struct {
	private
	ReadingID  string
	Value      int64
	OccurredAt time.Time
}

// OdometerDistance is the odometer source with its evidence: the trip
// instants asked, both anchors that exist, the meter method and the
// projection revision (B-BR-EMP-491).
type OdometerDistance struct {
	private
	Source     domain.DistanceSource
	AssetID    string
	From, To   *time.Time
	Start, End *OdometerAnchor
	Method     string
	Revision   int64
}

// SummaryFacts is the latest stored summary revision of the trip's session.
type SummaryFacts struct {
	private
	Revision                    int
	Method                      string
	Quality                     domain.Quality
	CoveragePermille            *int64
	ExpectedSeconds, GapSeconds int64
	Gaps                        int
	DistanceMetres              *int64
	DistanceMethod              *string
	ProcessedAt                 time.Time
}

// DistanceFacts is what the store reads in its transaction: the trip, and,
// unless the GPS source is withheld, its session and latest summary.
type DistanceFacts struct {
	private
	Trip      tripapp.Record
	Withheld  bool
	SessionID string
	Summary   *SummaryFacts
}

// TrackDistance is the GPS source with its evidence.
type TrackDistance struct {
	private
	Source                      domain.DistanceSource
	SessionID                   string
	Revision                    int
	Method, DistanceMethod      string
	CoveragePermille            *int64
	ExpectedSeconds, GapSeconds int64
	Gaps                        int
	ProcessedAt                 *time.Time
}

// DistanceView is one trip's four distance sources, the travelled basis and
// the divergence evaluation (B-BR-EMP-491..493).
type DistanceView struct {
	private
	TripID            string
	Method            string
	Odometer          OdometerDistance
	GPS               TrackDistance
	Planned, Estimate domain.DistanceSource
	Travelled         domain.Travelled
	Divergence        domain.Divergence
}

// ReadOdometer asks the meter port for the odometer usage between the trip's
// start and finish instants only when the trip has both, and accepts the
// figure only when both anchors are readings at exactly those instants
// (EDR-319).
func ReadOdometer(ctx context.Context, scope kernel.OrgScope, record tripapp.Record, meters fleetapp.MeterUsagePort) (OdometerDistance, error) {
	out := OdometerDistance{AssetID: record.AssetID, Source: domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient}}
	finished := record.Finish != nil
	switch {
	case record.State == trip.InProgress || record.State == trip.Paused:
		out.Source.Reason = domain.ReasonTripNotFinished
		if finished || record.Closure != nil {
			return OdometerDistance{}, ErrUnavailable
		}
		return out, nil
	case record.State == trip.Cancelled:
		out.Source.Reason = domain.ReasonTripCancelled
		return out, nil
	case record.State != trip.Completed || finished == (record.Closure != nil):
		return OdometerDistance{}, ErrUnavailable
	case record.Closure != nil:
		// EMP-TRIP-014: a supervisor closure carries no final reading.
		out.Source.Reason = domain.ReasonNoFinalReading
		return out, nil
	}
	if absent(meters) {
		return OdometerDistance{}, ErrUnavailable
	}
	from, to := record.Declared.OccurredAt, record.Finish.Declared.OccurredAt
	usage, err := meters.UsageBetween(ctx, scope, record.AssetID, meter.Odometer, from, to)
	if cancelled := ctx.Err(); cancelled != nil {
		return OdometerDistance{}, cancelled
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return OdometerDistance{}, err
	case errors.Is(err, fleetapp.ErrNotFound):
		// The trip's asset has no odometer: an hour meter is never a distance.
		out.Source.Reason = domain.ReasonNoMeter
		return out, nil
	case errors.Is(err, fleetapp.ErrUnavailable):
		out.Source.Reason = domain.ReasonMeterUnavailable
		return out, nil
	case err != nil:
		return OdometerDistance{}, ErrUnavailable
	}
	if usage.AssetID != record.AssetID || usage.Kind != meter.Odometer || !usage.From.Equal(from) || !usage.To.Equal(to) || usage.Method != fleetapp.MeterUsageMethod || usage.Revision < 0 {
		return OdometerDistance{}, ErrUnavailable
	}
	out.From, out.To, out.Method, out.Revision = &from, &to, usage.Method, usage.Revision
	// Only a reading at exactly the trip's own instant is the trip's reading.
	if a := usage.Start; a != nil && a.OccurredAt.Equal(from) {
		out.Start = &OdometerAnchor{ReadingID: a.ReadingID, Value: a.Value, OccurredAt: a.OccurredAt}
	}
	if a := usage.End; a != nil && a.OccurredAt.Equal(to) {
		out.End = &OdometerAnchor{ReadingID: a.ReadingID, Value: a.Value, OccurredAt: a.OccurredAt}
	}
	switch usage.Reason {
	case meter.NoReadingBefore, meter.MeterGap:
		if usage.Usage != 0 {
			return OdometerDistance{}, ErrUnavailable
		}
		out.Source.Reason = domain.DistanceReason(usage.Reason)
		return out, nil
	case "":
	default:
		return OdometerDistance{}, ErrUnavailable
	}
	switch {
	case usage.Start == nil || usage.End == nil || usage.Usage < 0 || usage.Usage > domain.MaxDistanceMetres:
		return OdometerDistance{}, ErrUnavailable
	case out.Start == nil:
		// The latest valid reading at the start is an earlier one.
		out.Source.Reason = domain.ReasonNoStartReading
	case out.End == nil:
		// The finish has no valid reading of its own: the usage up to an
		// earlier reading (possibly 0 m from the start reading) is not the
		// trip's distance (BR-TRUTH-003).
		out.Source.Reason = domain.ReasonNoFinalReading
	default:
		out.Source = domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: usage.Usage}
	}
	return out, nil
}

// TrackOf maps the stored facts to the GPS source.
func TrackOf(facts DistanceFacts) (TrackDistance, error) {
	out := TrackDistance{Source: domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceInsufficient}}
	switch {
	case facts.Withheld:
		// B-BR-EMP-494: nothing about the track, not even its existence.
		if facts.SessionID != "" || facts.Summary != nil {
			return TrackDistance{}, ErrUnavailable
		}
		return TrackDistance{Source: domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceWithheld}}, nil
	case facts.SessionID == "":
		if facts.Summary != nil {
			return TrackDistance{}, ErrUnavailable
		}
		out.Source.Reason = domain.ReasonNoTrack
		return out, nil
	case facts.Summary == nil:
		out.SessionID, out.Source.Reason = facts.SessionID, domain.ReasonNotProcessed
		return out, nil
	}
	s := *facts.Summary
	measured := s.DistanceMetres != nil
	if s.Revision < 1 || s.Method != domain.MethodVersion || measured != (s.DistanceMethod != nil) || measured && *s.DistanceMethod != domain.DistanceMethod ||
		s.ExpectedSeconds < 0 || s.GapSeconds < 0 || s.GapSeconds > s.ExpectedSeconds || s.Gaps < 0 || (s.ExpectedSeconds == 0) != (s.CoveragePermille == nil) ||
		s.CoveragePermille != nil && (*s.CoveragePermille < 0 || *s.CoveragePermille > 1000) || s.ProcessedAt.IsZero() {
		return TrackDistance{}, ErrUnavailable
	}
	if _, err := domain.ParseQuality(string(s.Quality)); err != nil {
		return TrackDistance{}, ErrUnavailable
	}
	processed := s.ProcessedAt
	out = TrackDistance{SessionID: facts.SessionID, Revision: s.Revision, Method: s.Method, ExpectedSeconds: s.ExpectedSeconds, GapSeconds: s.GapSeconds, Gaps: s.Gaps, ProcessedAt: &processed}
	if s.CoveragePermille != nil {
		v := *s.CoveragePermille
		out.CoveragePermille = &v
	}
	if !measured {
		out.Source = domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceInsufficient, Reason: domain.ReasonNoGPSDistance, Quality: s.Quality}
		return out, nil
	}
	// B-BR-EMP-464/465: nothing is derived from an insufficient track.
	if s.Quality == domain.QualityInsufficient || *s.DistanceMetres < 0 || *s.DistanceMetres > domain.MaxDistanceMetres {
		return TrackDistance{}, ErrUnavailable
	}
	out.DistanceMethod = *s.DistanceMethod
	out.Source = domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceOK, Metres: *s.DistanceMetres, Quality: s.Quality}
	return out, nil
}

// ComposeDistances evaluates the four sources of one trip; the planned
// distance and the engine estimate have no owner yet (B-BR-EMP-491).
func ComposeDistances(facts DistanceFacts, odometer OdometerDistance, thresholds domain.Thresholds) (DistanceView, error) {
	if !canonicalID(facts.Trip.ID) || odometer.AssetID != facts.Trip.AssetID {
		return DistanceView{}, ErrUnavailable
	}
	track, err := TrackOf(facts)
	if err != nil {
		return DistanceView{}, err
	}
	planned := domain.DistanceSource{Kind: domain.KindPlanned, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRoutePlan}
	estimate := domain.DistanceSource{Kind: domain.KindEstimate, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRouteCompute}
	evaluated, err := domain.EvaluateDistances(odometer.Source, track.Source, planned, estimate, thresholds)
	if err != nil {
		return DistanceView{}, ErrUnavailable
	}
	return DistanceView{TripID: facts.Trip.ID, Method: domain.SourcesMethod, Odometer: odometer, GPS: track, Planned: evaluated.Planned, Estimate: evaluated.Estimate,
		Travelled: evaluated.Travelled, Divergence: evaluated.Divergence}, nil
}

// DistanceRepository proves the reader's recorded-track grant in its read
// transaction, reads the facts, composes the answer and records the access
// before returning anything (B-BR-EMP-494).
type DistanceRepository interface {
	ReadDistances(context.Context, kernel.OrgScope, TrackActor, DistanceRequest) (DistanceView, error)
}

// DistanceService exposes the port when composed.
type DistanceService struct {
	Enabled    bool
	Repository DistanceRepository
}

func (s DistanceService) Read(ctx context.Context, scope kernel.OrgScope, actor TrackActor, request DistanceRequest) (DistanceView, error) {
	if !s.Enabled || absent(s.Repository) {
		return DistanceView{}, ErrDistanceDisabled
	}
	if err := actor.Validate(); err != nil {
		return DistanceView{}, err
	}
	if err := request.Validate(); err != nil {
		return DistanceView{}, err
	}
	return s.Repository.ReadDistances(ctx, scope, actor, request)
}
