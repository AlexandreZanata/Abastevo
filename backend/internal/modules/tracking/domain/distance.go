package domain

import "errors"

// Distance sources of a trip (EMP-GPS-006, B-BR-EMP-491..493, BR-GPS-027..029,
// BR-TRUTH-003/005, EDR-318..320). Pure evaluation of four independent
// sources: the travelled basis follows BR-GPS-028 and the divergence signal
// is data for review. Every value is an exact whole number of metres; an
// unknown value is a reason, never zero.

// ErrDistanceInput refuses an inconsistent source, an out-of-range value or
// threshold, or a source passed in another source's place.
var ErrDistanceInput = errors.New("tracking: invalid distance sources")

// SourcesMethod versions the evaluation of a trip's distance sources.
const SourcesMethod = "trip-distance-sources-v1"

// MaxDistanceMetres bounds every distance (BR-MONEY-010 odometer ceiling).
const MaxDistanceMetres int64 = 1_000_000_000_000

// Divergence thresholds: BR-GPS-029 defaults and accepted ranges.
const (
	DefaultDivergenceMetres  int64 = 2_000
	MinDivergenceMetres      int64 = 500
	MaxDivergenceMetres      int64 = 10_000
	DefaultDivergencePercent int64 = 10
	MinDivergencePercent     int64 = 5
	MaxDivergencePercent     int64 = 30
)

// DistanceKind names one of the four independent distances (§10.1).
type DistanceKind string

const (
	KindOdometer DistanceKind = "ODOMETER"
	KindGPS      DistanceKind = "GPS"
	KindPlanned  DistanceKind = "PLANNED"
	KindEstimate DistanceKind = "ESTIMATE"
)

// SourceStatus is whether a source has a value.
type SourceStatus string

const (
	SourceOK           SourceStatus = "OK"
	SourceInsufficient SourceStatus = "INSUFFICIENT_DATA"
	// SourceWithheld is a GPS source the reader may not see (B-BR-EMP-494).
	SourceWithheld SourceStatus = "WITHHELD"
)

// DistanceReason says why a source, the basis or the signal has no value.
type DistanceReason string

// Source, basis and signal reasons (the odometer's, the GPS source's, the
// planned and estimated sources' and the signal's).
const (
	ReasonTripNotFinished    DistanceReason = "TRIP_NOT_FINISHED"
	ReasonTripCancelled      DistanceReason = "TRIP_CANCELLED"
	ReasonNoMeter            DistanceReason = "NO_METER"
	ReasonNoStartReading     DistanceReason = "NO_START_READING"
	ReasonNoFinalReading     DistanceReason = "NO_FINAL_READING"
	ReasonNoReadingBefore    DistanceReason = "NO_READING_BEFORE"
	ReasonMeterGap           DistanceReason = "METER_GAP"
	ReasonMeterUnavailable   DistanceReason = "METER_UNAVAILABLE"
	ReasonNoTrack            DistanceReason = "NO_TRACK"
	ReasonNotProcessed       DistanceReason = "NOT_PROCESSED"
	ReasonNoGPSDistance      DistanceReason = "NO_GPS_DISTANCE"
	ReasonNoRoutePlan        DistanceReason = "NO_ROUTE_PLAN"
	ReasonNoRouteCompute     DistanceReason = "NO_ROUTE_COMPUTATION"
	ReasonNoOdometerDistance DistanceReason = "NO_ODOMETER_DISTANCE"
	ReasonGPSWithheld        DistanceReason = "GPS_WITHHELD"
	ReasonGPSNotPrecise      DistanceReason = "GPS_NOT_PRECISE"
)

// DistanceSource is one independent distance: a value only when OK.
// Quality belongs to the GPS source only.
type DistanceSource struct {
	Kind    DistanceKind
	Status  SourceStatus
	Reason  DistanceReason
	Metres  int64
	Quality Quality
}

// Thresholds are validated divergence thresholds.
type Thresholds struct {
	Metres  int64
	Percent int64
}

// NewThresholds accepts thresholds inside the BR-GPS-029 ranges.
func NewThresholds(metres, percent int64) (Thresholds, error) {
	t := Thresholds{Metres: metres, Percent: percent}
	if !t.valid() {
		return Thresholds{}, ErrDistanceInput
	}
	return t, nil
}

func (t Thresholds) valid() bool {
	return t.Metres >= MinDivergenceMetres && t.Metres <= MaxDivergenceMetres && t.Percent >= MinDivergencePercent && t.Percent <= MaxDivergencePercent
}

// DefaultThresholds are the BR-GPS-029 defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{Metres: DefaultDivergenceMetres, Percent: DefaultDivergencePercent}
}

// Basis names the source a travelled figure uses.
type Basis string

const (
	BasisOdometer Basis = "ODOMETER"
	BasisGPS      Basis = "GPS"
)

// Travelled is the BR-GPS-028 figure: a basis and metres, or a reason.
type Travelled struct {
	Status SourceStatus
	Basis  Basis
	Metres int64
	Reason DistanceReason
}

// Value returns the travelled metres with their basis; ok is false when the
// figure is INSUFFICIENT_DATA, so no consumer reads an absent basis as zero.
func (t Travelled) Value() (metres int64, basis Basis, ok bool) {
	if t.Status != SourceOK {
		return 0, "", false
	}
	return t.Metres, t.Basis, true
}

// DivergenceStatus is the BR-GPS-029 evaluation.
type DivergenceStatus string

const (
	DivergenceRaised       DivergenceStatus = "RAISED"
	DivergenceNotRaised    DivergenceStatus = "NOT_RAISED"
	DivergenceNotEvaluated DivergenceStatus = "NOT_EVALUATED"
)

// Divergence is evidence for review: both values, their difference and the
// thresholds, or why it was not evaluated. It names no person and no cause.
type Divergence struct {
	Status     DivergenceStatus
	Reason     DistanceReason
	Odometer   int64
	GPS        int64
	Difference int64
	Thresholds Thresholds
}

// Distances is the evaluation of one trip's four sources, kept apart.
type Distances struct {
	Odometer, GPS, Planned, Estimate DistanceSource
	Travelled                        Travelled
	Divergence                       Divergence
}

// reasons are the closed reasons each kind of source may carry.
var reasons = map[DistanceKind][]DistanceReason{
	KindOdometer: {ReasonTripNotFinished, ReasonTripCancelled, ReasonNoMeter, ReasonNoStartReading, ReasonNoFinalReading, ReasonNoReadingBefore, ReasonMeterGap, ReasonMeterUnavailable},
	KindGPS:      {ReasonNoTrack, ReasonNotProcessed, ReasonNoGPSDistance},
	KindPlanned:  {ReasonNoRoutePlan},
	KindEstimate: {ReasonNoRouteCompute},
}

// valid checks one source in its own place (kind) against BR-TRUTH-003: a
// value only when OK, a closed reason otherwise, quality only on GPS and
// only where a summary exists, WITHHELD only for GPS and empty.
func (s DistanceSource) valid(kind DistanceKind) bool {
	if s.Kind != kind || s.Metres < 0 || s.Metres > MaxDistanceMetres {
		return false
	}
	quality := s.Quality == "" || s.Quality == QualityPrecise || s.Quality == QualityApproximate || s.Quality == QualityInsufficient
	if !quality || kind != KindGPS && s.Quality != "" {
		return false
	}
	switch s.Status {
	case SourceOK:
		// A GPS distance exists only on a PRECISE or APPROXIMATE summary.
		return s.Reason == "" && (kind != KindGPS || s.Quality == QualityPrecise || s.Quality == QualityApproximate)
	case SourceInsufficient:
		if s.Metres != 0 || kind == KindGPS && s.Reason != ReasonNoGPSDistance && s.Quality != "" {
			return false
		}
		for _, r := range reasons[kind] {
			if s.Reason == r {
				return true
			}
		}
	case SourceWithheld:
		return kind == KindGPS && s.Reason == "" && s.Metres == 0 && s.Quality == ""
	}
	return false
}

// EvaluateDistances applies BR-GPS-027..029 to the four sources, each in its
// own place: the sources come back unchanged, the travelled figure uses the
// odometer, else a PRECISE GPS distance, else states the odometer's reason,
// and the divergence compares only an odometer distance with a PRECISE GPS
// distance in exact integers.
func EvaluateDistances(odometer, gps, planned, estimate DistanceSource, thresholds Thresholds) (Distances, error) {
	if !odometer.valid(KindOdometer) || !gps.valid(KindGPS) || !planned.valid(KindPlanned) || !estimate.valid(KindEstimate) || !thresholds.valid() {
		return Distances{}, ErrDistanceInput
	}
	out := Distances{Odometer: odometer, GPS: gps, Planned: planned, Estimate: estimate}
	precise := gps.Status == SourceOK && gps.Quality == QualityPrecise
	switch {
	case odometer.Status == SourceOK:
		out.Travelled = Travelled{Status: SourceOK, Basis: BasisOdometer, Metres: odometer.Metres}
	case precise:
		out.Travelled = Travelled{Status: SourceOK, Basis: BasisGPS, Metres: gps.Metres}
	default:
		out.Travelled = Travelled{Status: SourceInsufficient, Reason: odometer.Reason}
	}
	out.Divergence = Divergence{Status: DivergenceNotEvaluated, Thresholds: thresholds}
	switch {
	case odometer.Status != SourceOK:
		out.Divergence.Reason = ReasonNoOdometerDistance
	case gps.Status == SourceWithheld:
		out.Divergence.Reason = ReasonGPSWithheld
	case gps.Status != SourceOK:
		out.Divergence.Reason = ReasonNoGPSDistance
	case !precise:
		out.Divergence.Reason = ReasonGPSNotPrecise
	default:
		// Both values are at most 10¹² m: diff × 100 and P × odometer stay
		// far below the int64 range.
		diff := gps.Metres - odometer.Metres
		if diff < 0 {
			diff = -diff
		}
		status := DivergenceNotRaised
		if diff > thresholds.Metres && diff*100 > thresholds.Percent*odometer.Metres {
			status = DivergenceRaised
		}
		out.Divergence = Divergence{Status: status, Odometer: odometer.Metres, GPS: gps.Metres, Difference: diff, Thresholds: thresholds}
	}
	return out, nil
}
