package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// ActStore is the owned persistence port for operator actions: case
// reads plus atomic audit/status/job writes.
type ActStore interface {
	Get(ctx context.Context, id string) (domain.Case, error)
	RecordAction(ctx context.Context, a domain.Action, toStatus, kind string, payload []byte, dedupe string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, error)
}

// ActPorts wires operator actions through server clock and IDs plus the
// atomic store and the recompute enqueue threaded into the same
// transaction.
type ActPorts struct {
	Clock      func() time.Time
	NewID      func() (string, error)
	Store      ActStore
	EnqueueJob func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
}

// Caller carries the accountable operator identity. It arrives from
// restricted operator access (CLI environment), never from a public
// request: empty operators are denied before any work.
type Caller struct {
	OperatorID string
}

// ActDTO carries one reviewed operator intent: the case, the action and
// the mandatory reason.
type ActDTO struct {
	CaseID string
	Action string
	Reason string
}

// ActResult is the safe acknowledgment: the audit identity plus the
// case's new status.
type ActResult struct {
	ActionID string
	CaseID   string
	ToStatus string
	JobKind  string
	Enqueued bool
	Occurred time.Time
}

// Act records one audited operator action with its case status move and
// downstream recomputation, atomically. It authenticates the operator
// only by presence of the restricted-access identity: anonymous callers
// are denied, and there is no public admin path (BUC-006).
func Act(ctx context.Context, p ActPorts, caller Caller, dto ActDTO) (ActResult, error) {
	if strings.TrimSpace(caller.OperatorID) == "" {
		return ActResult{}, domain.ErrUnauthorized
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	id, err := p.NewID()
	if err != nil {
		return ActResult{}, err
	}
	c, err := p.Store.Get(ctx, strings.TrimSpace(dto.CaseID))
	if err != nil {
		return ActResult{}, err
	}
	// Domain input validation precedes state reads: malformed actions
	// fail before any status derivation.
	probe, _, err := domain.NewAction(domain.ActionParams{
		ID: id, CaseID: c.ID, ActorID: caller.OperatorID,
		Action: strings.TrimSpace(dto.Action), Reason: dto.Reason,
		OccurredAt: now,
	})
	if err != nil {
		return ActResult{}, err
	}
	if !domain.AllowedForTarget(probe.Action, c.TargetType) {
		return ActResult{}, domain.ErrActionForbidden
	}
	toStatus, err := domain.NextStatus(probe.Action, c.Status)
	if err != nil {
		return ActResult{}, err
	}
	kind, payload := actionJob(c, probe)
	storedID, err := p.Store.RecordAction(ctx, probe, toStatus, kind, payload, "moderation:"+c.ID+":"+probe.ID,
		func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
		})
	if err != nil {
		return ActResult{}, err
	}
	return ActResult{
		ActionID: storedID, CaseID: c.ID, ToStatus: toStatus,
		JobKind: kind, Enqueued: true, Occurred: now,
	}, nil
}

// actionJob maps an action onto its downstream recompute intent.
// Observation invalidation replays through the community consensus
// consumer; contributor blocks replay through the trust consumer (which
// parks like every trust verdict until its reader needs it); triage and
// closure record a moderation-applied intent for the audit trail.
func actionJob(c domain.Case, a domain.Action) (string, []byte) {
	switch {
	case a.Action == domain.ActionInvalidate && c.TargetType == domain.TargetObservation:
		raw, _ := json.Marshal(map[string]any{"version": 1, "observation_id": c.TargetID})
		return "community-consensus", raw
	case a.Action == domain.ActionBlock && c.TargetType == domain.TargetContributor:
		raw, _ := json.Marshal(map[string]any{"version": 1, "contributor_ref": c.TargetID})
		return "trust-affected-recompute", raw
	default:
		raw, _ := json.Marshal(map[string]any{
			"version": 1, "case_id": c.ID, "action": a.Action,
			"target_type": c.TargetType, "target_id": c.TargetID,
		})
		return "moderation-applied", raw
	}
}
