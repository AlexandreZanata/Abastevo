package application

import (
	"context"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// Store is the persistence port behind every rating use case.
// Atomicity lives here: UpsertRating converges, updates or inserts
// under one lock/transaction while refreshing the key stats, so
// concurrent writers cannot fork or inflate a target. The memory
// implementation guards with a mutex; the Postgres one serializes on
// the live row plus the partial unique index.
type Store interface {
	UpsertRating(ctx context.Context, rec domain.StoredRating) (domain.StoredRating, bool, error)
	DeleteRating(ctx context.Context, accountID, stationID, product string, nowUnix int64) (bool, error)
	Stats(ctx context.Context, stationID, product string) (domain.RatingStats, bool, error)
	RebuildStats(ctx context.Context, stationID, product string, nowUnix int64) (domain.RatingStats, error)
	ListRatingsByAccount(ctx context.Context, accountID string) ([]domain.StoredRating, error)
}

// AccountGate authorizes one account for social writes. It is REQUIRED
// (nil fails closed): feedback has no anonymous baseline (F01, F08).
// The composition root injects the account-owned check; transport
// derives the account from a live session before calling.
type AccountGate func(ctx context.Context, accountID string) error

// StationExists reports whether a directory station exists. REQUIRED
// (nil fails closed): ratings target existing stations only (F01).
type StationExists func(ctx context.Context, stationID string) (bool, error)

// Service wires frozen feedback policy to the Store and gate ports.
type Service struct {
	Clock         domain.Clock
	Store         Store
	Comments      CommentStore
	Votes         VoteStore
	CheckAccount  AccountGate
	StationExists StationExists
	ReportQuota   ReportQuota
	OpenCase      OpenCase
	IDGen         func() (string, error)
}

// RateResult is the outcome of rating a target.
type RateResult struct {
	Rating  domain.StoredRating
	Stats   domain.RatingStats
	Created bool
}

// Rate records one current rating per account/station/fuel (F02).
// Equal stars converge idempotently; changed stars bump the revision;
// every path refreshes the exact key stats.
func (s *Service) Rate(ctx context.Context, accountID, stationID, product string, stars int) (RateResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return RateResult{}, domain.ErrTargetInvalid
	}
	if s.CheckAccount == nil || s.StationExists == nil {
		return RateResult{}, domain.ErrGateRequired
	}
	if err := s.CheckAccount(ctx, accountID); err != nil {
		return RateResult{}, err
	}
	target := domain.Target{StationID: strings.TrimSpace(stationID), Product: strings.TrimSpace(product)}
	if err := target.Validate(); err != nil {
		return RateResult{}, err
	}
	if err := domain.Rating(stars).Validate(); err != nil {
		return RateResult{}, err
	}
	exists, err := s.StationExists(ctx, target.StationID)
	if err != nil {
		return RateResult{}, err
	}
	if !exists {
		return RateResult{}, domain.ErrTargetInvalid
	}
	id, err := s.IDGen()
	if err != nil {
		return RateResult{}, err
	}
	stored, created, err := s.Store.UpsertRating(ctx, domain.StoredRating{
		ID:        id,
		AccountID: accountID,
		StationID: target.StationID,
		Product:   target.Product,
		Stars:     stars,
		Revision:  1,
		CreatedAt: s.Clock.NowUnix(),
	})
	if err != nil {
		return RateResult{}, err
	}
	stats, found, err := s.Store.Stats(ctx, target.StationID, target.Product)
	if err != nil {
		return RateResult{}, err
	}
	if !found {
		return RateResult{}, domain.ErrStatsMissing
	}
	return RateResult{Rating: stored, Stats: stats, Created: created}, nil
}

// DeleteRating tombstones the caller's current rating. Unknown keys
// refuse without touching stats; tombstoned rows stay for audit but
// leave the aggregates.
func (s *Service) DeleteRating(ctx context.Context, accountID, stationID, product string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrTargetInvalid
	}
	if s.CheckAccount == nil {
		return domain.ErrGateRequired
	}
	if err := s.CheckAccount(ctx, accountID); err != nil {
		return err
	}
	deleted, err := s.Store.DeleteRating(ctx, accountID, strings.TrimSpace(stationID), strings.TrimSpace(product), s.Clock.NowUnix())
	if err != nil {
		return err
	}
	if !deleted {
		return domain.ErrRatingNotFound
	}
	return nil
}

// RebuildStats recomputes one key aggregate from live rows for
// reconciliation. Writers already maintain stats per write; this is
// the audited repair path, not the hot path.
func (s *Service) RebuildStats(ctx context.Context, stationID, product string) (domain.RatingStats, error) {
	return s.Store.RebuildStats(ctx, strings.TrimSpace(stationID), strings.TrimSpace(product), s.Clock.NowUnix())
}

// RatingStats reads the maintained aggregate for one station/product
// (P22-T02, B-BR-F02). Public anonymous read like comment lists and
// vote tallies: blank targets refuse, missing keys return zero
// counts (honest empty, never an error or invented mean).
func (s *Service) RatingStats(ctx context.Context, stationID, product string) (domain.RatingStats, error) {
	target := domain.Target{StationID: strings.TrimSpace(stationID), Product: strings.TrimSpace(product)}
	if err := target.Validate(); err != nil {
		return domain.RatingStats{}, err
	}
	stats, found, err := s.Store.Stats(ctx, target.StationID, target.Product)
	if err != nil {
		return domain.RatingStats{}, err
	}
	if !found {
		return domain.RatingStats{StationID: target.StationID, Product: target.Product}, nil
	}
	return stats, nil
}
