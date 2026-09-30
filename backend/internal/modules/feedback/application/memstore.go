package application

import (
	"context"
	"sync"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// MemStore is the mutex-guarded memory Store for unit tests. It honors
// the exact atomicity contract Postgres implements: UpsertRating,
// DeleteRating and RebuildStats complete under one lock with stats
// recomputed from live rows, so the same interleavings are covered
// here first.
type MemStore struct {
	mu      sync.Mutex
	ratings map[string]domain.StoredRating
	stats   map[string]domain.RatingStats
}

// NewMemStore returns an empty memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		ratings: map[string]domain.StoredRating{},
		stats:   map[string]domain.RatingStats{},
	}
}

func ratingKey(accountID, stationID, product string) string {
	return accountID + "\x00" + stationID + "\x00" + product
}

func targetKey(stationID, product string) string {
	return stationID + "\x00" + product
}

// UpsertRating converges on equal stars, bumps the revision on change
// and inserts otherwise, refreshing the key stats from live rows.
func (m *MemStore) UpsertRating(_ context.Context, rec domain.StoredRating) (domain.StoredRating, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ratingKey(rec.AccountID, rec.StationID, rec.Product)
	if existing, ok := m.ratings[key]; ok && existing.Live() {
		if existing.Stars == rec.Stars {
			return existing, false, nil
		}
		existing.Stars = rec.Stars
		existing.Revision++
		m.ratings[key] = existing
		m.refreshLocked(rec.StationID, rec.Product)
		return existing, false, nil
	}
	rec.Revision = 1
	m.ratings[key] = rec
	m.refreshLocked(rec.StationID, rec.Product)
	return rec, true, nil
}

// DeleteRating tombstones the live row and refreshes the key stats.
func (m *MemStore) DeleteRating(_ context.Context, accountID, stationID, product string, nowUnix int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ratingKey(accountID, stationID, product)
	existing, ok := m.ratings[key]
	if !ok || !existing.Live() {
		return false, nil
	}
	existing.DeletedAt = nowUnix
	m.ratings[key] = existing
	m.refreshLocked(stationID, product)
	return true, nil
}

// Stats returns the maintained aggregate for one key.
func (m *MemStore) Stats(_ context.Context, stationID, product string) (domain.RatingStats, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats, ok := m.stats[targetKey(stationID, product)]
	return stats, ok, nil
}

// RebuildStats recomputes one key aggregate from live rows.
func (m *MemStore) RebuildStats(_ context.Context, stationID, product string, _ int64) (domain.RatingStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshLocked(stationID, product)
	return m.stats[targetKey(stationID, product)], nil
}

func (m *MemStore) refreshLocked(stationID, product string) {
	var count, sum int64
	for _, r := range m.ratings {
		if r.StationID == stationID && r.Product == product && r.Live() {
			count++
			sum += int64(r.Stars)
		}
	}
	m.stats[targetKey(stationID, product)] = domain.RatingStats{
		StationID: stationID,
		Product:   product,
		Count:     count,
		Sum:       sum,
	}
}
