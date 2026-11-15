package domain

import (
	"errors"
	"time"
)

var (
	// ErrAttachmentEnded refuses detaching an attachment that is already
	// ENDED at the evaluated instant (B-BR-EMP-326).
	ErrAttachmentEnded = errors.New("fleet: attachment already ended")
	// ErrImplementMainAsset refuses an IMPLEMENT as the main asset of a trip
	// (BR-FLEET-033, B-BR-EMP-328).
	ErrImplementMainAsset = errors.New("fleet: an implement is never the main asset of a trip")
)

// ValidAttachmentKinds checks BR-FLEET-033: the attached asset is an
// IMPLEMENT and the powered asset is a VEHICLE or a MACHINE.
func ValidAttachmentKinds(implement, powered Kind) error {
	if implement != Implement || powered != Vehicle && powered != Machine {
		return ErrInvalid
	}
	return nil
}

// MainAssetAllowed is the trip-start eligibility of the main asset's kind:
// an IMPLEMENT is refused (AC4); the trip intake calls it (EMP-TRIP-002).
func MainAssetAllowed(kind Kind) error {
	switch kind {
	case Vehicle, Machine:
		return nil
	case Implement:
		return ErrImplementMainAsset
	}
	return ErrInvalid
}

// ValidNewAttachment checks an attachment period being created: non-empty,
// whole-microsecond bounds, starting no earlier than now.
func ValidNewAttachment(period AssignmentPeriod, now time.Time) error {
	if _, err := NewAssignmentPeriod(period.From, period.To); err != nil || period.Empty() || now.IsZero() || period.From.Before(now) {
		return ErrInvalid
	}
	return nil
}

// DetachAt ends an attachment that is not ENDED at now. Without an explicit
// end it ends at the microsecond floor of now (at its start when it has not
// started). An explicit end must be a whole microsecond, not before now, not
// before the attachment's start ("detach before attach") and not after a
// planned end; ending exactly at the start leaves the empty period of an
// attachment that never applied.
func DetachAt(period AssignmentPeriod, end *time.Time, now time.Time) (AssignmentPeriod, error) {
	if _, err := NewAssignmentPeriod(period.From, period.To); err != nil {
		return AssignmentPeriod{}, err
	}
	instant, err := AllocationInstant(now)
	if err != nil {
		return AssignmentPeriod{}, err
	}
	if period.StateAt(now) == AssignmentEnded {
		return AssignmentPeriod{}, ErrAttachmentEnded
	}
	if end == nil {
		return period.EndAt(now)
	}
	if !exactBoundary(*end) || end.Before(period.From) || end.Before(instant) || period.To != nil && end.After(*period.To) {
		return AssignmentPeriod{}, ErrInvalid
	}
	return NewAssignmentPeriod(period.From, end)
}
