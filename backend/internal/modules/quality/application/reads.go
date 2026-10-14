package application

import (
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// StaleAfterDays freezes when a published lease reads stale: twice the
// slowest routine source rhythm plus margin. Older leases stay served
// with the flag set, never silently dropped or refreshed.
const StaleAfterDays = 180

var (
	ErrBadStationID = errors.New("quality: station id must be a UUID")
	ErrUnknownFuel  = errors.New("quality: unknown fuel product")
)

// ValidateStationID accepts canonical UUID text, lowercased.
func ValidateStationID(id string) (string, error) {
	if !isUUID(id) {
		return "", ErrBadStationID
	}
	return strings.ToLower(id), nil
}

func isUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// ValidateFuel accepts the closed FQ-01 wire vocabulary or empty.
func ValidateFuel(fuel string) (string, error) {
	if fuel == "" {
		return "", nil
	}
	for _, f := range domain.AllFuels {
		if string(f) == fuel {
			return fuel, nil
		}
	}
	return "", ErrUnknownFuel
}

// ValidateTextFilter bounds free-text filters so pages cannot smuggle
// unbounded patterns into reads.
func ValidateTextFilter(field, value string, max int) (string, error) {
	if len(value) > max {
		return "", fmt.Errorf("quality: %s filter too long", field)
	}
	return value, nil
}

// Stale reports whether a lease reads stale with its reason. No published
// revision is always stale (unknown is never fresh); older leases stay
// served with the flag so clients see age instead of silence.
func Stale(hasLease bool, publishedAt time.Time, now time.Time) (bool, string) {
	if !hasLease {
		return true, "no published revision"
	}
	if now.Sub(publishedAt) > time.Duration(StaleAfterDays)*24*time.Hour {
		return true, fmt.Sprintf("revision older than %d days", StaleAfterDays)
	}
	return false, ""
}

// CoverageNote freezes the denominator explanation served with every
// read: one sample with ten assays is one sample and ten assays, and
// staging rows stay invisible until publication.
func CoverageNote(edition string, counts domain.Counts) string {
	return fmt.Sprintf(
		"edition %s reconciles %d input as %d accepted, %d duplicates, %d quarantined; "+
			"only published revisions serve reads; corrections supersede, never rewrite",
		edition, counts.Input, counts.Accepted, counts.Duplicates, counts.Quarantined)
}
