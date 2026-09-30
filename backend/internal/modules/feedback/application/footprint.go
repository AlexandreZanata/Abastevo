package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// FeedbackExportFormat versions the owner archive envelope. The bytes
// are deterministic (sorted rows, fixed field order) so the recorded
// hash verifies the payload.
const FeedbackExportFormat = "feedback-export-v1"

// RatingExport is one owned rating row in the archive: the fact plus
// its revision/audit markers, never other accounts' rows.
type RatingExport struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
	Product   string `json:"product"`
	Stars     int    `json:"stars"`
	Revision  int    `json:"revision"`
	CreatedAt int64  `json:"created_at"`
	DeletedAt int64  `json:"deleted_at"`
}

// CommentExport is one owned comment/reply row: verbatim text with its
// revision/visibility/audit markers, never other authors' rows.
type CommentExport struct {
	ID         string `json:"id"`
	StationID  string `json:"station_id"`
	Product    string `json:"product"`
	ParentID   string `json:"parent_id"`
	Depth      int    `json:"depth"`
	Text       string `json:"text"`
	Revision   int    `json:"revision"`
	Visibility string `json:"visibility"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	DeletedAt  int64  `json:"deleted_at"`
}

// VoteExport is one owned validity vote: the choice bound to the
// comment revision voted on, never other voters' rows.
type VoteExport struct {
	ID              string `json:"id"`
	CommentID       string `json:"comment_id"`
	CommentRevision int    `json:"comment_revision"`
	Choice          string `json:"choice"`
	CreatedAt       int64  `json:"created_at"`
	DeletedAt       int64  `json:"deleted_at"`
}

// AccountFootprint is the redacted owner inventory: only rows whose
// account matches the requesting owner, sorted for determinism.
type AccountFootprint struct {
	AccountID string          `json:"account_id"`
	Ratings   []RatingExport  `json:"ratings"`
	Comments  []CommentExport `json:"comments"`
	Votes     []VoteExport    `json:"votes"`
}

// ExportEnvelope is the canonical archive: format version, owner,
// generation time and the redacted footprint.
type ExportEnvelope struct {
	Format      string           `json:"format"`
	AccountID   string           `json:"account_id"`
	GeneratedAt string           `json:"generated_at"`
	Footprint   AccountFootprint `json:"footprint"`
}

// EraseReport is the auditable receipt: tombstoned rows per kind plus
// the affected aggregates rebuilt from live rows.
type EraseReport struct {
	AccountID  string `json:"account_id"`
	Ratings    int64  `json:"ratings_tombstoned"`
	Comments   int64  `json:"comments_tombstoned"`
	Votes      int64  `json:"votes_tombstoned"`
	RatingKeys int    `json:"rating_keys_rebuilt"`
	Tallies    int    `json:"tallies_rebuilt"`
}

// ExportAccount assembles one owner's feedback footprint (F02/F07).
// Reads filter by account by construction: stores list only the
// requested owner's rows, so a reader bug returning foreign rows would
// need to forge the owner column to leak. Empty owners export empty
// (non-nil) sections, never null.
func (s *Service) ExportAccount(ctx context.Context, accountID string) (ExportEnvelope, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ExportEnvelope{}, domain.ErrTargetInvalid
	}
	ratings, err := s.Store.ListRatingsByAccount(ctx, accountID)
	if err != nil {
		return ExportEnvelope{}, err
	}
	comments, err := s.Comments.ListCommentsByAccount(ctx, accountID)
	if err != nil {
		return ExportEnvelope{}, err
	}
	votes, err := s.Votes.ListVotesByAccount(ctx, accountID)
	if err != nil {
		return ExportEnvelope{}, err
	}
	out := AccountFootprint{AccountID: accountID}
	for _, r := range ratings {
		if r.AccountID != accountID {
			continue
		}
		out.Ratings = append(out.Ratings, RatingExport{
			ID: r.ID, StationID: r.StationID, Product: r.Product,
			Stars: r.Stars, Revision: r.Revision,
			CreatedAt: r.CreatedAt, DeletedAt: r.DeletedAt,
		})
	}
	for _, c := range comments {
		if c.AccountID != accountID {
			continue
		}
		visibility := c.Visibility
		if visibility == "" {
			visibility = domain.VisibilityVisible
		}
		out.Comments = append(out.Comments, CommentExport{
			ID: c.ID, StationID: c.StationID, Product: c.Product,
			ParentID: c.ParentID, Depth: c.Depth, Text: c.Text,
			Revision: c.Revision, Visibility: visibility,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, DeletedAt: c.DeletedAt,
		})
	}
	for _, v := range votes {
		if v.AccountID != accountID {
			continue
		}
		out.Votes = append(out.Votes, VoteExport{
			ID: v.ID, CommentID: v.CommentID, CommentRevision: v.CommentRevision,
			Choice: v.Choice, CreatedAt: v.CreatedAt, DeletedAt: v.DeletedAt,
		})
	}
	if out.Ratings == nil {
		out.Ratings = []RatingExport{}
	}
	if out.Comments == nil {
		out.Comments = []CommentExport{}
	}
	if out.Votes == nil {
		out.Votes = []VoteExport{}
	}
	sort.Slice(out.Ratings, func(i, j int) bool { return out.Ratings[i].ID < out.Ratings[j].ID })
	sort.Slice(out.Comments, func(i, j int) bool { return out.Comments[i].ID < out.Comments[j].ID })
	sort.Slice(out.Votes, func(i, j int) bool { return out.Votes[i].ID < out.Votes[j].ID })
	var now string
	if s.Clock != nil {
		now = time.Unix(s.Clock.NowUnix(), 0).UTC().Format(time.RFC3339)
	} else {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	return ExportEnvelope{
		Format: FeedbackExportFormat, AccountID: accountID,
		GeneratedAt: now, Footprint: out,
	}, nil
}

// ExportAccountBytes marshals the envelope deterministically for the
// owner archive and operator export path.
func (s *Service) ExportAccountBytes(ctx context.Context, accountID string) ([]byte, error) {
	env, err := s.ExportAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// EraseAccount tombstones one account's live feedback and rebuilds the
// affected aggregates from live rows (F02/F07).
//
// Policy (frozen): suspension/deletion blocks new writes at the gate,
// past rows stay counted/readable until this explicit erasure runs.
// Erasure tombstones (history stays for audit, aggregates ignore it),
// recomputes every touched rating key and vote tally from live rows,
// and converges: a second run reports zero and rebuilds nothing new.
// Votes cast by the account vanish from tallies; votes by others on
// the account's comments stay (their denominator survives the author);
// replies by others on erased comments stay readable.
func (s *Service) EraseAccount(ctx context.Context, accountID string) (EraseReport, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return EraseReport{}, domain.ErrTargetInvalid
	}
	if s.Store == nil || s.Comments == nil || s.Votes == nil || s.Clock == nil {
		return EraseReport{}, domain.ErrGateRequired
	}
	now := s.Clock.NowUnix()
	var report EraseReport
	report.AccountID = accountID

	ratings, err := s.Store.ListRatingsByAccount(ctx, accountID)
	if err != nil {
		return EraseReport{}, err
	}
	type ratingKey struct{ station, product string }
	affectedRatings := map[ratingKey]bool{}
	for _, r := range ratings {
		if r.AccountID != accountID || !r.Live() {
			continue
		}
		deleted, err := s.Store.DeleteRating(ctx, accountID, r.StationID, r.Product, now)
		if err != nil {
			return EraseReport{}, err
		}
		if deleted {
			report.Ratings++
			affectedRatings[ratingKey{r.StationID, r.Product}] = true
		}
	}
	for k := range affectedRatings {
		if _, err := s.Store.RebuildStats(ctx, k.station, k.product, now); err != nil {
			return EraseReport{}, err
		}
		report.RatingKeys++
	}

	comments, err := s.Comments.ListCommentsByAccount(ctx, accountID)
	if err != nil {
		return EraseReport{}, err
	}
	for _, c := range comments {
		if c.AccountID != accountID || !c.Live() {
			continue
		}
		if err := s.Comments.DeleteComment(ctx, c.ID, accountID, now); err != nil {
			// A concurrent eraser may have tombstoned the same row
			// between our list and delete: that converges, not fails.
			if errors.Is(err, domain.ErrCommentNotFound) {
				continue
			}
			return EraseReport{}, err
		}
		report.Comments++
	}

	votes, err := s.Votes.ListVotesByAccount(ctx, accountID)
	if err != nil {
		return EraseReport{}, err
	}
	type tallyKey struct {
		comment  string
		revision int
	}
	affectedTallies := map[tallyKey]bool{}
	for _, v := range votes {
		if v.AccountID != accountID || !v.Live() {
			continue
		}
		removed, err := s.Votes.RemoveVote(ctx, accountID, v.CommentID, v.CommentRevision, now)
		if err != nil {
			return EraseReport{}, err
		}
		if removed {
			report.Votes++
			affectedTallies[tallyKey{v.CommentID, v.CommentRevision}] = true
		}
	}
	for k := range affectedTallies {
		if _, err := s.Votes.RebuildTally(ctx, k.comment, k.revision, now); err != nil {
			return EraseReport{}, err
		}
		report.Tallies++
	}
	return report, nil
}
