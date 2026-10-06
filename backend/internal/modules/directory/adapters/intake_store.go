package adapters

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

// IntakeStore implements application.IntakeStore over the generated
// directory queries. All reads are owner-scoped; cancellation is
// owner-and-pending guarded in SQL as well as the service.
type IntakeStore struct {
	Q *directory.Queries
}

func intakeRow(row directory.StationSuggestion) application.SuggestionRow {
	var created time.Time
	if row.CreatedAt.Valid {
		created = row.CreatedAt.Time
	}
	return application.SuggestionRow{
		ID: uuidString(row.ID), AccountID: uuidString(row.AccountID),
		ClientSubmissionID: row.ClientSubmissionID, Proposal: row.Proposal,
		EvidenceRef: row.EvidenceRef, State: row.State, CreatedAt: created,
	}
}

func (s IntakeStore) CreateSuggestion(ctx context.Context, id, accountID, clientKey string, proposal []byte, evidenceRef string) (application.SuggestionRow, bool, error) {
	suggestionID, err := mustUUID(id)
	if err != nil {
		return application.SuggestionRow{}, false, err
	}
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return application.SuggestionRow{}, false, err
	}
	row, err := s.Q.CreateSuggestion(ctx, directory.CreateSuggestionParams{
		ID: suggestionID, AccountID: ownerID,
		ClientSubmissionID: clientKey, Proposal: proposal, EvidenceRef: evidenceRef,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.SuggestionRow{}, false, nil
		}
		return application.SuggestionRow{}, false, err
	}
	return intakeRow(row), true, nil
}

func (s IntakeStore) SuggestionByKey(ctx context.Context, accountID, clientKey string) (application.SuggestionRow, error) {
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return application.SuggestionRow{}, err
	}
	row, err := s.Q.GetSuggestionByKey(ctx, directory.GetSuggestionByKeyParams{
		AccountID: ownerID, ClientSubmissionID: clientKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return application.SuggestionRow{}, application.ErrSuggestionNotFound
		}
		return application.SuggestionRow{}, err
	}
	return intakeRow(row), nil
}

func (s IntakeStore) GetSuggestion(ctx context.Context, id string) (application.SuggestionRow, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.SuggestionRow{}, application.ErrSuggestionNotFound
	}
	row, err := s.Q.GetSuggestion(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return application.SuggestionRow{}, application.ErrSuggestionNotFound
		}
		return application.SuggestionRow{}, err
	}
	return intakeRow(row), nil
}

func (s IntakeStore) ListOwned(ctx context.Context, accountID string, limit, offset int) ([]application.SuggestionRow, error) {
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return nil, application.ErrSuggestionNotFound
	}
	rows, err := s.Q.ListOwnedSuggestions(ctx, directory.ListOwnedSuggestionsParams{
		AccountID: ownerID, PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]application.SuggestionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, intakeRow(row))
	}
	return out, nil
}

func (s IntakeStore) CountRecent(ctx context.Context, accountID string) (int64, error) {
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return 0, err
	}
	return s.Q.CountRecentSuggestions(ctx, ownerID)
}

func (s IntakeStore) CancelSuggestion(ctx context.Context, id, accountID string) (int64, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return 0, nil
	}
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return 0, nil
	}
	return s.Q.CancelSuggestion(ctx, directory.CancelSuggestionParams{ID: uid, AccountID: ownerID})
}

func (s IntakeStore) CreateDecision(ctx context.Context, id, suggestionID, decision, reason, reviewer, stationID string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	sugUID, err := mustUUID(suggestionID)
	if err != nil {
		return err
	}
	var stationUID pgtype.UUID
	if stationID != "" {
		stationUID, err = mustUUID(stationID)
		if err != nil {
			return err
		}
	}
	_, err = s.Q.CreateDecision(ctx, directory.CreateDecisionParams{
		ID: uid, SuggestionID: sugUID, Decision: decision,
		Reason: reason, Reviewer: reviewer, StationID: stationUID,
	})
	return err
}

func (s IntakeStore) SetSuggestionState(ctx context.Context, id, state string) (int64, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return 0, err
	}
	return s.Q.DecideSuggestion(ctx, directory.DecideSuggestionParams{ID: uid, State: state})
}

func (s IntakeStore) ListPending(ctx context.Context, limit int) ([]application.SuggestionRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.Q.ListPendingSuggestions(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]application.SuggestionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, intakeRow(directory.StationSuggestion{
			ID: row.ID, AccountID: row.AccountID,
			ClientSubmissionID: row.ClientSubmissionID, Proposal: row.Proposal,
			EvidenceRef: row.EvidenceRef, State: row.State,
			CreatedAt: row.CreatedAt, DecidedAt: row.DecidedAt,
		}))
	}
	return out, nil
}
