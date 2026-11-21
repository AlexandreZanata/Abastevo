package domain

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Live position projection and freshness rules (EMP-GPS-007, B-BR-EMP-563,
// BR-GPS-030..032, BR-GPS-035, VAL-16). Freshness is derived on read and never
// persisted; out-of-order older fixes never move a live position backwards.
type Freshness string

const (
	FreshnessCurrent  Freshness = "CURRENT"
	FreshnessStale    Freshness = "STALE"
	FreshnessNoSignal Freshness = "NO_SIGNAL"
)

const (
	DefaultCurrentLimitSeconds int64 = 180
	MinCurrentLimitSeconds     int64 = 60
	MaxCurrentLimitSeconds     int64 = 600
	DefaultStaleLimitSeconds   int64 = 900
	MinStaleLimitSeconds       int64 = 300
	MaxStaleLimitSeconds       int64 = 3600
	LivePositionRetention            = 24 * time.Hour
)

// FreshnessLimits bounds read-time live position classification (§6.3).
type FreshnessLimits struct {
	CurrentSeconds int64
	StaleSeconds   int64
}

// DefaultFreshnessLimits returns the §6.3 defaults (180s / 900s).
func DefaultFreshnessLimits() FreshnessLimits {
	return FreshnessLimits{
		CurrentSeconds: DefaultCurrentLimitSeconds,
		StaleSeconds:   DefaultStaleLimitSeconds,
	}
}

// Validate checks that limits sit inside the allowed specification ranges.
func (l FreshnessLimits) Validate() error {
	if l.CurrentSeconds < MinCurrentLimitSeconds || l.CurrentSeconds > MaxCurrentLimitSeconds ||
		l.StaleSeconds < MinStaleLimitSeconds || l.StaleSeconds > MaxStaleLimitSeconds ||
		l.CurrentSeconds >= l.StaleSeconds {
		return ErrInvalid
	}
	return nil
}

// LiveFix is the latest collected fix projection for an active trip; coordinates
// are redacted in logs and JSON serialization of the domain struct.
type LiveFix struct {
	redacted
	CollectedAt time.Time
	ReceivedAt  time.Time
	Lat, Lon    int32
	Accuracy    int32
	Speed       *int32
	Bearing     *int16
	Coarse      bool
	Source      string
	OperationID string
}

func (f LiveFix) Validate() error {
	if f.CollectedAt.IsZero() || f.ReceivedAt.IsZero() ||
		!validPoint(Point{Lat: f.Lat, Lon: f.Lon}) ||
		f.Accuracy < 0 || f.Accuracy > 10_000 ||
		(f.Speed != nil && (*f.Speed < 0 || *f.Speed > 100_000)) ||
		(f.Bearing != nil && (*f.Bearing < 0 || *f.Bearing > 359)) ||
		(f.Source != "" && f.Source != "PHONE") {
		return ErrInvalid
	}
	if f.OperationID != "" {
		id, err := kernel.ParseOrgID(f.OperationID)
		if err != nil || id.String() != f.OperationID {
			return ErrInvalid
		}
	}
	return nil
}

// LivePosition represents the derived live projection row per active trip.
type LivePosition struct {
	redacted
	TripID              string
	SessionID           string
	Fix                 *LiveFix
	LastCommunicationAt *time.Time
	Revision            int64
}

// AdvanceLivePosition applies an accepted fix to the current projection
// (BR-GPS-030, §10.6): only a strictly newer collection instant moves the
// projected fix, while last_communication_at always advances monotonically.
func AdvanceLivePosition(current LivePosition, fix LiveFix) (LivePosition, error) {
	if err := fix.Validate(); err != nil {
		return LivePosition{}, err
	}
	if fix.Source == "" {
		fix.Source = "PHONE"
	}
	next := current
	fixCopy := fix
	fixCopy.CollectedAt = fix.CollectedAt.UTC()
	fixCopy.ReceivedAt = fix.ReceivedAt.UTC()
	if fix.Speed != nil {
		s := *fix.Speed
		fixCopy.Speed = &s
	}
	if fix.Bearing != nil {
		b := *fix.Bearing
		fixCopy.Bearing = &b
	}

	if next.Fix == nil || fixCopy.CollectedAt.After(next.Fix.CollectedAt) {
		next.Fix = &fixCopy
	} else {
		existing := *next.Fix
		next.Fix = &existing
	}

	comm := fixCopy.ReceivedAt
	if next.LastCommunicationAt != nil && next.LastCommunicationAt.After(comm) {
		comm = next.LastCommunicationAt.UTC()
	}
	next.LastCommunicationAt = &comm
	next.Revision = current.Revision + 1
	return next, nil
}

// EvaluateFreshness classifies a live position against server clock `now`
// without mutating or extrapolating the position (BR-GPS-031..032, §10.6).
func EvaluateFreshness(pos LivePosition, now time.Time, limits FreshnessLimits) (Freshness, *int64, error) {
	if err := limits.Validate(); err != nil {
		return "", nil, err
	}
	if pos.Fix == nil {
		return FreshnessNoSignal, nil, nil
	}
	diff := now.UTC().Sub(pos.Fix.CollectedAt.UTC())
	age := int64(diff / time.Second)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= limits.CurrentSeconds:
		return FreshnessCurrent, &age, nil
	case age <= limits.StaleSeconds:
		return FreshnessStale, &age, nil
	default:
		return FreshnessNoSignal, &age, nil
	}
}

// LivePositionExpired reports whether 24 hours have passed since the trip ended
// (BR-GPS-035, VAL-16).
func LivePositionExpired(tripEndedAt *time.Time, now time.Time) bool {
	if tripEndedAt == nil {
		return false
	}
	return !now.UTC().Before(tripEndedAt.UTC().Add(LivePositionRetention))
}

// LivePositionExpiryReached reports whether a stored post-trip expiration
// deadline (tripEndedAt + 24h) has elapsed at `now` (BR-GPS-035, VAL-16).
func LivePositionExpiryReached(expiresAt *time.Time, now time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return !now.UTC().Before(expiresAt.UTC())
}
