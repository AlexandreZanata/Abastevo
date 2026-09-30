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
		if err := incrementStats(ctx, q, stationID, rec.Product, 0, int64(rec.Stars)-int64(existing.Stars)); err != nil {
			return domain.StoredRating{}, false, false, err
		}
		existing.Stars = rec.Stars
		existing.Revision = int(row.Revision + 1)
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
	if err := incrementStats(ctx, q, stationID, rec.Product, 1, int64(rec.Stars)); err != nil {
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

func incrementStats(ctx context.Context, q *feedback.Queries, stationID pgtype.UUID, product string, countDelta, sumDelta int64) error {
	return q.IncrementStats(ctx, feedback.IncrementStatsParams{
		StationID:  stationID,
		Product:    product,
		CountDelta: countDelta,
		SumDelta:   sumDelta,
		UpdatedAt:  stamp(time.Now().Unix()),
	})
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
	row, err := q.LockRating(ctx, feedback.LockRatingParams{
		AccountID: account,
		StationID: station,
		Product:   product,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if _, err := q.TombstoneRating(ctx, feedback.TombstoneRatingParams{
		Now:       stamp(nowUnix),
		AccountID: account,
		StationID: station,
		Product:   product,
	}); err != nil {
		return false, err
	}
	if err := incrementStats(ctx, q, station, product, -1, -int64(row.Stars)); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func toCommentView(
	id, accountID, stationID pgtype.UUID,
	alias, product string,
	parentID pgtype.UUID,
	depth int16,
	text string,
	revision int32,
	created, updated pgtype.Timestamptz,
) domain.CommentView {
	parent := ""
	if parentID.Valid {
		parent = uuidString(parentID)
	}
	_ = accountID
	return domain.CommentView{
		ID:        uuidString(id),
		Alias:     alias,
		StationID: uuidString(stationID),
		Product:   product,
		ParentID:  parent,
		Depth:     int(depth),
		Text:      text,
		Revision:  int(revision),
		CreatedAt: unix(created),
		UpdatedAt: unix(updated),
	}
}

// InsertComment stores one validated comment or reply. Parent rules
// (live, top-level, same target) are enforced by the caller holding
// the parent row; the depth CHECK guards the schema invariant.
func (s *PGStore) InsertComment(ctx context.Context, rec domain.StoredComment) error {
	id, err := mustUUID(rec.ID)
	if err != nil {
		return err
	}
	accountID, err := mustUUID(rec.AccountID)
	if err != nil {
		return err
	}
	stationID, err := mustUUID(rec.StationID)
	if err != nil {
		return err
	}
	var parent pgtype.UUID
	depth := int16(0)
	if rec.ParentID != "" {
		parent, err = mustUUID(rec.ParentID)
		if err != nil {
			return err
		}
		depth = 1
	}
	return feedback.New(s.pool).InsertComment(ctx, feedback.InsertCommentParams{
		ID:        id,
		AccountID: accountID,
		StationID: stationID,
		Product:   rec.Product,
		ParentID:  parent,
		Depth:     depth,
		Text:      rec.Text,
		Now:       stamp(rec.CreatedAt),
	})
}

// GetComment resolves one comment by ID, including tombstoned rows.
func (s *PGStore) GetComment(ctx context.Context, id string) (domain.StoredComment, bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.StoredComment{}, false, nil
	}
	row, err := feedback.New(s.pool).LockComment(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.StoredComment{}, false, nil
		}
		return domain.StoredComment{}, false, err
	}
	parent := ""
	if row.ParentID.Valid {
		parent = uuidString(row.ParentID)
	}
	return domain.StoredComment{
		ID:        uuidString(row.ID),
		AccountID: uuidString(row.AccountID),
		StationID: uuidString(row.StationID),
		Product:   row.Product,
		ParentID:  parent,
		Depth:     int(row.Depth),
		Text:      row.Text,
		Revision:  int(row.Revision),
		CreatedAt: unix(row.CreatedAt),
		UpdatedAt: unix(row.UpdatedAt),
		DeletedAt: unix(row.DeletedAt),
	}, true, nil
}

// EditComment compare-and-swaps the text of an owned live comment.
func (s *PGStore) EditComment(ctx context.Context, id, accountID, text string, scalars, expectedRevision int, nowUnix int64) (domain.StoredComment, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.StoredComment{}, domain.ErrCommentNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.StoredComment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	row, err := q.LockComment(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.StoredComment{}, domain.ErrCommentNotFound
		}
		return domain.StoredComment{}, err
	}
	if row.DeletedAt.Valid {
		return domain.StoredComment{}, domain.ErrCommentNotFound
	}
	if uuidString(row.AccountID) != accountID {
		return domain.StoredComment{}, domain.ErrNotAuthor
	}
	n, err := q.UpdateCommentText(ctx, feedback.UpdateCommentTextParams{
		Text:             text,
		Now:              stamp(nowUnix),
		ID:               uid,
		ExpectedRevision: int32(expectedRevision),
	})
	if err != nil {
		return domain.StoredComment{}, err
	}
	if n == 0 {
		return domain.StoredComment{}, domain.ErrStaleRevision
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.StoredComment{}, err
	}
	updated, found, err := s.GetComment(ctx, id)
	if err != nil || !found {
		return domain.StoredComment{}, domain.ErrCommentNotFound
	}
	_ = scalars
	return updated, nil
}

// DeleteComment tombstones an owned live comment.
func (s *PGStore) DeleteComment(ctx context.Context, id, accountID string, nowUnix int64) error {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.ErrCommentNotFound
	}
	acc, err := mustUUID(accountID)
	if err != nil {
		return domain.ErrCommentNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	row, err := q.LockComment(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrCommentNotFound
		}
		return err
	}
	if row.DeletedAt.Valid {
		return domain.ErrCommentNotFound
	}
	if uuidString(row.AccountID) != accountID {
		return domain.ErrNotAuthor
	}
	if _, err := q.TombstoneComment(ctx, feedback.TombstoneCommentParams{
		Now:       stamp(nowUnix),
		ID:        uid,
		AccountID: acc,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ViewComment resolves one live comment with its author alias.
func (s *PGStore) ViewComment(ctx context.Context, id string) (domain.CommentView, bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.CommentView{}, false, nil
	}
	row, err := feedback.New(s.pool).GetCommentView(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CommentView{}, false, nil
		}
		return domain.CommentView{}, false, err
	}
	return toCommentView(
		row.ID, row.AccountID, row.StationID, row.Alias, row.Product,
		row.ParentID, row.Depth, row.Text, row.Revision,
		row.CreatedAt, row.UpdatedAt,
	), true, nil
}

// ListComments pages live top-level comments oldest-first.
func (s *PGStore) ListComments(ctx context.Context, stationID, product string, afterUnix int64, afterID string, limit int) ([]domain.CommentView, error) {
	station, err := mustUUID(stationID)
	if err != nil {
		return nil, nil
	}
	after := pgtype.UUID{}
	if afterID != "" {
		after, err = mustUUID(afterID)
		if err != nil {
			return nil, err
		}
	}
	rows, err := feedback.New(s.pool).ListTopLevel(ctx, feedback.ListTopLevelParams{
		StationID: station,
		Product:   product,
		AfterAt:   stamp(afterUnix),
		AfterID:   after,
		PageLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.CommentView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCommentView(
			r.ID, r.AccountID, r.StationID, r.Alias, r.Product,
			r.ParentID, r.Depth, r.Text, r.Revision,
			r.CreatedAt, r.UpdatedAt,
		))
	}
	return out, nil
}

// ListReplies pages live replies of one comment oldest-first.
func (s *PGStore) ListReplies(ctx context.Context, parentID string, afterUnix int64, afterID string, limit int) ([]domain.CommentView, error) {
	parent, err := mustUUID(parentID)
	if err != nil {
		return nil, nil
	}
	after := pgtype.UUID{}
	if afterID != "" {
		after, err = mustUUID(afterID)
		if err != nil {
			return nil, err
		}
	}
	rows, err := feedback.New(s.pool).ListReplies(ctx, feedback.ListRepliesParams{
		ParentID:  parent,
		AfterAt:   stamp(afterUnix),
		AfterID:   after,
		PageLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.CommentView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCommentView(
			r.ID, r.AccountID, r.StationID, r.Alias, r.Product,
			r.ParentID, r.Depth, r.Text, r.Revision,
			r.CreatedAt, r.UpdatedAt,
		))
	}
	return out, nil
}

func toVote(row feedback.FeedbackVote) domain.StoredVote {
	return domain.StoredVote{
		ID:              uuidString(row.ID),
		AccountID:       uuidString(row.AccountID),
		CommentID:       uuidString(row.CommentID),
		CommentRevision: int(row.CommentRevision),
		Choice:          row.Choice,
		CreatedAt:       unix(row.CreatedAt),
		DeletedAt:       unix(row.DeletedAt),
	}
}

func toVoteTally(commentID string, revision int, valid, invalid int64) domain.VoteTally {
	return domain.VoteTally{
		CommentID: commentID,
		Revision:  revision,
		Valid:     valid,
		Invalid:   invalid,
	}
}

// CastVote converges on equal choices, rewrites on change and inserts
// otherwise, refreshing the revision tally from live rows in the same
// transaction. A lost unique race re-reads the winner.
func (s *PGStore) CastVote(ctx context.Context, rec domain.StoredVote) (domain.StoredVote, bool, error) {
	id, err := mustUUID(rec.ID)
	if err != nil {
		return domain.StoredVote{}, false, err
	}
	accountID, err := mustUUID(rec.AccountID)
	if err != nil {
		return domain.StoredVote{}, false, err
	}
	commentID, err := mustUUID(rec.CommentID)
	if err != nil {
		return domain.StoredVote{}, false, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		stored, created, retry, err := s.castOnce(ctx, id, accountID, commentID, rec)
		if err != nil {
			return domain.StoredVote{}, false, err
		}
		if !retry {
			return stored, created, nil
		}
	}
	return domain.StoredVote{}, false, domain.ErrCommentNotFound
}

func (s *PGStore) castOnce(ctx context.Context, id, accountID, commentID pgtype.UUID, rec domain.StoredVote) (domain.StoredVote, bool, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.StoredVote{}, false, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	row, err := q.LockVote(ctx, feedback.LockVoteParams{
		AccountID:       accountID,
		CommentID:       commentID,
		CommentRevision: int32(rec.CommentRevision),
	})
	if err == nil {
		existing := toVote(row)
		if existing.Choice == rec.Choice {
			if err := tx.Commit(ctx); err != nil {
				return domain.StoredVote{}, false, false, err
			}
			return existing, false, false, nil
		}
		if err := q.UpdateVoteChoice(ctx, feedback.UpdateVoteChoiceParams{
			Choice: rec.Choice,
			ID:     row.ID,
		}); err != nil {
			return domain.StoredVote{}, false, false, err
		}
		// A change moves one count across sides: remove the old
		// choice, add the new one.
		oldValid, oldInvalid := voteDelta(existing.Choice)
		newValid, newInvalid := voteDelta(rec.Choice)
		if err := incrementTally(ctx, q, commentID, rec.CommentRevision, newValid-oldValid, newInvalid-oldInvalid); err != nil {
			return domain.StoredVote{}, false, false, err
		}
		existing.Choice = rec.Choice
		if err := tx.Commit(ctx); err != nil {
			return domain.StoredVote{}, false, false, err
		}
		return existing, false, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.StoredVote{}, false, false, err
	}
	if err := q.InsertVote(ctx, feedback.InsertVoteParams{
		ID:              id,
		AccountID:       accountID,
		CommentID:       commentID,
		CommentRevision: int32(rec.CommentRevision),
		Choice:          rec.Choice,
		CreatedAt:       stamp(rec.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return domain.StoredVote{}, false, true, nil
		}
		return domain.StoredVote{}, false, false, err
	}
	newValid, newInvalid := voteDelta(rec.Choice)
	if err := incrementTally(ctx, q, commentID, rec.CommentRevision, newValid, newInvalid); err != nil {
		return domain.StoredVote{}, false, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		if isUniqueViolation(err) {
			return domain.StoredVote{}, false, true, nil
		}
		return domain.StoredVote{}, false, false, err
	}
	rec.ID = uuidString(id)
	return rec, true, false, nil
}

// voteDelta maps a newly stored choice onto (valid, invalid) deltas:
// the new choice adds one, the replaced choice (if any) removes one.
func voteDelta(choice string) (int64, int64) {
	if choice == domain.VoteValid {
		return 1, 0
	}
	return 0, 1
}

func incrementTally(ctx context.Context, q *feedback.Queries, commentID pgtype.UUID, revision int, validDelta, invalidDelta int64) error {
	return q.IncrementTally(ctx, feedback.IncrementTallyParams{
		CommentID:    commentID,
		Revision:     int32(revision),
		ValidDelta:   validDelta,
		InvalidDelta: invalidDelta,
		UpdatedAt:    stamp(time.Now().Unix()),
	})
}

func (s *PGStore) retally(ctx context.Context, q *feedback.Queries, commentID string, revision int) error {
	id, err := mustUUID(commentID)
	if err != nil {
		return err
	}
	counts, err := q.CountVotes(ctx, feedback.CountVotesParams{
		CommentID:       id,
		CommentRevision: int32(revision),
	})
	if err != nil {
		return err
	}
	return q.UpsertTally(ctx, feedback.UpsertTallyParams{
		CommentID:    id,
		Revision:     int32(revision),
		ValidCount:   counts.ValidCount,
		InvalidCount: counts.InvalidCount,
		UpdatedAt:    stamp(time.Now().Unix()),
	})
}

// RemoveVote tombstones the live vote; absent votes converge no-op.
func (s *PGStore) RemoveVote(ctx context.Context, accountID, commentID string, revision int, nowUnix int64) (bool, error) {
	account, err := mustUUID(accountID)
	if err != nil {
		return false, nil
	}
	comment, err := mustUUID(commentID)
	if err != nil {
		return false, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	row, err := q.LockVote(ctx, feedback.LockVoteParams{
		AccountID:       account,
		CommentID:       comment,
		CommentRevision: int32(revision),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if _, err := q.TombstoneVote(ctx, feedback.TombstoneVoteParams{
		Now:             stamp(nowUnix),
		AccountID:       account,
		CommentID:       comment,
		CommentRevision: int32(revision),
	}); err != nil {
		return false, err
	}
	oldValid, oldInvalid := voteDelta(row.Choice)
	if err := incrementTally(ctx, q, comment, revision, -oldValid, -oldInvalid); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// Tally returns the maintained per-revision counts.
func (s *PGStore) Tally(ctx context.Context, commentID string, revision int) (domain.VoteTally, bool, error) {
	id, err := mustUUID(commentID)
	if err != nil {
		return domain.VoteTally{}, false, nil
	}
	row, err := feedback.New(s.pool).GetTally(ctx, feedback.GetTallyParams{
		CommentID: id,
		Revision:  int32(revision),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.VoteTally{}, false, nil
		}
		return domain.VoteTally{}, false, err
	}
	return toVoteTally(commentID, revision, row.ValidCount, row.InvalidCount), true, nil
}

// RebuildTally recomputes one revision tally from live rows.
func (s *PGStore) RebuildTally(ctx context.Context, commentID string, revision int, _ int64) (domain.VoteTally, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.VoteTally{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := feedback.New(tx)
	if err := s.retally(ctx, q, commentID, revision); err != nil {
		return domain.VoteTally{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.VoteTally{}, err
	}
	tally, found, err := s.Tally(ctx, commentID, revision)
	if err != nil {
		return domain.VoteTally{}, err
	}
	if !found {
		return toVoteTally(commentID, revision, 0, 0), nil
	}
	return tally, nil
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
