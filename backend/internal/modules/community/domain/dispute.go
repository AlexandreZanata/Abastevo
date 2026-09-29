package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Dispute reasons from COMMUNITY_PRICING_SPEC. Reports flag content for
// review; no reason erases a price on its own.
const (
	ReasonPriceChanged     = "PRICE_CHANGED"
	ReasonWrongStation     = "WRONG_STATION"
	ReasonWrongProduct     = "WRONG_PRODUCT"
	ReasonWrongCondition   = "WRONG_CONDITION"
	ReasonEvidenceMismatch = "EVIDENCE_MISMATCH"
	ReasonOther            = "OTHER"
)

// Dispute lifecycle states. Resolution lands with moderation (P07);
// reports open here and stay open.
const (
	DisputeOpen     = "OPEN"
	DisputeResolved = "RESOLVED"
	DisputeRejected = "REJECTED"
)

var (
	ErrInvalidDispute = errors.New("community: invalid dispute")
	ErrUnknownReason  = errors.New("community: unknown dispute reason")
)

// DisputeParams carries one structured report. The replacement
// observation is optional but, when present, must name a different
// existing observation: self-references are meaningless.
type DisputeParams struct {
	ID                  string
	TargetObservationID string
	ContributorRef      string
	ClientSubmissionID  string
	Reason              string
	Detail              string
	ReplacementID       string
	ReceivedAt          time.Time
	PolicyVersion       string
}

// Dispute is one immutable report against an observation.
type Dispute struct {
	ID                  string
	TargetObservationID string
	ContributorRef      string
	ClientSubmissionID  string
	Reason              string
	Detail              string
	ReplacementID       string
	Status              string
	ReceivedAt          time.Time
	PolicyVersion       string
}

// ObservationDisputed is the domain event recorded at construction.
type ObservationDisputed struct {
	DisputeID     string
	ObservationID string
	OccurredAt    time.Time
}

// Name derives the emitted event name.
func (e ObservationDisputed) Name() string { return "ObservationDisputed" }

// NewDispute validates and freezes one report in OPEN state.
func NewDispute(p DisputeParams) (Dispute, ObservationDisputed, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.TargetObservationID) == "" ||
		strings.TrimSpace(p.ContributorRef) == "" || strings.TrimSpace(p.ClientSubmissionID) == "" {
		return Dispute{}, ObservationDisputed{}, ErrInvalidDispute
	}
	switch p.Reason {
	case ReasonPriceChanged, ReasonWrongStation, ReasonWrongProduct,
		ReasonWrongCondition, ReasonEvidenceMismatch, ReasonOther:
	default:
		return Dispute{}, ObservationDisputed{}, ErrUnknownReason
	}
	if p.ReplacementID != "" {
		if p.ReplacementID == p.TargetObservationID || !isUUID(p.ReplacementID) {
			return Dispute{}, ObservationDisputed{}, ErrInvalidDispute
		}
	}
	if p.ReceivedAt.IsZero() {
		return Dispute{}, ObservationDisputed{}, ErrInvalidDispute
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	d := Dispute{
		ID: p.ID, TargetObservationID: p.TargetObservationID,
		ContributorRef: p.ContributorRef, ClientSubmissionID: p.ClientSubmissionID,
		Reason: p.Reason, Detail: p.Detail, ReplacementID: p.ReplacementID,
		Status: DisputeOpen, ReceivedAt: p.ReceivedAt, PolicyVersion: p.PolicyVersion,
	}
	return d, ObservationDisputed{DisputeID: d.ID, ObservationID: d.TargetObservationID, OccurredAt: d.ReceivedAt}, nil
}

// isUUID accepts canonical UUID text without importing anything beyond
// stdlib, keeping this package dependency-free.
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
