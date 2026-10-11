package domain

import (
	"testing"
	"time"
)

// B-BR-CS01: a useful vote is not a truth signal. One active vote per
// account+contribution+revision; self-votes refuse; percentages use the
// exact denominator with the shared floor basis-point convention.
func TestHelpfulnessChoiceFreezesHelpfulPair(t *testing.T) {
	if !ValidChoice(ChoiceHelpful) || !ValidChoice(ChoiceNotHelpful) {
		t.Fatal("HELPFUL/NOT_HELPFUL must be the only frozen choices")
	}
	for _, other := range []string{"", "VALID", "INVALID", "LIKE", "DISLIKE", "helpful"} {
		if ValidChoice(Choice(other)) {
			t.Fatalf("choice %q must refuse: helpfulness never reuses confirmation/VALID votes", other)
		}
	}
}

func voteParams() VoteParams {
	return VoteParams{
		ID:                 "vote-1",
		ContributorRef:     "contrib-a",
		AuthorRef:          "contrib-b",
		ObservationID:      "obs-1",
		DecisionSequence:   3,
		Choice:             ChoiceHelpful,
		ClientSubmissionID: "sub-1",
		ReceivedAt:         time.Unix(1_700_000_000, 0).UTC(),
		PolicyVersion:      PolicyV1,
	}
}

func TestNewVoteFreezesOneHelpfulVote(t *testing.T) {
	v, evt, err := NewVote(voteParams())
	if err != nil {
		t.Fatalf("valid vote must construct: %v", err)
	}
	if v.Choice != ChoiceHelpful || v.DecisionSequence != 3 || !v.Live() {
		t.Fatalf("vote must freeze choice, sequence and live state: %+v", v)
	}
	if evt.VoteID != "vote-1" || evt.ObservationID != "obs-1" {
		t.Fatalf("event must reference the frozen vote: %+v", evt)
	}
}

func TestNewVoteRefusesSelfVote(t *testing.T) {
	p := voteParams()
	p.AuthorRef = p.ContributorRef
	if _, _, err := NewVote(p); err != ErrSelfVote {
		t.Fatalf("self-vote must refuse with ErrSelfVote, got %v", err)
	}
}

func TestNewVoteRefusesBlankFieldsAndBadChoice(t *testing.T) {
	base := voteParams()
	cases := []VoteParams{
		func() VoteParams { p := base; p.ID = ""; return p }(),
		func() VoteParams { p := base; p.ObservationID = ""; return p }(),
		func() VoteParams { p := base; p.ContributorRef = ""; return p }(),
		func() VoteParams { p := base; p.AuthorRef = ""; return p }(),
		func() VoteParams { p := base; p.ClientSubmissionID = ""; return p }(),
		func() VoteParams { p := base; p.Choice = "VALID"; return p }(),
		func() VoteParams { p := base; p.DecisionSequence = 0; return p }(),
		func() VoteParams { p := base; p.DecisionSequence = -1; return p }(),
	}
	for i, p := range cases {
		if _, _, err := NewVote(p); err == nil {
			t.Fatalf("case %d must refuse", i)
		}
	}
}

func TestComputeShareUsesFloorBasisPointsAndNullOnZero(t *testing.T) {
	// Pinned convention shared with feedback agreement math: floor.
	share, err := ComputeShare(1, 2)
	if err != nil {
		t.Fatalf("share must compute: %v", err)
	}
	if !share.HasVotes || share.BasisPoints != 3333 {
		t.Fatalf("floor(10000*1/3) must be 3333, got %+v", share)
	}
	empty, err := ComputeShare(0, 0)
	if err != nil {
		t.Fatalf("zero votes must not error: %v", err)
	}
	if empty.HasVotes {
		t.Fatalf("zero votes must yield no percentage: %+v", empty)
	}
	if _, err := ComputeShare(-1, 0); err == nil {
		t.Fatal("negative counts must refuse")
	}
}

func TestTallyFoldsThroughShare(t *testing.T) {
	tally := Tally{ObservationID: "obs-1", DecisionSequence: 3, Helpful: 2, NotHelpful: 1}
	share, err := tally.Share()
	if err != nil {
		t.Fatalf("tally must fold: %v", err)
	}
	if share.Helpful != 2 || share.NotHelpful != 1 || share.BasisPoints != 6666 {
		t.Fatalf("tally share mismatch: %+v", share)
	}
}

func TestComputeShareDisplayBoundaries(t *testing.T) {
	// CS06: display math is floor under review — shares never exceed
	// the true ratio, never show 0% with votes, and stay exact at the
	// basis-point edges the wire renders.
	vectors := []struct {
		helpful, notHelpful, wantBps int64
	}{
		{1, 0, 10000},
		{0, 1, 0},
		{1, 1, 5000},
		{1, 2, 3333},
		{2, 1, 6666},
		{9999, 1, 9999},
		{1, 9999, 1},
		{1 << 40, 1 << 40, 5000},
	}
	for _, v := range vectors {
		share, err := ComputeShare(v.helpful, v.notHelpful)
		if err != nil {
			t.Fatalf("%d/%d: %v", v.helpful, v.notHelpful, err)
		}
		if !share.HasVotes || share.BasisPoints != v.wantBps {
			t.Errorf("%d/%d = %+v, want %dbps", v.helpful, v.notHelpful, share, v.wantBps)
		}
		if share.BasisPoints < 0 || share.BasisPoints > 10000 {
			t.Errorf("%d/%d escapes basis points: %+v", v.helpful, v.notHelpful, share)
		}
	}
}
