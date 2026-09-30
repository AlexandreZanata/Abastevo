package application

import (
	"context"
	"sort"
	"sync"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// MemStore is the mutex-guarded memory Store for unit tests. It honors
// the exact atomicity contract Postgres implements: rating and comment
// mutations complete under one lock with author checks, so the same
// interleavings are covered here first. One struct implements both the
// rating Store and the CommentStore ports.
type MemStore struct {
	mu       sync.Mutex
	ratings  map[string]domain.StoredRating
	stats    map[string]domain.RatingStats
	comments map[string]domain.StoredComment
	aliases  map[string]string
}

// NewMemStore returns an empty memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		ratings:  map[string]domain.StoredRating{},
		stats:    map[string]domain.RatingStats{},
		comments: map[string]domain.StoredComment{},
		aliases:  map[string]string{},
	}
}

// SetAlias pins the opaque public alias resolved for one account in
// comment views (test-only; Postgres joins the accounts table).
func (m *MemStore) SetAlias(accountID, alias string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aliases[accountID] = alias
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

// InsertComment stores one validated comment or reply.
func (m *MemStore) InsertComment(_ context.Context, rec domain.StoredComment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.comments[rec.ID] = rec
	if _, ok := m.aliases[rec.AccountID]; !ok {
		m.aliases[rec.AccountID] = "alias-" + rec.AccountID
	}
	return nil
}

// GetComment resolves one comment by ID, including tombstoned rows
// so callers distinguish missing from deleted.
func (m *MemStore) GetComment(_ context.Context, id string) (domain.StoredComment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.comments[id]
	return rec, ok, nil
}

// EditComment compare-and-swaps the text of an owned live comment.
func (m *MemStore) EditComment(_ context.Context, id, accountID, text string, scalars, expectedRevision int, nowUnix int64) (domain.StoredComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.comments[id]
	if !ok || !rec.Live() {
		return domain.StoredComment{}, domain.ErrCommentNotFound
	}
	if rec.AccountID != accountID {
		return domain.StoredComment{}, domain.ErrNotAuthor
	}
	if rec.Revision != expectedRevision {
		return domain.StoredComment{}, domain.ErrStaleRevision
	}
	rec.Text = text
	rec.Scalars = scalars
	rec.Revision++
	rec.UpdatedAt = nowUnix
	m.comments[id] = rec
	return rec, nil
}

// DeleteComment tombstones an owned live comment.
func (m *MemStore) DeleteComment(_ context.Context, id, accountID string, nowUnix int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.comments[id]
	if !ok || !rec.Live() {
		return domain.ErrCommentNotFound
	}
	if rec.AccountID != accountID {
		return domain.ErrNotAuthor
	}
	rec.DeletedAt = nowUnix
	m.comments[id] = rec
	return nil
}

// ViewComment resolves one live comment with its author alias.
func (m *MemStore) ViewComment(_ context.Context, id string) (domain.CommentView, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.comments[id]
	if !ok || !rec.Live() {
		return domain.CommentView{}, false, nil
	}
	return m.viewLocked(rec), true, nil
}

// ListComments pages live top-level comments oldest-first.
func (m *MemStore) ListComments(_ context.Context, stationID, product string, afterUnix int64, afterID string, limit int) ([]domain.CommentView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.CommentView
	for _, rec := range m.comments {
		if rec.Live() && rec.ParentID == "" && rec.StationID == stationID && rec.Product == product &&
			(rec.CreatedAt > afterUnix || (rec.CreatedAt == afterUnix && rec.ID > afterID)) {
			out = append(out, m.viewLocked(rec))
		}
	}
	return takeViews(out, limit), nil
}

// ListReplies pages live replies of one comment oldest-first.
func (m *MemStore) ListReplies(_ context.Context, parentID string, afterUnix int64, afterID string, limit int) ([]domain.CommentView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.CommentView
	for _, rec := range m.comments {
		if rec.Live() && rec.ParentID == parentID &&
			(rec.CreatedAt > afterUnix || (rec.CreatedAt == afterUnix && rec.ID > afterID)) {
			out = append(out, m.viewLocked(rec))
		}
	}
	return takeViews(out, limit), nil
}

func (m *MemStore) viewLocked(rec domain.StoredComment) domain.CommentView {
	alias, ok := m.aliases[rec.AccountID]
	if !ok {
		alias = "alias-" + rec.AccountID
	}
	return domain.CommentView{
		ID:        rec.ID,
		Alias:     alias,
		StationID: rec.StationID,
		Product:   rec.Product,
		ParentID:  rec.ParentID,
		Depth:     rec.Depth,
		Text:      rec.Text,
		Revision:  rec.Revision,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
}

func takeViews(out []domain.CommentView, limit int) []domain.CommentView {
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > limit {
		return out[:limit]
	}
	return out
}
