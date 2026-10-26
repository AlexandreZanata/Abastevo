package charging

import (
	"regexp"
	"time"
)

// OperationalEvent is one dated community assertion, never live availability.
// ConnectionGroupID identifies a source-owned reference, not a nearby site.
type OperationalEvent struct {
	Event             string  `json:"event"`
	TargetScope       string  `json:"target_scope"`
	ConnectionGroupID *string `json:"connection_group_id,omitempty"`
	ObservedAt        string  `json:"observed_at"`
}

var operationalID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func operationalShape(event *OperationalEvent, placeID *string) (time.Time, error) {
	if event == nil {
		return time.Time{}, nil
	}
	if placeID == nil || !operationalID.MatchString(*placeID) {
		return time.Time{}, ErrInvalidObservation
	}
	switch event.Event {
	case "WORKED", "DID_NOT_WORK", "IN_USE", "ACCESS_RESTRICTED":
	default:
		return time.Time{}, ErrInvalidObservation
	}
	switch event.TargetScope {
	case "SITE":
		if event.ConnectionGroupID != nil {
			return time.Time{}, ErrInvalidObservation
		}
	case "EQUIPMENT", "CONNECTOR":
		if event.ConnectionGroupID == nil || !operationalID.MatchString(*event.ConnectionGroupID) {
			return time.Time{}, ErrInvalidObservation
		}
	default:
		return time.Time{}, ErrInvalidObservation
	}
	at, err := time.Parse(time.RFC3339Nano, event.ObservedAt)
	if err != nil {
		return time.Time{}, ErrInvalidObservation
	}
	return at, nil
}

func ValidateOperationalEvent(event *OperationalEvent, placeID *string, now time.Time) error {
	at, err := operationalShape(event, placeID)
	if err != nil {
		return err
	}
	if event != nil && (now.IsZero() || at.After(now)) {
		return ErrInvalidObservation
	}
	return nil
}
