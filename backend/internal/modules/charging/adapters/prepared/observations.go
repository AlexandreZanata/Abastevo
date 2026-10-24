package prepared

import (
	"context"
	"fmt"
	"strings"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ObservationStore stages signed community observations with owner
// isolation, quotas and revision guards. Cross-owner writes refuse;
// forged principals refuse; replays with matching IDs are idempotent
// no-ops (ON CONFLICT DO NOTHING); corrections require exact revisions.
type ObservationStore struct {
	Pool *pgxpool.Pool
}

// Submit creates one observation for the authenticated owner, enforcing
// quotas. Idempotent on client-generated IDs (replay returns existing).
func (s ObservationStore) Submit(ctx context.Context, accountID string, observation chargingdomain.Observation) (string, error) {
	if accountID == "" {
		return "", chargingdomain.ErrNotOwner
	}
	if err := chargingdomain.ValidateObservation(observation); err != nil {
		return "", err
	}
	q := charging.New(s.Pool)
	accountUUID, err := pgUUID(accountID)
	if err != nil {
		return "", chargingdomain.ErrNotOwner
	}
	count, err := q.CountAccountObservations(ctx, accountUUID)
	if err != nil {
		return "", err
	}
	if count >= chargingdomain.MaxObservationsPerAccount {
		return "", chargingdomain.ErrQuotaExceeded
	}
	var placeID pgtype.UUID
	if observation.PlaceID != nil {
		placeID, err = pgUUID(*observation.PlaceID)
		if err != nil {
			return "", chargingdomain.ErrInvalidObservation
		}
	}
	var price pgtype.Int4
	if observation.PriceMilliBRL != nil {
		price = pgtype.Int4{Int32: int32(*observation.PriceMilliBRL), Valid: true}
	}
	day, err := pgDate(observation.ObservedDay)
	if err != nil {
		return "", chargingdomain.ErrInvalidObservation
	}
	row, err := q.CreateChargingObservation(ctx, charging.CreateChargingObservationParams{
		ID: pgUUIDOrFail(accountID, observation.ID), AccountID: pgUUIDOrFail(accountID, accountID),
		PlaceID: placeID, ConnectorCode: observation.ConnectorCode, Price: price,
		Condition: observation.Condition, ObservedDay: day, Note: observation.Note,
	})
	if err != nil {
		// Replay: ON CONFLICT DO NOTHING returns no row when the ID
		// already exists for this account (UNIQUE(account_id, id)).
		// Return the requested ID (idempotent no-op); cross-owner
		// replays cannot collide (separate namespaces).
		if strings.Contains(err.Error(), "no rows") {
			return observation.ID, nil
		}
		return "", err
	}
	return hexUUID(row.ID), nil
}

// Correct replaces one owned observation under exact revision (optimistic
// concurrency). A stale revision conflicts instead of merging; withdrawn
// records refuse correction (recreate explicitly).
func (s ObservationStore) Correct(ctx context.Context, accountID string, observation chargingdomain.Observation, expected int64) (int64, error) {
	if accountID == "" {
		return 0, chargingdomain.ErrNotOwner
	}
	if err := chargingdomain.ValidateObservation(observation); err != nil {
		return 0, err
	}
	if expected <= 0 {
		return 0, chargingdomain.ErrConflict
	}
	q := charging.New(s.Pool)
	accountUUID, err := pgUUID(accountID)
	if err != nil {
		return 0, chargingdomain.ErrNotOwner
	}
	observationID, err := pgUUID(observation.ID)
	if err != nil {
		return 0, chargingdomain.ErrInvalidObservation
	}
	var price pgtype.Int4
	if observation.PriceMilliBRL != nil {
		price = pgtype.Int4{Int32: int32(*observation.PriceMilliBRL), Valid: true}
	}
	day, err := pgDate(observation.ObservedDay)
	if err != nil {
		return 0, chargingdomain.ErrInvalidObservation
	}
	revision, err := q.CorrectChargingObservation(ctx, charging.CorrectChargingObservationParams{
		AccountID: accountUUID, ID: observationID, Expected: expected,
		ConnectorCode: observation.ConnectorCode, Price: price,
		Condition: observation.Condition, ObservedDay: day, Note: observation.Note,
	})
	if err != nil {
		return 0, chargingdomain.ErrConflict
	}
	return int64(revision), nil
}

// Withdraw soft-deletes one owned observation under exact revision.
// Withdrawn records stay tombstoned (never hard-deleted here); replay of
// the same ID is a no-op via the create conflict path.
func (s ObservationStore) Withdraw(ctx context.Context, accountID, observationID string, expected int64) (int64, error) {
	if accountID == "" {
		return 0, chargingdomain.ErrNotOwner
	}
	if expected <= 0 {
		return 0, chargingdomain.ErrConflict
	}
	q := charging.New(s.Pool)
	accountUUID, err := pgUUID(accountID)
	if err != nil {
		return 0, chargingdomain.ErrNotOwner
	}
	id, err := pgUUID(observationID)
	if err != nil {
		return 0, chargingdomain.ErrInvalidObservation
	}
	revision, err := q.WithdrawChargingObservation(ctx, charging.WithdrawChargingObservationParams{
		AccountID: accountUUID, ID: id, Expected: expected,
	})
	if err != nil {
		return 0, chargingdomain.ErrConflict
	}
	return int64(revision), nil
}

func pgUUID(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(text); err != nil || !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID")
	}
	return id, nil
}

func pgUUIDOrFail(_, text string) pgtype.UUID {
	id, _ := pgUUID(text)
	return id
}

func pgDate(day string) (pgtype.Date, error) {
	var date pgtype.Date
	if err := date.Scan(day); err != nil || !date.Valid {
		return pgtype.Date{}, fmt.Errorf("invalid day")
	}
	return date, nil
}
