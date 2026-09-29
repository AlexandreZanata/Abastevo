package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// EligibilityWindow bounds confirmation freshness: only VALIDATED facts
// received within the consensus window stay eligible for new support.
const EligibilityWindow = 48 * time.Hour

var (
	// ErrTargetNotFound marks a missing vote/report target. Callers map
	// it onto 404 without distinguishing anything else.
	ErrTargetNotFound = errors.New("community: target observation not found")
	// ErrIneligibleTarget marks a target in the wrong state or age for
	// the command: unvalidated or stale for support, already rejected
	// for reports.
	ErrIneligibleTarget = errors.New("community: target not eligible")
)

// VoteStore is the owned persistence port for support votes and reports:
// state reads plus atomic insert-or-converge writes with a recompute job
// in the same transaction.
type VoteStore interface {
	Observation(ctx context.Context, id string) (domain.Observation, error)
	Decisions(ctx context.Context, id string) ([]domain.Decision, error)
	Confirm(ctx context.Context, c domain.Confirmation, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (id string, replayed bool, err error)
	Report(ctx context.Context, d domain.Dispute, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (id string, replayed bool, err error)
}

// VotePorts wires support and report commands through quota, identity
// and the durable recompute enqueue.
type VotePorts struct {
	Clock      func() time.Time
	NewID      func() (string, error)
	CheckQuota func(ctx context.Context, subject, operation string) (time.Duration, error)
	EnqueueJob func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
	Store      VoteStore
}

// ConfirmDTO carries one support vote: target plus client operation
// identity for safe retries. The supporter derives from proof.
type ConfirmDTO struct {
	ObservationID      string
	ClientSubmissionID string
}

// ConfirmResult is the safe acknowledgment: the vote identity, never a
// projection claim.
type ConfirmResult struct {
	ConfirmationID string
	ReceivedAt     time.Time
	Replayed       bool
}

// Confirm records one independent support vote on a VALIDATED,
// still-eligible observation. Retries converge; duplicate support and
// divergent payloads conflict; every accepted vote enqueues consensus
// recomputation for its target.
func Confirm(ctx context.Context, p VotePorts, caller Caller, dto ConfirmDTO) (ConfirmResult, error) {
	if caller.ContributorID == "" || caller.Token == "" {
		return ConfirmResult{}, ErrUnauthorized
	}
	if strings.TrimSpace(dto.ClientSubmissionID) == "" {
		return ConfirmResult{}, domain.ErrInvalidConfirmation
	}
	if _, err := p.CheckQuota(ctx, caller.Fingerprint, "write"); err != nil {
		return ConfirmResult{}, err
	}
	now := p.Clock()
	obs, _, err := eligibleObservation(ctx, p.Store, dto.ObservationID)
	if err != nil {
		return ConfirmResult{}, err
	}
	if now.Sub(obs.ReceivedAt) > EligibilityWindow {
		return ConfirmResult{}, ErrIneligibleTarget
	}
	id, err := p.NewID()
	if err != nil {
		return ConfirmResult{}, err
	}
	vote, _, err := domain.NewConfirmation(domain.ConfirmationParams{
		ID: id, ObservationID: obs.ID,
		ContributorRef: caller.Token, AuthorRef: obs.ContributorRef,
		ClientSubmissionID: dto.ClientSubmissionID, ReceivedAt: now,
		PolicyVersion: domain.PolicyV1,
	})
	if err != nil {
		return ConfirmResult{}, err
	}
	storedID, replayed, err := p.Store.Confirm(ctx, vote, func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return ConfirmResult{}, ErrConflict
		}
		return ConfirmResult{}, err
	}
	return ConfirmResult{ConfirmationID: storedID, ReceivedAt: now, Replayed: replayed}, nil
}

// DisputeDTO carries one structured report with an optional replacement
// reference. Reports flag content for review; none erases a price.
type DisputeDTO struct {
	TargetObservationID string
	ClientSubmissionID  string
	Reason              string
	Detail              string
	ReplacementID       string
}

// DisputeResult is the safe acknowledgment: the report identity and its
// OPEN state.
type DisputeResult struct {
	DisputeID  string
	State      string
	ReceivedAt time.Time
	Replayed   bool
}

// Dispute records one structured report against an existing,
// non-rejected observation. Active repeated reports converge; every
// accepted report enqueues consensus recomputation for its target.
func Dispute(ctx context.Context, p VotePorts, caller Caller, dto DisputeDTO) (DisputeResult, error) {
	if caller.ContributorID == "" || caller.Token == "" {
		return DisputeResult{}, ErrUnauthorized
	}
	if strings.TrimSpace(dto.ClientSubmissionID) == "" {
		return DisputeResult{}, domain.ErrInvalidDispute
	}
	if _, err := p.CheckQuota(ctx, caller.Fingerprint, "write"); err != nil {
		return DisputeResult{}, err
	}
	now := p.Clock()
	id, err := p.NewID()
	if err != nil {
		return DisputeResult{}, err
	}
	// Domain input validation precedes target reads: malformed reports
	// fail before any database work.
	probe, _, err := domain.NewDispute(domain.DisputeParams{
		ID: id, TargetObservationID: dto.TargetObservationID,
		ContributorRef: caller.Token, ClientSubmissionID: dto.ClientSubmissionID,
		Reason: dto.Reason, Detail: dto.Detail, ReplacementID: dto.ReplacementID,
		ReceivedAt: now, PolicyVersion: domain.PolicyV1,
	})
	if err != nil {
		return DisputeResult{}, err
	}
	obs, state, err := eligibleObservation(ctx, p.Store, dto.TargetObservationID)
	if err != nil {
		return DisputeResult{}, err
	}
	if state == domain.StateRejected {
		return DisputeResult{}, ErrIneligibleTarget
	}
	report := probe
	report.TargetObservationID = obs.ID
	if report.ReplacementID != "" {
		if _, err := p.Store.Observation(ctx, report.ReplacementID); err != nil {
			return DisputeResult{}, ErrTargetNotFound
		}
	}
	storedID, replayed, err := p.Store.Report(ctx, report, func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return DisputeResult{}, ErrConflict
		}
		return DisputeResult{}, err
	}
	return DisputeResult{DisputeID: storedID, State: domain.DisputeOpen, ReceivedAt: now, Replayed: replayed}, nil
}

// eligibleObservation loads one target with its derived validation
// state. Missing targets share one not-found shape.
func eligibleObservation(ctx context.Context, store VoteStore, id string) (domain.Observation, string, error) {
	obs, err := store.Observation(ctx, id)
	if err != nil {
		return domain.Observation{}, "", ErrTargetNotFound
	}
	decisions, err := store.Decisions(ctx, id)
	if err != nil {
		return domain.Observation{}, "", err
	}
	state := domain.StateReceived
	for _, d := range decisions {
		state = d.ToState
	}
	if state != domain.StateValidated {
		return domain.Observation{}, state, ErrIneligibleTarget
	}
	return obs, state, nil
}
