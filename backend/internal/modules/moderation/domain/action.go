package domain

import (
	"errors"
	"strings"
	"time"
)

// Operator actions. REVIEW triages; INVALIDATE and BLOCK apply the
// reviewed eligibility change; RESOLVE closes a substantiated case
// without a separate eligibility write; DISMISS rejects the report.
// Every action appends audit history: nothing here edits a fact.
const (
	ActionReview     = "REVIEW"
	ActionInvalidate = "INVALIDATE"
	ActionBlock      = "BLOCK"
	ActionResolve    = "RESOLVE"
	ActionDismiss    = "DISMISS"
)

var (
	ErrInvalidAction   = errors.New("moderation: invalid action")
	ErrUnknownAction   = errors.New("moderation: unknown action")
	ErrUnauthorized    = errors.New("moderation: operator identity required")
	ErrActionForbidden = errors.New("moderation: action not allowed for target")
	ErrTerminalCase    = errors.New("moderation: case already closed")
	// ErrBadTransition marks out-of-order triage (e.g. re-reviewing an
	// already triaged case).
	ErrBadTransition = errors.New("moderation: transition not allowed")
)

// ActionParams carries one audited operator intent. ActorID names the
// accountable operator (never empty, never anonymous); the restricted
// CLI supplies it from operator access, not from any public request.
type ActionParams struct {
	ID            string
	CaseID        string
	ActorID       string
	Action        string
	Reason        string
	OccurredAt    time.Time
	PolicyVersion string
}

// Action is one immutable audit record behind a case status move.
// Appeals and reversals arrive as new actions, never edits.
type Action struct {
	ID            string
	CaseID        string
	ActorID       string
	Action        string
	Reason        string
	OccurredAt    time.Time
	PolicyVersion string
}

// ModerationActionRecorded is the domain event recorded at each action.
// Downstream recomputation consumes it; the payload carries identifiers
// only (B-BR-011).
type ModerationActionRecorded struct {
	ActionID   string
	CaseID     string
	Action     string
	OccurredAt time.Time
}

// Name derives the emitted event name.
func (e ModerationActionRecorded) Name() string { return "ModerationActionRecorded" }

// NewAction validates and freezes one audit record.
func NewAction(p ActionParams) (Action, ModerationActionRecorded, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.CaseID) == "" {
		return Action{}, ModerationActionRecorded{}, ErrInvalidAction
	}
	if strings.TrimSpace(p.ActorID) == "" {
		return Action{}, ModerationActionRecorded{}, ErrUnauthorized
	}
	switch p.Action {
	case ActionReview, ActionInvalidate, ActionBlock, ActionResolve, ActionDismiss:
	default:
		return Action{}, ModerationActionRecorded{}, ErrUnknownAction
	}
	reason := strings.TrimSpace(p.Reason)
	if reason == "" || len([]rune(reason)) > MaxReasonChars {
		return Action{}, ModerationActionRecorded{}, ErrBadReason
	}
	if p.OccurredAt.IsZero() {
		return Action{}, ModerationActionRecorded{}, ErrInvalidAction
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	a := Action{
		ID: p.ID, CaseID: p.CaseID, ActorID: strings.TrimSpace(p.ActorID),
		Action: p.Action, Reason: reason,
		OccurredAt: p.OccurredAt, PolicyVersion: p.PolicyVersion,
	}
	return a, ModerationActionRecorded{
		ActionID: a.ID, CaseID: a.CaseID, Action: a.Action,
		OccurredAt: a.OccurredAt,
	}, nil
}

// AllowedForTarget gates eligibility-changing actions by target type:
// invalidation applies to facts and evidence, blocking to contributors,
// triage and closure to any target (BUC-006, B-BR-012).
func AllowedForTarget(action, targetType string) bool {
	switch action {
	case ActionInvalidate:
		return targetType == TargetObservation || targetType == TargetDispute ||
			targetType == TargetEvidence
	case ActionBlock:
		return targetType == TargetContributor
	case ActionReview, ActionResolve, ActionDismiss:
		return targetType == TargetObservation || targetType == TargetDispute ||
			targetType == TargetContributor || targetType == TargetEvidence
	default:
		return false
	}
}

// NextStatus derives the case status move for an action from the current
// status. Closed cases never reopen: an appeal records a new case or a
// new reviewed fact, never an edit.
func NextStatus(action, current string) (string, error) {
	switch current {
	case StatusOpen, StatusInReview:
	default:
		return "", ErrTerminalCase
	}
	switch action {
	case ActionReview:
		if current != StatusOpen {
			return "", ErrBadTransition
		}
		return StatusInReview, nil
	case ActionInvalidate, ActionBlock, ActionResolve:
		return StatusResolved, nil
	case ActionDismiss:
		return StatusRejected, nil
	default:
		return "", ErrUnknownAction
	}
}
