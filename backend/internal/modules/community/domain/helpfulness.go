package domain

import (
	"errors"
	"strings"
	"time"
)

// Helpfulness choices (B-BR-CS01). A useful vote is not a truth signal:
// these values never validate a price and never reuse confirmation,
// dispute or feedback VALID/INVALID meanings.
type Choice string

const (
	ChoiceHelpful    Choice = "HELPFUL"
	ChoiceNotHelpful Choice = "NOT_HELPFUL"
)

// ShareScale is the frozen basis-point denominator shared with the
// feedback agreement convention: floor(10000*helpful/total).
const ShareScale = 10000

var (
	ErrVoteChoiceInvalid = errors.New("community: helpfulness choice must be HELPFUL or NOT_HELPFUL")
	ErrSelfVote          = errors.New("community: contributor cannot vote on own contribution")
	ErrInvalidVote       = errors.New("community: invalid helpfulness vote")
	ErrShareNegative     = errors.New("community: helpfulness counts must not be negative")
)

// ValidChoice reports whether the choice names a frozen helpfulness value.
func ValidChoice(c Choice) bool {
	return c == ChoiceHelpful || c == ChoiceNotHelpful
}

// VoteParams carries one helpfulness vote plus server-resolved
// attribution. ContributorRef and AuthorRef arrive from verified facts,
// never from the client body, so no caller can forget the self-vote ban.
// DecisionSequence pins the immutable revision the voter saw: votes never
// transfer to a later revision.
type VoteParams struct {
	ID                 string
	ContributorRef     string
	AuthorRef          string
	ObservationID      string
	DecisionSequence   int64
	Choice             Choice
	ClientSubmissionID string
	ReceivedAt         time.Time
	PolicyVersion      string
}

// StoredVote is one persisted helpfulness vote. Changes rewrite Choice in
// place; removals tombstone via DeletedAt. Old-sequence votes stay
// restricted audit history when the target moves on.
type StoredVote struct {
	ID                 string
	ContributorRef     string
	ObservationID      string
	DecisionSequence   int64
	Choice             Choice
	CreatedAt          time.Time
	DeletedAt          *time.Time
	ClientSubmissionID string
}

// Live reports whether the vote counts toward its tally. The
// ContributorRef binding also lets account erasure tombstone live votes.
func (v StoredVote) Live() bool {
	return v.DeletedAt == nil
}

// HelpfulnessRecorded is the domain event recorded at construction.
type HelpfulnessRecorded struct {
	VoteID        string
	ObservationID string
	OccurredAt    time.Time
}

// Name derives the emitted event name.
func (e HelpfulnessRecorded) Name() string { return "HelpfulnessRecorded" }

// NewVote validates and freezes one helpfulness vote.
func NewVote(p VoteParams) (StoredVote, HelpfulnessRecorded, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ContributorRef) == "" ||
		strings.TrimSpace(p.AuthorRef) == "" || strings.TrimSpace(p.ObservationID) == "" ||
		strings.TrimSpace(p.ClientSubmissionID) == "" {
		return StoredVote{}, HelpfulnessRecorded{}, ErrInvalidVote
	}
	if !ValidChoice(p.Choice) {
		return StoredVote{}, HelpfulnessRecorded{}, ErrVoteChoiceInvalid
	}
	if p.DecisionSequence < 1 {
		return StoredVote{}, HelpfulnessRecorded{}, ErrInvalidVote
	}
	if p.ContributorRef == p.AuthorRef {
		return StoredVote{}, HelpfulnessRecorded{}, ErrSelfVote
	}
	if p.ReceivedAt.IsZero() {
		return StoredVote{}, HelpfulnessRecorded{}, ErrInvalidVote
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	v := StoredVote{
		ID: p.ID, ContributorRef: p.ContributorRef, ObservationID: p.ObservationID,
		DecisionSequence: p.DecisionSequence, Choice: p.Choice,
		CreatedAt: p.ReceivedAt, ClientSubmissionID: p.ClientSubmissionID,
	}
	return v, HelpfulnessRecorded{VoteID: v.ID, ObservationID: v.ObservationID, OccurredAt: v.CreatedAt}, nil
}

// Share is one exact helpfulness snapshot. BasisPoints is
// floor(10000*helpful/total), present only when HasVotes. Zero votes
// leaves it absent, never 0%.
type Share struct {
	Helpful     int64
	NotHelpful  int64
	BasisPoints int64
	HasVotes    bool
}

// ComputeShare derives the helpfulness snapshot. Negative counts refuse:
// denominators come from constrained stores, never client arithmetic.
func ComputeShare(helpful, notHelpful int64) (Share, error) {
	if helpful < 0 || notHelpful < 0 {
		return Share{}, ErrShareNegative
	}
	if helpful+notHelpful == 0 {
		return Share{HasVotes: false}, nil
	}
	return Share{
		Helpful:     helpful,
		NotHelpful:  notHelpful,
		BasisPoints: ShareScale * helpful / (helpful + notHelpful),
		HasVotes:    true,
	}, nil
}

// Tally is one exact per-revision count snapshot.
type Tally struct {
	ObservationID    string
	DecisionSequence int64
	Helpful          int64
	NotHelpful       int64
}

// Share folds the tally through the frozen basis-point math.
func (t Tally) Share() (Share, error) {
	return ComputeShare(t.Helpful, t.NotHelpful)
}
