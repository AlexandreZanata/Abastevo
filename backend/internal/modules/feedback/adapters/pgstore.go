package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	feedback "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/feedback"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// PGStore implements application.Store on a pgx pool.
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore wires the owned generated queries to a pool.
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(stripDashes(text))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("adapters: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func stamp(unix int64) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Unix(unix, 0).UTC(), Valid: true}
}

func unix(ts pgtype.Timestamptz) int64 {
	if !ts.Valid {
		return 0
	}
	return ts.Time.Unix()
}

func toRating(row feedback.FeedbackRating) domain.StoredRating {
	return domain.StoredRating{
		ID:        uuidString(row.ID),
		AccountID: uuidString(row.AccountID),
		StationID: uuidString(row.StationID),
		Product:   row.Product,
		Stars:     int(row.Stars),
		Revision:  int(row.Revision),
		CreatedAt: unix(row.CreatedAt),
		DeletedAt: unix(row.DeletedAt),
	}
}

func toStats(stationID string, product string, count, sum int64) domain.RatingStats {
	return domain.RatingStats{
		StationID: stationID,
		Product:   product,
		Count:     count,
		Sum:       sum,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// UpsertRating converges on equal stars, bumps the revision on change
// and inserts otherwise, refreshing the key stats from live rows in
// the same transaction. A lost unique race re-reads the winner under
// a fresh lock instead of forking a second row.
func (s *PGStore) UpsertRating(ctx context.Context, rec domain.StoredRating) (domain.StoredRating, bool, error) {
	id, err := mustUUID(rec.ID)
	if err != nil {
		return domain.StoredRating{}, false, err
	}
	accountID, err := mustUUID(rec.AccountID)
	if err != nil {
		return domain.StoredRating{}, false, err
	}
	stationID, err := mustUUID(rec.StationID)
	if err != nil {
		return domain.StoredRating{}, false, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		stored, created, retry, err := s.upsertOnce(ctx, id, accountID, stationID, rec)
		if err != nil {
			return domain.StoredRating{}, false, err
		}
		if !retry {
			return stored, created, nil
		}
	}
	return domain.StoredRating{}, false, domain.ErrRatingNotFound
}

func (s *PGStore) upsertOnce(ctx context.Context, id, accountID, stationID pgtype.UUID, rec domain.StoredRating) (domain.StoredRating, bool, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.StoredRating{}, false, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	row, err := q.LockRating(ctx, feedback.LockRatingParams{
		AccountID: accountID,
		StationID: stationID,
		Product:   rec.Product,
	})
	if err == nil {
		existing := toRating(row)
		if existing.Stars == rec.Stars {
			if err := s.refreshStats(ctx, q, rec.StationID, rec.Product); err != nil {
				return domain.StoredRating{}, false, false, err
			}
			if err := tx.Commit(ctx); err != nil {
				return domain.StoredRating{}, false, false, err
			}
			return existing, false, false, nil
		}
		if err := q.UpdateRatingStars(ctx, feedback.UpdateRatingStarsParams{
			Stars:    int16(rec.Stars),
			Revision: row.Revision + 1,
			ID:       row.ID,
		}); err != nil {
			return domain.StoredRating{}, false, false, err
		}
		existing.Stars = rec.Stars
		existing.Revision = int(row.Revision + 1)
		if err := s.refreshStats(ctx, q, rec.StationID, rec.Product); err != nil {
			return domain.StoredRating{}, false, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.StoredRating{}, false, false, err
		}
		return existing, false, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.StoredRating{}, false, false, err
	}
	if err := q.InsertRating(ctx, feedback.InsertRatingParams{
		ID:        id,
		AccountID: accountID,
		StationID: stationID,
		Product:   rec.Product,
		Stars:     int16(rec.Stars),
		Revision:  1,
		CreatedAt: stamp(rec.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return domain.StoredRating{}, false, true, nil
		}
		return domain.StoredRating{}, false, false, err
	}
	rec.Revision = 1
	if err := s.refreshStats(ctx, q, rec.StationID, rec.Product); err != nil {
		return domain.StoredRating{}, false, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isUniqueViolation(err) {
			return domain.StoredRating{}, false, true, nil
		}
		return domain.StoredRating{}, false, false, err
	}
	return rec, true, false, nil
}

func (s *PGStore) refreshStats(ctx context.Context, q *feedback.Queries, stationID, product string) error {
	id, err := mustUUID(stationID)
	if err != nil {
		return err
	}
	counts, err := q.CountSumLive(ctx, feedback.CountSumLiveParams{
		StationID: id,
		Product:   product,
	})
	if err != nil {
		return err
	}
	return q.UpsertStats(ctx, feedback.UpsertStatsParams{
		StationID:    id,
		Product:      product,
		RatingsCount: counts.RatingsCount,
		StarsSum:     counts.StarsSum,
		UpdatedAt:    stamp(time.Now().Unix()),
	})
}

// DeleteRating tombstones the live row and refreshes the key stats.
func (s *PGStore) DeleteRating(ctx context.Context, accountID, stationID, product string, nowUnix int64) (bool, error) {
	account, err := mustUUID(accountID)
	if err != nil {
		return false, nil
	}
	station, err := mustUUID(stationID)
	if err != nil {
		return false, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	n, err := q.TombstoneRating(ctx, feedback.TombstoneRatingParams{
		Now:       stamp(nowUnix),
		AccountID: account,
		StationID: station,
		Product:   product,
	})
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	if err := s.refreshStats(ctx, q, stationID, product); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// Stats returns the maintained aggregate for one key.
func (s *PGStore) Stats(ctx context.Context, stationID, product string) (domain.RatingStats, bool, error) {
	id, err := mustUUID(stationID)
	if err != nil {
		return domain.RatingStats{}, false, nil
	}
	row, err := feedback.New(s.pool).GetStats(ctx, feedback.GetStatsParams{
		StationID: id,
		Product:   product,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.RatingStats{}, false, nil
		}
		return domain.RatingStats{}, false, err
	}
	return toStats(stationID, product, row.RatingsCount, row.StarsSum), true, nil
}

// RebuildStats recomputes one key aggregate from live rows.
func (s *PGStore) RebuildStats(ctx context.Context, stationID, product string, _ int64) (domain.RatingStats, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.RatingStats{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	if err := s.refreshStats(ctx, q, stationID, product); err != nil {
		return domain.RatingStats{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.RatingStats{}, err
	}
	stats, found, err := s.Stats(ctx, stationID, product)
	if err != nil {
		return domain.RatingStats{}, err
	}
	if !found {
		return toStats(stationID, product, 0, 0), nil
	}
	return stats, nil
}
