package charging

import (
	"errors"
	"unicode/utf8"
)

var (
	ErrInvalidObservation = errors.New("charging: invalid observation")
	ErrQuotaExceeded      = errors.New("charging: quota exceeded")
	ErrNotOwner           = errors.New("charging: not owner")
	ErrConflict           = errors.New("charging: revision conflict")
)

// MaxObservationsPerAccount bounds community intake without silencing.
// Quotas refuse with explicit errors, never silent drops.
const MaxObservationsPerAccount = 1000

// Observation is one signed community EV charging report: price, access
// or experience at a place/connector on a day. No GPS or document is
// required; reporters stay anonymous in reads.
type Observation struct {
	ID            string
	PlaceID       *string
	ConnectorCode string
	PriceMilliBRL *int64
	Condition     string
	ObservedDay   string
	Note          string
}

// ValidateObservation checks shapes and bounds. Unknown connectors,
// conditions and malformed days refuse; prices stay positive milli-BRL
// per kWh when present (access-only reports omit price honestly).
func ValidateObservation(o Observation) error {
	if len(o.ID) != 36 || o.ConnectorCode == "" || o.Condition == "" || o.ObservedDay == "" {
		return ErrInvalidObservation
	}
	switch o.ConnectorCode {
	case "CCS2", "TYPE2", "CHADEMO", "NACS", "UNKNOWN":
	default:
		return ErrInvalidObservation
	}
	switch o.Condition {
	case "STANDARD", "OFF_PEAK", "PEAK", "MEMBER", "UNKNOWN":
	default:
		return ErrInvalidObservation
	}
	if len(o.ObservedDay) != 10 || o.ObservedDay[4] != '-' || o.ObservedDay[7] != '-' {
		return ErrInvalidObservation
	}
	if o.PriceMilliBRL != nil && (*o.PriceMilliBRL < 1 || *o.PriceMilliBRL > 10000000) {
		return ErrInvalidObservation
	}
	if !utf8.ValidString(o.Note) || len(o.Note) > 1024 {
		return ErrInvalidObservation
	}
	return nil
}
