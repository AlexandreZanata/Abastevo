package prepared

import (
	"context"
	"fmt"
	"strings"
	"time"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectionItem is one community observation with helpfulness tallies
// for read-side aggregation. Hidden, deleted and expired records never
// appear here (expiry filters by date without writing).
type ProjectionItem struct {
	ID            string
	ConnectorCode string
	PriceMilliBRL *int64
	Condition     string
	ObservedDay   string
	Note          string
	Helpful       int64
	NotHelpful    int64
}

// Vote records one helpfulness vote (no self-votes: callers verify the
// voter does not own the observation; duplicate submissions converge by
// client ID). Votes never hide records; moderation flows set visibility.
func Vote(ctx context.Context, pool *pgxpool.Pool, accountID, observationID, choice, submissionID string) error {
	if accountID == "" || observationID == "" || submissionID == "" {
		return chargingdomain.ErrNotOwner
	}
	if choice != "HELPFUL" && choice != "NOT_HELPFUL" {
		return chargingdomain.ErrInvalidObservation
	}
	q := charging.New(pool)
	accountUUID, err := pgUUID(accountID)
	if err != nil {
		return chargingdomain.ErrNotOwner
	}
	observationUUID, err := pgUUID(observationID)
	if err != nil {
		return chargingdomain.ErrInvalidObservation
	}
	voteID, err := newUUID()
	if err != nil {
		return err
	}
	_, err = q.VoteHelpfulness(ctx, charging.VoteHelpfulnessParams{
		ID: voteID, AccountID: accountUUID, ObservationID: observationUUID,
		Choice: choice, SubmissionID: submissionID,
	})
	if err != nil && strings.Contains(err.Error(), "no rows") {
		return nil
	}
	return err
}

// Report flags one observation for operator review (no self-reports:
// callers verify; duplicate reports converge). Reports never hide
// directly; resolution sets visibility explicitly.
func Report(ctx context.Context, pool *pgxpool.Pool, accountID, observationID, reason, detail string) error {
	if accountID == "" || observationID == "" {
		return chargingdomain.ErrNotOwner
	}
	switch reason {
	case "INACCURATE", "SPAM", "PRIVATE_DATA", "OTHER":
	default:
		return chargingdomain.ErrInvalidObservation
	}
	if len(detail) > 1024 {
		return chargingdomain.ErrInvalidObservation
	}
	q := charging.New(pool)
	accountUUID, err := pgUUID(accountID)
	if err != nil {
		return chargingdomain.ErrNotOwner
	}
	observationUUID, err := pgUUID(observationID)
	if err != nil {
		return chargingdomain.ErrInvalidObservation
	}
	reportID, err := newUUID()
	if err != nil {
		return err
	}
	_, err = q.ReportObservation(ctx, charging.ReportObservationParams{
		ID: reportID, AccountID: accountUUID, ObservationID: observationUUID,
		Reason: reason, Detail: detail,
	})
	if err != nil && strings.Contains(err.Error(), "no rows") {
		return nil
	}
	return err
}

// Project returns visible observations for a place with tallies,
// excluding hidden, deleted and expired (older than sinceDays). Expiry
// filters by date without writing; corrections invalidate by revision
// (callers re-project after correction).
func Project(ctx context.Context, pool *pgxpool.Pool, placeID string, sinceDays int, limit int) ([]ProjectionItem, error) {
	if placeID == "" || limit < 1 || limit > 100 || sinceDays < 0 || sinceDays > 3650 {
		return nil, fmt.Errorf("invalid projection")
	}
	q := charging.New(pool)
	placeUUID, err := pgUUID(placeID)
	if err != nil {
		return nil, chargingdomain.ErrInvalidObservation
	}
	rows, err := q.ObservationProjection(ctx, charging.ObservationProjectionParams{
		PlaceID: placeUUID, Since: daysAgo(sinceDays), LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProjectionItem, 0, min(limit, len(rows)))
	for i, row := range rows {
		if i == limit {
			break
		}
		item := ProjectionItem{
			ID: hexUUID(row.ID), ConnectorCode: row.ConnectorCode, Condition: row.Condition,
			ObservedDay: row.ObservedDay.Time.Format("2006-01-02"), Note: row.Note,
			Helpful: row.Helpful, NotHelpful: row.NotHelpful,
		}
		if row.PriceMilliBrlPerKwh.Valid {
			price := int64(row.PriceMilliBrlPerKwh.Int32)
			item.PriceMilliBRL = &price
		}
		out = append(out, item)
	}
	return out, nil
}

func daysAgo(days int) pgtype.Date {
	return pgtype.Date{Time: time.Now().AddDate(0, 0, -days), Valid: true}
}

// OwnedObservation is one non-deleted account observation with its exact
// revision for correction/withdrawal. Bounded, owner-scoped read.
type OwnedObservation struct {
	ID            string
	PlaceID       *string
	ConnectorCode string
	PriceMilliBRL *int64
	Condition     string
	ObservedDay   string
	Note          string
	Revision      int64
}

// Mine returns up to limit owned observations, newest day first.
func Mine(ctx context.Context, pool *pgxpool.Pool, accountID string, limit int) ([]OwnedObservation, error) {
	if accountID == "" || limit < 1 || limit > 100 {
		return nil, chargingdomain.ErrNotOwner
	}
	accountUUID, err := pgUUID(accountID)
	if err != nil {
		return nil, chargingdomain.ErrNotOwner
	}
	rows, err := charging.New(pool).ObservationsForAccount(ctx, charging.ObservationsForAccountParams{
		AccountID: accountUUID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, err
	}
	out := make([]OwnedObservation, 0, min(limit, len(rows)))
	for i, row := range rows {
		if i == limit {
			break
		}
		item := OwnedObservation{
			ID: hexUUID(row.ID), ConnectorCode: row.ConnectorCode,
			Condition: row.Condition, ObservedDay: row.ObservedDay.Time.Format("2006-01-02"),
			Note: row.Note, Revision: row.Revision,
		}
		if row.PlaceID.Valid {
			place := hexUUID(row.PlaceID)
			item.PlaceID = &place
		}
		if row.PriceMilliBrlPerKwh.Valid {
			price := int64(row.PriceMilliBrlPerKwh.Int32)
			item.PriceMilliBRL = &price
		}
		out = append(out, item)
	}
	return out, nil
}

// ObservationAccount returns the owner of a visible observation. Missing,
// deleted or hidden rows refuse as invalid targets.
func ObservationAccount(ctx context.Context, pool *pgxpool.Pool, observationID string) (string, error) {
	id, err := pgUUID(observationID)
	if err != nil {
		return "", chargingdomain.ErrInvalidObservation
	}
	row, err := charging.New(pool).ObservationOwner(ctx, id)
	if err != nil || row.HiddenAt.Valid {
		return "", chargingdomain.ErrInvalidObservation
	}
	return hexUUID(row.AccountID), nil
}

// Tally returns current helpfulness counts.
func Tally(ctx context.Context, pool *pgxpool.Pool, observationID string) (int64, int64, error) {
	id, err := pgUUID(observationID)
	if err != nil {
		return 0, 0, chargingdomain.ErrInvalidObservation
	}
	row, err := charging.New(pool).HelpfulnessTally(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return row.Helpful, row.NotHelpful, nil
}
