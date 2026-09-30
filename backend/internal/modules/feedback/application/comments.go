package application

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// CommentStore is the persistence port behind comment use cases.
// Atomicity lives here: edits compare-and-swap the revision and
// deletes tombstone under one lock/transaction with author checks,
// so concurrent writers cannot fork a revision or erase foreign
// text. Reads resolve the opaque author alias.
type CommentStore interface {
	InsertComment(ctx context.Context, rec domain.StoredComment) error
	GetComment(ctx context.Context, id string) (domain.StoredComment, bool, error)
	EditComment(ctx context.Context, id, accountID, text string, scalars, expectedRevision int, nowUnix int64) (domain.StoredComment, error)
	DeleteComment(ctx context.Context, id, accountID string, nowUnix int64) error
	ViewComment(ctx context.Context, id string) (domain.CommentView, bool, error)
	ListComments(ctx context.Context, stationID, product string, afterUnix int64, afterID string, limit int) ([]domain.CommentView, error)
	ListReplies(ctx context.Context, parentID string, afterUnix int64, afterID string, limit int) ([]domain.CommentView, error)
}

// CommentPage is one bounded keyset page. NextCursor is empty at the
// end; cursors are opaque and tamper-evident (fail closed, never
// trusted).
type CommentPage struct {
	Items      []domain.CommentView
	NextCursor string
}

// SubmitComment records one top-level comment (F03). Only active
// accounts write; the gate refusal maps every account failure onto
// ErrAuthorForbidden so no oracle leaks account status.
func (s *Service) SubmitComment(ctx context.Context, accountID, stationID, product, raw string) (domain.StoredComment, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.StoredComment{}, domain.ErrTargetInvalid
	}
	return s.submit(ctx, accountID, stationID, product, "", raw)
}

func (s *Service) submit(ctx context.Context, accountID, stationID, product, parentID, raw string) (domain.StoredComment, error) {
	if err := s.authorize(ctx, accountID); err != nil {
		return domain.StoredComment{}, err
	}
	target := domain.Target{StationID: strings.TrimSpace(stationID), Product: strings.TrimSpace(product)}
	if err := target.Validate(); err != nil {
		return domain.StoredComment{}, err
	}
	exists, err := s.stationExists(ctx, target.StationID)
	if err != nil {
		return domain.StoredComment{}, err
	}
	if !exists {
		return domain.StoredComment{}, domain.ErrTargetInvalid
	}
	text, err := domain.ParseComment(raw)
	if err != nil {
		return domain.StoredComment{}, err
	}
	depth := 0
	if parentID != "" {
		parent, found, err := s.Comments.GetComment(ctx, parentID)
		if err != nil {
			return domain.StoredComment{}, err
		}
		if !found || !parent.Live() || parent.IsReply() ||
			parent.StationID != target.StationID || parent.Product != target.Product {
			return domain.StoredComment{}, domain.ErrParentInvalid
		}
		depth = 1
	}
	id, err := s.IDGen()
	if err != nil {
		return domain.StoredComment{}, err
	}
	now := s.Clock.NowUnix()
	rec := domain.StoredComment{
		ID:        id,
		AccountID: accountID,
		StationID: target.StationID,
		Product:   target.Product,
		ParentID:  parentID,
		Depth:     depth,
		Text:      text.Text,
		Scalars:   text.Scalars,
		Revision:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.Comments.InsertComment(ctx, rec); err != nil {
		return domain.StoredComment{}, err
	}
	return rec, nil
}

// Reply records one one-level reply (F04). The parent must be a live
// top-level comment of the same target; replies to replies, foreign
// targets and tombstoned parents refuse.
func (s *Service) Reply(ctx context.Context, accountID, stationID, product, parentID, raw string) (domain.StoredComment, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.StoredComment{}, domain.ErrTargetInvalid
	}
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return domain.StoredComment{}, domain.ErrParentInvalid
	}
	return s.submit(ctx, accountID, stationID, product, parentID, raw)
}

// EditComment replaces the text of an owned live comment, bumping the
// revision only when the expected revision still holds. Cross-author
// edits, tombstoned rows and stale revisions refuse distinctly.
func (s *Service) EditComment(ctx context.Context, accountID, commentID, raw string, expectedRevision int) (domain.StoredComment, error) {
	if strings.TrimSpace(accountID) == "" {
		return domain.StoredComment{}, domain.ErrTargetInvalid
	}
	if err := s.authorize(ctx, accountID); err != nil {
		return domain.StoredComment{}, err
	}
	text, err := domain.ParseComment(raw)
	if err != nil {
		return domain.StoredComment{}, err
	}
	return s.Comments.EditComment(ctx, strings.TrimSpace(commentID), accountID, text.Text, text.Scalars, expectedRevision, s.Clock.NowUnix())
}

// DeleteComment tombstones an owned comment. Unknown keys and foreign
// rows share the not-found shape; history stays for audit.
func (s *Service) DeleteComment(ctx context.Context, accountID, commentID string) error {
	if strings.TrimSpace(accountID) == "" {
		return domain.ErrTargetInvalid
	}
	if err := s.authorize(ctx, accountID); err != nil {
		return err
	}
	return s.Comments.DeleteComment(ctx, strings.TrimSpace(commentID), accountID, s.Clock.NowUnix())
}

// ViewComment resolves one live comment with its author alias.
func (s *Service) ViewComment(ctx context.Context, id string) (domain.CommentView, error) {
	view, found, err := s.Comments.ViewComment(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.CommentView{}, err
	}
	if !found {
		return domain.CommentView{}, domain.ErrCommentNotFound
	}
	return view, nil
}

// ListComments pages live top-level comments oldest-first. Public and
// anonymous: no gate, no identity beyond the alias.
func (s *Service) ListComments(ctx context.Context, stationID, product, cursor string, limit int) (CommentPage, error) {
	afterUnix, afterID, err := decodeCursor(cursor)
	if err != nil {
		return CommentPage{}, err
	}
	items, err := s.Comments.ListComments(ctx, strings.TrimSpace(stationID), strings.TrimSpace(product), afterUnix, afterID, clampLimit(limit))
	if err != nil {
		return CommentPage{}, err
	}
	return pageOf(items, limit), nil
}

// ListReplies pages live replies of one comment oldest-first.
func (s *Service) ListReplies(ctx context.Context, parentID, cursor string, limit int) (CommentPage, error) {
	afterUnix, afterID, err := decodeCursor(cursor)
	if err != nil {
		return CommentPage{}, err
	}
	items, err := s.Comments.ListReplies(ctx, strings.TrimSpace(parentID), afterUnix, afterID, clampLimit(limit))
	if err != nil {
		return CommentPage{}, err
	}
	return pageOf(items, limit), nil
}

func pageOf(items []domain.CommentView, limit int) CommentPage {
	want := clampLimit(limit)
	if len(items) < want {
		return CommentPage{Items: items}
	}
	last := items[len(items)-1]
	return CommentPage{Items: items, NextCursor: encodeCursor(last.CreatedAt, last.ID)}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func encodeCursor(createdUnix int64, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(createdUnix, 10) + ":" + id))
}

func decodeCursor(cursor string) (int64, string, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", domain.ErrTargetInvalid
	}
	at, id, ok := strings.Cut(string(raw), ":")
	if !ok || id == "" {
		return 0, "", domain.ErrTargetInvalid
	}
	nanos, err := strconv.ParseInt(at, 10, 64)
	if err != nil || nanos < 0 {
		return 0, "", domain.ErrTargetInvalid
	}
	return nanos, id, nil
}

func (s *Service) authorize(ctx context.Context, accountID string) error {
	if s.CheckAccount == nil || s.StationExists == nil {
		return domain.ErrGateRequired
	}
	if err := s.CheckAccount(ctx, accountID); err != nil {
		return domain.ErrAuthorForbidden
	}
	return nil
}

func (s *Service) stationExists(ctx context.Context, stationID string) (bool, error) {
	return s.StationExists(ctx, stationID)
}
