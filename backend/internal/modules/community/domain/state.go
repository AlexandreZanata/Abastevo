package domain

import (
	"errors"
	"strings"
	"time"
)

// Validation states. Freshness (B-BR-008), confidence, disputes and expiry
// are separate concepts that never appear here: this machine tracks
// admissibility only, and worker retries never regress it.
const (
	StateReceived   = "RECEIVED"
	StateValidating = "VALIDATING"
	StateValidated  = "VALIDATED"
	StateRejected   = "REJECTED"
)

// Actors allowed on transitions. Moderation invalidation is privileged;
// everything else is worker or system driven.
const (
	ActorWorker    = "worker"
	ActorModerator = "moderator"
)

var (
	ErrBadTransition = errors.New("community: transition not allowed")
	ErrBadActor      = errors.New("community: actor not allowed for transition")
	ErrBadReasons    = errors.New("community: stable reason codes required")
	ErrBadCase       = errors.New("community: moderation case reference required")
	ErrBadCommand    = errors.New("community: persisted command reference required")
)

// Decision is one immutable admissibility event. State itself is a
// projection over the decision log; nothing ever edits a recorded decision.
type Decision struct {
	ObservationID string
	Sequence      int64
	FromState     string
	ToState       string
	ReasonCodes   []string
	PolicyVersion string
	OccurredAt    time.Time
	Actor         string
	CaseRef       string
	CommandRef    string
}

// EventName derives the emitted event for a transition.
func (d Decision) EventName() string {
	switch d.FromState + ">" + d.ToState {
	case StateReceived + ">" + StateValidating:
		return "ValidationStarted"
	case StateValidating + ">" + StateValidated:
		return "ObservationValidated"
	case StateValidating + ">" + StateRejected:
		return "ObservationRejected"
	case StateValidated + ">" + StateRejected:
		return "ObservationInvalidated"
	default:
		return ""
	}
}

// ClaimValidation moves RECEIVED to VALIDATING. It requires the persisted
// command/job reference that proves a worker (not an HTTP handler) owns
// the claim, and a worker actor. State arrives separately because it is a
// projection over decisions, not a fact field.
func ClaimValidation(obs Observation, state, commandRef, actor string, at time.Time, sequence int64) (Decision, error) {
	if state != StateReceived {
		return Decision{}, ErrBadTransition
	}
	if actor != ActorWorker {
		return Decision{}, ErrBadActor
	}
	if strings.TrimSpace(commandRef) == "" {
		return Decision{}, ErrBadCommand
	}
	return Decision{
		ObservationID: obs.ID, Sequence: sequence,
		FromState: StateReceived, ToState: StateValidating,
		PolicyVersion: obs.PolicyVersion, OccurredAt: at,
		Actor: actor, CommandRef: commandRef,
	}, nil
}

// Admit moves VALIDATING to VALIDATED when admissibility checks pass.
func Admit(obs Observation, state string, actor string, at time.Time, sequence int64) (Decision, error) {
	if state != StateValidating {
		return Decision{}, ErrBadTransition
	}
	if actor != ActorWorker {
		return Decision{}, ErrBadActor
	}
	return Decision{
		ObservationID: obs.ID, Sequence: sequence,
		FromState: StateValidating, ToState: StateValidated,
		PolicyVersion: obs.PolicyVersion, OccurredAt: at, Actor: actor,
	}, nil
}

// Reject moves VALIDATING to REJECTED with stable reason codes.
func Reject(obs Observation, state string, actor string, reasons []string, at time.Time, sequence int64) (Decision, error) {
	if state != StateValidating {
		return Decision{}, ErrBadTransition
	}
	if actor != ActorWorker {
		return Decision{}, ErrBadActor
	}
	if len(reasons) == 0 {
		return Decision{}, ErrBadReasons
	}
	for _, r := range reasons {
		if strings.TrimSpace(r) == "" {
			return Decision{}, ErrBadReasons
		}
	}
	return Decision{
		ObservationID: obs.ID, Sequence: sequence,
		FromState: StateValidating, ToState: StateRejected,
		ReasonCodes:   append([]string{}, reasons...),
		PolicyVersion: obs.PolicyVersion, OccurredAt: at, Actor: actor,
	}, nil
}

// Invalidate moves VALIDATED to REJECTED through audited moderation: a
// privileged actor, a case reference and reasons. It adds a revocation
// event; recomputation of affected keys happens downstream, never here.
func Invalidate(obs Observation, state string, actor, caseRef string, reasons []string, at time.Time, sequence int64) (Decision, error) {
	if state != StateValidated {
		return Decision{}, ErrBadTransition
	}
	if actor != ActorModerator {
		return Decision{}, ErrBadActor
	}
	if strings.TrimSpace(caseRef) == "" {
		return Decision{}, ErrBadCase
	}
	if len(reasons) == 0 {
		return Decision{}, ErrBadReasons
	}
	return Decision{
		ObservationID: obs.ID, Sequence: sequence,
		FromState: StateValidated, ToState: StateRejected,
		ReasonCodes:   append([]string{}, reasons...),
		PolicyVersion: obs.PolicyVersion, OccurredAt: at,
		Actor: actor, CaseRef: caseRef,
	}, nil
}
