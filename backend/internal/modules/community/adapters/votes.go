package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// consensusPayload is the versioned recompute envelope shared by
// validation, support and report writes; the P06-T05 consumer resolves
// the price key from the observation.
func consensusPayload(observationID string) []byte {
	raw, _ := json.Marshal(map[string]any{"version": 1, "observation_id": observationID})
	return raw
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Confirm persists one support vote with its consensus recompute job in
// a single transaction. Identical retries converge on the natural key;
// divergent payloads conflict; a second vote by the same contributor on
// the same observation refuses through the pair unique index, so one
// contributor can never amplify support.
func (s *Store) Confirm(ctx context.Context, c domain.Confirmation, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	uid, err := mustUUID(c.ID)
	if err != nil {
		return "", false, err
	}
	obsUUID, err := mustUUID(c.ObservationID)
	if err != nil {
		return "", false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := community.New(tx)
	inserted, err := tq.InsertConfirmation(ctx, community.InsertConfirmationParams{
		ID: uid, ObservationID: obsUUID,
		ContributorRef: c.ContributorRef, ClientSubmissionID: c.ClientSubmissionID,
		ReceivedAt: pgTime(c.ReceivedAt), PolicyVersion: c.PolicyVersion,
	})
	_ = inserted
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.resolveConfirmConflict(ctx, c)
		}
		if isUniqueViolation(err) {
			return "", false, domain.ErrAlreadyConfirmed
		}
		return "", false, err
	}
	if err := enqueue(ctx, tx, "community-consensus", consensusPayload(c.ObservationID), "consensus:"+c.ObservationID); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return c.ID, false, nil
}

// resolveConfirmConflict compares a retried vote against the stored row.
func (s *Store) resolveConfirmConflict(ctx context.Context, c domain.Confirmation) (string, bool, error) {
	row, err := community.New(s.pool).GetConfirmationByNaturalKey(ctx, community.GetConfirmationByNaturalKeyParams{
		ContributorRef: c.ContributorRef, ClientSubmissionID: c.ClientSubmissionID,
	})
	if err != nil {
		return "", false, err
	}
	if uuidString(row.ObservationID) != c.ObservationID {
		return "", false, domain.ErrConflict
	}
	return uuidString(row.ID), true, nil
}

// Report persists one dispute with its consensus recompute job in a
// single transaction. Identical retries converge; divergent payloads
// conflict; an already-open report for the same reporter, target and
// reason converges instead of flooding (reports never erase prices).
func (s *Store) Report(ctx context.Context, d domain.Dispute, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	uid, err := mustUUID(d.ID)
	if err != nil {
		return "", false, err
	}
	targetUUID, err := mustUUID(d.TargetObservationID)
	if err != nil {
		return "", false, err
	}
	replacementUUID, err := uuidOrNull(d.ReplacementID)
	if err != nil {
		return "", false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := community.New(tx)
	detail := strings.TrimSpace(d.Detail)
	inserted, err := tq.InsertDispute(ctx, community.InsertDisputeParams{
		ID: uid, TargetObservationID: targetUUID,
		ContributorRef: d.ContributorRef, ClientSubmissionID: d.ClientSubmissionID,
		Reason: d.Reason, Detail: detail, ReplacementID: replacementUUID,
		Status: d.Status, ReceivedAt: pgTime(d.ReceivedAt), PolicyVersion: d.PolicyVersion,
	})
	_ = inserted
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.resolveDisputeConflict(ctx, d)
		}
		if isUniqueViolation(err) {
			return s.convergeOpenReport(ctx, targetUUID, d)
		}
		return "", false, err
	}
	if err := enqueue(ctx, tx, "community-consensus", consensusPayload(d.TargetObservationID), "consensus:"+d.TargetObservationID); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return d.ID, false, nil
}

// resolveDisputeConflict compares a retried report against the stored row.
func (s *Store) resolveDisputeConflict(ctx context.Context, d domain.Dispute) (string, bool, error) {
	row, err := community.New(s.pool).GetDisputeByNaturalKey(ctx, community.GetDisputeByNaturalKeyParams{
		ContributorRef: d.ContributorRef, ClientSubmissionID: d.ClientSubmissionID,
	})
	if err != nil {
		return "", false, err
	}
	if uuidString(row.TargetObservationID) != d.TargetObservationID || row.Reason != d.Reason ||
		row.Detail != strings.TrimSpace(d.Detail) || uuidString(row.ReplacementID) != d.ReplacementID {
		return "", false, domain.ErrConflict
	}
	return uuidString(row.ID), true, nil
}

// convergeOpenReport returns the already-open report for the same
// reporter, target and reason: active repeated reports deduplicate.
// The target UUID is pre-validated by the caller.
func (s *Store) convergeOpenReport(ctx context.Context, targetUUID pgtype.UUID, d domain.Dispute) (string, bool, error) {
	row, err := community.New(s.pool).GetOpenDispute(ctx, community.GetOpenDisputeParams{
		ContributorRef: d.ContributorRef, TargetObservationID: targetUUID, Reason: d.Reason,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, domain.ErrConflict
		}
		return "", false, err
	}
	return uuidString(row.ID), true, nil
}
