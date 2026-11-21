package domain

import "time"

// Operational location alerts (EMP-GPS-014, BR-GPS-015..016, BR-GPS-031,
// BR-GPS-038..039). Alerts report observed facts with time, threshold and
// scope, never misconduct and never coordinates.
type AlertKind string

const (
	AlertStaleTracker AlertKind = "STALE_TRACKER"
	AlertLongStop     AlertKind = "LONG_STOP"
	AlertGeofence     AlertKind = "GEOFENCE"
)

const (
	DefaultStaleTrackerSeconds int64 = 900
	MinStaleTrackerSeconds     int64 = 300
	MaxStaleTrackerSeconds     int64 = 7200
	DefaultLongStopSeconds     int64 = 14_400
	MinLongStopSeconds         int64 = 3600
	MaxLongStopSeconds         int64 = 43_200
	LongStopRadiusMetres       int64 = 200
	LongStopCurrentWindow            = 30 * time.Minute
)

// OperationalAlert represents a factual, coordinate-free location alert.
type OperationalAlert struct {
	Kind             AlertKind
	TripID           string
	SessionID        string
	UnitID           string
	GeofenceID       string
	EventKind        string
	AnchorAt         time.Time
	ObservedAt       time.Time
	DurationSeconds  int64
	ThresholdSeconds int64
	RadiusMetres     int64
	LateSuppressed   bool
}

// ValidateStaleTrackerThreshold validates the organization threshold in seconds.
func ValidateStaleTrackerThreshold(seconds int64) error {
	if seconds < MinStaleTrackerSeconds || seconds > MaxStaleTrackerSeconds {
		return ErrInvalid
	}
	return nil
}

// ValidateLongStopThreshold validates the long stationary threshold in seconds.
func ValidateLongStopThreshold(seconds int64) error {
	if seconds < MinLongStopSeconds || seconds > MaxLongStopSeconds {
		return ErrInvalid
	}
	return nil
}

// EvaluateStaleTracker checks whether a recording session on an open trip has
// exceeded its communication silence limit (BR-GPS-031, BR-GPS-038).
func EvaluateStaleTracker(tripID, sessionID, unitID string, tripInProgress, sessionRecording bool, sessionOpenedAt time.Time, latestFixAt *time.Time, now time.Time, thresholdSeconds int64) (*OperationalAlert, error) {
	if err := ValidateStaleTrackerThreshold(thresholdSeconds); err != nil {
		return nil, err
	}
	if !tripInProgress || !sessionRecording || sessionOpenedAt.IsZero() {
		return nil, nil
	}
	anchor := sessionOpenedAt.UTC()
	if latestFixAt != nil && latestFixAt.UTC().After(anchor) {
		anchor = latestFixAt.UTC()
	}
	obs := now.UTC()
	silence := int64(obs.Sub(anchor) / time.Second)
	if silence <= thresholdSeconds {
		return nil, nil
	}
	return &OperationalAlert{
		Kind:             AlertStaleTracker,
		TripID:           tripID,
		SessionID:        sessionID,
		UnitID:           unitID,
		AnchorAt:         anchor,
		ObservedAt:       obs,
		DurationSeconds:  silence,
		ThresholdSeconds: thresholdSeconds,
	}, nil
}

// EvaluateLongStop inspects precise accepted fixes in chronological order to
// detect whether the trailing cluster remains within 200m of its anchor fix
// for at least thresholdSeconds (BR-GPS-016, BR-GPS-038). Old stops whose
// latest fix is older than 30 minutes at `now` are not raised as current.
func EvaluateLongStop(tripID, sessionID, unitID string, fixes []TrackFix, now time.Time, thresholdSeconds int64) (*OperationalAlert, error) {
	if err := ValidateLongStopThreshold(thresholdSeconds); err != nil {
		return nil, err
	}
	var precise []TrackFix
	for _, f := range fixes {
		if !f.Coarse && f.Accuracy >= 0 && f.Accuracy <= MaxPreciseAccuracyMetres && validPoint(Point{Lat: f.Lat, Lon: f.Lon}) {
			precise = append(precise, f)
		}
	}
	if len(precise) < 2 {
		return nil, nil
	}
	obs := now.UTC()
	last := precise[len(precise)-1]
	if obs.Sub(last.At.UTC()) > LongStopCurrentWindow {
		return nil, nil
	}
	// Walk backwards from the latest fix while points remain within 200 m of `last`
	// and check against the earliest fix of the cluster as anchor.
	anchorIdx := len(precise) - 1
	lastPoint := Point{Lat: last.Lat, Lon: last.Lon}
	for i := len(precise) - 2; i >= 0; i-- {
		p := Point{Lat: precise[i].Lat, Lon: precise[i].Lon}
		if Metres(p, lastPoint) > float64(LongStopRadiusMetres) {
			break
		}
		anchorIdx = i
	}
	anchor := precise[anchorIdx]
	// Re-verify all points from anchorIdx..end stay within 200 m of anchor itself.
	anchorPoint := Point{Lat: anchor.Lat, Lon: anchor.Lon}
	for i := anchorIdx + 1; i < len(precise); i++ {
		if Metres(anchorPoint, Point{Lat: precise[i].Lat, Lon: precise[i].Lon}) > float64(LongStopRadiusMetres) {
			anchorIdx = i
			anchor = precise[i]
			anchorPoint = Point{Lat: anchor.Lat, Lon: anchor.Lon}
		}
	}
	duration := int64(last.At.UTC().Sub(anchor.At.UTC()) / time.Second)
	if duration < thresholdSeconds {
		return nil, nil
	}
	return &OperationalAlert{
		Kind:             AlertLongStop,
		TripID:           tripID,
		SessionID:        sessionID,
		UnitID:           unitID,
		AnchorAt:         anchor.At.UTC(),
		ObservedAt:       obs,
		DurationSeconds:  duration,
		ThresholdSeconds: thresholdSeconds,
		RadiusMetres:     LongStopRadiusMetres,
	}, nil
}
