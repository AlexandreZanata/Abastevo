package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	dbmoderation "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/moderation"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// RecordAction appends one audit record, moves the case to its derived
// status and enqueues downstream recomputation in a single transaction
// (B-BR-012): the audit, the eligibility change and the downstream
// intent commit together or not at all, and raw history is never
// edited. Closed cases refuse through the guarded transition. The job
// kind/payload/dedupe arrive from the application caller so this package
// stays decoupled from downstream consumers.
func (s *Store) RecordAction(ctx context.Context, a domain.Action, toStatus, kind string, payload []byte, dedupe string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, error) {
	uid, err := mustUUID(a.ID)
	if err != nil {
		return "", err
	}
	caseUUID, err := mustUUID(a.CaseID)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := dbmoderation.New(tx)
	if _, err := tq.InsertAction(ctx, dbmoderation.InsertActionParams{
		ID: uid, CaseID: caseUUID, ActorID: a.ActorID, Action: a.Action,
		Reason: a.Reason, OccurredAt: pgTime(a.OccurredAt),
		PolicyVersion: a.PolicyVersion,
	}); err != nil {
		return "", err
	}
	moved, err := tq.TransitionCase(ctx, dbmoderation.TransitionCaseParams{
		ID: caseUUID, Status: toStatus,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrTerminalCase
		}
		return "", err
	}
	_ = moved
	if enqueue != nil {
		if err := enqueue(ctx, tx, kind, payload, dedupe); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return a.ID, nil
}

// GetAction returns one audit record by ID.
func (s *Store) GetAction(ctx context.Context, id string) (domain.Action, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Action{}, err
	}
	row, err := dbmoderation.New(s.pool).GetAction(ctx, uid)
	if err != nil {
		return domain.Action{}, err
	}
	return domain.Action{
		ID: uuidString(row.ID), CaseID: uuidString(row.CaseID),
		ActorID: row.ActorID, Action: row.Action, Reason: row.Reason,
		OccurredAt: row.OccurredAt.Time, PolicyVersion: row.PolicyVersion,
	}, nil
}

// ListActions returns one case's audit history oldest-first for review
// and rebuilds.
func (s *Store) ListActions(ctx context.Context, caseID string) ([]domain.Action, error) {
	uid, err := mustUUID(caseID)
	if err != nil {
		return nil, err
	}
	rows, err := dbmoderation.New(s.pool).ListActionsByCase(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Action, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Action{
			ID: uuidString(row.ID), CaseID: uuidString(row.CaseID),
			ActorID: row.ActorID, Action: row.Action, Reason: row.Reason,
			OccurredAt: row.OccurredAt.Time, PolicyVersion: row.PolicyVersion,
		})
	}
	return out, nil
}
