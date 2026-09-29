package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Moderation policy version frozen with the case queue. Changes require
// a new version, never silent reinterpretation.
const PolicyV1 = "moderation-v1"

// Review targets. OBSERVATION and DISPUTE name community facts by UUID;
// EVIDENCE names an upload/object by UUID; CONTRIBUTOR names an
// attribution token (not a UUID) so abuse cases stay actionable without
// a fact row.
const (
	TargetObservation = "OBSERVATION"
	TargetDispute     = "DISPUTE"
	TargetContributor = "CONTRIBUTOR"
	TargetEvidence    = "EVIDENCE"
)

// Case statuses. T01 opens OPEN only; IN_REVIEW/RESOLVED/REJECTED arrive
// through audited operator actions in P07-T02.
const (
	StatusOpen     = "OPEN"
	StatusInReview = "IN_REVIEW"
	StatusResolved = "RESOLVED"
	StatusRejected = "REJECTED"
)

// Priorities, P1 highest. The queue orders P1 first, then oldest first,
// so a noisy reporter cannot bury an abuse case with volume.
const (
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
)

var (
	ErrInvalidCase     = errors.New("moderation: invalid case")
	ErrUnknownTarget   = errors.New("moderation: unknown target type")
	ErrUnknownStatus   = errors.New("moderation: unknown status")
	ErrUnknownPriority = errors.New("moderation: unknown priority")
	ErrBadReason       = errors.New("moderation: reason required")
)

// Reason/detail bounds mirror API_PLAN text limits: reason detail stays
// short and reviewable, never a payload dump.
const (
	MaxReasonChars = 200
	MaxDetailChars = 500
)

// CaseParams carries one case-open intent. EvidenceID is an optional
// safe reference (object/session UUID) for privileged review; it never
// carries bytes, GPS or URLs.
type CaseParams struct {
	ID            string
	TargetType    string
	TargetID      string
	Priority      string
	Reason        string
	Detail        string
	EvidenceID    string
	OpenedAt      time.Time
	PolicyVersion string
}

// Case is one bounded actionable review item. Corrections arrive as
// operator actions on the case, never edits of the target fact.
type Case struct {
	ID            string
	TargetType    string
	TargetID      string
	Status        string
	Priority      string
	Reason        string
	Detail        string
	EvidenceID    string
	OpenedAt      time.Time
	PolicyVersion string
}

// ModerationCaseOpened is the domain event recorded at opening.
// Downstream recomputation and trust inputs consume it; the payload
// carries identifiers only, no media or coordinates (B-BR-011).
type ModerationCaseOpened struct {
	CaseID     string
	TargetType string
	TargetID   string
	OccurredAt time.Time
}

// Name derives the emitted event name.
func (e ModerationCaseOpened) Name() string { return "ModerationCaseOpened" }

// NewCase validates and freezes one case in OPEN state.
func NewCase(p CaseParams) (Case, ModerationCaseOpened, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.TargetID) == "" {
		return Case{}, ModerationCaseOpened{}, ErrInvalidCase
	}
	switch p.TargetType {
	case TargetObservation, TargetDispute, TargetContributor, TargetEvidence:
	default:
		return Case{}, ModerationCaseOpened{}, ErrUnknownTarget
	}
	// UUID-shaped targets stay UUID-shaped; contributor tokens stay
	// opaque non-empty strings (they are attribution tokens, not UUIDs).
	if p.TargetType != TargetContributor && !isUUID(p.TargetID) {
		return Case{}, ModerationCaseOpened{}, ErrInvalidCase
	}
	switch p.Priority {
	case PriorityP1, PriorityP2, PriorityP3:
	default:
		return Case{}, ModerationCaseOpened{}, ErrUnknownPriority
	}
	reason := strings.TrimSpace(p.Reason)
	if reason == "" || len([]rune(reason)) > MaxReasonChars {
		return Case{}, ModerationCaseOpened{}, ErrBadReason
	}
	if len([]rune(p.Detail)) > MaxDetailChars {
		return Case{}, ModerationCaseOpened{}, ErrInvalidCase
	}
	if p.EvidenceID != "" && !isUUID(p.EvidenceID) {
		return Case{}, ModerationCaseOpened{}, ErrInvalidCase
	}
	if p.OpenedAt.IsZero() {
		return Case{}, ModerationCaseOpened{}, ErrInvalidCase
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	c := Case{
		ID: p.ID, TargetType: p.TargetType, TargetID: p.TargetID,
		Status: StatusOpen, Priority: p.Priority,
		Reason: reason, Detail: p.Detail, EvidenceID: p.EvidenceID,
		OpenedAt: p.OpenedAt, PolicyVersion: p.PolicyVersion,
	}
	return c, ModerationCaseOpened{
		CaseID: c.ID, TargetType: c.TargetType, TargetID: c.TargetID,
		OccurredAt: c.OpenedAt,
	}, nil
}

// PriorityRank orders priorities for the queue: lower rank first.
func PriorityRank(p string) int {
	switch p {
	case PriorityP1:
		return 0
	case PriorityP2:
		return 1
	case PriorityP3:
		return 2
	default:
		return 3
	}
}

// DefaultPriority maps a target onto its queue priority when the caller
// supplies no explicit hint: contributor abuse triages first, facts and
// reports follow. Explicit caller hints still validate through NewCase.
func DefaultPriority(targetType string) string {
	if targetType == TargetContributor {
		return PriorityP1
	}
	return PriorityP2
}

func isUUID(s string) bool {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			clean = append(clean, s[i])
		}
	}
	raw, err := hex.DecodeString(string(clean))
	return err == nil && len(raw) == 16
}
