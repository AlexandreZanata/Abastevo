package domain

import (
	"errors"
	"testing"
)

func TestParseReaction(t *testing.T) {
	cases := map[string]Reaction{"LIKE": ReactionLike, "DISLIKE": ReactionDislike, "NONE": ReactionNone}
	for wire, want := range cases {
		got, err := ParseReaction(wire)
		if err != nil || got != want {
			t.Fatalf("ParseReaction(%q) = %v, %v; want %v", wire, got, err, want)
		}
		if got.Wire() != wire {
			t.Fatalf("%v.Wire() = %q; want %q", got, got.Wire(), wire)
		}
	}
	for _, bad := range []string{"", "like", "LOVE", "1", " LIKE"} {
		if _, err := ParseReaction(bad); !errors.Is(err, ErrInvalidReaction) {
			t.Fatalf("ParseReaction(%q) error = %v; want ErrInvalidReaction", bad, err)
		}
	}
}

func TestParseReplySort(t *testing.T) {
	cases := map[string]ReplySort{"": ReplySortOldest, "oldest": ReplySortOldest, "recent": ReplySortRecent,
		"liked": ReplySortLiked, "relevant": ReplySortRelevant}
	for wire, want := range cases {
		got, err := ParseReplySort(wire)
		if err != nil || got != want {
			t.Fatalf("ParseReplySort(%q) = %v, %v; want %v", wire, got, err, want)
		}
	}
	if _, err := ParseReplySort("top"); !errors.Is(err, ErrInvalidReplySort) {
		t.Fatalf("ParseReplySort(top) error = %v; want ErrInvalidReplySort", err)
	}
}

func TestReplyCursorRoundTrip(t *testing.T) {
	c := Comment{Sequence: 42, LikeCount: 9, DislikeCount: 3}
	cases := []struct {
		sort  ReplySort
		raw   string
		score *int64
	}{
		{ReplySortOldest, "42", nil},
		{ReplySortRecent, "42", nil},
		{ReplySortLiked, "9.42", ptr(int64(9))},
		{ReplySortRelevant, "6.42", ptr(int64(6))},
	}
	for _, tc := range cases {
		raw := FormatReplyCursor(tc.sort, c)
		if raw != tc.raw {
			t.Fatalf("FormatReplyCursor(%s) = %q; want %q", tc.sort, raw, tc.raw)
		}
		cursor, err := ParseReplyCursor(tc.sort, raw)
		if err != nil {
			t.Fatalf("ParseReplyCursor(%s, %q): %v", tc.sort, raw, err)
		}
		if cursor.Sequence != 42 || (tc.score == nil) != (cursor.Score == nil) || (tc.score != nil && *cursor.Score != *tc.score) {
			t.Fatalf("ParseReplyCursor(%s, %q) = %+v", tc.sort, raw, cursor)
		}
	}
}

func TestReplyCursorRejectsMalformedInput(t *testing.T) {
	bad := []struct {
		sort ReplySort
		raw  string
	}{
		{ReplySortOldest, "0"}, {ReplySortOldest, "-1"}, {ReplySortOldest, "1.2"}, {ReplySortRecent, "x"},
		{ReplySortLiked, "42"}, {ReplySortLiked, "-1.42"}, {ReplySortLiked, "1.0"}, {ReplySortLiked, "1.2.3"},
		{ReplySortRelevant, "a.1"}, {ReplySortRelevant, "99999999999999999999.1"},
	}
	for _, tc := range bad {
		if _, err := ParseReplyCursor(tc.sort, tc.raw); !errors.Is(err, ErrInvalidReplyCursor) {
			t.Fatalf("ParseReplyCursor(%s, %q) error = %v; want ErrInvalidReplyCursor", tc.sort, tc.raw, err)
		}
	}
	// Net reactions can be negative under relevant sort.
	if c, err := ParseReplyCursor(ReplySortRelevant, "-3.7"); err != nil || *c.Score != -3 || c.Sequence != 7 {
		t.Fatalf("negative relevant cursor = %+v, %v", c, err)
	}
}

func TestReactionCountsApply(t *testing.T) {
	steps := []struct {
		from, to        Reaction
		likes, dislikes int64
	}{
		{ReactionNone, ReactionLike, 1, 0},
		{ReactionLike, ReactionLike, 1, 0},
		{ReactionLike, ReactionDislike, 0, 1},
		{ReactionDislike, ReactionNone, 0, 0},
		{ReactionNone, ReactionNone, 0, 0},
	}
	var counts ReactionCounts
	for i, s := range steps {
		counts = counts.Apply(s.from, s.to)
		if counts.Likes != s.likes || counts.Dislikes != s.dislikes {
			t.Fatalf("step %d: %+v; want %d/%d", i, counts, s.likes, s.dislikes)
		}
	}
	// Never below zero even if a stale client reports a reaction it no longer holds.
	if got := (ReactionCounts{}).Apply(ReactionLike, ReactionNone); got.Likes != 0 {
		t.Fatalf("counts went negative: %+v", got)
	}
}

func ptr[T any](v T) *T { return &v }
