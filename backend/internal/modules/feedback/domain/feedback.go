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
