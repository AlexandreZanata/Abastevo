package application

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

// Admit keeps the fixes of a batch that lie in a recording period of the
// session and inside the trip, coarse when that period is approximate, and
// counts the rest: mock-flagged fixes as mock, fixes outside every period or
// while the trip was not running (before its start, paused, at or after its
// finish or cancellation) as OUTSIDE_SESSION and instants already stored for the session as
// duplicates (B-BR-EMP-432/433). The device's coarse flag never raises or
// lowers the period's precision.
func Admit(batch BatchInput, session domain.Session, bounds TripBounds, stored []time.Time) (Segment, error) {
	if len(batch.Fixes) == 0 || len(batch.Fixes) > MaxFixes {
		return Segment{}, ErrUnavailable
	}
	seen := make(map[int64]bool, len(stored))
	for _, at := range stored {
		seen[at.UnixMicro()] = true
	}
	periods := session.RecordingPeriods()
	segment := Segment{Batch: batch.copy()}
	for _, f := range batch.Fixes {
		precision, inside := periodOf(periods, f.At)
		switch {
		case f.Flags&FlagMock != 0:
			segment.Mock++
		case !inside || !bounds.Running(f.At):
			segment.Outside++
		case seen[f.At.UnixMicro()]:
			segment.Duplicates++
		default:
			seen[f.At.UnixMicro()] = true
			segment.Fixes = append(segment.Fixes, StoredFix{At: f.At, Lat: f.Lat, Lon: f.Lon, Accuracy: f.Accuracy, Speed: copyPointer(f.Speed), Bearing: copyPointer(f.Bearing),
				Coarse: precision == domain.PermissionApproximate})
		}
	}
	return segment, nil
}

// periodOf finds the half-open recording period holding an instant; periods
// are ordered and disjoint, so only the last one starting at or before it can.
func periodOf(periods []domain.Period, at time.Time) (domain.Permission, bool) {
	for i := len(periods) - 1; i >= 0; i-- {
		p := periods[i]
		if at.Before(p.Start()) {
			continue
		}
		if end, closed := p.End(); closed && !at.Before(end) {
			return "", false
		}
		return p.Precision(), true
	}
	return "", false
}
