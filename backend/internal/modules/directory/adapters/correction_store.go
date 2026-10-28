package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// CorrectionStore implements application.CorrectionStore over the generated
// directory queries. The decision and its audit row are written in one
// transaction; accepted locations append a reviewed revision through the
// existing repository ladder.
type CorrectionStore struct {
	Pool *pgxpool.Pool
	Repo *Repository
}

func correctionRow(row directory.StationCorrection) application.CorrectionRow {
	var created time.Time
	if row.CreatedAt.Valid {
		created = row.CreatedAt.Time
	}
	var decided *time.Time
	if row.DecidedAt.Valid {
		t := row.DecidedAt.Time
		decided = &t
	}
	return application.CorrectionRow{
		ID: uuidString(row.ID), StationID: uuidString(row.StationID),
		AccountID: uuidString(row.AccountID), Field: row.Field,
		ProposedValue: row.ProposedValue, Note: row.Note,
		EvidenceRef: row.EvidenceRef, ProximityBucket: row.ProximityBucket,
		ClientSubmissionID: row.ClientSubmissionID, Status: row.Status,
		ReasonCodes: row.ReasonCodes, CreatedAt: created, DecidedAt: decided,
	}
}

func (s CorrectionStore) CreateCorrection(ctx context.Context, id, stationID, accountID, field string, value []byte, note, evidenceRef, proximity, key string) (application.CorrectionRow, bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.CorrectionRow{}, false, err
	}
	stationUID, err := mustUUID(stationID)
	if err != nil {
		return application.CorrectionRow{}, false, err
	}
	accountUID, err := mustUUID(accountID)
	if err != nil {
		return application.CorrectionRow{}, false, err
	}
	row, err := directory.New(s.Pool).CreateCorrection(ctx, directory.CreateCorrectionParams{
		ID: uid, StationID: stationUID, AccountID: accountUID, Field: field,
		ProposedValue: value, Note: note, EvidenceRef: evidenceRef,
		ProximityBucket: proximity, ClientSubmissionID: key,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "station_corrections_idempotency_unique":
				return application.CorrectionRow{}, false, nil
			case "station_corrections_one_open_unique":
				return application.CorrectionRow{}, false, application.ErrCorrectionOpen
			}
		}
		return application.CorrectionRow{}, false, err
	}
	return correctionRow(row), true, nil
}

func (s CorrectionStore) CorrectionByKey(ctx context.Context, accountID, clientKey string) (application.CorrectionRow, error) {
	uid, err := mustUUID(accountID)
	if err != nil {
		return application.CorrectionRow{}, application.ErrCorrectionNotFound
	}
	row, err := directory.New(s.Pool).GetCorrectionByKey(ctx, directory.GetCorrectionByKeyParams{
		AccountID: uid, ClientSubmissionID: clientKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.CorrectionRow{}, application.ErrCorrectionNotFound
		}
		return application.CorrectionRow{}, err
	}
	return correctionRow(row), nil
}

func (s CorrectionStore) GetCorrection(ctx context.Context, id string) (application.CorrectionRow, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.CorrectionRow{}, application.ErrCorrectionNotFound
	}
	row, err := directory.New(s.Pool).GetCorrection(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.CorrectionRow{}, application.ErrCorrectionNotFound
		}
		return application.CorrectionRow{}, err
	}
	return correctionRow(row), nil
}

func (s CorrectionStore) ListOwned(ctx context.Context, accountID string, limit, offset int) ([]application.CorrectionRow, error) {
	uid, err := mustUUID(accountID)
	if err != nil {
		return nil, application.ErrCorrectionNotFound
	}
	rows, err := directory.New(s.Pool).ListOwnedCorrections(ctx, directory.ListOwnedCorrectionsParams{
		AccountID: uid, PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]application.CorrectionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, correctionRow(row))
	}
	return out, nil
}

func (s CorrectionStore) CountRecent(ctx context.Context, accountID string) (int64, error) {
	uid, err := mustUUID(accountID)
	if err != nil {
		return 0, application.ErrCorrectionNotFound
	}
	return directory.New(s.Pool).CountRecentCorrections(ctx, uid)
}

func (s CorrectionStore) CountOpen(ctx context.Context, accountID, stationID, field string) (int64, error) {
	accountUID, err := mustUUID(accountID)
	if err != nil {
		return 0, application.ErrCorrectionNotFound
	}
	stationUID, err := mustUUID(stationID)
	if err != nil {
		return 0, application.ErrCorrectionInvalid
	}
	return directory.New(s.Pool).CountOpenCorrections(ctx, directory.CountOpenCorrectionsParams{
		AccountID: accountUID, StationID: stationUID, Field: field,
	})
}

func (s CorrectionStore) CancelCorrection(ctx context.Context, id, accountID string) (int64, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return 0, nil
	}
	accountUID, err := mustUUID(accountID)
	if err != nil {
		return 0, nil
	}
	return directory.New(s.Pool).CancelCorrection(ctx, directory.CancelCorrectionParams{ID: uid, AccountID: accountUID})
}

func (s CorrectionStore) ListPending(ctx context.Context, limit int) ([]application.CorrectionRow, error) {
	rows, err := directory.New(s.Pool).ListPendingCorrections(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]application.CorrectionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, correctionRow(row))
	}
	return out, nil
}

func (s CorrectionStore) Decide(ctx context.Context, id string, status string, reasonCodes []string, decision, reason, deciderKind, deciderID string, evidence []byte) (int64, error) {
	correctionUID, err := mustUUID(id)
	if err != nil {
		return 0, application.ErrCorrectionNotFound
	}
	if len(evidence) == 0 {
		evidence = []byte("{}")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := directory.New(tx)
	n, err := q.DecideCorrection(ctx, directory.DecideCorrectionParams{
		ID: correctionUID, Status: status, ReasonCodes: reasonCodes,
	})
	if err != nil || n == 0 {
		return 0, err
	}
	decisionID, err := NewUUIDv4()
	if err != nil {
		return 0, err
	}
	decisionUID, err := mustUUID(decisionID)
	if err != nil {
		return 0, err
	}
	if _, err := q.CreateCorrectionDecision(ctx, directory.CreateCorrectionDecisionParams{
		ID: decisionUID, CorrectionID: correctionUID, Decision: decision,
		Reason: reason, DeciderKind: deciderKind, DeciderID: deciderID, Evidence: evidence,
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

// ApplyAcceptedLocation appends a reviewed revision from an accepted
// location correction and projects it. reviewed is the highest pin tier, so
// the existing projection cannot downgrade another pin.
func (s CorrectionStore) ApplyAcceptedLocation(ctx context.Context, correction application.Correction) error {
	if s.Repo == nil {
		return errors.New("adapters: correction location repository is not configured")
	}
	rev := domain.LocationRevision{
		StationID:       correction.StationID,
		PointWKT:        fmt.Sprintf("POINT(%f %f)", correction.ProposedValue.Longitude, correction.ProposedValue.Latitude),
		Quality:         "reviewed",
		Provider:        "community-correction",
		SourceReference: correction.ID,
	}
	created, err := s.Repo.RecordLocation(ctx, rev)
	if err != nil {
		return err
	}
	_, err = s.Repo.ProjectLocation(ctx, correction.StationID, created.ID)
	return err
}

var _ application.CorrectionStore = CorrectionStore{}
