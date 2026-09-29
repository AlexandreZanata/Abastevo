package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidConfirmation = errors.New("community: invalid confirmation")
	ErrSelfConfirmation    = errors.New("community: contributor cannot confirm own observation")
	ErrAlreadyConfirmed    = errors.New("community: observation already confirmed by contributor")
)

// ConfirmationParams carries one support vote plus server-resolved
// attribution. Both ContributorRef and AuthorRef arrive from verified
// facts, never from the client body: the domain compares them itself so
// no caller can forget the self-confirmation ban (B-BR-006).
type ConfirmationParams struct {
	ID                 string
	ObservationID      string
	ContributorRef     string
	AuthorRef          string
	ClientSubmissionID string
	ReceivedAt         time.Time
	PolicyVersion      string
}

// Confirmation is one immutable support vote. It affirms the referenced
// observation's full price key and changes nothing about it.
type Confirmation struct {
	ID                 string
	ObservationID      string
	ContributorRef     string
	ClientSubmissionID string
	ReceivedAt         time.Time
	PolicyVersion      string
}

// PriceConfirmed is the domain event recorded at construction.
type PriceConfirmed struct {
	ConfirmationID string
	ObservationID  string
	OccurredAt     time.Time
}

// Name derives the emitted event name.
func (e PriceConfirmed) Name() string { return "PriceConfirmed" }

// NewConfirmation validates and freezes one vote.
func NewConfirmation(p ConfirmationParams) (Confirmation, PriceConfirmed, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ObservationID) == "" ||
		strings.TrimSpace(p.ContributorRef) == "" || strings.TrimSpace(p.AuthorRef) == "" ||
		strings.TrimSpace(p.ClientSubmissionID) == "" {
		return Confirmation{}, PriceConfirmed{}, ErrInvalidConfirmation
	}
	if p.ContributorRef == p.AuthorRef {
		return Confirmation{}, PriceConfirmed{}, ErrSelfConfirmation
	}
	if p.ReceivedAt.IsZero() {
		return Confirmation{}, PriceConfirmed{}, ErrInvalidConfirmation
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	c := Confirmation{
		ID: p.ID, ObservationID: p.ObservationID,
		ContributorRef: p.ContributorRef, ClientSubmissionID: p.ClientSubmissionID,
		ReceivedAt: p.ReceivedAt, PolicyVersion: p.PolicyVersion,
	}
	return c, PriceConfirmed{ConfirmationID: c.ID, ObservationID: c.ObservationID, OccurredAt: c.ReceivedAt}, nil
}
