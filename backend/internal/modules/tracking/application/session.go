package application

import (
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// tripEvent is one stored state change of the trip after its start.
type tripEvent struct {
	at    time.Time
	state trip.State
}

// timeline is the stored trip's state changes in revision order (its
// lifecycle rows, then its finish), proved consistent with the stored state.
func timeline(record tripapp.Record) ([]tripEvent, error) {
	events := make([]tripEvent, 0, len(record.Transitions)+1)
	state, last := trip.InProgress, record.Declared.OccurredAt
	for _, t := range record.Transitions {
		at := t.Declared.OccurredAt
		if t.From != state || at.Before(last) || t.To != trip.Paused && t.To != trip.InProgress && t.To != trip.Cancelled {
			return nil, ErrUnavailable
		}
		events = append(events, tripEvent{at: at, state: t.To})
		state, last = t.To, at
	}
	if record.Finish != nil {
		at := record.Finish.Declared.OccurredAt
		if state == trip.Cancelled || at.Before(last) {
			return nil, ErrUnavailable
		}
		events = append(events, tripEvent{at: at, state: trip.Completed})
		state = trip.Completed
	}
	if state != record.State {
		return nil, ErrUnavailable
	}
	return events, nil
}

// TripFactsAt derives the stored trip's facts at a declared instant
// (EDR-278): IN_PROGRESS from its start, then the state of its latest
// pause, resume, cancellation or finish at or before the instant. An instant
// before the trip started is ErrMalformed; an inconsistent stored trip is
// ErrUnavailable. The driver is the member responsible for the trip at the
// instant: its author, or the receiver of a handover (EDR-279, EMP-TRIP-012
// EDR-256), so a giving member's session collects nothing after it.
func TripFactsAt(scope kernel.OrgScope, record tripapp.Record, at time.Time) (domain.TripFacts, error) {
	ref, err := trip.NewTripRef(scope, record.ID)
	if err != nil {
		return domain.TripFacts{}, ErrUnavailable
	}
	responsible, _ := record.ResponsibleAt(at)
	driver, err := trip.NewMembershipRef(scope, responsible)
	if err != nil {
		return domain.TripFacts{}, ErrUnavailable
	}
	events, err := timeline(record)
	if err != nil {
		return domain.TripFacts{}, err
	}
	if at.Before(record.Declared.OccurredAt) {
		return domain.TripFacts{}, ErrMalformed
	}
	state := trip.InProgress
	for _, e := range events {
		if !at.Before(e.at) {
			state = e.state
		}
	}
	return domain.TripFacts{Ref: ref, Driver: driver, State: state}, nil
}

// Bounds are the stored trip's running intervals: from its start, without
// its paused intervals, until its finish or cancellation.
func Bounds(record tripapp.Record) TripBounds {
	b := TripBounds{Start: record.Declared.OccurredAt}
	events, err := timeline(record)
	if err != nil {
		// An inconsistent trip runs nowhere.
		end := b.Start
		b.Finish = &end
		return b
	}
	var paused *time.Time
	for _, e := range events {
		at := e.at
		switch e.state {
		case trip.Paused:
			paused = &at
		case trip.InProgress:
			if paused != nil {
				b.Pauses = append(b.Pauses, Pause{From: *paused, To: &at})
				paused = nil
			}
		case trip.Completed, trip.Cancelled:
			b.Finish = &at
		}
	}
	if paused != nil {
		b.Pauses = append(b.Pauses, Pause{From: *paused, To: b.Finish})
	}
	return b
}

// Running reports whether the trip was IN_PROGRESS at an instant.
func (b TripBounds) Running(at time.Time) bool {
	if at.Before(b.Start) || b.Finish != nil && !at.Before(*b.Finish) {
		return false
	}
	for _, p := range b.Pauses {
		if !at.Before(p.From) && (p.To == nil || at.Before(*p.To)) {
			return false
		}
	}
	return true
}

// FollowTrip applies to a stored session, through the EMP-GPS-001 FOLLOW
// rule, every stored trip pause, cancellation and finish from the session's
// last transition up to (not including) an operation's instant: a pause
// suspends a recording session, a cancellation or finish ends it. The
// operation itself decides at its own instant.
func FollowTrip(scope kernel.OrgScope, record tripapp.Record, s domain.Session, before time.Time) (domain.Session, error) {
	events, err := timeline(record)
	if err != nil {
		return domain.Session{}, err
	}
	history := s.History()
	if len(history) == 0 {
		return domain.Session{}, ErrUnavailable
	}
	last := history[len(history)-1].At()
	for _, e := range events {
		if e.at.Before(last) || !e.at.Before(before) {
			continue
		}
		facts, err := TripFactsAt(scope, record, e.at)
		if err != nil {
			return domain.Session{}, ErrUnavailable
		}
		if s, err = s.Follow(scope, facts, e.at); err != nil {
			return domain.Session{}, ErrUnavailable
		}
	}
	return s, nil
}

// replayStates are the trip states a stored row's cause implies; the stored
// history was decided on real facts, so its replay supplies them.
var replayStates = map[domain.Cause]trip.State{
	domain.CauseTripPaused:    trip.Paused,
	domain.CauseTripCompleted: trip.Completed,
	domain.CauseTripCancelled: trip.Cancelled,
}

// Rebuild replays a stored session through the EMP-GPS-001 domain and
// proves the replay reproduces every stored row (B-BR-EMP-430).
func Rebuild(scope kernel.OrgScope, record SessionRecord) (domain.Session, error) {
	rows := record.Transitions
	if len(rows) == 0 || !rows[0].At.Equal(record.OpenedAt) {
		return domain.Session{}, ErrUnavailable
	}
	ref, err := domain.NewSessionRef(scope, record.ID)
	if err != nil {
		return domain.Session{}, ErrUnavailable
	}
	tripRef, err := trip.NewTripRef(scope, record.TripID)
	if err != nil {
		return domain.Session{}, ErrUnavailable
	}
	owner, err := trip.NewMembershipRef(scope, record.OwnerMembershipID)
	if err != nil {
		return domain.Session{}, ErrUnavailable
	}
	facts := func(state trip.State) domain.TripFacts {
		return domain.TripFacts{Ref: tripRef, Driver: owner, State: state}
	}
	s, err := domain.Open(scope, domain.OpenInput{Session: ref, Trip: facts(trip.InProgress), Author: owner, Origin: domain.OriginTripScreen, Policy: record.Policy, At: rows[0].At})
	if err != nil {
		return domain.Session{}, ErrUnavailable
	}
	for _, row := range rows[1:] {
		previous := s.State()
		attempt := row.Cause == domain.CausePermissionDenied || row.Cause == domain.CauseNotificationBlocked && row.State == previous
		if row.Actor == domain.ActorDriver && (row.Cause == "" || attempt) {
			action := domain.DriverAction{Trip: facts(trip.InProgress), Author: owner, Origin: domain.OriginTripScreen,
				Device: domain.Device{Permission: row.Permission, NotificationVisible: row.Cause != domain.CauseNotificationBlocked}, At: row.At}
			if previous == domain.NotStarted {
				s, err = s.Start(scope, action)
			} else {
				s, err = s.Resume(scope, action)
			}
		} else {
			state, ok := replayStates[row.Cause]
			if !ok {
				state = trip.InProgress
			}
			s, err = command(s, scope, domain.Event{Trip: facts(state), Cause: row.Cause, Author: owner, At: row.At})
		}
		if err != nil {
			return domain.Session{}, ErrUnavailable
		}
	}
	history := s.History()
	if len(history) != len(rows) {
		return domain.Session{}, ErrUnavailable
	}
	for i, h := range history {
		r := rows[i]
		if r.Seq != i+1 || r.State != h.State() || r.Cause != h.Cause() || r.Actor != h.Actor() || r.Permission != h.Permission() || !r.At.Equal(h.At()) {
			return domain.Session{}, ErrUnavailable
		}
	}
	return s, nil
}

// command dispatches a stored event by its cause to the domain command that
// owns that cause.
func command(s domain.Session, scope kernel.OrgScope, e domain.Event) (domain.Session, error) {
	switch e.Cause {
	case domain.CauseTripPaused, domain.CauseLongStationary:
		return s.Suspend(scope, e)
	case domain.CausePrecisionReduced:
		return s.ReducePrecision(scope, e)
	case domain.CauseTripCompleted, domain.CauseTripCancelled, domain.CauseMembershipEnded, domain.CauseDeviceRevoked:
		return s.End(scope, e)
	}
	return s.Interrupt(scope, e)
}

// Decide applies one session operation with the trip facts at its declared
// instant to the trip's stored session (nil when it has none); a first start
// opens the session under the policy in force (B-BR-EMP-428/429).
func Decide(scope kernel.OrgScope, in SessionInput, facts domain.TripFacts, existing *domain.Session, policy PolicyVersion) (domain.Session, error) {
	author, err := trip.NewMembershipRef(scope, in.AuthorMembershipID)
	if err != nil {
		return domain.Session{}, domain.ErrInvalid
	}
	action := domain.DriverAction{Trip: facts, Author: author, Origin: in.Origin, Device: domain.Device{Permission: in.Permission, NotificationVisible: in.NotificationVisible}, At: in.Declared}
	event := domain.Event{Trip: facts, Cause: in.Cause, Author: author, At: in.Declared}
	if in.Type == StartOperationType {
		if existing != nil && existing.Reference().ID() == in.SessionID {
			return existing.Start(scope, action)
		}
		if _, err := domain.ParsePolicy(string(policy.Policy)); err != nil || policy.Policy != domain.PolicyOff && policy.Revision < 1 {
			return domain.Session{}, ErrUnavailable
		}
		ref, err := domain.NewSessionRef(scope, in.SessionID)
		if err != nil {
			return domain.Session{}, domain.ErrInvalid
		}
		opened, err := domain.Open(scope, domain.OpenInput{Session: ref, Trip: facts, Author: author, Origin: in.Origin, Policy: policy.Policy, At: in.Declared, Existing: existing})
		if err != nil {
			return domain.Session{}, err
		}
		return opened.Start(scope, action)
	}
	if existing == nil || existing.Reference().ID() != in.SessionID {
		return domain.Session{}, ErrUnavailable
	}
	switch in.Type {
	case ResumeOperationType:
		return existing.Resume(scope, action)
	case SuspendOperationType:
		return existing.Suspend(scope, event)
	case InterruptOperationType:
		return existing.Interrupt(scope, event)
	case ReduceOperationType:
		return existing.ReducePrecision(scope, event)
	case EndOperationType:
		return existing.End(scope, event)
	}
	return domain.Session{}, ErrUnavailable
}

// Refusal maps a refusal of Decide to its final kind (EDR-278): ErrPolicyOff
// (REJECTED FORBIDDEN), domain.ErrForbidden (whole-request FORBIDDEN) or
// ErrMalformed; anything else is ErrUnavailable.
func Refusal(err error) error {
	switch {
	case errors.Is(err, domain.ErrTrackingOff):
		return ErrPolicyOff
	case errors.Is(err, domain.ErrForbidden):
		return domain.ErrForbidden
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrScope), errors.Is(err, domain.ErrPersonalTrip), errors.Is(err, domain.ErrImplicitStart),
		errors.Is(err, domain.ErrSessionExists), errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrTripState), errors.Is(err, domain.ErrOutOfOrder),
		errors.Is(err, ErrMalformed):
		return ErrMalformed
	}
	return ErrUnavailable
}

// Change is what an accepted operation stores beyond the previous record:
// the session row when it opens, then the new transition rows.
func Change(previous *SessionRecord, next domain.Session, in SessionInput, policy PolicyVersion) (SessionChange, error) {
	history := next.History()
	from := 0
	change := SessionChange{}
	if previous == nil {
		if policy.Revision < 1 || next.Policy() != policy.Policy || len(history) < 2 {
			return SessionChange{}, ErrUnavailable
		}
		change.Open = true
		change.Session = SessionRecord{ID: next.Reference().ID(), TripID: next.Trip().ID(), OwnerMembershipID: next.Owner().ID(), OperationID: in.OperationID,
			Policy: next.Policy(), PolicyRevision: policy.Revision, OpenedAt: history[0].At()}
	} else {
		from = len(previous.Transitions)
		if previous.ID != next.Reference().ID() || len(history) <= from {
			return SessionChange{}, ErrUnavailable
		}
		change.Session = previous.Copy()
	}
	for i, h := range history[from:] {
		change.Transitions = append(change.Transitions, StoredTransition{Seq: from + i + 1, State: h.State(), Cause: h.Cause(), Actor: h.Actor(), Permission: h.Permission(), At: h.At(), OperationID: in.OperationID})
	}
	return change, nil
}
