package application

import (
	"context"
	"sort"
	"strconv"
	"sync"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// MemStore is the mutex-guarded memory Store for unit tests. It honors
// the exact atomicity contract Postgres implements: rating, comment
// and vote mutations complete under one lock with author checks, so
// the same interleavings are covered here first. One struct implements
// the rating Store, the CommentStore and the VoteStore ports.
type MemStore struct {
	mu       sync.Mutex
	ratings  map[string]domain.StoredRating
	stats    map[string]domain.RatingStats
	comments map[string]domain.StoredComment
	aliases  map[string]string
	votes    map[string]domain.StoredVote
	tallies  map[string]domain.VoteTally
}

// NewMemStore returns an empty memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		ratings:  map[string]domain.StoredRating{},
		stats:    map[string]domain.RatingStats{},
		comments: map[string]domain.StoredComment{},
		aliases:  map[string]string{},
		votes:    map[string]domain.StoredVote{},
		tallies:  map[string]domain.VoteTally{},
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
// and inserts otherwise, applying signed stat deltas atomically like
// the Postgres lane (full recomputation stays in RebuildStats).
func (m *MemStore) UpsertRating(_ context.Context, rec domain.StoredRating) (domain.StoredRating, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ratingKey(rec.AccountID, rec.StationID, rec.Product)
	if existing, ok := m.ratings[key]; ok && existing.Live() {
		if existing.Stars == rec.Stars {
			return existing, false, nil
		}
		m.bumpStatsLocked(rec.StationID, rec.Product, 0, int64(rec.Stars)-int64(existing.Stars))
		existing.Stars = rec.Stars
		existing.Revision++
		m.ratings[key] = existing
		return existing, false, nil
	}
	rec.Revision = 1
	m.ratings[key] = rec
	m.bumpStatsLocked(rec.StationID, rec.Product, 1, int64(rec.Stars))
	return rec, true, nil
}

// DeleteRating tombstones the live row and backs its contribution out.
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
	m.bumpStatsLocked(stationID, product, -1, -int64(existing.Stars))
	return true, nil
}

func (m *MemStore) bumpStatsLocked(stationID, product string, countDelta, sumDelta int64) {
	key := targetKey(stationID, product)
	stats := m.stats[key]
	stats.StationID = stationID
	stats.Product = product
	stats.Count += countDelta
	stats.Sum += sumDelta
	m.stats[key] = stats
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
func hiddenLocked(rec domain.StoredComment) bool {
	return rec.Visibility == domain.VisibilityHidden
}

func (m *MemStore) ViewComment(_ context.Context, id string) (domain.CommentView, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.comments[id]
	if !ok || !rec.Live() || hiddenLocked(rec) {
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
		if rec.Live() && !hiddenLocked(rec) && rec.ParentID == "" && rec.StationID == stationID && rec.Product == product &&
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
		if rec.Live() && !hiddenLocked(rec) && rec.ParentID == parentID &&
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

func voteKey(accountID, commentID string, revision int) string {
	return accountID + "\x00" + commentID + "\x00" + strconv.Itoa(revision)
}

func tallyKey(commentID string, revision int) string {
	return commentID + "\x00" + strconv.Itoa(revision)
}

func voteDeltas(choice string) (int64, int64) {
	if choice == domain.VoteValid {
		return 1, 0
	}
	return 0, 1
}

func (m *MemStore) bumpTallyLocked(commentID string, revision int, validDelta, invalidDelta int64) {
	key := tallyKey(commentID, revision)
	tally := m.tallies[key]
	tally.CommentID = commentID
	tally.Revision = revision
	tally.Valid += validDelta
	tally.Invalid += invalidDelta
	m.tallies[key] = tally
}

// CastVote converges on equal choices, rewrites on change and inserts
// otherwise, applying signed tally deltas like the Postgres lane.
func (m *MemStore) CastVote(_ context.Context, rec domain.StoredVote) (domain.StoredVote, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := voteKey(rec.AccountID, rec.CommentID, rec.CommentRevision)
	if existing, ok := m.votes[key]; ok && existing.Live() {
		if existing.Choice == rec.Choice {
			return existing, false, nil
		}
		oldValid, oldInvalid := voteDeltas(existing.Choice)
		newValid, newInvalid := voteDeltas(rec.Choice)
		m.bumpTallyLocked(rec.CommentID, rec.CommentRevision, newValid-oldValid, newInvalid-oldInvalid)
		existing.Choice = rec.Choice
		m.votes[key] = existing
		return existing, false, nil
	}
	m.votes[key] = rec
	newValid, newInvalid := voteDeltas(rec.Choice)
	m.bumpTallyLocked(rec.CommentID, rec.CommentRevision, newValid, newInvalid)
	return rec, true, nil
}

// RemoveVote tombstones the live vote and backs its contribution out;
// absent votes converge no-op.
func (m *MemStore) RemoveVote(_ context.Context, accountID, commentID string, revision int, nowUnix int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := voteKey(accountID, commentID, revision)
	existing, ok := m.votes[key]
	if !ok || !existing.Live() {
		return false, nil
	}
	existing.DeletedAt = nowUnix
	m.votes[key] = existing
	oldValid, oldInvalid := voteDeltas(existing.Choice)
	m.bumpTallyLocked(commentID, revision, -oldValid, -oldInvalid)
	return true, nil
}

// Tally returns the maintained per-revision counts.
func (m *MemStore) Tally(_ context.Context, commentID string, revision int) (domain.VoteTally, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tally, ok := m.tallies[tallyKey(commentID, revision)]
	return tally, ok, nil
}

// RebuildTally recomputes one revision tally from live rows.
func (m *MemStore) RebuildTally(_ context.Context, commentID string, revision int, _ int64) (domain.VoteTally, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retallyLocked(commentID, revision)
	return m.tallies[tallyKey(commentID, revision)], nil
}

func (m *MemStore) retallyLocked(commentID string, revision int) {
	var valid, invalid int64
	for _, v := range m.votes {
		if v.CommentID == commentID && v.CommentRevision == revision && v.Live() {
			if v.Choice == domain.VoteValid {
				valid++
			} else {
				invalid++
			}
		}
	}
	m.tallies[tallyKey(commentID, revision)] = domain.VoteTally{
		CommentID: commentID,
		Revision:  revision,
		Valid:     valid,
		Invalid:   invalid,
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

// ListRatingsByAccount returns every rating row of one account,
// including tombstoned history, ordered for deterministic export.
func (m *MemStore) ListRatingsByAccount(_ context.Context, accountID string) ([]domain.StoredRating, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.StoredRating
	for _, r := range m.ratings {
		if r.AccountID == accountID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StationID != out[j].StationID {
			return out[i].StationID < out[j].StationID
		}
		if out[i].Product != out[j].Product {
			return out[i].Product < out[j].Product
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ListCommentsByAccount returns every comment/reply row of one
// account, including tombstoned history, ordered for export.
func (m *MemStore) ListCommentsByAccount(_ context.Context, accountID string) ([]domain.StoredComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.StoredComment
	for _, c := range m.comments {
		if c.AccountID == accountID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ListVotesByAccount returns every vote row of one account,
// including tombstoned history, ordered for export.
func (m *MemStore) ListVotesByAccount(_ context.Context, accountID string) ([]domain.StoredVote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.StoredVote
	for _, v := range m.votes {
		if v.AccountID == accountID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CommentID != out[j].CommentID {
			return out[i].CommentID < out[j].CommentID
		}
		if out[i].CommentRevision != out[j].CommentRevision {
			return out[i].CommentRevision < out[j].CommentRevision
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// FlagComment marks a reported comment flagged, once. Only visible
// rows transition; hidden, flagged and tombstoned rows are untouched.
func (m *MemStore) FlagComment(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.comments[id]
	if !ok {
		return domain.ErrCommentNotFound
	}
	if rec.Visibility == "" {
		rec.Visibility = domain.VisibilityVisible
	}
	if rec.Visibility == domain.VisibilityVisible {
		rec.Visibility = domain.VisibilityFlagged
		m.comments[id] = rec
	}
	return nil
}

// SetVisibility moves one live comment along the moderator lane.
func (m *MemStore) SetVisibility(_ context.Context, id, visibility string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if visibility != domain.VisibilityVisible && visibility != domain.VisibilityFlagged &&
		visibility != domain.VisibilityHidden {
		return domain.ErrVisibilityInvalid
	}
	rec, ok := m.comments[id]
	if !ok || !rec.Live() {
		return domain.ErrCommentNotFound
	}
	rec.Visibility = visibility
	m.comments[id] = rec
	return nil
}
