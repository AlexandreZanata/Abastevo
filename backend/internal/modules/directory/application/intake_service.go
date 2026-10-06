package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Suggestion states (frozen P27-T01; approved/rejected transitions land
// in T02 review, cancellation stays owner-side here).
const (
	SuggestionPending   = "pending"
	SuggestionCancelled = "cancelled"
)

var (
	ErrSuggestionQuota    = errors.New("directory: daily suggestion quota exceeded")
	ErrSuggestionConflict = errors.New("directory: same key with different proposal")
	ErrSuggestionNotFound = errors.New("directory: suggestion not found")
	ErrSuggestionClosed   = errors.New("directory: suggestion is no longer pending")
	ErrAuthorForbidden    = errors.New("directory: account cannot write suggestions")
)

// Suggestion is one private intake record.
type Suggestion struct {
	ID                 string
	AccountID          string
	ClientSubmissionID string
	Proposal           Proposal
	EvidenceRef        string
	State              string
	CreatedAt          time.Time
}

// IntakeStore persists suggestions. The pg implementation uses the
// generated directory queries; tests use a fake.
type IntakeStore interface {
	CreateSuggestion(ctx context.Context, id, accountID, clientKey string, proposal []byte, evidenceRef string) (SuggestionRow, bool, error)
	SuggestionByKey(ctx context.Context, accountID, clientKey string) (SuggestionRow, error)
	GetSuggestion(ctx context.Context, id string) (SuggestionRow, error)
	ListOwned(ctx context.Context, accountID string, limit, offset int) ([]SuggestionRow, error)
	CountRecent(ctx context.Context, accountID string) (int64, error)
	CancelSuggestion(ctx context.Context, id, accountID string) (int64, error)
}

// SuggestionRow is the stored shape.
type SuggestionRow struct {
	ID                 string
	AccountID          string
	ClientSubmissionID string
	Proposal           []byte
	EvidenceRef        string
	State              string
	CreatedAt          time.Time
}

// IntakeService owns suggestion submission, owner status and
// cancellation. Anonymous contributor proof alone is insufficient: the
// HTTP layer resolves a live account session first and passes its ID
// here; this service never sees tokens.
type IntakeService struct {
	Store IntakeStore
	Clock func() time.Time
	NewID func() (string, error)
}

// Submit stores one proposal idempotently: the same key with the same
// body returns the existing record; the same key with a changed body
// is a conflict (never a silent overwrite); over-quota accounts are
// refused before any write.
func (s IntakeService) Submit(ctx context.Context, accountID, clientKey string, in ProposalInput) (Suggestion, bool, error) {
	if accountID == "" || clientKey == "" {
		return Suggestion{}, false, errors.New("directory: account and client key are required")
	}
	proposal, err := ParseProposal(in)
	if err != nil {
		return Suggestion{}, false, err
	}
	recent, err := s.Store.CountRecent(ctx, accountID)
	if err != nil {
		return Suggestion{}, false, err
	}
	if recent >= MaxSuggestionsPerDay {
		return Suggestion{}, false, ErrSuggestionQuota
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		return Suggestion{}, false, err
	}
	id, err := s.newID()
	if err != nil {
		return Suggestion{}, false, err
	}
	row, created, err := s.Store.CreateSuggestion(ctx, id, accountID, clientKey, body, proposal.EvidenceRef)
	if err != nil {
		return Suggestion{}, false, err
	}
	if !created {
		existing, err := s.Store.SuggestionByKey(ctx, accountID, clientKey)
		if err != nil {
			return Suggestion{}, false, err
		}
		// Compare semantically: JSONB storage normalizes bytes
		// (whitespace/key order), so byte equality would false-conflict.
		var stored Proposal
		if err := json.Unmarshal(existing.Proposal, &stored); err != nil || stored != proposal || existing.EvidenceRef != proposal.EvidenceRef {
			return Suggestion{}, false, ErrSuggestionConflict
		}
		return mapSuggestion(existing), false, nil
	}
	return mapSuggestion(row), true, nil
}

// Cancel closes an owner's pending suggestion. Other states, foreign
// owners and unknown ids fail without leaking which condition fired
// beyond not-found vs closed.
func (s IntakeService) Cancel(ctx context.Context, accountID, id string) error {
	if accountID == "" || id == "" {
		return ErrSuggestionNotFound
	}
	row, err := s.Store.GetSuggestion(ctx, id)
	if err != nil {
		return ErrSuggestionNotFound
	}
	if row.AccountID != accountID {
		return ErrSuggestionNotFound
	}
	if row.State != SuggestionPending {
		return ErrSuggestionClosed
	}
	affected, err := s.Store.CancelSuggestion(ctx, id, accountID)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSuggestionClosed
	}
	return nil
}

// Owned returns one suggestion only to its owner (IDOR-safe: foreign
// owners see not-found).
func (s IntakeService) Owned(ctx context.Context, accountID, id string) (Suggestion, error) {
	if accountID == "" || id == "" {
		return Suggestion{}, ErrSuggestionNotFound
	}
	row, err := s.Store.GetSuggestion(ctx, id)
	if err != nil {
		return Suggestion{}, ErrSuggestionNotFound
	}
	if row.AccountID != accountID {
		return Suggestion{}, ErrSuggestionNotFound
	}
	return mapSuggestion(row), nil
}

// ListOwned returns the owner's private requests, newest first.
func (s IntakeService) ListOwned(ctx context.Context, accountID string, limit, offset int) ([]Suggestion, error) {
	if accountID == "" {
		return nil, ErrSuggestionNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.Store.ListOwned(ctx, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapSuggestion(row))
	}
	return out, nil
}

func (s IntakeService) newID() (string, error) {
	if s.NewID != nil {
		return s.NewID()
	}
	return "", errors.New("directory: id generator required")
}

func mapSuggestion(row SuggestionRow) Suggestion {
	var proposal Proposal
	_ = json.Unmarshal(row.Proposal, &proposal)
	return Suggestion{
		ID: row.ID, AccountID: row.AccountID,
		ClientSubmissionID: row.ClientSubmissionID, Proposal: proposal,
		EvidenceRef: row.EvidenceRef, State: row.State,
		CreatedAt: row.CreatedAt,
	}
}
