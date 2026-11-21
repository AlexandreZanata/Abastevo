package domain

import "time"

// Track retention and trimming rules (EMP-GPS-011, BR-GPS-035..036).
const (
	DefaultRawRetentionDays = 90
	MinRawRetentionDays     = 30
	MaxRawRetentionDays     = 365
)

// PurgeReason records why raw track segments were deleted while retaining the
// derived summary and simplified geometry.
type PurgeReason string

const (
	PurgeRetentionExpired PurgeReason = "RETENTION_EXPIRED"
	PurgeTrimmedAfterEnd  PurgeReason = "TRIMMED_AFTER_END"
)

// ValidateRetentionDays enforces the BR-GPS-035 organization configuration range.
func ValidateRetentionDays(days int) error {
	if days < MinRawRetentionDays || days > MaxRawRetentionDays {
		return ErrInvalid
	}
	return nil
}

// RawTrackExpired reports whether raw fixes for a finished trip have passed
// the configured retention window (BR-GPS-035), blocking reads even if the
// cleanup job has not run yet.
func RawTrackExpired(tripEndedAt *time.Time, retentionDays int, now time.Time) (bool, error) {
	if err := ValidateRetentionDays(retentionDays); err != nil {
		return false, err
	}
	if tripEndedAt == nil {
		return false, nil
	}
	deadline := tripEndedAt.UTC().Add(time.Duration(retentionDays) * 24 * time.Hour)
	return !now.UTC().Before(deadline), nil
}

// TrimFixesAfterEnd excludes fixes collected after a corrected/effective trip
// end instant (BR-GPS-036).
func TrimFixesAfterEnd(fixes []TrackFix, effectiveEnd *time.Time) (kept []TrackFix, trimmedCount int) {
	if effectiveEnd == nil {
		kept = make([]TrackFix, len(fixes))
		copy(kept, fixes)
		return kept, 0
	}
	end := effectiveEnd.UTC()
	for _, f := range fixes {
		if f.At.UTC().After(end) {
			trimmedCount++
			continue
		}
		kept = append(kept, f)
	}
	return kept, trimmedCount
}
