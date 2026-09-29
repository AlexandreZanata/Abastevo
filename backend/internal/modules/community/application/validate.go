package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// Stable reason codes recorded on VALIDATING→REJECTED decisions. They are
// part of the contract: rename only with a policy-version bump.
const (
	ReasonInvalidStation        = "invalid-station"
	ReasonInvalidSupersedes     = "invalid-supersedes"
	ReasonEvidenceOwnerMismatch = "evidence-owner-mismatch"
	ReasonEvidenceTimeout       = "evidence-timeout"
	ReasonEvidenceReused        = "evidence-reused"
	ReasonContributorBlocked    = "contributor-blocked"
)

// EvidenceTimeout bounds the media-required path: an explicitly
// photo-dependent submission that is still not ready after this long
// rejects instead of waiting forever. Metadata-only observations (no
// evidence_id) validate without waiting.
const EvidenceTimeout = 24 * time.Hour

// ConsensusJobKind is the downstream intent enqueued atomically with
// VALIDATING→VALIDATED. Consensus computation itself lands in P06; until
// then the worker parks these jobs for audited replay, never silently.
const ConsensusJobKind = "community-consensus"

// ErrPending signals the media-required path is still waiting: the
// observation stays VALIDATING with no terminal decision and the job
// retries. It is distinct from transient infrastructure errors so the
// handler can back off without treating the wait as a failure.
var ErrPending = errors.New("community: validation pending, evidence not ready")

// ErrEvidenceInUse marks an evidence object already bound to a different
// observation. The composition root maps the evidence store refusal onto
// this sentinel; no community code imports the evidence module.
var ErrEvidenceInUse = errors.New("community: evidence already bound to another observation")

// EvidenceState is the read-only evidence signal. Found=false (with nil
// error) means the evidence is missing or not yet ready: pending while
// within the deadline, timeout after it. A port error means the signal is
// unavailable and must retry, never count as verified.
type EvidenceState struct {
	Found    bool
	Ready    bool
	OwnerRef string
}

// ValidateDeps declares every collaborator for validation orchestration.
// All cross-module reads arrive as narrow closures built at the composition
// root; this package never imports directory, evidence, trust or SQL.
type ValidateDeps struct {
	Clock func() time.Time
	// StationExists reports whether the station fact exists.
	StationExists func(ctx context.Context, stationID string) (bool, error)
	// StationLocation reports the reviewed projection quality. The result
	// never blocks admissibility (B-BR-014): unknown or centroid
	// coordinates proceed as UNKNOWN, never as precise positions.
	StationLocation func(ctx context.Context, stationID string) (quality string, hasPoint bool, err error)
	// Evidence reports the bound evidence object, if any.
	Evidence func(ctx context.Context, evidenceID string) (EvidenceState, error)
	// Trust reports whether the contributor is currently blocked. Unknown
	// trust is fail-closed toward retry (error), never toward verified.
	Trust func(ctx context.Context, contributorRef string) (blocked bool, err error)
	// ClaimEvidence binds one READY object to this observation with
	// set-if-unbound-or-same semantics. Nil skips the claim (pre-binding
	// behavior); the composition root injects the evidence store claim
	// and maps its refusal onto ErrEvidenceInUse.
	ClaimEvidence func(ctx context.Context, evidenceID, observationID, contributorRef string) error
	Store         ValidateStore
	// EnqueueConsensus persists the downstream consensus intent in the
	// same transaction as the VALIDATED decision. It receives the open
	// transaction as an opaque handle, mirroring Submit's EnqueueJob.
	EnqueueConsensus func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
}

// ValidateStore is the owned persistence port for orchestration.
type ValidateStore interface {
	Observation(ctx context.Context, id string) (domain.Observation, error)
	Decisions(ctx context.Context, id string) ([]domain.Decision, error)
	RecordDecision(ctx context.Context, d domain.Decision) error
	RecordDecisionWithJob(ctx context.Context, d domain.Decision, kind string, payload []byte, dedupe string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) error
}

// Validate processes one received observation through owned ports.
//
// It claims RECEIVED→VALIDATING with the persisted job ID as command proof,
// then runs station/trust/evidence admissibility and persists the terminal
// transition plus the consensus intent atomically. Terminal states are
// idempotent: replays return the stored state without appending. Pending
// evidence (within the deadline) and unavailable signals return retryable
// errors and leave VALIDATING with no terminal decision, so worker
// retries never regress business state.
func Validate(ctx context.Context, deps ValidateDeps, observationID, commandRef string) (string, error) {
	now := time.Now()
	if deps.Clock != nil {
		now = deps.Clock()
	}
	obs, err := deps.Store.Observation(ctx, observationID)
	if err != nil {
		return "", err
	}
	decisions, err := deps.Store.Decisions(ctx, observationID)
	if err != nil {
		return "", err
	}
	state := deriveState(decisions)
	if state == domain.StateValidated || state == domain.StateRejected {
		return state, nil
	}
	if state == domain.StateReceived {
		if strings.TrimSpace(commandRef) == "" {
			return "", domain.ErrBadCommand
		}
		claim, err := domain.ClaimValidation(obs, domain.StateReceived, commandRef, domain.ActorWorker, now, int64(len(decisions)+1))
		if err != nil {
			return "", err
		}
		if err := deps.Store.RecordDecision(ctx, claim); err != nil {
			// Lost a concurrent claim: reload and continue instead of
			// failing, so retries converge on one VALIDATING.
			decisions, rerr := deps.Store.Decisions(ctx, observationID)
			if rerr != nil {
				return "", rerr
			}
			state = deriveState(decisions)
			if state == domain.StateValidated || state == domain.StateRejected {
				return state, nil
			}
			if state != domain.StateValidating {
				return "", err
			}
		} else {
			state = domain.StateValidating
			decisions = append(decisions, claim)
		}
	}
	if state != domain.StateValidating {
		return "", domain.ErrBadTransition
	}
	seq := int64(len(decisions) + 1)

	// Trust first: a blocked contributor never validates, and an
	// unavailable trust signal retries instead of passing as verified.
	blocked, err := deps.Trust(ctx, obs.ContributorRef)
	if err != nil {
		return state, err
	}
	if blocked {
		return reject(ctx, deps, obs, []string{ReasonContributorBlocked}, now, seq)
	}

	// Station existence is structural: missing stations reject
	// permanently, while lookup failures retry.
	exists, err := deps.StationExists(ctx, obs.StationID)
	if err != nil {
		return state, err
	}
	if !exists {
		return reject(ctx, deps, obs, []string{ReasonInvalidStation}, now, seq)
	}

	// Location quality is consulted explicitly but never gates: unknown
	// or centroid projections proceed as UNKNOWN per B-BR-014. Only a
	// lookup failure retries.
	if _, _, err := deps.StationLocation(ctx, obs.StationID); err != nil {
		return state, err
	}

	// Supersedes links must resolve to the same owner's fact. A lookup
	// failure retries (unknown vs. unavailable is indistinguishable
	// here); a resolved foreign or self link rejects.
	if strings.TrimSpace(obs.SupersedesID) != "" {
		if obs.SupersedesID == obs.ID {
			return reject(ctx, deps, obs, []string{ReasonInvalidSupersedes}, now, seq)
		}
		target, err := deps.Store.Observation(ctx, obs.SupersedesID)
		if err != nil {
			return state, err
		}
		if target.ContributorRef != obs.ContributorRef {
			return reject(ctx, deps, obs, []string{ReasonInvalidSupersedes}, now, seq)
		}
	}

	// Evidence path: metadata-only observations (no evidence_id) admit
	// explicitly with weaker signals; photo-dependent ones wait up to the
	// policy deadline, then reject. Owner mismatches reject per B-BR-010.
	if strings.TrimSpace(obs.EvidenceID) != "" {
		st, err := deps.Evidence(ctx, obs.EvidenceID)
		if err != nil {
			return state, err
		}
		if !st.Found || !st.Ready {
			if now.Sub(obs.ReceivedAt) > EvidenceTimeout {
				return reject(ctx, deps, obs, []string{ReasonEvidenceTimeout}, now, seq)
			}
			return state, ErrPending
		}
		if st.OwnerRef != obs.ContributorRef {
			return reject(ctx, deps, obs, []string{ReasonEvidenceOwnerMismatch}, now, seq)
		}
		// Bind exactly once: a photo backs a single observation, so a
		// reused object rejects instead of double-counting support.
		if deps.ClaimEvidence != nil {
			if err := deps.ClaimEvidence(ctx, obs.EvidenceID, obs.ID, obs.ContributorRef); err != nil {
				if errors.Is(err, ErrEvidenceInUse) {
					return reject(ctx, deps, obs, []string{ReasonEvidenceReused}, now, seq)
				}
				return state, err
			}
		}
	}

	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, now, seq)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]any{"version": 1, "observation_id": obs.ID})
	if err := deps.Store.RecordDecisionWithJob(ctx, admit, ConsensusJobKind, payload, "consensus:"+obs.ID, func(ctx context.Context, tx pgx.Tx, kind string, p []byte, dedupe string) error {
		if deps.EnqueueConsensus == nil {
			return errors.New("community: no consensus enqueue configured")
		}
		return deps.EnqueueConsensus(ctx, tx, kind, p, dedupe)
	}); err != nil {
		if errors.Is(err, domain.ErrBadTransition) {
			// A concurrent worker reached terminal first: converge.
			decisions, rerr := deps.Store.Decisions(ctx, observationID)
			if rerr != nil {
				return "", rerr
			}
			if terminal := deriveState(decisions); terminal == domain.StateValidated || terminal == domain.StateRejected {
				return terminal, nil
			}
		}
		return "", err
	}
	return domain.StateValidated, nil
}

func deriveState(ds []domain.Decision) string {
	state := domain.StateReceived
	for _, d := range ds {
		state = d.ToState
	}
	return state
}

func reject(ctx context.Context, deps ValidateDeps, obs domain.Observation, reasons []string, now time.Time, seq int64) (string, error) {
	decision, err := domain.Reject(obs, domain.StateValidating, domain.ActorWorker, reasons, now, seq)
	if err != nil {
		return "", err
	}
	if err := deps.Store.RecordDecision(ctx, decision); err != nil {
		if errors.Is(err, domain.ErrBadTransition) {
			decisions, rerr := deps.Store.Decisions(ctx, obs.ID)
			if rerr != nil {
				return "", rerr
			}
			if terminal := deriveState(decisions); terminal == domain.StateValidated || terminal == domain.StateRejected {
				return terminal, nil
			}
		}
		return "", err
	}
	return domain.StateRejected, nil
}
