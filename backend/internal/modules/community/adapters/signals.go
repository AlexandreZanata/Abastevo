package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

// ErrNoSignals marks observations whose bands were never derived:
// pre-hook history or a derivation that has not run yet. Callers fail
// closed to no-proximity, never to verified.
var ErrNoSignals = errors.New("adapters: unknown signals")

// UpsertSignals persists one derived signal set, converging recomputation
// without history forks: the projection is mutable and rebuildable by
// design, bands only, never coordinates.
func (s *Store) UpsertSignals(ctx context.Context, observationID string, b application.Bands, computedAt time.Time) error {
	uid, err := mustUUID(observationID)
	if err != nil {
		return err
	}
	_, err = community.New(s.pool).UpsertSignals(ctx, community.UpsertSignalsParams{
		ObservationID:  uid,
		ProximityBand:  b.Proximity,
		RecencyBand:    b.Recency,
		CaptureFlag:    b.Capture,
		PhotoSignal:    b.Photo,
		DuplicateCount: int32(b.DuplicateCount),
		RegionalBand:   b.Regional,
		RiskCodes:      append([]string{}, b.RiskCodes...),
		NeedsReview:    b.NeedsReview,
		PolicyVersion:  b.PolicyVersion,
		ComputedAt:     pgTime(computedAt),
	})
	return err
}

// LoadSignals reads one observation's derived signals for consensus and
// review. Missing rows report a typed miss: signals may simply not have
// been derived yet.
func (s *Store) LoadSignals(ctx context.Context, observationID string) (application.Bands, error) {
	uid, err := mustUUID(observationID)
	if err != nil {
		return application.Bands{}, err
	}
	row, err := community.New(s.pool).GetSignals(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.Bands{}, ErrNoSignals
		}
		return application.Bands{}, err
	}
	return application.Bands{
		Proximity: row.ProximityBand, Recency: row.RecencyBand,
		Capture: row.CaptureFlag, Photo: row.PhotoSignal,
		DuplicateCount: int(row.DuplicateCount), Regional: row.RegionalBand,
		RiskCodes:   append([]string{}, row.RiskCodes...),
		NeedsReview: row.NeedsReview, PolicyVersion: row.PolicyVersion,
	}, nil
}
