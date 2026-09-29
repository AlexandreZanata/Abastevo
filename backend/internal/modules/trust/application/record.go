package application

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
)

// Ports declares every collaborator for verdict recording: server clock
// and IDs plus the atomic store and the recompute enqueue threaded into
// the same transaction.
type Ports struct {
	Clock      func() time.Time
	NewID      func() (string, error)
	Store      RecordStore
	EnqueueJob func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
}

// RecordStore is the owned persistence port for verdicts.
type RecordStore interface {
	AppendDecision(ctx context.Context, d domain.Decision, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, error)
}

// RecordDTO carries one audited verdict intent: contributor, tier,
// reason and the reviewed case references behind it. No volume,
// payment or reputation input exists on this path by construction.
type RecordDTO struct {
	ContributorRef string
	Tier           string
	Reason         string
	CaseRefs       []string
}

// Record validates and persists one verdict with downstream
// recomputation. It authenticates nobody itself: operator or pilot
// authorization lands with the P07 callers that invoke it.
func Record(ctx context.Context, p Ports, dto RecordDTO) (domain.Decision, error) {
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	id, err := p.NewID()
	if err != nil {
		return domain.Decision{}, err
	}
	decision, _, err := domain.NewDecision(domain.DecisionParams{
		ID: id, ContributorRef: dto.ContributorRef, Tier: dto.Tier,
		Reason: dto.Reason, CaseRefs: dto.CaseRefs,
		OccurredAt: now, PolicyVersion: domain.PolicyV1,
	})
	if err != nil {
		return domain.Decision{}, err
	}
	if _, err := p.Store.AppendDecision(ctx, decision, func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
	}); err != nil {
		return domain.Decision{}, err
	}
	return decision, nil
}
