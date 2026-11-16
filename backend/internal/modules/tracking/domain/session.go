package domain

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// SessionRef is a scoped session identity; its shape proves no existence.
type SessionRef struct {
	redacted
	org kernel.OrgID
	id  string
}

func (r SessionRef) ID() string                   { return r.id }
func (r SessionRef) OrganizationID() kernel.OrgID { return r.org }

// NewSessionRef binds a canonical session identity to the scope.
func NewSessionRef(scope kernel.OrgScope, id string) (SessionRef, error) {
	if scope.Validate() != nil {
		return SessionRef{}, ErrScope
	}
	if v, e := kernel.ParseOrgID(id); e != nil || v.String() != id {
		return SessionRef{}, ErrInvalid
	}
	return SessionRef{org: scope.OrgID(), id: id}, nil
}

// owned checks a constructed reference first, then its organization.
func owned(scope kernel.OrgScope, org kernel.OrgID, id string) error {
	if id == "" || org == (kernel.OrgID{}) {
		return ErrInvalid
	}
	if org != scope.OrgID() {
		return ErrScope
	}
	return nil
}

// TripFacts are the bound trip's current facts, supplied by the trip owner
// with every command; the session never stores a trip state of its own.
type TripFacts struct {
	redacted
	Ref    trip.TripRef
	Driver trip.MembershipRef // the member who currently runs the trip
	State  trip.State
}

func (f TripFacts) check(scope kernel.OrgScope) error {
	if e := owned(scope, f.Ref.OrganizationID(), f.Ref.ID()); e != nil {
		return e
	}
	if e := owned(scope, f.Driver.OrganizationID(), f.Driver.ID()); e != nil {
		return e
	}
	if _, e := trip.ParseState(string(f.State)); e != nil {
		return ErrInvalid
	}
	return nil
}

// Device is what the device reports at a start or resume.
type Device struct {
	redacted
	Permission          Permission
	NotificationVisible bool
}

// OpenInput opens the session of a trip when the driver starts it.
type OpenInput struct {
	redacted
	Session  SessionRef
	Trip     TripFacts
	Author   trip.MembershipRef
	Origin   Origin
	Policy   Policy
	At       time.Time
	Existing *Session // the trip's current session, if it already has one
}

// DriverAction is the owner's start or resume on the visible trip screen.
type DriverAction struct {
	redacted
	Trip   TripFacts
	Author trip.MembershipRef
	Origin Origin
	Device Device
	At     time.Time
}

// Event is a suspension, interruption, reduction or end with its typed cause.
// Author is consulted only for the driver's own stop (STOPPED_BY_DRIVER).
type Event struct {
	redacted
	Trip   TripFacts
	Cause  Cause
	Author trip.MembershipRef
	At     time.Time
}

// Transition is one recorded history entry.
type Transition struct {
	redacted
	state      State
	cause      Cause
	actor      Actor
	permission Permission
	at         time.Time
}

func (t Transition) State() State           { return t.state }
func (t Transition) Cause() Cause           { return t.cause }
func (t Transition) Actor() Actor           { return t.actor }
func (t Transition) Permission() Permission { return t.permission }
func (t Transition) At() time.Time          { return t.at }

// Period is one half-open recording period with its precision.
type Period struct {
	redacted
	start     time.Time
	end       *time.Time
	precision Permission
}

func (p Period) Start() time.Time { return p.start }

// End returns the end of a closed period; an open period has none.
func (p Period) End() (time.Time, bool) {
	if p.end == nil {
		return time.Time{}, false
	}
	return *p.end, true
}
func (p Period) Precision() Permission { return p.precision }

// Session is an immutable tracking session; only Open and its commands build one.
type Session struct {
	redacted
	ref     SessionRef
	trip    trip.TripRef
	owner   trip.MembershipRef
	policy  Policy
	history []Transition
}

// validInstant refuses the unset instant and anything outside years 1..9999.
func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

// Open creates the NOT_STARTED session of an IN_PROGRESS trip (B-BR-EMP-421):
// scope, structure and ownership first, then the existing session's binding,
// then origin, driver, policy, trip state and the one-session rule.
func Open(scope kernel.OrgScope, in OpenInput) (Session, error) {
	if scope.Validate() != nil {
		return Session{}, ErrScope
	}
	if e := owned(scope, in.Session.org, in.Session.id); e != nil {
		return Session{}, e
	}
	if e := in.Trip.check(scope); e != nil {
		return Session{}, e
	}
	if e := owned(scope, in.Author.OrganizationID(), in.Author.ID()); e != nil {
		return Session{}, e
	}
	if _, e := ParseOrigin(string(in.Origin)); e != nil {
		return Session{}, e
	}
	if _, e := ParsePolicy(string(in.Policy)); e != nil {
		return Session{}, e
	}
	if !validInstant(in.At) {
		return Session{}, ErrInvalid
	}
	if x := in.Existing; x != nil {
		if e := owned(scope, x.ref.org, x.ref.id); e != nil {
			return Session{}, e
		}
		if x.trip != in.Trip.Ref {
			return Session{}, ErrScope
		}
	}
	if e := explicitOrigin(in.Origin); e != nil {
		return Session{}, e
	}
	switch {
	case in.Author != in.Trip.Driver:
		return Session{}, ErrForbidden
	case in.Policy == PolicyOff:
		return Session{}, ErrTrackingOff
	case in.Trip.State != trip.InProgress:
		return Session{}, ErrTripState
	case in.Existing != nil:
		return Session{}, ErrSessionExists
	}
	return Session{ref: in.Session, trip: in.Trip.Ref, owner: in.Author, policy: in.Policy,
		history: []Transition{{state: NotStarted, cause: CauseNotStarted, actor: ActorDriver, at: in.At}}}, nil
}

// explicitOrigin admits only the visible enterprise trip screen (BR-GPS-002).
func explicitOrigin(o Origin) error {
	switch o {
	case OriginTripScreen:
		return nil
	case OriginPersonal:
		return ErrPersonalTrip
	}
	return ErrImplicitStart
}

func (s Session) Reference() SessionRef     { return s.ref }
func (s Session) Trip() trip.TripRef        { return s.trip }
func (s Session) Owner() trip.MembershipRef { return s.owner }
func (s Session) Policy() Policy            { return s.policy }

func (s Session) last() Transition {
	if len(s.history) == 0 {
		return Transition{}
	}
	return s.history[len(s.history)-1]
}

// State is the state of the last history entry.
func (s Session) State() State { return s.last().state }

// Cause is the cause of the last history entry; empty after a successful start or resume.
func (s Session) Cause() Cause { return s.last().cause }

// Precision is the precision of the current recording period; empty when not recording.
func (s Session) Precision() Permission {
	if s.State() != Recording {
		return ""
	}
	p := s.RecordingPeriods()
	return p[len(p)-1].precision
}

// Coarse reports whether any recording period was approximate.
func (s Session) Coarse() bool {
	for _, p := range s.RecordingPeriods() {
		if p.precision == PermissionApproximate {
			return true
		}
	}
	return false
}

// History returns a copy of every recorded entry.
func (s Session) History() []Transition {
	if s.history == nil {
		return nil
	}
	return append([]Transition(nil), s.history...)
}

// RecordingPeriods derives the half-open recording periods from the history;
// a reduction closes the precise period and opens an approximate one.
func (s Session) RecordingPeriods() []Period {
	var out []Period
	for i, h := range s.history {
		wasRecording := i > 0 && s.history[i-1].state == Recording
		if wasRecording && (h.state != Recording || h.cause == CausePrecisionReduced) {
			end := h.at
			out[len(out)-1].end = &end
		}
		if h.state == Recording && (!wasRecording || h.cause == CausePrecisionReduced) {
			out = append(out, Period{start: h.at, precision: h.permission})
		}
	}
	return out
}

// check validates the scope, the receiver and the bound trip's facts.
func (s Session) check(scope kernel.OrgScope, facts TripFacts) error {
	if scope.Validate() != nil {
		return ErrScope
	}
	if e := owned(scope, s.ref.org, s.ref.id); e != nil {
		return e
	}
	if e := facts.check(scope); e != nil {
		return e
	}
	if facts.Ref != s.trip {
		return ErrScope
	}
	return nil
}

// record appends one entry to a copy of the history; instants never go back.
func (s Session) record(t Transition) (Session, error) {
	if t.at.Before(s.last().at) {
		return Session{}, ErrOutOfOrder
	}
	next := s
	next.history = make([]Transition, len(s.history), len(s.history)+1)
	copy(next.history, s.history)
	next.history = append(next.history, t)
	return next, nil
}

// Start begins recording from NOT_STARTED (B-BR-EMP-422).
func (s Session) Start(scope kernel.OrgScope, a DriverAction) (Session, error) {
	return s.driverAction(scope, a, NotStarted)
}

// Resume records again from SUSPENDED or INTERRUPTED (B-BR-EMP-422).
func (s Session) Resume(scope kernel.OrgScope, a DriverAction) (Session, error) {
	return s.driverAction(scope, a, Suspended, Interrupted)
}

// driverAction checks scope, structure, the explicit origin, the owner, the
// from-state, the trip state and the instant order; a refused permission or a
// hidden notification is recorded as the attempt's cause and keeps the state.
func (s Session) driverAction(scope kernel.OrgScope, a DriverAction, from ...State) (Session, error) {
	if e := s.check(scope, a.Trip); e != nil {
		return Session{}, e
	}
	if e := owned(scope, a.Author.OrganizationID(), a.Author.ID()); e != nil {
		return Session{}, e
	}
	if _, e := ParseOrigin(string(a.Origin)); e != nil {
		return Session{}, e
	}
	if _, e := ParsePermission(string(a.Device.Permission)); e != nil {
		return Session{}, e
	}
	if !validInstant(a.At) {
		return Session{}, ErrInvalid
	}
	if e := explicitOrigin(a.Origin); e != nil {
		return Session{}, e
	}
	if a.Author != s.owner || a.Trip.Driver != s.owner {
		return Session{}, ErrForbidden
	}
	state := s.State()
	if !contains(from, state) {
		return Session{}, ErrInvalidTransition
	}
	if a.Trip.State != trip.InProgress {
		return Session{}, ErrTripState
	}
	t := Transition{state: Recording, actor: ActorDriver, permission: a.Device.Permission, at: a.At}
	switch {
	case a.Device.Permission == PermissionNone:
		t.state, t.cause = state, CausePermissionDenied
	case !a.Device.NotificationVisible:
		t.state, t.cause = state, CauseNotificationBlocked
	}
	return s.record(t)
}

type command int

const (
	commandSuspend command = iota
	commandInterrupt
	commandReduce
	commandEnd
)

// rule is the closed meaning of one event cause.
type rule struct {
	command command
	to      State
	actor   Actor
	trip    []trip.State // trip states that allow it
}

var (
	inProgress = []trip.State{trip.InProgress}
	started    = []trip.State{trip.InProgress, trip.Paused, trip.Completed, trip.Cancelled}
	rules      = map[Cause]rule{
		CauseTripPaused:          {commandSuspend, Suspended, ActorTrip, []trip.State{trip.Paused}},
		CauseLongStationary:      {commandSuspend, Suspended, ActorDevice, inProgress},
		CausePermissionRevoked:   {commandInterrupt, Interrupted, ActorDevice, inProgress},
		CauseNotificationBlocked: {commandInterrupt, Interrupted, ActorDevice, inProgress},
		CauseProcessStopped:      {commandInterrupt, Interrupted, ActorDevice, inProgress},
		CauseDeviceRestarted:     {commandInterrupt, Interrupted, ActorDevice, inProgress},
		CauseStoppedByDriver:     {commandInterrupt, Interrupted, ActorDriver, inProgress},
		CauseOwnerChanged:        {commandInterrupt, Interrupted, ActorDevice, inProgress},
		CausePrecisionReduced:    {commandReduce, Recording, ActorDevice, inProgress},
		CauseTripCompleted:       {commandEnd, Ended, ActorTrip, []trip.State{trip.Completed}},
		CauseTripCancelled:       {commandEnd, Ended, ActorTrip, []trip.State{trip.Cancelled}},
		CauseMembershipEnded:     {commandEnd, Ended, ActorServer, started},
		CauseDeviceRevoked:       {commandEnd, Ended, ActorServer, started},
	}
)

// Suspend moves RECORDING to SUSPENDED (B-BR-EMP-423).
func (s Session) Suspend(scope kernel.OrgScope, e Event) (Session, error) {
	return s.event(scope, e, commandSuspend)
}

// Interrupt moves RECORDING to INTERRUPTED (B-BR-EMP-423).
func (s Session) Interrupt(scope kernel.OrgScope, e Event) (Session, error) {
	return s.event(scope, e, commandInterrupt)
}

// ReducePrecision keeps RECORDING with approximate precision (B-BR-EMP-423).
func (s Session) ReducePrecision(scope kernel.OrgScope, e Event) (Session, error) {
	return s.event(scope, e, commandReduce)
}

// End moves any open state to ENDED (B-BR-EMP-424).
func (s Session) End(scope kernel.OrgScope, e Event) (Session, error) {
	return s.event(scope, e, commandEnd)
}

// event checks scope, structure, the cause of this command, the driver's own
// stop, the from-state, the trip state and the instant order, in that order.
func (s Session) event(scope kernel.OrgScope, e Event, cmd command) (Session, error) {
	if err := s.check(scope, e.Trip); err != nil {
		return Session{}, err
	}
	if _, err := ParseCause(string(e.Cause)); err != nil {
		return Session{}, err
	}
	if !validInstant(e.At) {
		return Session{}, ErrInvalid
	}
	r, ok := rules[e.Cause]
	if !ok || r.command != cmd {
		return Session{}, ErrInvalid
	}
	if r.actor == ActorDriver {
		if err := owned(scope, e.Author.OrganizationID(), e.Author.ID()); err != nil {
			return Session{}, err
		}
		if e.Author != s.owner {
			return Session{}, ErrForbidden
		}
	}
	if !s.allows(cmd) {
		return Session{}, ErrInvalidTransition
	}
	if !contains(r.trip, e.Trip.State) {
		return Session{}, ErrTripState
	}
	return s.apply(e.Cause, r, e.At)
}

func (s Session) allows(cmd command) bool {
	switch cmd {
	case commandEnd:
		return s.State() != Ended
	case commandReduce:
		return s.Precision() == PermissionPrecise
	}
	return s.State() == Recording
}

func contains[T comparable](values []T, value T) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func (s Session) apply(c Cause, r rule, at time.Time) (Session, error) {
	t := Transition{state: r.to, cause: c, actor: r.actor, at: at}
	if r.command == commandReduce {
		t.permission = PermissionApproximate
	}
	return s.record(t)
}

// Follow maps the trip's current state onto the session (B-BR-EMP-425) and
// never refuses with INVALID_TRANSITION: PAUSED suspends a recording session,
// COMPLETED and CANCELLED end any open session, IN_PROGRESS changes nothing.
func (s Session) Follow(scope kernel.OrgScope, facts TripFacts, at time.Time) (Session, error) {
	if e := s.check(scope, facts); e != nil {
		return Session{}, e
	}
	if !validInstant(at) {
		return Session{}, ErrInvalid
	}
	var c Cause
	switch facts.State {
	case trip.InProgress:
		return s, nil
	case trip.Paused:
		if s.State() != Recording {
			return s, nil
		}
		c = CauseTripPaused
	case trip.Completed, trip.Cancelled:
		if s.State() == Ended {
			return s, nil
		}
		c = CauseTripCompleted
		if facts.State == trip.Cancelled {
			c = CauseTripCancelled
		}
	default:
		return Session{}, ErrTripState
	}
	return s.apply(c, rules[c], at)
}

// Collecting reports whether a fix may be collected now and its precision
// (B-BR-EMP-425, BR-GPS-001/007): only for the bound trip IN_PROGRESS, still
// run by the owner, while the session is RECORDING.
func (s Session) Collecting(scope kernel.OrgScope, facts TripFacts) (Permission, bool, error) {
	if e := s.check(scope, facts); e != nil {
		return "", false, e
	}
	if facts.State != trip.InProgress || facts.Driver != s.owner || s.State() != Recording {
		return "", false, nil
	}
	return s.Precision(), true, nil
}
