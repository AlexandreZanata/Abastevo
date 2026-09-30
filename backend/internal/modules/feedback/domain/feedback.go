package domain

import (
	"strings"
	"unicode/utf8"
)

// Frozen feedback bounds (P14-T01, B-BR-F02…F06). Values change only
// with a new product analysis, never to make a failing test pass.
const (
	// RatingMin/Max bound personal-experience stars: integers 1–5,
	// one current rating per account/target. No fractional stars.
	RatingMin = 1
	RatingMax = 5
	// MaxCommentScalars bounds comments and replies alike: at most
	// 280 Unicode scalar values after trimming and CRLF
	// normalization. Replies share the limit (F03).
	MaxCommentScalars = 280
	// MaxReplyDepth freezes the proposed MVP restriction (F04):
	// replies attach to a comment, never to another reply.
	MaxReplyDepth = 1
	// AgreementScale carries community agreement in integer basis
	// points: floor(10000*valid/total). Zero votes is null, never
	// 0% certainty (F06).
	AgreementScale = 10000
)

// Rating is one personal-experience score for an account/target.
type Rating int

// Valid reports whether the rating sits inside the frozen range.
func (r Rating) Valid() bool {
	return r >= RatingMin && r <= RatingMax
}

// Validate refuses out-of-range stars without coercion.
func (r Rating) Validate() error {
	if !r.Valid() {
		return ErrRatingOutOfRange
	}
	return nil
}

// CommentText is one normalized plain-text comment or reply. The
// zero value is invalid: comments are nonempty by construction.
type CommentText struct {
	// Text is the trimmed, CRLF-normalized form that persists.
	Text string
	// Scalars is the scalar count of Text, always within bound.
	Scalars int
}

// NormalizeComment trims surrounding whitespace and folds CRLF/CR
// line breaks to LF, mirroring the F03 counting rule shared with
// Kotlin/Swift.
func NormalizeComment(text string) string {
	trimmed := strings.TrimSpace(text)
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	return strings.ReplaceAll(trimmed, "\r", "\n")
}

// ParseComment validates raw input into a CommentText. Invalid UTF-8
// fails (Go strings cannot carry lone surrogates; Kotlin/Swift count
// per their own string models, documented in the fixtures). Empty
// and over-long inputs fail; nothing truncates silently.
func ParseComment(raw string) (CommentText, error) {
	if !utf8.ValidString(raw) {
		return CommentText{}, ErrTextInvalidEncoding
	}
	normalized := NormalizeComment(raw)
	if normalized == "" {
		return CommentText{}, ErrTextEmpty
	}
	scalars := utf8.RuneCountInString(normalized)
	if scalars > MaxCommentScalars {
		return CommentText{}, ErrTextTooLong
	}
	return CommentText{Text: normalized, Scalars: scalars}, nil
}

// Agreement is one frozen community-agreement snapshot: exact valid,
// invalid and total counts plus floor basis points, or null when no
// vote exists. Counts, never bare percentages, travel with every
// projection (F06).
type Agreement struct {
	Valid   int64
	Invalid int64
	// BasisPoints is floor(10000*Valid/Total), present only when
	// HasVotes. Zero votes leaves it absent (null on the wire).
	BasisPoints int64
	HasVotes    bool
}

// ComputeAgreement derives the agreement snapshot. Negative counts
// refuse: denominators come from constrained stores, never from
// client arithmetic.
func ComputeAgreement(valid, invalid int64) (Agreement, error) {
	if valid < 0 || invalid < 0 {
		return Agreement{}, ErrAgreementNegative
	}
	total := valid + invalid
	if total == 0 {
		return Agreement{HasVotes: false}, nil
	}
	return Agreement{
		Valid:       valid,
		Invalid:     invalid,
		BasisPoints: AgreementScale * valid / total,
		HasVotes:    true,
	}, nil
}

// Target binds feedback to an existing station plus a catalog fuel
// product (F01). Ratings describe personal experience, never
// laboratory quality or pump-price facts; feedback never overwrites
// official ANP records.
type Target struct {
	// StationID is the directory UUID of the reviewed station.
	StationID string
	// Product is the catalog fuel product wire name (e.g.
	// GASOLINE_ADDITIVED). Vocabulary ownership stays with the
	// directory/official modules; here only presence is frozen.
	Product string
}

// Validate refuses blank targets without consulting storage.
func (t Target) Validate() error {
	if strings.TrimSpace(t.StationID) == "" || strings.TrimSpace(t.Product) == "" {
		return ErrTargetInvalid
	}
	return nil
}

// Clock is the only time source portable logic may use; tests inject a fake.
type Clock interface {
	NowUnix() int64
}

// Vote choices for comment validity (F05). Authors never vote on
// their own comments; one live vote binds each account, comment and
// comment revision.
const (
	VoteValid   = "VALID"
	VoteInvalid = "INVALID"
)

// ValidVote reports whether choice names a frozen vote value.
// Matching is exact: clients send the wire constant verbatim.
func ValidVote(choice string) bool {
	return choice == VoteValid || choice == VoteInvalid
}

// StoredVote is one persisted validity vote bound to the comment
// revision voted on. Changes rewrite Choice in place; removals
// tombstone via DeletedAt. Old-revision votes stay restricted audit
// history when edits open a new denominator.
type StoredVote struct {
	ID              string
	AccountID       string
	CommentID       string
	CommentRevision int
	Choice          string
	CreatedAt       int64
	DeletedAt       int64
}

// Live reports whether the vote counts toward its tally.
func (v StoredVote) Live() bool {
	return v.DeletedAt == 0
}

// VoteTally is one exact per-revision count snapshot. Percentages
// derive through ComputeAgreement, keeping one math path for domain
// and projections alike.
type VoteTally struct {
	CommentID string
	Revision  int
	Valid     int64
	Invalid   int64
}

// Agreement folds the tally through the frozen basis-point math.
func (t VoteTally) Agreement() (Agreement, error) {
	return ComputeAgreement(t.Valid, t.Invalid)
}

// StoredComment is one persisted comment or reply revision. Depth 0
// marks a top-level comment (no parent); depth 1 marks a reply to a
// comment of the same station/fuel. Edits bump Revision in place with
// compare-and-swap; deletes tombstone via DeletedAt. History stays
// for audit and moderation; reads ignore tombstoned rows.
type StoredComment struct {
	ID        string
	AccountID string
	StationID string
	Product   string
	ParentID  string
	Depth     int
	Text      string
	Scalars   int
	Revision  int
	CreatedAt int64
	UpdatedAt int64
	DeletedAt int64
}

// Live reports whether the row is visible to reads.
func (c StoredComment) Live() bool {
	return c.DeletedAt == 0
}

// IsReply reports whether the row is a one-level reply.
func (c StoredComment) IsReply() bool {
	return c.ParentID != ""
}

// CommentView is the public read shape: opaque author alias, never
// addresses, provider subjects or precise locations.
type CommentView struct {
	ID        string
	Alias     string
	StationID string
	Product   string
	ParentID  string
	Depth     int
	Text      string
	Revision  int
	CreatedAt int64
	UpdatedAt int64
}

// StoredRating is one persisted rating revision. Exactly one live
// (non-tombstoned) row exists per account/station/product; edits bump
// Revision in place and deletes tombstone. History stays for audit;
// aggregates read live rows only.
type StoredRating struct {
	ID        string
	AccountID string
	StationID string
	Product   string
	Stars     int
	Revision  int
	CreatedAt int64
	DeletedAt int64
}

// Live reports whether the row counts toward aggregates.
func (r StoredRating) Live() bool {
	return r.DeletedAt == 0
}

// RatingStats is one rebuildable aggregate: exact count and star sum
// per station/product. Means derive at read time; the snapshot is
// recomputed from live rows, never accumulated across restarts.
type RatingStats struct {
	StationID string
	Product   string
	Count     int64
	Sum       int64
}

// MeanMilli returns the truncated integer mean in thousandths of a
// star (e.g. 4600 for 4.6), or false when no rating exists. Integer
// math only; display formatting stays outside the domain.
func (s RatingStats) MeanMilli() (int64, bool) {
	if s.Count <= 0 {
		return 0, false
	}
	return 1000 * s.Sum / s.Count, true
}
