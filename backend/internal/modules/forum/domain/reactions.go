package domain

import (
	"errors"
	"strconv"
	"strings"
)

// Reaction is one account's opinion of a post or comment (B-BR-FOR-17).
// The stored value is 1 for LIKE and -1 for DISLIKE; NONE means no row.
type Reaction int16

const (
	ReactionNone    Reaction = 0
	ReactionLike    Reaction = 1
	ReactionDislike Reaction = -1
)

// ReactionTargetType names what a reaction applies to.
type ReactionTargetType string

const (
	ReactionTargetPost    ReactionTargetType = "POST"
	ReactionTargetComment ReactionTargetType = "COMMENT"
)

var (
	ErrInvalidReaction    = errors.New("forum: invalid reaction")
	ErrInvalidReplySort   = errors.New("forum: invalid reply sort")
	ErrInvalidReplyCursor = errors.New("forum: invalid reply cursor")
)

// ParseReaction accepts the exact wire values LIKE, DISLIKE and NONE.
func ParseReaction(wire string) (Reaction, error) {
	switch wire {
	case "LIKE":
		return ReactionLike, nil
	case "DISLIKE":
		return ReactionDislike, nil
	case "NONE":
		return ReactionNone, nil
	}
	return ReactionNone, ErrInvalidReaction
}

// Wire returns the API spelling of the reaction.
func (r Reaction) Wire() string {
	switch r {
	case ReactionLike:
		return "LIKE"
	case ReactionDislike:
		return "DISLIKE"
	}
	return "NONE"
}

// ReactionCounts are the public aggregates of a post or comment.
type ReactionCounts struct {
	Likes    int64 `json:"like_count"`
	Dislikes int64 `json:"dislike_count"`
}

// Apply moves one account's reaction from one value to another, never below zero.
func (c ReactionCounts) Apply(from, to Reaction) ReactionCounts {
	if from == to {
		return c
	}
	switch from {
	case ReactionLike:
		c.Likes = max(c.Likes-1, 0)
	case ReactionDislike:
		c.Dislikes = max(c.Dislikes-1, 0)
	}
	switch to {
	case ReactionLike:
		c.Likes++
	case ReactionDislike:
		c.Dislikes++
	}
	return c
}

// ReplySort orders the top-level replies of a conversation (B-BR-FOR-18).
type ReplySort string

const (
	// ReplySortOldest is the original order and the API default for older clients.
	ReplySortOldest ReplySort = "oldest"
	ReplySortRecent ReplySort = "recent"
	// ReplySortLiked ranks by likes, newest first on ties.
	ReplySortLiked ReplySort = "liked"
	// ReplySortRelevant ranks by likes minus dislikes, newest first on ties.
	ReplySortRelevant ReplySort = "relevant"
)

// ParseReplySort maps the query value; empty keeps the oldest-first default.
func ParseReplySort(wire string) (ReplySort, error) {
	switch ReplySort(wire) {
	case "", ReplySortOldest:
		return ReplySortOldest, nil
	case ReplySortRecent, ReplySortLiked, ReplySortRelevant:
		return ReplySort(wire), nil
	}
	return "", ErrInvalidReplySort
}

// Ranked reports whether the sort keys on a reaction score besides the sequence.
func (s ReplySort) Ranked() bool {
	return s == ReplySortLiked || s == ReplySortRelevant
}

// ReplyCursor is the keyset position after the last returned reply.
type ReplyCursor struct {
	Score    *int64
	Sequence int64
}

// Score returns the ranking key of a comment under this sort.
func (s ReplySort) Score(c Comment) int64 {
	if s == ReplySortLiked {
		return c.LikeCount
	}
	return c.LikeCount - c.DislikeCount
}

// FormatReplyCursor encodes the position after comment c: "<sequence>" for
// chronological sorts, "<score>.<sequence>" for ranked ones.
func FormatReplyCursor(sort ReplySort, c Comment) string {
	seq := strconv.FormatInt(c.Sequence, 10)
	if !sort.Ranked() {
		return seq
	}
	return strconv.FormatInt(sort.Score(c), 10) + "." + seq
}

// ParseReplyCursor validates a cursor produced by FormatReplyCursor for the same sort.
func ParseReplyCursor(sort ReplySort, raw string) (ReplyCursor, error) {
	parsePositive := func(v string) (int64, bool) {
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil && n > 0 && strconv.FormatInt(n, 10) == v
	}
	if !sort.Ranked() {
		seq, ok := parsePositive(raw)
		if !ok {
			return ReplyCursor{}, ErrInvalidReplyCursor
		}
		return ReplyCursor{Sequence: seq}, nil
	}
	scoreRaw, seqRaw, found := strings.Cut(raw, ".")
	if !found {
		return ReplyCursor{}, ErrInvalidReplyCursor
	}
	score, err := strconv.ParseInt(scoreRaw, 10, 64)
	if err != nil || strconv.FormatInt(score, 10) != scoreRaw || (sort == ReplySortLiked && score < 0) {
		return ReplyCursor{}, ErrInvalidReplyCursor
	}
	seq, ok := parsePositive(seqRaw)
	if !ok {
		return ReplyCursor{}, ErrInvalidReplyCursor
	}
	return ReplyCursor{Score: &score, Sequence: seq}, nil
}
