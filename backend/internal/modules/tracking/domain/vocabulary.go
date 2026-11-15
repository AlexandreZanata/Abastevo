// Package domain models the pure organization tracking session (EMP-GPS-001):
// a session bound to one organization trip of its driver, opened and started
// only from the visible trip screen, its closed states, typed causes and
// actors, the device permission carried explicitly, recorded transitions and
// recording periods, the trip-follow rule (the trip never depends on the
// session) and the track quality classification. It holds no persistence,
// clock, authority, location provider, buffer or coordinate.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

// Closed failure vocabulary shared with Kotlin through session-v1.
var (
	ErrInvalid           = errors.New("tracking: invalid value")
	ErrScope             = errors.New("tracking: organization or trip scope denied")
	ErrForbidden         = errors.New("tracking: action by a member other than the session owner")
	ErrTrackingOff       = errors.New("tracking: organization tracking policy is off")
	ErrPersonalTrip      = errors.New("tracking: personal mode never feeds an organization session")
	ErrImplicitStart     = errors.New("tracking: recording starts only from the visible trip screen")
	ErrSessionExists     = errors.New("tracking: the trip already has a session")
	ErrInvalidTransition = errors.New("tracking: transition not allowed")
	ErrTripState         = errors.New("tracking: trip state does not allow this session command")
	ErrOutOfOrder        = errors.New("tracking: instant before the last recorded transition")
)

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseTrackingValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// State is a tracking session state of the canonical catalog (08-state-model §3).
type State string

const (
	NotStarted  State = "NOT_STARTED"
	Recording   State = "RECORDING"
	Suspended   State = "SUSPENDED"
	Interrupted State = "INTERRUPTED"
	Ended       State = "ENDED"
)

// ParseState accepts only an exact catalog token.
func ParseState(raw string) (State, error) {
	v := State(raw)
	switch v {
	case NotStarted, Recording, Suspended, Interrupted, Ended:
		return v, nil
	}
	return "", ErrInvalid
}

// Terminal reports ENDED.
func (s State) Terminal() bool { return s == Ended }

// Permission is the device location permission observed at a start, resume or reduction.
type Permission string

const (
	PermissionPrecise     Permission = "PRECISE"
	PermissionApproximate Permission = "APPROXIMATE"
	PermissionNone        Permission = "NONE"
)

// ParsePermission accepts only an exact permission token.
func ParsePermission(raw string) (Permission, error) {
	v := Permission(raw)
	switch v {
	case PermissionPrecise, PermissionApproximate, PermissionNone:
		return v, nil
	}
	return "", ErrInvalid
}

// Policy is the organization's tracking policy resolved at opening.
type Policy string

const (
	PolicyOff      Policy = "OFF"
	PolicyOptional Policy = "OPTIONAL"
	PolicyExpected Policy = "EXPECTED"
)

// ParsePolicy accepts only an exact policy token.
func ParsePolicy(raw string) (Policy, error) {
	v := Policy(raw)
	switch v {
	case PolicyOff, PolicyOptional, PolicyExpected:
		return v, nil
	}
	return "", ErrInvalid
}

// Origin is where an opening, start or resume request came from.
type Origin string

const (
	OriginTripScreen   Origin = "TRIP_SCREEN"
	OriginNotification Origin = "NOTIFICATION"
	OriginPersonal     Origin = "PERSONAL"
	OriginBoot         Origin = "BOOT"
	OriginWorker       Origin = "WORKER"
	OriginPush         Origin = "PUSH"
	OriginGeofence     Origin = "GEOFENCE"
	OriginServer       Origin = "SERVER"
)

// ParseOrigin accepts only an exact origin token.
func ParseOrigin(raw string) (Origin, error) {
	v := Origin(raw)
	switch v {
	case OriginTripScreen, OriginNotification, OriginPersonal, OriginBoot, OriginWorker, OriginPush, OriginGeofence, OriginServer:
		return v, nil
	}
	return "", ErrInvalid
}

// Cause is the typed reason recorded with a history entry.
type Cause string

const (
	CauseNotStarted          Cause = "NOT_STARTED"
	CausePermissionDenied    Cause = "PERMISSION_DENIED"
	CauseNotificationBlocked Cause = "NOTIFICATION_BLOCKED"
	CauseTripPaused          Cause = "TRIP_PAUSED"
	CauseLongStationary      Cause = "LONG_STATIONARY"
	CausePermissionRevoked   Cause = "PERMISSION_REVOKED"
	CauseProcessStopped      Cause = "PROCESS_STOPPED"
	CauseDeviceRestarted     Cause = "DEVICE_RESTARTED"
	CauseStoppedByDriver     Cause = "STOPPED_BY_DRIVER"
	CauseOwnerChanged        Cause = "OWNER_CHANGED"
	CausePrecisionReduced    Cause = "PRECISION_REDUCED"
	CauseTripCompleted       Cause = "TRIP_COMPLETED"
	CauseTripCancelled       Cause = "TRIP_CANCELLED"
	CauseMembershipEnded     Cause = "MEMBERSHIP_ENDED"
	CauseDeviceRevoked       Cause = "DEVICE_REVOKED"
)

// ParseCause accepts only an exact cause token.
func ParseCause(raw string) (Cause, error) {
	v := Cause(raw)
	switch v {
	case CauseNotStarted, CausePermissionDenied, CauseNotificationBlocked, CauseTripPaused, CauseLongStationary, CausePermissionRevoked, CauseProcessStopped, CauseDeviceRestarted, CauseStoppedByDriver, CauseOwnerChanged, CausePrecisionReduced, CauseTripCompleted, CauseTripCancelled, CauseMembershipEnded, CauseDeviceRevoked:
		return v, nil
	}
	return "", ErrInvalid
}

// Actor is the kind of actor of a history entry, fixed by command and cause.
type Actor string

const (
	ActorDriver Actor = "DRIVER"
	ActorDevice Actor = "DEVICE"
	ActorTrip   Actor = "TRIP"
	ActorServer Actor = "SERVER"
)

// ParseActor accepts only an exact actor token.
func ParseActor(raw string) (Actor, error) {
	v := Actor(raw)
	switch v {
	case ActorDriver, ActorDevice, ActorTrip, ActorServer:
		return v, nil
	}
	return "", ErrInvalid
}

// Quality is the track quality of the canonical catalog (08-state-model §3).
type Quality string

const (
	QualityPrecise      Quality = "PRECISE"
	QualityApproximate  Quality = "APPROXIMATE"
	QualityInsufficient Quality = "INSUFFICIENT"
)

// ParseQuality accepts only an exact quality token.
func ParseQuality(raw string) (Quality, error) {
	v := Quality(raw)
	switch v {
	case QualityPrecise, QualityApproximate, QualityInsufficient:
		return v, nil
	}
	return "", ErrInvalid
}
